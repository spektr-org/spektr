package schema

// ============================================================================
// SCHEMA — Describes the shape of a dataset for the engine + AI translator
// ============================================================================
// Auto-discovered from data sources (Tier 1) or built by consumer apps (Tier 3).
// The translator uses schema metadata to build AI prompts.
// The engine uses schema for record parsing and measure/dimension resolution.
// ============================================================================

// Config describes the complete shape of a dataset.
type Config struct {
	Name        string          `json:"name"`
	Version     string          `json:"version,omitempty"`
	Description string          `json:"description,omitempty"`

	Dimensions []DimensionMeta  `json:"dimensions"`
	Measures   []MeasureMeta    `json:"measures"`

	// Optional: currency conversion settings
	Currency *CurrencyConfig   `json:"currency,omitempty"`

	// Auto-discovery metadata
	DiscoveredFrom string       `json:"discoveredFrom,omitempty"`
	DiscoveredAt   string       `json:"discoveredAt,omitempty"`

	// Columns skipped during auto-discovery
	SkippedColumns []SkippedColumn `json:"skippedColumns,omitempty"`
}

// DimensionMeta describes a string field used for grouping/filtering.
type DimensionMeta struct {
	Key            string   `json:"key"`
	DisplayName    string   `json:"displayName"`
	Description    string   `json:"description,omitempty"`
	SampleValues   []string `json:"sampleValues"`
	Groupable      bool     `json:"groupable"`
	Filterable     bool     `json:"filterable"`
	Parent         string   `json:"parent,omitempty"`       // Parent dimension key for hierarchies
	IsTemporal     bool     `json:"isTemporal,omitempty"`
	TemporalFormat string   `json:"temporalFormat,omitempty"`
	TemporalOrder  string   `json:"temporalOrder,omitempty"` // "chronological" or "reverse"
	IsCurrencyCode bool     `json:"isCurrencyCode,omitempty"`
	CardinalityHint string  `json:"cardinalityHint,omitempty"` // "low", "medium", "high"
	DerivedFrom    string   `json:"derivedFrom,omitempty"`     // Original column if auto-bucketed
}

// MeasureMeta describes a numeric field used for aggregation.
type MeasureMeta struct {
	Key                string   `json:"key"`
	DisplayName        string   `json:"displayName"`
	Description        string   `json:"description,omitempty"`
	Unit               string   `json:"unit,omitempty"` // "currency", "units", "hours", "points", "percent"
	IsCurrency         bool     `json:"isCurrency,omitempty"`
	IsSynthetic        bool     `json:"isSynthetic,omitempty"` // Auto-generated (e.g., record_count)
	Aggregations       []string `json:"aggregations,omitempty"`
	DefaultAggregation string   `json:"defaultAggregation,omitempty"`
	Format             string   `json:"format,omitempty"` // "#,##0.00", "0.0%"
}

// CurrencyConfig enables multi-currency normalization.
type CurrencyConfig struct {
	Enabled       bool               `json:"enabled"`
	CodeDimension string             `json:"codeDimension"` // Which dimension holds currency codes
	BaseCurrency  string             `json:"baseCurrency"`
	Rates         map[string]float64 `json:"rates"` // Foreign → base rate
}

// SkippedColumn records why a column was excluded during auto-discovery.
type SkippedColumn struct {
	Column      string `json:"column"`
	Reason      string `json:"reason"`
	Recoverable bool   `json:"recoverable"` // Can be restored if consumer overrides
}

// DefaultDimension creates a DimensionMeta with sensible defaults.
func DefaultDimension(key, displayName string, samples []string) DimensionMeta {
	return DimensionMeta{
		Key:         key,
		DisplayName: displayName,
		SampleValues: samples,
		Groupable:   true,
		Filterable:  true,
	}
}

// DefaultMeasure creates a MeasureMeta with sensible defaults.
func DefaultMeasure(key, displayName string) MeasureMeta {
	return MeasureMeta{
		Key:                key,
		DisplayName:        displayName,
		Aggregations:       []string{"sum", "avg", "min", "max", "count"},
		DefaultAggregation: "sum",
	}
}

// GetDefaultMeasure returns the first measure's key, or "amount" as fallback.
func (c Config) GetDefaultMeasure() string {
	if len(c.Measures) > 0 {
		return c.Measures[0].Key
	}
	return "amount"
}

// DimensionKeys returns all dimension keys.
func (c Config) DimensionKeys() []string {
	keys := make([]string, len(c.Dimensions))
	for i, d := range c.Dimensions {
		keys[i] = d.Key
	}
	return keys
}

// MeasureKeys returns all measure keys.
func (c Config) MeasureKeys() []string {
	keys := make([]string, len(c.Measures))
	for i, m := range c.Measures {
		keys[i] = m.Key
	}
	return keys
}
