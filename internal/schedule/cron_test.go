package schedule

import (
	"testing"
	"time"
)

func TestParseRejectsBadExpressions(t *testing.T) {
	for _, expr := range []string{
		"", "* * * *", "* * * * * *", "60 * * * *", "* 24 * * *",
		"*/0 * * * *", "a * * * *", "* * 0 * *", "* * * 13 *", "* * * * 7",
		"5-1 * * * *",
	} {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", expr)
		}
	}
}

func TestNextMatchesFields(t *testing.T) {
	base := time.Date(2026, 3, 4, 10, 30, 0, 0, time.UTC) // a Wednesday

	cases := []struct {
		expr string
		want time.Time
	}{
		{"* * * * *", time.Date(2026, 3, 4, 10, 31, 0, 0, time.UTC)},
		{"0 8 * * 1-5", time.Date(2026, 3, 5, 8, 0, 0, 0, time.UTC)},
		{"*/15 * * * *", time.Date(2026, 3, 4, 10, 45, 0, 0, time.UTC)},
		{"0 0 1 * *", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"30 10 * * *", time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		parsed, err := Parse(tc.expr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.expr, err)
		}
		got, err := parsed.Next(base)
		if err != nil {
			t.Fatalf("Next(%q): %v", tc.expr, err)
		}
		if !got.Equal(tc.want) {
			t.Errorf("%q: got %s, want %s", tc.expr, got, tc.want)
		}
	}
}

func TestDayOfMonthAndWeekUseOr(t *testing.T) {
	// Day 1 OR Monday: 2026-03-04 is a Wednesday, so the next Monday wins.
	parsed, err := Parse("0 9 1 * 1")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	got, err := parsed.Next(base)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 9, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}
