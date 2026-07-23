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
	// EventRefund tears down an active subscription and returns money for the
	// current period according to Event.Policy (see RefundPolicy). It closes the
	// subscription at Event.At (plan is cleared, no access after the refund) and
	// moves at most the cash actually paid for the current period — never the
	// nominal plan price — so the "credit <= actually paid" invariant holds even
	// when a promo or the credit balance reduced the original charge. Refund is
	// distinct from cancellation: a refund always moves money and ends access at
	// Event.At, whereas cancelling without a refund (access kept until period
	// end, no money) is a separate, future concern (see specs/08-refund.md).
	EventRefund EventType = "refund"
	// EventSetVAT arms a standing value-added-tax rate the engine adds on top of
	// the net charge of every subsequent taxable event. It carries no plan and
	// produces no invoice line itself; it uses Event.Bps as the rate in basis
	// points (2000 == 20%, 10000 == 100%), the same scale as a percentage promo.
	// The rate persists until changed by a later EventSetVAT and is 0 by default,
	// so an unset rate adds no VAT and leaves pre-VAT invoices byte-identical.
	// Only charges from this event onward are taxed; earlier charges are never
	// taxed retroactively (see specs/09-vat.md).
	EventSetVAT EventType = "set_vat"
)

// RefundPolicy selects how much of the current period is returned by an
// EventRefund and where it goes. It lives on the event, not on the plan,
// because a refund is an exceptional, per-case decision made at refund time,
// not a standing property of the plan (see specs/08-refund.md).
type RefundPolicy string

// Supported refund policies. Amounts are always drawn from the cash actually
// paid for the current period (state.cashPaid), so no policy can return more
// than was paid.
const (
	// RefundFull returns the entire cash paid for the current period as a
	// negative refund line, regardless of how much of the period was used
	// (a goodwill refund). RuleID refund.full.
	RefundFull RefundPolicy = "full"
	// RefundProrated returns only the unused remainder of the current period,
	// by days (the same day-granular Allocate split used by proration), as a
	// negative refund line. RuleID refund.prorated.
	RefundProrated RefundPolicy = "prorated"
	// RefundCredit banks the unused remainder of the current period (the same
	// amount RefundProrated would return in cash) into the subscription's credit
	// balance instead of returning cash, kin to a downgrade. It emits a
	// zero-amount refund.credit line documenting the banked amount; the credit
	// surfaces later as credit.applied when a future charge draws it down.
	RefundCredit RefundPolicy = "credit"
)

// Event is a single subscription lifecycle event. Events are fed to Compute
// in chronological order; the engine never reorders them.
//
// Bps, AmountCents and Code are used only by EventApplyPromo and EventSetVAT
// and are omitted from the JSON of every other event type; the other types
// ignore them. A promo event sets exactly one of Bps (percentage) or
// AmountCents (fixed); see EventApplyPromo. EventSetVAT uses Bps alone as the
// tax rate; see EventSetVAT. Policy is used only by EventRefund; see
// RefundPolicy.
type Event struct {
	At          time.Time    `json:"at"`
	Type        EventType    `json:"type"`
	PlanID      string       `json:"plan"`
	Bps         int64        `json:"bps,omitempty"`
	AmountCents Money        `json:"amount_cents,omitempty"`
	Code        string       `json:"code,omitempty"`
	Policy      RefundPolicy `json:"policy,omitempty"`
}
