package prorata

import "fmt"

// init registers the promo rule for EventApplyPromo.
func init() {
	registerRule(EventApplyPromo, armPromo)
}

// armPromo handles EventApplyPromo: it validates the requested one-shot
// percentage discount and arms st.promo so the engine (applyPromo) can apply
// it to the next event that produces a positive charge. No active
// subscription is required (see specs/04-promo-percent.md, D5): a promo armed
// before subscribe discounts the first charge, e.g. subscribe itself.
//
// armPromo never produces invoice lines itself; the promo.percent line is
// emitted by the engine's applyPromo hook once a positive charge appears to
// discount (see specs/04-promo-percent.md).
func armPromo(st *state, c Catalog, ev Event) ([]Line, error) {
	if ev.Code == "" {
		return nil, fmt.Errorf("prorata: promo: empty code")
	}
	if ev.Bps < 0 || ev.Bps > 10000 {
		return nil, fmt.Errorf("prorata: promo: bps %d out of range [0,10000]", ev.Bps)
	}
	if st.promo.armed {
		return nil, fmt.Errorf("prorata: promo: promo already pending")
	}

	st.promo = pendingPromo{armed: true, bps: ev.Bps, code: ev.Code}
	return nil, nil
}
