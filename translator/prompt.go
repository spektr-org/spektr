package translator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spektr-org/spektr/schema"
)

// ============================================================================
// PROMPT BUILDER — Schema-Driven AI Prompt Generation
// ============================================================================
// TPL origin: nlm.go → buildQuerySpecPrompt()
//
// TPL's prompt was ~280 lines of hardcoded finance logic:
//   "category = Income, Expense, Investment, Savings, Transfer"
//   "field = Rent, Salary, Groceries..."
//   "when user says 'expenses' → filter categories: ['Expense']"
//
// Spektr's prompt is generated dynamically from schema.Config:
//   - Dimensions → listed with sample values
//   - Measures → listed with aggregation types
//   - Hierarchies → parent/child relationships explained
//   - Temporal → identified for date-based queries
//   - Currency → if enabled, conversion rules included
//
// Total data sent to AI: ~500-2000 bytes of metadata per query. Never raw data.
// ============================================================================

// BuildPrompt generates the complete system prompt for the AI translator.
// This is the schema-driven replacement for TPL's buildQuerySpecPrompt.
func BuildPrompt(sch schema.Config, dataSummary *DataSummary) string {
	var b strings.Builder

	currentTime := time.Now()

	// ── Header ────────────────────────────────────────────────────────────
	b.WriteString(fmt.Sprintf(`You are a query translator for "%s", a data analytics application.

CURRENT DATE: %s

YOUR ROLE:
Translate the user's natural language query into a structured QuerySpec that a computation engine will execute.
You are a TRANSLATOR ONLY — do NOT compute any values. The engine will do all computation locally.

`, sch.Name, currentTime.Format("2006-01-02")))

	// ── Data Summary ──────────────────────────────────────────────────────
	if dataSummary != nil {
		summaryJSON, _ := json.MarshalIndent(dataSummary, "", "  ")
		b.WriteString(fmt.Sprintf("DATA SUMMARY (what data is available — NOT actual values):\n%s\n\n", string(summaryJSON)))
	}

	// ── Schema Description ────────────────────────────────────────────────
	b.WriteString("DATA MODEL:\n")
	b.WriteString(buildDimensionDescription(sch))
	b.WriteString(buildMeasureDescription(sch))
	b.WriteString("\n")

	// ── Hierarchy Relationships ───────────────────────────────────────────
	hierarchies := buildHierarchyDescription(sch)
	if hierarchies != "" {
		b.WriteString("DIMENSION HIERARCHIES:\n")
		b.WriteString(hierarchies)
		b.WriteString("\n")
	}

	// ── Currency Rules ────────────────────────────────────────────────────
	if sch.Currency != nil && sch.Currency.Enabled {
		b.WriteString(fmt.Sprintf(`CURRENCY:
Base currency: %s
Currency codes are stored in the "%s" dimension.
When querying a single location/group with one currency, filter by that currency.
Cross-group queries will be normalized to %s by the engine.

`, sch.Currency.BaseCurrency, sch.Currency.CodeDimension, sch.Currency.BaseCurrency))
	}

	// ── Response Format ───────────────────────────────────────────────────
	b.WriteString(buildResponseFormat(sch))

	// ── QuerySpec Rules ───────────────────────────────────────────────────
	b.WriteString(buildQuerySpecRules(sch))

	// ── Common Query Translations ─────────────────────────────────────────
	b.WriteString(buildExampleTranslations(sch))

	// ── Footer ────────────────────────────────────────────────────────────
	b.WriteString("\nRemember: You are a TRANSLATOR. Output structured instructions for the engine. Do NOT compute values.\n")

	return b.String()
}

// DataSummary provides lightweight metadata about available data.
// This is what the AI sees — never raw records.
type DataSummary struct {
	RecordCount int                 `json:"recordCount"`
	Dimensions  map[string][]string `json:"dimensions"` // dimension key → unique values found
}

// ============================================================================
// SECTION BUILDERS
// ============================================================================

func buildDimensionDescription(sch schema.Config) string {
	var b strings.Builder

	b.WriteString("DIMENSIONS (string fields for grouping and filtering):\n")
	for _, d := range sch.Dimensions {
		b.WriteString(fmt.Sprintf("- \"%s\"", d.Key))
		if d.DisplayName != "" && d.DisplayName != d.Key {
			b.WriteString(fmt.Sprintf(" (%s)", d.DisplayName))
		}
		if d.Description != "" {
			b.WriteString(fmt.Sprintf(": %s", d.Description))
		}
		if len(d.SampleValues) > 0 {
			b.WriteString(fmt.Sprintf(" — values: [%s]", strings.Join(quotedValues(d.SampleValues), ", ")))
		}
		if d.IsTemporal {
			b.WriteString(" [TEMPORAL — use for time-based queries]")
		}
		if d.IsCurrencyCode {
			b.WriteString(" [CURRENCY CODE]")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buildMeasureDescription(sch schema.Config) string {
	var b strings.Builder

	b.WriteString("\nMEASURES (numeric fields for aggregation):\n")
	for _, m := range sch.Measures {
		b.WriteString(fmt.Sprintf("- \"%s\"", m.Key))
		if m.DisplayName != "" && m.DisplayName != m.Key {
			b.WriteString(fmt.Sprintf(" (%s)", m.DisplayName))
		}
		if m.Description != "" {
			b.WriteString(fmt.Sprintf(": %s", m.Description))
		}
		if m.Unit != "" {
			b.WriteString(fmt.Sprintf(" [unit: %s]", m.Unit))
		}
		aggs := m.Aggregations
		if len(aggs) == 0 {
			aggs = []string{"sum", "avg", "min", "max", "count"}
		}
		b.WriteString(fmt.Sprintf(" — aggregations: [%s]", strings.Join(aggs, ", ")))
		if m.DefaultAggregation != "" {
			b.WriteString(fmt.Sprintf(", default: %s", m.DefaultAggregation))
		}
		if m.IsSynthetic {
			b.WriteString(" [auto-generated]")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buildHierarchyDescription(sch schema.Config) string {
	var b strings.Builder
	for _, d := range sch.Dimensions {
		if d.Parent != "" {
			b.WriteString(fmt.Sprintf("- \"%s\" is a child of \"%s\" (e.g., filter parent then group by child for breakdown)\n", d.Key, d.Parent))
		}
	}
	return b.String()
}

func buildResponseFormat(sch schema.Config) string {
	// Build dimension filter keys dynamically
	filterExample := "{\n"
	for _, d := range sch.Dimensions {
		filterExample += fmt.Sprintf("      \"%s\": [],\n", d.Key)
	}
	filterExample += "    }"

	// Build measure key list
	measureKeys := make([]string, len(sch.Measures))
	for i, m := range sch.Measures {
		measureKeys[i] = m.Key
	}

	// Build groupBy dimension list
	dimKeys := make([]string, len(sch.Dimensions))
	for i, d := range sch.Dimensions {
		dimKeys[i] = fmt.Sprintf("\"%s\"", d.Key)
	}

	return fmt.Sprintf(`RESPONSE FORMAT (ALWAYS valid JSON, no markdown):
{
  "interpretation": {
    "visualType": "bar|line|pie|area|stacked_bar|table|text",
    "summary": "A one-line description of what will be shown",
    "details": [
      {"label": "Data", "value": "Description of data being analyzed"},
      {"label": "Time Period", "value": "Period description"},
      {"label": "Display", "value": "Chart/table/text description"}
    ],
    "suggestions": [
      {"label": "refinement label", "modifier": "appended to query"}
    ],
    "confidence": 0.9
  },
  "querySpec": {
    "intent": "text|table|chart",
    "filters": {
      "dimensions": %s
    },
    "compareFilters": null,
    "aggregation": "sum|count|avg|max|min|list|growth|ratio|none",
    "measure": "%s",
    "groupBy": [],
    "sortBy": "value_desc|value_asc|date_asc|date_desc|alpha_asc",
    "limit": 0,
    "visualize": "bar|line|pie|stacked_bar|area|table|text",
    "title": "Chart or table title",
    "reply": "Template with {total}, {count}, {period}, {top_category}, {top_amount}, {avg}, {max}, {min}, {growth_percent}, {direction}, {earliest_value}, {latest_value}, {ratio_percent} placeholders",
    "confidence": 0.9
  }
}

`, filterExample, sch.GetDefaultMeasure())
}

func buildQuerySpecRules(sch schema.Config) string {
	// Build dimension keys for groupBy examples
	dimKeys := make([]string, 0)
	for _, d := range sch.Dimensions {
		dimKeys = append(dimKeys, fmt.Sprintf("\"%s\"", d.Key))
	}

	// Identify temporal dimensions
	var temporalDims []string
	for _, d := range sch.Dimensions {
		if d.IsTemporal {
			temporalDims = append(temporalDims, d.Key)
		}
	}

	temporalNote := ""
	if len(temporalDims) > 0 {
		temporalNote = fmt.Sprintf(`
TEMPORAL DIMENSIONS: %s
- Use these for time-series queries, trends, and growth analysis.
- Sort by "date_asc" for chronological, "date_desc" for reverse.
`, strings.Join(temporalDims, ", "))
	}

	return fmt.Sprintf(`QUERYSPEC RULES:

1. "intent" — what type of response to generate:
   - "text" → simple total, count, or average (e.g., "how much?", "how many?")
   - "table" → list of records or summary table (e.g., "show all", "list")
   - "chart" → visual chart (e.g., "show by X", "compare", "breakdown")

2. "filters" — which records to include:
   - Keys are dimension names: %s
   - Empty array = no filter (include all values for that dimension)
   - Values must match from the DATA SUMMARY above
   - Filters are AND across dimensions, OR within a dimension

3. "aggregation" — how to combine records:
   - "sum" → total (default for "how much" queries)
   - "count" → number of records ("how many")
   - "avg" → average value
   - "max" → largest value ("biggest", "highest", "largest")
   - "min" → smallest value ("smallest", "lowest")
   - "list" → no aggregation, show individual records ("show all", "list")
   - "growth" → percentage change from earliest to latest period ("trend", "increased", "insights")
   - "ratio" → percentage comparison between two datasets ("what %% of X was Y")
   - "none" → pass-through

4. "measure" — which numeric field to aggregate (from MEASURES above)

5. "groupBy" — dimensions to group by: %s
   - [] → no grouping (single result)
   - Can combine for multi-dimensional: ["dim1", "dim2"]
%s
6. "sortBy":
   - "value_desc" → highest first (default for totals)
   - "value_asc" → lowest first
   - "date_asc" → chronological (for time series)
   - "date_desc" → reverse chronological
   - "alpha_asc" → alphabetical

7. "limit" — max results (0 = all)

8. "visualize" — chart type:
   - For intent "chart": "bar", "line", "pie", "stacked_bar", "area"
   - For intent "table": "table"
   - For intent "text": "text"

9. "reply" — natural language template with placeholders:
   {total}, {count}, {period}, {top_category}, {top_amount}, {avg}, {max}, {min}
   Growth: {growth_percent}, {change_amount}, {earliest_value}, {latest_value}, {direction}
   Ratio: {ratio_percent}, {numerator_total}, {denominator_total}

GROWTH QUERIES:
When user asks about trends, insights, percentage change, whether something increased/decreased:
- aggregation: "growth", intent: "text"

RATIO QUERIES:
When user asks "what percentage of X was Y", "how much of A went to B":
- aggregation: "ratio", intent: "text"
- "filters" = DENOMINATOR (the total/base)
- "compareFilters" = NUMERATOR (the part)

IMPORTANT:
- "list" aggregation → always intent: "table"
- Charts must have at least one groupBy dimension
- max/min with no groupBy → intent: "text"
`, strings.Join(dimKeys, ", "), strings.Join(dimKeys, ", "), temporalNote)
}

func buildExampleTranslations(sch schema.Config) string {
	if len(sch.Dimensions) == 0 || len(sch.Measures) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("EXAMPLE QUERY TRANSLATIONS:\n")

	measure := sch.GetDefaultMeasure()

	// Pick dimensions for examples
	var firstDim, secondDim, temporalDim string
	for _, d := range sch.Dimensions {
		if d.IsTemporal && temporalDim == "" {
			temporalDim = d.Key
		} else if firstDim == "" {
			firstDim = d.Key
		} else if secondDim == "" {
			secondDim = d.Key
		}
	}
	if firstDim == "" && len(sch.Dimensions) > 0 {
		firstDim = sch.Dimensions[0].Key
	}

	// Generate contextual examples
	if firstDim != "" {
		b.WriteString(fmt.Sprintf("- \"show %s by %s\" → groupBy:[\"%s\"], intent:\"chart\", aggregation:\"sum\", measure:\"%s\"\n",
			measure, firstDim, firstDim, measure))
	}
	if temporalDim != "" {
		b.WriteString(fmt.Sprintf("- \"trend over time\" → groupBy:[\"%s\"], intent:\"chart\", visualize:\"line\", sortBy:\"date_asc\"\n",
			temporalDim))
		b.WriteString(fmt.Sprintf("- \"has it increased?\" → intent:\"text\", aggregation:\"growth\"\n"))
	}
	if firstDim != "" {
		b.WriteString(fmt.Sprintf("- \"total %s\" → intent:\"text\", aggregation:\"sum\", measure:\"%s\"\n",
			measure, measure))
		b.WriteString(fmt.Sprintf("- \"show all records\" → intent:\"table\", aggregation:\"list\"\n"))
		b.WriteString(fmt.Sprintf("- \"top 5 by %s\" → groupBy:[\"%s\"], sortBy:\"value_desc\", limit:5\n",
			firstDim, firstDim))
	}
	if firstDim != "" && secondDim != "" {
		b.WriteString(fmt.Sprintf("- \"compare %s across %s\" → groupBy:[\"%s\", \"%s\"], intent:\"chart\", visualize:\"stacked_bar\"\n",
			firstDim, secondDim, secondDim, firstDim))
	}

	b.WriteString("\n")
	return b.String()
}

// ============================================================================
// HELPERS
// ============================================================================

func quotedValues(vals []string) []string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("\"%s\"", v)
	}
	return quoted
}
