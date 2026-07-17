/**
 * Spektr Analytics — Google Sheets Add-on
 * 
 * A universal client for any Spektr HTTP endpoint.
 * Turns any Google Sheet into an AI-powered analytics dashboard.
 *
 * https://github.com/spektr-org/spektr
 */

// ── Menu & Lifecycle ─────────────────────────────────────────────

function onOpen(e) {
  SpreadsheetApp.getUi()
    .createAddonMenu()
    .addItem('Open Spektr', 'showSidebar')
    .addItem('Run last query', 'rerunLastQuery')
    .addSeparator()
    .addItem('Settings', 'showSettings')
    .addItem('Health check', 'runHealthCheck')
    .addToUi();
}

function onInstall(e) {
  onOpen(e);
}

function onHomepage(e) {
  return CardService.newCardBuilder()
    .setHeader(CardService.newCardHeader().setTitle('Spektr Analytics'))
    .addSection(
      CardService.newCardSection()
        .addWidget(CardService.newTextParagraph().setText(
          'Open the sidebar from the Extensions menu to start querying your data.'
        ))
    )
    .build();
}

// ── Sidebar ──────────────────────────────────────────────────────

function showSidebar() {
  var html = HtmlService.createHtmlOutputFromFile('Sidebar')
    .setTitle('Spektr Analytics')
    .setWidth(360);
  SpreadsheetApp.getUi().showSidebar(html);
}

function showSettings() {
  var html = HtmlService.createHtmlOutputFromFile('Settings')
    .setTitle('Spektr Settings')
    .setWidth(360);
  SpreadsheetApp.getUi().showSidebar(html);
}

// ── Health Check ─────────────────────────────────────────────────

function runHealthCheck() {
  var config = getConfig();
  if (!config.endpoint) {
    SpreadsheetApp.getUi().alert(
      'No endpoint configured.\n\nGo to Extensions → Spektr Analytics → Settings to set your Spektr endpoint URL.'
    );
    return;
  }

  try {
    var url = config.endpoint.replace(/\/+$/, '') + '/health';
    var response = UrlFetchApp.fetch(url, {
      method: 'get',
      muteHttpExceptions: true,
      headers: {
        'Accept': 'application/json',
        'ngrok-skip-browser-warning': 'true'
      }
    });

    var body = JSON.parse(response.getContentText());
    if (body.ok) {
      SpreadsheetApp.getUi().alert(
        '✅ Spektr is ready\n\n' +
        'Version: ' + body.data.version + '\n' +
        'Status: ' + body.data.status + '\n' +
        'Endpoint: ' + config.endpoint
      );
    } else {
      SpreadsheetApp.getUi().alert('❌ Spektr returned an error:\n\n' + (body.error || 'Unknown error'));
    }
  } catch (err) {
    SpreadsheetApp.getUi().alert(
      '❌ Could not reach Spektr\n\n' +
      'Endpoint: ' + config.endpoint + '\n' +
      'Error: ' + err.message
    );
  }
}

// ── Rerun Last Query ─────────────────────────────────────────────

function rerunLastQuery() {
  var config = getConfig();
  if (!config.lastQuery) {
    SpreadsheetApp.getUi().alert('No previous query found. Open the sidebar and run a query first.');
    return;
  }
  var result = runPipeline(config.lastQuery);
  if (result.ok) {
    renderResult(result.data);
  } else {
    SpreadsheetApp.getUi().alert('Query failed: ' + result.error);
  }
}

// ── Debug Helper (can remove after testing) ──────────────────────

function testSidebarStorage() {
  var props = PropertiesService.getScriptProperties();
  props.setProperty('sidebar_test', 'works');
  var val = props.getProperty('sidebar_test');
  props.deleteProperty('sidebar_test');
  return { ok: true, value: val };
}