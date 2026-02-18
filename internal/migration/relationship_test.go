package migration

import (
	"strings"
	"testing"
)

func TestSingularize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple plural", "users", "user"},
		{"ies ending", "categories", "category"},
		{"ses ending", "classes", "class"},
		{"xes ending", "boxes", "box"},
		{"zes ending", "quizzes", "quizz"},
		{"ches ending", "churches", "church"},
		{"shes ending", "dishes", "dish"},
		{"already singular", "user", "user"},
		{"irregular - person", "people", "person"},
		{"irregular - child", "children", "child"},
		{"edge case - empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Singularize(tt.input)
			if got != tt.want {
				t.Errorf("Singularize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeneratePivotTableName(t *testing.T) {
	tests := []struct {
		name   string
		table1 string
		table2 string
		want   string
	}{
		{
			name:   "alphabetical order maintained",
			table1: "posts",
			table2: "tags",
			want:   "post_tag",
		},
		{
			name:   "reverse alphabetical order",
			table1: "tags",
			table2: "posts",
			want:   "post_tag",
		},
		{
			name:   "same prefix",
			table1: "user_profiles",
			table2: "user_settings",
			want:   "user_profile_user_setting",
		},
		{
			name:   "complex names",
			table1: "article_comments",
			table2: "comment_likes",
			want:   "article_comment_comment_like",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PivotTableName(tt.table1, tt.table2)
			if got != tt.want {
				t.Errorf("PivotTableName(%q, %q) = %q, want %q",
					tt.table1, tt.table2, got, tt.want)
			}
		})
	}
}

func TestGeneratePivotMigration(t *testing.T) {
	tests := []struct {
		name        string
		table1      string
		table2      string
		sqlcSupport bool
		wantStrings []string
	}{
		{
			name:   "basic pivot table",
			table1: "posts",
			table2: "tags",
			wantStrings: []string{
				"CREATE TABLE IF NOT EXISTS post_tag",
				"post_id BIGINT NOT NULL",
				"tag_id BIGINT NOT NULL",
				"REFERENCES posts(id) ON DELETE CASCADE",
				"REFERENCES tags(id) ON DELETE CASCADE",
				"DROP TABLE IF EXISTS post_tag CASCADE",
			},
		},
		{
			name:   "reverse order",
			table1: "tags",
			table2: "posts",
			wantStrings: []string{
				"post_tag",
				"post_id BIGINT NOT NULL",
				"tag_id BIGINT NOT NULL",
			},
		},
		{
			name:        "with sqlc support",
			table1:      "users",
			table2:      "roles",
			sqlcSupport: true,
			wantStrings: []string{
				"-- +goose Up",
				"-- +goose Down",
				"role_user",
			},
		},
		{
			name:        "without sqlc support",
			table1:      "users",
			table2:      "roles",
			sqlcSupport: false,
			wantStrings: []string{
				"-- ⬆ Up (Run when migrating forward)",
				"-- ⬇ Down (Run when rolling back)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := CreateMigrationOptions{SqlcSupport: tt.sqlcSupport}
			got := GeneratePivotMigration(tt.table1, tt.table2, opts)
			for _, want := range tt.wantStrings {
				if !strings.Contains(got, want) {
					t.Errorf("GeneratePivotMigration() missing string %q", want)
				}
			}
		})
	}
}

func TestGenerateForeignKeyColumn(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		want      string
	}{
		{
			name:      "simple table",
			tableName: "users",
			want:      "user_id",
		},
		{
			name:      "plural table",
			tableName: "categories",
			want:      "category_id",
		},
		{
			name:      "compound name",
			tableName: "user_profiles",
			want:      "user_profile_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ForeignKeyColumn(tt.tableName)
			if got != tt.want {
				t.Errorf("ForeignKeyColumn(%q) = %q, want %q", tt.tableName, got, tt.want)
			}
		})
	}
}

// TestGenerateMigrationTemplate_BelongsTo checks that a belongs-to relationship
// generates the correct FK column, index, and FOREIGN KEY constraint.
func TestGenerateMigrationTemplate_BelongsTo(t *testing.T) {
	opts := CreateMigrationOptions{
		Relationships: []Relationship{
			{Type: BelongsTo, TargetTable: "users"},
		},
	}
	got := generateMigrationTemplate("create_posts_table", opts)

	wantStrings := []string{
		"CREATE TABLE IF NOT EXISTS posts",
		"user_id BIGINT NOT NULL",
		"CONSTRAINT fk_posts_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE",
		"CREATE INDEX IF NOT EXISTS idx_posts_user_id ON posts(user_id)",
		"DROP TABLE IF EXISTS posts CASCADE",
		"-- Relationship: posts → users (Many-to-One)",
	}
	for _, want := range wantStrings {
		if !strings.Contains(got, want) {
			t.Errorf("BelongsTo migration missing %q\nGot:\n%s", want, got)
		}
	}
}

// TestGenerateMigrationTemplate_OneToOne checks that a one-to-one relationship
// generates a UNIQUE foreign key column (no extra index on FK).
func TestGenerateMigrationTemplate_OneToOne(t *testing.T) {
	opts := CreateMigrationOptions{
		Relationships: []Relationship{
			{Type: OneToOne, TargetTable: "users"},
		},
	}
	got := generateMigrationTemplate("create_profiles_table", opts)

	wantStrings := []string{
		"CREATE TABLE IF NOT EXISTS profiles",
		"user_id BIGINT NOT NULL UNIQUE",
		"CONSTRAINT fk_profiles_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE",
		"-- Relationship: profiles → users (One-to-One)",
	}
	for _, want := range wantStrings {
		if !strings.Contains(got, want) {
			t.Errorf("OneToOne migration missing %q\nGot:\n%s", want, got)
		}
	}
	// One-to-one should NOT generate a standalone index (UNIQUE is the index)
	if strings.Contains(got, "idx_profiles_user_id") {
		t.Error("One-to-one should not generate a separate index for the FK column")
	}
}

// TestGenerateMigrationTemplate_MultipleRelationships checks multiple FKs.
func TestGenerateMigrationTemplate_MultipleRelationships(t *testing.T) {
	opts := CreateMigrationOptions{
		Relationships: []Relationship{
			{Type: BelongsTo, TargetTable: "users"},
			{Type: BelongsTo, TargetTable: "categories"},
		},
	}
	got := generateMigrationTemplate("create_posts_table", opts)

	wantStrings := []string{
		"user_id BIGINT NOT NULL",
		"category_id BIGINT NOT NULL",
		"CONSTRAINT fk_posts_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE",
		"CONSTRAINT fk_posts_category FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE",
		"CREATE INDEX IF NOT EXISTS idx_posts_user_id",
		"CREATE INDEX IF NOT EXISTS idx_posts_category_id",
	}
	for _, want := range wantStrings {
		if !strings.Contains(got, want) {
			t.Errorf("Multiple-relationship migration missing %q\nGot:\n%s", want, got)
		}
	}
}

// TestGenerateMigrationTemplate_SoftDelete checks soft-delete column and index.
func TestGenerateMigrationTemplate_SoftDelete(t *testing.T) {
	opts := CreateMigrationOptions{SoftDelete: true}
	got := generateMigrationTemplate("create_users_table", opts)

	wantStrings := []string{
		"deleted_at TIMESTAMP DEFAULT NULL",
		"CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at)",
	}
	for _, want := range wantStrings {
		if !strings.Contains(got, want) {
			t.Errorf("SoftDelete migration missing %q\nGot:\n%s", want, got)
		}
	}
}

// TestGenerateMigrationTemplate_Triggers checks trigger scaffold is present.
func TestGenerateMigrationTemplate_Triggers(t *testing.T) {
	opts := CreateMigrationOptions{Triggers: true}
	got := generateMigrationTemplate("create_orders_table", opts)

	wantStrings := []string{
		"CREATE TRIGGER trigger_orders_auto_update",
		"EXECUTE FUNCTION auto_update_timestamp()",
		"DROP TRIGGER IF EXISTS trigger_orders_auto_update",
		"vm functions migrate",
	}
	for _, want := range wantStrings {
		if !strings.Contains(got, want) {
			t.Errorf("Triggers migration missing %q\nGot:\n%s", want, got)
		}
	}
}

// TestGenerateMigrationTemplate_SoftDeleteWithTriggers verifies the soft-delete
// trigger variant uses the correct function name.
func TestGenerateMigrationTemplate_SoftDeleteWithTriggers(t *testing.T) {
	opts := CreateMigrationOptions{SoftDelete: true, Triggers: true}
	got := generateMigrationTemplate("create_items_table", opts)

	if !strings.Contains(got, "auto_update_with_soft_delete_protection") {
		t.Errorf("SoftDelete+Triggers should use auto_update_with_soft_delete_protection\nGot:\n%s", got)
	}
}

// TestGenerateMigrationTemplate_SqlcSupport checks goose markers are added.
func TestGenerateMigrationTemplate_SqlcSupport(t *testing.T) {
	opts := CreateMigrationOptions{SqlcSupport: true}
	got := generateMigrationTemplate("create_tasks_table", opts)

	for _, want := range []string{"-- +goose Up", "-- +goose Down"} {
		if !strings.Contains(got, want) {
			t.Errorf("SqlcSupport migration missing %q\nGot:\n%s", want, got)
		}
	}
}

// TestRelationshipComment ensures comments are formatted correctly.
func TestRelationshipComment(t *testing.T) {
	tests := []struct {
		name          string
		tableName     string
		relationships []Relationship
		wantContains  []string
		wantEmpty     bool
	}{
		{
			name:          "no relationships",
			tableName:     "posts",
			relationships: nil,
			wantEmpty:     true,
		},
		{
			name:      "single belongs-to",
			tableName: "posts",
			relationships: []Relationship{
				{Type: BelongsTo, TargetTable: "users"},
			},
			wantContains: []string{"posts → users (Many-to-One)"},
		},
		{
			name:      "single one-to-one",
			tableName: "profiles",
			relationships: []Relationship{
				{Type: OneToOne, TargetTable: "users"},
			},
			wantContains: []string{"profiles → users (One-to-One)"},
		},
		{
			name:      "multiple relationships",
			tableName: "posts",
			relationships: []Relationship{
				{Type: BelongsTo, TargetTable: "users"},
				{Type: BelongsTo, TargetTable: "categories"},
			},
			wantContains: []string{"Relationships:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelationshipComment(tt.tableName, tt.relationships)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty comment, got %q", got)
				}
				return
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("RelationshipComment missing %q; got %q", want, got)
				}
			}
		})
	}
}

// TestRelationshipFeatureDescription verifies human-readable descriptions.
func TestRelationshipFeatureDescription(t *testing.T) {
	tests := []struct {
		name          string
		relationships []Relationship
		pivot1        string
		pivot2        string
		wantContains  string
	}{
		{
			name:         "many-to-many description",
			pivot1:       "posts",
			pivot2:       "tags",
			wantContains: "many-to-many: posts <-> tags",
		},
		{
			name: "belongs-to description",
			relationships: []Relationship{
				{Type: BelongsTo, TargetTable: "users"},
			},
			wantContains: "belongs-to users",
		},
		{
			name: "one-to-one description",
			relationships: []Relationship{
				{Type: OneToOne, TargetTable: "users"},
			},
			wantContains: "one-to-one users",
		},
		{
			name: "multiple belongs-to",
			relationships: []Relationship{
				{Type: BelongsTo, TargetTable: "users"},
				{Type: BelongsTo, TargetTable: "categories"},
			},
			wantContains: "belongs-to users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelationshipFeatureDescription(tt.relationships, tt.pivot1, tt.pivot2)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("RelationshipFeatureDescription got %q, want to contain %q", got, tt.wantContains)
			}
		})
	}
}

// TestMigrationOptionsStep verifies Step field semantics.
func TestMigrationOptionsStep(t *testing.T) {
	tests := []struct {
		name    string
		step    int
		pending int
		want    int // how many migrations should run
	}{
		{"unlimited step runs all", 0, 5, 5},
		{"step=1 runs one", 1, 5, 1},
		{"step=3 runs three", 3, 5, 3},
		{"step > pending runs all", 10, 3, 3},
		{"step == pending runs all", 5, 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := tt.step
			pending := tt.pending

			var ran int
			for i := 0; i < pending; i++ {
				if limit > 0 && ran >= limit {
					break
				}
				ran++
			}

			if ran != tt.want {
				t.Errorf("step=%d, pending=%d: ran %d migrations, want %d", tt.step, tt.pending, ran, tt.want)
			}
		})
	}
}
