package helpers

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ketank3007/spektr/engine"
	"github.com/ketank3007/spektr/schema"
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
				if f, err := strconv.ParseFloat(val, 64); err == nil {
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

			// Try numeric first
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				rec.Measures[keys[i]] = f
			} else {
				rec.Dimensions[keys[i]] = val
			}
		}

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

// toSnakeCase converts "Column Name" → "column_name".
func toSnakeCase(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}