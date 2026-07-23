package prorata

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestAllocatePropertySumPreserved verifies the core money invariant: an
// allocation never creates or destroys a single cent, zero weights get zero
// parts, and every part deviates from the exact proportional share by less
// than one cent.
func TestAllocatePropertySumPreserved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := Money(rapid.Int64Range(-100_000_00, 100_000_00).Draw(t, "amount"))
		n := rapid.IntRange(1, 12).Draw(t, "parts")
		weights := make([]int64, n)
		var total int64
		for i := range weights {
			weights[i] = rapid.Int64Range(0, 366).Draw(t, "weight")
			total += weights[i]
		}
		if total == 0 {
			weights[0] = 1
			total = 1
		}

		parts, err := m.Allocate(weights)
		if err != nil {
			t.Fatalf("Allocate error: %v", err)
		}
		if len(parts) != n {
			t.Fatalf("got %d parts, want %d", len(parts), n)
		}

		var sum Money
		for i, p := range parts {
			sum += p
			if weights[i] == 0 && p != 0 {
				t.Fatalf("zero weight got non-zero part %d", p)
			}
			// |part*total - m*weight| < total  <=>  the part is within one
			// cent of the exact proportional share.
			diff := int64(p)*total - int64(m)*weights[i]
			if diff < 0 {
				diff = -diff
			}
			if diff >= total {
				t.Fatalf("part %d deviates from exact share by a full cent or more", i)
			}
		}
		if sum != m {
			t.Fatalf("allocation sum %d != amount %d", sum, m)
		}
	})
}

// TestPercentProperty verifies sign symmetry and boundedness of Percent.
func TestPercentProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := Money(rapid.Int64Range(-100_000_00, 100_000_00).Draw(t, "amount"))
		bps := rapid.Int64Range(0, 10000).Draw(t, "bps")

		got := m.Percent(bps)
		if neg := (-m).Percent(bps); neg != -got {
			t.Fatalf("Percent is not sign-symmetric: %d vs %d", neg, -got)
		}
		abs, gotAbs := m, got
		if abs < 0 {
			abs = -abs
		}
		if gotAbs < 0 {
			gotAbs = -gotAbs
		}
		if gotAbs > abs {
			t.Fatalf("|Percent(%d)| = %d exceeds |amount| = %d", bps, gotAbs, abs)
		}
	})
}

// TestAddIntervalProperty verifies the calendar-clamp decision from
// specs/00-core.md D3: the anchor day never grows, the result is always
// strictly later, and adding a month lands in the next calendar month.
func TestAddIntervalProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		start := time.Date(
			rapid.IntRange(2020, 2030).Draw(t, "year"),
			time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 31).Draw(t, "day"),
			0, 0, 0, 0, time.UTC,
		)
		iv := IntervalMonth
		if rapid.Bool().Draw(t, "yearly") {
			iv = IntervalYear
		}

		got, err := AddInterval(start, iv)
		if err != nil {
			t.Fatalf("AddInterval error: %v", err)
		}
		if !got.After(start) {
			t.Fatalf("AddInterval(%s) = %s is not after the start", start, got)
		}
		if got.Day() > start.Day() {
			t.Fatalf("clamped day %d exceeds anchor day %d", got.Day(), start.Day())
		}
		wantMonths := 1
		if iv == IntervalYear {
			wantMonths = 12
		}
		gotMonths := int(got.Month()) - int(start.Month()) + 12*(got.Year()-start.Year())
		if gotMonths != wantMonths {
			t.Fatalf("AddInterval(%s, %s) moved %d months, want %d", start, iv, gotMonths, wantMonths)
		}
	})
}
