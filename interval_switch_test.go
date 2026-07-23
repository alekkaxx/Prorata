package prorata

import (
	"testing"
	"time"
)

// TestIntervalSwitch tests interval switching (month↔year) through public Compute.
func TestIntervalSwitch(t *testing.T) {
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
			name: "month-to-year-366-days",
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
					At:     time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2024, 2, 5, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify total is 49032
				if inv.Total != 49032 {
					t.Errorf("invoice total = %d, want 49032", inv.Total)
				}
				// Verify prorate.credit line exists with -968
				var foundCredit bool
				for _, line := range inv.Lines {
					if line.RuleID == "prorate.credit" {
						if line.Amount != -968 {
							t.Errorf("prorate.credit amount = %d, want -968", line.Amount)
						}
						foundCredit = true
					}
				}
				if !foundCredit {
					t.Errorf("prorate.credit line not found")
				}
				// Verify prorate.charge line exists with +48000 for business-year
				var foundCharge bool
				for _, line := range inv.Lines {
					if line.RuleID == "prorate.charge" && line.Amount == 48000 {
						if line.Description != "business-year: full period 2024-02-05 to 2025-02-05" {
							t.Errorf("prorate.charge description = %q, want %q", line.Description, "business-year: full period 2024-02-05 to 2025-02-05")
						}
						foundCharge = true
					}
				}
				if !foundCharge {
					t.Errorf("prorate.charge line not found with correct amount and description")
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
			name: "year-to-month-clamp-31-to-28-feb",
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
					PlanID: "business-year",
				},
				{
					At:     time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify total is 48000
				if inv.Total != 48000 {
					t.Errorf("invoice total = %d, want 48000", inv.Total)
				}
				// Verify downgrade.charge line with clamped period
				var foundDowngradeCharge bool
				for _, line := range inv.Lines {
					if line.RuleID == "downgrade.charge" {
						if line.Description != "pro-month: full period 2026-01-31 to 2026-02-28" {
							t.Errorf("downgrade.charge description = %q, want %q", line.Description, "pro-month: full period 2026-01-31 to 2026-02-28")
						}
						foundDowngradeCharge = true
					}
				}
				if !foundDowngradeCharge {
					t.Errorf("downgrade.charge line not found with clamped period")
				}
				// Verify exactly one credit.applied line with -2000
				creditAppliedCount := 0
				for _, line := range inv.Lines {
					if line.RuleID == "credit.applied" {
						creditAppliedCount++
						if line.Amount != -2000 {
							t.Errorf("credit.applied amount = %d, want -2000", line.Amount)
						}
					}
				}
				if creditAppliedCount != 1 {
					t.Errorf("credit.applied line count = %d, want 1", creditAppliedCount)
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
			name: "year-anchor-feb29-365-days",
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
					At:     time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "business-year",
				},
				{
					At:     time.Date(2024, 8, 29, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify charge.full line with period clamped to 2025-02-28
				var foundChargeFullLine bool
				for _, line := range inv.Lines {
					if line.RuleID == "charge.full" {
						if line.Description != "business-year: full period 2024-02-29 to 2025-02-28" {
							t.Errorf("charge.full description = %q, want %q", line.Description, "business-year: full period 2024-02-29 to 2025-02-28")
						}
						foundChargeFullLine = true
					}
				}
				if !foundChargeFullLine {
					t.Errorf("charge.full line not found with clamped period")
				}
				// Verify total is 48000
				if inv.Total != 48000 {
					t.Errorf("invoice total = %d, want 48000", inv.Total)
				}
				// Verify sum of |credit.applied| == 2000 (all applied credit)
				var creditAppliedSum Money
				for _, line := range inv.Lines {
					if line.RuleID == "credit.applied" {
						creditAppliedSum += -line.Amount
					}
				}
				if creditAppliedSum != 2000 {
					t.Errorf("sum of |credit.applied| = %d, want 2000", creditAppliedSum)
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
			name: "chain-month-year-month-366-days",
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
					At:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2024, 2, 5, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
				{
					At:     time.Date(2024, 8, 5, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify total is 49355
				if inv.Total != 49355 {
					t.Errorf("invoice total = %d, want 49355", inv.Total)
				}
				// Verify exactly 5 lines
				if len(inv.Lines) != 5 {
					t.Errorf("invoice line count = %d, want 5", len(inv.Lines))
				}
				// Verify prorate.charge line for business-year with end 2025-02-05
				var foundYearlyCharge bool
				for _, line := range inv.Lines {
					if line.RuleID == "prorate.charge" && line.Amount == 48000 {
						if line.Description != "business-year: full period 2024-02-05 to 2025-02-05" {
							t.Errorf("prorate.charge description = %q, want %q", line.Description, "business-year: full period 2024-02-05 to 2025-02-05")
						}
						foundYearlyCharge = true
					}
				}
				if !foundYearlyCharge {
					t.Errorf("prorate.charge line not found for business-year with end 2025-02-05")
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
			name: "big-credit-many-charges-invariant",
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
					PlanID: "business-year",
				},
				{
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "team-month",
				},
				{
					At:     time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify total is 49709
				if inv.Total != 49709 {
					t.Errorf("invoice total = %d, want 49709", inv.Total)
				}
				// Verify sum of |credit.applied| == 48000 (all banked credit used)
				var creditAppliedSum Money
				creditAppliedCount := 0
				for _, line := range inv.Lines {
					if line.RuleID == "credit.applied" {
						creditAppliedSum += -line.Amount
						creditAppliedCount++
					}
				}
				if creditAppliedSum != 48000 {
					t.Errorf("sum of |credit.applied| = %d, want 48000", creditAppliedSum)
				}
				// Verify exactly 3 credit.applied lines
				if creditAppliedCount != 3 {
					t.Errorf("credit.applied line count = %d, want 3", creditAppliedCount)
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
