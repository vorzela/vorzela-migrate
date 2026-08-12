package query

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type relUser struct {
	ID    int64  `db:"id"`
	Email string `db:"email"`

	Posts []relPost `db:"-"`
	Team  *relTeam  `db:"-"`
	Tags  []relTag  `db:"-"`

	TeamID int64 `db:"team_id"`
}

type relPost struct {
	ID     int64  `db:"id"`
	UserID int64  `db:"user_id"`
	Title  string `db:"title"`
}

type relTeam struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

type relTag struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

var (
	relUsers = Model[relUser](Meta{Table: "rel_users", Columns: []string{"id", "email", "team_id"}})
	relPosts = Model[relPost](Meta{Table: "rel_posts", Columns: []string{"id", "user_id", "title"}})
	relTeams = Model[relTeam](Meta{Table: "rel_teams", Columns: []string{"id", "name"}})
	relTags  = Model[relTag](Meta{Table: "rel_tags", Columns: []string{"id", "name"}})
)

func TestLoadHasManyIssuesOneQueryForWholeBatch(t *testing.T) {
	db := (&fakeDB{}).on("rel_posts", []string{"id", "user_id", "title"},
		[]any{int64(10), int64(1), "first"},
		[]any{int64(11), int64(1), "second"},
		[]any{int64(12), int64(2), "third"},
	)
	users := []relUser{{ID: 1}, {ID: 2}, {ID: 3}}
	parents := []*relUser{&users[0], &users[1], &users[2]}

	err := LoadHasMany(context.Background(), db, parents, HasMany[relUser, relPost]{
		Related:    relPosts,
		ForeignKey: "user_id",
		ParentKey:  func(u *relUser) any { return u.ID },
		ChildKey:   func(p *relPost) any { return p.UserID },
		Assign:     func(u *relUser, ps []relPost) { u.Posts = ps },
	})
	if err != nil {
		t.Fatal(err)
	}
	if db.count() != 1 {
		t.Fatalf("expected exactly 1 query (no N+1), got %d: %v", db.count(), db.statements())
	}
	if len(users[0].Posts) != 2 || len(users[1].Posts) != 1 || len(users[2].Posts) != 0 {
		t.Fatalf("bad grouping: %d/%d/%d", len(users[0].Posts), len(users[1].Posts), len(users[2].Posts))
	}
	if got := db.statements()[0].SQL; !strings.Contains(got, "IN (") {
		t.Fatalf("expected batched IN query, got %s", got)
	}
}

func TestLoadHasManyNormalizesMixedKeyTypes(t *testing.T) {
	// Drivers report integers with different widths; grouping must still match.
	db := (&fakeDB{}).on("rel_posts", []string{"id", "user_id", "title"},
		[]any{int64(10), int32(1), "first"},
	)
	users := []relUser{{ID: 1}}
	parents := []*relUser{&users[0]}
	err := LoadHasMany(context.Background(), db, parents, HasMany[relUser, relPost]{
		Related:    relPosts,
		ForeignKey: "user_id",
		ParentKey:  func(u *relUser) any { return int32(u.ID) },
		ChildKey:   func(p *relPost) any { return p.UserID },
		Assign:     func(u *relUser, ps []relPost) { u.Posts = ps },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(users[0].Posts) != 1 {
		t.Fatalf("int32/int64 keys did not match")
	}
}

func TestLoadBelongsTo(t *testing.T) {
	db := (&fakeDB{}).on("rel_teams", []string{"id", "name"},
		[]any{int64(5), "Platform"},
	)
	users := []relUser{{ID: 1, TeamID: 5}, {ID: 2, TeamID: 5}, {ID: 3, TeamID: 9}}
	parents := []*relUser{&users[0], &users[1], &users[2]}

	err := LoadBelongsTo(context.Background(), db, parents, BelongsTo[relUser, relTeam]{
		Related:   relTeams,
		ParentKey: func(u *relUser) any { return u.TeamID },
		ChildKey:  func(tm *relTeam) any { return tm.ID },
		Assign:    func(u *relUser, tm *relTeam) { u.Team = tm },
	})
	if err != nil {
		t.Fatal(err)
	}
	if db.count() != 1 {
		t.Fatalf("expected 1 query, got %d", db.count())
	}
	if users[0].Team == nil || users[0].Team.Name != "Platform" {
		t.Fatalf("owner not assigned: %+v", users[0].Team)
	}
	if users[2].Team != nil {
		t.Fatal("missing owner must stay nil")
	}
}

func TestLoadBelongsToManyUsesTwoQueries(t *testing.T) {
	db := (&fakeDB{}).
		on("rel_user_tags", []string{"user_id", "tag_id"},
			[]any{int64(1), int64(100)},
			[]any{int64(1), int64(101)},
			[]any{int64(2), int64(100)},
		).
		on("rel_tags", []string{"id", "name"},
			[]any{int64(100), "go"},
			[]any{int64(101), "sql"},
		)

	users := []relUser{{ID: 1}, {ID: 2}}
	parents := []*relUser{&users[0], &users[1]}

	err := LoadBelongsToMany(context.Background(), db, parents, BelongsToMany[relUser, relTag]{
		Related:         relTags,
		PivotTable:      "rel_user_tags",
		PivotParentKey:  "user_id",
		PivotRelatedKey: "tag_id",
		ParentKey:       func(u *relUser) any { return u.ID },
		ChildKey:        func(tg *relTag) any { return tg.ID },
		Assign:          func(u *relUser, ts []relTag) { u.Tags = ts },
	})
	if err != nil {
		t.Fatal(err)
	}
	if db.count() != 2 {
		t.Fatalf("expected 2 queries for the whole batch, got %d", db.count())
	}
	if len(users[0].Tags) != 2 || len(users[1].Tags) != 1 {
		t.Fatalf("bad pivot grouping: %d/%d", len(users[0].Tags), len(users[1].Tags))
	}
}

func TestWithRunsRegisteredLoader(t *testing.T) {
	RegisterRelation(Relation{
		Name:       "posts",
		Kind:       RelationHasMany,
		Table:      "rel_posts",
		LocalKey:   "id",
		ForeignKey: "user_id",
	}, func(ctx context.Context, db DB, parents []*relUser) error {
		return LoadHasMany(ctx, db, parents, HasMany[relUser, relPost]{
			Related:    relPosts,
			ForeignKey: "user_id",
			ParentKey:  func(u *relUser) any { return u.ID },
			ChildKey:   func(p *relPost) any { return p.UserID },
			Assign:     func(u *relUser, ps []relPost) { u.Posts = ps },
		})
	})

	db := (&fakeDB{}).
		on("rel_users", []string{"id", "email", "team_id"},
			[]any{int64(1), "a@x.io", int64(5)},
		).
		on("rel_posts", []string{"id", "user_id", "title"},
			[]any{int64(10), int64(1), "hello"},
		)

	got, err := relUsers.With("posts").Get(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Posts) != 1 || got[0].Posts[0].Title != "hello" {
		t.Fatalf("relation not eager-loaded: %+v", got)
	}
	if names := Relations[relUser](); len(names) == 0 || names[0].Name != "posts" {
		t.Fatalf("relation metadata missing: %+v", names)
	}
}

func TestWithUnknownRelationIsValidationError(t *testing.T) {
	db := (&fakeDB{}).on("rel_users", []string{"id", "email", "team_id"},
		[]any{int64(1), "a@x.io", int64(5)},
	)
	_, err := relUsers.With("nope").Get(context.Background(), db)
	if err == nil {
		t.Fatal("expected error for unknown relation")
	}
	if !IsValidationError(err) {
		t.Fatalf("want validation error, got %v (kind %s)", err, Classify(err))
	}
}

func TestNormalizeKey(t *testing.T) {
	if normalizeKey(int32(5)) != normalizeKey(int64(5)) {
		t.Error("int widths must normalize together")
	}
	if normalizeKey([]byte("a")) != normalizeKey("a") {
		t.Error("[]byte and string must normalize together")
	}
	if normalizeKey(nil) != nil {
		t.Error("nil stays nil")
	}
	v := 7
	if normalizeKey(&v) != normalizeKey(7) {
		t.Error("pointer must dereference")
	}
	var np *int
	if normalizeKey(np) != nil {
		t.Error("nil pointer must normalize to nil")
	}
}

func TestLoadHasManySkipsQueryWhenNoParents(t *testing.T) {
	db := &fakeDB{}
	err := LoadHasMany(context.Background(), db, nil, HasMany[relUser, relPost]{
		Related:    relPosts,
		ForeignKey: "user_id",
		ParentKey:  func(u *relUser) any { return u.ID },
		ChildKey:   func(p *relPost) any { return p.UserID },
		Assign:     func(u *relUser, ps []relPost) { u.Posts = ps },
	})
	if err != nil {
		t.Fatal(err)
	}
	if db.count() != 0 {
		t.Fatalf("expected no query, got %d", db.count())
	}
}

func TestLoadHasManyRequiresConfiguration(t *testing.T) {
	users := []relUser{{ID: 1}}
	err := LoadHasMany(context.Background(), &fakeDB{}, []*relUser{&users[0]}, HasMany[relUser, relPost]{})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("want validation error, got %v", err)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatal("expected *query.Error")
	}
}
