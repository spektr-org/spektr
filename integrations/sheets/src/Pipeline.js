/**
 * Pipeline.gs — Sheet → CSV → Spektr /pipeline → Result
 *
 * Core data flow. Reads the active (or configured) sheet,
 * serializes to CSV, POSTs to the Spektr /pipeline endpoint,
 * and returns the parsed response.
 */

// ── Public Entry Point ───────────────────────────────────────────

/**
 * Called from the sidebar. Returns the full pipeline response
 * or an error envelope.
 *
 * @param {string} query — Natural language or keyword query
 * @returns {Object} { ok: true, data: PipelineResult } | { ok: false, error: string }
 */
function runPipeline(query) {
  var config = getConfig();

  // ── Validate config
  if (!config.endpoint) {
    return { ok: false, error: 'No Spektr endpoint configured. Go to Settings to set it up.' };
  }

  if (config.mode === 'ai' && !config.apiKey) {
    return { ok: false, error: 'AI mode requires a Gemini API key. Add one in Settings, or switch to "local" mode.' };
  }

  // ── Read sheet data as CSV
  var csvResult = readSheetAsCsv_(config.dataSheet);
  if (!csvResult.ok) {
    return csvResult;
  }

  // ── Build request body (matches PipelineRequest in swagger)
  var payload = {
    csv: csvResult.csv,
    query: query,
    mode: config.mode
  };

  if (config.mode === 'ai') {
    payload.apiKey = config.apiKey;
  }

  if (config.model) {
    payload.model = config.model;
  }

  // ── Call /pipeline
  var url = config.endpoint.replace(/\/+$/, '') + '/pipeline';

  try {
    var response = UrlFetchApp.fetch(url, {
      method: 'post',
      contentType: 'application/json',
      payload: JSON.stringify(payload),
      muteHttpExceptions: true,
      headers: {
        'Accept': 'application/json',
        'ngrok-skip-browser-warning': 'true'
      }
    });

    var status = response.getResponseCode();
    var body;

    try {
      body = JSON.parse(response.getContentText());
    } catch (parseErr) {
      return {
        ok: false,
        error: 'Spektr returned invalid JSON (HTTP ' + status + '). Response: ' + response.getContentText().substring(0, 200)
      };
    }

    if (status !== 200 || !body.ok) {
      return {
        ok: false,
        error: body.error || ('Spektr returned HTTP ' + status)
      };
    }

    // ── Success — save query to history and return
    saveLastQuery(query);

    return { ok: true, data: body.data };

  } catch (fetchErr) {
    return {
      ok: false,
      error: 'Could not reach Spektr at ' + config.endpoint + ': ' + fetchErr.message
    };
  }
}

// ── CSV Serialization ────────────────────────────────────────────

/**
 * Reads a sheet and converts it to a CSV string.
 * Uses the configured data sheet name, or the active sheet if not set.
 *
 * @param {string} sheetName — Sheet name override (empty = active sheet)
 * @returns {Object} { ok: true, csv: string } | { ok: false, error: string }
 */
function readSheetAsCsv_(sheetName) {
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  var sheet;

  if (sheetName) {
    sheet = ss.getSheetByName(sheetName);
    if (!sheet) {
      return { ok: false, error: 'Data sheet "' + sheetName + '" not found. Check Settings.' };
    }
  } else {
    sheet = ss.getActiveSheet();
  }

  var data = sheet.getDataRange().getValues();

  if (data.length < 2) {
    return { ok: false, error: 'Sheet "' + sheet.getName() + '" has no data (need at least a header row and one data row).' };
  }

  var csv = data.map(function(row) {
    return row.map(function(cell) {
      return csvEscapeCell_(cell);
    }).join(',');
  }).join('\n');

  return { ok: true, csv: csv };
}

/**
 * Escapes a single cell value for CSV output.
 * Handles strings with commas, quotes, newlines, and date objects.
 */
function csvEscapeCell_(value) {
  if (value === null || value === undefined) return '';

  // Date objects → ISO string
  if (value instanceof Date) {
    return value.toISOString().split('T')[0];
  }

  var str = String(value);

  // If the string contains comma, quote, or newline, wrap in quotes
  if (str.indexOf(',') > -1 || str.indexOf('"') > -1 || str.indexOf('\n') > -1) {
    return '"' + str.replace(/"/g, '""') + '"';
  }

  return str;
}