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
