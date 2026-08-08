package query

import "context"

type ctxKey int

const mapperKey ctxKey = 1

// RowMapper scans one result row into T. Generated code or hand-written mappers
// keep column order aligned with Builder.Select / Meta.Columns.
type RowMapper[T any] func(rows Rows) (T, error)

// WithMapper attaches a RowMapper for Get/First.
func WithMapper[T any](ctx context.Context, m RowMapper[T]) context.Context {
	return context.WithValue(ctx, mapperKey, m)
}

func rowMapperFromContext[T any](ctx context.Context) RowMapper[T] {
	v := ctx.Value(mapperKey)
	if v == nil {
		return nil
	}
	m, ok := v.(RowMapper[T])
	if !ok {
		return nil
	}
	return m
}
