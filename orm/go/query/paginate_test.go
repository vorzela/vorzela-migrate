package query

import "testing"

func TestPageCount(t *testing.T) {
	cases := []struct {
		total, perPage int64
		want           int
	}{
		{0, 15, 0},
		{1, 15, 1},
		{15, 15, 1},
		{16, 15, 2},
		{100, 10, 10},
		{101, 10, 11},
	}
	for _, tc := range cases {
		if got := pageCount(tc.total, int(tc.perPage)); got != tc.want {
			t.Fatalf("pageCount(%d,%d)=%d want %d", tc.total, tc.perPage, got, tc.want)
		}
	}
}
