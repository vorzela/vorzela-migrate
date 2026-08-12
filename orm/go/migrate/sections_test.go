package migrate

import "testing"

func TestExtractSections(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantUp   string
		wantDown string
	}{
		{
			name: "arrow markers as scaffolded by vm",
			content: "-- Migration: create_users_table\n" +
				"-- Created: 2024-04-05\n" +
				"\n" +
				"-- ⬆ Up (Run when migrating forward)\n" +
				"CREATE TABLE users (id SERIAL PRIMARY KEY);\n" +
				"\n" +
				"-- ⬇ Down (Run when rolling back)\n" +
				"DROP TABLE users;\n",
			wantUp:   "CREATE TABLE users (id SERIAL PRIMARY KEY);",
			wantDown: "DROP TABLE users;",
		},
		{
			name:     "goose markers",
			content:  "-- +goose Up\nCREATE TABLE t (id INT);\n-- +goose Down\nDROP TABLE t;\n",
			wantUp:   "CREATE TABLE t (id INT);",
			wantDown: "DROP TABLE t;",
		},
		{
			name:     "golang-migrate markers",
			content:  "-- migrate:up\nCREATE TABLE t (id INT);\n-- migrate:down\nDROP TABLE t;\n",
			wantUp:   "CREATE TABLE t (id INT);",
			wantDown: "DROP TABLE t;",
		},
		{
			name:     "simple markers",
			content:  "-- Up\nCREATE TABLE t (id INT);\n-- Down\nDROP TABLE t;\n",
			wantUp:   "CREATE TABLE t (id INT);",
			wantDown: "DROP TABLE t;",
		},
		{
			name:     "simple markers are case insensitive and need no comment prefix",
			content:  "UP\nCREATE TABLE t (id INT);\n--down\nDROP TABLE t;\n",
			wantUp:   "CREATE TABLE t (id INT);",
			wantDown: "DROP TABLE t;",
		},
		{
			name:     "comment lines are stripped from both sections",
			content:  "-- Up\n-- create the table\nCREATE TABLE t (id INT);\n-- Down\n-- and drop it\nDROP TABLE t;\n",
			wantUp:   "CREATE TABLE t (id INT);",
			wantDown: "DROP TABLE t;",
		},
		{
			name:     "interior blank lines are kept",
			content:  "-- Up\nCREATE TABLE a (id INT);\n\nCREATE TABLE b (id INT);\n-- Down\nDROP TABLE b;\n",
			wantUp:   "CREATE TABLE a (id INT);\n\nCREATE TABLE b (id INT);",
			wantDown: "DROP TABLE b;",
		},
		{
			name:     "down section runs to end of file",
			content:  "-- Up\nSELECT 1;\n-- Down\nSELECT 2;\nSELECT 3;\n",
			wantUp:   "SELECT 1;",
			wantDown: "SELECT 2;\nSELECT 3;",
		},
		{
			name:     "no markers yields nothing",
			content:  "CREATE TABLE t (id INT);\n",
			wantUp:   "",
			wantDown: "",
		},
		{
			name:     "down only",
			content:  "-- ⬇ Down\nDROP TABLE t;\n",
			wantUp:   "",
			wantDown: "DROP TABLE t;",
		},
		{
			name:     "up only",
			content:  "-- ⬆ Up\nCREATE TABLE t (id INT);\n",
			wantUp:   "CREATE TABLE t (id INT);",
			wantDown: "",
		},
		{
			name:     "text before the up marker is ignored",
			content:  "SELECT 'stray';\n-- Up\nSELECT 1;\n",
			wantUp:   "SELECT 1;",
			wantDown: "",
		},
		{
			name:     "empty file",
			content:  "",
			wantUp:   "",
			wantDown: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractUp(tc.content); got != tc.wantUp {
				t.Errorf("ExtractUp() = %q, want %q", got, tc.wantUp)
			}
			if got := ExtractDown(tc.content); got != tc.wantDown {
				t.Errorf("ExtractDown() = %q, want %q", got, tc.wantDown)
			}
		})
	}
}

func TestMarkerDetection(t *testing.T) {
	tests := []struct {
		line     string
		wantUp   bool
		wantDown bool
	}{
		{"-- ⬆ Up (Run when migrating forward)", true, false},
		{"-- ⬇ Down (Run when rolling back)", false, true},
		{"-- +goose Up", true, false},
		{"-- +goose Down", false, true},
		{"-- migrate:up", true, false},
		{"-- migrate:down", false, true},
		{"-- Up", true, false},
		{"--up", true, false},
		{"UP", true, false},
		{"-- DOWN", false, true},
		{"-- Update the users table", false, false},
		{"CREATE TABLE up (id INT);", false, false},
		{"---- Up", false, false},
		{"", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			if got := isUpMarker(tc.line); got != tc.wantUp {
				t.Errorf("isUpMarker(%q) = %t, want %t", tc.line, got, tc.wantUp)
			}
			if got := isDownMarker(tc.line); got != tc.wantDown {
				t.Errorf("isDownMarker(%q) = %t, want %t", tc.line, got, tc.wantDown)
			}
		})
	}
}
