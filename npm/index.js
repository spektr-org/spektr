// @spektr/engine — JavaScript wrapper for Spektr WASM
// Works in Node.js and browser environments.

"use strict";

const path = require("path");
const fs = require("fs");

let initialized = false;
let initPromise = null;

/**
 * Initialize Spektr WASM engine.
 * Automatically loads wasm_exec.js and spektr.wasm.
 * Safe to call multiple times — only initializes once.
 */
async function init() {
  if (initialized) return;
  if (initPromise) return initPromise;

  initPromise = _doInit();
  await initPromise;
  initialized = true;
}

async function _doInit() {
  // Load Go's wasm_exec.js runtime
  const wasmExecPath = path.join(__dirname, "wasm_exec.js");
  require(wasmExecPath);

  const go = new Go();

  // Load WASM binary
  const wasmPath = path.join(__dirname, "spektr.wasm");
  const wasmBuffer = fs.readFileSync(wasmPath);
  const wasmModule = await WebAssembly.compile(wasmBuffer);
  const instance = await WebAssembly.instantiate(wasmModule, go.importObject);

  // Start Go runtime (runs main(), registers __spektr on globalThis)
  // Don't await — go.run() blocks until the Go program exits (which is never for WASM)
  go.run(instance);

  // Wait for ready signal
  await new Promise((resolve) => {
    if (globalThis.__spektr) {
      resolve();
    } else {
      globalThis.__spektrReady = resolve;
    }
  });
}

function ensureInit() {
  if (!initialized) {
    throw new Error("Spektr not initialized. Call await spektr.init() first.");
  }
}

/**
 * Auto-detect schema from CSV data.
 * @param {string} csv - CSV string
 * @returns {{ ok: boolean, data?: object, error?: string }}
 */
function discover(csv) {
  ensureInit();
  return globalThis.__spektr.discover(csv);
}

/**
 * Enrich schema with AI (one-time Gemini call).
 * @param {object} schema - Schema config object
 * @param {string} apiKey - Gemini API key
 * @param {string} [model] - Gemini model name
 * @returns {{ ok: boolean, data?: object, error?: string }}
 */
function refine(schema, apiKey, model) {
  ensureInit();
  return globalThis.__spektr.refine(JSON.stringify(schema), apiKey, model);
}

/**
 * Execute a QuerySpec against records.
 * @param {object} spec - QuerySpec object
 * @param {Array} records - Array of { dimensions: {}, measures: {} }
 * @param {object} [options] - { defaultMeasure, baseCurrency, exchangeRates }
 * @returns {{ ok: boolean, data?: object, error?: string }}
 */
function execute(spec, records, options) {
  ensureInit();
  return globalThis.__spektr.execute(
    JSON.stringify(spec),
    JSON.stringify(records),
    options ? JSON.stringify(options) : undefined
  );
}

/**
 * Translate natural language to QuerySpec using Gemini.
 * @param {string} query - Natural language query
 * @param {object} schema - Schema config
 * @param {object} summary - Data summary { recordCount, dimensions }
 * @param {string} apiKey - Gemini API key
 * @param {string} [model] - Gemini model name
 * @returns {{ ok: boolean, data?: object, error?: string }}
 */
function translate(query, schema, summary, apiKey, model) {
  ensureInit();
  return globalThis.__spektr.translate(
    query,
    JSON.stringify(schema),
    JSON.stringify(summary),
    apiKey,
    model
  );
}

/**
 * Parse CSV string into records using a schema.
 * @param {string} csv - CSV string
 * @param {object} schema - Schema config
 * @returns {{ ok: boolean, data?: Array, error?: string }}
 */
function parseCSV(csv, schema) {
  ensureInit();
  return globalThis.__spektr.parseCSV(csv, JSON.stringify(schema));
}

/**
 * Get Spektr WASM version.
 * @returns {{ ok: boolean, data?: string, error?: string }}
 */
function version() {
  ensureInit();
  return globalThis.__spektr.version();
}

module.exports = {
  init,
  discover,
  refine,
  execute,
  translate,
  parseCSV,
  version,
};