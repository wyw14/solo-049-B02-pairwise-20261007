package scheduler

import (
	"strings"
	"testing"
	"time"

	"orbitlink/internal/audit"
	"orbitlink/internal/store"
)

func newTestTimeline(t *testing.T) *Timeline {
	t.Helper()
	paths := store.NewPaths(t.TempDir())
	timeline, err := NewTimeline(paths, audit.NewTrail(store.NewAppendLog(paths)))
	if err != nil {
		t.Fatal(err)
	}
	return timeline
}

func TestReleasedReservationCannotBeRevived(t *testing.T) {
	timeline := newTestTimeline(t)
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	reservation, err := timeline.Reserve("pass-1", "ant-1", start, start.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := timeline.Release(reservation.ReservationID); err != nil {
		t.Fatal(err)
	}
	if _, err := timeline.Activate(reservation.ReservationID); err == nil {
		t.Fatal("expected activating a released reservation to fail")
	} else if !strings.Contains(err.Error(), "released") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, found := timeline.CurrentOwner("ant-1", start.Add(5*time.Minute)); found {
		t.Fatal("released reservation must not be reported as current owner")
	}
	// History stays auditable: the released interval remains listed.
	intervals := timeline.List("ant-1")
	if len(intervals) != 1 || intervals[0].Status != Released {
		t.Fatalf("expected released interval to remain in history, got %+v", intervals)
	}
}

func TestLegalTransitionsAndTerminalState(t *testing.T) {
	timeline := newTestTimeline(t)
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	reservation, err := timeline.Reserve("pass-1", "ant-1", start, start.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := timeline.Activate(reservation.ReservationID); err != nil {
		t.Fatalf("reserved -> active should be legal: %v", err)
	}
	if _, err := timeline.Activate(reservation.ReservationID); err == nil {
		t.Fatal("active -> active should be rejected")
	}
	if _, err := timeline.Release(reservation.ReservationID); err != nil {
		t.Fatalf("active -> released should be legal: %v", err)
	}
	if _, err := timeline.Release(reservation.ReservationID); err == nil {
		t.Fatal("released -> released should be rejected")
	}
	if _, err := timeline.Extend(reservation.ReservationID, start.Add(20*time.Minute)); err == nil {
		t.Fatal("extending a released reservation should be rejected")
	}
}

func TestReleasedSlotIsReusableWithoutRevival(t *testing.T) {
	timeline := newTestTimeline(t)
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	first, err := timeline.Reserve("pass-1", "ant-1", start, start.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := timeline.Release(first.ReservationID); err != nil {
		t.Fatal(err)
	}
	second, err := timeline.Reserve("pass-2", "ant-1", start, start.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("released slot should accept a new reservation: %v", err)
	}
	if _, err := timeline.Activate(first.ReservationID); err == nil {
		t.Fatal("reviving the released reservation must stay impossible")
	}
	owner, found := timeline.CurrentOwner("ant-1", start.Add(5*time.Minute))
	if !found || owner.ReservationID != second.ReservationID {
		t.Fatalf("expected second reservation to own the antenna, got %+v", owner)
	}
}
