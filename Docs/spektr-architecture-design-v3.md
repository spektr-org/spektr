# Spektr: Complete Architecture Design Document

**Picolytics for any dataset**
Extracted from The Pocket Ledger | Version 3.0 | February 2026

---

## 1. What Is Spektr?

Spektr is a portable, language-agnostic analytics engine — picolytics for any dataset. Extracted from The Pocket Ledger and designed to run anywhere. It takes any dataset (CSV, Excel, database, Google Sheets), auto-discovers its structure, and lets users ask natural language questions that return render-ready results (charts, tables, text summaries).

**Core promise:** Import a module. Pass your data. Ask a question in English. Get a chart back. 20 lines of code. Any language.

**Three design pillars:**

1. **Privacy-first** — User data never reaches AI. Gemini only sees column names and sample values (~500 bytes). All computation runs locally in-process.
2. **Intent-driven** — The AI decides the presentation format (chart/table/text). No presentation config needed. The engine returns render-ready output.
3. **Universal** — One Go codebase compiles to native module, WASM binary, and CLI. Works in Go, JavaScript, Python, React, AppScript — any language.

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                     CONSUMER APPLICATION                             │
│                     (owns auth, owns data, owns rendering)           │
│                                                                      │
│  ┌────────────┐    ┌──────────────┐    ┌───────────────────────┐    │
│  │ Data       │    │ Spektr  │    │ Rendering             │    │
│  │ Source     │───▶│ Engine       │───▶│ (Consumer's choice)   │    │
│  │            │    │ (in-process) │    │                       │    │
│  │ • CSV file │    │              │    │ • Google Sheets       │    │
│  │ • DB conn  │    │ • Discover   │    │ • React + Recharts    │    │
│  │ • Sheets   │    │ • Query      │    │ • CLI terminal        │    │
│  │ • Excel    │    │              │    │ • Excel VBA           │    │
│  └────────────┘    └──────┬───────┘    └───────────────────────┘    │
│                           │                                          │
│                    ONLY EXTERNAL CALL                                 │
│                           │                                          │
│                    ┌──────▼───────┐                                  │
│                    │ Gemini API   │                                  │
│                    │ (consumer's  │                                  │
│                    │  API key)    │                                  │
│                    │              │                                  │
│                    │ Sends: schema│                                  │
│                    │ metadata +   │                                  │
│                    │ question     │                                  │
│                    │ (~500 bytes) │                                  │
│                    │              │                                  │
│                    │ Receives:    │                                  │
│                    │ QuerySpec    │                                  │
│                    │ (intent +    │                                  │
│                    │ filters)     │                                  │
│                    └──────────────┘                                  │
│                                                                      │
│  DATA NEVER LEAVES THIS BOX.                                         │
│  AI never sees raw records. Only column names + sample values.       │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. Three-Layer Engine

There is no presentation layer. There is no auth layer. There is no server. The engine is three layers — and one external API call.

### Layer 1: Schema (describes the data shape)

Auto-discovered from the data source or built programmatically by the consumer application. Defines what dimensions (string fields for grouping/filtering) and measures (numeric fields for aggregation) exist, plus metadata the AI needs to understand the domain.

### Layer 2: Data Source (fetches records)

Pluggable connectors that read data from wherever it lives and convert rows into generic `Record` structs the engine understands. The engine never queries a data source directly — the consumer passes data in.

### Layer 3: Engine (translates, computes, builds output)

- **Translator**: Sends schema metadata + user question to Gemini. Receives a QuerySpec (intent, filters, groupBy, measure, chartType). Gemini's only job is natural language → structured query.
- **Executor**: Filter → Group → Aggregate → Build. Pure computation, no network, no dependencies.
- **Builders**: Produce render-ready output based on intent (chart builder, table builder, text builder).

---

## 4. Dynamic Schema Auto-Discovery

No consumer should have to write a schema by hand. The engine connects to any data source, introspects its structure, and generates a working schema automatically.

### 4.1 Three Tiers of Schema Generation

| Tier | Who | When | AI Needed? |
|---|---|---|---|
| **Tier 1 — Full auto** | Engine heuristics | CSV uploads, quick demos | No |
| **Tier 2 — AI-assisted** | Engine + Gemini refinement | Production setup (once) | Yes (one-time, ~500 bytes) |
| **Tier 3 — App-driven** | Consumer app builds schema from own metadata | SaaS platforms like TPL | No |

### 4.2 Tier 1: Heuristic Auto-Discovery

The engine samples N rows and classifies each column:

```
For each column in the data source:

  1. DETECT TYPE
     ├─ All values numeric (int/float)     → numeric_column
     ├─ All values are dates/timestamps    → date_column
     ├─ All values are booleans            → bool_column
     └─ Mixed or all strings               → string_column

  2. CLASSIFY ROLE
     ├─ numeric_column
     │   ├─ Unique count == row count      → SKIP (likely ID)
     │   ├─ Unique count < 20              → DIMENSION (coded numeric)
     │   └─ Unique count >= 20             → MEASURE
     │
     ├─ date_column
     │   └─ Always                         → DIMENSION (temporal, auto-bucket to month)
     │
     ├─ bool_column
     │   └─ Always                         → DIMENSION
     │
     └─ string_column
         ├─ Unique count == row count      → SKIP (likely ID)
         ├─ Unique count > 50% of rows     → SKIP (high cardinality, warn, recoverable)
         ├─ Matches currency code patterns  → DIMENSION (currency)
         └─ Otherwise                       → DIMENSION

  3. DETECT SPECIAL TYPES
     ├─ Currency codes: 3-letter uppercase matching known codes → is_currency_code
     ├─ Month patterns: "Jan-2025", "2025-01" etc.             → is_temporal
     ├─ Quarter patterns: "Q1-2025" etc.                       → is_temporal
     ├─ Year patterns: "2023", "2024" etc.                     → is_temporal
     └─ Hierarchy detection: if col A has N unique values
        and col B has M > N, and every B maps to one A         → parent/child

  4. SYNTHETIC MEASURES
     └─ Auto-create "record_count" (COUNT *) for any dataset
        where counting records is meaningful

  5. DERIVED DIMENSIONS
     └─ Date/timestamp columns auto-create bucketed versions:
        "created" → "created_month", "created_quarter", "created_year"
```

### 4.3 Tier 2: AI-Assisted Refinement (One-Time)

After Tier 1 produces a draft schema, optionally send it to Gemini for semantic enrichment:

**Sent to Gemini (once, at setup):**
```json
{
  "columns": [
    { "name": "Status", "type": "string", "samples": ["To Do", "In Progress", "Done"], "unique": 4 },
    { "name": "Priority", "type": "string", "samples": ["P1 - Critical", "P2 - High"], "unique": 4 },
    { "name": "Story Points", "type": "float", "samples": ["1", "3", "5", "8"], "unique": 6 }
  ],
  "row_count": 2847
}
```

**Gemini returns:**
```json
{
  "suggested_name": "Jira Project Tracker",
  "enrichments": [
    { "column": "Status", "display_name": "Status", "description": "Issue workflow state" },
    { "column": "Priority", "display_name": "Priority", "description": "Issue severity level", "sort_hint": "P1 > P2 > P3 > P4" },
    { "column": "Story Points", "display_name": "Story Points", "description": "Effort estimation in Fibonacci scale" }
  ],
  "suggested_hierarchies": [
    { "parent": "issue_type", "child": "component" }
  ]
}
```

**Total data sent to AI:** ~200-500 bytes of column names and sample values. Never raw data.

### 4.4 Tier 3: App-Driven (TPL Pattern)

The consumer application has its own metadata store and builds the schema programmatically:

```go
// TPL builds schema from user's DB tables at runtime
func buildSchemaForUser(userID string) engine.SchemaConfig {
    locations := db.GetUserLocations(userID)     // ["Singapore", "India"]
    categories := db.GetUserCategories(userID)   // ["Salary", "Rent", "Groceries"]
    fields := db.GetUserFields(userID)           // ["Base Pay", "Monthly Rent"]

    schema := engine.SchemaConfig{
        Name: "Personal Finance",
        Dimensions: []engine.DimensionMeta{
            {Key: "location", DisplayName: "Location", SampleValues: locations},
            {Key: "category", DisplayName: "Category", SampleValues: categories},
            {Key: "field", DisplayName: "Field", SampleValues: fields, Parent: "category"},
            {Key: "month", DisplayName: "Month", IsTemporal: true},
            {Key: "currency", DisplayName: "Currency", IsCurrencyCode: true},
        },
        Measures: []engine.MeasureMeta{
            {Key: "amount", DisplayName: "Amount", IsCurrency: true, DefaultAggregation: "sum"},
        },
    }
    return schema
}
```

No discovery needed. The app already knows its domain.

---

## 5. Schema Configuration Format

Whether auto-discovered or hand-crafted, the schema follows the same structure:

### 5.1 Full Schema Structure

```json
{
  "name": "Human-readable name",
  "version": "1.0",
  "description": "Optional description for AI context",
  "discovered_from": "source-file.csv (auto-populated if discovered)",
  "discovered_at": "2026-02-23T10:00:00Z",

  "dimensions": [
    {
      "key": "unique_snake_case_key",
      "display_name": "Human Label",
      "description": "What this dimension represents (helps AI)",
      "sample_values": ["val1", "val2", "val3"],
      "groupable": true,
      "filterable": true,
      "parent": "parent_dimension_key_or_null",
      "is_temporal": false,
      "temporal_format": "MMM-yyyy",
      "temporal_order": "chronological",
      "is_currency_code": false,
      "cardinality_hint": "low|medium|high",
      "derived_from": "original_column_if_auto_bucketed"
    }
  ],

  "measures": [
    {
      "key": "unique_snake_case_key",
      "display_name": "Human Label",
      "description": "What this measure represents",
      "unit": "currency|units|hours|percent|points|custom",
      "is_currency": false,
      "is_synthetic": false,
      "aggregations": ["sum", "avg", "min", "max", "count"],
      "default_aggregation": "sum",
      "format": "#,##0.00"
    }
  ],

  "currency": {
    "enabled": false,
    "code_dimension": "currency",
    "base_currency": "SGD",
    "rates": { "INR": 0.016, "USD": 1.35 }
  },

  "skipped_columns": [
    {
      "column": "Original Column Name",
      "reason": "Why it was skipped",
      "recoverable": true
    }
  ],

  "defaults": {
    "chart_type": "bar",
    "group_by": ["first_temporal_dimension"],
    "sort_by": "primary_measure",
    "sort_order": "descending",
    "limit": 50
  }
}
```

### 5.2 Dimension Field Reference

| Field | Type | Required | Description |
|---|---|---|---|
| `key` | string | ✅ | Unique internal identifier, used in QuerySpec |
| `display_name` | string | ✅ | Human-readable label for charts, tables, AI prompts |
| `description` | string | ❌ | Helps AI understand what this dimension represents |
| `sample_values` | string[] | ✅ | Representative values (min 2, max 10). AI uses these to understand valid filter values |
| `groupable` | bool | ❌ | Can be used in GROUP BY. Default: true |
| `filterable` | bool | ❌ | Can be used in WHERE filters. Default: true |
| `parent` | string | ❌ | Key of parent dimension for hierarchies |
| `is_temporal` | bool | ❌ | Time-based dimension. Enables chronological sorting |
| `temporal_format` | string | ❌ | Parse pattern for temporal values |
| `temporal_order` | string | ❌ | "chronological" or "reverse". Default: "chronological" |
| `is_currency_code` | bool | ❌ | Contains currency codes (SGD, INR, USD) |
| `cardinality_hint` | string | ❌ | "low" (<10), "medium" (10-100), "high" (100+). Helps AI choose grouping |
| `derived_from` | string | ❌ | Original column if this was auto-bucketed from a date |
| `recoverable` | bool | ❌ | If skipped, can be restored to active schema |

### 5.3 Measure Field Reference

| Field | Type | Required | Description |
|---|---|---|---|
| `key` | string | ✅ | Unique internal identifier |
| `display_name` | string | ✅ | Human-readable label |
| `description` | string | ❌ | Helps AI understand the measure |
| `unit` | string | ❌ | Unit label for display |
| `is_currency` | bool | ❌ | If true, engine applies currency conversion |
| `is_synthetic` | bool | ❌ | True for auto-generated measures (e.g., record_count) |
| `aggregations` | string[] | ❌ | Allowed aggregation types. Default: ["sum", "avg", "min", "max", "count"] |
| `default_aggregation` | string | ❌ | Default when user doesn't specify. Default: "sum" |
| `format` | string | ❌ | Display format: "#,##0.00", "0.0%", etc. |

---

## 6. Data Source Connection

### 6.1 Two Connection Patterns

The engine does not manage data sources. The consumer passes data to the engine. Two patterns:

**Ephemeral (data comes to engine):**
Consumer reads the data from wherever it lives using their own auth/credentials, and passes it to the engine as bytes or string. Engine holds it in memory for the duration of the query. No persistence.

```go
csvBytes := readFile("jira-export.csv")                    // consumer's responsibility
result := engine.Query(csvBytes, schema, "bugs by priority") // engine's responsibility
// csvBytes can be garbage collected — engine doesn't keep a reference
```

**Persistent (consumer manages connection):**
Consumer maintains the connection to their database/sheet. On each query, consumer fetches fresh records and passes them to the engine.

```go
// Consumer connects to their DB with their credentials
rows := db.Query("SELECT * FROM transactions WHERE user_id = $1", userID)
records := convertToRecords(rows, schema)                    // consumer's responsibility
result := engine.Query(records, schema, "spending by month") // engine's responsibility
```

### 6.2 Data Source Helpers (Optional)

While the engine itself doesn't connect to sources, the module ships **optional helper functions** for common formats. These are convenience utilities, not core engine code:

```go
// CSV helper — parses CSV bytes into []engine.Record using schema mapping
records, err := helpers.ParseCSV(csvBytes, schema)

// Excel helper — reads a sheet into []engine.Record
records, err := helpers.ParseExcel(xlsxBytes, "Sheet1", schema)

// JSON helper — converts JSON array into []engine.Record
records, err := helpers.ParseJSON(jsonBytes, schema)
```

These helpers use the schema's mapping information to know which columns become which dimensions/measures. They handle type coercion, date bucketing, and null handling.

**Database connections, Google Sheets API calls, S3 fetches — these are always the consumer's responsibility.** The engine never holds credentials, never opens connections, never manages auth. Consumer fetches, engine processes.

### 6.3 Data Source Mapping

When auto-discovery generates a schema from a CSV, it also generates a mapping that links source column names to schema keys:

```json
{
  "mapping": {
    "Status":         { "key": "status",        "type": "string" },
    "Priority":       { "key": "priority",      "type": "string" },
    "Story Points":   { "key": "story_points",  "type": "float" },
    "Created":        { "key": "created_month", "type": "date",   "transform": "date_to_month_year" },
    "Sprint":         { "key": "sprint",        "type": "string" }
  }
}
```

**Transforms Reference:**

| Transform | Input | Output | Example |
|---|---|---|---|
| `none` | any | same | "Singapore" → "Singapore" |
| `date_to_month_year` | date/datetime | "MMM-yyyy" | "2025-03-15" → "Mar-2025" |
| `date_to_quarter` | date/datetime | "QN-yyyy" | "2025-03-15" → "Q1-2025" |
| `date_to_year` | date/datetime | "yyyy" | "2025-03-15" → "2025" |
| `lowercase` | string | string | "APAC" → "apac" |
| `uppercase` | string | string | "apac" → "APAC" |
| `trim` | string | string | " hello " → "hello" |
| `round_2` | float | float | 3.14159 → 3.14 |

Custom transforms can be registered by the consumer for domain-specific needs.

---

## 7. Intent-Driven Output

There is no presentation layer. The AI decides the output format via intent. The engine returns render-ready results. The consumer writes a simple switch to render on their platform.

### 7.1 Intent Model

| Intent | AI Returns | Engine Builds | Consumer Renders |
|---|---|---|---|
| `chart` | chart_type, groupBy, measure, splitBy | Complete chart config with series, labels, axes, colors, title | Call platform chart API |
| `table` | columns, sortBy, filters | Headers, rows (pre-sorted, pre-formatted), totals row, column alignment | Write cells |
| `text` | measure, filters, comparison | Natural language summary + key metric cards | Display text |
| `growth` | measure, period comparison | % change values, period labels, direction indicators | Display comparison cards |
| `ratio` | measure, compare groups | Percentage breakdown, labels, values | Display pie/donut or cards |

### 7.2 Output Contracts

#### Chart Output

```json
{
  "type": "chart",
  "title": "Bug Count by Priority — Last 6 Sprints",
  "summary": "847 bugs total. P2 dominates at 43%. Spike in Sprint 15.",

  "chart": {
    "chart_type": "stacked_bar",
    "x_axis": {
      "title": "Sprint",
      "labels": ["Sprint 12", "Sprint 13", "Sprint 14", "Sprint 15", "Sprint 16", "Sprint 17"]
    },
    "y_axis": {
      "title": "Bug Count",
      "format": "integer"
    },
    "series": [
      { "name": "P1 - Critical", "values": [12, 15, 8, 20, 11, 14], "color": "#EF4444" },
      { "name": "P2 - High",     "values": [45, 52, 48, 61, 55, 50], "color": "#F59E0B" },
      { "name": "P3 - Medium",   "values": [30, 28, 35, 32, 40, 38], "color": "#3B82F6" },
      { "name": "P4 - Low",      "values": [10, 8, 12, 9, 15, 11],   "color": "#10B981" }
    ]
  }
}
```

#### Table Output

```json
{
  "type": "table",
  "title": "All P1 Bugs — Current Sprint",
  "summary": "14 critical bugs, 8 in progress.",

  "table": {
    "headers": [
      { "key": "field", "label": "Issue", "align": "left", "width": "40%" },
      { "key": "status", "label": "Status", "align": "left", "width": "20%" },
      { "key": "assignee", "label": "Assignee", "align": "left", "width": "25%" },
      { "key": "story_points", "label": "Points", "align": "right", "width": "15%" }
    ],
    "rows": [
      ["Login timeout on mobile", "In Progress", "alice@corp.com", "5"],
      ["Payment fails with expired card", "To Do", "bob@corp.com", "8"],
      ["Dashboard crash on Safari", "In Review", "charlie@corp.com", "3"]
    ],
    "totals_row": ["14 issues", "", "", "67 pts"],
    "column_formats": ["string", "string", "string", "integer"]
  }
}
```

#### Text Output

```json
{
  "type": "text",
  "title": "Sprint 17 Summary",
  "summary": "113 bugs this sprint, up 8% from Sprint 16. P1 bugs decreased 21% to 14. Average resolution time improved to 3.2 days.",

  "metrics": [
    { "label": "Total Bugs", "value": "113", "change": "+8%", "direction": "up" },
    { "label": "P1 Bugs", "value": "14", "change": "-21%", "direction": "down" },
    { "label": "Avg Resolution", "value": "3.2 days", "change": "-0.5 days", "direction": "down" }
  ]
}
```

#### Growth Output

```json
{
  "type": "growth",
  "title": "Bug Trend — Q4 2025 vs Q1 2026",
  "summary": "Overall bug volume increased 15%. Critical bugs doubled.",

  "comparisons": [
    { "label": "Total Bugs", "previous": 312, "current": 359, "change_pct": "+15.1%", "direction": "up" },
    { "label": "P1 Critical", "previous": 18, "current": 36, "change_pct": "+100%", "direction": "up" },
    { "label": "Avg Points/Bug", "previous": 4.2, "current": 3.8, "change_pct": "-9.5%", "direction": "down" }
  ],
  "period_labels": { "previous": "Q4 2025", "current": "Q1 2026" }
}
```

#### Ratio Output

```json
{
  "type": "ratio",
  "title": "Bug Distribution by Component",
  "summary": "Backend accounts for 45% of all bugs.",

  "ratios": [
    { "label": "Backend", "value": 382, "percentage": 45.1, "color": "#EF4444" },
    { "label": "Frontend", "value": 245, "percentage": 28.9, "color": "#3B82F6" },
    { "label": "Mobile", "value": 138, "percentage": 16.3, "color": "#F59E0B" },
    { "label": "Infrastructure", "value": 82, "percentage": 9.7, "color": "#10B981" }
  ],
  "total": 847
}
```

### 7.3 Consumer Rendering — Simple Switch

The consumer writes ONE switch statement. Everything else is platform-specific API calls:

```javascript
// AppScript — entire rendering logic
function render(result, sheet) {
  switch (result.type) {
    case 'chart':
      var chart = sheet.newChart()
        .setChartType(mapChartType(result.chart.chart_type))
        .setPosition(1, 1, 0, 0);
      // set series from result.chart.series
      sheet.insertChart(chart.build());
      break;

    case 'table':
      sheet.getRange(1, 1, 1, result.table.headers.length)
        .setValues([result.table.headers.map(h => h.label)]);
      sheet.getRange(2, 1, result.table.rows.length, result.table.rows[0].length)
        .setValues(result.table.rows);
      break;

    case 'text':
      sheet.getRange(1, 1).setValue(result.summary);
      break;
  }
}
```

```python
# Python CLI — entire rendering logic
def render(result):
    if result['type'] == 'table':
        print(tabulate(result['table']['rows'], headers=[h['label'] for h in result['table']['headers']]))
    elif result['type'] == 'text':
        print(result['summary'])
    elif result['type'] == 'chart':
        print(f"[{result['chart']['chart_type']}] {result['title']}")
        for s in result['chart']['series']:
            print(f"  {s['name']}: {s['values']}")
```

---

## 8. Privacy Boundary

### 8.1 What Gemini Sees vs. What It Never Sees

**Sent to Gemini (per query, ~500 bytes):**
```
Schema: 9 dimensions (status, priority, issue_type, assignee, component,
        sprint, fix_version, created_month, resolved_month)
        4 measures (issue_count, story_points, time_spent_hours,
        original_estimate_hours)
Dimension samples: status=[To Do, In Progress, Done],
                   priority=[P1, P2, P3, P4], ...
User query: "show bug count by priority over last 6 sprints"
```

**Never sent to Gemini — ever:**
- Raw CSV data
- Individual records/rows
- Actual values (who's assigned to what, real story point values)
- Issue summaries, descriptions, comments
- Any PII (names, emails)
- Database credentials
- File contents

**Gemini's only job:** translate natural language + schema metadata → QuerySpec. The QuerySpec is a structured instruction (intent, filters, groupBy, measure). All computation happens locally.

### 8.2 For All Data Sources

| Data Source | What Gemini Sees | What Gemini Never Sees |
|---|---|---|
| TPL (PostgreSQL) | Dimension names, category samples | Transaction amounts, user data |
| Jira CSV | Column names, status/priority values | Issue details, assignee names |
| E-Commerce Excel | Column names, region/category samples | Revenue figures, customer data |
| HR Google Sheet | Dimension names, department/level samples | Salary figures, employee names |

---

## 9. Multi-Tier Distribution

One Go codebase. Three compilation targets. Every language.

### 9.1 Distribution Tiers

```
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│  TIER 1: NATIVE GO MODULE (zero overhead)                   │
│  └─ go get github.com/pocketledger/spektr              │
│     Direct import. Full power including DB helpers.          │
│     Consumer: TPL, Go applications.                         │
│                                                             │
│  TIER 2: WASM BINARY (near-native, in-process)              │
│  ├─ npm install spektr    (Node.js / React / Browser)  │
│  ├─ pip install spektr    (Python)                     │
│  └─ Any WASM-capable runtime   (Deno, Rust, C#, Java)      │
│     Loads ~5-8 MB WASM binary. Runs in-process.             │
│     Supports inline data (CSV, JSON, Excel bytes).          │
│     No DB drivers (no CGo in WASM).                         │
│                                                             │
│  TIER 3: CLI BINARY (universal, any language)                │
│  └─ spektr (single static binary, every OS/arch)       │
│     Any language spawns process, pipes JSON in/out.          │
│     Python: subprocess.run(['spektr'], ...)            │
│     Node: child_process.exec('spektr', ...)            │
│     Ruby, PHP, Bash, Perl — anything with process spawn.    │
│     AppScript: consumer deploys as own Cloud Function.       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 9.2 What Each Tier Supports

| Capability | Tier 1 (Go) | Tier 2 (WASM) | Tier 3 (CLI) |
|---|---|---|---|
| Inline CSV/JSON/Excel | ✅ | ✅ | ✅ |
| DB helpers (PostgreSQL) | ✅ | ❌ (no CGo) | ✅ |
| Schema discovery | ✅ | ✅ | ✅ |
| AI translation (Gemini) | ✅ | ✅ | ✅ |
| Local computation | ✅ | ✅ (~80% perf) | ✅ |
| No server required | ✅ | ✅ | ✅ |
| No auth required | ✅ | ✅ | ✅ |
| Zero dependencies | ✅ | WASM runtime | OS binary |

### 9.3 Go Compilation Constraints

For WASM compatibility, the core engine MUST be **pure Go**:

- No CGo dependencies in engine, translator, builders, helpers
- No OS-level file I/O in the core path (accept bytes, not file paths)
- No database drivers in the core module (provide as optional add-on)
- No network calls except Gemini translation (and that uses standard `net/http`)

DB helpers (PostgreSQL, MySQL, SQLite) ship as a **separate Go package** that imports the core engine. WASM consumers don't get DB helpers — they don't need them because they pass inline data.

### 9.4 Consumer Experience by Language

**Go (Tier 1):**
```go
import "github.com/pocketledger/spektr/engine"

schema, _ := engine.Discover(csvBytes)
result, _ := engine.Query(csvBytes, schema, "bugs by priority", geminiKey)
// result.Type == "chart", result.Chart has everything
```

**Node.js / React (Tier 2 — WASM):**
```javascript
const { discover, query } = require('spektr');

const schema = await discover(csvString);
const result = await query(csvString, schema, 'bugs by priority', geminiKey);
// result.type === 'chart', result.chart has everything
```

**Python (Tier 2 — WASM):**
```python
from spektr import discover, query

schema = discover(csv_string)
result = query(csv_string, schema, 'bugs by priority', gemini_key)
# result['type'] == 'chart', result['chart'] has everything
```

**AppScript (Tier 3 — via consumer's Cloud Function):**
```javascript
function analyzeJira() {
  var csv = DriveApp.getFileById('...').getBlob().getDataAsString();
  var response = UrlFetchApp.fetch('https://my-cloud-function.run.app/query', {
    method: 'post',
    payload: JSON.stringify({ data: csv, query: 'bugs by priority' })
  });
  var result = JSON.parse(response.getContentText());
  renderToSheet(result);
}
```

**CLI (Tier 3):**
```bash
$ cat jira-export.csv | spektr --query "bugs by priority" --gemini-key $KEY
# JSON output to stdout

$ spektr --file jira-export.csv --query "bugs by priority" --gemini-key $KEY --format table
# Pretty-printed table to terminal
```

---

## 10. Schema Examples by Domain

### 10.1 Personal Finance (TPL)

Schema is built dynamically per-user from `user_locations`, `user_categories`, `user_field_definitions` tables (Tier 3).

```json
{
  "name": "Personal Finance Tracker",
  "dimensions": [
    { "key": "location", "display_name": "Location", "sample_values": ["Singapore", "India"] },
    { "key": "category_type", "display_name": "Type", "sample_values": ["Income", "Expense", "Investment"] },
    { "key": "category", "display_name": "Category", "sample_values": ["Salary", "Rent", "Groceries"], "parent": "category_type" },
    { "key": "field", "display_name": "Field", "sample_values": ["Base Pay", "Monthly Rent"], "parent": "category" },
    { "key": "month", "display_name": "Month", "sample_values": ["Jan-2025", "Feb-2025"], "is_temporal": true },
    { "key": "currency", "display_name": "Currency", "sample_values": ["SGD", "INR"], "is_currency_code": true }
  ],
  "measures": [
    { "key": "amount", "display_name": "Amount", "is_currency": true, "default_aggregation": "sum" }
  ],
  "currency": { "enabled": true, "code_dimension": "currency", "base_currency": "SGD", "rates": { "INR": 0.016 } }
}
```

### 10.2 Jira Project Tracker

Auto-discovered from CSV export (Tier 1 + optional Tier 2 refinement).

```json
{
  "name": "Jira Project Tracker",
  "discovered_from": "PROJ-export-feb-2026.csv",
  "dimensions": [
    { "key": "status", "display_name": "Status", "sample_values": ["To Do", "In Progress", "In Review", "Done"], "cardinality_hint": "low" },
    { "key": "priority", "display_name": "Priority", "sample_values": ["P1 - Critical", "P2 - High", "P3 - Medium", "P4 - Low"], "cardinality_hint": "low" },
    { "key": "issue_type", "display_name": "Issue Type", "sample_values": ["Bug", "Story", "Task", "Epic"], "cardinality_hint": "low" },
    { "key": "assignee", "display_name": "Assignee", "sample_values": ["alice@corp.com", "bob@corp.com", "charlie@corp.com"], "cardinality_hint": "medium" },
    { "key": "component", "display_name": "Component", "sample_values": ["Backend", "Frontend", "Mobile", "Infrastructure"], "cardinality_hint": "low" },
    { "key": "sprint", "display_name": "Sprint", "sample_values": ["Sprint 12", "Sprint 13", "Sprint 14"], "is_temporal": true, "temporal_order": "chronological" },
    { "key": "fix_version", "display_name": "Fix Version", "sample_values": ["v2.1", "v2.2", "v3.0"], "cardinality_hint": "low" },
    { "key": "created_month", "display_name": "Created Month", "sample_values": ["Jan-2026", "Feb-2026"], "is_temporal": true, "derived_from": "Created" },
    { "key": "resolved_month", "display_name": "Resolved Month", "sample_values": ["Jan-2026", "Feb-2026"], "is_temporal": true, "derived_from": "Resolved" }
  ],
  "measures": [
    { "key": "issue_count", "display_name": "Issue Count", "unit": "issues", "is_synthetic": true, "default_aggregation": "count" },
    { "key": "story_points", "display_name": "Story Points", "unit": "points", "default_aggregation": "sum" },
    { "key": "time_spent_hours", "display_name": "Time Spent", "unit": "hours", "default_aggregation": "sum" },
    { "key": "original_estimate_hours", "display_name": "Original Estimate", "unit": "hours", "default_aggregation": "sum" }
  ],
  "skipped_columns": [
    { "column": "Issue Key", "reason": "Unique per row — identifier, not analytically useful", "recoverable": false },
    { "column": "Summary", "reason": "Free text, high cardinality — not groupable", "recoverable": false },
    { "column": "Description", "reason": "Free text blob", "recoverable": false },
    { "column": "Reporter", "reason": "High cardinality — available if needed", "recoverable": true }
  ],
  "defaults": { "chart_type": "bar", "group_by": ["status"], "sort_by": "issue_count" }
}
```

### 10.3 E-Commerce Sales

```json
{
  "name": "E-Commerce Sales Dashboard",
  "dimensions": [
    { "key": "region", "display_name": "Region", "sample_values": ["APAC", "EMEA", "Americas"], "cardinality_hint": "low" },
    { "key": "product_category", "display_name": "Product Category", "sample_values": ["Electronics", "Clothing", "Home & Garden"], "cardinality_hint": "low" },
    { "key": "channel", "display_name": "Sales Channel", "sample_values": ["Online", "Retail", "Wholesale"], "cardinality_hint": "low" },
    { "key": "month", "display_name": "Month", "sample_values": ["Jan-2025", "Feb-2025"], "is_temporal": true }
  ],
  "measures": [
    { "key": "revenue", "display_name": "Revenue", "unit": "USD", "default_aggregation": "sum" },
    { "key": "quantity", "display_name": "Units Sold", "unit": "units", "default_aggregation": "sum" },
    { "key": "cost", "display_name": "Cost", "unit": "USD", "default_aggregation": "sum" }
  ]
}
```

### 10.4 HR / People Analytics

```json
{
  "name": "People Analytics",
  "dimensions": [
    { "key": "department", "display_name": "Department", "sample_values": ["Engineering", "Sales", "Marketing", "HR"] },
    { "key": "level", "display_name": "Job Level", "sample_values": ["IC1", "IC2", "IC3", "Manager", "Director"] },
    { "key": "office", "display_name": "Office", "sample_values": ["Singapore", "San Francisco", "London", "Bangalore"] },
    { "key": "quarter", "display_name": "Quarter", "sample_values": ["Q1-2025", "Q2-2025"], "is_temporal": true }
  ],
  "measures": [
    { "key": "headcount", "display_name": "Headcount", "unit": "people", "default_aggregation": "sum" },
    { "key": "salary", "display_name": "Salary", "unit": "USD", "aggregations": ["avg", "min", "max", "sum"], "default_aggregation": "avg" },
    { "key": "satisfaction_score", "display_name": "Satisfaction", "unit": "score", "aggregations": ["avg"], "default_aggregation": "avg" }
  ]
}
```

---

## 11. Engine API Surface

The entire public API is five functions:

```go
package engine

// Configure sets the AI translator credentials. Called once.
func Configure(config Config) error

// Discover auto-generates a schema from raw data (CSV bytes, JSON bytes, etc.)
// Uses Tier 1 heuristics. Pass refine=true for Tier 2 AI-assisted enrichment.
func Discover(data []byte, opts ...DiscoverOption) (*SchemaConfig, error)

// DiscoverOption controls discovery behavior
func WithRefinement(enabled bool) DiscoverOption  // Tier 2 AI enrichment
func WithSampleSize(n int) DiscoverOption          // rows to inspect (0 = all)
func WithRecoverable(columns []string) DiscoverOption // include skipped columns

// Query takes raw data + schema + natural language question → render-ready result.
// This is the primary function consumers call.
func Query(data []byte, schema SchemaConfig, question string) (*Result, error)

// QueryWithSpec takes pre-parsed records + a QuerySpec (skips AI translation).
// Used when the consumer already has a QuerySpec (e.g., from a saved/cached query).
func QueryWithSpec(records []Record, schema SchemaConfig, spec QuerySpec) (*Result, error)

// ParseRecords converts raw bytes into []Record using schema mapping.
// Exposed for consumers who want to manage records separately.
func ParseRecords(data []byte, schema SchemaConfig) ([]Record, error)
```

**Config:**
```go
type Config struct {
    GeminiAPIKey string // consumer's API key
    Model        string // optional, defaults to "gemini-2.0-flash"
}
```

**Result:**
```go
type Result struct {
    Type    string      `json:"type"`    // "chart", "table", "text", "growth", "ratio"
    Title   string      `json:"title"`
    Summary string      `json:"summary"`
    Chart   *ChartData  `json:"chart,omitempty"`
    Table   *TableData  `json:"table,omitempty"`
    Text    *TextData   `json:"text,omitempty"`
    Growth  *GrowthData `json:"growth,omitempty"`
    Ratio   *RatioData  `json:"ratio,omitempty"`
}
```

---

## 12. Migration Path from TPL

### Phase 1: Extract Core Engine (2-3 days)

Extract existing analytics code into standalone Go module with zero breaking changes to TPL.

| Current TPL File | Becomes | Key Change |
|---|---|---|
| `analytics/types.go` | `engine/types.go` | TPL-specific `Transaction` → generic `Record` with `map[string]string` dimensions + `map[string]float64` measures |
| `analytics/engine.go` | `engine/executor.go` | `ExecuteQuerySpec` → `Execute`, accepts `[]Record` |
| `analytics/filters.go` | `engine/filters.go` | Dimension-based filtering (generic keys) |
| `analytics/aggregators.go` | `engine/aggregators.go` | Accept measure key parameter |
| `analytics/chart_builder.go` | `engine/builders/chart.go` | Minimal changes |
| `analytics/table_builder.go` | `engine/builders/table.go` | Minimal changes |
| `analytics/text_builder.go` | `engine/builders/text.go` | Minimal changes |

### Phase 2: Schema + Discovery (2 days)

- Implement `SchemaConfig` type and `Discover()` function
- Implement Tier 1 heuristic classification
- Implement `ParseRecords()` helpers for CSV/JSON
- Build schema-driven prompt builder for Translator (replaces hardcoded finance prompts)

### Phase 3: Translator Extraction (1-2 days)

| Current TPL File | Becomes | Key Change |
|---|---|---|
| `nlm.go` (prompt building) | `translator/gemini.go` | Schema-driven prompt builder, not finance-specific |
| `nlm.go` (response parsing) | `translator/parser.go` | Parse Gemini response → QuerySpec |
| `nlm.go` (handler logic) | Stays in TPL | Imports engine + translator, builds schema from user DB |

### Phase 4: TPL Integration (1 day)

- TPL imports `spektr` as a Go dependency
- TPL handler builds `SchemaConfig` from user's DB metadata (Tier 3)
- TPL handler fetches records, calls `engine.Query()`, returns result to frontend
- Zero changes to frontend — same output contract

### Phase 5: WASM Build + npm/pip Packages (2-3 days)

- Ensure core engine has no CGo dependencies
- Set up WASM build pipeline: `GOOS=js GOARCH=wasm go build`
- Create thin npm wrapper loading WASM
- Create thin pip wrapper loading WASM via wasmtime
- Build CLI binary with flag parsing

### Phase 6: Validation with Non-Finance Data (1-2 days)

- Test with Jira CSV export
- Test with e-commerce sample data
- Test with HR dataset
- Verify auto-discovery produces sensible schemas
- Verify AI translation works across domains

**Total estimated effort: ~10-12 days**

---

## 13. Open Design Questions

| # | Question | Recommendation |
|---|---|---|
| 1 | Computed measures (margin = revenue - cost)? | Defer to v2. Pre-compute in data source. |
| 2 | Custom transform plugins? | Fixed set for v1. Go plugin functions in v2. |
| 3 | Multi-source JOINs? | No. Single source per schema. Consumer pre-joins. |
| 4 | Secrets from vault/SSM? | Env vars only for v1. Consumer manages secrets. |
| 5 | Real-time streaming sources? | Not in scope. Batch analytics only for v1. |
| 6 | Multi-language AI summary? | Defer to Bhashini integration in TPL. Engine returns English. |
| 7 | Schema versioning? | `version` field in schema. Engine validates compatibility. |
| 8 | Caching of QuerySpec for repeated questions? | `QueryWithSpec()` enables this. Consumer manages cache. |
