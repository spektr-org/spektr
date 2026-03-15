const spektr = require('@spektr/engine');

async function run() {

await spektr.init();

const csv = `Category,Field,Amount
Expense,Rent,2500
Expense,Groceries,800
Income,Salary,8000`;

const schema = spektr.discover(csv);

const records = spektr.parseCSV(csv, schema.data);

const result = spektr.execute({
    intent: "chart",
    aggregation: "sum",
    measure: "amount",
    groupBy: ["field"]
}, records.data);

console.log(result.data.reply);
}

run();
