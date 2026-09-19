package engine

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

// ============================================================================
// EXECUTOR — Dispatcher + Placeholder Resolution
// ============================================================================
// Entry point: Execute(spec, view, opts...)
//
// Pipeline:
//   1. Apply filters from QuerySpec → SubView
//   2. (Optional) Wrap in CurrencyView for normalization
//   3. Group and aggregate
//   4. Dispatch to builder (chart / table / text)
//   5. Resolve reply template placeholders
//   6. Return Result
//
// This function never calls an AI service. All computation is local.
// Zero data copy — the engine reads consumer data through RecordView.
// ============================================================================

// Execute runs a QuerySpec against a RecordView and returns a render-ready Result.
// This is the primary function consumers call after AI translation.
//
// Options:
//   - WithCurrency(base, dimension, rates) — enables multi-currency normalization
//   - WithDefaultMeasure(key) — sets the measure when QuerySpec.Measure is empty
func Execute(spec QuerySpec, view RecordView, opts ...Option) (*Result, error) {
	cfg := applyOptions(opts)

	// Resolve which measure to aggregate
	measure := spec.Measure
	if measure == "" {
		measure = cfg.DefaultMeasure
	}
	if measure == "" {
		measure = "amount" // last-resort default
	}

	if view.Len() == 0 {
		return &Result{
			Success: true,
			Type:    "text",
			Reply:   "No data available to analyze.",
		}, nil
	}

	// ── EVIDENCE: everything below this line is LOCAL. The engine package
	// imports no HTTP/AI client; no network call or AI invocation can occur
	// from here on. All numbers are computed in-process. ──
	trace("================= ENTERING LOCAL EXECUTION =================")
	trace("No AI and no network from this point. Executing QuerySpec over %d in-memory records.", view.Len())
	trace("QuerySpec: intent=%q aggregation=%q measure=%q groupBy=%v filters=%v",
		spec.Intent, spec.Aggregation, measure, spec.GroupBy, spec.Filters.Dimensions)

	log.Printf("🔧 Spektr: Processing %d records, intent=%s, visualize=%s, aggregation=%s, measure=%s",
		view.Len(), spec.Intent, spec.Visualize, spec.Aggregation, measure)

	// ── RATIO AGGREGATION (early return) ──────────────────────────────────
	if spec.Aggregation == "ratio" && spec.CompareFilters != nil {
		return executeRatio(spec, view, measure, cfg)
	}

	// ── PROGRESS / DIFFERENCE / MARGIN (early return) ─────────────────────
	// Two-operand aggregations. Unlike the others they may read a dataset
	// Reference, so they cannot be expressed as a fold over records.
	switch spec.Aggregation {
	case "progress", "margin":
		return executeProgress(spec, view, measure, cfg)
	case "difference":
		return executeDifference(spec, view, measure, cfg)
	}

	// ── MULTI-MEASURE COMPARISON CHART (early return) ──────────────────────
	if spec.Intent == "chart" && len(spec.Measures) > 1 {
		return executeMultiMeasure(spec, view, cfg)
	}

	// 1. Apply filters → SubView (zero-copy)
	filtered := ApplyFilters(view, spec.Filters)

	if filtered.Len() == 0 {
		return &Result{
			Success: true,
			Type:    "text",
			Reply:   "No records match your query filters. Try broadening your search.",
		}, nil
	}

	log.Printf("🔧 Spektr: %d records after filtering (from %d)", filtered.Len(), view.Len())

	// 2. Currency normalization — wrap in CurrencyView (zero-copy)
	displayUnit := cfg.BaseCurrency
	needsConversion := false
	if cfg.BaseCurrency != "" && cfg.CurrencyDimension != "" && len(cfg.ExchangeRates) > 0 {
		displayUnit, needsConversion = detectDisplayCurrency(filtered, cfg.CurrencyDimension, cfg.BaseCurrency)
		if needsConversion {
			log.Printf("💱 Spektr: Multi-currency detected, normalizing to %s", cfg.BaseCurrency)
			filtered = newCurrencyView(filtered, measure, cfg.CurrencyDimension, cfg.BaseCurrency, cfg.ExchangeRates)
			displayUnit = cfg.BaseCurrency
		}
	}
	if displayUnit == "" {
		displayUnit = inferUnit(filtered, cfg.CurrencyDimension)
	}

	// 3. Group and aggregate
	groups := GroupAndAggregate(filtered, spec.GroupBy, measure, spec.Aggregation, spec.SortBy, spec.Limit)

	// 4. Dispatch to builder
	result := &Result{
		Success:       true,
		DisplayUnit:   displayUnit,
		ShouldConvert: needsConversion,
	}

	switch spec.Intent {
	case "chart":
		result.Type = "chart"
		result.ChartConfig = BuildChart(spec, groups)
		if result.ChartConfig == nil {
			result.Type = "text"
			result.Reply = "Not enough data to generate a chart."
			return result, nil
		}

	case "table":
		result.Type = "table"
		result.TableData = BuildTable(spec, groups, filtered, measure, displayUnit)

	case "text":
		result.Type = "text"
		result.Data = BuildText(spec, groups, filtered, measure, displayUnit)
		// Growth with insufficient data override
		if spec.Aggregation == "growth" {
			if textData, ok := result.Data.(*TextData); ok && textData.Growth != nil && textData.Growth.Direction == "insufficient data" {
				result.Reply = fmt.Sprintf("Your data shows %s for %s. Need at least 2 months of data to show trends.",
					textData.Value, textData.Period)
				return result, nil
			}
		}

	default:
		result.Type = "text"
		result.Data = BuildText(spec, groups, filtered, measure, displayUnit)
	}

	// 5. Resolve reply template placeholders
	// ── EVIDENCE: computed values are substituted into the reply template
	// LOCALLY, after all computation. These numbers are never returned to the AI. ──
	trace("Resolving reply template LOCALLY (computed values never sent to AI):")
	trace("  template (from AI, unresolved): %q", spec.Reply)
	result.Reply = ResolvePlaceholders(spec.Reply, groups, filtered, measure, displayUnit)
	trace("  resolved (filled in-process):   %q", result.Reply)
	trace("================= LOCAL EXECUTION COMPLETE =================")

	return result, nil
}

// ============================================================================
// MULTI-MEASURE EXECUTION (early return path)
// ============================================================================

func executeMultiMeasure(spec QuerySpec, view RecordView, cfg *config) (*Result, error) {
	filtered := ApplyFilters(view, spec.Filters)
	if filtered.Len() == 0 {
		return &Result{
			Success: true,
			Type:    "text",
			Reply:   "No records match your query filters. Try broadening your search.",
		}, nil
	}

	log.Printf("📊 Spektr: Multi-measure chart — measures=%v, groupBy=%v", spec.Measures, spec.GroupBy)

	chartConfig := BuildMultiMeasureChart(spec, filtered, spec.Measures)
	if chartConfig == nil {
		return &Result{
			Success: true,
			Type:    "text",
			Reply:   "Not enough data to generate a comparison chart.",
		}, nil
	}

	measureLabels := make([]string, len(spec.Measures))
	for i, m := range spec.Measures {
		measureLabels[i] = LabelForDimension(m)
	}

	return &Result{
		Success:     true,
		Type:        "chart",
		ChartConfig: chartConfig,
		Reply:       fmt.Sprintf("Comparing %s.", strings.Join(measureLabels, " vs ")),
	}, nil
}

// ============================================================================
// RATIO EXECUTION (early return path)
// ============================================================================

func executeRatio(spec QuerySpec, view RecordView, measure string, cfg *config) (*Result, error) {
	denominator := ApplyFilters(view, spec.Filters)
	numerator := ApplyFilters(view, *spec.CompareFilters)

	denomSum := SumMeasure(denominator, measure)
	numSum := SumMeasure(numerator, measure)

	var pct float64
	if denomSum > 0 {
		pct = (numSum / denomSum) * 100
	}

	// Detect display unit
	unit := cfg.BaseCurrency
	if unit == "" {
		unit = inferUnit(denominator, cfg.CurrencyDimension)
	}

	numLabel := buildFilterLabel(spec.CompareFilters)
	denomLabel := buildFilterLabel(&spec.Filters)

	displayValue := fmt.Sprintf("%.1f%%", pct)
	// ConcatView for period derivation — no data copy
	combined := newConcatView(denominator, numerator)
	period := DerivePeriod(combined)

	textData := &TextData{
		Value:    displayValue,
		RawValue: pct,
		Unit:     unit,
		Period:   period,
		Count:    numerator.Len() + denominator.Len(),
		Ratio: &RatioData{
			NumeratorTotal:   numSum,
			DenominatorTotal: denomSum,
			Percentage:       pct,
			NumeratorLabel:   numLabel,
			DenominatorLabel: denomLabel,
		},
	}

	// Resolve placeholders
	reply := spec.Reply
	replacements := map[string]string{
		"{ratio_percent}":     displayValue,
		"{numerator_total}":   FormatCurrency(numSum, unit),
		"{denominator_total}": FormatCurrency(denomSum, unit),
		"{numerator_label}":   numLabel,
		"{denominator_label}": denomLabel,
		"{period}":            period,
		"{total}":             FormatCurrency(numSum, unit),
	}
	for k, v := range replacements {
		reply = strings.ReplaceAll(reply, k, v)
	}

	log.Printf("📊 Spektr: Ratio — %s / %s = %.1f%%", numLabel, denomLabel, pct)

	return &Result{
		Success:       true,
		Type:          "text",
		Reply:         reply,
		Data:          textData,
		DisplayUnit:   unit,
		ShouldConvert: false,
	}, nil
}

// ============================================================================
// TWO-OPERAND AGGREGATIONS — progress, margin, difference
// ============================================================================

// resolveOperand returns an operand's value and a label for it.
//
// An operand is either a named dataset Reference — the plan — or a sum over a
// filtered subset of the records — the actual. Everything else in the engine
// folds over records; these two aggregations are the only place a value can come
// from outside them.
func resolveOperand(op *Operand, view RecordView, defaultMeasure string, cfg *config) (float64, string, RecordView, error) {
	if op == nil {
		return 0, "", nil, fmt.Errorf("operand is missing")
	}

	if op.IsReference() {
		ref, ok := cfg.References[op.Reference]
		if !ok {
			// Deliberately an error rather than a zero. A missing plan silently
			// treated as 0 makes every attainment infinite and every remaining
			// negative — plausible-looking numbers with nothing behind them.
			return 0, "", nil, fmt.Errorf("unknown reference %q: supply it with engine.WithReferences", op.Reference)
		}
		return ref.Value, op.Reference, nil, nil
	}

	if op.Filters == nil {
		return 0, "", nil, fmt.Errorf("operand has neither a reference nor filters")
	}

	measure := op.Measure
	if measure == "" {
		measure = defaultMeasure
	}
	sub := ApplyFilters(view, *op.Filters)
	return SumMeasure(sub, measure), buildFilterLabel(op.Filters), sub, nil
}

// executeProgress answers how the actual is doing against the plan.
//
// "Cycling 2,000 km in six months" is the plan, and the user set it — reading it
// back tells them nothing. "Currently at 45%, with one month to go" is the
// answer, and it is the reason this engine is more than a calculator.
//
// Both aggregation names land here. "progress" reports the whole picture;
// "margin" is the same computation when the user asked only for the proportion
// left, and exists as a separate name because the translator needs a word to
// pick.
func executeProgress(spec QuerySpec, view RecordView, measure string, cfg *config) (*Result, error) {
	plan, planLabel, _, err := resolveOperand(spec.Minuend, view, measure, cfg)
	if err != nil {
		return nil, fmt.Errorf("progress minuend: %w", err)
	}
	actual, actualLabel, actualView, err := resolveOperand(spec.Subtrahend, view, measure, cfg)
	if err != nil {
		return nil, fmt.Errorf("progress subtrahend: %w", err)
	}

	// A zero plan is an error, not a zero.
	//
	// ratio returns 0 when its denominator is 0; progress must not copy that. A
	// project with no contract value has an UNDEFINED attainment, and "0%" reads
	// as "nothing done yet" — a plausible figure standing in for an absent one,
	// which is the failure this whole aggregation exists to prevent.
	if plan == 0 {
		return nil, fmt.Errorf("progress against a zero plan is undefined: %q has no value", planLabel)
	}

	remaining := plan - actual
	data := &ProgressData{
		Plan:        plan,
		Actual:      actual,
		Remaining:   remaining,
		Attained:    (actual / plan) * 100,
		Outstanding: (remaining / plan) * 100,
		PlanLabel:   planLabel,
		ActualLabel: actualLabel,
	}

	// Pace — only when the caller told us how far through the plan period we are.
	if ref, ok := cfg.References[spec.Minuend.Reference]; ok && ref.Elapsed != nil {
		elapsedPct := *ref.Elapsed * 100
		data.Elapsed = &elapsedPct

		switch {
		case *ref.Elapsed <= 0:
			// Nothing has elapsed; a rate cannot be projected from no time.
			data.Pace = "on track"
		default:
			projected := actual / *ref.Elapsed
			data.Projected = &projected
			data.Pace = pace(data.Attained, elapsedPct)
		}
	}

	unit := cfg.BaseCurrency
	if unit == "" && actualView != nil {
		unit = inferUnit(actualView, cfg.CurrencyDimension)
	}

	// "margin" asked only for the proportion; "progress" for the whole picture.
	displayValue := fmt.Sprintf("%.1f%%", data.Attained)
	if spec.Aggregation == "margin" {
		displayValue = fmt.Sprintf("%.1f%%", data.Outstanding)
	}

	period := ""
	if actualView != nil {
		period = DerivePeriod(actualView)
	}

	textData := &TextData{
		Value:    displayValue,
		RawValue: data.Attained,
		Unit:     unit,
		Period:   period,
		Count:    viewLen(actualView),
		Progress: data,
	}
	if spec.Aggregation == "margin" {
		textData.RawValue = data.Outstanding
	}

	reply := resolveProgressPlaceholders(spec.Reply, data, unit, period)

	// A chart if one was asked for. Progress is the one addition here with an
	// obvious visual: the bar against the line says "not there yet" faster than
	// the sentence does.
	var chart *ChartConfig
	resultType := "text"
	if spec.Intent == "chart" && actualView != nil {
		groups := GroupAndAggregate(actualView, spec.GroupBy, measure, "sum", spec.SortBy, spec.Limit)
		chart = BuildProgressChart(spec, data, groups)
		resultType = "chart"
	}

	log.Printf("📊 Spektr: Progress — %s of %s = %.1f%% attained, %.1f%% remaining%s",
		FormatCurrency(actual, unit), FormatCurrency(plan, unit),
		data.Attained, data.Outstanding, paceSuffix(data))

	return &Result{
		Success:       true,
		Type:          resultType,
		Reply:         reply,
		Title:         spec.Title,
		ChartConfig:   chart,
		Data:          textData,
		DisplayUnit:   unit,
		ShouldConvert: false,
	}, nil
}

// executeDifference subtracts one operand from another.
//
// It exists for the cases with no plan at all — invoiced minus received, where
// both operands are filtered sets of records and nothing was promised in
// advance. Where a plan IS involved, progress says more and says it better.
func executeDifference(spec QuerySpec, view RecordView, measure string, cfg *config) (*Result, error) {
	minuend, minLabel, minView, err := resolveOperand(spec.Minuend, view, measure, cfg)
	if err != nil {
		return nil, fmt.Errorf("difference minuend: %w", err)
	}
	subtrahend, subLabel, subView, err := resolveOperand(spec.Subtrahend, view, measure, cfg)
	if err != nil {
		return nil, fmt.Errorf("difference subtrahend: %w", err)
	}

	diff := minuend - subtrahend
	data := &DifferenceData{
		MinuendValue:    minuend,
		SubtrahendValue: subtrahend,
		Difference:      diff,
		MinuendLabel:    minLabel,
		SubtrahendLabel: subLabel,
	}

	unit := cfg.BaseCurrency
	if unit == "" {
		unit = inferUnit(firstNonNil(minView, subView, view), cfg.CurrencyDimension)
	}

	period := DerivePeriod(newConcatView(orEmpty(minView, view), orEmpty(subView, view)))

	textData := &TextData{
		Value:      FormatCurrency(diff, unit),
		RawValue:   diff,
		Unit:       unit,
		Period:     period,
		Count:      viewLen(minView) + viewLen(subView),
		Difference: data,
	}

	reply := spec.Reply
	for k, v := range map[string]string{
		"{difference}":       FormatCurrency(diff, unit),
		"{minuend_total}":    FormatCurrency(minuend, unit),
		"{subtrahend_total}": FormatCurrency(subtrahend, unit),
		"{minuend_label}":    minLabel,
		"{subtrahend_label}": subLabel,
		"{period}":           period,
		"{total}":            FormatCurrency(diff, unit),
	} {
		reply = strings.ReplaceAll(reply, k, v)
	}

	log.Printf("📊 Spektr: Difference — %s − %s = %s",
		FormatCurrency(minuend, unit), FormatCurrency(subtrahend, unit), FormatCurrency(diff, unit))

	return &Result{
		Success:       true,
		Type:          "text",
		Reply:         reply,
		Data:          textData,
		DisplayUnit:   unit,
		ShouldConvert: false,
	}, nil
}

// pace compares what has been attained against how much of the plan period has
// gone. A tolerance band keeps a reading of 49.8% against 50% from being called
// "behind" — that is noise, not a finding, and an alert fired on it is one a
// user learns to ignore.
const paceTolerancePct = 5.0

func pace(attainedPct, elapsedPct float64) string {
	switch {
	case attainedPct > elapsedPct+paceTolerancePct:
		return "ahead"
	case attainedPct < elapsedPct-paceTolerancePct:
		return "behind"
	default:
		return "on track"
	}
}

func paceSuffix(d *ProgressData) string {
	if d.Pace == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", d.Pace)
}

func viewLen(v RecordView) int {
	if v == nil {
		return 0
	}
	return v.Len()
}

func orEmpty(v, fallback RecordView) RecordView {
	if v == nil {
		return fallback
	}
	return v
}

func firstNonNil(views ...RecordView) RecordView {
	for _, v := range views {
		if v != nil {
			return v
		}
	}
	return nil
}

// ============================================================================
// PLACEHOLDER RESOLUTION
// ============================================================================

// ResolvePlaceholders substitutes computed values into the reply template.
func ResolvePlaceholders(template string, groups []Group, view RecordView, measure string, unit string) string {
	if template == "" {
		return buildDefaultReply(view, measure, unit)
	}

	total := SumMeasure(view, measure)
	count := view.Len()
	period := DerivePeriod(view)

	replacements := map[string]string{
		"{total}":    FormatCurrency(total, unit),
		"{count}":    fmt.Sprintf("%d", count),
		"{period}":   period,
		"{currency}": unit,
	}

	// Top group (highest value)
	if len(groups) > 0 {
		topGroup := groups[0]
		for _, g := range groups[1:] {
			if g.Value > topGroup.Value {
				topGroup = g
			}
		}
		replacements["{top_category}"] = topGroup.Label
		replacements["{top_amount}"] = FormatCurrency(topGroup.Value, unit)
	}

	// Average
	if count > 0 {
		replacements["{avg}"] = FormatCurrency(total/float64(count), unit)
	}

	// Max and Min
	if count > 0 {
		replacements["{max}"] = FormatCurrency(MaxMeasure(view, measure), unit)
		replacements["{min}"] = FormatCurrency(MinMeasure(view, measure), unit)
	}

	// Growth placeholders
	growthData := BuildGrowthText(view, measure, unit)
	if growthData.Growth != nil {
		g := growthData.Growth
		replacements["{growth_percent}"] = fmt.Sprintf("%.1f%%", g.ChangePercent)
		replacements["{change_amount}"] = FormatCurrency(g.ChangeAmount, unit)
		replacements["{earliest_value}"] = FormatCurrency(g.EarliestValue, unit)
		replacements["{latest_value}"] = FormatCurrency(g.LatestValue, unit)
		replacements["{earliest_period}"] = g.EarliestPeriod
		replacements["{latest_period}"] = g.LatestPeriod
		replacements["{direction}"] = g.Direction
	}

	result := template
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Safety net: strip unresolved placeholders
	result = stripUnresolvedPlaceholders(result)
	return result
}

// ============================================================================
// QUERYSPEC NORMALIZATION
// ============================================================================

// NormalizeQuerySpec applies deterministic rules to fix common AI inconsistencies.
func NormalizeQuerySpec(spec QuerySpec) QuerySpec {
	changed := false

	// Rule 1: "list" aggregation must be a table
	if spec.Aggregation == "list" && spec.Intent != "table" {
		spec.Intent = "table"
		spec.Visualize = "table"
		changed = true
	}

	// Rule 2: Charts must have a groupBy dimension
	if spec.Intent == "chart" && len(spec.GroupBy) == 0 {
		spec.Intent = "text"
		spec.Visualize = "text"
		changed = true
	}

	// Rule 3: max/min with no groupBy → text
	if (spec.Aggregation == "max" || spec.Aggregation == "min") && len(spec.GroupBy) == 0 {
		spec.Intent = "text"
		spec.Visualize = "text"
		changed = true
	}

	if changed {
		log.Printf("🔧 NormalizeQuerySpec: Adjusted → intent=%s, groupBy=%v, aggregation=%s",
			spec.Intent, spec.GroupBy, spec.Aggregation)
	}

	return spec
}

// ============================================================================
// CURRENCY HELPERS
// ============================================================================

// detectDisplayCurrency checks if records span multiple currencies.
func detectDisplayCurrency(view RecordView, currencyDimension string, baseCurrency string) (string, bool) {
	if view.Len() == 0 {
		return baseCurrency, false
	}

	currencies := make(map[string]bool)
	for i := 0; i < view.Len(); i++ {
		c := view.Dimension(i, currencyDimension)
		if c != "" {
			currencies[c] = true
		}
	}

	if len(currencies) == 1 {
		for c := range currencies {
			return c, false
		}
	}

	return baseCurrency, true
}

// inferUnit tries to determine a unit from the first record's currency dimension.
func inferUnit(view RecordView, currencyDimension string) string {
	if currencyDimension == "" || view.Len() == 0 {
		return ""
	}
	return view.Dimension(0, currencyDimension)
}

// ============================================================================
// INTERNAL HELPERS
// ============================================================================

func buildDefaultReply(view RecordView, measure string, unit string) string {
	if view.Len() == 0 {
		return "No matching records found."
	}
	return fmt.Sprintf("Found %d records totalling %s.",
		view.Len(), FormatCurrency(SumMeasure(view, measure), unit))
}

// buildFilterLabel creates a human-readable label from Filters.
func buildFilterLabel(f *Filters) string {
	if f == nil || f.IsEmpty() {
		return "All"
	}

	parts := []string{}
	for _, vals := range f.Dimensions {
		if len(vals) > 0 {
			parts = append(parts, strings.Join(vals, ", "))
		}
	}

	if len(parts) == 0 {
		return "All records"
	}
	return strings.Join(parts, " — ")
}

var placeholderRegex = regexp.MustCompile(`\{[a-z_]+\}`)

func stripUnresolvedPlaceholders(text string) string {
	cleaned := placeholderRegex.ReplaceAllString(text, "")
	cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimRight(cleaned, " .—-–")
	if cleaned == "" {
		return text
	}
	return cleaned
}