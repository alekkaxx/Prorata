package prorata

import (
	"testing"
	"time"
)

// TestVAT exercises the EventSetVAT rule and the engine's applyVAT hook
// through public Compute, covering the eight table cases of
// specs/09-vat.md.
func TestVAT(t *testing.T) {
	tests := []struct {
		name    string
		catalog Catalog
		events  []Event
		period  Period
		wantErr bool
		errMsg  string
		check   func(t *testing.T, inv *Invoice)
	}{
		{
			// Case 1: set_vat + subscribe (example 1). VAT taxes the full charge.
			name: "set-vat-then-subscribe",
			catalog: Catalog{
				"pro-month": {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSetVAT, Bps: 2000},
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSubscribe, PlanID: "pro-month"},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "charge.full", Description: "pro-month: full period 2026-01-01 to 2026-02-01", Amount: 2000},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 20.00", Amount: 400},
				}
				assertLines(t, inv, wantLines, 2400)
			},
		},
		{
			// Case 2: set_vat + subscribe + upgrade (example 2). VAT base is net
			// (post proration-credit), so the upgrade's tax lands on 39.68, not
			// on the full 50.00 sticker price: no double VAT on overlapping days.
			name: "set-vat-upgrade-net-base",
			catalog: Catalog{
				"pro-month": {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
				"biz-month": {ID: "biz-month", Price: 5000, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSetVAT, Bps: 2000},
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSubscribe, PlanID: "pro-month"},
				{At: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Type: EventUpgrade, PlanID: "biz-month"},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "charge.full", Description: "pro-month: full period 2026-01-01 to 2026-02-01", Amount: 2000},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 20.00", Amount: 400},
					{RuleID: "prorate.credit", Description: "pro-month: unused 16/31 days 2026-01-16 to 2026-02-01", Amount: -1032},
					{RuleID: "prorate.charge", Description: "biz-month: full period 2026-01-16 to 2026-02-16", Amount: 5000},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 39.68", Amount: 794},
				}
				assertLines(t, inv, wantLines, 7162)
			},
		},
		{
			// Case 3: set_vat + promo + subscribe (example 3). VAT taxes the
			// discounted price (15.00), not the sticker price (20.00).
			name: "set-vat-promo-discounted-base",
			catalog: Catalog{
				"pro-month": {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSetVAT, Bps: 2000},
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventApplyPromo, Bps: 2500, Code: "WELCOME"},
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSubscribe, PlanID: "pro-month"},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "charge.full", Description: "pro-month: full period 2026-01-01 to 2026-02-01", Amount: 2000},
					{RuleID: "promo.percent", Description: "WELCOME: -25% off 20.00", Amount: -500},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 15.00", Amount: 300},
				}
				assertLines(t, inv, wantLines, 1800)
			},
		},
		{
			// Case 4: set_vat + subscribe + downgrade (example 4). VAT is
			// computed before applyCredit (on the full 800 downgrade charge);
			// credit.applied itself carries no VAT.
			name: "set-vat-downgrade-credit-order",
			catalog: Catalog{
				"pro-month":   {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
				"basic-month": {ID: "basic-month", Price: 800, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSetVAT, Bps: 2000},
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSubscribe, PlanID: "pro-month"},
				{At: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Type: EventDowngrade, PlanID: "basic-month"},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "charge.full", Description: "pro-month: full period 2026-01-01 to 2026-02-01", Amount: 2000},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 20.00", Amount: 400},
					{RuleID: "downgrade.charge", Description: "basic-month: full period 2026-01-16 to 2026-02-16", Amount: 800},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 8.00", Amount: 160},
					{RuleID: ruleCreditApplied, Description: creditAppliedDescription, Amount: -960},
				}
				assertLines(t, inv, wantLines, 2400)
			},
		},
		{
			// Case 5: subscribe without any set_vat. Retro-compatibility: the
			// invoice is byte-for-byte golden/01-monthly-subscribe.json, no
			// vat.standard line anywhere.
			name: "no-set-vat-is-retrocompatible",
			catalog: Catalog{
				"pro-month": {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSubscribe, PlanID: "pro-month"},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "charge.full", Description: "pro-month: full period 2026-01-01 to 2026-02-01", Amount: 2000},
				}
				assertLines(t, inv, wantLines, 2000)
			},
		},
		{
			// Case 6: set_vat armed AFTER a charge. No retroactive VAT: the
			// first charge (subscribe) is untaxed; only the later upgrade,
			// which happens after the rate is armed, is taxed.
			name: "set-vat-after-charge-is-not-retroactive",
			catalog: Catalog{
				"pro-month": {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
				"biz-month": {ID: "biz-month", Price: 5000, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSubscribe, PlanID: "pro-month"},
				{At: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), Type: EventSetVAT, Bps: 2000},
				{At: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Type: EventUpgrade, PlanID: "biz-month"},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "charge.full", Description: "pro-month: full period 2026-01-01 to 2026-02-01", Amount: 2000},
					{RuleID: "prorate.credit", Description: "pro-month: unused 16/31 days 2026-01-16 to 2026-02-01", Amount: -1032},
					{RuleID: "prorate.charge", Description: "biz-month: full period 2026-01-16 to 2026-02-16", Amount: 5000},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 39.68", Amount: 794},
				}
				assertLines(t, inv, wantLines, 6762)
			},
		},
		{
			// Case 7: set_vat armed, then trial_start. A trial's net is 0
			// (nothing charged), so no vat line is produced.
			name: "set-vat-trial-has-no-vat",
			catalog: Catalog{
				"pro-month": {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSetVAT, Bps: 2000},
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventTrial, PlanID: "pro-month"},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "trial.start", Description: "pro-month: trial 2026-01-01 to 2026-02-01 (free)", Amount: 0},
				}
				assertLines(t, inv, wantLines, 0)
			},
		},
		{
			// Case 8: set_vat + subscribe + prorated refund. The charge is
			// taxed, but the refund (net < 0) is not: refund does not reverse
			// VAT (known limitation, see specs/09-vat.md D3 and edge case 7).
			name: "set-vat-refund-does-not-reverse-vat",
			catalog: Catalog{
				"pro-month": {ID: "pro-month", Price: 2000, Interval: "month", Currency: "USD"},
			},
			events: []Event{
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSetVAT, Bps: 2000},
				{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Type: EventSubscribe, PlanID: "pro-month"},
				{At: time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC), Type: EventRefund, PlanID: "pro-month", Policy: RefundProrated},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			check: func(t *testing.T, inv *Invoice) {
				wantLines := []Line{
					{RuleID: "charge.full", Description: "pro-month: full period 2026-01-01 to 2026-02-01", Amount: 2000},
					{RuleID: ruleVATStandard, Description: "VAT 20% on 20.00", Amount: 400},
					{RuleID: "refund.prorated", Description: "pro-month: refund unused 19/31 days 2026-01-13 to 2026-02-01", Amount: -1226},
				}
				assertLines(t, inv, wantLines, 1174)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := Compute(tt.catalog, tt.events, tt.period)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Compute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if err.Error() != tt.errMsg {
					t.Errorf("Compute() error message = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}
			if tt.check != nil {
				tt.check(t, &inv)
			}
		})
	}
}

// assertLines checks that inv has exactly wantLines in order and that its
// Total matches wantTotal, and that Total equals the sum of the line
// amounts (the money invariant).
func assertLines(t *testing.T, inv *Invoice, wantLines []Line, wantTotal Money) {
	t.Helper()
	if len(inv.Lines) != len(wantLines) {
		t.Fatalf("lines = %+v, want %+v", inv.Lines, wantLines)
	}
	for i, want := range wantLines {
		got := inv.Lines[i]
		if got.RuleID != want.RuleID || got.Description != want.Description || got.Amount != want.Amount {
			t.Errorf("line[%d] = %+v, want %+v", i, got, want)
		}
	}
	var sum Money
	for _, ln := range inv.Lines {
		sum += ln.Amount
	}
	if sum != inv.Total {
		t.Errorf("sum of line amounts = %d, total = %d, mismatch", sum, inv.Total)
	}
	if inv.Total != wantTotal {
		t.Errorf("invoice total = %d, want %d", inv.Total, wantTotal)
	}
}
