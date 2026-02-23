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
