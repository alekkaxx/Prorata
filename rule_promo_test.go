package prorata

import (
	"testing"
	"time"
)

// TestPromo tests the promo percentage discount rule behavior through public Compute.
func TestPromo(t *testing.T) {
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
			name: "empty-code",
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
					Code: "",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: empty code",
		},
		{
			name: "bps-out-of-range",
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
					Bps:  10001,
					Code: "X",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: bps 10001 out of range [0,10000]",
		},
		{
			name: "promo-already-pending",
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
					Code: "FIRST",
				},
				{
					At:   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					Type: "apply_promo",
					Bps:  1000,
					Code: "SECOND",
				},
			},
			period: Period{
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: promo: promo already pending",
		},
		{
			name: "first-charge-with-promo",
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
					Code: "WELCOME20",
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
				// Verify promo.percent line exists with -400
				var foundPromoLine bool
				var foundChargeFullLine bool
				for _, line := range inv.Lines {
					if line.RuleID == "promo.percent" {
						if line.Amount != -400 {
							t.Errorf("promo.percent amount = %d, want -400", line.Amount)
						}
						foundPromoLine = true
					}
					if line.RuleID == "charge.full" {
						if line.Amount != 2000 {
							t.Errorf("charge.full amount = %d, want 2000", line.Amount)
						}
						foundChargeFullLine = true
					}
				}
				if !foundPromoLine {
					t.Errorf("promo.percent line not found")
				}
				if !foundChargeFullLine {
					t.Errorf("charge.full line not found")
				}
				// Verify total equals sum of amounts (invariant)
				var sum Money
				for _, line := range inv.Lines {
					sum += line.Amount
				}
				if sum != inv.Total {
					t.Errorf("sum of line amounts = %d, total = %d, mismatch", sum, inv.Total)
				}
				if inv.Total != 1600 {
					t.Errorf("invoice total = %d, want 1600", inv.Total)
				}
				// Verify |discount| < base
				if 400 >= 2000 {
					t.Errorf("discount 400 >= base 2000")
				}
			},
		},
		{
			name: "promo-before-credit-balance",
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
					At:   time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type: "apply_promo",
					Bps:  5000,
					Code: "HALF",
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
				// Verify credit.applied is -1000 (by discounted net)
				var creditApplied Money
				for _, line := range inv.Lines {
					if line.RuleID == "credit.applied" {
						creditApplied += line.Amount
					}
				}
				if creditApplied != -1000 {
					t.Errorf("credit.applied = %d, want -1000", creditApplied)
				}
				// Verify total is 0
				if inv.Total != 0 {
					t.Errorf("invoice total = %d, want 0", inv.Total)
				}
				// Verify no line is negative except promo.percent and credit.applied
				for _, line := range inv.Lines {
					if line.Amount < 0 && line.RuleID != "promo.percent" && line.RuleID != "credit.applied" {
						t.Errorf("found negative line %q with amount %d, want only promo.percent and credit.applied", line.RuleID, line.Amount)
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
			name: "promo-no-discount-credit-remainder",
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
					At:   time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type: "apply_promo",
					Bps:  2000,
					Code: "WELCOME20",
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
				// Verify promo.percent is -9600
				var promoPercent Money
				for _, line := range inv.Lines {
					if line.RuleID == "promo.percent" {
						promoPercent = line.Amount
					}
				}
				if promoPercent != -9600 {
					t.Errorf("promo.percent = %d, want -9600", promoPercent)
				}
				// Verify total is 37174
				if inv.Total != 37174 {
					t.Errorf("invoice total = %d, want 37174", inv.Total)
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
			name: "sum-invariant",
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
					At:   time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
					Type: "apply_promo",
					Bps:  2000,
					Code: "WELCOME20",
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
				// Sum of all line amounts must equal total (money invariant)
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
			name: "promo-hundred-percent",
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
					Bps:  10000,
					Code: "FREEMONTH",
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
				// Verify promo.percent is -2000 (equals base)
				var promoPercent Money
				for _, line := range inv.Lines {
					if line.RuleID == "promo.percent" {
						promoPercent = line.Amount
					}
				}
				if promoPercent != -2000 {
					t.Errorf("promo.percent = %d, want -2000", promoPercent)
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
