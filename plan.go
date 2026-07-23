package prorata

import (
	"errors"
	"fmt"
)

// Interval is the billing interval of a plan.
type Interval string

// Supported billing intervals.
const (
	IntervalMonth Interval = "month"
	IntervalYear  Interval = "year"
)

// Plan describes a subscription plan a customer can be billed for.
type Plan struct {
	ID       string   `json:"id"`
	Price    Money    `json:"price_cents"`
	Interval Interval `json:"interval"`
	Currency string   `json:"currency"`
}

// Catalog is the set of known plans, keyed by Plan.ID.
// All plans in a catalog must share one currency (see specs/00-core.md, D5).
type Catalog map[string]Plan

// lookupPlan resolves a plan ID against the catalog for the named operation,
// with the uniform "unknown plan" error every rule reports.
func lookupPlan(c Catalog, id, op string) (Plan, error) {
	p, ok := c[id]
	if !ok {
		return Plan{}, fmt.Errorf("prorata: %s: unknown plan %q", op, id)
	}
	return p, nil
}

// validate checks catalog integrity: key/ID agreement, known intervals,
// non-negative prices and a single currency across all plans.
func (c Catalog) validate() error {
	currency := ""
	for id, p := range c {
		if p.ID != id {
			return fmt.Errorf("prorata: catalog key %q does not match plan ID %q", id, p.ID)
		}
		if p.Interval != IntervalMonth && p.Interval != IntervalYear {
			return fmt.Errorf("prorata: plan %q has unknown interval %q", id, p.Interval)
		}
		if p.Price < 0 {
			return fmt.Errorf("prorata: plan %q has negative price", id)
		}
		if p.Currency == "" {
			return fmt.Errorf("prorata: plan %q has empty currency", id)
		}
		if currency == "" {
			currency = p.Currency
		} else if p.Currency != currency {
			return errors.New("prorata: catalog mixes currencies")
		}
	}
	return nil
}

// currency returns the single currency shared by all plans in the catalog,
// or "" for an empty catalog. Only meaningful after validate has passed.
func (c Catalog) currency() string {
	for _, p := range c {
		return p.Currency
	}
	return ""
}
