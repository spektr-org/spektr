package engine

// ============================================================================
// SPEKTR ENGINE TYPES — Domain-Agnostic Analytics
// ============================================================================
// TPL origin: analytics/types.go + analytics/analyticstypes.go
// Key change: Transaction (hardcoded fields) → Record (dimension/measure maps)
//             QueryFilters (named fields) → Filters (generic map)
//
// Dependency: engine has ZERO external dependencies.
// ============================================================================

// ============================================================================
// RECORD — Generic data row (replaces TPL's Transaction)
// ============================================================================

// Record is a single data row with string dimensions and numeric measures.
//
// TPL had: Transaction{LocationName, CategoryName, FieldName, Month, Currency, Amount}
// Spektr: Record{Dimensions["location"]="Singapore", Measures["amount"]=3500.00}
type Record struct {
	Dimensions map[string]string  `json:"dimensions"`
	Measures   map[string]float64 `json:"measures"`
}

// ============================================================================
// QUERYSPEC — Contract between AI Translator and Engine
// ============================================================================

// QuerySpec defines what the engine should compute.
// The Translator (Gemini/OpenAI) produces this; the Engine consumes it.
type QuerySpec struct {
	Intent         string   `json:"intent"`                   // "text", "table", "chart"
	Filters        Filters  `json:"filters"`                  // Which records to include
	CompareFilters *Filters `json:"compareFilters,omitempty"` // For ratio: numerator filters
	Aggregation    string   `json:"aggregation"`              // "sum", "count", "avg", "max", "min", "list", "growth", "ratio", "none"
	Measure        string   `json:"measure"`                  // Which measure to aggregate (empty → use default)
	GroupBy        []string `json:"groupBy"`                  // Dimension keys: ["month"], ["category", "location"]
	SortBy         string   `json:"sortBy"`                   // "value_desc", "value_asc", "date_asc", "date_desc", "alpha_asc"
	Limit          int      `json:"limit"`                    // 0 = all
	Visualize      string   `json:"visualize"`                // "bar", "line", "pie", "stacked_bar", "area", "table", "text"
	Title          string   `json:"title"`                    // Chart/table title
	Reply          string   `json:"reply"`                    // Template: "You spent {total} on {filter_label} in {period}."
	Confidence     float64  `json:"confidence"`               // 0.0–1.0
}

// Filters define which records to include.
// Keys are dimension names. Values are allowed values.
// OR within a dimension, AND across dimensions. Empty = all.
//
// TPL had: QueryFilters{Categories: [], Locations: [], Months: [], Fields: [], Currencies: []}
// Spektr:  Filters{Dimensions: {"category": ["Expense"], "location": ["Singapore"]}}
type Filters struct {
	Dimensions map[string][]string `json:"dimensions"`
}

// HasFilter returns true if a specific dimension filter is set.
func (f Filters) HasFilter(dimension string) bool {
	if f.Dimensions == nil {
		return false
	}
	vals, ok := f.Dimensions[dimension]
	return ok && len(vals) > 0
}

// IsEmpty returns true if no filters are set.
func (f Filters) IsEmpty() bool {
	if f.Dimensions == nil {
		return true
	}
	for _, vals := range f.Dimensions {
		if len(vals) > 0 {
			return false
		}
	}
	return true
}

// ============================================================================
// RESULT — Render-ready output (replaces TPL's Response)
// ============================================================================

// Result is the engine's render-ready output.
type Result struct {
	Success bool   `json:"success"`
	Type    string `json:"type"` // "chart", "table", "text"
	Reply   string `json:"reply"`
	Title   string `json:"title"`
	Summary string `json:"summary"`

	// Exactly one of these is populated based on Type:
	ChartConfig *ChartConfig `json:"chartConfig,omitempty"`
	TableData   *TableData   `json:"tableData,omitempty"`
	Data        interface{}  `json:"data,omitempty"` // *TextData for type="text"

	// Metadata
	DisplayUnit   string   `json:"displayUnit,omitempty"`
	ShouldConvert bool     `json:"shouldConvert"`
	Errors        []string `json:"errors,omitempty"`

	// Pass-through for two-phase flow
	QuerySpec      *QuerySpec      `json:"querySpec,omitempty"`
	Interpretation *Interpretation `json:"interpretation,omitempty"`
}

// ============================================================================
// GROUP — Intermediate computation result
// ============================================================================

// Group represents a grouped/aggregated result.
// Builders convert these into ChartConfig, TableData, or TextData.
type Group struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Value     float64    `json:"value"`
	Count     int        `json:"count"`
	SubGroups []Group    `json:"subGroups,omitempty"`
	View      RecordView `json:"-"` // Sub-view for records in this group (zero-copy)
}

// ============================================================================
// CHART TYPES
// ============================================================================

// ChartConfig defines how to render a chart.
// Matches TPL's ChartConfig shape so frontends work unchanged.
type ChartConfig struct {
	ChartType  string        `json:"chartType"`
	Title      string        `json:"title"`
	XAxis      string        `json:"xAxis,omitempty"`
	YAxis      string        `json:"yAxis,omitempty"`
	Series     []ChartSeries `json:"series"`
	Colors     []string      `json:"colors,omitempty"`
	ShowLegend bool          `json:"showLegend"`
	ShowGrid   bool          `json:"showGrid"`
}

// ChartSeries represents a data series in a chart.
type ChartSeries struct {
	Name  string       `json:"name"`
	Data  []ChartPoint `json:"data"`
	Color string       `json:"color,omitempty"`
}

// ChartPoint represents a single data point.
type ChartPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// ============================================================================
// TABLE TYPES
// ============================================================================

// TableData defines how to render a table.
type TableData struct {
	Title   string     `json:"title"`
	Columns []Column   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Summary *Summary   `json:"summary,omitempty"`
}

// Column defines a table column.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`  // "text", "number", "currency"
	Align string `json:"align"` // "left", "center", "right"
}

// Summary provides totals or aggregations for a table.
type Summary struct {
	Label  string            `json:"label"`
	Values map[string]string `json:"values"`
}

// ============================================================================
// TEXT TYPES
// ============================================================================

// TextData is structured data for simple query answers (type="text").
type TextData struct {
	Value    string      `json:"value"`
	RawValue float64     `json:"rawValue"`
	Unit     string      `json:"unit"`
	Period   string      `json:"period"`
	Count    int         `json:"count"`
	Growth   *GrowthData `json:"growth,omitempty"`
	Ratio    *RatioData  `json:"ratio,omitempty"`
}

// GrowthData contains change-over-time metrics.
type GrowthData struct {
	EarliestValue  float64 `json:"earliestValue"`
	LatestValue    float64 `json:"latestValue"`
	EarliestPeriod string  `json:"earliestPeriod"`
	LatestPeriod   string  `json:"latestPeriod"`
	ChangeAmount   float64 `json:"changeAmount"`
	ChangePercent  float64 `json:"changePercent"`
	Direction      string  `json:"direction"` // "increased", "decreased", "unchanged", "insufficient data"
}

// RatioData contains cross-group percentage comparison.
type RatioData struct {
	NumeratorTotal   float64 `json:"numeratorTotal"`
	DenominatorTotal float64 `json:"denominatorTotal"`
	Percentage       float64 `json:"percentage"`
	NumeratorLabel   string  `json:"numeratorLabel"`
	DenominatorLabel string  `json:"denominatorLabel"`
}

// ============================================================================
// INTERPRETATION — Two-phase flow support
// ============================================================================

// Interpretation describes what the AI understood from the query.
type Interpretation struct {
	VisualType  string                `json:"visualType"`
	Summary     string                `json:"summary"`
	Details     []InterpretDetail     `json:"details"`
	Suggestions []InterpretSuggestion `json:"suggestions,omitempty"`
	Confidence  float64               `json:"confidence"`
}

// InterpretDetail is a label-value pair.
type InterpretDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// InterpretSuggestion is a refinement option.
type InterpretSuggestion struct {
	Label    string `json:"label"`
	Modifier string `json:"modifier"`
}