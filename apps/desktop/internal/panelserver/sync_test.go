package panelserver

import (
	"fmt"
	"testing"
)

func TestRequestedDates(t *testing.T) {
	got := requestedDates("", []string{"2026-10-08", "2026-10-06", " 2026-10-06 ", "malo", ""})
	want := []string{"2026-10-06", "2026-10-08"}
	if len(got) != len(want) {
		t.Fatalf("requestedDates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("requestedDates = %v, want %v", got, want)
		}
	}
}

func TestRequestedDatesFallsBack(t *testing.T) {
	got := requestedDates("2030-01-01", nil)
	if len(got) != 3 {
		t.Fatalf("fallback = %v, want today, tomorrow and the shown day", got)
	}
}

func TestRequestedDatesCaps(t *testing.T) {
	dates := make([]string, 0, 40)
	for day := 1; day <= 40; day++ {
		dates = append(dates, fmt.Sprintf("2026-11-%02d", day))
	}
	if got := requestedDates("", dates); len(got) != maxSyncDates {
		t.Fatalf("len = %d, want %d", len(got), maxSyncDates)
	}
}
