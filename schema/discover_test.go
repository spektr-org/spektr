package schema

import (
	"encoding/json"
	"fmt"
	"testing"
)

// ============================================================================
// DISCOVERY TESTS
// ============================================================================

// Sample Jira CSV export
var jiraCSV = []byte(`Issue Key,Summary,Status,Priority,Issue Type,Assignee,Component,Sprint,Story Points,Time Spent Hours,Created,Resolved
PROJ-101,Login timeout on mobile,In Progress,P1 - Critical,Bug,alice@corp.com,Backend,Sprint 17,5,12.5,2026-01-15,
PROJ-102,Dashboard crash on Safari,To Do,P2 - High,Bug,bob@corp.com,Frontend,Sprint 17,3,0,2026-01-16,
PROJ-103,Add dark mode toggle,Done,P3 - Medium,Story,charlie@corp.com,Frontend,Sprint 16,8,16,2026-01-10,2026-01-20
PROJ-104,Update user docs,In Review,P4 - Low,Task,alice@corp.com,Documentation,Sprint 17,2,4,2026-01-18,
PROJ-105,Payment fails with expired card,In Progress,P1 - Critical,Bug,dave@corp.com,Backend,Sprint 17,8,20,2026-01-12,
PROJ-106,Optimize DB queries,Done,P2 - High,Task,eve@corp.com,Backend,Sprint 16,5,10,2026-01-08,2026-01-15
PROJ-107,Mobile push notifications,To Do,P2 - High,Story,frank@corp.com,Mobile,Sprint 18,13,0,2026-01-20,
PROJ-108,Fix memory leak in worker,In Progress,P1 - Critical,Bug,alice@corp.com,Infrastructure,Sprint 17,5,8,2026-01-14,
PROJ-109,Redesign settings page,Done,P3 - Medium,Story,bob@corp.com,Frontend,Sprint 15,8,14,2026-01-05,2026-01-12
PROJ-110,API rate limiting,Done,P2 - High,Story,charlie@corp.com,Backend,Sprint 16,5,9,2026-01-09,2026-01-18
PROJ-111,Add export to CSV,To Do,P3 - Medium,Story,dave@corp.com,Backend,Sprint 18,3,0,2026-01-22,
PROJ-112,Update SSL certs,Done,P1 - Critical,Task,eve@corp.com,Infrastructure,Sprint 16,1,2,2026-01-07,2026-01-07
`)

// Sample Finance CSV (TPL-like)
var financeCSV = []byte(`Month,Location,Category,Field,Currency,Amount
Jan-2026,Singapore,Income,Salary,SGD,8500.00
Jan-2026,Singapore,Expense,Rent,SGD,2200.00
Jan-2026,Singapore,Expense,Groceries,SGD,450.00
Jan-2026,Singapore,Expense,Transport,SGD,120.00
Jan-2026,India,Income,Rental Income,INR,25000.00
Jan-2026,India,Expense,Property Tax,INR,5000.00
Feb-2026,Singapore,Income,Salary,SGD,8500.00
Feb-2026,Singapore,Expense,Rent,SGD,2200.00
Feb-2026,Singapore,Expense,Internet,SGD,49.90
Feb-2026,India,Transfer,ToIndia,INR,50000.00
`)

func TestDiscoverJiraCSV(t *testing.T) {
	config, err := DiscoverFromCSV(jiraCSV)
	if err != nil {
		t.Fatalf("DiscoverFromCSV failed: %v", err)
	}

	// Print for visual inspection
	pretty, _ := json.MarshalIndent(config, "", "  ")
	fmt.Printf("=== JIRA SCHEMA ===\n%s\n\n", string(pretty))

	// Validate dimensions
	dimKeys := config.DimensionKeys()
	assertContains(t, dimKeys, "status", "Status should be a dimension")
	assertContains(t, dimKeys, "priority", "Priority should be a dimension")
	assertContains(t, dimKeys, "issue_type", "Issue Type should be a dimension")
	assertContains(t, dimKeys, "component", "Component should be a dimension")
	assertContains(t, dimKeys, "sprint", "Sprint should be a dimension")

	// Validate measures
	measKeys := config.MeasureKeys()
	assertContains(t, measKeys, "story_points", "Story Points should be a measure")
	assertContains(t, measKeys, "time_spent_hours", "Time Spent Hours should be a measure")
	assertContains(t, measKeys, "record_count", "record_count synthetic measure should exist")

	// Validate skipped columns
	skippedNames := make([]string, len(config.SkippedColumns))
	for i, s := range config.SkippedColumns {
		skippedNames[i] = s.Column
	}
	assertContains(t, skippedNames, "Issue Key", "Issue Key should be skipped (unique ID)")
	assertContains(t, skippedNames, "Summary", "Summary should be skipped (unique free text)")

	// Validate temporal detection
	for _, d := range config.Dimensions {
		if d.Key == "sprint" {
			// Sprint might or might not be detected as temporal depending on pattern
			// "Sprint 17" doesn't match month patterns, so it should be a plain dimension
		}
	}

	// Validate Issue Key is NOT recoverable (unique ID)
	for _, s := range config.SkippedColumns {
		if s.Column == "Issue Key" && s.Recoverable {
			t.Error("Issue Key should NOT be recoverable — it's a unique ID")
		}
	}
}

func TestDiscoverFinanceCSV(t *testing.T) {
	config, err := DiscoverFromCSV(financeCSV)
	if err != nil {
		t.Fatalf("DiscoverFromCSV failed: %v", err)
	}

	pretty, _ := json.MarshalIndent(config, "", "  ")
	fmt.Printf("=== FINANCE SCHEMA ===\n%s\n\n", string(pretty))

	// Validate dimensions
	dimKeys := config.DimensionKeys()
	assertContains(t, dimKeys, "month", "Month should be a dimension")
	assertContains(t, dimKeys, "location", "Location should be a dimension")
	assertContains(t, dimKeys, "category", "Category should be a dimension")
	assertContains(t, dimKeys, "field", "Field should be a dimension")
	assertContains(t, dimKeys, "currency", "Currency should be a dimension")

	// Validate measures
	measKeys := config.MeasureKeys()
	assertContains(t, measKeys, "amount", "Amount should be a measure")

	// Validate temporal detection on Month
	for _, d := range config.Dimensions {
		if d.Key == "month" && !d.IsTemporal {
			t.Error("Month dimension should be detected as temporal")
		}
	}

	// Validate currency detection
	for _, d := range config.Dimensions {
		if d.Key == "currency" && !d.IsCurrencyCode {
			t.Error("Currency dimension should be detected as currency code")
		}
	}

	// Validate currency config was auto-detected
	if config.Currency == nil {
		t.Error("Currency config should be auto-detected from currency dimension")
	} else if !config.Currency.Enabled {
		t.Error("Currency config should be enabled")
	} else if config.Currency.CodeDimension != "currency" {
		t.Errorf("Currency code dimension should be 'currency', got '%s'", config.Currency.CodeDimension)
	}

	// Validate hierarchy: Field is child of Category
	for _, d := range config.Dimensions {
		if d.Key == "field" && d.Parent != "category" {
			t.Errorf("Field should have parent 'category', got '%s'", d.Parent)
		}
	}
}

func TestDiscoverWithRecovery(t *testing.T) {
	// Summary is skipped (unique per row). Recover it as a dimension.
	config, err := DiscoverFromCSV(jiraCSV, DiscoverOptions{
		RecoverColumns: []string{"Summary"},
		Name:           "Jira with Summary",
	})
	if err != nil {
		t.Fatalf("DiscoverFromCSV with recovery failed: %v", err)
	}

	dimKeys := config.DimensionKeys()
	assertContains(t, dimKeys, "summary", "Summary should be recovered as dimension")

	// Verify it's NOT in skipped anymore
	for _, s := range config.SkippedColumns {
		if s.Column == "Summary" {
			t.Error("Summary should not be in skipped columns after recovery")
		}
	}
}

func TestSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Story Points", "story_points"},
		{"Issue Key", "issue_key"},
		{"issueType", "issue_type"},
		{"StoryPoints", "story_points"},
		{"Time Spent Hours", "time_spent_hours"},
		{"ID", "id"},
		{"created_at", "created_at"},
		{"Sprint", "sprint"},
	}

	for _, tt := range tests {
		got := toSnakeCase(tt.input)
		if got != tt.expected {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"story_points", "Story Points"},
		{"Sprint", "Sprint"},
		{"Issue Type", "Issue Type"},
		{"time_spent_hours", "Time Spent Hours"},
	}

	for _, tt := range tests {
		got := toDisplayName(tt.input)
		if got != tt.expected {
			t.Errorf("toDisplayName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCurrencyCodeDetection(t *testing.T) {
	tests := []struct {
		samples  []string
		expected bool
	}{
		{[]string{"SGD", "INR", "USD"}, true},
		{[]string{"SGD", "INR"}, true},
		{[]string{"Yes", "No", "Maybe"}, false},
		{[]string{"ABC", "DEF", "GHI"}, false}, // 3-letter but not currencies
		{[]string{"SGD", "NotCurrency"}, false},
	}

	for _, tt := range tests {
		got := detectCurrencyCodes(tt.samples)
		if got != tt.expected {
			t.Errorf("detectCurrencyCodes(%v) = %v, want %v", tt.samples, got, tt.expected)
		}
	}
}

func TestTemporalDetection(t *testing.T) {
	tests := []struct {
		samples    []string
		isTemporal bool
	}{
		{[]string{"Jan-2026", "Feb-2026", "Mar-2026"}, true},
		{[]string{"2025-01", "2025-02", "2025-03"}, true},
		{[]string{"Q1-2026", "Q2-2026"}, true},
		{[]string{"2024", "2025", "2026"}, true},
		{[]string{"Sprint 15", "Sprint 16", "Sprint 17"}, false},
		{[]string{"Backend", "Frontend", "Mobile"}, false},
	}

	for _, tt := range tests {
		got, _ := detectTemporalPattern(tt.samples)
		if got != tt.isTemporal {
			t.Errorf("detectTemporalPattern(%v) = %v, want %v", tt.samples, got, tt.isTemporal)
		}
	}
}

// ============================================================================
// HELPERS
// ============================================================================

func assertContains(t *testing.T, slice []string, item string, msg string) {
	t.Helper()
	for _, s := range slice {
		if s == item {
			return
		}
	}
	t.Errorf("%s: %q not found in %v", msg, item, slice)
}
