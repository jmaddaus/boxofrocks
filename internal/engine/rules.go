package engine

import "github.com/jmaddaus/boxofrocks/internal/model"

// IsTerminal returns true if the status is a terminal state (no further transitions allowed).
func IsTerminal(s model.Status) bool {
	return s == model.StatusDeleted
}

// FromStatusMatch returns true if the event's from_status matches the current status.
// An empty from_status (legacy events) is always accepted.
func FromStatusMatch(current, from model.Status) bool {
	return from == "" || from == current
}
