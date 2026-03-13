// +build js,wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/spektr-org/spektr/engine"
	"github.com/spektr-org/spektr/helpers"
	"github.com/spektr-org/spektr/schema"
	"github.com/spektr-org/spektr/translator"
	"github.com/spektr-org/spektr/translator/adapters"
)

// ============================================================================
// SPEKTR WASM BRIDGE
// ============================================================================
// Exposes Spektr's core functions to JavaScript/TypeScript.
//
// Registered functions (on globalThis.__spektr):
//
//   __spektr.discover(csvString)
//     → JSON schema config
//
//   __spektr.refine(schemaJSON, apiKey, model)
//     → JSON enriched schema (makes HTTP call to Gemini)
//
//   __spektr.execute(querySpecJSON, recordsJSON, optionsJSON)
//     → JSON result (chart/table/text)
//
//   __spektr.translate(query, schemaJSON, summaryJSON, apiKey, model, [endpoint])
//     → JSON { querySpec, interpretation }
//
//   __spektr.parseCSV(csvString, schemaJSON)
//     → JSON records array
//
//   __spektr.version()
//     → version string
//
// All functions return { ok: true, data: ... } or { ok: false, error: "..." }
// ============================================================================

const wasmVersion = "0.2.0"

func main() {
	// Create namespace object
	ns := js.Global().Get("Object").New()

	ns.Set("discover", js.FuncOf(jsDiscover))
	ns.Set("refine", js.FuncOf(jsRefine))
	ns.Set("execute", js.FuncOf(jsExecute))
	ns.Set("translate", js.FuncOf(jsTranslate))
	ns.Set("parseCSV", js.FuncOf(jsParseCSV))
	ns.Set("version", js.FuncOf(jsVersion))

	js.Global().Set("__spektr", ns)

	// Signal ready
	readyCb := js.Global().Get("__spektrReady")
	if !readyCb.IsUndefined() && !readyCb.IsNull() {
		readyCb.Invoke()
	}

	// Keep WASM alive
	select {}
}

// ============================================================================
// DISCOVER — CSV → Schema
// ============================================================================

func jsDiscover(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return errResult("discover requires 1 argument: csvString")
	}

	csvStr := args[0].String()
	config, err := schema.DiscoverFromCSV([]byte(csvStr))
	if err != nil {
		return errResult(fmt.Sprintf("discover failed: %v", err))
	}

	return okResult(config)
}

// ============================================================================
// REFINE — Schema + AI → Enriched Schema
// ============================================================================

func jsRefine(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return newRejectedPromise("refine requires 2-3 arguments: schemaJSON, apiKey, [model]")
	}

	var config schema.Config
	if err := json.Unmarshal([]byte(args[0].String()), &config); err != nil {
		return newRejectedPromise(fmt.Sprintf("invalid schema JSON: %v", err))
	}

	apiKey := args[1].String()
	model := ""
	if len(args) > 2 && !args[2].IsUndefined() {
		model = args[2].String()
	}
	if model == "" {
		return newRejectedPromise("refine requires model (e.g. gemini-2.5-flash, gpt-4o)")
	}

	cfgCopy := config
	cfg := schema.RefineConfig{APIKey: apiKey, Model: model}

	return newPromise(func(resolve, reject func(interface{})) {
		refined, err := schema.Refine(&cfgCopy, cfg)
		if err != nil {
			resolve(errResult(fmt.Sprintf("refine failed: %v", err)))
			return
		}
		resolve(okResult(refined))
	})
}

// ============================================================================
// EXECUTE — QuerySpec + Records → Result
// ============================================================================

func jsExecute(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return errResult("execute requires 2-3 arguments: querySpecJSON, recordsJSON, [optionsJSON]")
	}

	// Parse QuerySpec
	var spec engine.QuerySpec
	if err := json.Unmarshal([]byte(args[0].String()), &spec); err != nil {
		return errResult(fmt.Sprintf("invalid querySpec JSON: %v", err))
	}

	// Parse Records
	var records []engine.Record
	if err := json.Unmarshal([]byte(args[1].String()), &records); err != nil {
		return errResult(fmt.Sprintf("invalid records JSON: %v", err))
	}

	// Parse options
	opts := []engine.Option{}
	if len(args) > 2 && !args[2].IsUndefined() {
		var options struct {
			DefaultMeasure    string             `json:"defaultMeasure"`
			BaseCurrency      string             `json:"baseCurrency"`
			CurrencyDimension string             `json:"currencyDimension"`
			ExchangeRates     map[string]float64 `json:"exchangeRates"`
		}
		if err := json.Unmarshal([]byte(args[2].String()), &options); err == nil {
			if options.DefaultMeasure != "" {
				opts = append(opts, engine.WithDefaultMeasure(options.DefaultMeasure))
			}
			if options.BaseCurrency != "" && len(options.ExchangeRates) > 0 { 
				dim := options.CurrencyDimension
				if dim == "" {
					dim = "currency"
				}
				opts = append(opts, engine.WithCurrency(options.BaseCurrency, dim, options.ExchangeRates))
			}
			
		}
	}
	
	
	

	// Execute
	view := engine.NewSliceView(records)
	result, err := engine.Execute(spec, view, opts...)
	if err != nil {
		return errResult(fmt.Sprintf("execute failed: %v", err))
	}

	return okResult(result)
}

// ============================================================================
// TRANSLATE — NL Query + Schema → QuerySpec
// ============================================================================

func jsTranslate(this js.Value, args []js.Value) interface{} {
	if len(args) < 4 {
		return newRejectedPromise("translate requires 4-6 arguments: query, schemaJSON, summaryJSON, apiKey, model, [endpoint]")
	}

	query := args[0].String()

	var sch schema.Config
	if err := json.Unmarshal([]byte(args[1].String()), &sch); err != nil {
		return newRejectedPromise(fmt.Sprintf("invalid schema JSON: %v", err))
	}

	var summary translator.DataSummary
	if err := json.Unmarshal([]byte(args[2].String()), &summary); err != nil {
		return newRejectedPromise(fmt.Sprintf("invalid summary JSON: %v", err))
	}

	apiKey := args[3].String()
	if apiKey == "" {
		return newRejectedPromise("translate requires apiKey")
	}

	if len(args) < 5 || args[4].IsUndefined() || args[4].String() == "" {
		return newRejectedPromise("translate requires model (e.g. gemini-2.5-flash, gpt-4o)")
	}
	model := args[4].String()

	endpoint := ""
	if len(args) > 5 && !args[5].IsUndefined() {
		endpoint = args[5].String()
	}

	t := translator.NewTranslator(adapters.New(apiKey, model, endpoint))

	return newPromise(func(resolve, reject func(interface{})) {
		result, err := t.TranslateWithSummary(query, sch, &summary)
		if err != nil {
			resolve(errResult(fmt.Sprintf("translate failed: %v", err)))
			return
		}
		resolve(okResult(result))
	})
}

// ============================================================================
// PARSE CSV — CSV + Schema → Records
// ============================================================================

func jsParseCSV(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return errResult("parseCSV requires 2 arguments: csvString, schemaJSON")
	}

	var sch schema.Config
	if err := json.Unmarshal([]byte(args[1].String()), &sch); err != nil {
		return errResult(fmt.Sprintf("invalid schema JSON: %v", err))
	}

	records, err := helpers.ParseCSV([]byte(args[0].String()), sch)
	if err != nil {
		return errResult(fmt.Sprintf("parseCSV failed: %v", err))
	}

	return okResult(records)
}

// ============================================================================
// VERSION
// ============================================================================

func jsVersion(this js.Value, args []js.Value) interface{} {
	return okResult(wasmVersion)
}

// ============================================================================
// HELPERS — JSON bridge
// ============================================================================


// ============================================================================
// PROMISE HELPERS — Required for async HTTP calls in WASM
// ============================================================================

// newPromise runs fn in a goroutine and returns a JS Promise.
// fn must call resolve(value) exactly once.
func newPromise(fn func(resolve, reject func(interface{}))) js.Value {
	var resolve, reject js.Func
	promise := js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resolve = js.FuncOf(func(this js.Value, pArgs []js.Value) interface{} {
			if len(pArgs) > 0 {
				args[0].Invoke(pArgs[0])
			}
			return nil
		})
		reject = js.FuncOf(func(this js.Value, pArgs []js.Value) interface{} {
			if len(pArgs) > 0 {
				args[1].Invoke(pArgs[0])
			}
			return nil
		})
		go func() {
			defer resolve.Release()
			defer reject.Release()
			fn(
				func(v interface{}) { resolve.Invoke(v) },
				func(v interface{}) { reject.Invoke(v) },
			)
		}()
		return nil
	}))
	return promise
}

// newRejectedPromise returns a Promise that resolves immediately with an error result.
// (We resolve rather than reject so callers always get { ok, error } shape.)
func newRejectedPromise(msg string) js.Value {
	return newPromise(func(resolve, reject func(interface{})) {
		resolve(errResult(msg))
	})
}

func okResult(data interface{}) interface{} {
	b, err := json.Marshal(data)
	if err != nil {
		return errResult(fmt.Sprintf("marshal failed: %v", err))
	}
	result := js.Global().Get("Object").New()
	result.Set("ok", true)
	result.Set("data", js.Global().Get("JSON").Call("parse", string(b)))
	return result
}

func errResult(msg string) interface{} {
	result := js.Global().Get("Object").New()
	result.Set("ok", false)
	result.Set("error", msg)
	return result
}