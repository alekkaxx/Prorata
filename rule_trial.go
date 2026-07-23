package prorata

import "fmt"

// ruleTrialStart is the RuleID for the zero-amount line emitted when a free
// trial begins.
const ruleTrialStart RuleID = "trial.start"

// ruleConvertCharge is the RuleID for the full-price line emitted when a
// trial converts into a paid subscription.
const ruleConvertCharge RuleID = "convert.charge"

// init registers the trial-start and trial-conversion rules for EventTrial
// and EventConvert.
func init() {
	registerRule(EventTrial, trialStart)
	registerRule(EventConvert, trialConvert)
}

// trialStart handles EventTrial: it opens a free trial billing period on the
// requested plan, with nothing charged.
//
// The trial period is [ev.At, AddInterval(ev.At, plan.Interval)) — the same
// length and day clamp as a paid period on the same plan. State is set with
// paid = 0 and trial = true, so a later EventConvert (or a plan change) never
// credits money that was never actually paid. A trial requires a clean
// state: it errors if a subscription is already active (see specs/07-trial.md,
// D5). The returned line always carries a zero Amount plus a RuleID and
// Description, so the invoice explains why the period is free rather than
// silently omitting the period (see specs/07-trial.md, D2).
func trialStart(st *state, c Catalog, ev Event) ([]Line, error) {
	plan, err := lookupPlan(c, ev.PlanID, "trial")
	if err != nil {
		return nil, err
	}
	if st.plan != nil {
		return nil, fmt.Errorf("prorata: trial: already subscribed")
	}

	end, err := st.openPeriod(plan, ev.At)
	if err != nil {
		return nil, err
	}
	// A trial opens the same period shape as a paid subscription but collects
	// nothing, so the paid bases openPeriod recorded are zeroed.
	st.paid = 0
	st.cashPaid = 0
	st.trial = true

	line := Line{
		RuleID: ruleTrialStart,
		Description: fmt.Sprintf(
			"%s: trial %s to %s (free)",
			plan.ID, formatDay(ev.At), formatDay(end),
		),
		Amount: 0,
	}
	return []Line{line}, nil
}

// trialConvert handles EventConvert: it converts an active trial into a paid
// subscription on the plan named by Event.PlanID (the trial plan, or a
// different one) for a fresh billing period starting at ev.At.
//
// Conversion requires an active trial (st.trial); otherwise it errors,
// covering both an empty state and an active paid subscription. Because the
// trial was free (paid == 0), any unused remainder of the trial period has
// zero value and simply burns — there is nothing to credit or refund,
// regardless of whether ev.At falls mid-trial, at the trial's end, or after
// it (see specs/07-trial.md, D3-D4, D8). The new period is charged in full and
// the trial flag is cleared.
func trialConvert(st *state, c Catalog, ev Event) ([]Line, error) {
	if !st.trial {
		return nil, fmt.Errorf("prorata: convert: no active trial")
	}
	plan, err := lookupPlan(c, ev.PlanID, "convert")
	if err != nil {
		return nil, err
	}

	end, err := st.openPeriod(plan, ev.At)
	if err != nil {
		return nil, err
	}
	st.trial = false

	return []Line{fullPeriodLine(ruleConvertCharge, plan, ev.At, end)}, nil
}
