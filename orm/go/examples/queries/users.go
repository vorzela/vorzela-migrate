// Package queries holds hand-written // vorm:query stubs. Run `vorm generate`
// to lower each fluent chain into parameterized SQL under ./vorm/gen.
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

// ListActiveAdults — fluent IR for vorm generate → gen.ListActiveAdults.
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

// ListActiveAdultsFind — TypeORM-style Find options (runtime only: the options
// struct is built at call time).
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

// GetUserOrFail — First that turns "no rows" into an error.
//
// vorm:query name=GetUserOrFail
func GetUserOrFail(ctx context.Context, db query.DB, id int64) (*User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Where("id", id).FirstOrFail(ctx, db)
}

// ListUsersByIDs — IN over a slice: the placeholder count is only known at call
// time, so the generated code assembles that one group and binds every element.
//
// vorm:query name=ListUsersByIDs
func ListUsersByIDs(ctx context.Context, db query.DB, ids ...any) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.WhereIn("id", ids...).OrderBy("id").Get(ctx, db)
}

// ListUsersExcept — NOT IN over a slice, plus a fixed filter.
//
// vorm:query name=ListUsersExcept
func ListUsersExcept(ctx context.Context, db query.DB, ids ...any) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Where("active", true).WhereNotIn("id", ids...).OrderByDesc("created_at").Get(ctx, db)
}

// ListUsersByStatus — IN over a fixed set, which stays a const statement.
//
// vorm:query name=ListUsersByStatus
func ListUsersByStatus(ctx context.Context, db query.DB) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.WhereIn("age", 18, 21, 65).OrderBy("age").Get(ctx, db)
}

// ListTrashedUsers — WithTrashed drops the automatic deleted_at filter, then an
// explicit IS NOT NULL keeps only the soft-deleted rows.
//
// vorm:query name=ListTrashedUsers
func ListTrashedUsers(ctx context.Context, db query.DB) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.WithTrashed().WhereNotNull("deleted_at").OrderByDesc("deleted_at").Get(ctx, db)
}

// ListActiveOrAdult — OR groups.
//
// vorm:query name=ListActiveOrAdult
func ListActiveOrAdult(ctx context.Context, db query.DB) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Where("active", true).OrWhere("age", ">=", 18).OrderBy("id").Get(ctx, db)
}

// SearchUsers — case-insensitive search across name + email with a bound LIMIT,
// so one prepared statement serves every page size.
//
// vorm:query name=SearchUsers
func SearchUsers(ctx context.Context, db query.DB, q string, limit int) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.WhereSearch([]string{"name", "email"}, q).OrderBy("name").Limit(limit).Get(ctx, db)
}

// ListUsersWithPosts — join + explicit projection across both tables.
//
// vorm:query name=ListUsersWithPosts
func ListUsersWithPosts(ctx context.Context, db query.DB) ([]User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.New().
		Join("posts", "posts.user_id = users.id").
		Select("id", "email", "name", "active", "age", "created_at", "updated_at", "deleted_at").
		Where("active", true).
		GroupBy("id").
		OrderBy("name").
		Get(ctx, db)
}

// CountActiveUsers — COUNT(*) with the same predicate pipeline.
//
// vorm:query name=CountActiveUsers
func CountActiveUsers(ctx context.Context, db query.DB) (int64, error) {
	return Users.Where("active", true).Count(ctx, db)
}

// UserExistsByEmail — existence probe (SELECT 1 … LIMIT 1).
//
// vorm:query name=UserExistsByEmail
func UserExistsByEmail(ctx context.Context, db query.DB, email string) (bool, error) {
	return Users.Where("email", email).Exists(ctx, db)
}

// PaginateActiveUsers — offset pagination: one page query plus one COUNT.
//
// vorm:query name=PaginateActiveUsers
func PaginateActiveUsers(ctx context.Context, db query.DB, page, perPage int) (*query.PageResult[User], error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Where("active", true).OrderBy("id").OffsetPage(ctx, db, page, perPage)
}

// PaginateUsersCursor — keyset cursor; stateful, so it stays on the builder.
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
		Style:   query.PageCursor,
		Cursor:  cursor,
		PerPage: perPage,
		OrderBy: "id",
	})
}

// LockUserForUpdate — row lock for read-modify-write inside a transaction.
//
// vorm:query name=LockUserForUpdate
func LockUserForUpdate(ctx context.Context, db query.DB, id int64) (*User, error) {
	ctx = query.WithMapper(ctx, scanUser)
	return Users.Where("id", id).LockForUpdate().First(ctx, db)
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

// RestoreUser — clear deleted_at again.
//
// vorm:query name=RestoreUser
func RestoreUser(ctx context.Context, db query.DB, id int64) (int64, error) {
	return Users.Where("id", id).Restore(ctx, db)
}

// ForceDeleteUser — permanent delete (ignores soft deletes).
//
// vorm:query name=ForceDeleteUser
func ForceDeleteUser(ctx context.Context, db query.DB, id int64) (int64, error) {
	return Users.ForceDelete(ctx, db, id)
}

// ActivateUsersInTx — transaction + lock + update; multi-statement bodies stay
// on the runtime builder.
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
