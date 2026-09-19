package engine

// Tests for the two-operand aggregations: progress, margin and difference.
//
// ── What these exist to prevent ──────────────────────────────────────────────
//
// TPL asked Spektr "what is my total profit across projects" and got
// ₹307,131.58. That is total SPEND; the real figure is ₹78,056.39. Nothing
// malfunctioned — the spec was well formed, the engine summed correctly, and
// the translator labelled the sum with the word the user typed. Three layers
// each did something defensible and the result was a confidently wrong number.
//
// The engine had no way to subtract, and no way to see a value that was not in
// a row. Progress fixes both, and these tests pin the boundaries where a
// plausible number could creep back in: a missing plan, a zero plan, a pace
// reading that flips on noise.

import (
	"math"
	"strings"
	"testing"
)

// ── fixtures ────────────────────────────────────────────────────────────────

// A cycling goal: 2,000 km over six months, 900 km ridden.
func cyclingView() RecordView {
	return NewSliceView([]Record{
		{Dimensions: map[string]string{"month": "Jan-2026", "activity": "ride"}, Measures: map[string]float64{"distance": 300}},
		{Dimensions: map[string]string{"month": "Feb-2026", "activity": "ride"}, Measures: map[string]float64{"distance": 250}},
		{Dimensions: map[string]string{"month": "Mar-2026", "activity": "ride"}, Measures: map[string]float64{"distance": 200}},
		{Dimensions: map[string]string{"month": "Apr-2026", "activity": "ride"}, Measures: map[string]float64{"distance": 150}},
		{Dimensions: map[string]string{"month": "Apr-2026", "activity": "walk"}, Measures: map[string]float64{"distance": 40}},
	})
}

func elapsed(f float64) *float64 { return &f }

func progressSpec() QuerySpec {
	return QuerySpec{
		Intent:      "text",
		Aggregation: "progress",
		Measure:     "distance",
		Minuend:     &Operand{Reference: "distance_goal"},
		Subtrahend:  &Operand{Filters: &Filters{Dimensions: map[string][]string{"activity": {"ride"}}}},
	}
}

func progressOf(t *testing.T, r *Result) *ProgressData {
	t.Helper()
	td, ok := r.Data.(*TextData)
	if !ok || td.Progress == nil {
		t.Fatalf("expected ProgressData, got %#v", r.Data)
	}
	return td.Progress
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// ── the plan and the actual ─────────────────────────────────────────────────

func TestProgress_AttainedAndRemaining(t *testing.T) {
	res, err := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d := progressOf(t, res)
	if !near(d.Plan, 2000) || !near(d.Actual, 900) || !near(d.Remaining, 1100) {
		t.Fatalf("plan/actual/remaining = %.0f/%.0f/%.0f", d.Plan, d.Actual, d.Remaining)
	}
	if !near(d.Attained, 45) {
		t.Errorf("attained = %.2f%%, want 45", d.Attained)
	}
	// The walk is filtered out; only rides count toward a cycling goal.
	if !near(d.Outstanding, 55) {
		t.Errorf("outstanding = %.2f%%, want 55", d.Outstanding)
	}
}

func TestProgress_ReportsBothEnds(t *testing.T) {
	// Attained and outstanding are complements and BOTH are reported, because
	// which end a user reads from is a property of the domain: fundraising asks
	// how far along, project work asks what is left. Choosing for them means
	// choosing wrongly for half.
	res, _ := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}}))

	d := progressOf(t, res)
	if !near(d.Attained+d.Outstanding, 100) {
		t.Fatalf("%.2f + %.2f should be 100", d.Attained, d.Outstanding)
	}
}

func TestProgress_Overrun(t *testing.T) {
	// Past the plan: remaining goes negative and attainment past 100. Clamping
	// either would hide an overrun, which is the one thing a user most needs to
	// be told.
	res, err := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 500}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d := progressOf(t, res)
	if d.Remaining >= 0 {
		t.Errorf("remaining = %.0f, want negative", d.Remaining)
	}
	if d.Attained <= 100 {
		t.Errorf("attained = %.1f%%, want over 100", d.Attained)
	}
}

// ── pace ────────────────────────────────────────────────────────────────────

func TestProgress_PaceBehind(t *testing.T) {
	// The Strava reading: 45% done with five of six months gone.
	res, _ := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{
			"distance_goal": {Value: 2000, Elapsed: elapsed(0.83)},
		}))

	d := progressOf(t, res)
	if d.Pace != "behind" {
		t.Errorf("pace = %q, want behind", d.Pace)
	}
	if d.Elapsed == nil || !near(*d.Elapsed, 83) {
		t.Errorf("elapsed = %v, want 83", d.Elapsed)
	}
	// At this rate the goal finishes around 1,084 km — short of 2,000, which is
	// the number that makes "behind" actionable rather than a label.
	if d.Projected == nil || *d.Projected >= 2000 {
		t.Errorf("projected = %v, want under the plan", d.Projected)
	}
}

func TestProgress_PaceAhead(t *testing.T) {
	res, _ := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{
			"distance_goal": {Value: 2000, Elapsed: elapsed(0.25)},
		}))

	d := progressOf(t, res)
	if d.Pace != "ahead" {
		t.Errorf("pace = %q, want ahead", d.Pace)
	}
}

func TestProgress_PaceToleranceBand(t *testing.T) {
	// 45% attained against 44% elapsed is "on track", not "ahead". Without a
	// tolerance the reading flips on noise, and an alert that fires on noise is
	// one a user learns to ignore — which costs more than the alert was worth.
	res, _ := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{
			"distance_goal": {Value: 2000, Elapsed: elapsed(0.44)},
		}))

	if d := progressOf(t, res); d.Pace != "on track" {
		t.Errorf("pace = %q, want on track", d.Pace)
	}
}

func TestProgress_NoPaceWithoutElapsed(t *testing.T) {
	// Elapsed is optional. Without it the engine reports attainment and says
	// nothing about rate, rather than inventing a period from the data — the
	// data's span is not the plan's span, and conflating them would answer a
	// question nobody asked.
	res, _ := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}}))

	d := progressOf(t, res)
	if d.Pace != "" || d.Elapsed != nil || d.Projected != nil {
		t.Errorf("expected no pace without Elapsed, got %q / %v / %v", d.Pace, d.Elapsed, d.Projected)
	}
}

func TestProgress_ZeroElapsedProjectsNothing(t *testing.T) {
	// A rate cannot be projected from no time. Dividing by it would produce
	// +Inf, which formats as a number and means nothing.
	res, _ := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{
			"distance_goal": {Value: 2000, Elapsed: elapsed(0)},
		}))

	if d := progressOf(t, res); d.Projected != nil {
		t.Errorf("projected = %v, want none", d.Projected)
	}
}

// ── the error paths, which are the point ────────────────────────────────────

func TestProgress_ZeroPlanIsAnError(t *testing.T) {
	// ratio returns 0 when its denominator is 0. Progress must NOT copy that: a
	// project with no contract value has an undefined attainment, and "0%" reads
	// as "nothing done yet" — a plausible figure standing in for an absent one,
	// which is exactly the failure this aggregation exists to prevent.
	_, err := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 0}}))

	if err == nil {
		t.Fatal("expected an error for a zero plan, got a result")
	}
}

func TestProgress_MissingReferenceIsAnError(t *testing.T) {
	// A missing plan silently treated as 0 makes every attainment infinite and
	// every remaining negative.
	_, err := Execute(progressSpec(), cyclingView(),
		WithReferences(map[string]Reference{"something_else": {Value: 2000}}))

	if err == nil {
		t.Fatal("expected an error for an unknown reference")
	}
}

func TestProgress_NoReferencesSuppliedIsAnError(t *testing.T) {
	_, err := Execute(progressSpec(), cyclingView())
	if err == nil {
		t.Fatal("expected an error when no references were supplied")
	}
}

func TestProgress_MissingOperandIsAnError(t *testing.T) {
	spec := progressSpec()
	spec.Subtrahend = nil

	_, err := Execute(spec, cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}}))
	if err == nil {
		t.Fatal("expected an error for a missing operand")
	}
}

// ── margin ──────────────────────────────────────────────────────────────────

func TestMargin_ReportsTheProportionLeft(t *testing.T) {
	// Same computation as progress; a separate name because the translator needs
	// a word to pick when the user asked only for that number.
	spec := progressSpec()
	spec.Aggregation = "margin"

	res, err := Execute(spec, cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	td := res.Data.(*TextData)
	if !near(td.RawValue, 55) {
		t.Errorf("margin raw value = %.2f, want 55", td.RawValue)
	}
	if td.Progress == nil {
		t.Error("margin should still carry the full ProgressData")
	}
}

// ── difference ──────────────────────────────────────────────────────────────

func receivablesView() RecordView {
	return NewSliceView([]Record{
		{Dimensions: map[string]string{"field": "ToReceive"}, Measures: map[string]float64{"amount": 200000}},
		{Dimensions: map[string]string{"field": "ToReceive"}, Measures: map[string]float64{"amount": 100000}},
		{Dimensions: map[string]string{"field": "Received"}, Measures: map[string]float64{"amount": 120000}},
	})
}

func TestDifference_TwoFilteredSets(t *testing.T) {
	// Outstanding: invoiced minus received. No plan involved — the case that
	// justifies difference existing alongside progress.
	spec := QuerySpec{
		Intent:      "text",
		Aggregation: "difference",
		Measure:     "amount",
		Minuend:     &Operand{Filters: &Filters{Dimensions: map[string][]string{"field": {"ToReceive"}}}},
		Subtrahend:  &Operand{Filters: &Filters{Dimensions: map[string][]string{"field": {"Received"}}}},
	}

	res, err := Execute(spec, receivablesView())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	td := res.Data.(*TextData)
	if td.Difference == nil {
		t.Fatal("expected DifferenceData")
	}
	if !near(td.Difference.Difference, 180000) {
		t.Errorf("difference = %.0f, want 180000", td.Difference.Difference)
	}
}

func TestDifference_MixesAReferenceAndRecords(t *testing.T) {
	// Profit: contract value minus costs. Expressible as a difference, though
	// progress says more about the same two numbers.
	spec := QuerySpec{
		Intent:      "text",
		Aggregation: "difference",
		Measure:     "amount",
		Minuend:     &Operand{Reference: "contract_value"},
		Subtrahend:  &Operand{Filters: &Filters{Dimensions: map[string][]string{"field": {"Received"}}}},
	}

	res, err := Execute(spec, receivablesView(),
		WithReferences(map[string]Reference{"contract_value": {Value: 200000}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if d := res.Data.(*TextData).Difference; !near(d.Difference, 80000) {
		t.Errorf("difference = %.0f, want 80000", d.Difference)
	}
}

// ── replies and charts ──────────────────────────────────────────────────────

func TestProgress_ResolvesReplyPlaceholders(t *testing.T) {
	spec := progressSpec()
	spec.Reply = "You're at {attained_percent} of {plan} — {remaining} to go, and you're {pace}."

	res, _ := Execute(spec, cyclingView(),
		WithReferences(map[string]Reference{
			"distance_goal": {Value: 2000, Elapsed: elapsed(0.83)},
		}))

	for _, unresolved := range []string{"{attained_percent}", "{plan}", "{remaining}", "{pace}"} {
		if strings.Contains(res.Reply, unresolved) {
			t.Errorf("%s left unresolved in %q", unresolved, res.Reply)
		}
	}
	if !strings.Contains(res.Reply, "behind") {
		t.Errorf("expected the pace in the reply, got %q", res.Reply)
	}
}

func TestProgress_PacePlaceholdersEmptyWithoutElapsed(t *testing.T) {
	// A template written for the richer case must degrade to a shorter sentence
	// rather than printing "{pace}" at the user.
	spec := progressSpec()
	spec.Reply = "You're at {attained_percent}. {pace}"

	res, _ := Execute(spec, cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}}))

	if strings.Contains(res.Reply, "{pace}") {
		t.Errorf("unresolved placeholder in %q", res.Reply)
	}
}

func TestProgress_ChartCarriesThePlanAsAReferenceLine(t *testing.T) {
	spec := progressSpec()
	spec.Intent = "chart"
	spec.Visualize = "bar"
	spec.GroupBy = []string{"month"}

	res, err := Execute(spec, cyclingView(),
		WithReferences(map[string]Reference{"distance_goal": {Value: 2000}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Type != "chart" || res.ChartConfig == nil {
		t.Fatalf("expected a chart, got type %q", res.Type)
	}
	if res.ChartConfig.ReferenceLine == nil {
		t.Fatal("a progress chart without the plan on it is just a bar chart")
	}
	if !near(res.ChartConfig.ReferenceLine.Value, 2000) {
		t.Errorf("reference line = %.0f, want 2000", res.ChartConfig.ReferenceLine.Value)
	}
	// Still carries the numbers for anyone reading rather than looking.
	if td, ok := res.Data.(*TextData); !ok || td.Progress == nil {
		t.Error("a progress chart should still carry ProgressData")
	}
}

// ── currency ────────────────────────────────────────────────────────────────

func mixedCurrencyView() RecordView {
	// Two projects: one billing in INR, one in SGD.
	return NewSliceView([]Record{
		{Dimensions: map[string]string{"currency": "INR", "category": "Expense"},
		 Measures: map[string]float64{"amount": 104500}},
		{Dimensions: map[string]string{"currency": "SGD", "category": "Expense"},
		 Measures: map[string]float64{"amount": 700}},
	})
}

func TestProgress_ConvertsTheActualBeforeComparingIt(t *testing.T) {
	// TPL staging, 19 Sep: "what is my total profit" across three projects said
	// 66.3% used. The PLAN had been converted to INR by the caller; the ACTUAL
	// was summed raw, so S$700 counted as 700 rupees instead of ₹52,631.58.
	// Plan in one currency, actual in another, a percentage across the two, and
	// no error anywhere.
	//
	// Execute normalises currency at step 2, after the early returns — so every
	// two-operand aggregation has to do it itself.
	spec := QuerySpec{
		Intent:      "text",
		Aggregation: "progress",
		Measure:     "amount",
		Minuend:     &Operand{Reference: "contract_value"},
		Subtrahend:  &Operand{Filters: &Filters{Dimensions: map[string][]string{"category": {"Expense"}}}},
	}

	res, err := Execute(spec, mixedCurrencyView(),
		WithCurrency("INR", "currency", map[string]float64{"SGD": 75.19, "INR": 1}),
		WithReferences(map[string]Reference{"contract_value": {Value: 385187.97}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d := progressOf(t, res)
	// 104,500 + (700 × 75.19) = 157,133. NOT 105,200.
	if d.Actual < 157000 || d.Actual > 157200 {
		t.Fatalf("actual = %.2f — the SGD row was not converted", d.Actual)
	}
	if !near(d.Plan, 385187.97) {
		t.Errorf("plan = %.2f, want 385187.97", d.Plan)
	}
}

func TestDifference_ConvertsBothSides(t *testing.T) {
	spec := QuerySpec{
		Intent:      "text",
		Aggregation: "difference",
		Measure:     "amount",
		Minuend:     &Operand{Filters: &Filters{Dimensions: map[string][]string{"currency": {"INR"}}}},
		Subtrahend:  &Operand{Filters: &Filters{Dimensions: map[string][]string{"currency": {"SGD"}}}},
	}

	res, err := Execute(spec, mixedCurrencyView(),
		WithCurrency("INR", "currency", map[string]float64{"SGD": 75.19, "INR": 1}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 104,500 − (700 × 75.19) = 51,867.
	d := res.Data.(*TextData).Difference
	if d.SubtrahendValue < 52000 || d.SubtrahendValue > 52700 {
		t.Errorf("subtrahend = %.2f — not converted", d.SubtrahendValue)
	}
}