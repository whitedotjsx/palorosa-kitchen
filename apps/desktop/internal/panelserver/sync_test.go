package panelserver

import (
	"fmt"
	"testing"
	"time"
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

// TestSyncerDueCadence pins the two clocks: the fast lookup runs on the sync
// interval and the exact export is forced on the full interval.
func TestSyncerDueCadence(t *testing.T) {
	start := time.Now()
	syncer := &orderSyncer{last: start, lastFull: start}
	cases := []struct {
		name        string
		after       time.Duration
		wantRun     bool
		wantFull    bool
	}{
		{"nothing due", 5 * time.Minute, false, false},
		{"lookup due", 11 * time.Minute, true, false},
		{"full export due", 181 * time.Minute, true, true},
	}
	for _, test := range cases {
		run, full := syncer.due(DefaultSyncMinutes, DefaultFullSyncMinutes, start.Add(test.after))
		if run != test.wantRun || full != test.wantFull {
			t.Fatalf("%s: run=%v full=%v, want run=%v full=%v", test.name, run, full, test.wantRun, test.wantFull)
		}
	}
	run, _ := syncer.due(0, DefaultFullSyncMinutes, start.Add(24*time.Hour))
	if run {
		t.Fatal("sync turned off should never run")
	}
	run, full := syncer.due(DefaultSyncMinutes, 0, start.Add(24*time.Hour))
	if !run || full {
		t.Fatalf("full export off: run=%v full=%v, want run=true full=false", run, full)
	}
}
