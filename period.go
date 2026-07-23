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
