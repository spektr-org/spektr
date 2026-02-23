package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// AGGREGATORS — Grouping, Aggregation, and Sorting via RecordView
// ============================================================================
// All functions operate on RecordView — zero-copy access to any data source.
// Grouping produces SubViews (index lists into parent view).
// ============================================================================

// GroupAndAggregate is the main entry point for the aggregation pipeline.
// Pipeline: group → aggregate → sort → limit.
func GroupAndAggregate(
	view RecordView,
	groupBy []string,
	measure string,
	aggregation string,
	sortBy string,
	limit int,
) []Group {
	if view.Len() == 0 {
		return nil
	}

	// 1. Group
	var groups []Group
	if len(groupBy) == 0 {
		groups = []Group{{
			Key:   "all",
			Label: "Total",
			View:  view,
		}}
	} else if len(groupBy) == 1 {
		groups = groupBySingle(view, groupBy[0])
	} else {
		groups = groupByMulti(view, groupBy)
	}

	// 2. Aggregate
	for i := range groups {
		aggregateGroup(&groups[i], measure, aggregation)
		for j := range groups[i].SubGroups {
			aggregateGroup(&groups[i].SubGroups[j], measure, aggregation)
		}
	}

	// 3. Sort
	SortGroups(groups, sortBy)

	// 4. Limit
	if limit > 0 && len(groups) > limit {
		groups = groups[:limit]
	}

	return groups
}

// ============================================================================
// GROUPING
// ============================================================================

func groupBySingle(view RecordView, dimension string) []Group {
	grouped := make(map[string][]int)
	order := make([]string, 0)

	for i := 0; i < view.Len(); i++ {
		key := getDimensionValue(view, i, dimension)
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], i)
	}

	groups := make([]Group, 0, len(order))
	for _, key := range order {
		groups = append(groups, Group{
			Key:   key,
			Label: key,
			View:  newSubView(view, grouped[key]),
		})
	}
	return groups
}

func groupByMulti(view RecordView, dimensions []string) []Group {
	if len(dimensions) < 2 {
		return groupBySingle(view, dimensions[0])
	}

	primaryGroups := groupBySingle(view, dimensions[0])
	for i := range primaryGroups {
		primaryGroups[i].SubGroups = groupBySingle(primaryGroups[i].View, dimensions[1])
	}
	return primaryGroups
}

// getDimensionValue extracts a dimension value from a view at index.
// Handles "year" as a virtual dimension derived from "month".
func getDimensionValue(view RecordView, i int, dimension string) string {
	if dimension == "year" {
		month := view.Dimension(i, "month")
		if parts := strings.Split(month, "-"); len(parts) == 2 {
			return parts[1] // "Jan-2026" → "2026"
		}
		// Try parsing as full date
		if t, err := time.Parse("Jan-2006", month); err == nil {
			return fmt.Sprintf("%d", t.Year())
		}
	}

	return view.Dimension(i, dimension)
}

// ============================================================================
// AGGREGATION
// ============================================================================

func aggregateGroup(group *Group, measure string, aggregation string) {
	group.Count = group.View.Len()
	if group.Count == 0 {
		return
	}

	switch aggregation {
	case "sum":
		group.Value = SumMeasure(group.View, measure)
	case "count":
		group.Value = float64(group.Count)
	case "avg":
		group.Value = AvgMeasure(group.View, measure)
	case "max":
		group.Value = MaxMeasure(group.View, measure)
	case "min":
		group.Value = MinMeasure(group.View, measure)
	case "list":
		group.Value = SumMeasure(group.View, measure) // for sorting
	case "none":
		// pass through
	default:
		group.Value = SumMeasure(group.View, measure)
	}
}

// SumMeasure sums a named measure across a view.
func SumMeasure(view RecordView, measure string) float64 {
	var total float64
	for i := 0; i < view.Len(); i++ {
		total += view.Measure(i, measure)
	}
	return total
}

// AvgMeasure computes average of a named measure.
func AvgMeasure(view RecordView, measure string) float64 {
	n := view.Len()
	if n == 0 {
		return 0
	}
	return SumMeasure(view, measure) / float64(n)
}

// MaxMeasure returns the largest value of a named measure.
func MaxMeasure(view RecordView, measure string) float64 {
	n := view.Len()
	if n == 0 {
		return 0
	}
	m := math.Inf(-1)
	found := false
	for i := 0; i < n; i++ {
		v := view.Measure(i, measure)
		if !found || v > m {
			m = v
			found = true
		}
	}
	if !found {
		return 0
	}
	return m
}

// MinMeasure returns the smallest value of a named measure.
func MinMeasure(view RecordView, measure string) float64 {
	n := view.Len()
	if n == 0 {
		return 0
	}
	m := math.Inf(1)
	found := false
	for i := 0; i < n; i++ {
		v := view.Measure(i, measure)
		if !found || v < m {
			m = v
			found = true
		}
	}
	if !found {
		return 0
	}
	return m
}

// ============================================================================
// SORTING
// ============================================================================

// SortGroups sorts aggregate groups by the specified sort mode.
func SortGroups(groups []Group, sortBy string) {
	switch sortBy {
	case "value_desc", "amount_desc":
		sort.Slice(groups, func(i, j int) bool { return groups[i].Value > groups[j].Value })
	case "value_asc", "amount_asc":
		sort.Slice(groups, func(i, j int) bool { return groups[i].Value < groups[j].Value })
	case "chronological", "date_asc":
		sort.Slice(groups, func(i, j int) bool { return parseSortableDate(groups[i].Key) < parseSortableDate(groups[j].Key) })
	case "reverse_chronological", "date_desc":
		sort.Slice(groups, func(i, j int) bool { return parseSortableDate(groups[i].Key) > parseSortableDate(groups[j].Key) })
	case "label_asc", "alpha_asc":
		sort.Slice(groups, func(i, j int) bool { return strings.ToLower(groups[i].Key) < strings.ToLower(groups[j].Key) })
	case "label_desc":
		sort.Slice(groups, func(i, j int) bool { return strings.ToLower(groups[i].Key) > strings.ToLower(groups[j].Key) })
	default:
		// preserve grouping order
	}
}

// ============================================================================
// FORMATTING UTILITIES
// ============================================================================

// ParseMonthOrder converts "Jan-2026" to sortable int (202601).
func ParseMonthOrder(monthStr string) int {
	t, err := time.Parse("Jan-2006", monthStr)
	if err != nil {
		return 0
	}
	return t.Year()*100 + int(t.Month())
}

func parseSortableDate(key string) int {
	if v := ParseMonthOrder(key); v > 0 {
		return v
	}
	t, err := time.Parse("2006", key)
	if err == nil {
		return t.Year() * 100
	}
	return 0
}

// FormatCurrency formats an amount with currency prefix and comma separators.
func FormatCurrency(amount float64, currency string) string {
	negative := amount < 0
	if negative {
		amount = -amount
	}

	intPart := int64(amount)
	decPart := int64((amount-float64(intPart))*100 + 0.5)

	intStr := fmt.Sprintf("%d", intPart)
	if len(intStr) > 3 {
		var parts []string
		for len(intStr) > 3 {
			parts = append([]string{intStr[len(intStr)-3:]}, parts...)
			intStr = intStr[:len(intStr)-3]
		}
		parts = append([]string{intStr}, parts...)
		intStr = strings.Join(parts, ",")
	}

	result := fmt.Sprintf("%s %s.%02d", currency, intStr, decPart)
	if negative {
		result = "-" + result
	}
	return result
}

// FormatInt formats an integer with comma separators.
func FormatInt(n int) string {
	if n < 0 {
		return "-" + FormatInt(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%s,%03d", FormatInt(n/1000), n%1000)
}

// RoundTo2 rounds to 2 decimal places.
func RoundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}

// UniqueValues returns distinct values for a dimension across a view.
func UniqueValues(view RecordView, dimension string) []string {
	seen := make(map[string]bool)
	var result []string
	for i := 0; i < view.Len(); i++ {
		val := getDimensionValue(view, i, dimension)
		if val != "" && !seen[val] {
			seen[val] = true
			result = append(result, val)
		}
	}
	return result
}

// LabelForDimension returns a capitalized label for a dimension.
func LabelForDimension(dimension string) string {
	if len(dimension) == 0 {
		return ""
	}
	return strings.ToUpper(dimension[:1]) + dimension[1:]
}

// LabelForAggregation returns a human-readable label for an aggregation type.
func LabelForAggregation(aggregation string) string {
	switch aggregation {
	case "sum":
		return "Amount"
	case "count":
		return "Count"
	case "avg":
		return "Average"
	case "max":
		return "Maximum"
	case "min":
		return "Minimum"
	default:
		return "Value"
	}
}