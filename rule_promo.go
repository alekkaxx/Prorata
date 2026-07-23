package prorata

import "fmt"

// init registers the promo rule for EventApplyPromo.
func init() {
	registerRule(EventApplyPromo, armPromo)
}

// armPromo handles EventApplyPromo: it validates the requested one-shot
// discount — either a percentage (Bps) or a fixed amount (AmountCents), never
// both — and arms st.promo so the engine (applyPromo) can apply it to the next
// event that produces a positive charge. No active subscription is required
// (see specs/04-promo-percent.md, D5): a promo armed before subscribe
// discounts the first charge, e.g. subscribe itself.
//
// The kind of discount is picked by which field is set: a nonzero AmountCents
// selects a fixed discount (specs/05-promo-fixed.md); otherwise the event is
// treated as a percentage discount (specs/04-promo-percent.md), including the
// AmountCents == 0 && Bps == 0 case, which is indistinguishable from a
// zero-percent promo and burns as such.
//
// Stacking is forbidden regardless of the kind of either promo: if a promo is
// already armed and has not yet burned, arming a second one — of any kind —
// fails. This single armed check catches percent-over-percent,
// percent-over-fixed, fixed-over-percent and fixed-over-fixed alike (see
// specs/05-promo-fixed.md, D3).
//
// armPromo never produces invoice lines itself; the promo.percent or
// promo.fixed line is emitted by the engine's applyPromo hook once a positive
// charge appears to discount.
func armPromo(st *state, c Catalog, ev Event) ([]Line, error) {
	if ev.Code == "" {
		return nil, fmt.Errorf("prorata: promo: empty code")
	}
	if ev.Bps != 0 && ev.AmountCents != 0 {
		return nil, fmt.Errorf("prorata: promo: both bps and amount set")
	}

	var next pendingPromo
	if ev.AmountCents != 0 {
		if ev.AmountCents < 0 {
			return nil, fmt.Errorf("prorata: promo: negative amount %d", ev.AmountCents)
		}
		next = pendingPromo{armed: true, kind: promoFixed, amount: ev.AmountCents, code: ev.Code}
	} else {
		if err := validateBps(ev.Bps, "promo"); err != nil {
			return nil, err
		}
		next = pendingPromo{armed: true, kind: promoPercent, bps: ev.Bps, code: ev.Code}
	}

	if st.promo.armed {
		return nil, fmt.Errorf("prorata: promo: promo already pending")
	}

	st.promo = next
	return nil, nil
}
