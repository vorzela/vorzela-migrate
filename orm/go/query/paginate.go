package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

// PageStyle is the caller's preferred pagination strategy.
type PageStyle string

const (
	PageOffset PageStyle = "offset"
	PageCursor PageStyle = "cursor"
)

// PageRequest is a unified pagination input (offset or cursor).
type PageRequest struct {
	Style   PageStyle // default offset
	Page    int       // 1-based page number (offset style)
	PerPage int       // page size (both); default 15
	// Cursor style:
	Cursor  string // opaque cursor from previous PageResult.NextCursor
	OrderBy string // cursor column (default primary key)
	Desc    bool
}

// PageResult is a page of rows plus navigation metadata.
type PageResult[T any] struct {
	Data       []T    `json:"data"`
	Style      string `json:"style"`
	PerPage    int    `json:"per_page"`
	Page       int    `json:"page,omitempty"`         // offset only — current 1-based page
	Pages      int    `json:"pages,omitempty"`        // offset only — total number of pages
	LastPage   int    `json:"last_page,omitempty"`    // same as Pages (Laravel-style alias)
	Total      *int64 `json:"total,omitempty"`        // offset only — total matching rows
	NextCursor string `json:"next_cursor,omitempty"`  // cursor only
	HasMore    bool   `json:"has_more"`
}

type cursorPayload struct {
	V any    `json:"v"`
	C string `json:"c"`
}

// Paginate runs offset or cursor pagination based on req.Style / user preference.
func (b *Builder[T]) Paginate(ctx context.Context, db DB, req PageRequest) (*PageResult[T], error) {
	if req.PerPage <= 0 {
		req.PerPage = 15
	}
	if req.Style == "" || req.Style == PageOffset {
		return b.paginateOffset(ctx, db, req)
	}
	return b.paginateCursor(ctx, db, req)
}

func (b *Builder[T]) paginateOffset(ctx context.Context, db DB, req PageRequest) (*PageResult[T], error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	cp := *b
	cp.limit = req.PerPage
	cp.offset = (req.Page - 1) * req.PerPage

	rows, err := cp.Get(ctx, db)
	if err != nil {
		return nil, err
	}
	total, err := b.Count(ctx, db)
	if err != nil {
		return nil, err
	}
	pages := pageCount(total, req.PerPage)
	hasMore := req.Page < pages
	return &PageResult[T]{
		Data:     rows,
		Style:    string(PageOffset),
		PerPage:  req.PerPage,
		Page:     req.Page,
		Pages:    pages,
		LastPage: pages,
		Total:    &total,
		HasMore:  hasMore,
	}, nil
}

// pageCount returns how many pages are needed for total rows at perPage size.
func pageCount(total int64, perPage int) int {
	if perPage <= 0 {
		return 0
	}
	if total <= 0 {
		return 0
	}
	return int((total + int64(perPage) - 1) / int64(perPage))
}

func (b *Builder[T]) paginateCursor(ctx context.Context, db DB, req PageRequest) (*PageResult[T], error) {
	col := req.OrderBy
	if col == "" {
		col = b.meta.PrimaryKey
	}
	cp := *b
	dir := "ASC"
	op := ">"
	if req.Desc {
		dir = "DESC"
		op = "<"
	}
	// Replace order for stable cursor pages
	cp.orderBy = []order{{col: col, dir: dir}}
	if req.Cursor != "" {
		val, err := decodeCursor(req.Cursor)
		if err != nil {
			return nil, fmt.Errorf("vorm/query: invalid cursor: %w", err)
		}
		cp.wheres = append(cp.wheres, pred{col: col, op: op, arg: val})
	}
	cp.limit = req.PerPage + 1 // peek one more
	rows, err := cp.Get(ctx, db)
	if err != nil {
		return nil, err
	}
	hasMore := len(rows) > req.PerPage
	if hasMore {
		rows = rows[:req.PerPage]
	}
	var next string
	if hasMore && len(rows) > 0 {
		// Caller should prefer WithMapper that fills comparable fields;
		// cursor encodes the last row's order column via CursorValue hook.
		next = encodeCursor(cursorValueFromContext(ctx, rows[len(rows)-1], col))
	}
	return &PageResult[T]{
		Data:       rows,
		Style:      string(PageCursor),
		PerPage:    req.PerPage,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// OffsetPage is sugar for offset pagination.
func (b *Builder[T]) OffsetPage(ctx context.Context, db DB, page, perPage int) (*PageResult[T], error) {
	return b.Paginate(ctx, db, PageRequest{Style: PageOffset, Page: page, PerPage: perPage})
}

// CursorPage is sugar for keyset/cursor pagination.
func (b *Builder[T]) CursorPage(ctx context.Context, db DB, cursor string, perPage int, orderBy string, desc bool) (*PageResult[T], error) {
	return b.Paginate(ctx, db, PageRequest{
		Style: PageCursor, Cursor: cursor, PerPage: perPage, OrderBy: orderBy, Desc: desc,
	})
}

func encodeCursor(v any) string {
	raw, _ := json.Marshal(cursorPayload{V: v, C: "vorm1"})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s string) (any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	// JSON numbers → float64; normalize ints when possible
	switch n := p.V.(type) {
	case float64:
		if n == float64(int64(n)) {
			return int64(n), nil
		}
		return n, nil
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i, nil
		}
		return n, nil
	default:
		return p.V, nil
	}
}

type cursorValKey struct{}

// CursorValueFunc extracts the cursor column value from a scanned row.
type CursorValueFunc[T any] func(row T, column string) any

// WithCursorValue attaches a cursor extractor for CursorPage / Paginate(cursor).
func WithCursorValue[T any](ctx context.Context, fn CursorValueFunc[T]) context.Context {
	return context.WithValue(ctx, cursorValKey{}, fn)
}

func cursorValueFromContext[T any](ctx context.Context, row T, col string) any {
	if v := ctx.Value(cursorValKey{}); v != nil {
		if fn, ok := v.(CursorValueFunc[T]); ok {
			return fn(row, col)
		}
	}
	return nil
}
