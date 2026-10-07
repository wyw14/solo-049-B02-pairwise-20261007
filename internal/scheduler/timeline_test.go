package scheduler

import (
	"strings"
	"testing"
	"time"

	"orbitlink/internal/audit"
	"orbitlink/internal/store"
)

func newTestTimeline(t *testing.T, root string) *Timeline {
	t.Helper()
	paths := store.NewPaths(root)
	timeline, err := NewTimeline(paths, audit.NewTrail(store.NewAppendLog(paths)))
	if err != nil {
		t.Fatal(err)
	}
	return timeline
}

func TestReservationLifecycleRejectsTerminalResurrection(t *testing.T) {
	root := t.TempDir()
	timeline := newTestTimeline(t, root)
	start := time.Now().UTC().Add(-time.Minute)
	end := start.Add(10 * time.Minute)

	reservation, err := timeline.Reserve("PASS-1", "ANT-01", start, end)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.Status != Reserved {
		t.Fatalf("new reservation status = %s", reservation.Status)
	}

	// Legal path: reserved -> active -> released.
	if _, err := timeline.Activate(reservation.ReservationID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if owner, found := timeline.CurrentOwner("ANT-01", start.Add(time.Minute)); !found || owner.ReservationID != reservation.ReservationID {
		t.Fatal("active reservation should own the antenna inside its window")
	}
	if _, err := timeline.Release(reservation.ReservationID); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Terminal state rejects every transition, including 复活路径 Released -> Active.
	for _, target := range []ReservationStatus{Active, Reserved, Released} {
		if Released.CanTransitionTo(target) {
			t.Fatalf("released unexpectedly transitions to %s", target)
		}
	}
	if _, err := timeline.Activate(reservation.ReservationID); err == nil ||
		!strings.Contains(err.Error(), "released") || !strings.Contains(err.Error(), "active") {
		t.Fatalf("released reservation must not reactivate, got %v", err)
	}
	if _, err := timeline.Release(reservation.ReservationID); err == nil {
		t.Fatal("released reservation must not re-release")
	}

	// The ghost must not occupy: conflict query and CurrentOwner ignore it.
	if _, found := timeline.CurrentOwner("ANT-01", start.Add(time.Minute)); found {
		t.Fatal("released reservation must not own the antenna")
	}
	successor, err := timeline.Reserve("PASS-2", "ANT-01", start, end)
	if err != nil {
		t.Fatalf("released slot should be reservable: %v", err)
	}

	// History stays auditable: the released interval remains listed and
	// survives a reload from the snapshot.
	var released []Interval
	for _, value := range timeline.List("ANT-01") {
		if value.Status == Released {
			released = append(released, value)
		}
	}
	if len(released) != 1 || released[0].ReservationID != reservation.ReservationID {
		t.Fatalf("released reservation missing from history: %#v", released)
	}
	reloaded := newTestTimeline(t, root)
	stillReleased := false
	for _, value := range reloaded.List("ANT-01") {
		if value.ReservationID == reservation.ReservationID {
			stillReleased = value.Status == Released
		}
	}
	if !stillReleased {
		t.Fatal("released status did not survive snapshot reload")
	}
	if _, err := reloaded.Activate(reservation.ReservationID); err == nil {
		t.Fatal("reloaded released reservation must not reactivate")
	}
	if _, err := reloaded.Extend(reservation.ReservationID, end.Add(time.Minute)); err == nil {
		t.Fatal("released reservation must not extend")
	}
	if owner, found := reloaded.CurrentOwner("ANT-01", start.Add(time.Minute)); !found || owner.ReservationID != successor.ReservationID {
		t.Fatal("successor reservation should own the antenna after reload")
	}
}

func TestReservationRejectsBackwardsAndNoopTransitions(t *testing.T) {
	timeline := newTestTimeline(t, t.TempDir())
	start := time.Now().UTC().Add(time.Hour)
	reservation, err := timeline.Reserve("PASS-1", "ANT-01", start, start.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := timeline.Activate(reservation.ReservationID); err != nil {
		t.Fatal(err)
	}
	if _, err := timeline.setStatus(reservation.ReservationID, Reserved); err == nil {
		t.Fatal("active reservation must not move backwards to reserved")
	}
	if _, err := timeline.Activate(reservation.ReservationID); err == nil {
		t.Fatal("active reservation must not re-activate")
	}
	current := timeline.List("ANT-01")[0]
	if current.Status != Active {
		t.Fatalf("rejected transitions must leave status untouched, got %s", current.Status)
	}
}
