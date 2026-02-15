package engine

import "github.com/jmaddaus/boxofrocks/internal/model"

// transitions defines the set of allowed status transitions.
// Each key is a source status, and the value is the set of valid target statuses.
var transitions = map[model.Status]map[model.Status]bool{
	model.StatusOpen: {
		model.StatusInProgress: true,
		model.StatusClosed:     true,
		model.StatusDeleted:    true,
	},
	model.StatusInProgress: {
		model.StatusOpen:    true,
		model.StatusClosed:  true,
		model.StatusDeleted: true,
	},
	model.StatusClosed: {
		model.StatusOpen:    true,
		model.StatusDeleted: true,
	},
	model.StatusDeleted: {},
}

// ValidTransition returns true if the status change from -> to is allowed.
func ValidTransition(from, to model.Status) bool {
	targets, ok := transitions[from]
	if !ok {
		return false
	}
	return targets[to]
}
