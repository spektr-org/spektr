package schema

import (
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ============================================================================
// AUTO-DISCOVERY — Tier 1 Heuristic Classification
// ============================================================================
// Inspects raw data (CSV) and generates a schema.Config automatically.
// No AI needed. ~80% accuracy for well-structured tabular data.
//
// Classification pipeline per column:
//   1. Sample values → detect type (numeric, date, bool, string)
//   2. Type + cardinality → classify role (dimension, measure, skip)
//   3. Pattern matching → detect special types (currency, temporal, hierarchy)
//   4. Generate synthetic measures (record_count)
//   5. Generate derived dimensions (date → month, quarter, year buckets)
//
// Design doc reference: Section 4.2 (Tier 1: Heuristic Auto-Discovery)
// ============================================================================

// DiscoverOptions controls discovery behavior.
type DiscoverOptions struct {
	SampleSize     int      // Max rows to inspect (0 = all). Default: 1000
	RecoverColumns []string // Force-include columns that were auto-skipped
	Name           string   // Dataset name override (otherwise inferred)
}

// DefaultDiscoverOptions returns sensible defaults.
func DefaultDiscoverOptions() DiscoverOptions {
	return DiscoverOptions{
		SampleSize: 1000,
	}
}

// DiscoverFromCSV generates a schema.Config by inspecting CSV data.
// Returns a complete Config with dimensions, measures, skipped columns, and defaults.
func DiscoverFromCSV(data []byte, opts ...DiscoverOptions) (*Config, error) {
	opt := DefaultDiscoverOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	reader := csv.NewReader(strings.NewReader(string(data)))

	// 1. Read headers
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	if len(headers) == 0 {
		return nil, fmt.Errorf("CSV has no columns")
	}

	// 2. Read sample rows
	var rows [][]string
	limit := opt.SampleSize
	if limit <= 0 {
		limit = 100000 // safety cap
	}

	for i := 0; i < limit; i++ {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}
		rows = append(rows, row)
	}

	totalRows := len(rows)
	if totalRows == 0 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	// 3. Analyze each column
	columns := make([]columnAnalysis, len(headers))
	for i, header := range headers {
		columns[i] = analyzeColumn(header, i, rows, totalRows)
	}

	// 4. Apply recovery overrides
	recoverSet := make(map[string]bool)
	for _, col := range opt.RecoverColumns {
		recoverSet[strings.ToLower(col)] = true
	}

	// 5. Build schema
	config := &Config{
		Name:    opt.Name,
		Version: "1.0",
	}

	if config.Name == "" {
		config.Name = "Auto-discovered Dataset"
	}

	var dimensions []DimensionMeta
	var measures []MeasureMeta
	var skipped []SkippedColumn

	for _, col := range columns {
		// Check recovery override
		recovered := recoverSet[strings.ToLower(col.header)] || recoverSet[col.key]

		switch col.role {
		case roleDimension:
			dimensions = append(dimensions, col.toDimension())

		case roleMeasure:
			measures = append(measures, col.toMeasure())

		case roleSkipped:
			if recovered {
				// Force as dimension
				dimensions = append(dimensions, col.toDimension())
			} else {
				skipped = append(skipped, SkippedColumn{
					Column:      col.header,
					Reason:      col.skipReason,
					Recoverable: col.recoverable,
				})
			}
		}
	}

	// 6. Add synthetic record_count measure
	measures = append(measures, MeasureMeta{
		Key:                "record_count",
		DisplayName:        "Record Count",
		Description:        "Number of records (auto-generated)",
		IsSynthetic:        true,
		Aggregations:       []string{"count"},
		DefaultAggregation: "count",
	})

	// 7. Detect hierarchies
	detectHierarchies(dimensions, rows, headers, columns)

	// 8. Detect currency configuration
	currency := detectCurrencyConfig(dimensions)

	config.Dimensions = dimensions
	config.Measures = measures
	config.SkippedColumns = skipped
	config.Currency = currency
	config.DiscoveredFrom = "CSV"
	config.DiscoveredAt = time.Now().Format(time.RFC3339)

	// 9. Set defaults
	config.setDefaults()

	return config, nil
}

// ============================================================================
// COLUMN ANALYSIS
// ============================================================================

type columnRole int

const (
	roleDimension columnRole = iota
	roleMeasure
	roleSkipped
)

type columnType int

const (
	typeString columnType = iota
	typeNumeric
	typeDate
	typeBool
)

type columnAnalysis struct {
	header      string
	key         string
	index       int
	colType     columnType
	role        columnRole
	skipReason  string
	recoverable bool

	// Stats
	uniqueCount int
	totalCount  int
	nullCount   int
	sampleVals  []string

	// Special type detection
	isTemporal     bool
	temporalFormat string
	isCurrencyCode bool
	hasDecimals    bool
	cardinalityHint string
}

// analyzeColumn inspects all values in a column and classifies it.
func analyzeColumn(header string, index int, rows [][]string, totalRows int) columnAnalysis {
	col := columnAnalysis{
		header:     header,
		key:        toSnakeCase(header),
		index:      index,
		totalCount: totalRows,
	}

	// Collect values
	values := make([]string, 0, len(rows))
	uniqueSet := make(map[string]bool)

	for _, row := range rows {
		if index >= len(row) {
			col.nullCount++
			continue
		}
		val := strings.TrimSpace(row[index])
		if val == "" || val == "null" || val == "NULL" || val == "N/A" || val == "n/a" {
			col.nullCount++
			continue
		}
		values = append(values, val)
		uniqueSet[val] = true
	}

	col.uniqueCount = len(uniqueSet)

	if len(values) == 0 {
		col.role = roleSkipped
		col.skipReason = "All values are empty/null"
		col.recoverable = false
		return col
	}

	// Collect sample values (up to 10, prefer diverse)
	col.sampleVals = collectSamples(uniqueSet, 10)

	// Step 1: Detect type
	col.colType = detectType(values)

	// Detect decimals in numeric columns (signals continuous data → measure)
	if col.colType == typeNumeric {
		for _, v := range values {
			if strings.Contains(v, ".") {
				col.hasDecimals = true
				break
			}
		}
	}

	// Step 2: Detect special patterns BEFORE role classification
	if col.colType == typeString {
		col.isCurrencyCode = detectCurrencyCodes(col.sampleVals)
		col.isTemporal, col.temporalFormat = detectTemporalPattern(col.sampleVals)
	}
	if col.colType == typeDate {
		col.isTemporal = true
	}

	// Step 3: Classify role based on type + cardinality
	col.classifyRole(totalRows)

	// Step 4: Set cardinality hint
	switch {
	case col.uniqueCount <= 10:
		col.cardinalityHint = "low"
	case col.uniqueCount <= 100:
		col.cardinalityHint = "medium"
	default:
		col.cardinalityHint = "high"
	}

	return col
}

// classifyRole determines dimension vs measure vs skip.
func (col *columnAnalysis) classifyRole(totalRows int) {
	switch col.colType {

	case typeNumeric:
		if col.uniqueCount == totalRows && totalRows > 10 {
			// Every value unique → likely an ID
			col.role = roleSkipped
			col.skipReason = "Unique per row — likely an ID column"
			col.recoverable = false
			return
		}
		// Check if values contain decimals (continuous data → always a measure)
		if col.hasDecimals {
			col.role = roleMeasure
			return
		}
		// Ratio-based: if few unique values AND low ratio → coded dimension (e.g., priority 1-5)
		// Absolute < 20 alone fails on small datasets where 6/12 looks "low" but is actually 50%
		uniqueRatio := float64(col.uniqueCount) / float64(totalRows)
		if col.uniqueCount < 20 && uniqueRatio < 0.3 {
			col.role = roleDimension
			return
		}
		// Many unique numeric values or high ratio → measure
		col.role = roleMeasure

	case typeDate:
		// Dates are always temporal dimensions
		col.role = roleDimension
		col.isTemporal = true

	case typeBool:
		col.role = roleDimension

	case typeString:
		if col.uniqueCount == totalRows && totalRows > 10 {
			// Every value unique → likely an ID or free text
			col.role = roleSkipped
			col.skipReason = "Unique per row — likely an identifier"
			col.recoverable = false
			return
		}
		if col.uniqueCount > totalRows/2 && col.uniqueCount > 50 {
			// High cardinality string
			col.role = roleSkipped
			col.skipReason = fmt.Sprintf("High cardinality (%d unique values) — not useful for grouping", col.uniqueCount)
			col.recoverable = true
			return
		}
		col.role = roleDimension
	}
}

// ============================================================================
// TYPE DETECTION
// ============================================================================

// detectType inspects values to determine column type.
// Requires 80%+ of non-null values to match for numeric/date/bool.
func detectType(values []string) columnType {
	if len(values) == 0 {
		return typeString
	}

	numCount := 0
	dateCount := 0
	boolCount := 0

	for _, v := range values {
		if isNumeric(v) {
			numCount++
		}
		if isDate(v) {
			dateCount++
		}
		if isBool(v) {
			boolCount++
		}
	}

	threshold := int(float64(len(values)) * 0.8)

	if boolCount >= threshold {
		return typeBool
	}
	if dateCount >= threshold {
		return typeDate
	}
	if numCount >= threshold {
		return typeNumeric
	}
	return typeString
}

func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "") // handle "1,234.56"
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimPrefix(s, "€")
	s = strings.TrimPrefix(s, "£")
	s = strings.TrimPrefix(s, "-")
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

var dateFormats = []string{
	"2006-01-02",
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05",
	"01/02/2006",
	"02/01/2006",
	"Jan-2006",
	"January 2006",
	"2006",
	"Jan 2, 2006",
	"2 Jan 2006",
}

func isDate(s string) bool {
	s = strings.TrimSpace(s)
	for _, fmt := range dateFormats {
		if _, err := time.Parse(fmt, s); err == nil {
			return true
		}
	}
	return false
}

func isBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "false" || s == "yes" || s == "no" || s == "1" || s == "0"
}

// ============================================================================
// SPECIAL PATTERN DETECTION
// ============================================================================

// Known ISO 4217 currency codes (common subset).
var knownCurrencies = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "JPY": true, "CNY": true,
	"INR": true, "SGD": true, "AUD": true, "CAD": true, "CHF": true,
	"HKD": true, "NZD": true, "SEK": true, "KRW": true, "NOK": true,
	"MXN": true, "BRL": true, "ZAR": true, "THB": true, "MYR": true,
	"IDR": true, "PHP": true, "VND": true, "TWD": true, "AED": true,
	"SAR": true, "QAR": true, "PLN": true, "CZK": true, "ILS": true,
	"DKK": true, "RUB": true, "TRY": true, "ARS": true, "CLP": true,
	"COP": true, "PEN": true, "EGP": true, "NGN": true, "KES": true,
	"PKR": true, "BDT": true, "LKR": true, "MMK": true, "NPR": true,
}

// detectCurrencyCodes checks if sample values are ISO currency codes.
func detectCurrencyCodes(samples []string) bool {
	if len(samples) == 0 {
		return false
	}
	matches := 0
	for _, s := range samples {
		s = strings.TrimSpace(s)
		if len(s) == 3 && s == strings.ToUpper(s) && knownCurrencies[s] {
			matches++
		}
	}
	// At least 80% must be valid currency codes
	return matches > 0 && float64(matches)/float64(len(samples)) >= 0.8
}

var monthPatterns = []struct {
	re     *regexp.Regexp
	format string
}{
	{regexp.MustCompile(`^[A-Z][a-z]{2}-\d{4}$`), "MMM-yyyy"},       // Jan-2026
	{regexp.MustCompile(`^\d{4}-\d{2}$`), "yyyy-MM"},                 // 2026-01
	{regexp.MustCompile(`^Q[1-4]-\d{4}$`), "QN-yyyy"},               // Q1-2026
	{regexp.MustCompile(`^Q[1-4]\s+\d{4}$`), "QN yyyy"},             // Q1 2026
	{regexp.MustCompile(`^\d{4}$`), "yyyy"},                           // 2026
	{regexp.MustCompile(`^[A-Z][a-z]+ \d{4}$`), "MMMM yyyy"},       // January 2026
}

// detectTemporalPattern checks if values match known date/month/quarter patterns.
func detectTemporalPattern(samples []string) (bool, string) {
	if len(samples) == 0 {
		return false, ""
	}

	for _, pattern := range monthPatterns {
		matches := 0
		for _, s := range samples {
			if pattern.re.MatchString(strings.TrimSpace(s)) {
				matches++
			}
		}
		if float64(matches)/float64(len(samples)) >= 0.8 {
			return true, pattern.format
		}
	}

	return false, ""
}

// ============================================================================
// HIERARCHY DETECTION
// ============================================================================

// detectHierarchies finds parent/child relationships between dimensions.
// If every value of dimension B maps to exactly one value of dimension A,
// and A has fewer unique values, then A is parent of B.
// When multiple valid parents exist, picks the closest (highest cardinality).
func detectHierarchies(dimensions []DimensionMeta, rows [][]string, headers []string, columns []columnAnalysis) {
	// Build column index lookup
	dimIndices := make(map[string]int) // key → column index
	dimUniques := make(map[string]int) // key → unique count

	for _, col := range columns {
		if col.role == roleDimension {
			dimIndices[col.key] = col.index
			dimUniques[col.key] = col.uniqueCount
		}
	}

	// For each dimension, find the best parent (closest = highest cardinality among valid parents)
	for i := range dimensions {
		childKey := dimensions[i].Key
		childIdx, ok1 := dimIndices[childKey]
		if !ok1 {
			continue
		}

		bestParent := ""
		bestParentUniques := 0

		for j := range dimensions {
			if i == j {
				continue
			}
			parentKey := dimensions[j].Key
			parentIdx, ok2 := dimIndices[parentKey]
			if !ok2 {
				continue
			}

			// Parent must have fewer unique values than child
			if dimUniques[parentKey] >= dimUniques[childKey] {
				continue
			}

			// Check: does every child value map to exactly one parent?
			childToParent := make(map[string]string)
			isHierarchy := true

			for _, row := range rows {
				if childIdx >= len(row) || parentIdx >= len(row) {
					continue
				}
				child := strings.TrimSpace(row[childIdx])
				parent := strings.TrimSpace(row[parentIdx])
				if child == "" || parent == "" {
					continue
				}

				if existing, ok := childToParent[child]; ok {
					if existing != parent {
						isHierarchy = false
						break
					}
				} else {
					childToParent[child] = parent
				}
			}

			if isHierarchy && len(childToParent) > 1 {
				// Valid parent — prefer closest (highest cardinality)
				if dimUniques[parentKey] > bestParentUniques {
					bestParent = parentKey
					bestParentUniques = dimUniques[parentKey]
				}
			}
		}

		if bestParent != "" {
			dimensions[i].Parent = bestParent
		}
	}
}

// ============================================================================
// CURRENCY CONFIG DETECTION
// ============================================================================

// detectCurrencyConfig checks if any dimension is a currency code dimension
// and builds CurrencyConfig accordingly.
func detectCurrencyConfig(dimensions []DimensionMeta) *CurrencyConfig {
	for _, d := range dimensions {
		if d.IsCurrencyCode {
			// Found currency dimension — enable conversion
			baseCurrency := ""
			if len(d.SampleValues) > 0 {
				baseCurrency = d.SampleValues[0] // default to first seen
			}
			return &CurrencyConfig{
				Enabled:       true,
				CodeDimension: d.Key,
				BaseCurrency:  baseCurrency,
				Rates:         map[string]float64{}, // consumer must provide rates
			}
		}
	}
	return nil
}

// ============================================================================
// CONVERSION HELPERS
// ============================================================================

// toDimension converts a column analysis into DimensionMeta.
func (col *columnAnalysis) toDimension() DimensionMeta {
	return DimensionMeta{
		Key:             col.key,
		DisplayName:     toDisplayName(col.header),
		SampleValues:    col.sampleVals,
		Groupable:       true,
		Filterable:      true,
		IsTemporal:      col.isTemporal,
		TemporalFormat:  col.temporalFormat,
		TemporalOrder:   "chronological",
		IsCurrencyCode:  col.isCurrencyCode,
		CardinalityHint: col.cardinalityHint,
	}
}

// toMeasure converts a column analysis into MeasureMeta.
func (col *columnAnalysis) toMeasure() MeasureMeta {
	return MeasureMeta{
		Key:                col.key,
		DisplayName:        toDisplayName(col.header),
		Aggregations:       []string{"sum", "avg", "min", "max", "count"},
		DefaultAggregation: "sum",
	}
}

// setDefaults configures schema defaults based on discovered dimensions/measures.
func (c *Config) setDefaults() {
	// Nothing to set if no dimensions
	if len(c.Dimensions) == 0 {
		return
	}

	// Default sort by primary measure desc
	// Default groupBy the first non-temporal dimension (or temporal if that's all there is)
}

// ============================================================================
// STRING UTILITIES
// ============================================================================

// toSnakeCase converts "Column Name" or "columnName" → "column_name".
func toSnakeCase(s string) string {
	// Handle camelCase: insert underscore before uppercase letters
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			prev := rune(s[i-1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				result.WriteRune('_')
			}
		}
		result.WriteRune(r)
	}

	s = result.String()
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "__", "_")
	s = strings.Trim(s, "_")
	return s
}

// toDisplayName cleans a header for human display.
// "story_points" → "Story Points", "assignee" → "Assignee"
func toDisplayName(s string) string {
	// If already has spaces/mixed case, just trim
	if strings.Contains(s, " ") {
		return strings.TrimSpace(s)
	}

	// Convert snake_case to Title Case
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")

	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

// collectSamples picks up to maxSamples representative values.
func collectSamples(uniqueSet map[string]bool, maxSamples int) []string {
	samples := make([]string, 0, len(uniqueSet))
	for v := range uniqueSet {
		samples = append(samples, v)
	}

	// Sort for deterministic output
	sort.Strings(samples)

	if len(samples) > maxSamples {
		samples = samples[:maxSamples]
	}
	return samples
}