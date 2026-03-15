package main

import (
    "fmt"
    "github.com/spektr-org/spektr/api"
)

func main() {

csv := `Category,Field,Amount
Expense,Rent,2500
Expense,Groceries,800
Income,Salary,8000`

resp := api.Pipeline(api.PipelineRequest{
    CSV:   csv,
    Query: "sum amount by category",
    Mode:  api.PipelineModeLocal,
})

if !resp.OK {
    panic(resp.Error)
}

fmt.Println(resp.Data.Result.Reply)
fmt.Println(resp.Data.Result.ChartConfig)
}
