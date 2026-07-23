package prorata

import (
	"testing"
	"time"
)

// TestDowngrade tests the downgrade-credit rule behavior through public Compute.
func TestDowngrade(t *testing.T) {
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
			name: "downgrade-without-subscription",
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
					At:     time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: downgrade: no active subscription",
		},
		{
			name: "downgrade-same-plan",
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
					At:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: downgrade: already on plan \"pro-month\"",
		},
		{
			name: "downgrade-unknown-plan",
			catalog: Catalog{
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
					At:     time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "ghost",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: downgrade: unknown plan \"ghost\"",
		},
		{
			name: "bank-does-not-go-negative",
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
				// Total must not be negative
				if inv.Total < 0 {
					t.Errorf("invoice total = %d, want >= 0", inv.Total)
				}
				// No line except credit.applied should have negative amount
				negativeCount := 0
				for _, line := range inv.Lines {
					if line.Amount < 0 && line.RuleID != "credit.applied" {
						negativeCount++
					}
				}
				if negativeCount > 0 {
					t.Errorf("found %d negative lines (excluding credit.applied), want 0", negativeCount)
				}
				// Sum of all amounts must equal total
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
			name: "first-day-banks-full-payment",
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
					At:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "team-month",
				},
				{
					At:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify charge.full team-month exists with +5000
				var foundChargeFullTeam bool
				var sumCreditApplied Money
				for _, line := range inv.Lines {
					if line.RuleID == "charge.full" && line.Description == "team-month: full period 2026-03-01 to 2026-04-01" {
						if line.Amount != 5000 {
							t.Errorf("charge.full team-month = %d, want 5000", line.Amount)
						}
						foundChargeFullTeam = true
					}
					if line.RuleID == "credit.applied" {
						sumCreditApplied += line.Amount // negative value
					}
				}
				if !foundChargeFullTeam {
					t.Errorf("charge.full team-month line not found")
				}
				// Sum of credit.applied amounts (absolute) should be <= 5000
				absCreditSum := -sumCreditApplied
				if absCreditSum > 5000 {
					t.Errorf("sum of |credit.applied| = %d, want <= 5000", absCreditSum)
				}
			},
		},
		{
			name: "bank-within-paid-in-chain",
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
					At:     time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify total is 2030
				if inv.Total != 2030 {
					t.Errorf("invoice total = %d, want 2030", inv.Total)
				}
				// Sum all credit.applied amounts (absolute)
				var sumCreditApplied Money
				for _, line := range inv.Lines {
					if line.RuleID == "credit.applied" {
						sumCreditApplied += -line.Amount // convert to positive
					}
				}
				// Should be 46422 (2000 + 44422)
				if sumCreditApplied != 46422 {
					t.Errorf("sum of |credit.applied| = %d, want 46422", sumCreditApplied)
				}
				// Sum of line amounts should equal total (invariant)
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
			name: "expiry-banks-zero",
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
					At:     time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
					Type:   "downgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Should have exactly one line: downgrade.charge
				if len(inv.Lines) != 1 {
					t.Errorf("invoice line count = %d, want 1", len(inv.Lines))
				}
				// That line should be downgrade.charge with 2000
				if len(inv.Lines) > 0 {
					if inv.Lines[0].RuleID != "downgrade.charge" {
						t.Errorf("first line RuleID = %q, want downgrade.charge", inv.Lines[0].RuleID)
					}
					if inv.Lines[0].Amount != 2000 {
						t.Errorf("downgrade.charge amount = %d, want 2000", inv.Lines[0].Amount)
					}
				}
				// No credit.applied line should exist
				for _, line := range inv.Lines {
					if line.RuleID == "credit.applied" {
						t.Errorf("found credit.applied line, want none")
					}
				}
				// Total should be 2000
				if inv.Total != 2000 {
					t.Errorf("invoice total = %d, want 2000", inv.Total)
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
