package prorata

import (
	"testing"
	"time"
)

// TestGrace tests the three grace-period rule handlers (payment_failed,
// grace_recover, grace_expire) behavior through public Compute.
func TestGrace(t *testing.T) {
	tests := []struct {
		name    string
		catalog Catalog
		events  []Event
		period  Period
		wantErr bool
		errMsg  string
	}{
		{
			name: "payment-failed-no-subscription",
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
					Type:   "payment_failed",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: grace: no active subscription",
		},
		{
			name: "payment-failed-on-trial",
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
					Type:   "trial_start",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
					Type:   "payment_failed",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: grace: cannot fail payment on a trial",
		},
		{
			name: "double-payment-failed",
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
					At:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
					Type:   "payment_failed",
					PlanID: "pro-month",
				},
				{
					At:     time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC),
					Type:   "payment_failed",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: grace: already in grace",
		},
		{
			name: "grace-recover-without-grace",
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
					At:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
					Type:   "grace_recover",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: grace: no grace in progress",
		},
		{
			name: "grace-expire-without-grace-empty-state",
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
					Type:   "grace_expire",
					PlanID: "pro-month",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  "prorata: grace: no grace in progress",
		},
		{
			name: "payment-failed-unknown-plan",
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
					At:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
					Type:   "payment_failed",
					PlanID: "ghost",
				},
			},
			period: Period{
				Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErr: true,
			errMsg:  `prorata: grace: unknown plan "ghost"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compute(tt.catalog, tt.events, tt.period)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("Compute() error message = %q, want %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}
