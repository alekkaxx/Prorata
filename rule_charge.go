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
	plan, ok := c[ev.PlanID]
	if !ok {
		return nil, fmt.Errorf("prorata: subscribe: unknown plan %q", ev.PlanID)
	}
	if st.plan != nil {
		return nil, fmt.Errorf("prorata: subscribe: already subscribed")
	}

	end, err := AddInterval(ev.At, plan.Interval)
	if err != nil {
		return nil, err
	}

	st.plan = &plan
	st.periodStart = ev.At
	st.periodEnd = end
	st.paid = plan.Price
	st.cashPaid = plan.Price

	line := Line{
		RuleID: ruleFullCharge,
		Description: fmt.Sprintf(
			"%s: full period %s to %s",
			plan.ID,
			ev.At.UTC().Format("2006-01-02"),
			end.UTC().Format("2006-01-02"),
		),
		Amount: plan.Price,
	}
	return []Line{line}, nil
}
