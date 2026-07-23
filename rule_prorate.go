package prorata

import "fmt"

// ruleProrateCredit is the RuleID for the unused-remainder credit line
// produced when an active subscription is upgraded mid-period.
const ruleProrateCredit RuleID = "prorate.credit"

// ruleProrateCharge is the RuleID for the new-plan full-period charge line
// produced by an upgrade.
const ruleProrateCharge RuleID = "prorate.charge"

// init registers the proration rule for EventUpgrade.
func init() {
	registerRule(EventUpgrade, prorateUpgrade)
}

// prorateUpgrade handles EventUpgrade: it switches an active subscription to
// a new plan mid-period. It credits the unused remainder of the old plan's
// current period (by days, via Money.Allocate) and charges the new plan in
// full for its first billing period starting at ev.At. See
// specs/02-prorate-upgrade.md for the derivation.
//
// The two lines are always returned in the order [credit, charge], and the
// subscription state is rewritten to the new plan regardless of whether
// ev.At falls inside the period requested from Compute.
func prorateUpgrade(st *state, c Catalog, ev Event) ([]Line, error) {
	if st.plan == nil {
		return nil, fmt.Errorf("prorata: upgrade: no active subscription")
	}
	newPlan, err := lookupPlan(c, ev.PlanID, "upgrade")
	if err != nil {
		return nil, err
	}
	if st.plan.ID == ev.PlanID {
		return nil, fmt.Errorf("prorata: upgrade: already on plan %q", ev.PlanID)
	}

	oldPlan := st.plan
	oldPeriodEnd := st.periodEnd
	credit, rem, total, err := unusedShare(
		st.paid, Period{Start: st.periodStart, End: oldPeriodEnd}, ev.At)
	if err != nil {
		return nil, err
	}

	newEnd, err := st.openPeriod(newPlan, ev.At)
	if err != nil {
		return nil, err
	}

	creditLine := Line{
		RuleID: ruleProrateCredit,
		Description: fmt.Sprintf(
			"%s: unused %d/%d days %s to %s",
			oldPlan.ID, rem, total,
			formatDay(ev.At), formatDay(oldPeriodEnd),
		),
		Amount: -credit,
	}
	return []Line{creditLine, fullPeriodLine(ruleProrateCharge, newPlan, ev.At, newEnd)}, nil
}
