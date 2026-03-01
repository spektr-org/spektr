// Type definitions for @spektr/engine

export interface SpektrResult<T> {
  ok: boolean;
  data?: T;
  error?: string;
}

export interface SchemaConfig {
  name: string;
  version: string;
  dimensions: DimensionMeta[];
  measures: MeasureMeta[];
  skippedColumns?: SkippedColumn[];
  currency?: CurrencyConfig;
  discoveredFrom?: string;
  discoveredAt?: string;
  refinedAt?: string;
  refinedBy?: string;
}

export interface DimensionMeta {
  key: string;
  displayName: string;
  description?: string;
  sampleValues: string[];
  groupable: boolean;
  filterable: boolean;
  parent?: string;
  isTemporal?: boolean;
  isCurrencyCode?: boolean;
  sortHint?: string;
  temporalOrder?: string;
  cardinalityHint?: string;
}

export interface MeasureMeta {
  key: string;
  displayName: string;
  description?: string;
  unit?: string;
  isCurrency?: boolean;
  isSynthetic?: boolean;
  aggregations: string[];
  defaultAggregation: string;
}

export interface SkippedColumn {
  column: string;
  reason: string;
  recoverable: boolean;
}

export interface CurrencyConfig {
  enabled: boolean;
  codeDimension: string;
  defaultCurrency?: string;
  rates?: Record<string, number>;
}

export interface QuerySpec {
  intent: string;
  filters: Filters;
  compareFilters?: Filters;
  aggregation: string;
  measure: string;
  groupBy: string[];
  sortBy: string;
  limit: number;
  visualize: string;
  title: string;
  reply: string;
  confidence: number;
}

export interface Filters {
  dimensions: Record<string, string[]>;
}

export interface Record {
  dimensions: Record<string, string>;
  measures: Record<string, number>;
}

export interface ExecuteOptions {
  defaultMeasure?: string;
  baseCurrency?: string;
  currencyDimension?: string;
  exchangeRates?: Record<string, number>;
}

export interface EngineResult {
  success: boolean;
  type: string;
  reply: string;
  chartConfig?: ChartConfig;
  tableData?: TableData;
  data?: any;
  displayUnit?: string;
  shouldConvert?: boolean;
}

export interface ChartConfig {
  chartType: string;
  title: string;
  xAxis: string;
  yAxis: string;
  series: ChartSeries[];
  colors?: string[];
  showLegend: boolean;
  showGrid: boolean;
}

export interface ChartSeries {
  name: string;
  data: { label: string; value: number }[];
}

export interface TableData {
  title: string;
  columns: { key: string; label: string; type: string; align: string }[];
  rows: string[][];
  summary?: { label: string; values: Record<string, string> };
}

export interface DataSummary {
  recordCount: number;
  dimensions: Record<string, string[]>;
}

export interface TranslateResult {
  querySpec: QuerySpec;
  interpretation: {
    visualType: string;
    summary: string;
    confidence: number;
  };
}

/**
 * Initialize Spektr WASM engine.
 * Must be called before using any other functions.
 */
export function init(): Promise<void>;

/**
 * Auto-detect schema from CSV data.
 */
export function discover(csv: string): SpektrResult<SchemaConfig>;

/**
 * Enrich schema with AI (one-time Gemini call).
 */
export function refine(schema: SchemaConfig, apiKey: string, model?: string): SpektrResult<SchemaConfig>;

/**
 * Execute a QuerySpec against records.
 */
export function execute(spec: QuerySpec, records: Record[], options?: ExecuteOptions): SpektrResult<EngineResult>;

/**
 * Translate natural language to QuerySpec using Gemini.
 */
export function translate(
  query: string,
  schema: SchemaConfig,
  summary: DataSummary,
  apiKey: string,
  model?: string
): SpektrResult<TranslateResult>;

/**
 * Parse CSV string into records using a schema.
 */
export function parseCSV(csv: string, schema: SchemaConfig): SpektrResult<Record[]>;

/**
 * Get Spektr WASM version.
 */
export function version(): SpektrResult<string>;