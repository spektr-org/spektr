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

	log.Printf("🔧 Spektr: Processing %d records, intent=%s, visualize=%s, aggregation=%s, measure=%s",
		view.Len(), spec.Intent, spec.Visualize, spec.Aggregation, measure)

	// ── RATIO AGGREGATION (early return) ──────────────────────────────────
	if spec.Aggregation == "ratio" && spec.CompareFilters != nil {
		return executeRatio(spec, view, measure, cfg)
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
	result.Reply = ResolvePlaceholders(spec.Reply, groups, filtered, measure, displayUnit)

	return result, nil
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