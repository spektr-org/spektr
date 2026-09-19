package engine

// ============================================================================
// ENGINE OPTIONS — Functional options for Execute()
// ============================================================================

// Option configures engine behavior via functional options pattern.
type Option func(*config)

type config struct {
	BaseCurrency      string
	CurrencyDimension string             // dimension key holding currency codes
	ExchangeRates     map[string]float64 // foreign → base rate
	DefaultMeasure    string             // default measure key if QuerySpec.Measure is empty
	References        map[string]Reference
}

// WithCurrency configures multi-currency normalization.
// baseCurrency: target currency (e.g., "SGD")
// dimension: which dimension holds currency codes (e.g., "currency")
// rates: map of foreign currency → baseCurrency (e.g., {"INR": 0.016, "USD": 1.35})
func WithCurrency(baseCurrency, dimension string, rates map[string]float64) Option {
	return func(c *config) {
		c.BaseCurrency = baseCurrency
		c.CurrencyDimension = dimension
		c.ExchangeRates = rates
	}
}

// WithReferences supplies the dataset's plan values — contract value, budget,
// quota, goal — by name, for the "progress", "margin" and "difference"
// aggregations to refer to.
//
// References are NOT records. They never appear in counts, in "list", in a
// groupBy, or in period derivation. They are what the records are measured
// against, and the caller supplies them because the domain knows what its
// collection was promised; the engine does not.
//
// Placement note: a reference is a property of the DATASET, so it arguably
// belongs on the view beside the rows it qualifies rather than in execution
// config, where currency rates correctly sit because those govern how a result
// is rendered rather than what the data is. It is here because RecordView is an
// interface for zero-copy row access and one Execute analyses exactly one view,
// which makes the two placements behaviourally identical. They diverge the first
// time a caller executes across several collections in one call — which is when
// to move it, with a real case to shape the API.
func WithReferences(refs map[string]Reference) Option {
	return func(c *config) {
		c.References = refs
	}
}

// WithDefaultMeasure sets the measure to aggregate when QuerySpec.Measure is empty.
func WithDefaultMeasure(measure string) Option {
	return func(c *config) {
		c.DefaultMeasure = measure
	}
}

// applyOptions creates a config from functional options.
func applyOptions(opts []Option) *config {
	cfg := &config{
		DefaultMeasure: "amount", // sensible default for finance
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}