package query

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Event describes one executed statement. Args is nil unless the logger opts in,
// because bound values routinely contain personal data.
type Event struct {
	Op       string
	Table    string
	SQL      string
	Args     []any
	ArgCount int
	Rows     int
	Duration time.Duration
	Err      error
}

// Logger receives one Event per executed statement.
type Logger interface {
	LogQuery(ctx context.Context, ev Event)
}

// LoggerFunc adapts a function to Logger.
type LoggerFunc func(ctx context.Context, ev Event)

func (f LoggerFunc) LogQuery(ctx context.Context, ev Event) { f(ctx, ev) }

const loggerKey ctxKey = 2

// WithLogger attaches a Logger for the duration of ctx. It takes precedence
// over the process-wide default.
func WithLogger(ctx context.Context, l Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey, l)
}

var defaultLogger atomic.Pointer[Logger]

// SetDefaultLogger installs a process-wide Logger used when ctx carries none.
// Pass nil to disable.
func SetDefaultLogger(l Logger) {
	if l == nil {
		defaultLogger.Store(nil)
		return
	}
	defaultLogger.Store(&l)
}

func loggerFrom(ctx context.Context) Logger {
	if ctx != nil {
		if v := ctx.Value(loggerKey); v != nil {
			if l, ok := v.(Logger); ok {
				return l
			}
		}
	}
	if p := defaultLogger.Load(); p != nil {
		return *p
	}
	return nil
}

// observation carries timing for one statement.
type observation struct {
	logger Logger
	op     string
	table  string
	sql    string
	args   []any
	start  time.Time
}

func observe(ctx context.Context, op, table, sqlText string, args []any) observation {
	l := loggerFrom(ctx)
	if l == nil {
		return observation{}
	}
	return observation{logger: l, op: op, table: table, sql: sqlText, args: args, start: time.Now()}
}

// done emits the Event. It returns err unchanged so callers can `return o.done(ctx, n, err)`.
func (o observation) done(ctx context.Context, rows int, err error) error {
	if o.logger == nil {
		return err
	}
	o.logger.LogQuery(ctx, Event{
		Op:       o.op,
		Table:    o.table,
		SQL:      o.sql,
		Args:     o.args,
		ArgCount: len(o.args),
		Rows:     rows,
		Duration: time.Since(o.start),
		Err:      err,
	})
	return err
}

// SlogLogger writes query events to a *slog.Logger.
type SlogLogger struct {
	Logger *slog.Logger
	// Level for successful statements. Failures always log at Error.
	Level slog.Level
	// SlowThreshold promotes slower statements to Warn. Zero disables.
	SlowThreshold time.Duration
	// IncludeArgs logs bound values. Leave false unless the data is non-sensitive.
	IncludeArgs bool
	// MaxSQLLength truncates very long statements. Zero means no limit.
	MaxSQLLength int
}

// NewSlogLogger returns a Logger writing to l at Debug level, with a 200ms
// slow-query threshold and bound values omitted.
func NewSlogLogger(l *slog.Logger) *SlogLogger {
	if l == nil {
		l = slog.Default()
	}
	return &SlogLogger{Logger: l, Level: slog.LevelDebug, SlowThreshold: 200 * time.Millisecond}
}

func (s *SlogLogger) LogQuery(ctx context.Context, ev Event) {
	if s == nil || s.Logger == nil {
		return
	}
	sqlText := ev.SQL
	if s.MaxSQLLength > 0 && len(sqlText) > s.MaxSQLLength {
		sqlText = sqlText[:s.MaxSQLLength] + "…"
	}
	attrs := []any{
		slog.String("op", ev.Op),
		slog.String("sql", sqlText),
		slog.Duration("duration", ev.Duration),
		slog.Int("args", ev.ArgCount),
	}
	if ev.Table != "" {
		attrs = append(attrs, slog.String("table", ev.Table))
	}
	if ev.Err == nil {
		attrs = append(attrs, slog.Int("rows", ev.Rows))
	}
	if s.IncludeArgs && len(ev.Args) > 0 {
		attrs = append(attrs, slog.Any("values", ev.Args))
	}

	switch {
	case ev.Err != nil:
		attrs = append(attrs,
			slog.String("error", ev.Err.Error()),
			slog.String("kind", string(Classify(ev.Err))),
		)
		if code := Code(ev.Err); code != "" {
			attrs = append(attrs, slog.String("code", code))
		}
		if c := Constraint(ev.Err); c != "" {
			attrs = append(attrs, slog.String("constraint", c))
		}
		s.Logger.LogAttrs(ctx, slog.LevelError, "vorm query failed", toAttrs(attrs)...)
	case s.SlowThreshold > 0 && ev.Duration >= s.SlowThreshold:
		s.Logger.LogAttrs(ctx, slog.LevelWarn, "vorm slow query", toAttrs(attrs)...)
	default:
		s.Logger.LogAttrs(ctx, s.Level, "vorm query", toAttrs(attrs)...)
	}
}

func toAttrs(vals []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(vals))
	for _, v := range vals {
		if a, ok := v.(slog.Attr); ok {
			out = append(out, a)
		}
	}
	return out
}
