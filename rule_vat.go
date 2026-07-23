package prorata

import "fmt"

// init registers the VAT-arming rule for EventSetVAT.
func init() {
	registerRule(EventSetVAT, armVAT)
}

// armVAT handles EventSetVAT: it validates the requested standing
// value-added-tax rate and arms st.vatBps so the engine (applyVAT) taxes the
// net charge of every subsequent event while the rate stays positive.
//
// The rate is basis points on the same [0,10000] scale as a percentage promo
// (10000 == 100%); 0 explicitly disarms VAT, which is indistinguishable from
// never having called EventSetVAT (see specs/09-vat.md, D1, edge case 2). The
// rate is a standing state change, not one-shot like a promo: it persists
// until a later EventSetVAT changes it again, and it never taxes charges that
// already happened before this event (see specs/09-vat.md, edge cases 3-4).
//
// armVAT never produces invoice lines itself; the vat.standard line is
// emitted by the engine's applyVAT hook once a taxable positive net charge
// appears.
func armVAT(st *state, c Catalog, ev Event) ([]Line, error) {
	if ev.Bps < 0 || ev.Bps > 10000 {
		return nil, fmt.Errorf("prorata: vat: bps %d out of range [0,10000]", ev.Bps)
	}

	st.vatBps = ev.Bps
	return nil, nil
}
