package helpers

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/schema"
)

// ============================================================================
// CSV HELPER — Parses CSV data into []engine.Record
// ============================================================================
// Consumer reads the CSV from wherever it lives (file, S3, Sheets).
// This helper converts the raw bytes into generic Records using the schema.
// ============================================================================

// ParseCSV parses CSV bytes into Records using schema for classification.
// Each row becomes a Record with dimensions (string) and measures (numeric).
func ParseCSV(data []byte, sch schema.Config) ([]engine.Record, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))

	// Read header
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	// Build column index → schema mapping
	dimSet := make(map[string]bool)
	for _, d := range sch.Dimensions {
		dimSet[d.Key] = true
	}
	measSet := make(map[string]bool)
	for _, m := range sch.Measures {
		if !m.IsSynthetic {
			measSet[m.Key] = true
		}
	}

	type colMapping struct {
		schemaKey string
		isDimension bool
		isMeasure   bool
	}

	mappings := make([]colMapping, len(headers))
	for i, h := range headers {
		key := toSnakeCase(strings.TrimSpace(h))
		if dimSet[key] {
			mappings[i] = colMapping{schemaKey: key, isDimension: true}
		} else if measSet[key] {
			mappings[i] = colMapping{schemaKey: key, isMeasure: true}
		}
		// Unmapped columns are silently skipped
	}

	// Read rows
	var records []engine.Record
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}

		rec := engine.Record{
			Dimensions: make(map[string]string),
			Measures:   make(map[string]float64),
		}

		for i, val := range row {
			if i >= len(mappings) {
				break
			}
			m := mappings[i]
			val = strings.TrimSpace(val)

			if m.isDimension {
				rec.Dimensions[m.schemaKey] = val
			} else if m.isMeasure {
				if f, ok := cleanAndParseNumeric(val); ok {
					rec.Measures[m.schemaKey] = f
				}
			}
		}

		// Add synthetic measures (e.g., record_count)
		for _, m := range sch.Measures {
			if m.IsSynthetic && m.DefaultAggregation == "count" {
				rec.Measures[m.Key] = 1
			}
		}

		records = append(records, rec)
	}

	return records, nil
}

// ParseCSVAuto parses CSV without a pre-existing schema.
// Returns both the discovered records and inferred column info.
// Consumers can use this for quick demos before refining the schema.
func ParseCSVAuto(data []byte) ([]engine.Record, []string, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))

	headers, err := reader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	keys := make([]string, len(headers))
	for i, h := range headers {
		keys[i] = toSnakeCase(strings.TrimSpace(h))
	}

	var records []engine.Record
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		rec := engine.Record{
			Dimensions: make(map[string]string),
			Measures:   make(map[string]float64),
		}

		for i, val := range row {
			if i >= len(keys) {
				break
			}
			val = strings.TrimSpace(val)

			// Try numeric first (handles %, commas, duration suffixes)
			if f, ok := cleanAndParseNumeric(val); ok {
				rec.Measures[keys[i]] = f
			} else {
				rec.Dimensions[keys[i]] = val
			}
		}

		// Always inject record_count so engine has something to aggregate
		// even when all columns are non-numeric (e.g. user/event exports)
		rec.Measures["record_count"] = 1

		records = append(records, rec)
	}

	return records, keys, nil
}

// ParseCSVView parses CSV into a RecordView (convenience wrapper).
// Consumers who don't need []Record can use this directly.
func ParseCSVView(data []byte, sch schema.Config) (engine.RecordView, error) {
	records, err := ParseCSV(data, sch)
	if err != nil {
		return nil, err
	}
	return engine.NewSliceView(records), nil
}

// ParseCSVAutoView parses CSV without schema and returns a RecordView.
func ParseCSVAutoView(data []byte) (engine.RecordView, []string, error) {
	records, keys, err := ParseCSVAuto(data)
	if err != nil {
		return nil, nil, err
	}
	return engine.NewSliceView(records), keys, nil
}

// cleanAndParseNumeric attempts to extract a numeric value from a string that
// may contain common formatting: percentages ("72.2%"), comma separators
// ("1,234.56"), and duration suffixes ("2.37 s", "794 ms", "4.54 mins").
//
// Duration normalization converts to milliseconds:
//   "794 ms"    → 794
//   "2.37 s"    → 2370
//   "4.54 mins" → 272400
//   "1.2 hrs"   → 4320000
//
// Returns the parsed value and true, or 0 and false if unparseable.
func cleanAndParseNumeric(val string) (float64, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, false
	}

	// Fast path: try raw parse first (covers clean numbers)
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f, true
	}

	// Strip commas (thousands separators): "1,234.56" → "1234.56"
	cleaned := strings.ReplaceAll(val, ",", "")
	if f, err := strconv.ParseFloat(cleaned, 64); err == nil {
		return f, true
	}

	// Percentage: "72.2%" → 72.2
	if strings.HasSuffix(cleaned, "%") {
		numStr := strings.TrimSuffix(cleaned, "%")
		numStr = strings.TrimSpace(numStr)
		if f, err := strconv.ParseFloat(numStr, 64); err == nil {
			return f, true
		}
	}

	// Duration suffixes — normalize to milliseconds
	// Order matters: check longer suffixes first to avoid "mins" matching "ms" partial
	lower := strings.ToLower(cleaned)
	durationSuffixes := []struct {
		suffix     string
		multiplier float64
	}{
		{"mins", 60000},
		{"min", 60000},
		{"hrs", 3600000},
		{"hr", 3600000},
		{"ms", 1},
		{"s", 1000},
	}

	for _, ds := range durationSuffixes {
		if strings.HasSuffix(lower, ds.suffix) {
			numStr := strings.TrimSpace(cleaned[:len(cleaned)-len(ds.suffix)])
			if f, err := strconv.ParseFloat(numStr, 64); err == nil {
				return f * ds.multiplier, true
			}
		}
	}

	return 0, false
}

// toSnakeCase converts "Column Name" or "camelCase" → "snake_case".
// Must stay in sync with schema/discover.go toSnakeCase — both files produce
// schema keys and they must agree on the output for every header string.
func toSnakeCase(s string) string {
	// Handle camelCase: insert underscore before each uppercase letter
	// that follows a lowercase letter or digit (e.g. playbookId → playbook_Id)
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' && i > 0 {
			prev := rune(s[i-1])
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				result.WriteRune('_')
			}
		}
		result.WriteRune(r)
	}

	s = result.String()
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "__", "_")
	s = strings.Trim(s, "_")
	return s
}