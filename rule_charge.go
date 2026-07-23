package prorata

import "fmt"

// ruleFullCharge is the RuleID for the initial full-period charge produced
// when a subscription starts.
const ruleFullCharge RuleID = "charge.full"

// init registers the full-charge rule for EventSubscribe.
func init() {
	registerRule(EventSubscribe, chargeFull)
}

// chargeFull handles EventSubscribe: it starts a subscription on the
// requested plan and charges its full price for the first billing period.
//
// The billing period is [ev.At, AddInterval(ev.At, plan.Interval)), with the
// day of the period end clamped per AddInterval's rules (see specs/00-core.md,
// D3). The subscription state is built regardless of whether ev.At falls
// inside the period requested from Compute; the engine decides whether the
// returned line is included in the invoice.
func chargeFull(st *state, c Catalog, ev Event) ([]Line, error) {
	plan, err := lookupPlan(c, ev.PlanID, "subscribe")
	if err != nil {
		return nil, err
	}
	if st.plan != nil {
		return nil, fmt.Errorf("prorata: subscribe: already subscribed")
	}

	end, err := st.openPeriod(plan, ev.At)
	if err != nil {
		return nil, err
	}
	return []Line{fullPeriodLine(ruleFullCharge, plan, ev.At, end)}, nil
}
