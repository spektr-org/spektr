// Package spektr provides a domain-agnostic analytics engine.
// Picolytics for any dataset.
//
// Usage:
//
//	import "github.com/ketank3007/spektr/engine"
//
//	result, err := engine.Execute(querySpec, records,
//	    engine.WithDefaultMeasure("amount"),
//	    engine.WithCurrency("SGD", "currency", rates),
//	)
//
// The engine takes a QuerySpec (produced by an AI translator) and records
// (generic dimension/measure maps), and returns render-ready output
// (chart config, table data, or text summary).
//
// AI translation is handled separately by the translator package.
// The engine never calls any external service — all computation is local.
package spektr
