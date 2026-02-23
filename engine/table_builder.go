package engine

import (
	"fmt"
)

// ============================================================================
// TABLE BUILDER — Produces TableData from QuerySpec + Groups
// ============================================================================
// All functions operate on RecordView — zero-copy access to any data source.
// Column discovery uses view.DimensionKeys() instead of inspecting Record maps.
// ============================================================================

// BuildTable produces a TableData from a QuerySpec, groups, filtered view, and display unit.
func BuildTable(spec QuerySpec, groups []Group, view RecordView, measure string, unit string) *TableData {
	if spec.Aggregation == "list" {
		return buildListTable(spec, view, measure, unit)
	}
	return buildAggregatedTable(spec, groups, measure, unit)
}

// ============================================================================
// LIST TABLE — Row per record
// ============================================================================

func buildListTable(spec QuerySpec, view RecordView, measure string, unit string) *TableData {
	if view.Len() == 0 {
		return &TableData{
			Title:   spec.Title,
			Columns: []Column{},
			Rows:    [][]string{},
		}
	}

	// Discover columns from view's registered dimension keys
	dimKeys := view.DimensionKeys()
	columns := make([]Column, 0, len(dimKeys)+1)

	for _, key := range dimKeys {
		columns = append(columns, Column{
			Key:   key,
			Label: LabelForDimension(key),
			Type:  "text",
			Align: "left",
		})
	}

	// Add measure column
	columns = append(columns, Column{
		Key:   measure,
		Label: LabelForDimension(measure),
		Type:  "number",
		Align: "right",
	})

	// Build rows
	rows := make([][]string, 0, view.Len())
	var total float64

	for i := 0; i < view.Len(); i++ {
		row := make([]string, 0, len(columns))
		for _, key := range dimKeys {
			row = append(row, view.Dimension(i, key))
		}
		val := view.Measure(i, measure)
		row = append(row, fmt.Sprintf("%.2f", val))
		rows = append(rows, row)
		total += val
	}

	return &TableData{
		Title:   spec.Title,
		Columns: columns,
		Rows:    rows,
		Summary: &Summary{
			Label: fmt.Sprintf("Total (%d records)", view.Len()),
			Values: map[string]string{
				measure: FormatCurrency(total, unit),
			},
		},
	}
}

// ============================================================================
// AGGREGATED TABLE — Summary rows
// ============================================================================

func buildAggregatedTable(spec QuerySpec, groups []Group, measure string, unit string) *TableData {
	if len(groups) == 0 {
		return &TableData{
			Title:   spec.Title,
			Columns: []Column{},
			Rows:    [][]string{},
		}
	}

	groupLabel := "Group"
	if len(spec.GroupBy) > 0 {
		groupLabel = LabelForDimension(spec.GroupBy[0])
	}
	valueLabel := LabelForAggregation(spec.Aggregation)

	columns := []Column{
		{Key: "group", Label: groupLabel, Type: "text", Align: "left"},
		{Key: "value", Label: valueLabel, Type: "number", Align: "right"},
		{Key: "count", Label: "Count", Type: "number", Align: "center"},
	}

	rows := make([][]string, 0, len(groups))
	var totalValue float64
	var totalCount int

	for _, g := range groups {
		rows = append(rows, []string{
			g.Label,
			fmt.Sprintf("%.2f", g.Value),
			fmt.Sprintf("%d", g.Count),
		})
		totalValue += g.Value
		totalCount += g.Count
	}

	return &TableData{
		Title:   spec.Title,
		Columns: columns,
		Rows:    rows,
		Summary: &Summary{
			Label: "Total",
			Values: map[string]string{
				"value": FormatCurrency(totalValue, unit),
				"count": fmt.Sprintf("%d", totalCount),
			},
		},
	}
}