# Spektr

**Domain-agnostic analytics engine. Picolytics for any dataset.**

Spektr takes structured data from any domain — finance, project management, security operations, HR — and produces render-ready analytics output: charts, tables, and text summaries. Pair it with an AI translator to get natural language analytics out of the box.

```
"Show spending by category"  →  AI Translator  →  QuerySpec  →  Spektr Engine  →  Chart JSON
"Average response time by severity"  →  same pipeline, different data
```

## Why Spektr?

Most analytics libraries are tightly coupled to their domain. Spektr isn't. It operates on two primitives:

- **Dimensions** — string fields you group and filter by (category, status, location, month)
- **Measures** — numeric fields you aggregate (amount, story_points, response_time)

Any dataset that has these two things works with Spektr. No schema migration, no ETL pipeline, no database required.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     YOUR APPLICATION                      │
├──────────────┬──────────────┬────────────────────────────┤
│   Translator │    Engine    │     Schema (optional)       │
│              │              │                             │
│  NL → Query  │  Query →     │  Describes dataset shape    │
│  (Gemini,    │  Result      │  for AI prompt building     │
│   OpenAI)    │  (local,     │  and auto-discovery         │
│              │   no AI)     │                             │
│  Optional —  │  ← Core ──→ │  Optional —                 │
│  bring your  │  zero deps   │  auto or manual             │
│  own LLM     │  WASM-safe   │                             │
└──────────────┴──────────────┴────────────────────────────┘
```

The engine has **zero external dependencies** and never calls any AI service. All computation is local. This makes it safe for WASM compilation and privacy-sensitive environments.

## Installation

```bash
go get github.com/spektr-org/spektr
```

Requires Go 1.21+.

## Quick Start

### Option 1: With `[]Record` (ad-hoc / CSV data)

```go
package main

import (
    "fmt"
    "github.com/spektr-org/spektr/engine"
)

func main() {
    // Your data as generic records
    records := []engine.Record{
        {
            Dimensions: map[string]string{"category": "Expense", "field": "Rent", "month": "Jan-2026"},
            Measures:   map[string]float64{"amount": 2500},
        },
        {
            Dimensions: map[string]string{"category": "Expense", "field": "Groceries", "month": "Jan-2026"},
            Measures:   map[string]float64{"amount": 800},
        },
        {
            Dimensions: map[string]string{"category": "Income", "field": "Salary", "month": "Jan-2026"},
            Measures:   map[string]float64{"amount": 8000},
        },
    }

    // Wrap in a view
    view := engine.NewSliceView(records)

    // Define what to compute
    spec := engine.QuerySpec{
        Intent:      "chart",
        Filters:     engine.Filters{Dimensions: map[string][]string{"category": {"Expense"}}},
        GroupBy:     []string{"field"},
        Aggregation: "sum",
        Visualize:   "bar",
        Reply:       "You spent {total} across {count} transactions.",
    }

    // Execute
    result, err := engine.Execute(spec, view, engine.WithDefaultMeasure("amount"))
    if err != nil {
        panic(err)
    }

    fmt.Println("Type:", result.Type)           // "chart"
    fmt.Println("Reply:", result.Reply)          // "You spent SGD 3,300.00 across 2 transactions."
    fmt.Println("Chart:", result.ChartConfig)    // Bar chart with Rent=2500, Groceries=800
}
```

### Option 2: With `DomainAdapter` (zero-copy typed structs)

This is the recommended approach for applications. Your data stays in its original struct — no maps, no conversion, no allocation overhead.

```go
package main

import (
    "fmt"
    "github.com/spektr-org/spektr/engine"
)

// Your existing domain type — no changes needed
type Transaction struct {
    CategoryName string
    FieldName    string
    LocationName string
    Month        string
    Currency     string
    Amount       float64
}

// Declare once at init — tells Spektr how to read your struct
var adapter = engine.NewDomainAdapter[Transaction]().
    Dimension("category", func(t Transaction) string { return t.CategoryName }).
    Dimension("field",    func(t Transaction) string { return t.FieldName }).
    Dimension("location", func(t Transaction) string { return t.LocationName }).
    Dimension("month",    func(t Transaction) string { return t.Month }).
    Dimension("currency", func(t Transaction) string { return t.Currency }).
    Measure("amount",     func(t Transaction) float64 { return t.Amount })

func main() {
    transactions := []Transaction{
        {"Expense", "Rent", "Singapore", "Jan-2026", "SGD", 2500},
        {"Expense", "Groceries", "Singapore", "Jan-2026", "SGD", 800},
        {"Income", "Salary", "Singapore", "Jan-2026", "SGD", 8000},
    }

    // Bind — O(1), zero copy. Spektr reads your struct fields directly.
    view := adapter.Bind(transactions)

    spec := engine.QuerySpec{
        Intent:      "text",
        Filters:     engine.Filters{Dimensions: map[string][]string{"category": {"Expense"}}},
        Aggregation: "sum",
        Reply:       "Total expenses: {total}",
    }

    result, _ := engine.Execute(spec, view, engine.WithDefaultMeasure("amount"))
    fmt.Println(result.Reply) // "Total expenses: SGD 3,300.00"
}
```

## Domain Examples

### Personal Finance

```go
var financeAdapter = engine.NewDomainAdapter[Transaction]().
    Dimension("category", func(t Transaction) string { return t.CategoryName }).
    Dimension("field",    func(t Transaction) string { return t.FieldName }).
    Dimension("location", func(t Transaction) string { return t.LocationName }).
    Dimension("month",    func(t Transaction) string { return t.Month }).
    Dimension("currency", func(t Transaction) string { return t.Currency }).
    Measure("amount",     func(t Transaction) float64 { return t.Amount })

// "How much did I spend on rent in Singapore?"
// "Show income vs expenses by month"
// "What percentage of salary was transferred to India?"
```

### Jira / Project Management

```go
type JiraIssue struct {
    Key        string
    Status     string
    Priority   string
    Assignee   string
    Component  string
    Sprint     string
    IssueType  string
    Created    string  // "Jan-2026"
    Points     float64
    TimeSpent  float64 // hours
}

var jiraAdapter = engine.NewDomainAdapter[JiraIssue]().
    Dimension("status",    func(j JiraIssue) string { return j.Status }).
    Dimension("priority",  func(j JiraIssue) string { return j.Priority }).
    Dimension("assignee",  func(j JiraIssue) string { return j.Assignee }).
    Dimension("component", func(j JiraIssue) string { return j.Component }).
    Dimension("sprint",    func(j JiraIssue) string { return j.Sprint }).
    Dimension("type",      func(j JiraIssue) string { return j.IssueType }).
    Dimension("month",     func(j JiraIssue) string { return j.Created }).
    Measure("story_points",     func(j JiraIssue) float64 { return j.Points }).
    Measure("time_spent_hours", func(j JiraIssue) float64 { return j.TimeSpent })

// "Show story points by assignee"
// "Average time spent per priority level"
// "Sprint velocity trend by month"
// "What percentage of bugs are critical?"
```

### Security Operations (SOAR / SIEM)

```go
type Incident struct {
    ID              string
    Severity        string  // Critical, High, Medium, Low
    Status          string  // Open, In Progress, Resolved, Closed
    Assignee        string
    IncidentType    string  // Phishing, Malware, Data Leak, Brute Force
    Source          string  // SIEM, Email, Endpoint, User Report
    Month           string
    ResponseMinutes float64
    SLAHours        float64
    RiskScore       float64
}

var soarAdapter = engine.NewDomainAdapter[Incident]().
    Dimension("severity", func(i Incident) string { return i.Severity }).
    Dimension("status",   func(i Incident) string { return i.Status }).
    Dimension("assignee", func(i Incident) string { return i.Assignee }).
    Dimension("type",     func(i Incident) string { return i.IncidentType }).
    Dimension("source",   func(i Incident) string { return i.Source }).
    Dimension("month",    func(i Incident) string { return i.Month }).
    Measure("response_min", func(i Incident) float64 { return i.ResponseMinutes }).
    Measure("sla_hours",    func(i Incident) float64 { return i.SLAHours }).
    Measure("risk_score",   func(i Incident) float64 { return i.RiskScore })

// "Average response time by severity"
// "Show incident trend by month"
// "Which assignee handles the most critical incidents?"
// "What percentage of incidents breached SLA?"
```

### HR / People Analytics

```go
type Employee struct {
    Department string
    Level      string
    Location   string
    Gender     string
    JoinMonth  string
    Salary     float64
    Tenure     float64 // years
}

var hrAdapter = engine.NewDomainAdapter[Employee]().
    Dimension("department", func(e Employee) string { return e.Department }).
    Dimension("level",      func(e Employee) string { return e.Level }).
    Dimension("location",   func(e Employee) string { return e.Location }).
    Dimension("gender",     func(e Employee) string { return e.Gender }).
    Dimension("month",      func(e Employee) string { return e.JoinMonth }).
    Measure("salary",       func(e Employee) float64 { return e.Salary }).
    Measure("tenure_years", func(e Employee) float64 { return e.Tenure })

// "Average salary by department"
// "Headcount by location"
// "Show hiring trend by month"
```

## Core Concepts

### RecordView — Zero-Copy Data Access

The engine never owns your data. It reads through the `RecordView` interface:

```go
type RecordView interface {
    Len() int
    Dimension(index int, key string) string
    Measure(index int, key string) float64
    DimensionKeys() []string
    MeasureKeys() []string
}
```

Five implementations ship with Spektr:

| View | Purpose | Allocates? |
|------|---------|-----------|
| `SliceView` | Wraps `[]Record` (CSV, ad-hoc) | No — holds reference |
| `DomainView[T]` | Reads typed structs via accessor functions | No — reads fields directly |
| `SubView` | Filtered subset (indices into parent) | No — just index list |
| `CurrencyView` | Normalizes currency on read | No — converts per call |
| `ConcatView` | Virtual join of two views | No — delegates reads |

The entire filter → group → aggregate pipeline works through this interface. Filtering returns a `SubView` (index list), currency normalization wraps in a `CurrencyView`, ratio queries use `ConcatView` for period derivation — all zero-copy.

### QuerySpec — What to Compute

```go
type QuerySpec struct {
    Intent         string   // "text", "table", "chart"
    Filters        Filters  // Which records to include
    CompareFilters *Filters // For ratio queries (numerator)
    Aggregation    string   // "sum", "count", "avg", "max", "min", "list", "growth", "ratio"
    Measure        string   // Which measure to aggregate
    GroupBy        []string // Dimension keys to group by
    SortBy         string   // "value_desc", "value_asc", "date_asc", etc.
    Limit          int      // Max groups (0 = all)
    Visualize      string   // "bar", "line", "pie", "table", "text"
    Title          string   // Chart/table title
    Reply          string   // Template: "You spent {total} on {filter_label}"
    Confidence     float64  // 0.0–1.0 (from AI translator)
}
```

QuerySpec can be built manually, from a UI, or generated by the AI translator from natural language.

### Filters — Dimension-Based Selection

```go
filters := engine.Filters{
    Dimensions: map[string][]string{
        "category": {"Expense"},           // OR within dimension
        "location": {"Singapore", "India"}, // AND across dimensions
        "month":    {"Jan-2026", "Feb-2026"},
    },
}
```

Values within a dimension are OR-combined. Dimensions are AND-combined. All matching is case-insensitive.

### Result — Render-Ready Output

```go
type Result struct {
    Success     bool         // Always check this
    Type        string       // "chart", "table", "text"
    Reply       string       // Human-readable answer with resolved placeholders

    ChartConfig *ChartConfig // Populated when Type = "chart"
    TableData   *TableData   // Populated when Type = "table"
    Data        interface{}  // *TextData when Type = "text"

    DisplayUnit   string     // Currency/unit for display
    ShouldConvert bool       // Whether multi-currency normalization was applied
}
```

The result is designed to be serialized as JSON and consumed directly by frontend chart libraries (Recharts, Chart.js, etc.) or rendered into spreadsheets.

## Features

### Aggregation Types

| Aggregation | Description | Example Query |
|-------------|-------------|---------------|
| `sum` | Total value | "Total spending" |
| `count` | Number of records | "How many transactions" |
| `avg` | Average value | "Average expense" |
| `max` | Largest value | "Biggest expense" |
| `min` | Smallest value | "Smallest income" |
| `list` | No aggregation, row per record | "Show all transactions" |
| `growth` | Change from earliest to latest period | "Has my salary increased?" |
| `ratio` | Percentage comparison between two sets | "What % of salary was transferred?" |

### Multi-Currency Normalization

```go
rates := map[string]float64{
    "INR": 0.016,  // 1 INR = 0.016 SGD
    "USD": 1.35,   // 1 USD = 1.35 SGD
}

result, err := engine.Execute(spec, view,
    engine.WithDefaultMeasure("amount"),
    engine.WithCurrency("SGD", "currency", rates),
)
```

When records span multiple currencies, Spektr wraps the view in a `CurrencyView` that converts on read — no data copy. Single-currency queries display in the source currency untouched.

### Reply Templates

The `Reply` field in QuerySpec supports placeholders that get resolved after computation:

```
"You spent {total} on {top_category} in {period}"
→ "You spent SGD 3,300.00 on Rent in Jan-2026 – Feb-2026"
```

Available placeholders:

| Placeholder | Description |
|-------------|-------------|
| `{total}` | Sum of filtered records |
| `{count}` | Number of matching records |
| `{period}` | Date range |
| `{currency}` | Display currency |
| `{top_category}` | Highest value group |
| `{top_amount}` | Highest value |
| `{avg}` | Average |
| `{max}` | Maximum single value |
| `{min}` | Minimum single value |
| `{growth_percent}` | Change percentage |
| `{direction}` | "increased", "decreased", "unchanged" |
| `{earliest_value}` | First period value |
| `{latest_value}` | Last period value |
| `{ratio_percent}` | Ratio result |
| `{numerator_total}` | Ratio numerator sum |
| `{denominator_total}` | Ratio denominator sum |

Unresolved placeholders are automatically stripped from the output.

### Growth Analysis

```go
spec := engine.QuerySpec{
    Intent:      "text",
    Filters:     engine.Filters{Dimensions: map[string][]string{"field": {"Salary"}}},
    Aggregation: "growth",
    Reply:       "Your salary has {direction} by {growth_percent} — from {earliest_value} to {latest_value}.",
}
```

Groups records by month, compares earliest vs latest period. Returns direction, change amount, and percentage. Requires at least 2 months of data.

### Ratio Queries

```go
spec := engine.QuerySpec{
    Intent:      "text",
    Aggregation: "ratio",
    Filters:     engine.Filters{Dimensions: map[string][]string{"field": {"Salary"}}},         // Denominator
    CompareFilters: &engine.Filters{Dimensions: map[string][]string{"field": {"ToIndia"}}},     // Numerator
    Reply:       "You transferred {ratio_percent} of your salary ({numerator_total} out of {denominator_total}).",
}
// → "You transferred 18.2% of your salary (SGD 3,000.00 out of SGD 16,500.00)."
```

### QuerySpec Normalization

AI translators are non-deterministic. `NormalizeQuerySpec` applies deterministic rules to fix common inconsistencies before execution:

```go
spec = engine.NormalizeQuerySpec(spec)
result, err := engine.Execute(spec, view)
```

Rules applied:
- `aggregation: "list"` forces `intent: "table"`
- `intent: "chart"` with no `groupBy` falls back to `intent: "text"`
- `aggregation: "max"/"min"` with no `groupBy` forces `intent: "text"`

## Package Structure

```
spektr/
├── engine/          # Core computation — zero dependencies
│   ├── view.go          # RecordView interface + all implementations
│   ├── types.go         # QuerySpec, Result, Group, Chart/Table/Text types
│   ├── filters.go       # Dimension-based filtering → SubView
│   ├── aggregators.go   # Grouping, aggregation, sorting, formatting
│   ├── executor.go      # Main pipeline: Execute() + placeholder resolution
│   ├── chart_builder.go # Groups → ChartConfig
│   ├── table_builder.go # Groups → TableData
│   ├── text_builder.go  # Groups → TextData (includes growth)
│   └── options.go       # Functional options: WithCurrency, WithDefaultMeasure
│
├── schema/          # Dataset shape description
│   ├── schema.go        # Config, DimensionMeta, MeasureMeta types
│   └── discover.go      # Auto-discovery from CSV/data samples
│
├── translator/      # AI boundary (natural language → QuerySpec)
│   ├── types.go         # Translator interface, Config
│   ├── prompt.go        # Schema-driven prompt builder
│   ├── parser.go        # JSON response parser
│   └── gemini.go        # Google Gemini implementation
│
├── helpers/         # Convenience utilities
│   └── csv.go           # CSV → []Record / RecordView parser
│
└── spektr.go        # Package doc
```

### Dependency Rule

```
engine ← has ZERO external dependencies (WASM-safe)
schema ← no dependency on engine
translator ← depends on engine (QuerySpec) + schema (Config)
helpers ← depends on engine + schema
```

## Integration Tiers

### Tier 1: Auto-Discovery (CSV / ad-hoc data)

Schema auto-discovered from data. Best for quick analysis and demos.

```go
import (
    "github.com/spektr-org/spektr/helpers"
    "github.com/spektr-org/spektr/schema"
)

// Auto-discover schema from CSV
sch, _ := schema.DiscoverFromCSV(csvBytes)

// Parse CSV into RecordView
view, _ := helpers.ParseCSVView(csvBytes, sch)

// Execute
result, _ := engine.Execute(spec, view)
```

### Tier 2: Schema-Guided (defined schema, generic records)

You define the schema explicitly. Records are still generic maps.

```go
sch := schema.Config{
    Name: "Jira Issues",
    Dimensions: []schema.DimensionMeta{
        {Key: "status", DisplayName: "Status", SampleValues: []string{"Open", "In Progress", "Done"}},
        {Key: "priority", DisplayName: "Priority", SampleValues: []string{"Critical", "High", "Medium", "Low"}},
    },
    Measures: []schema.MeasureMeta{
        {Key: "story_points", DisplayName: "Story Points", DefaultAggregation: "sum"},
    },
}
```

### Tier 3: App-Driven (typed structs via DomainAdapter)

Your application has its own data types and metadata store. Use `DomainAdapter` for zero-copy access.

```go
// Declare once at init
var adapter = engine.NewDomainAdapter[YourType]().
    Dimension("key", func(r YourType) string { return r.SomeField }).
    Measure("key", func(r YourType) float64 { return r.SomeNumber })

// Bind per request — O(1)
view := adapter.Bind(yourData)
result, _ := engine.Execute(spec, view, opts...)
```

This is what production applications use. The adapter is declared once and reused across all requests.

## AI Translator (Optional)

The translator converts natural language to QuerySpec using an LLM. It needs a schema to build prompts — it never sees raw data.

```go
import (
    "github.com/spektr-org/spektr/translator"
    "github.com/spektr-org/spektr/schema"
    "github.com/spektr-org/spektr/engine"
)

// Configure
cfg := translator.DefaultGeminiConfig("your-api-key")
t := translator.NewGemini(cfg)

// Translate
result, _ := t.Translate("show spending by category", mySchema)

// Execute
engineResult, _ := engine.Execute(result.QuerySpec, view)
```

The translator is optional. You can build QuerySpec manually, from a UI, or use any other AI provider — just produce a valid QuerySpec and hand it to the engine.

## Output Formats

### ChartConfig

```json
{
    "chartType": "bar",
    "title": "Expenses by Category",
    "xAxis": "Field",
    "yAxis": "Amount",
    "series": [
        {
            "name": "Value",
            "data": [
                {"label": "Rent", "value": 2500},
                {"label": "Groceries", "value": 800}
            ]
        }
    ],
    "showLegend": true,
    "showGrid": true
}
```

Compatible with Recharts, Chart.js, Google Sheets Charts API, and most charting libraries.

### TableData

```json
{
    "title": "All Transactions",
    "columns": [
        {"key": "category", "label": "Category", "type": "text", "align": "left"},
        {"key": "field", "label": "Field", "type": "text", "align": "left"},
        {"key": "amount", "label": "Amount", "type": "number", "align": "right"}
    ],
    "rows": [
        ["Expense", "Rent", "2500.00"],
        ["Expense", "Groceries", "800.00"]
    ],
    "summary": {
        "label": "Total (2 records)",
        "values": {"amount": "SGD 3,300.00"}
    }
}
```

### TextData

```json
{
    "value": "SGD 3,300.00",
    "rawValue": 3300,
    "unit": "SGD",
    "period": "Jan-2026",
    "count": 2,
    "growth": null,
    "ratio": null
}
```

## Testing

```bash
go test ./... -v
```

The engine test suite covers all view types, all aggregation modes, filtering, grouping, currency normalization, growth, ratio, and full pipeline execution through both `SliceView` and `DomainAdapter`.

## Roadmap

- [x] Core engine with RecordView interface
- [x] DomainAdapter for zero-copy typed access
- [x] Schema auto-discovery
- [x] AI translator (Gemini)
- [ ] WASM build for browser/Node.js
- [ ] npm package (`@spektr/engine`)
- [ ] Python bindings
- [ ] Google Sheets integration
- [ ] OpenAI translator implementation
- [ ] Streaming execution for large datasets

## License

MIT
