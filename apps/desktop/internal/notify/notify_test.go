package notify

import (
	"path/filepath"
	"testing"
	"time"
)

type recorder struct {
	sent []string
}

func (r *recorder) send(botID, phone, text string) error {
	r.sent = append(r.sent, botID+"|"+phone+"|"+text)
	return nil
}

func newManager(t *testing.T) (*Manager, *recorder) {
	t.Helper()
	rec := &recorder{}
	manager, err := New(filepath.Join(t.TempDir(), "targets.json"), rec.send, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manager, rec
}

func TestImmediateDispatchAndKinds(t *testing.T) {
	manager, rec := newManager(t)
	if _, err := manager.AddTarget(Target{Label: "A", Phone: "+1", Kinds: []string{KindNew}, Schedule: Schedule{Mode: "immediate"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if sent := manager.Dispatch("hola", KindNew, "2026-10-02"); sent != 1 || len(rec.sent) != 1 {
		t.Fatalf("sent=%d rec=%v", sent, rec.sent)
	}
	if sent := manager.Dispatch("x", KindUpdate, "2026-10-02"); sent != 0 {
		t.Fatal("target should not want updates")
	}
}

func TestScheduledQueueFlushes(t *testing.T) {
	manager, rec := newManager(t)
	now := time.Date(2026, 10, 2, 5, 0, 0, 0, location("America/Bogota"))
	manager.now = func() time.Time { return now }
	if _, err := manager.AddTarget(Target{Phone: "+1", Kinds: []string{KindNew}, Schedule: Schedule{Mode: "times", Times: []string{"06:30"}, Timezone: "America/Bogota"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	manager.Dispatch("hola", KindNew, "2026-10-02")
	if len(rec.sent) != 0 {
		t.Fatal("expected the notice to be queued")
	}
	manager.Tick(now.Add(30 * time.Minute))
	if len(rec.sent) != 0 {
		t.Fatal("not due yet")
	}
	manager.Tick(now.Add(2 * time.Hour))
	if len(rec.sent) != 1 {
		t.Fatalf("expected a flush, got %v", rec.sent)
	}
}

func TestQuietHoursRollForward(t *testing.T) {
	manager, rec := newManager(t)
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, location("America/Bogota"))
	manager.now = func() time.Time { return now }
	if _, err := manager.AddTarget(Target{Phone: "+1", Kinds: []string{KindNew}, Schedule: Schedule{
		Mode: "times", Times: []string{"21:30"}, QuietHours: []string{"21:00", "06:00"}, Timezone: "America/Bogota",
	}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	manager.Dispatch("hola", KindNew, "2026-10-02")
	manager.Tick(now.Add(105 * time.Minute)) // 21:45, inside quiet hours
	if len(rec.sent) != 0 {
		t.Fatalf("quiet hours should hold: %v", rec.sent)
	}
	manager.Tick(time.Date(2026, 10, 3, 6, 0, 0, 0, location("America/Bogota")))
	if len(rec.sent) != 1 {
		t.Fatalf("expected a send after quiet hours: %v", rec.sent)
	}
}

func TestNextSendAndTestAll(t *testing.T) {
	manager, rec := newManager(t)
	now := time.Date(2026, 10, 2, 5, 0, 0, 0, location("America/Bogota"))
	if _, err := manager.AddTarget(Target{Phone: "+1", Kinds: []string{KindNew}, Schedule: Schedule{Mode: "immediate"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := manager.NextSend(now); ok {
		t.Fatal("an immediate-only target has no next send")
	}
	if _, err := manager.AddTarget(Target{Phone: "+2", Kinds: []string{KindNew}, Schedule: Schedule{Mode: "times", Times: []string{"12:00"}, Timezone: "America/Bogota"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	when, count, ok := manager.NextSend(now)
	if !ok || count != 1 {
		t.Fatalf("next send ok=%v count=%d", ok, count)
	}
	if got := when.In(location("America/Bogota")).Format("15:04"); got != "12:00" {
		t.Fatalf("next send = %s, want 12:00", got)
	}
	if sent := manager.TestAll(); sent != 2 || len(rec.sent) != 2 {
		t.Fatalf("test all sent=%d rec=%v", sent, rec.sent)
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.json")
	rec := &recorder{}
	manager, err := New(path, rec.send, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := manager.AddTarget(Target{Phone: "+1", Kinds: []string{KindNew}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(path, rec.send, nil)
	if err != nil {
		t.Fatal(err)
	}
	targets := reopened.Targets()
	if len(targets) != 1 || targets[0].ID != target.ID {
		t.Fatalf("target lost: %+v", targets)
	}
}

func TestRemoveTargetDropsPending(t *testing.T) {
	manager, _ := newManager(t)
	target, err := manager.AddTarget(Target{Phone: "+1", Kinds: []string{KindNew}, Schedule: Schedule{Mode: "times", Times: []string{"06:30"}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	manager.Dispatch("hola", KindNew, "2026-10-02")
	if len(manager.Pending()) != 1 {
		t.Fatal("expected a pending notice")
	}
	if err := manager.RemoveTarget(target.ID); err != nil {
		t.Fatal(err)
	}
	if len(manager.Pending()) != 0 {
		t.Fatal("pending notices should go with the target")
	}
}
