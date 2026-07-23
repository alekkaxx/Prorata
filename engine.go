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
	// creditBalance is prepaid value the customer is owed but was not refunded
	// in cash (banked by a downgrade). It is always >= 0 and never exceeds the
	// sum of amounts actually paid. The engine draws it down against charges
	// (see applyCredit and specs/03-downgrade-credit.md).
	creditBalance Money
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
	return append(lines, Line{
		RuleID:      ruleCreditApplied,
		Description: creditAppliedDescription,
		Amount:      -applied,
	})
}
