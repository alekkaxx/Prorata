package prorata

import (
	"fmt"
	"time"
)

// Period is a half-open time interval [Start, End). Half-openness guarantees
// that the end of one billing period equals the start of the next and no day
// is ever counted twice.
type Period struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Days returns the number of whole 24-hour days contained in the period.
// Days is the proration granularity of the engine (see specs/00-core.md).
func (p Period) Days() int {
	return int(p.End.Sub(p.Start) / (24 * time.Hour))
}

// Contains reports whether t falls within the half-open interval [Start, End).
func (p Period) Contains(t time.Time) bool {
	return !t.Before(p.Start) && t.Before(p.End)
}

// validate reports whether the period is well-formed.
func (p Period) validate() error {
	if !p.Start.Before(p.End) {
		return fmt.Errorf("prorata: period start %s is not before end %s", p.Start, p.End)
	}
	return nil
}

// formatDay renders a timestamp as its UTC calendar day, the date granularity
// every invoice line description uses.
func formatDay(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// unusedShare splits base between the used and unused days of period p as of
// the moment at, using the same largest-remainder Allocate as every other
// money split, and returns the unused part along with the day counts (rem
// unused days of total). rem is clamped at zero so a split after the period
// has ended yields a zero unused share rather than an error. The used and
// unused parts always sum to exactly base, so the unused share can never
// exceed it — the mechanical form of the "credit <= actually paid" invariant
// shared by proration, downgrade banking and refunds.
func unusedShare(base Money, p Period, at time.Time) (unused Money, rem, total int, err error) {
	total = p.Days()
	rem = max(Period{Start: at, End: p.End}.Days(), 0)
	used := total - rem
	parts, err := base.Allocate([]int64{int64(used), int64(rem)})
	if err != nil {
		return 0, 0, 0, err
	}
	return parts[1], rem, total, nil
}

// AddInterval returns t advanced by one billing interval. Months are calendar
// months anchored to the day of t, with the day clamped to the last day of
// the target month (Jan 31 + month = Feb 28/29). A year is twelve months.
func AddInterval(t time.Time, iv Interval) (time.Time, error) {
	switch iv {
	case IntervalMonth:
		return addMonthsClamped(t, 1), nil
	case IntervalYear:
		return addMonthsClamped(t, 12), nil
	default:
		return time.Time{}, fmt.Errorf("prorata: unknown interval %q", iv)
	}
}

// addMonthsClamped adds n calendar months to t, clamping the day of month to
// the last day of the target month instead of letting it overflow the way
// time.AddDate does.
func addMonthsClamped(t time.Time, n int) time.Time {
	y, mo, d := t.Date()
	h, min, sec := t.Clock()
	first := time.Date(y, mo+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	if last := daysIn(first.Year(), first.Month()); d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, h, min, sec, t.Nanosecond(), t.Location())
}

// daysIn returns the number of days in the given month.
func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
