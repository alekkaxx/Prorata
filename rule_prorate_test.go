package prorata

import (
	"testing"
	"time"
)

// TestProrate tests the upgrade-proration rule behavior through public Compute.
func TestProrate(t *testing.T) {
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
			name: "upgrade-without-subscription",
			catalog: Catalog{
				"business-year": {
					ID:       "business-year",
					Price:    48000,
					Interval: "year",
					Currency: "USD",
				},
			},
			events: []Event{
				{
					At:     time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: upgrade: no active subscription",
		},
		{
			name: "upgrade-same-plan",
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
					Type:   "upgrade",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: upgrade: already on plan \"pro-month\"",
		},
		{
			name: "upgrade-unknown-plan",
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
					Type:   "upgrade",
					PlanID: "ghost",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: upgrade: unknown plan \"ghost\"",
		},
		{
			name: "upgrade-first-day-full-credit",
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
					At:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Find prorate.credit line to verify full credit
				for _, line := range inv.Lines {
					if line.RuleID == "prorate.credit" {
						if line.Amount != -2000 {
							t.Errorf("prorate.credit amount = %d, want -2000", line.Amount)
						}
						return
					}
				}
				t.Errorf("prorate.credit line not found")
			},
		},
		{
			name: "upgrade-last-day-partial-credit",
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
					At:     time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Find prorate.credit line and verify it's less than paid
				for _, line := range inv.Lines {
					if line.RuleID == "prorate.credit" {
						absCredit := -line.Amount // Convert to positive for comparison
						if absCredit >= 2000 {
							t.Errorf("prorate.credit absolute value = %d, want < 2000", absCredit)
						}
						if absCredit != 65 {
							t.Errorf("prorate.credit absolute value = %d, want 65", absCredit)
						}
						return
					}
				}
				t.Errorf("prorate.credit line not found")
			},
		},
		{
			name: "upgrade-invoice-sum-invariant",
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
					At:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
					Type:   "subscribe",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
					Type:   "upgrade",
					PlanID: "business-year",
				},
			},
			period: Period{
				Start: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: false,
			check: func(t *testing.T, inv *Invoice) {
				// Verify invoice total equals sum of line amounts
				if inv.Total != 48000 {
					t.Errorf("invoice total = %d, want 48000", inv.Total)
				}
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
