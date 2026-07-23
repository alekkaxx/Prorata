package prorata

import (
	"fmt"
	"time"
)

// ruleGraceStart is the RuleID for the zero-amount line emitted when
// EventPaymentFailed opens a grace period on an active subscription.
const ruleGraceStart RuleID = "grace.start"

// ruleGraceRecover is the RuleID for the zero-amount line emitted when
// EventGraceRecover closes an open grace period after the outstanding
// charge was finally collected.
const ruleGraceRecover RuleID = "grace.recover"

// ruleGraceExpire is the RuleID for the zero-amount line emitted when
// EventGraceExpire closes an open grace period by tearing down the
// subscription for non-payment.
const ruleGraceExpire RuleID = "grace.expire"

// init registers the three grace-period rules for EventPaymentFailed,
// EventGraceRecover and EventGraceExpire.
func init() {
	registerRule(EventPaymentFailed, gracePaymentFailed)
	registerRule(EventGraceRecover, graceRecover)
	registerRule(EventGraceExpire, graceExpire)
}

// gracePaymentFailed handles EventPaymentFailed: it opens a grace period on
// the active subscription when the caller reports that the charge for the
// current period was not collected.
//
// Access is retained (plan, periodStart, periodEnd are untouched) but the
// state is made honest about what was actually collected: the pre-grace
// cashPaid is stashed in graceHeldCash and both paid and cashPaid are zeroed,
// so any refund or proration that runs while grace is open credits nothing
// (see specs/10-grace.md, D5) rather than crediting cash that was never
// received. It requires an active, non-trial subscription that is not
// already in grace (a trial has nothing to fail payment on, since cashPaid
// is already 0; see specs/10-grace.md, D5's cross-reference and the trial
// carve-out). The returned line always carries a zero Amount so the invoice
// explains why access was retained despite the failed charge, rather than
// silently changing state (see specs/10-grace.md, D3).
func gracePaymentFailed(st *state, c Catalog, ev Event) ([]Line, error) {
	if st.plan == nil {
		return nil, fmt.Errorf("prorata: grace: no active subscription")
	}
	if st.trial {
		return nil, fmt.Errorf("prorata: grace: cannot fail payment on a trial")
	}
	if st.grace {
		return nil, fmt.Errorf("prorata: grace: already in grace")
	}
	if _, ok := c[ev.PlanID]; !ok {
		return nil, fmt.Errorf("prorata: grace: unknown plan %q", ev.PlanID)
	}

	st.graceHeldCash = st.cashPaid
	st.paid = 0
	st.cashPaid = 0
	st.grace = true

	line := Line{
		RuleID: ruleGraceStart,
		Description: fmt.Sprintf(
			"%s: payment failed %s, access retained",
			st.plan.ID,
			ev.At.UTC().Format("2006-01-02"),
		),
		Amount: 0,
	}
	return []Line{line}, nil
}

// graceRecover handles EventGraceRecover: it closes an open grace period
// after the caller reports the outstanding charge was finally collected.
//
// It restores state.paid to the plan price and state.cashPaid to the cash
// stashed by gracePaymentFailed, clears graceHeldCash and the grace flag. No
// new charge is produced — the original charge line already stands on the
// invoice; recovery only restores the honest accounting of what was
// collected. It requires an open grace period (see specs/10-grace.md).
func graceRecover(st *state, c Catalog, ev Event) ([]Line, error) {
	if !st.grace {
		return nil, fmt.Errorf("prorata: grace: no grace in progress")
	}
	if _, ok := c[ev.PlanID]; !ok {
		return nil, fmt.Errorf("prorata: grace: unknown plan %q", ev.PlanID)
	}

	st.paid = st.plan.Price
	st.cashPaid = st.graceHeldCash
	st.graceHeldCash = 0
	st.grace = false

	line := Line{
		RuleID: ruleGraceRecover,
		Description: fmt.Sprintf(
			"%s: payment recovered %s, subscription continues",
			st.plan.ID,
			ev.At.UTC().Format("2006-01-02"),
		),
		Amount: 0,
	}
	return []Line{line}, nil
}

// graceExpire handles EventGraceExpire: it closes an open grace period by
// tearing down the subscription for non-payment.
//
// The plan and period are cleared (access ends at ev.At), matching the
// teardown a refund performs, while the engine ledger (creditBalance, any
// armed promo) is left untouched (see specs/10-grace.md, D6). The uncollected
// charge already on the invoice is never reversed: doing so would be a
// credit exceeding the zero cash actually collected during grace, which
// would break the "credit <= actually paid" invariant (see specs/10-grace.md,
// D4). It requires an open grace period.
func graceExpire(st *state, c Catalog, ev Event) ([]Line, error) {
	if !st.grace {
		return nil, fmt.Errorf("prorata: grace: no grace in progress")
	}
	if _, ok := c[ev.PlanID]; !ok {
		return nil, fmt.Errorf("prorata: grace: unknown plan %q", ev.PlanID)
	}

	planID := st.plan.ID

	st.plan = nil
	st.periodStart = time.Time{}
	st.periodEnd = time.Time{}
	st.paid = 0
	st.cashPaid = 0
	st.trial = false
	st.grace = false
	st.graceHeldCash = 0

	line := Line{
		RuleID: ruleGraceExpire,
		Description: fmt.Sprintf(
			"%s: grace expired %s, access ended for non-payment",
			planID,
			ev.At.UTC().Format("2006-01-02"),
		),
		Amount: 0,
	}
	return []Line{line}, nil
}
