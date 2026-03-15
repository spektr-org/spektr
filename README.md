
# Spektr

**Stateless analytics engine for any structured dataset.**

Ask questions in natural language. Feed any CSV, sheet, or table. Get **charts, tables, or summaries.
=======
![Spektr Execution pipeline](Docs/spektr_pipeline_animation.gif)

Ask questions in natural language. Feed any CSV, sheet, or table. Get **charts, tables, or summaries** instantly.


Spektr is **not a hosted service**.  
Every consumer runs its **own private instance** of the engine inside its own boundary.

Examples of consumers:

- ThePocketLedger (TPL)
- Jira dashboards
- CLI analytics tools
- AWS Lambda analytics services
- Google Sheets extensions
- Internal enterprise dashboards

No shared servers. No central processing.

---

## Execution Pipeline

![Spektr Pipeline](Docs/spektr-pipeline.gif)
=======
Spektr is a **stateless analytics engine** that runs inside the consumer’s environment.

Each application embeds its own instance of the engine.

![Spektr Architecture](Docs/Spektr_architecture.png)

### Key design principles

• **Consumer‑owned instances** — every consumer runs Spektr privately  
• **No central processing** — no shared service or hosted backend  
• **Stateless execution** — each request is independent  
• **AI optional** — analytics computation is always deterministic  

Execution pipeline:
```
Dataset → Discover → Translate → Execute → Result
```

**Discover**  
Automatically infer schema (dimensions, measures, dates).

**Translate**  
Natural language → QuerySpec (AI optional).

**Execute**  
Deterministic analytics computation.

Output:

- ChartConfig
- TableData
- TextData

---

## Architecture

![Spektr Architecture](Docs/spektr-architecture.png)

Each consumer embeds its **own Spektr instance**.

Examples:

```
TPL → private Spektr instance
Org tools → private Spektr instance
(no central processing)
```

---

## Try Spektr in 30 seconds

```bash
git clone https://github.com/spektr-org/spektr
cd spektr
go build -o spektr ./cmd/spektr/
```

Run demo queries:

```bash
./spektr_examples/examples/run_examples.sh
```

or (Windows):

```
spektr_examples\examples\run_examples.bat
```

---

## Example Datasets

Sample datasets are provided here:

```
/spektr_examples/datasets
```

Included datasets:

- `jira_sample.csv`
- `finance_sample.csv`
- `hr_sample.csv`

Example query:

```bash
spektr --file spektr_examples/datasets/finance_sample.csv --query "expenses by category"
```

---

## Example Code

Runnable examples are provided here:

```
/spektr_examples/examples
```

Examples include:

| File | Description |
|-----|-------------|
| go_pipeline_example.go | Run Spektr from Go |
| node_wasm_example.js | Use Spektr WASM in Node |
| python_http_example.py | Call Spektr HTTP endpoint |
| run_examples.sh | Batch CLI demo (Linux/Mac) |
| run_examples.bat | Batch CLI demo (Windows) |
| queries.txt | Natural language queries to try |

---

## Minimal Go Example

```go
resp := api.Pipeline(api.PipelineRequest{
    CSV:   csv,
    Query: "sum amount by category",
    Mode:  api.PipelineModeLocal,
})

fmt.Println(resp.Data.Result.Reply)
```

Result contains render‑ready output:

```
ChartConfig
TableData
TextData
```

---

## Documentation

Full documentation lives in:

```
Docs/
```

Includes:

- API reference
- Architecture design
- OpenAPI specification

Swagger UI:

https://petstore.swagger.io/?url=https://raw.githubusercontent.com/spektr-org/spektr/main/Docs/swagger.yaml

---

## License

MIT
