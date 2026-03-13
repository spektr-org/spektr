package engine

// ============================================================================
// CHART BUILDER — Produces ChartConfig from QuerySpec + Groups
// ============================================================================
// TPL origin: analytics/chartbuilder.go
// Key change: None significant — types renamed but logic identical.
// ============================================================================

// Default color palette for chart series.
var defaultColors = []string{
	"#4F46E5", "#10B981", "#F59E0B", "#EF4444", "#8B5CF6",
	"#06B6D4", "#EC4899", "#84CC16", "#F97316", "#6366F1",
}

// BuildChart produces a ChartConfig from a QuerySpec and aggregated groups.
// For single-measure queries. For multi-measure use BuildMultiMeasureChart.
func BuildChart(spec QuerySpec, groups []Group) *ChartConfig {
	if len(groups) == 0 {
		return nil
	}

	chartType := spec.Visualize
	if chartType == "" {
		chartType = "bar"
	}

	config := &ChartConfig{
		ChartType:  chartType,
		Title:      spec.Title,
		ShowLegend: true,
		ShowGrid:   chartType != "pie",
	}

	if len(spec.GroupBy) > 0 {
		config.XAxis = LabelForDimension(spec.GroupBy[0])
	}

	// Use measure display name for YAxis when available — avoids generic "Amount"
	if spec.Measure != "" {
		config.YAxis = LabelForDimension(spec.Measure)
	} else {
		config.YAxis = LabelForAggregation(spec.Aggregation)
	}

	if len(spec.GroupBy) >= 2 && hasSubGroups(groups) {
		config.Series = buildMultiSeries(groups)
	} else {
		config.Series = buildSingleSeries(groups, spec.Title)
	}

	config.Colors = assignColors(len(config.Series))
	return config
}

// BuildMultiMeasureChart produces a ChartConfig with one series per measure.
// Used when QuerySpec.Measures has more than one entry — e.g. comparing
// successful_runs vs failed_runs grouped by playbook_id.
//
// Sort alignment: the canonical group order is determined from the first measure,
// then all subsequent series are reordered to match — so bar positions are
// consistent across series regardless of sortBy.
func BuildMultiMeasureChart(spec QuerySpec, view RecordView, measures []string) *ChartConfig {
	if view.Len() == 0 || len(measures) == 0 {
		return nil
	}

	chartType := spec.Visualize
	if chartType == "" {
		chartType = "bar"
	}

	config := &ChartConfig{
		ChartType:  chartType,
		Title:      spec.Title,
		ShowLegend: true,
		ShowGrid:   chartType != "pie",
	}

	if len(spec.GroupBy) > 0 {
		config.XAxis = LabelForDimension(spec.GroupBy[0])
	}
	config.YAxis = LabelForAggregation(spec.Aggregation)

	// Build series — canonical group order from first measure, all others aligned to it.
	series := make([]ChartSeries, 0, len(measures))
	var canonicalLabels []string // label order established by first measure

	for i, measure := range measures {
		groups := GroupAndAggregate(view, spec.GroupBy, measure, spec.Aggregation, spec.SortBy, spec.Limit)

		var points []ChartPoint
		if i == 0 {
			// First measure: record the canonical label order
			canonicalLabels = make([]string, len(groups))
			points = make([]ChartPoint, len(groups))
			for j, g := range groups {
				canonicalLabels[j] = g.Label
				points[j] = ChartPoint{Label: g.Label, Value: RoundTo2(g.Value)}
			}
		} else {
			// Subsequent measures: align to canonical label order to keep bars aligned.
			// Build a lookup by label, then emit in canonical order.
			lookup := make(map[string]float64, len(groups))
			for _, g := range groups {
				lookup[g.Label] = g.Value
			}
			points = make([]ChartPoint, len(canonicalLabels))
			for j, label := range canonicalLabels {
				points[j] = ChartPoint{Label: label, Value: RoundTo2(lookup[label])}
			}
		}

		series = append(series, ChartSeries{
			Name:  LabelForDimension(measure),
			Data:  points,
			Color: defaultColors[i%len(defaultColors)],
		})
	}

	config.Series = series
	config.Colors = assignColors(len(series))
	return config
}

// ============================================================================
// SERIES BUILDERS
// ============================================================================

func buildSingleSeries(groups []Group, seriesName string) []ChartSeries {
	if seriesName == "" {
		seriesName = "Value"
	}

	points := make([]ChartPoint, 0, len(groups))
	for _, g := range groups {
		points = append(points, ChartPoint{
			Label: g.Label,
			Value: RoundTo2(g.Value),
		})
	}

	return []ChartSeries{{
		Name: seriesName,
		Data: points,
	}}
}

func buildMultiSeries(groups []Group) []ChartSeries {
	subKeySet := make(map[string]bool)
	for _, g := range groups {
		for _, sg := range g.SubGroups {
			subKeySet[sg.Key] = true
		}
	}

	subKeys := make([]string, 0, len(subKeySet))
	for k := range subKeySet {
		subKeys = append(subKeys, k)
	}

	seriesMap := make(map[string][]ChartPoint)
	for _, key := range subKeys {
		seriesMap[key] = make([]ChartPoint, 0, len(groups))
	}

	for _, g := range groups {
		sgLookup := make(map[string]float64)
		for _, sg := range g.SubGroups {
			sgLookup[sg.Key] = sg.Value
		}

		for _, key := range subKeys {
			seriesMap[key] = append(seriesMap[key], ChartPoint{
				Label: g.Label,
				Value: RoundTo2(sgLookup[key]),
			})
		}
	}

	series := make([]ChartSeries, 0, len(subKeys))
	for i, key := range subKeys {
		series = append(series, ChartSeries{
			Name:  key,
			Data:  seriesMap[key],
			Color: defaultColors[i%len(defaultColors)],
		})
	}

	return series
}

func hasSubGroups(groups []Group) bool {
	for _, g := range groups {
		if len(g.SubGroups) > 0 {
			return true
		}
	}
	return false
}

func assignColors(count int) []string {
	colors := make([]string, count)
	for i := 0; i < count; i++ {
		colors[i] = defaultColors[i%len(defaultColors)]
	}
	return colors
}