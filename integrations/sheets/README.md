# Spektr Sheets Add-on

A Google Sheets add-on that turns any spreadsheet into an AI-powered analytics dashboard.

**Spektr Sheets is a universal client.** It works with any Spektr HTTP endpoint — Lambda, local server, Docker container, or `spektr serve`. Paste your data, type a question, get charts.

## How It Works

```
┌─────────────────────────────────────────────────┐
│                 Google Sheet                     │
│                                                  │
│  ┌──────────┐  ┌────────────┐  ┌─────────────┐  │
│  │ Your Data│  │  Results   │  │   Charts    │  │
│  │ (any CSV)│  │  (table)   │  │  (native)   │  │
│  └──────────┘  └────────────┘  └─────────────┘  │
│                                                  │
│  ┌──────────────────────────────────────────┐    │
│  │         Sidebar                          │    │
│  │  ┌────────────────────────────────┐      │    │
│  │  │ "bugs by priority"        [Go] │      │    │
│  │  └────────────────────────────────┘      │    │
│  │  There are 12 bugs. Top: P1 (6)         │    │
│  │  ✓ Rendered: table + chart              │    │
│  └──────────────────────────────────────────┘    │
└───────────────────┬──────────────────────────────┘
                    │ POST /pipeline
                    ▼
          ┌──────────────────┐
          │  Spektr Instance │  ← You deploy this
          │  (Lambda / local)│
          └──────────────────┘
```

## Quick Start

### 1. Deploy a Spektr Instance

The add-on needs a Spektr HTTP endpoint. Choose one:

**Option A: Local server (for testing)**

```bash
# Clone Spektr
git clone https://github.com/spektr-org/spektr.git
cd spektr

# Run the HTTP server
go run cmd/server/main.go --port 8080
```

**Option B: AWS Lambda (for production)**

```bash
# Build and deploy the Lambda
cd cmd/lambda
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
zip function.zip bootstrap
aws lambda create-function \
  --function-name spektr \
  --runtime provided.al2023 \
  --handler bootstrap \
  --zip-file fileb://function.zip
```

**Option C: Docker**

```bash
docker run -p 8080:8080 ghcr.io/spektr-org/spektr:latest serve
```

### 2. Install the Add-on

**From source (development):**

1. Open [script.google.com](https://script.google.com) → New Project
2. Copy each `.gs` and `.html` file from `src/` into the project
3. Click **Deploy → Test deployments** → **Install**
4. Open any Google Sheet → **Extensions → Spektr Analytics → Settings**

**Organization-internal (Google Workspace):**

1. In the Apps Script editor, click **Deploy → New deployment**
2. Select **Add-on** as the type
3. Set visibility to **Your Organization**
4. Submit for internal review

### 3. Configure

1. Open any Google Sheet with data
2. **Extensions → Spektr Analytics → Settings**
3. Paste your Spektr endpoint URL (e.g. `http://localhost:8080` or your Lambda URL)
4. Choose mode:
   - **AI** — natural language queries ("which assignees close the most bugs?"). Requires a Gemini API key.
   - **Local** — keyword queries ("sum story_points by priority"). No API key needed.
5. Save → Back → Start querying

### 4. Query

Open the sidebar (**Extensions → Spektr Analytics → Open Spektr**), type a question, hit **Analyze**.

**Local mode examples:**
```
count record_count by status
sum story_points by priority
avg duration_seconds by playbook_id
```

**AI mode examples:**
```
which assignees close the most critical bugs?
show me spend by service this month
average response time by severity
```

Results appear in two auto-created sheets:
- **Spektr Results** — formatted table with headers, alignment, alternating rows
- **Spektr Charts** — native Google Sheets chart (bar, line, or pie)

## File Structure

```
src/
├── appsscript.json    Manifest (scopes, triggers)
├── Code.gs            Menu, sidebar launcher, lifecycle hooks
├── Config.gs          Settings read/write (UserProperties)
├── Pipeline.gs        Sheet → CSV → POST /pipeline → parse response
├── Render.gs          Write tableData + create charts from chartConfig
├── Sidebar.html       Query input, results display, history
└── Settings.html      Endpoint URL, API key, mode configuration
```

## Architecture

The add-on is a **pure client**. All analytics computation happens on the Spektr instance.

| What | Where |
|------|-------|
| Data | Stays in Google Sheets |
| CSV serialization | Apps Script (Pipeline.gs) |
| Schema discovery | Spektr instance |
| AI translation | Spektr instance → Gemini API |
| Query execution | Spektr instance |
| Chart rendering | Google Sheets Charts API |

**Privacy model:** When using AI mode, only column names and sample values are sent to the AI provider. Raw data and measure values stay on the Spektr instance and are never forwarded. The full CSV is sent from the Sheet to your Spektr endpoint — deploy the endpoint where your data policy requires (your AWS account, your VPC, localhost).

## API Contract

The add-on uses a single Spektr endpoint:

```
POST /pipeline
{
  "csv": "...",       // Sheet data as CSV string
  "query": "...",     // Natural language or keyword query
  "mode": "ai|local", 
  "apiKey": "...",    // Required for mode=ai
  "model": "..."      // Optional model override
}

→ {
  "ok": true,
  "data": {
    "recordCount": 42,
    "query": "bugs by priority",
    "schema": { ... },
    "spec": { ... },
    "result": {
      "success": true,
      "type": "chart",
      "reply": "There are 12 bugs...",
      "chartConfig": { ... },
      "tableData": { ... }
    }
  }
}
```

Full API spec: [`swagger.yaml`](../../swagger.yaml)

## Required Scopes

| Scope | Why |
|-------|-----|
| `spreadsheets.currentonly` | Read sheet data, write results, create charts |
| `script.container.ui` | Show sidebar and menus |
| `script.external_request` | Call the Spektr endpoint via UrlFetchApp |

No Drive access. No Gmail access. Minimal permissions.

## License

MIT — same as Spektr.