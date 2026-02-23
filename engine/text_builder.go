package engine

import (
	"fmt"
	"sort"
)

// ============================================================================
// TEXT BUILDER — Produces TextData for simple queries
// ============================================================================
// All functions operate on RecordView — zero-copy access to any data source.
// ============================================================================

// BuildText produces text response data from filtered records.
func BuildText(spec QuerySpec, groups []Group, view RecordView, measure string, unit string) *TextData {
	if view.Len() == 0 {
		return &TextData{
			Value:    "0",
			RawValue: 0,
			Unit:     unit,
			Period:   DerivePeriod(view),
			Count:    0,
		}
	}

	var value float64
	switch spec.Aggregation {
	case "sum":
		value = SumMeasure(view, measure)
	case "count":
		value = float64(view.Len())
	case "avg":
		value = AvgMeasure(view, measure)
	case "max":
		value = MaxMeasure(view, measure)
	case "min":
		value = MinMeasure(view, measure)
	case "growth":
		return BuildGrowthText(view, measure, unit)
	default:
		value = SumMeasure(view, measure)
	}

	var formatted string
	if spec.Aggregation == "count" {
		formatted = FormatInt(int(value))
	} else {
		formatted = FormatCurrency(value, unit)
	}

	return &TextData{
		Value:    formatted,
		RawValue: value,
		Unit:     unit,
		Period:   DerivePeriod(view),
		Count:    view.Len(),
	}
}

// ============================================================================
// GROWTH BUILDER
// ============================================================================

// BuildGrowthText computes growth/change metrics from chronological data.
func BuildGrowthText(view RecordView, measure string, unit string) *TextData {
	if view.Len() == 0 {
		return &TextData{
			Value:  "No data",
			Unit:   unit,
			Period: "No data",
			Count:  0,
		}
	}

	// Group amounts by month
	monthTotals := make(map[string]float64)
	for i := 0; i < view.Len(); i++ {
		month := view.Dimension(i, "month")
		if month == "" {
			continue
		}
		monthTotals[month] += view.Measure(i, measure)
	}

	// Need at least 2 distinct months
	if len(monthTotals) < 2 {
		total := SumMeasure(view, measure)
		period := DerivePeriod(view)
		return &TextData{
			Value:    FormatCurrency(total, unit),
			RawValue: total,
			Unit:     unit,
			Period:   period,
			Count:    view.Len(),
			Growth: &GrowthData{
				EarliestValue:  total,
				LatestValue:    total,
				EarliestPeriod: period,
				LatestPeriod:   period,
				ChangeAmount:   0,
				ChangePercent:  0,
				Direction:      "insufficient data",
			},
		}
	}

	// Sort months chronologically
	type entry struct {
		Month string
		Order int
		Total float64
	}
	entries := make([]entry, 0, len(monthTotals))
	for m, total := range monthTotals {
		entries = append(entries, entry{
			Month: m,
			Order: ParseMonthOrder(m),
			Total: total,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Order < entries[j].Order
	})

	earliest := entries[0]
	latest := entries[len(entries)-1]

	changeAmount := latest.Total - earliest.Total
	var changePercent float64
	if earliest.Total != 0 {
		changePercent = (changeAmount / earliest.Total) * 100
	}

	direction := "unchanged"
	if changePercent > 0.5 {
		direction = "increased"
	} else if changePercent < -0.5 {
		direction = "decreased"
	}

	absPercent := changePercent
	if absPercent < 0 {
		absPercent = -absPercent
	}
	var displayValue string
	switch direction {
	case "increased":
		displayValue = fmt.Sprintf("↑ %.1f%%", absPercent)
	case "decreased":
		displayValue = fmt.Sprintf("↓ %.1f%%", absPercent)
	default:
		displayValue = "→ No change"
	}

	return &TextData{
		Value:    displayValue,
		RawValue: changePercent,
		Unit:     unit,
		Period:   fmt.Sprintf("%s – %s", earliest.Month, latest.Month),
		Count:    view.Len(),
		Growth: &GrowthData{
			EarliestValue:  earliest.Total,
			LatestValue:    latest.Total,
			EarliestPeriod: earliest.Month,
			LatestPeriod:   latest.Month,
			ChangeAmount:   changeAmount,
			ChangePercent:  changePercent,
			Direction:      direction,
		},
	}
}

// ============================================================================
// PERIOD HELPER
// ============================================================================

// DerivePeriod builds a human-readable period string from a view.
func DerivePeriod(view RecordView) string {
	if view.Len() == 0 {
		return "No data"
	}

	months := make(map[string]bool)
	for i := 0; i < view.Len(); i++ {
		m := view.Dimension(i, "month")
		if m != "" {
			months[m] = true
		}
	}

	if len(months) == 0 {
		return "All time"
	}
	if len(months) == 1 {
		for m := range months {
			return m
		}
	}

	var earliest, latest string
	var earliestOrder, latestOrder int
	first := true

	for m := range months {
		order := ParseMonthOrder(m)
		if first || order < earliestOrder {
			earliest = m
			earliestOrder = order
		}
		if first || order > latestOrder {
			latest = m
			latestOrder = order
		}
		first = false
	}

	return fmt.Sprintf("%s – %s", earliest, latest)
}