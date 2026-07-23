package prorata

import "time"

// EventType identifies the kind of subscription lifecycle event.
// Each type is handled by exactly one billing rule (rule_*.go).
type EventType string

// Known event types. New types are introduced together with the rule that
// handles them.
const (
	// EventSubscribe starts a subscription on a plan.
	EventSubscribe EventType = "subscribe"
)

// Event is a single subscription lifecycle event. Events are fed to Compute
// in chronological order; the engine never reorders them.
type Event struct {
	At     time.Time `json:"at"`
	Type   EventType `json:"type"`
	PlanID string    `json:"plan"`
}
