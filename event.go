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
	// EventUpgrade switches an active subscription to a new plan mid-period,
	// crediting the unused remainder of the current plan and charging the new
	// plan in full (see specs/02-prorate-upgrade.md).
	EventUpgrade EventType = "upgrade"
	// EventDowngrade switches an active subscription to a cheaper plan
	// mid-period. Unlike EventUpgrade, the unused remainder of the old plan is
	// not paid back as a refund line but banked to the subscription's credit
	// balance, which the engine applies against future charges
	// (see specs/03-downgrade-credit.md).
	EventDowngrade EventType = "downgrade"
)

// Event is a single subscription lifecycle event. Events are fed to Compute
// in chronological order; the engine never reorders them.
type Event struct {
	At     time.Time `json:"at"`
	Type   EventType `json:"type"`
	PlanID string    `json:"plan"`
}
