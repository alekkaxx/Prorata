package prorata

import (
	"errors"
	"fmt"
	"sort"
)

// Money is an amount in minor currency units (e.g. cents).
// All arithmetic is integer-based; floats never enter the API.
type Money int64

// Allocate splits m proportionally to weights using the largest remainder
// method: every part gets floor(m*w/W), then leftover cents go to the parts
// with the largest fractional remainders (ties resolved by lower index).
// The sum of the returned parts is always exactly m; a zero weight always
// yields a zero part. Negative amounts are allocated by absolute value and
// negated.
//
// Intermediate products m*w must fit in int64, which holds for any realistic
// subscription amount (see specs/00-core.md).
func (m Money) Allocate(weights []int64) ([]Money, error) {
	if len(weights) == 0 {
		return nil, errors.New("prorata: allocate: no weights")
	}
	var total int64
	for _, w := range weights {
		if w < 0 {
			return nil, fmt.Errorf("prorata: allocate: negative weight %d", w)
		}
		total += w
	}
	if total == 0 {
		return nil, errors.New("prorata: allocate: zero total weight")
	}

	abs := int64(m)
	neg := abs < 0
	if neg {
		abs = -abs
	}

	parts := make([]Money, len(weights))
	remainders := make([]int64, len(weights))
	var assigned int64
	for i, w := range weights {
		share := abs * w / total
		parts[i] = Money(share)
		remainders[i] = abs * w % total
		assigned += share
	}

	order := make([]int, len(weights))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return remainders[order[a]] > remainders[order[b]]
	})
	for i := int64(0); i < abs-assigned; i++ {
		parts[order[i]]++
	}

	if neg {
		for i := range parts {
			parts[i] = -parts[i]
		}
	}
	return parts, nil
}

// Percent returns bps basis points of m (10000 bps == 100%), rounded
// half away from zero.
func (m Money) Percent(bps int64) Money {
	num := int64(m) * bps
	neg := num < 0
	if neg {
		num = -num
	}
	q := num / 10000
	if num%10000*2 >= 10000 {
		q++
	}
	if neg {
		q = -q
	}
	return Money(q)
}

// validateBps checks that a basis-point rate fits the [0,10000] scale shared
// by percentage promos and VAT, reporting the error under the named
// operation.
func validateBps(bps int64, op string) error {
	if bps < 0 || bps > 10000 {
		return fmt.Errorf("prorata: %s: bps %d out of range [0,10000]", op, bps)
	}
	return nil
}

// formatPercent renders basis points as a human percentage for promo line
// descriptions: whole percents print without a fraction ("20"), sub-percent
// precision prints two decimals ("7.50"). Only non-negative bps occur (promos
// validate their range before arming), so no sign handling is needed.
func formatPercent(bps int64) string {
	if bps%100 == 0 {
		return fmt.Sprintf("%d", bps/100)
	}
	return fmt.Sprintf("%d.%02d", bps/100, bps%100)
}

// String formats the amount with two decimal places, e.g. "12.34" or "-0.05".
// Two minor units per major unit is assumed, which holds for the supported
// currencies.
func (m Money) String() string {
	v := int64(m)
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}
