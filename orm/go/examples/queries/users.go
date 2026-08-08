package queries

import (
	"context"

	"github.com/vorzela/vorm"
	"github.com/vorzela/vorm/query"
)

// User model — columns listed once on the entity (never SELECT *).
type User struct {
	vorm.Model
	Email  string `json:"email" db:"email"`
	Name   string `json:"name" db:"name"`
	Active bool   `json:"active" db:"active"`
	Age    int    `json:"age" db:"age"`
}

// Users is the typed query entrypoint: Users.Where(...).Get(...)
var Users = query.Model[User](query.Meta{
	Table: "users",
	Columns: []string{
		"id", "email", "name", "active", "age",
		"created_at", "updated_at", "deleted_at",
	},
	SoftDeletes: true,
})

// scanUser maps selected columns in Meta order (narrow Select if you change order).
func scanUser(rows query.Rows) (User, error) {
	var u User
	err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Active, &u.Age, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt)
	return u, err
}

// ListActiveAdults — fluent IR for vorm generate → gen.ListActiveAdults ([]models.User).
// Prefer calling the generated func in app code after `vorm generate`.
//
// vorm:query name=ListActiveAdults
func ListActiveAdults(ctx context.Context, db query.DB) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.
		Where("active", true).
		Where("age", ">", 18).
		OrderBy("name").
		Limit(10).
		Get(ctx, db)
}

// ListActiveAdultsFind — TypeORM-style Find options.
//
// vorm:query name=ListActiveAdultsFind
func ListActiveAdultsFind(ctx context.Context, db query.DB) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Find(ctx, db, query.FindOptions{
		Where: query.WhereMap{
			"active": true,
			"age":    query.MoreThan(18),
		},
		Order: query.OrderMap{"name": query.Asc},
		Take:  10,
	})
}

// GetUserByEmail — equality Where + First.
//
// vorm:query name=GetUserByEmail
func GetUserByEmail(ctx context.Context, db query.DB, email string) (*User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Where("email", email).First(ctx, db)
}

// CreateUser — easy CRUD insert (explicit columns).
//
// vorm:query name=CreateUser
func CreateUser(ctx context.Context, db query.DB, email, name string, age int) (int64, error) {
	return Users.Create(ctx, db, map[string]any{
		"email":  email,
		"name":   name,
		"active": true,
		"age":    age,
	})
}

// DeactivateUser — update by id.
//
// vorm:query name=DeactivateUser
func DeactivateUser(ctx context.Context, db query.DB, id int64) (int64, error) {
	return Users.Where("id", id).Update(ctx, db, map[string]any{"active": false})
}

// SoftDeleteUser — soft delete (Meta.SoftDeletes).
//
// vorm:query name=SoftDeleteUser
func SoftDeleteUser(ctx context.Context, db query.DB, id int64) (int64, error) {
	return Users.SoftDelete(ctx, db, id)
}

// ForceDeleteUser — permanent delete (ignores soft deletes).
//
// vorm:query name=ForceDeleteUser
func ForceDeleteUser(ctx context.Context, db query.DB, id int64) (int64, error) {
	return Users.ForceDelete(ctx, db, id)
}

// SearchUsers — complex search across name + email (ILIKE / LIKE).
//
// vorm:query name=SearchUsers
func SearchUsers(ctx context.Context, db query.DB, q string, limit int) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.WhereSearch([]string{"name", "email"}, q).OrderBy("name").Limit(limit).Get(ctx, db)
}

// PaginateUsersOffset — classic page/perPage (+ total + pages/last_page).
//
// vorm:query name=PaginateUsersOffset
func PaginateUsersOffset(ctx context.Context, db query.DB, page, perPage int) (*query.PageResult[User], error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Paginate(ctx, db, query.PageRequest{
		Style:   query.PageOffset, // user preference
		Page:    page,
		PerPage: perPage,
	})
	// result.Pages / result.LastPage = total page count; result.Total = row count
}

// PaginateUsersCursor — keyset cursor (prefer for large tables).
//
// vorm:query name=PaginateUsersCursor
func PaginateUsersCursor(ctx context.Context, db query.DB, cursor string, perPage int) (*query.PageResult[User], error) {
	ctx = query.WithMapper(ctx, scanUser)
	ctx = query.WithCursorValue(ctx, func(u User, col string) any {
		if col == "id" {
			return u.ID
		}
		return nil
	})
	return Users.Paginate(ctx, db, query.PageRequest{
		Style:   query.PageCursor, // user preference
		Cursor:  cursor,
		PerPage: perPage,
		OrderBy: "id",
	})
}

// ActivateUsersInTx — transaction + lock + update (performant, race-safe).
//
// vorm:query name=ActivateUsersInTx
func ActivateUsersInTx(ctx context.Context, db query.Beginner, ids ...any) error {
	return query.Transaction(ctx, db, func(ctx context.Context, tx query.Tx) error {
		ctx = query.WithMapper(ctx, scanUser)
		users, err := Users.WhereIn("id", ids...).LockForUpdate().Get(ctx, tx)
		if err != nil {
			return err
		}
		for _, u := range users {
			if _, err := Users.Where("id", u.ID).Update(ctx, tx, map[string]any{"active": true}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListDistinctActiveEmails — DISTINCT + filter (no SELECT *).
//
// vorm:query name=ListDistinctActiveEmails
func ListDistinctActiveEmails(ctx context.Context, db query.DB) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.New().
		Distinct().
		Select("id", "email", "name", "active", "age", "created_at", "updated_at", "deleted_at").
		Where("active", true).
		OrderBy("email").
		Get(ctx, db)
}
