/**
 * Config.gs — Persistent settings via Script Properties
 *
 * Uses ScriptProperties (not UserProperties) to avoid permission issues
 * in multi-account browser sessions and container-bound scripts.
 */

var CONFIG_KEYS = {
  ENDPOINT:     'spektr_endpoint',
  API_KEY:      'spektr_api_key',
  MODE:         'spektr_mode',
  MODEL:        'spektr_model',
  DATA_SHEET:   'spektr_data_sheet',
  RESULT_SHEET: 'spektr_result_sheet',
  CHART_SHEET:  'spektr_chart_sheet',
  LAST_QUERY:   'spektr_last_query',
  QUERY_HISTORY:'spektr_query_history'
};

var DEFAULTS = {
  mode: 'ai',
  model: '',
  dataSheet: '',
  resultSheet: 'Spektr Results',
  chartSheet: 'Spektr Charts'
};

// ── Read ──────────────────────────────────────────────────────────

function getConfig() {
  var props = PropertiesService.getScriptProperties();
  return {
    endpoint:    props.getProperty(CONFIG_KEYS.ENDPOINT) || '',
    apiKey:      props.getProperty(CONFIG_KEYS.API_KEY) || '',
    mode:        props.getProperty(CONFIG_KEYS.MODE) || DEFAULTS.mode,
    model:       props.getProperty(CONFIG_KEYS.MODEL) || DEFAULTS.model,
    dataSheet:   props.getProperty(CONFIG_KEYS.DATA_SHEET) || DEFAULTS.dataSheet,
    resultSheet: props.getProperty(CONFIG_KEYS.RESULT_SHEET) || DEFAULTS.resultSheet,
    chartSheet:  props.getProperty(CONFIG_KEYS.CHART_SHEET) || DEFAULTS.chartSheet,
    lastQuery:   props.getProperty(CONFIG_KEYS.LAST_QUERY) || '',
    queryHistory: getQueryHistory_()
  };
}

// ── Write ─────────────────────────────────────────────────────────

function saveConfig(settings) {
  var props = PropertiesService.getScriptProperties();

  if (settings.endpoint !== undefined)    props.setProperty(CONFIG_KEYS.ENDPOINT, settings.endpoint.replace(/\/+$/, ''));
  if (settings.apiKey !== undefined)      props.setProperty(CONFIG_KEYS.API_KEY, settings.apiKey);
  if (settings.mode !== undefined)        props.setProperty(CONFIG_KEYS.MODE, settings.mode);
  if (settings.model !== undefined)       props.setProperty(CONFIG_KEYS.MODEL, settings.model);
  if (settings.dataSheet !== undefined)   props.setProperty(CONFIG_KEYS.DATA_SHEET, settings.dataSheet);
  if (settings.resultSheet !== undefined) props.setProperty(CONFIG_KEYS.RESULT_SHEET, settings.resultSheet);
  if (settings.chartSheet !== undefined)  props.setProperty(CONFIG_KEYS.CHART_SHEET, settings.chartSheet);

  return { ok: true };
}

function saveLastQuery(query) {
  var props = PropertiesService.getScriptProperties();
  props.setProperty(CONFIG_KEYS.LAST_QUERY, query);
  appendQueryHistory_(query);
}

// ── Query History ─────────────────────────────────────────────────

var MAX_HISTORY = 20;

function getQueryHistory_() {
  var props = PropertiesService.getScriptProperties();
  var raw = props.getProperty(CONFIG_KEYS.QUERY_HISTORY);
  if (!raw) return [];
  try {
    return JSON.parse(raw);
  } catch (e) {
    return [];
  }
}

function appendQueryHistory_(query) {
  var history = getQueryHistory_();

  var idx = history.indexOf(query);
  if (idx > -1) history.splice(idx, 1);

  history.unshift(query);

  if (history.length > MAX_HISTORY) history = history.slice(0, MAX_HISTORY);

  PropertiesService.getScriptProperties()
    .setProperty(CONFIG_KEYS.QUERY_HISTORY, JSON.stringify(history));
}

function clearQueryHistory() {
  PropertiesService.getScriptProperties()
    .deleteProperty(CONFIG_KEYS.QUERY_HISTORY);
  return { ok: true };
}

// ── Helpers ───────────────────────────────────────────────────────

function getSheetNames() {
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  return ss.getSheets().map(function(s) { return s.getName(); });
}