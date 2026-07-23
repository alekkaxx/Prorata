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

// TestProrateCreditNeverExceedsPaid verifies the upgrade invariant from
// specs/02-prorate-upgrade.md D1 on arbitrary upgrade dates: for any
// subscribe-then-upgrade history, the credited remainder of the old plan is
// never positive and its magnitude never exceeds what was actually paid for
// the old plan, and the invoice total always equals the sum of its lines.
func TestProrateCreditNeverExceedsPaid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		oldPrice := Money(rapid.Int64Range(0, 10_000_00).Draw(t, "oldPrice"))
		newPrice := Money(rapid.Int64Range(0, 10_000_00).Draw(t, "newPrice"))
		oldIv := IntervalMonth
		if rapid.Bool().Draw(t, "oldYearly") {
			oldIv = IntervalYear
		}
		newIv := IntervalMonth
		if rapid.Bool().Draw(t, "newYearly") {
			newIv = IntervalYear
		}
		catalog := Catalog{
			"old": {ID: "old", Price: oldPrice, Interval: oldIv, Currency: "USD"},
			"new": {ID: "new", Price: newPrice, Interval: newIv, Currency: "USD"},
		}

		subscribeAt := time.Date(
			rapid.IntRange(2020, 2030).Draw(t, "year"),
			time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"),
			0, 0, 0, 0, time.UTC,
		)
		// The upgrade lands a whole number of days at or after subscribe,
		// including well past the old period's end where the remainder clamps
		// to zero and the credit vanishes.
		upgradeAt := subscribeAt.AddDate(0, 0, rapid.IntRange(0, 800).Draw(t, "offsetDays"))

		events := []Event{
			{At: subscribeAt, Type: EventSubscribe, PlanID: "old"},
			{At: upgradeAt, Type: EventUpgrade, PlanID: "new"},
		}
		period := Period{
			Start: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		inv, err := Compute(catalog, events, period)
		if err != nil {
			t.Fatalf("Compute error: %v", err)
		}

		var credit Money
		found := false
		for _, ln := range inv.Lines {
			if ln.RuleID == ruleProrateCredit {
				credit = ln.Amount
				found = true
			}
		}
		if !found {
			t.Fatalf("no prorate.credit line in invoice")
		}
		if credit > 0 {
			t.Fatalf("credit %d is positive, want <= 0", credit)
		}
		if -credit > oldPrice {
			t.Fatalf("credit magnitude %d exceeds paid %d", -credit, oldPrice)
		}

		var sum Money
		for _, ln := range inv.Lines {
			sum += ln.Amount
		}
		if sum != inv.Total {
			t.Fatalf("sum of lines %d != invoice total %d", sum, inv.Total)
		}
	})
}

// TestCreditLedgerNeverOverdraws verifies the credit-ledger invariant from
// specs/03-downgrade-credit.md on arbitrary chains of plan changes: for any
// subscribe followed by random upgrades and downgrades, the credit balance
// behaves like a real ledger. Because a downgrade only banks a share of what
// was actually paid and the engine only ever applies min(net, balance), the
// balance can never go negative and can never hand out more credit than was
// banked. Both are unobservable directly, so the test asserts their strongest
// public consequence: the sum of all credit.applied magnitudes never exceeds
// the sum of the positive charge lines (each application is capped by its own
// event's net charge, so it is capped by paid), every credit.applied line is
// strictly negative, and the invoice total equals the sum of its lines.
func TestCreditLedgerNeverOverdraws(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		catalog := Catalog{
			"a": {ID: "a", Price: Money(rapid.Int64Range(0, 10_000_00).Draw(t, "priceA")), Interval: IntervalMonth, Currency: "USD"},
			"b": {ID: "b", Price: Money(rapid.Int64Range(0, 10_000_00).Draw(t, "priceB")), Interval: IntervalMonth, Currency: "USD"},
			"c": {ID: "c", Price: Money(rapid.Int64Range(0, 10_000_00).Draw(t, "priceC")), Interval: IntervalYear, Currency: "USD"},
		}
		ids := []string{"a", "b", "c"}

		at := time.Date(
			rapid.IntRange(2020, 2030).Draw(t, "year"),
			time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"),
			0, 0, 0, 0, time.UTC,
		)
		cur := ids[rapid.IntRange(0, 2).Draw(t, "firstPlan")]
		events := []Event{{At: at, Type: EventSubscribe, PlanID: cur}}

		steps := rapid.IntRange(1, 6).Draw(t, "steps")
		for range steps {
			// Draw a plan different from the current one to avoid the
			// same-plan error; the rule does not compare prices, so upgrade
			// and downgrade are both legal to either plan.
			next := ids[rapid.IntRange(0, 1).Draw(t, "planPick")]
			if next == cur {
				next = ids[2]
			}
			typ := EventUpgrade
			if rapid.Bool().Draw(t, "isDowngrade") {
				typ = EventDowngrade
			}
			at = at.AddDate(0, 0, rapid.IntRange(0, 400).Draw(t, "gapDays"))
			events = append(events, Event{At: at, Type: typ, PlanID: next})
			cur = next
		}

		period := Period{
			Start: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		inv, err := Compute(catalog, events, period)
		if err != nil {
			t.Fatalf("Compute error: %v", err)
		}

		var lineSum, positiveCharges, appliedMagnitude Money
		for _, ln := range inv.Lines {
			lineSum += ln.Amount
			if ln.Amount > 0 {
				positiveCharges += ln.Amount
			}
			if ln.RuleID == ruleCreditApplied {
				if ln.Amount >= 0 {
					t.Fatalf("credit.applied amount %d is not negative", ln.Amount)
				}
				appliedMagnitude += -ln.Amount
			}
		}
		if lineSum != inv.Total {
			t.Fatalf("sum of lines %d != invoice total %d", lineSum, inv.Total)
		}
		if appliedMagnitude > positiveCharges {
			t.Fatalf("applied credit %d exceeds positive charges %d (ledger overdrawn)", appliedMagnitude, positiveCharges)
		}
	})
}

// TestPromoOneShotAndBounded verifies the promo invariants from
// specs/04-promo-percent.md on arbitrary discount rates and plan-change chains:
// a single promo armed before subscribe is consumed by the first positive
// charge and never again (one-shot), the discount never creates money, and the
// invoice stays balanced. Because prices are strictly positive the subscribe
// charge is always positive, so the promo is always consumed exactly once no
// matter how long the subsequent chain is. The test asserts the strongest
// public consequences: exactly one promo.percent line exists (D3 burn, no
// repeat), it is never positive, its magnitude never exceeds the sum of all
// positive charges (D2/D4: a discount cannot exceed what is charged), and the
// invoice total equals the sum of its lines.
func TestPromoOneShotAndBounded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		catalog := Catalog{
			"a": {ID: "a", Price: Money(rapid.Int64Range(1, 10_000_00).Draw(t, "priceA")), Interval: IntervalMonth, Currency: "USD"},
			"b": {ID: "b", Price: Money(rapid.Int64Range(1, 10_000_00).Draw(t, "priceB")), Interval: IntervalMonth, Currency: "USD"},
			"c": {ID: "c", Price: Money(rapid.Int64Range(1, 10_000_00).Draw(t, "priceC")), Interval: IntervalYear, Currency: "USD"},
		}
		ids := []string{"a", "b", "c"}

		at := time.Date(
			rapid.IntRange(2020, 2030).Draw(t, "year"),
			time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"),
			0, 0, 0, 0, time.UTC,
		)
		bps := rapid.Int64Range(0, 10000).Draw(t, "bps")
		cur := ids[rapid.IntRange(0, 2).Draw(t, "firstPlan")]

		// The promo is armed before subscribe, so it discounts the first charge
		// (subscribe itself), then must never fire again over the chain.
		events := []Event{
			{At: at, Type: EventApplyPromo, Bps: bps, Code: "PROMO"},
			{At: at, Type: EventSubscribe, PlanID: cur},
		}

		steps := rapid.IntRange(0, 6).Draw(t, "steps")
		for range steps {
			next := ids[rapid.IntRange(0, 1).Draw(t, "planPick")]
			if next == cur {
				next = ids[2]
			}
			typ := EventUpgrade
			if rapid.Bool().Draw(t, "isDowngrade") {
				typ = EventDowngrade
			}
			at = at.AddDate(0, 0, rapid.IntRange(0, 400).Draw(t, "gapDays"))
			events = append(events, Event{At: at, Type: typ, PlanID: next})
			cur = next
		}

		period := Period{
			Start: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		inv, err := Compute(catalog, events, period)
		if err != nil {
			t.Fatalf("Compute error: %v", err)
		}

		var lineSum, positiveCharges, discountMagnitude Money
		promoLines := 0
		for _, ln := range inv.Lines {
			lineSum += ln.Amount
			if ln.Amount > 0 {
				positiveCharges += ln.Amount
			}
			if ln.RuleID == rulePromoPercent {
				promoLines++
				if ln.Amount > 0 {
					t.Fatalf("promo.percent amount %d is positive, want <= 0", ln.Amount)
				}
				discountMagnitude += -ln.Amount
			}
		}
		if promoLines != 1 {
			t.Fatalf("got %d promo.percent lines, want exactly 1 (one-shot)", promoLines)
		}
		if discountMagnitude > positiveCharges {
			t.Fatalf("discount %d exceeds positive charges %d (money created)", discountMagnitude, positiveCharges)
		}
		if lineSum != inv.Total {
			t.Fatalf("sum of lines %d != invoice total %d", lineSum, inv.Total)
		}
	})
}

// TestPromoFixedOneShotAndBounded verifies the fixed-promo invariants from
// specs/05-promo-fixed.md on arbitrary flat amounts and plan-change chains. The
// amount is drawn wide enough to span both regimes — below the first charge
// (no clamp) and far above it (clamp min(amount, base)) — so the clamp path is
// exercised. As in the percent case the promo is armed before subscribe and the
// subscribe charge is strictly positive, so it is consumed exactly once. The
// test asserts the strongest public consequences: exactly one promo.fixed line
// exists (D3 burn, no repeat), it is never positive, its magnitude never exceeds
// the sum of all positive charges (D2/D4 clamp: a fixed discount cannot exceed
// what is charged and cannot create money), and the invoice total equals the
// sum of its lines.
func TestPromoFixedOneShotAndBounded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		catalog := Catalog{
			"a": {ID: "a", Price: Money(rapid.Int64Range(1, 10_000_00).Draw(t, "priceA")), Interval: IntervalMonth, Currency: "USD"},
			"b": {ID: "b", Price: Money(rapid.Int64Range(1, 10_000_00).Draw(t, "priceB")), Interval: IntervalMonth, Currency: "USD"},
			"c": {ID: "c", Price: Money(rapid.Int64Range(1, 10_000_00).Draw(t, "priceC")), Interval: IntervalYear, Currency: "USD"},
		}
		ids := []string{"a", "b", "c"}

		at := time.Date(
			rapid.IntRange(2020, 2030).Draw(t, "year"),
			time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"),
			0, 0, 0, 0, time.UTC,
		)
		amount := rapid.Int64Range(1, 100_000_00).Draw(t, "amount")
		cur := ids[rapid.IntRange(0, 2).Draw(t, "firstPlan")]

		// The promo is armed before subscribe, so it discounts the first charge
		// (subscribe itself), then must never fire again over the chain.
		events := []Event{
			{At: at, Type: EventApplyPromo, AmountCents: Money(amount), Code: "PROMO"},
			{At: at, Type: EventSubscribe, PlanID: cur},
		}

		steps := rapid.IntRange(0, 6).Draw(t, "steps")
		for range steps {
			next := ids[rapid.IntRange(0, 1).Draw(t, "planPick")]
			if next == cur {
				next = ids[2]
			}
			typ := EventUpgrade
			if rapid.Bool().Draw(t, "isDowngrade") {
				typ = EventDowngrade
			}
			at = at.AddDate(0, 0, rapid.IntRange(0, 400).Draw(t, "gapDays"))
			events = append(events, Event{At: at, Type: typ, PlanID: next})
			cur = next
		}

		period := Period{
			Start: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		inv, err := Compute(catalog, events, period)
		if err != nil {
			t.Fatalf("Compute error: %v", err)
		}

		var lineSum, positiveCharges, discountMagnitude Money
		promoLines := 0
		for _, ln := range inv.Lines {
			lineSum += ln.Amount
			if ln.Amount > 0 {
				positiveCharges += ln.Amount
			}
			if ln.RuleID == rulePromoFixed {
				promoLines++
				if ln.Amount > 0 {
					t.Fatalf("promo.fixed amount %d is positive, want <= 0", ln.Amount)
				}
				discountMagnitude += -ln.Amount
			}
		}
		if promoLines != 1 {
			t.Fatalf("got %d promo.fixed lines, want exactly 1 (one-shot)", promoLines)
		}
		if discountMagnitude > positiveCharges {
			t.Fatalf("discount %d exceeds positive charges %d (money created)", discountMagnitude, positiveCharges)
		}
		if lineSum != inv.Total {
			t.Fatalf("sum of lines %d != invoice total %d", lineSum, inv.Total)
		}
	})
}

// TestTrialChainNeverOverdraws verifies the trial invariants from
// specs/07-trial.md on arbitrary trial->convert->plan-change chains: a free
// trial followed by a conversion and then random upgrades and downgrades keeps
// the ledger honest. Because the trial is free (paid == 0) and D5 forces a clean
// state, the trial banks no credit; the credit ledger can only ever be fed by
// the paid periods that follow conversion, so it can never hand out more than
// what was actually paid. The test asserts the strongest public consequences:
// exactly one trial.start line exists (the trial opens once), it is always zero
// (nothing is charged for the trial), every credit.applied line is negative, the
// sum of all credit.applied magnitudes never exceeds the sum of the positive
// charges (the ledger never overdraws), and the invoice total equals the sum of
// its lines.
func TestTrialChainNeverOverdraws(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		catalog := Catalog{
			"a": {ID: "a", Price: Money(rapid.Int64Range(0, 10_000_00).Draw(t, "priceA")), Interval: IntervalMonth, Currency: "USD"},
			"b": {ID: "b", Price: Money(rapid.Int64Range(0, 10_000_00).Draw(t, "priceB")), Interval: IntervalMonth, Currency: "USD"},
			"c": {ID: "c", Price: Money(rapid.Int64Range(0, 10_000_00).Draw(t, "priceC")), Interval: IntervalYear, Currency: "USD"},
		}
		ids := []string{"a", "b", "c"}

		at := time.Date(
			rapid.IntRange(2020, 2030).Draw(t, "year"),
			time.Month(rapid.IntRange(1, 12).Draw(t, "month")),
			rapid.IntRange(1, 28).Draw(t, "day"),
			0, 0, 0, 0, time.UTC,
		)
		// A trial opens on a clean state (D5); it is later converted to the same
		// or a different plan (D8). Conversion may land mid-trial or past its end
		// (D4) — the offset spans both regimes.
		trialPlan := ids[rapid.IntRange(0, 2).Draw(t, "trialPlan")]
		events := []Event{{At: at, Type: EventTrial, PlanID: trialPlan}}

		at = at.AddDate(0, 0, rapid.IntRange(0, 400).Draw(t, "convertGap"))
		cur := ids[rapid.IntRange(0, 2).Draw(t, "convertPlan")]
		events = append(events, Event{At: at, Type: EventConvert, PlanID: cur})

		steps := rapid.IntRange(0, 6).Draw(t, "steps")
		for range steps {
			// Draw a plan different from the current one to avoid the same-plan
			// error; upgrade and downgrade are both legal to either plan.
			next := ids[rapid.IntRange(0, 1).Draw(t, "planPick")]
			if next == cur {
				next = ids[2]
			}
			typ := EventUpgrade
			if rapid.Bool().Draw(t, "isDowngrade") {
				typ = EventDowngrade
			}
			at = at.AddDate(0, 0, rapid.IntRange(0, 400).Draw(t, "gapDays"))
			events = append(events, Event{At: at, Type: typ, PlanID: next})
			cur = next
		}

		period := Period{
			Start: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		inv, err := Compute(catalog, events, period)
		if err != nil {
			t.Fatalf("Compute error: %v", err)
		}

		var lineSum, positiveCharges, appliedMagnitude Money
		trialLines := 0
		for _, ln := range inv.Lines {
			lineSum += ln.Amount
			if ln.Amount > 0 {
				positiveCharges += ln.Amount
			}
			if ln.RuleID == ruleTrialStart {
				trialLines++
				if ln.Amount != 0 {
					t.Fatalf("trial.start amount %d is not zero", ln.Amount)
				}
			}
			if ln.RuleID == ruleCreditApplied {
				if ln.Amount >= 0 {
					t.Fatalf("credit.applied amount %d is not negative", ln.Amount)
				}
				appliedMagnitude += -ln.Amount
			}
		}
		if trialLines != 1 {
			t.Fatalf("got %d trial.start lines, want exactly 1", trialLines)
		}
		if appliedMagnitude > positiveCharges {
			t.Fatalf("applied credit %d exceeds positive charges %d (ledger overdrawn)", appliedMagnitude, positiveCharges)
		}
		if lineSum != inv.Total {
			t.Fatalf("sum of lines %d != invoice total %d", lineSum, inv.Total)
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
