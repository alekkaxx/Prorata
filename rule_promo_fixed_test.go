package prorata

import (
	"testing"
	"time"
)

// TestPromoFixed tests the promo fixed discount rule behavior through public Compute.
func TestPromoFixed(t *testing.T) {
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
			name: "fixed-on-first-charge",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 500,
					Code:        "FIVEOFF",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify promo.fixed line exists with -500
				var foundPromoLine bool
				for _, line := range inv.Lines {
					if line.RuleID == "promo.fixed" {
						if line.Amount != -500 {
							t.Errorf("promo.fixed amount = %d, want -500", line.Amount)
						}
						if line.Description != "FIVEOFF: -5.00 off 20.00" {
							t.Errorf("promo.fixed description = %q, want %q", line.Description, "FIVEOFF: -5.00 off 20.00")
						}
						foundPromoLine = true
					}
				}
				if !foundPromoLine {
					t.Errorf("promo.fixed line not found")
				}
				// Verify total is 1500
				if inv.Total != 1500 {
					t.Errorf("invoice total = %d, want 1500", inv.Total)
				}
				// Verify sum equals total (invariant)
				var sum Money
				for _, line := range inv.Lines {
					sum += line.Amount
				}
				if sum != inv.Total {
					t.Errorf("sum of line amounts = %d, total = %d, mismatch", sum, inv.Total)
				}
			},
		},
		{
			name: "fixed-clamp-exceeds-charge",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 5000,
					Code:        "BIGCREDIT",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify promo.fixed line exists with -2000 (clamped)
				var foundPromoLine bool
				for _, line := range inv.Lines {
					if line.RuleID == "promo.fixed" {
						if line.Amount != -2000 {
							t.Errorf("promo.fixed amount = %d, want -2000", line.Amount)
						}
						if line.Description != "BIGCREDIT: -20.00 off 20.00 (capped from 50.00)" {
							t.Errorf("promo.fixed description = %q, want %q", line.Description, "BIGCREDIT: -20.00 off 20.00 (capped from 50.00)")
						}
						foundPromoLine = true
					}
				}
				if !foundPromoLine {
					t.Errorf("promo.fixed line not found")
				}
				// Verify total is 0
				if inv.Total != 0 {
					t.Errorf("invoice total = %d, want 0", inv.Total)
				}
				// Verify sum equals total (invariant)
				var sum Money
				for _, line := range inv.Lines {
					sum += line.Amount
				}
				if sum != inv.Total {
					t.Errorf("sum of line amounts = %d, total = %d, mismatch", sum, inv.Total)
				}
			},
		},
		{
			name: "fixed-does-not-discount-prorate-credit",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
				"business-year": {
					ID:       "business-year",
					Price:    48000,
					Interval: "year",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
				{
					At:          time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 1000,
					Code:        "TENOFF",
				},
				{
					At:     time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2027, 1, 13, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify prorate.credit remains -1226 (not discounted)
				var prorateCredit Money
				for _, line := range inv.Lines {
					if line.RuleID == "prorate.credit" {
						prorateCredit = line.Amount
					}
				}
				if prorateCredit != -1226 {
					t.Errorf("prorate.credit = %d, want -1226", prorateCredit)
				}
				// Verify promo.fixed is -1000
				var promoFixed Money
				for _, line := range inv.Lines {
					if line.RuleID == "promo.fixed" {
						promoFixed = line.Amount
					}
				}
				if promoFixed != -1000 {
					t.Errorf("promo.fixed = %d, want -1000", promoFixed)
				}
				// Verify total is 45774
				if inv.Total != 45774 {
					t.Errorf("invoice total = %d, want 45774", inv.Total)
				}
				// Verify sum equals total (invariant)
				var sum Money
				for _, line := range inv.Lines {
					sum += line.Amount
				}
				if sum != inv.Total {
					t.Errorf("sum of line amounts = %d, total = %d, mismatch", sum, inv.Total)
				}
			},
		},
		{
			name: "fixed-before-credit-balance-order",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
				"team-month": {
					ID:       "team-month",
					Price:    5000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "team-month",
				},
				{
					At:          time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 500,
					Code:        "SAVE5",
				},
				{
					At:     time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify credit.applied is -1500 (by discounted net)
				var creditApplied Money
				for _, line := range inv.Lines {
					if line.RuleID == "credit.applied" {
						creditApplied += line.Amount
					}
				}
				if creditApplied != -1500 {
					t.Errorf("credit.applied = %d, want -1500", creditApplied)
				}
				// Verify promo.fixed is -500
				var promoFixed Money
				for _, line := range inv.Lines {
					if line.RuleID == "promo.fixed" {
						promoFixed = line.Amount
					}
				}
				if promoFixed != -500 {
					t.Errorf("promo.fixed = %d, want -500", promoFixed)
				}
				// Verify total is 0
				if inv.Total != 0 {
					t.Errorf("invoice total = %d, want 0", inv.Total)
				}
				// Verify no line is negative except promo.fixed and credit.applied
				for _, line := range inv.Lines {
					if line.Amount < 0 && line.RuleID != "promo.fixed" && line.RuleID != "credit.applied" {
						t.Errorf("found negative line %q with amount %d, want only promo.fixed and credit.applied", line.RuleID, line.Amount)
					}
				}
				// Verify sum equals total (invariant)
				var sum Money
				for _, line := range inv.Lines {
					sum += line.Amount
				}
				if sum != inv.Total {
					t.Errorf("sum of line amounts = %d, total = %d, mismatch", sum, inv.Total)
				}
			},
		},
		{
			name: "both-bps-and-amount-set",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					Bps:         2000,
					AmountCents: 500,
					Code:        "X",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: both bps and amount set",
		},
		{
			name: "stacking-fixed-over-percent",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type: "apply_promo",
					Bps:  2000,
					Code: "PCT20",
				},
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 500,
					Code:        "FIX5",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: promo already pending",
		},
		{
			name: "stacking-percent-over-fixed",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 500,
					Code:        "F",
				},
				{
					At:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type: "apply_promo",
					Bps:  2000,
					Code: "P",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: promo already pending",
		},
		{
			name: "stacking-fixed-over-fixed",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 200,
					Code:        "FIRST",
				},
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: 300,
					Code:        "SECOND",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: promo already pending",
		},
		{
			name: "negative-fixed-amount",
			catalog: Catalog{
				"pro-month": {
					ID:       "pro-month",
					Price:    2000,
					Interval: "month",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:        "apply_promo",
					AmountCents: -100,
					Code:        "X",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: negative amount -100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := Compute(tt.catalog, tt.events, tt.period)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("Compute() error message = %q, want %q", err.Error(), tt.errMsg)
				}
			}
			if !tt.wantErr && err == nil && tt.check != nil {
				tt.check(t, &inv)
			}
		})
	}
}
