// Test: @spektr/engine npm package
// Run: node test.js

const spektr = require("./index");

const testCSV = `Issue Key,Summary,Issue Type,Status,Priority,Assignee,Story Points,Time Spent (hours),Created
PROJ-001,Login bug,Bug,Done,P1 - Critical,Alice,3,6,2025-11-01
PROJ-002,OAuth flow,Story,Done,P2 - High,Bob,8,18,2025-11-01
PROJ-003,Update docs,Task,Done,P3 - Medium,Charlie,2,3,2025-11-02
PROJ-004,New dashboard,Story,In Progress,P2 - High,Alice,5,10,2025-11-03
PROJ-005,Memory leak,Bug,Done,P1 - Critical,Bob,5,14,2025-11-04
`;

async function main() {
  console.log("Initializing Spektr WASM...");
  await spektr.init();

  // Version
  const ver = spektr.version();
  console.log("Version:", ver.data);

  // Discover
  console.log("\n--- DISCOVER ---");
  const schema = spektr.discover(testCSV);
  if (!schema.ok) {
    console.error("Discover failed:", schema.error);
    process.exit(1);
  }
  console.log("Dataset:", schema.data.name);
  console.log(
    "Dimensions:",
    schema.data.dimensions.map((d) => d.key).join(", ")
  );
  console.log(
    "Measures:",
    schema.data.measures.map((m) => `${m.key} (${m.unit || "no unit"})`).join(", ")
  );
  console.log(
    "Skipped:",
    (schema.data.skippedColumns || []).map((s) => s.column).join(", ")
  );

  // Parse CSV
  console.log("\n--- PARSE CSV ---");
  const records = spektr.parseCSV(testCSV, schema.data);
  if (!records.ok) {
    console.error("ParseCSV failed:", records.error);
    process.exit(1);
  }
  console.log("Parsed", records.data.length, "records");

  // Execute
  console.log("\n--- EXECUTE ---");
  const spec = {
    intent: "chart",
    filters: {
      dimensions: {
        issue_type: ["Bug"],
      },
    },
    aggregation: "count",
    measure: "record_count",
    groupBy: ["priority"],
    sortBy: "value_desc",
    limit: 0,
    visualize: "bar",
    title: "Bugs by Priority",
    reply: "Found {count} bugs. Most are {top_category}.",
    confidence: 0.9,
  };

  const result = spektr.execute(spec, records.data, {
    defaultMeasure: "record_count",
  });

  if (!result.ok) {
    console.error("Execute failed:", result.error);
    process.exit(1);
  }

  console.log("Result type:", result.data.type);
  console.log("Reply:", result.data.reply);
  if (result.data.chartConfig) {
    console.log("Chart type:", result.data.chartConfig.chartType);
    for (const series of result.data.chartConfig.series || []) {
      for (const d of series.data || []) {
        console.log(`  ${d.label}: ${d.value}`);
      }
    }
  }

  console.log("\n✅ All tests passed!");
}

main().catch((err) => {
  console.error("Test failed:", err);
  process.exit(1);
});