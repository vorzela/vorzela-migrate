package query

// Operator for Where / Find maps (TypeORM-style helpers).
type Operator struct {
	Op    string
	Value any
}

func Eq(v any) Operator              { return Operator{Op: "=", Value: v} }
func Not(v any) Operator             { return Operator{Op: "<>", Value: v} }
func MoreThan(v any) Operator        { return Operator{Op: ">", Value: v} }
func MoreThanOrEqual(v any) Operator { return Operator{Op: ">=", Value: v} }
func LessThan(v any) Operator        { return Operator{Op: "<", Value: v} }
func LessThanOrEqual(v any) Operator { return Operator{Op: "<=", Value: v} }
func Like(v any) Operator            { return Operator{Op: "LIKE", Value: v} }
func ILike(v any) Operator           { return Operator{Op: "ILIKE", Value: v} }
func In(vals ...any) Operator        { return Operator{Op: "IN", Value: vals} }
func NotIn(vals ...any) Operator     { return Operator{Op: "NOT IN", Value: vals} }
func IsNull() Operator               { return Operator{Op: "IS NULL", Value: nil} }
func IsNotNull() Operator            { return Operator{Op: "IS NOT NULL", Value: nil} }

// Sort direction for FindOptions.Order.
type Sort string

const (
	Asc  Sort = "ASC"
	Desc Sort = "DESC"
)

// FindOptions mirrors TypeORM-style find:
//
//	Users.Find(ctx, db, query.FindOptions{
//	    Where: query.WhereMap{"active": true, "age": query.MoreThan(18)},
//	    Order: query.OrderMap{"name": query.Asc},
//	    Take: 10,
//	})
type FindOptions struct {
	Where  WhereMap
	Order  OrderMap
	Take   int
	Skip   int
	Select []string // subset of Meta.Columns; empty = all registered columns
}

// WhereMap is column → value or Operator.
type WhereMap map[string]any

// OrderMap is column → Asc|Desc|"ASC"|"DESC".
type OrderMap map[string]any
