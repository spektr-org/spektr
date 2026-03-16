/**
 * Render.gs — Write results to sheets and create charts
 *
 * Maps Spektr's Result object (chartConfig, tableData, reply)
 * to native Google Sheets tables and charts.
 */

// ── Public Entry Point ───────────────────────────────────────────

/**
 * Renders a PipelineResult into the spreadsheet.
 * Called from the sidebar after a successful pipeline call.
 *
 * @param {Object} pipelineData — The `data` field from PipelineResponse
 * @returns {Object} { ok: true, summary: string } | { ok: false, error: string }
 */
function renderResult(pipelineData) {
  if (!pipelineData || !pipelineData.result) {
    return { ok: false, error: 'No result data to render.' };
  }

  var result = pipelineData.result;
  var config = getConfig();
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  var rendered = [];

  try {
    // ── Render table data
    if (result.tableData && result.tableData.columns && result.tableData.rows) {
      renderTable_(ss, config.resultSheet, result.tableData, pipelineData.query);
      rendered.push('table');
    }

    // ── Render chart
    if (result.chartConfig && result.chartConfig.series && result.chartConfig.series.length > 0) {
      renderChart_(ss, config.chartSheet, config.resultSheet, result.chartConfig);
      rendered.push('chart');
    }

    var summary = 'Rendered: ' + rendered.join(' + ');
    if (rendered.length === 0) {
      summary = 'Text result only (no table or chart data).';
    }

    return { ok: true, summary: summary };

  } catch (err) {
    return { ok: false, error: 'Render failed: ' + err.message };
  }
}

// ── Table Rendering ──────────────────────────────────────────────

/**
 * Writes tableData into the results sheet.
 * Clears previous results, writes header + rows, applies formatting.
 */
function renderTable_(ss, resultSheetName, tableData, queryText) {
  var sheet = getOrCreateSheet_(ss, resultSheetName);

  // Clear previous results
  sheet.clear();

  var startRow = 1;

  // ── Title row
  if (tableData.title || queryText) {
    var title = tableData.title || ('Query: ' + queryText);
    sheet.getRange(startRow, 1).setValue(title);
    sheet.getRange(startRow, 1)
      .setFontSize(12)
      .setFontWeight('bold')
      .setFontColor('#1a1a2e');
    startRow += 2;  // blank row after title
  }

  // ── Header row
  var headers = tableData.columns.map(function(col) { return col.label || col.key; });
  var headerRange = sheet.getRange(startRow, 1, 1, headers.length);
  headerRange.setValues([headers]);
  headerRange
    .setFontWeight('bold')
    .setBackground('#f0f4f8')
    .setFontColor('#334155')
    .setBorder(false, false, true, false, false, false, '#cbd5e1', SpreadsheetApp.BorderStyle.SOLID);

  // Apply alignment from column metadata
  tableData.columns.forEach(function(col, idx) {
    var align = col.align || (col.type === 'number' ? 'right' : 'left');
    headerRange.getCell(1, idx + 1).setHorizontalAlignment(align);
  });

  startRow++;

  // ── Data rows
  if (tableData.rows.length > 0) {
    var dataRange = sheet.getRange(startRow, 1, tableData.rows.length, headers.length);
    dataRange.setValues(tableData.rows);

    // Apply number formatting and alignment per column
    tableData.columns.forEach(function(col, idx) {
      var colRange = sheet.getRange(startRow, idx + 1, tableData.rows.length, 1);

      if (col.type === 'number') {
        colRange.setHorizontalAlignment('right');
        colRange.setNumberFormat('#,##0.##');
      } else {
        colRange.setHorizontalAlignment(col.align || 'left');
      }
    });

    // Alternate row shading
    for (var r = 0; r < tableData.rows.length; r++) {
      if (r % 2 === 1) {
        sheet.getRange(startRow + r, 1, 1, headers.length).setBackground('#f8fafc');
      }
    }
  }

  // ── Summary row
  if (tableData.summary) {
    var summaryRow = startRow + tableData.rows.length + 1;
    var summaryLabel = tableData.summary.label || 'Total';
    sheet.getRange(summaryRow, 1).setValue(summaryLabel).setFontWeight('bold');

    if (tableData.summary.values) {
      tableData.columns.forEach(function(col, idx) {
        var val = tableData.summary.values[col.key];
        if (val !== undefined) {
          sheet.getRange(summaryRow, idx + 1)
            .setValue(val)
            .setFontWeight('bold')
            .setHorizontalAlignment('right');
        }
      });
    }

    sheet.getRange(summaryRow, 1, 1, headers.length)
      .setBorder(true, false, false, false, false, false, '#94a3b8', SpreadsheetApp.BorderStyle.SOLID)
      .setBackground('#f0f4f8');
  }

  // Auto-resize columns
  for (var c = 1; c <= headers.length; c++) {
    sheet.autoResizeColumn(c);
  }
}

// ── Chart Rendering ──────────────────────────────────────────────

/**
 * Creates or updates a native Google Sheets chart from Spektr's chartConfig.
 * Chart data is sourced from the Results sheet (already written by renderTable_).
 */
function renderChart_(ss, chartSheetName, resultSheetName, chartConfig) {
  var chartSheet = getOrCreateSheet_(ss, chartSheetName);
  var resultSheet = ss.getSheetByName(resultSheetName);

  // ── Write chart data into a dedicated area on the chart sheet
  // This ensures charts have a clean data source even if the results
  // sheet is also used for table display.
  chartSheet.clear();

  var series = chartConfig.series;
  if (!series || series.length === 0) return;

  var primarySeries = series[0];
  var dataPoints = primarySeries.data || [];
  if (dataPoints.length === 0) return;

  // Write header
  var xLabel = chartConfig.xAxis || 'Label';
  var yLabel = chartConfig.yAxis || 'Value';
  chartSheet.getRange(1, 1).setValue(xLabel);
  chartSheet.getRange(1, 2).setValue(yLabel);

  // Write data points
  var chartData = dataPoints.map(function(dp) {
    return [dp.label, dp.value];
  });
  if (chartData.length > 0) {
    chartSheet.getRange(2, 1, chartData.length, 2).setValues(chartData);
  }

  // ── Multi-series support
  if (series.length > 1) {
    for (var s = 1; s < series.length; s++) {
      var colIdx = s + 2; // Column C, D, E...
      chartSheet.getRange(1, colIdx).setValue(series[s].name || ('Series ' + (s + 1)));
      var sData = series[s].data || [];
      for (var d = 0; d < sData.length; d++) {
        chartSheet.getRange(d + 2, colIdx).setValue(sData[d].value);
      }
    }
  }

  var numRows = chartData.length + 1; // +1 for header
  var numCols = 1 + series.length;    // label col + one per series

  // ── Remove existing charts on this sheet
  var existingCharts = chartSheet.getCharts();
  for (var i = 0; i < existingCharts.length; i++) {
    chartSheet.removeChart(existingCharts[i]);
  }

  // ── Map Spektr chartType to Google Sheets chart type
  var gChartType;
  switch ((chartConfig.chartType || 'bar').toLowerCase()) {
    case 'line':
      gChartType = Charts.ChartType.LINE;
      break;
    case 'pie':
      gChartType = Charts.ChartType.PIE;
      break;
    case 'bar':
    default:
      gChartType = Charts.ChartType.BAR;
      break;
  }

  // ── Build chart
  var dataRange = chartSheet.getRange(1, 1, numRows, numCols);

  var chartBuilder = chartSheet.newChart()
    .setChartType(gChartType)
    .addRange(dataRange)
    .setPosition(numRows + 3, 1, 0, 0)  // Below the data
    .setNumHeaders(1)
    .setOption('title', chartConfig.title || '')
    .setOption('width', 700)
    .setOption('height', 420);

  // ── Axis labels
  if (chartConfig.xAxis) {
    chartBuilder.setOption('hAxis.title', chartConfig.xAxis);
  }
  if (chartConfig.yAxis) {
    chartBuilder.setOption('vAxis.title', chartConfig.yAxis);
  }

  // ── Grid
  if (chartConfig.showGrid === false) {
    chartBuilder.setOption('hAxis.gridlines.count', 0);
    chartBuilder.setOption('vAxis.gridlines.count', 0);
  }

  // ── Legend
  if (chartConfig.showLegend === false) {
    chartBuilder.setOption('legend.position', 'none');
  }

  // ── Colors
  if (chartConfig.colors && chartConfig.colors.length > 0) {
    chartBuilder.setOption('colors', chartConfig.colors);
  }

  chartSheet.insertChart(chartBuilder.build());

  // Auto-resize data columns
  for (var c = 1; c <= numCols; c++) {
    chartSheet.autoResizeColumn(c);
  }
}

// ── Helpers ───────────────────────────────────────────────────────

/**
 * Gets a sheet by name, or creates it if it doesn't exist.
 */
function getOrCreateSheet_(ss, name) {
  var sheet = ss.getSheetByName(name);
  if (!sheet) {
    sheet = ss.insertSheet(name);
  }
  return sheet;
}