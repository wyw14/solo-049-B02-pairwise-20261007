package scheduler

import (
	"fmt"
	"time"
)

type ReservationStatus string

const (
	Reserved ReservationStatus = "reserved"
	Active   ReservationStatus = "active"
	Released ReservationStatus = "released"
)

// transitions is the reservation state machine. Released is terminal: a
// released reservation keeps its history for audit but can never occupy
// an antenna again.
var transitions = map[ReservationStatus][]ReservationStatus{
	Reserved: {Active, Released},
	Active:   {Released},
	Released: {},
}

// Terminal reports whether the status ends the reservation lifecycle.
func (s ReservationStatus) Terminal() bool { return s == Released }

// CanTransitionTo reports whether moving from s to next is a legal edge.
func (s ReservationStatus) CanTransitionTo(next ReservationStatus) bool {
	for _, allowed := range transitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

type Interval struct {
	ReservationID string            `json:"reservation_id"`
	PassID        string            `json:"pass_id"`
	AntennaID     string            `json:"antenna_id"`
	Start         time.Time         `json:"start"`
	End           time.Time         `json:"end"`
	Generation    int64             `json:"generation"`
	Status        ReservationStatus `json:"status"`
}

func (i Interval) Validate() error {
	if i.ReservationID == "" || i.PassID == "" || i.AntennaID == "" {
		return fmt.Errorf("reservation, pass and antenna identity are required")
	}
	if !i.End.After(i.Start) {
		return fmt.Errorf("reservation end must follow start")
	}
	if i.Generation < 1 {
		return fmt.Errorf("owner generation must be positive")
	}
	return nil
}

func (i Interval) Overlaps(other Interval) bool {
	return i.Start.Before(other.End) && other.Start.Before(i.End)
}

// Occupies reports whether the interval still holds the antenna. Conflict
// detection and ownership queries must agree on this predicate.
func (i Interval) Occupies() bool { return !i.Status.Terminal() }

type ConflictError struct {
	AntennaID string
	Existing  Interval
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("antenna %s is reserved by pass %s", e.AntennaID, e.Existing.PassID)
}
