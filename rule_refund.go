package prorata

import (
	"fmt"
	"time"
)

// ruleRefundFull is the RuleID for the unconditional cash-refund line
// produced by RefundFull: the entire cash actually paid for the current
// period, regardless of how much of it was used.
const ruleRefundFull RuleID = "refund.full"

// ruleRefundProrated is the RuleID for the day-prorated cash-refund line
// produced by RefundProrated: only the unused remainder of the current
// period.
const ruleRefundProrated RuleID = "refund.prorated"

// ruleRefundCredit is the RuleID for the zero-amount line produced by
// RefundCredit, documenting the unused remainder banked to the credit
// balance instead of returned in cash.
const ruleRefundCredit RuleID = "refund.credit"

// init registers the refund rule for EventRefund.
func init() {
	registerRule(EventRefund, refundSubscription)
}

// refundSubscription handles EventRefund: it tears down the active
// subscription and returns money for the current period according to
// ev.Policy, computed from the cash actually paid for the period
// (state.cashPaid) rather than the nominal plan price, so a refund never
// hands back more than the customer actually paid even when a promo or the
// credit balance reduced the original charge (see specs/08-refund.md D3).
//
// RefundFull returns the whole cashPaid unconditionally, regardless of use.
// RefundProrated and RefundCredit return only the unused remainder of the
// period, computed by the same day-granular largest-remainder Allocate split
// used by proration elsewhere (see specs/08-refund.md D4): RefundProrated
// pays it back in cash, RefundCredit banks it to the subscription's credit
// balance instead and emits a zero-amount documenting line.
//
// Refund requires an active subscription; both a clean state and a plan
// already closed by a prior refund report the same error. After any policy
// the subscription is torn down (plan cleared, no access past ev.At) but the
// engine's ledger — creditBalance and any armed promo — is left untouched,
// since it reflects money already paid independent of the period being
// closed (see specs/08-refund.md D5).
func refundSubscription(st *state, c Catalog, ev Event) ([]Line, error) {
	if st.plan == nil {
		return nil, fmt.Errorf("prorata: refund: no active subscription")
	}
	if _, ok := c[ev.PlanID]; !ok {
		return nil, fmt.Errorf("prorata: refund: unknown plan %q", ev.PlanID)
	}

	planID := st.plan.ID
	periodStart := st.periodStart
	periodEnd := st.periodEnd
	cashPaid := st.cashPaid

	total := Period{Start: periodStart, End: periodEnd}.Days()
	rem := max(Period{Start: ev.At, End: periodEnd}.Days(), 0)
	used := total - rem

	parts, err := cashPaid.Allocate([]int64{int64(used), int64(rem)})
	if err != nil {
		return nil, err
	}
	unused := parts[1]

	var line Line
	switch ev.Policy {
	case RefundFull:
		line = Line{
			RuleID: ruleRefundFull,
			Description: fmt.Sprintf(
				"%s: full refund %s to %s",
				planID,
				periodStart.UTC().Format("2006-01-02"),
				periodEnd.UTC().Format("2006-01-02"),
			),
			Amount: -cashPaid,
		}
	case RefundProrated:
		line = Line{
			RuleID: ruleRefundProrated,
			Description: fmt.Sprintf(
				"%s: refund unused %d/%d days %s to %s",
				planID, rem, total,
				ev.At.UTC().Format("2006-01-02"),
				periodEnd.UTC().Format("2006-01-02"),
			),
			Amount: -unused,
		}
	case RefundCredit:
		st.creditBalance += unused
		line = Line{
			RuleID: ruleRefundCredit,
			Description: fmt.Sprintf(
				"%s: credit %s for unused %d/%d days %s to %s",
				planID, unused.String(), rem, total,
				ev.At.UTC().Format("2006-01-02"),
				periodEnd.UTC().Format("2006-01-02"),
			),
			Amount: 0,
		}
	default:
		return nil, fmt.Errorf("prorata: refund: unknown policy %q", ev.Policy)
	}

	st.plan = nil
	st.periodStart = time.Time{}
	st.periodEnd = time.Time{}
	st.paid = 0
	st.cashPaid = 0
	st.trial = false

	return []Line{line}, nil
}
