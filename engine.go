package prorata

import (
	"fmt"
	"time"
)

// state is the subscription state folded over the event history. Rules read
// and mutate it as events are applied in order.
type state struct {
	plan        *Plan
	periodStart time.Time
	periodEnd   time.Time
	paid        Money
	// cashPaid is the cash actually collected for the current period: the plan
	// price a charging rule opened the period with, reduced by any promo discount
	// and any credit balance the engine applied to that charge. It is the honest
	// basis for a refund (see specs/08-refund.md): refunding against nominal paid
	// could hand back more than the customer ever paid when a promo or credit
	// covered part of the charge, breaking the "credit <= actually paid"
	// invariant. cashPaid is always in [0, paid]. It is separate from paid on
	// purpose: proration on upgrade/downgrade credits the unused remainder at
	// list value (paid), a deliberate product choice, whereas a cash refund must
	// never exceed cash received.
	cashPaid Money
	// trial reports whether the current plan/period is a free trial rather than
	// a paid subscription. It is set by the trial rule (EventTrial) and cleared
	// when the trial is converted (EventConvert). It distinguishes a genuine
	// trial (paid == 0 because nothing was ever charged) from a subscription to
	// a zero-price plan, so conversion applies only to a real trial and the
	// credit invariant (credit <= actually paid) is never at risk
	// (see specs/07-trial.md).
	trial bool
	// creditBalance is prepaid value the customer is owed but was not refunded
	// in cash (banked by a downgrade). It is always >= 0 and never exceeds the
	// sum of amounts actually paid. The engine draws it down against charges
	// (see applyCredit and specs/03-downgrade-credit.md).
	creditBalance Money
	// promo is a one-shot percentage discount armed by an EventApplyPromo event
	// and consumed by the engine (applyPromo) against the next event that
	// produces a positive charge. It carries no discount once burned
	// (see specs/04-promo-percent.md).
	promo pendingPromo
	// vatBps is the standing value-added-tax rate in basis points (10000 ==
	// 100%) armed by an EventSetVAT event. Unlike promo it is not one-shot: it
	// persists and the engine (applyVAT) taxes the net charge of every
	// subsequent event while it is > 0. It defaults to 0, so an event history
	// with no EventSetVAT adds no VAT and leaves pre-VAT invoices byte-identical
	// (see specs/09-vat.md).
	vatBps int64
}

// promoKind distinguishes the two one-shot promo flavors a pendingPromo can
// hold. It is only meaningful while armed is true; the zero value coincides
// with promoPercent, which is harmless because a disarmed promo is never read.
type promoKind uint8

const (
	// promoPercent takes bps basis points off the gross charge, rounded half
	// away from zero (rule promo.percent, specs/04-promo-percent.md).
	promoPercent promoKind = iota
	// promoFixed takes a flat number of minor units off the gross charge,
	// clamped so the discount never exceeds the charge (rule promo.fixed,
	// specs/05-promo-fixed.md).
	promoFixed
)

// pendingPromo is a one-shot discount waiting to be applied to the next
// positive charge. The rule handling EventApplyPromo arms it; applyPromo burns
// it the moment it discounts a charge. A zero pendingPromo (armed == false)
// means nothing is pending. kind selects which of bps (promoPercent) or amount
// (promoFixed) carries the discount.
type pendingPromo struct {
	armed  bool
	kind   promoKind
	bps    int64
	amount Money
	code   string
}

// ruleCreditApplied is the RuleID for lines that draw down the subscription's
// credit balance to offset a charge. These lines are produced by the engine,
// not by an event rule: the balance is core ledger state, so any charging
// rule's output can be offset against it uniformly
// (see specs/03-downgrade-credit.md).
const ruleCreditApplied RuleID = "credit.applied"

// creditAppliedDescription is the fixed explanation carried by every
// credit.applied line. The provenance of the balance (which downgrade banked
// it) is documented in specs/03-downgrade-credit.md rather than per line.
const creditAppliedDescription = "credit balance applied"

// rulePromoPercent is the RuleID for the discount line produced when a pending
// percentage promo is applied to an event's positive charges. Like
// credit.applied, this line is produced by the engine, not by an event rule:
// the promo is armed as core state, so any charging rule's output can be
// discounted uniformly (see specs/04-promo-percent.md).
const rulePromoPercent RuleID = "promo.percent"

// rulePromoFixed is the RuleID for the discount line produced when a pending
// fixed-amount promo is applied to an event's positive charges. Like
// promo.percent it is produced by the engine (applyPromo), not by an event
// rule, so any charging rule's output can be discounted uniformly. The applied
// amount is clamped to the gross charge so money is never created
// (see specs/05-promo-fixed.md).
const rulePromoFixed RuleID = "promo.fixed"

// ruleVATStandard is the RuleID for the value-added-tax line the engine adds
// on top of an event's net charge when a standing VAT rate is armed. Like
// promo.percent and credit.applied it is produced by the engine (applyVAT),
// not by an event rule: the rate is core ledger state (state.vatBps), so any
// charging rule's output is taxed uniformly (see specs/09-vat.md).
const ruleVATStandard RuleID = "vat.standard"

// ruleFunc applies one event to the subscription state and returns the
// invoice lines the event produces, if any.
type ruleFunc func(st *state, c Catalog, ev Event) ([]Line, error)

// rules maps each event type to the single rule that handles it. Rule files
// (rule_*.go) register themselves via registerRule in init, so adding a rule
// never requires editing the core.
var rules = map[EventType]ruleFunc{}

// registerRule binds a rule to an event type. Exactly one rule per event
// type is allowed; a duplicate registration is a programming error.
func registerRule(t EventType, fn ruleFunc) {
	if _, dup := rules[t]; dup {
		panic(fmt.Sprintf("prorata: duplicate rule for event type %q", t))
	}
	rules[t] = fn
}

// Compute replays the event history and returns the invoice for the
// requested period. Events must be sorted chronologically; all events build
// subscription state, but only events inside the period contribute lines.
// Compute is a pure function: the same input always yields the same invoice.
func Compute(c Catalog, events []Event, period Period) (Invoice, error) {
	if err := period.validate(); err != nil {
		return Invoice{}, err
	}
	if err := c.validate(); err != nil {
		return Invoice{}, err
	}
	for i := 1; i < len(events); i++ {
		if events[i].At.Before(events[i-1].At) {
			return Invoice{}, fmt.Errorf("prorata: events out of order at index %d", i)
		}
	}

	st := &state{}
	inv := Invoice{Currency: c.currency()}
	for _, ev := range events {
		fn, ok := rules[ev.Type]
		if !ok {
			return Invoice{}, fmt.Errorf("prorata: no rule registered for event type %q", ev.Type)
		}
		lines, err := fn(st, c, ev)
		if err != nil {
			return Invoice{}, err
		}
		lines = applyPromo(st, lines)
		lines = applyVAT(st, lines)
		lines = applyCredit(st, lines)
		if !period.Contains(ev.At) {
			continue
		}
		for _, ln := range lines {
			if ln.RuleID == "" || ln.Description == "" {
				return Invoice{}, fmt.Errorf("prorata: rule produced unexplained line %+v", ln)
			}
		}
		inv.Lines = append(inv.Lines, lines...)
	}

	for _, ln := range inv.Lines {
		inv.Total += ln.Amount
	}
	return inv, nil
}

// applyCredit offsets the net positive charge of one event's lines against the
// subscription's credit balance. If the balance is positive and the lines sum
// to a positive amount, it draws the smaller of the two from the balance and
// appends a single credit.applied line for that amount (negative). Because it
// never draws more than the balance, the balance stays >= 0, so the total
// credit ever applied cannot exceed the total banked, which cannot exceed what
// was actually paid.
//
// The balance is mutated for every event, including events outside the
// requested period, so the ledger stays consistent across the period boundary;
// the caller decides whether the returned lines reach the invoice.
func applyCredit(st *state, lines []Line) []Line {
	if st.creditBalance <= 0 {
		return lines
	}
	var net Money
	for _, ln := range lines {
		net += ln.Amount
	}
	if net <= 0 {
		return lines
	}
	applied := min(net, st.creditBalance)
	st.creditBalance -= applied
	st.cashPaid = max(0, st.cashPaid-applied)
	return append(lines, Line{
		RuleID:      ruleCreditApplied,
		Description: creditAppliedDescription,
		Amount:      -applied,
	})
}

// applyPromo discounts an event's positive charges by the armed one-shot promo,
// if any. It sums the positive-amount lines of the event (its gross charge,
// base), computes the discount according to the promo kind, appends a single
// negative discount line, and burns the promo. Negative lines (credits) are
// never discounted: a promo reduces what is charged, not what is refunded.
//
// For promoPercent the discount is base.Percent(bps) (half away from zero) on a
// promo.percent line. For promoFixed the discount is min(amount, base) — clamped
// so it can never exceed the charge and create money — on a promo.fixed line; a
// clamped line names the nominal amount it was capped from.
//
// If no promo is armed, or the event has no positive charge, the lines are
// returned unchanged and the promo stays armed until a charge appears. applyPromo
// runs before applyCredit so the credit balance draws down the already-discounted
// net, keeping credit.applied on the post-discount charge
// (see specs/04-promo-percent.md D2, specs/05-promo-fixed.md).
func applyPromo(st *state, lines []Line) []Line {
	if !st.promo.armed {
		return lines
	}
	var base Money
	for _, ln := range lines {
		if ln.Amount > 0 {
			base += ln.Amount
		}
	}
	if base <= 0 {
		return lines
	}
	p := st.promo
	st.promo = pendingPromo{}
	if p.kind == promoFixed {
		applied := min(p.amount, base)
		st.cashPaid = max(0, st.cashPaid-applied)
		desc := fmt.Sprintf("%s: -%s off %s", p.code, applied.String(), base.String())
		if p.amount > base {
			desc = fmt.Sprintf("%s: -%s off %s (capped from %s)",
				p.code, applied.String(), base.String(), p.amount.String())
		}
		return append(lines, Line{
			RuleID:      rulePromoFixed,
			Description: desc,
			Amount:      -applied,
		})
	}
	applied := base.Percent(p.bps)
	st.cashPaid = max(0, st.cashPaid-applied)
	return append(lines, Line{
		RuleID: rulePromoPercent,
		Description: fmt.Sprintf(
			"%s: -%s%% off %s",
			p.code,
			formatPercent(p.bps),
			base.String(),
		),
		Amount: -applied,
	})
}

// applyVAT adds value-added tax on top of one event's net charge when a
// standing VAT rate is armed (state.vatBps > 0). It sums the event's lines as
// they stand after applyPromo and before applyCredit — that net is the amount
// actually invoiced for the event: positive charges, less any proration credit
// (which reverses VAT on the overlapping days) and any promo discount (VAT is
// due on the discounted price). If that net is positive it appends a single
// vat.standard line for net.Percent(vatBps), rounded half away from zero per
// charge, not once over the whole invoice — the per-line vs per-total penny
// trap this rule exists to pin down (see specs/09-vat.md D2).
//
// A net <= 0 event (a standalone credit or refund, a trial, a zero-price plan)
// produces no VAT line: VAT is added to positive charges only, and standalone
// credits do not reverse it (out of scope, see specs/09-vat.md). Because the
// rate defaults to 0, a history with no EventSetVAT yields no vat lines and
// leaves pre-VAT invoices byte-identical. applyVAT runs before applyCredit so
// the credit balance draws down the tax-inclusive net.
func applyVAT(st *state, lines []Line) []Line {
	if st.vatBps <= 0 {
		return lines
	}
	var net Money
	for _, ln := range lines {
		net += ln.Amount
	}
	if net <= 0 {
		return lines
	}
	return append(lines, Line{
		RuleID:      ruleVATStandard,
		Description: fmt.Sprintf("VAT %s%% on %s", formatPercent(st.vatBps), net.String()),
		Amount:      net.Percent(st.vatBps),
	})
}
