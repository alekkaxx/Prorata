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
}

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
