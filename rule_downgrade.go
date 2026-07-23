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
	newPlan, ok := c[ev.PlanID]
	if !ok {
		return nil, fmt.Errorf("prorata: downgrade: unknown plan %q", ev.PlanID)
	}
	if st.plan.ID == ev.PlanID {
		return nil, fmt.Errorf("prorata: downgrade: already on plan %q", ev.PlanID)
	}

	oldPeriodEnd := st.periodEnd
	total := Period{Start: st.periodStart, End: oldPeriodEnd}.Days()
	rem := max(Period{Start: ev.At, End: oldPeriodEnd}.Days(), 0)
	used := total - rem

	parts, err := st.paid.Allocate([]int64{int64(used), int64(rem)})
	if err != nil {
		return nil, err
	}
	st.creditBalance += parts[1]

	newEnd, err := AddInterval(ev.At, newPlan.Interval)
	if err != nil {
		return nil, err
	}

	st.plan = &newPlan
	st.periodStart = ev.At
	st.periodEnd = newEnd
	st.paid = newPlan.Price

	chargeLine := Line{
		RuleID: ruleDowngradeCharge,
		Description: fmt.Sprintf(
			"%s: full period %s to %s",
			newPlan.ID,
			ev.At.UTC().Format("2006-01-02"),
			newEnd.UTC().Format("2006-01-02"),
		),
		Amount: newPlan.Price,
	}
	return []Line{chargeLine}, nil
}
