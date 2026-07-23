package prorata

import "fmt"

// ruleDowngradeCharge is the RuleID for the new-plan full-period charge line
// produced by a downgrade.
const ruleDowngradeCharge RuleID = "downgrade.charge"

// init registers the downgrade rule for EventDowngrade.
func init() {
	registerRule(EventDowngrade, downgradePlan)
}

// downgradePlan handles EventDowngrade: it switches an active subscription to
// a new plan mid-period, banking the unused remainder of the old plan's
// current period into the subscription's credit balance instead of refunding
// it as a line, and charges the new plan in full for its first billing
// period starting at ev.At. The engine (applyCredit) later draws down the
// balance against the net charge of this same event (and future events). See
// specs/03-downgrade-credit.md for the derivation.
func downgradePlan(st *state, c Catalog, ev Event) ([]Line, error) {
	if st.plan == nil {
		return nil, fmt.Errorf("prorata: downgrade: no active subscription")
	}
	newPlan, err := lookupPlan(c, ev.PlanID, "downgrade")
	if err != nil {
		return nil, err
	}
	if st.plan.ID == ev.PlanID {
		return nil, fmt.Errorf("prorata: downgrade: already on plan %q", ev.PlanID)
	}

	unused, _, _, err := unusedShare(
		st.paid, Period{Start: st.periodStart, End: st.periodEnd}, ev.At)
	if err != nil {
		return nil, err
	}
	st.creditBalance += unused

	newEnd, err := st.openPeriod(newPlan, ev.At)
	if err != nil {
		return nil, err
	}
	return []Line{fullPeriodLine(ruleDowngradeCharge, newPlan, ev.At, newEnd)}, nil
}
