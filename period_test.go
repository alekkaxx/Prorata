package prorata

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestAddInterval(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		iv   Interval
		want time.Time
	}{
		{"plain month", date(2026, time.January, 1), IntervalMonth, date(2026, time.February, 1)},
		{"jan 31 clamps to feb 28", date(2026, time.January, 31), IntervalMonth, date(2026, time.February, 28)},
		{"jan 31 clamps to feb 29 in leap year", date(2024, time.January, 31), IntervalMonth, date(2024, time.February, 29)},
		{"mar 31 clamps to apr 30", date(2026, time.March, 31), IntervalMonth, date(2026, time.April, 30)},
		{"feb 29 plus year clamps to feb 28", date(2024, time.February, 29), IntervalYear, date(2025, time.February, 28)},
		{"plain year", date(2026, time.January, 13), IntervalYear, date(2027, time.January, 13)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddInterval(tt.from, tt.iv)
			if err != nil {
				t.Fatalf("AddInterval error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("AddInterval(%s, %s) = %s, want %s", tt.from, tt.iv, got, tt.want)
			}
		})
	}
}

func TestAddIntervalUnknown(t *testing.T) {
	if _, err := AddInterval(date(2026, time.January, 1), Interval("week")); err == nil {
		t.Fatal("expected error for unknown interval")
	}
}

func TestPeriodDays(t *testing.T) {
	tests := []struct {
		name string
		p    Period
		want int
	}{
		{"january", Period{date(2026, time.January, 1), date(2026, time.February, 1)}, 31},
		{"february leap", Period{date(2024, time.February, 1), date(2024, time.March, 1)}, 29},
		{"year", Period{date(2026, time.January, 1), date(2027, time.January, 1)}, 365},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Days(); got != tt.want {
				t.Fatalf("Days() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPeriodContains(t *testing.T) {
	p := Period{date(2026, time.January, 1), date(2026, time.February, 1)}
	if !p.Contains(p.Start) {
		t.Fatal("period must contain its start")
	}
	if p.Contains(p.End) {
		t.Fatal("period must not contain its end (half-open)")
	}
}
