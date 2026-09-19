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
	Aggregation    string   `json:"aggregation"`              // "sum", "count", "avg", "max", "min", "list", "growth", "ratio", "progress", "difference", "margin", "none"
	Measure        string   `json:"measure"`                  // Single measure (used when Measures is empty)
	Measures       []string `json:"measures,omitempty"`       // Multiple measures for comparison charts (one series per measure)
	GroupBy        []string `json:"groupBy"`                  // Dimension keys: ["month"], ["category", "location"]
	SortBy         string   `json:"sortBy"`                   // "value_desc", "value_asc", "date_asc", "date_desc", "alpha_asc"
	Limit          int      `json:"limit"`                    // 0 = all
	Visualize      string   `json:"visualize"`                // "bar", "line", "pie", "stacked_bar", "area", "table", "text"
	Title          string   `json:"title"`                    // Chart/table title
	Reply          string   `json:"reply"`                    // Template: "You spent {total} on {filter_label} in {period}."
	Confidence     float64  `json:"confidence"`               // 0.0–1.0

	// Two-operand aggregations: "progress", "difference", "margin".
	// Each operand is either a named Reference or a filtered sum of records.
	Minuend    *Operand `json:"minuend,omitempty"`
	Subtrahend *Operand `json:"subtrahend,omitempty"`
}

// ============================================================================
// REFERENCES — what the data was measured against
// ============================================================================

// Reference is a scalar belonging to the DATASET rather than to any record in
// it: a contract value, a budget, a quota, a fundraising goal, a distance goal.
//
// The rows are the actual; the reference is the plan. Every one of them is
// agreed, allowed or intended in advance and is not derivable from the data,
// because none of them was ever a row.
//
// Named "reference" rather than "fact" deliberately: in dimensional modelling a
// fact IS a measure in a fact table, which is what a Record carries here, so the
// word would name the opposite of what this holds.
type Reference struct {
	Value float64 `json:"value"`

	// Elapsed is how far through the plan's own period the dataset is, 0–1.
	// Optional. Without it, progress reports attainment but not pace.
	//
	// The CALLER computes this. The engine does not own a calendar and should
	// not learn one: "six months, one to go" is 0.83, arithmetic the domain
	// already has, in a date format the engine would otherwise have to parse and
	// be wrong about.
	Elapsed *float64 `json:"elapsed,omitempty"`
}

// Operand is one side of a two-operand aggregation: either a named dataset
// reference, or a sum over a filtered subset of the records.
type Operand struct {
	Reference string   `json:"reference,omitempty"`
	Filters   *Filters `json:"filters,omitempty"`
	Measure   string   `json:"measure,omitempty"` // defaults to the spec's measure
}

// IsReference reports whether this operand names a dataset reference.
func (o *Operand) IsReference() bool { return o != nil && o.Reference != "" }

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

	// ReferenceLine draws the plan across a progress chart — the 2,000 km, the
	// contract value, the quota. A progress number is the kind of thing people
	// want to SEE rather than read: a bar against a line says "not there yet" in
	// less time than any sentence.
	//
	// Optional and additive; a renderer that does not know about it draws the
	// chart it always drew.
	ReferenceLine *ReferenceLine `json:"referenceLine,omitempty"`
}

// ReferenceLine marks a plan value on a chart.
type ReferenceLine struct {
	Value float64 `json:"value"`
	Label string  `json:"label"`
	Color string  `json:"color,omitempty"`
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

	Progress   *ProgressData   `json:"progress,omitempty"`
	Difference *DifferenceData `json:"difference,omitempty"`
}

// ============================================================================
// PROGRESS — how the actual is doing against the plan
// ============================================================================

// ProgressData answers the question a plan exists to raise.
//
// A reference on its own tells a user nothing they did not already know — they
// set the goal. What they cannot see without analytics is how they are doing
// against it, and whether their current rate gets them there:
//
//	"Cycling 2,000 km in six months"         ← the plan. Not analytics.
//	"currently at 45%, with one month to go" ← this.
type ProgressData struct {
	Plan      float64 `json:"plan"`      // the reference value
	Actual    float64 `json:"actual"`    // sum over the filtered records
	Remaining float64 `json:"remaining"` // plan − actual; negative once overrun

	// Attained and Outstanding are complements, and BOTH are reported rather
	// than one being chosen, because which end a user reads from is a property
	// of the domain: fundraising asks how far along, project work asks what is
	// left, a phone allowance asks both in one sentence. Choosing for them means
	// choosing wrongly for half.
	Attained    float64 `json:"attained"`    // actual ÷ plan, percent
	Outstanding float64 `json:"outstanding"` // remaining ÷ plan, percent

	// Pace — present only when the Reference carried Elapsed.
	//
	// This is where progress stops being arithmetic. Comparing attainment
	// against elapsed answers what the numbers were gathered for: 45% attained
	// at 83% elapsed is behind, and projecting the rate forward says by how much.
	Elapsed   *float64 `json:"elapsed,omitempty"`   // percent of the plan period gone
	Pace      string   `json:"pace,omitempty"`      // "ahead", "behind", "on track"
	Projected *float64 `json:"projected,omitempty"` // the finishing figure at this rate

	PlanLabel   string `json:"planLabel"`
	ActualLabel string `json:"actualLabel"`
}

// DifferenceData is a plain two-operand subtraction, for the cases with no plan
// involved at all — invoiced minus received, say, where both operands are
// filtered sets of records.
type DifferenceData struct {
	MinuendValue    float64 `json:"minuendValue"`
	SubtrahendValue float64 `json:"subtrahendValue"`
	Difference      float64 `json:"difference"`
	MinuendLabel    string  `json:"minuendLabel"`
	SubtrahendLabel string  `json:"subtrahendLabel"`
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