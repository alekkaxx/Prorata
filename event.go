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
	// EventApplyPromo arms a one-shot discount that the engine applies to the
	// next event producing a positive charge. The event carries no plan; it
	// uses Event.Code (promo code shown in the line) plus exactly one of
	// Event.Bps (a percentage discount in basis points, see
	// specs/04-promo-percent.md) or Event.AmountCents (a fixed-amount discount
	// in minor units, clamped to the charge, see specs/05-promo-fixed.md). It
	// produces no invoice line itself.
	EventApplyPromo EventType = "apply_promo"
	// EventTrial starts a free trial of a plan: it opens a billing period on
	// the plan with nothing paid (state.paid == 0) and emits a single zero
	// trial.start line so the invoice explains why the period is free. A trial
	// requires no active subscription and never charges; the customer only
	// begins paying on a later EventConvert (see specs/07-trial.md).
	EventTrial EventType = "trial_start"
	// EventConvert converts an active trial into a paid subscription: it charges
	// the plan named by Event.PlanID (the trial plan, or a different one, since
	// the free trial leaves no remainder to credit either way) in full for a
	// fresh billing period starting at Event.At, and clears the trial flag. The
	// unused remainder of the trial has zero value and simply burns. It requires
	// an active trial (see specs/07-trial.md).
	EventConvert EventType = "trial_convert"
)

// Event is a single subscription lifecycle event. Events are fed to Compute
// in chronological order; the engine never reorders them.
//
// Bps, AmountCents and Code are used only by EventApplyPromo and are omitted
// from the JSON of every other event type; the other types ignore them. A
// promo event sets exactly one of Bps (percentage) or AmountCents (fixed); see
// EventApplyPromo.
type Event struct {
	At          time.Time `json:"at"`
	Type        EventType `json:"type"`
	PlanID      string    `json:"plan"`
	Bps         int64     `json:"bps,omitempty"`
	AmountCents Money     `json:"amount_cents,omitempty"`
	Code        string    `json:"code,omitempty"`
}
