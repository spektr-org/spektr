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
	config.YAxis = LabelForAggregation(spec.Aggregation)

	if len(spec.GroupBy) >= 2 && hasSubGroups(groups) {
		config.Series = buildMultiSeries(groups)
	} else {
		config.Series = buildSingleSeries(groups, spec.Title)
	}

	config.Colors = assignColors(len(config.Series))
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
