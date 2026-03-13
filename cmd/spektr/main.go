package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/helpers"
	"github.com/spektr-org/spektr/schema"
	"github.com/spektr-org/spektr/translator"
	"github.com/spektr-org/spektr/translator/adapters"
)

// ============================================================================
// SPEKTR CLI — Picolytics for any dataset
// ============================================================================

const version = "0.2.0"

func main() {
	// ── Flags ─────────────────────────────────────────────────────────────
	filePath := flag.String("file", "", "Path to CSV data file (required)")
	queryStr := flag.String("query", "", "Natural language query to execute")
	schemaPath := flag.String("schema", "", "Path to pre-built schema JSON (skips auto-detect)")
	discover := flag.Bool("discover", false, "Print auto-detected schema and exit")
	refine := flag.Bool("refine", false, "Apply Smart Refine (AI enrichment) to auto-detected schema")
	model := flag.String("model", "", "AI model name (e.g. gemini-2.5-flash-lite, gpt-4o)")
	endpoint := flag.String("endpoint", "", "AI provider endpoint URL (default: Gemini)")
	format := flag.String("format", "json", "Output format: json, pretty, text, csv")
	outFile := flag.String("out", "", "Write output to file instead of stdout")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Spektr — Picolytics for any dataset

Usage:
  spektr --file data.csv --query "revenue by region" --format csv
  spektr --file data.csv --query "bugs by priority" --format csv --out results.csv
  spektr --file data.csv --discover --format pretty
  spektr --file data.csv --schema schema.json --query "total story points"

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Environment:
  AI_API_KEY    Required for --query and --refine

Formats:
  json      Full JSON output (default)
  pretty    Pretty-printed JSON
  text      Human-readable summary only
  csv       Chart/table data as CSV (ready for Sheets/Excel)

Examples:
  # Analyze and get CSV for Sheets
  spektr --file sales.csv --query "revenue by region" --format csv --out results.csv

  # Quick text answer
  spektr --file jira.csv --query "total story points" --format text

  # Save schema for reuse
  spektr --file data.csv --discover --refine --out schema.json --format pretty
`)
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("spektr %s\n", version)
		os.Exit(0)
	}

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --file is required")
		flag.Usage()
		os.Exit(1)
	}

	if !*discover && *queryStr == "" {
		fmt.Fprintln(os.Stderr, "Error: either --discover or --query is required")
		flag.Usage()
		os.Exit(1)
	}

	// ── Output writer ─────────────────────────────────────────────────────
	writer := os.Stdout
	if *outFile != "" {
		f, err := os.Create(*outFile)
		if err != nil {
			fatalf("Failed to create output file: %v", err)
		}
		defer f.Close()
		writer = f
	}

	// ── Read data ─────────────────────────────────────────────────────────
	data, err := os.ReadFile(*filePath)
	if err != nil {
		fatalf("Failed to read file: %v", err)
	}

	// ── Schema ────────────────────────────────────────────────────────────
	var sch *schema.Config

	if *schemaPath != "" {
		schemaData, err := os.ReadFile(*schemaPath)
		if err != nil {
			fatalf("Failed to read schema file: %v", err)
		}
		sch = &schema.Config{}
		if err := json.Unmarshal(schemaData, sch); err != nil {
			fatalf("Failed to parse schema JSON: %v", err)
		}
		log.Printf("📋 Loaded schema: %s (%d dimensions, %d measures)",
			sch.Name, len(sch.Dimensions), len(sch.Measures))
	} else {
		sch, err = schema.DiscoverFromCSV(data)
		if err != nil {
			fatalf("Auto-Detect failed: %v", err)
		}
		log.Printf("🔍 Auto-Detect: %s (%d dims, %d measures, %d skipped)",
			sch.Name, len(sch.Dimensions), len(sch.Measures), len(sch.SkippedColumns))

		if *refine {
			apiKey := os.Getenv("AI_API_KEY")
			if apiKey == "" {
				fatalf("AI_API_KEY required for --refine")
			}
			refined, err := schema.Refine(sch, schema.RefineConfig{APIKey: apiKey, Model: *model})
			if err != nil {
				log.Printf("⚠️ Smart Refine failed (using auto-detect): %v", err)
			} else {
				sch = refined
				log.Printf("🧠 Smart Refine: enriched → %s", sch.Name)
			}
		}
	}

	// ── Discover mode ─────────────────────────────────────────────────────
	if *discover {
		writeJSON(writer, sch, *format)
		if *outFile != "" {
			log.Printf("📄 Schema written to %s", *outFile)
		}
		return
	}

	// ── Query mode ────────────────────────────────────────────────────────
	apiKey := os.Getenv("AI_API_KEY")
	if apiKey == "" {
		fatalf("AI_API_KEY required for --query")
	}
	if *model == "" {
		fatalf("--model is required (e.g. gemini-2.5-flash-lite, gpt-4o)")
	}

	records, err := helpers.ParseCSV(data, *sch)
	if err != nil {
		fatalf("Failed to parse CSV records: %v", err)
	}
	log.Printf("📊 Parsed %d records", len(records))

	summary := translator.BuildDataSummaryFromRecords(records, *sch)

	t := translator.NewTranslator(adapters.New(apiKey, *model, *endpoint))
	result, err := t.TranslateWithSummary(*queryStr, *sch, summary)
	if err != nil {
		fatalf("Translation failed: %v", err)
	}
	log.Printf("🔄 Translated: intent=%s, visualize=%s, confidence=%.2f",
		result.QuerySpec.Intent, result.QuerySpec.Visualize, result.QuerySpec.Confidence)

	view := engine.NewSliceView(records)
	execResult, err := engine.Execute(result.QuerySpec, view,
		engine.WithDefaultMeasure(sch.GetDefaultMeasure()),
	)
	if err != nil {
		fatalf("Execution failed: %v", err)
	}

	// ── Render output ─────────────────────────────────────────────────────
	switch *format {
	case "csv":
		writeCSV(writer, execResult)
		if *outFile != "" {
			log.Printf("📄 CSV written to %s", *outFile)
		}
	case "text":
		lines := []string{}
		if result.Interpretation.Summary != "" {
			lines = append(lines, result.Interpretation.Summary)
		}
		if execResult != nil && execResult.Reply != "" {
			lines = append(lines, execResult.Reply)
		}
		if len(lines) > 0 {
			fmt.Fprintln(writer, strings.Join(lines, "\n"))
		} else {
			fmt.Fprintln(writer, "No result.")
		}
	default:
		out := cliOutput{
			Query:          *queryStr,
			Interpretation: result.Interpretation,
			QuerySpec:      result.QuerySpec,
			Result:         execResult,
		}
		writeJSON(writer, out, *format)
	}
}

// ============================================================================
// OUTPUT TYPES
// ============================================================================

type cliOutput struct {
	Query          string               `json:"query"`
	Interpretation engine.Interpretation `json:"interpretation"`
	QuerySpec      engine.QuerySpec     `json:"querySpec"`
	Result         *engine.Result       `json:"result"`
}

// ============================================================================
// CSV OUTPUT — The key feature: Spektr → Sheets-ready CSV
// ============================================================================

func writeCSV(w *os.File, result *engine.Result) {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if result == nil {
		cw.Write([]string{"Result", "No data"})
		return
	}

	// Try chart data first (most queries produce charts)
	if result.ChartConfig != nil && writeChartCSV(cw, result.ChartConfig) {
		return
	}

	// Then table data
	if result.TableData != nil {
		writeTableCSV(cw, result.TableData)
		return
	}

	// Fallback: text result as single-row CSV
	cw.Write([]string{"Summary", "Value", "Unit"})
	reply := result.Reply
	if reply == "" {
		reply = "No data"
	}
	cw.Write([]string{reply, "", result.DisplayUnit})
}

func writeChartCSV(cw *csv.Writer, chartConfig interface{}) bool {
	b, err := json.Marshal(chartConfig)
	if err != nil {
		return false
	}

	var chart struct {
		XAxis  string `json:"xAxis"`
		YAxis  string `json:"yAxis"`
		Series []struct {
			Name string `json:"name"`
			Data []struct {
				Label string  `json:"label"`
				Value float64 `json:"value"`
			} `json:"data"`
		} `json:"series"`
	}
	if err := json.Unmarshal(b, &chart); err != nil || len(chart.Series) == 0 {
		return false
	}

	xLabel := chart.XAxis
	yLabel := chart.YAxis
	if xLabel == "" {
		xLabel = "Label"
	}
	if yLabel == "" {
		yLabel = "Value"
	}

	// Single series → two columns
	if len(chart.Series) == 1 {
		cw.Write([]string{xLabel, yLabel})
		for _, d := range chart.Series[0].Data {
			cw.Write([]string{d.Label, fmtNum(d.Value)})
		}
		return true
	}

	// Multi-series → label + one column per series
	headers := []string{xLabel}
	for _, s := range chart.Series {
		headers = append(headers, s.Name)
	}
	cw.Write(headers)

	if len(chart.Series[0].Data) > 0 {
		for i, d := range chart.Series[0].Data {
			row := []string{d.Label}
			for _, s := range chart.Series {
				if i < len(s.Data) {
					row = append(row, fmtNum(s.Data[i].Value))
				} else {
					row = append(row, "")
				}
			}
			cw.Write(row)
		}
	}
	return true
}

func writeTableCSV(cw *csv.Writer, td *engine.TableData) {
	headers := make([]string, len(td.Columns))
	for i, col := range td.Columns {
		headers[i] = col.Label
	}
	cw.Write(headers)
	for _, row := range td.Rows {
		cw.Write(row)
	}
}

// ============================================================================
// JSON OUTPUT
// ============================================================================

func writeJSON(w *os.File, v interface{}, format string) {
	var out []byte
	var err error

	if format == "pretty" {
		out, err = json.MarshalIndent(v, "", "  ")
	} else {
		out, err = json.Marshal(v)
	}

	if err != nil {
		fatalf("Failed to marshal output: %v", err)
	}
	fmt.Fprintln(w, string(out))
}

// ============================================================================
// HELPERS
// ============================================================================

func fmtNum(v float64) string {
	// Whole numbers → no decimals, fractional → 2 decimals
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.2f", v)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}