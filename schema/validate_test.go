package schema

import (
	"testing"
)

// ============================================================================
// PHASE 6 VALIDATION — Non-Finance Dataset Classification
// ============================================================================

// ── Test Data ─────────────────────────────────────────────────────────────────

var jiraValidationCSV = []byte("Issue Key,Summary,Issue Type,Status,Priority,Assignee,Reporter,Component,Sprint,Story Points,Original Estimate (hours),Time Spent (hours),Created,Resolved,Labels\nPROJ-001,User login fails on Safari,Bug,Done,P1 - Critical,Alice Chen,David Kim,Authentication,Sprint 1,3,8,6,2025-11-01,2025-11-05,backend;urgent\nPROJ-002,Implement OAuth2 flow,Story,Done,P2 - High,Bob Patel,Sarah Lee,Authentication,Sprint 1,8,16,18,2025-11-01,2025-11-12,backend;security\nPROJ-003,Update API documentation,Task,Done,P3 - Medium,Charlie Wong,Charlie Wong,Documentation,Sprint 1,2,4,3,2025-11-02,2025-11-06,docs\nPROJ-004,Design new dashboard layout,Story,Done,P2 - High,Diana Reyes,Sarah Lee,Dashboard,Sprint 1,5,12,10,2025-11-03,2025-11-10,frontend;design\nPROJ-005,Fix memory leak in data service,Bug,Done,P1 - Critical,Alice Chen,Bob Patel,Data Pipeline,Sprint 1,5,10,14,2025-11-04,2025-11-11,backend;performance\nPROJ-006,Add export to CSV feature,Story,Done,P3 - Medium,Eve Johnson,David Kim,Dashboard,Sprint 2,3,8,7,2025-11-15,2025-11-20,frontend\nPROJ-007,Database connection pool exhaustion,Bug,Done,P1 - Critical,Bob Patel,Alice Chen,Data Pipeline,Sprint 2,8,16,20,2025-11-15,2025-11-25,backend;urgent;database\nPROJ-008,Create onboarding tutorial,Story,Done,P2 - High,Frank Tanaka,Sarah Lee,Documentation,Sprint 2,5,12,11,2025-11-16,2025-11-22,docs;ux\nPROJ-009,Refactor notification service,Task,Done,P3 - Medium,Alice Chen,Alice Chen,Notifications,Sprint 2,3,6,5,2025-11-17,2025-11-21,backend;tech-debt\nPROJ-010,Mobile responsive header,Bug,Done,P2 - High,Diana Reyes,David Kim,Dashboard,Sprint 2,2,4,3,2025-11-18,2025-11-20,frontend;mobile\nPROJ-011,Implement role-based access control,Story,In Progress,P1 - Critical,Bob Patel,Sarah Lee,Authentication,Sprint 3,13,32,28,2025-12-01,,backend;security\nPROJ-012,Search results pagination broken,Bug,Done,P2 - High,Charlie Wong,Eve Johnson,Search,Sprint 3,3,6,5,2025-12-01,2025-12-04,frontend\nPROJ-013,Add dark mode support,Story,In Progress,P3 - Medium,Diana Reyes,David Kim,Dashboard,Sprint 3,8,20,12,2025-12-02,,frontend;design\nPROJ-014,Write unit tests for auth module,Task,Done,P2 - High,Alice Chen,Bob Patel,Authentication,Sprint 3,5,10,9,2025-12-02,2025-12-06,backend;testing\nPROJ-015,Optimize SQL queries for reports,Task,In Progress,P2 - High,Bob Patel,Alice Chen,Data Pipeline,Sprint 3,5,12,8,2025-12-03,,backend;performance;database\nPROJ-016,Customer feedback widget,Story,To Do,P3 - Medium,Eve Johnson,Sarah Lee,Notifications,Sprint 4,5,12,0,2025-12-15,,frontend;ux\nPROJ-017,Fix timezone handling in scheduler,Bug,In Progress,P1 - Critical,Alice Chen,Frank Tanaka,Data Pipeline,Sprint 4,3,8,4,2025-12-16,,backend\nPROJ-018,API rate limiting implementation,Story,To Do,P2 - High,Bob Patel,Sarah Lee,Authentication,Sprint 4,8,20,0,2025-12-16,,backend;security\nPROJ-019,Accessibility audit fixes,Task,In Progress,P2 - High,Diana Reyes,David Kim,Dashboard,Sprint 4,5,12,6,2025-12-17,,frontend;accessibility\nPROJ-020,Migrate to PostgreSQL 16,Task,To Do,P3 - Medium,Charlie Wong,Bob Patel,Data Pipeline,Sprint 4,3,8,0,2025-12-17,,backend;database;tech-debt\n")

var ecommerceValidationCSV = []byte("Order ID,Customer Name,Product,Category,Sub-Category,Region,City,Order Date,Ship Date,Quantity,Unit Price,Discount,Revenue,Shipping Cost,Currency\nORD-10001,James Wilson,Wireless Mouse,Technology,Accessories,North America,New York,2025-08-01,2025-08-03,2,29.99,0.00,59.98,5.99,USD\nORD-10002,Maria Santos,Office Chair,Furniture,Chairs,South America,São Paulo,2025-08-01,2025-08-07,1,349.99,0.10,314.99,45.00,BRL\nORD-10003,Yuki Tanaka,Notebook Set,Office Supplies,Paper,Asia Pacific,Tokyo,2025-08-02,2025-08-04,5,12.50,0.00,62.50,8.00,JPY\nORD-10004,Hans Mueller,Standing Desk,Furniture,Desks,Europe,Berlin,2025-08-02,2025-08-09,1,599.00,0.15,509.15,0.00,EUR\nORD-10005,Priya Sharma,Laser Printer,Technology,Machines,Asia Pacific,Mumbai,2025-08-03,2025-08-06,1,299.99,0.05,284.99,25.00,INR\nORD-10006,James Wilson,USB Hub,Technology,Accessories,North America,New York,2025-08-03,2025-08-05,3,19.99,0.00,59.97,3.99,USD\nORD-10007,Sophie Laurent,Desk Lamp,Furniture,Furnishings,Europe,Paris,2025-08-04,2025-08-06,2,45.00,0.00,90.00,7.50,EUR\nORD-10008,Chen Wei,Whiteboard,Office Supplies,Art,Asia Pacific,Shanghai,2025-08-04,2025-08-08,1,89.99,0.20,71.99,15.00,CNY\nORD-10009,Emma Brown,Filing Cabinet,Furniture,Storage,North America,Chicago,2025-08-05,2025-08-10,1,199.99,0.00,199.99,35.00,USD\nORD-10010,Raj Patel,Laptop Stand,Technology,Accessories,Asia Pacific,Mumbai,2025-08-05,2025-08-07,1,79.99,0.10,71.99,10.00,INR\nORD-10011,Maria Santos,Binder Clips,Office Supplies,Fasteners,South America,São Paulo,2025-08-06,2025-08-08,10,3.99,0.00,39.90,2.50,BRL\nORD-10012,Hans Mueller,Monitor Arm,Technology,Accessories,Europe,Berlin,2025-08-06,2025-08-09,1,149.99,0.00,149.99,12.00,EUR\nORD-10013,Yuki Tanaka,Ergonomic Keyboard,Technology,Accessories,Asia Pacific,Tokyo,2025-08-07,2025-08-09,1,129.99,0.05,123.49,8.00,JPY\nORD-10014,Sophie Laurent,Bookshelf,Furniture,Storage,Europe,Paris,2025-08-07,2025-08-14,1,279.99,0.10,251.99,40.00,EUR\nORD-10015,Emma Brown,Sticky Notes,Office Supplies,Paper,North America,Chicago,2025-08-08,2025-08-09,20,2.99,0.00,59.80,3.99,USD\nORD-10016,James Wilson,Webcam HD,Technology,Machines,North America,New York,2025-08-08,2025-08-10,1,89.99,0.00,89.99,5.99,USD\nORD-10017,Chen Wei,Desk Organizer,Office Supplies,Art,Asia Pacific,Shanghai,2025-08-09,2025-08-12,2,24.99,0.00,49.98,6.00,CNY\nORD-10018,Raj Patel,Office Chair,Furniture,Chairs,Asia Pacific,Mumbai,2025-08-09,2025-08-15,1,349.99,0.15,297.49,30.00,INR\nORD-10019,Hans Mueller,Paper Shredder,Technology,Machines,Europe,Berlin,2025-08-10,2025-08-14,1,159.99,0.00,159.99,18.00,EUR\nORD-10020,Maria Santos,Stapler,Office Supplies,Fasteners,South America,São Paulo,2025-08-10,2025-08-12,3,8.99,0.00,26.97,2.50,BRL\n")

var hrValidationCSV = []byte("Employee ID,Full Name,Department,Job Title,Level,Location,Hire Date,Annual Salary,Bonus Percent,Performance Score,Manager,Employment Status\nEMP-001,Alice Johnson,Engineering,Senior Engineer,L5,San Francisco,2019-03-15,185000,15.0,4.2,Bob Smith,Active\nEMP-002,Bob Smith,Engineering,Engineering Manager,L6,San Francisco,2017-06-01,210000,20.0,4.5,Carol Davis,Active\nEMP-003,Carol Davis,Engineering,VP Engineering,L7,San Francisco,2015-01-10,280000,25.0,4.8,David Lee,Active\nEMP-004,Diana Chen,Product,Product Manager,L5,New York,2020-07-20,165000,15.0,4.0,Edward Park,Active\nEMP-005,Edward Park,Product,Senior PM,L6,New York,2018-02-14,195000,18.0,4.3,Frank White,Active\nEMP-006,Frank White,Product,VP Product,L7,New York,2016-09-01,260000,22.0,4.6,Grace Kim,Active\nEMP-007,Grace Kim,Executive,CEO,L8,San Francisco,2014-01-01,350000,30.0,5.0,,Active\nEMP-008,Hannah Lee,Design,UX Designer,L4,Austin,2021-04-12,120000,10.0,3.8,Ivan Torres,Active\nEMP-009,Ivan Torres,Design,Design Lead,L5,Austin,2019-08-05,155000,15.0,4.1,Carol Davis,Active\nEMP-010,Jack Brown,Engineering,Junior Engineer,L3,San Francisco,2023-01-09,115000,8.0,3.5,Alice Johnson,Active\nEMP-011,Karen Wu,Sales,Account Executive,L4,Chicago,2022-05-16,95000,12.0,3.9,Larry Green,Active\nEMP-012,Larry Green,Sales,Sales Manager,L5,Chicago,2018-11-20,145000,18.0,4.4,Frank White,Active\nEMP-013,Mike Patel,Engineering,DevOps Engineer,L4,San Francisco,2021-10-03,140000,12.0,4.0,Bob Smith,Active\nEMP-014,Nina Reyes,Marketing,Marketing Specialist,L3,New York,2023-06-15,85000,8.0,3.6,Oscar Hill,Active\nEMP-015,Oscar Hill,Marketing,Marketing Manager,L5,New York,2019-04-22,155000,15.0,4.2,Edward Park,Active\nEMP-016,Paula Scott,Engineering,Senior Engineer,L5,Austin,2020-02-28,175000,15.0,4.3,Bob Smith,Active\nEMP-017,Quinn Adams,HR,HR Coordinator,L3,Chicago,2022-09-12,72000,8.0,3.7,Rachel Ng,Active\nEMP-018,Rachel Ng,HR,HR Director,L6,Chicago,2017-03-08,185000,18.0,4.5,Grace Kim,Active\nEMP-019,Sam Taylor,Engineering,ML Engineer,L5,San Francisco,2020-11-15,195000,15.0,4.4,Bob Smith,Resigned\nEMP-020,Tina Vo,Sales,Sales Rep,L3,Chicago,2023-08-01,78000,10.0,3.3,Larry Green,Active\n")

// ============================================================================
// 1. JIRA CSV VALIDATION
// ============================================================================

func TestValidateJiraDiscovery(t *testing.T) {
	config, err := DiscoverFromCSV(jiraValidationCSV)
	if err != nil {
		t.Fatalf("DiscoverFromCSV failed: %v", err)
	}

	dimKeys := config.DimensionKeys()
	measKeys := config.MeasureKeys()

	// Dimension classification
	expectedDims := []string{"issue_type", "status", "priority", "assignee", "reporter", "component", "sprint", "created", "resolved", "labels"}
	for _, key := range expectedDims {
		v6AssertContains(t, dimKeys, key, key+" should be a dimension")
	}

	// Measure classification (the key fix)
	expectedMeas := []string{"story_points", "original_estimate_(hours)", "time_spent_(hours)", "record_count"}
	for _, key := range expectedMeas {
		v6AssertContains(t, measKeys, key, key+" should be a measure")
	}

	// Story Points should NOT be a dimension
	v6AssertNotContains(t, dimKeys, "story_points", "story_points should be a measure, not a dimension")

	// Unit detection
	for _, m := range config.Measures {
		switch m.Key {
		case "story_points":
			v6AssertEqual(t, m.Unit, "points", "story_points unit")
			v6AssertEqual(t, m.DefaultAggregation, "avg", "story_points default agg")
		case "original_estimate_(hours)":
			v6AssertEqual(t, m.Unit, "hours", "original_estimate unit")
		case "time_spent_(hours)":
			v6AssertEqual(t, m.Unit, "hours", "time_spent unit")
		}
	}

	// Temporal detection
	for _, d := range config.Dimensions {
		if d.Key == "created" || d.Key == "resolved" {
			if !d.IsTemporal {
				t.Errorf("%s should be temporal", d.Key)
			}
		}
	}

	// Hierarchy: created->sprint should exist
	for _, d := range config.Dimensions {
		if d.Key == "created" {
			v6AssertEqual(t, d.Parent, "sprint", "created should have parent=sprint")
		}
	}

	// No false numeric hierarchies
	for _, m := range config.Measures {
		if m.Key == "original_estimate_(hours)" || m.Key == "story_points" {
			// These are measures now, so they should not appear as dimensions with parents
			for _, d := range config.Dimensions {
				if d.Key == m.Key {
					t.Errorf("%s should be a measure, not a dimension", m.Key)
				}
			}
		}
	}

	// Skipped columns
	skippedNames := make([]string, len(config.SkippedColumns))
	for i, s := range config.SkippedColumns {
		skippedNames[i] = s.Column
	}
	v6AssertContainsStr(t, skippedNames, "Issue Key", "Issue Key should be skipped")
	v6AssertContainsStr(t, skippedNames, "Summary", "Summary should be skipped")

	t.Logf("Jira: %d dims, %d measures, %d skipped", len(config.Dimensions), len(config.Measures), len(config.SkippedColumns))
}

// ============================================================================
// 2. E-COMMERCE CSV VALIDATION
// ============================================================================

func TestValidateEcommerceDiscovery(t *testing.T) {
	config, err := DiscoverFromCSV(ecommerceValidationCSV)
	if err != nil {
		t.Fatalf("DiscoverFromCSV failed: %v", err)
	}

	dimKeys := config.DimensionKeys()
	measKeys := config.MeasureKeys()

	// Dimension classification
	expectedDims := []string{"category", "sub_category", "region", "city"}
	for _, key := range expectedDims {
		v6AssertContains(t, dimKeys, key, key+" should be a dimension")
	}

	// Measure classification
	expectedMeas := []string{"quantity", "unit_price", "revenue", "shipping_cost"}
	for _, key := range expectedMeas {
		v6AssertContains(t, measKeys, key, key+" should be a measure")
	}

	// Currency detection
	if config.Currency == nil || !config.Currency.Enabled {
		t.Error("Currency should be detected (Currency column has ISO codes)")
	} else {
		v6AssertEqual(t, config.Currency.CodeDimension, "currency", "currency code dimension")
	}

	// Temporal detection
	temporalKeys := []string{}
	for _, d := range config.Dimensions {
		if d.IsTemporal {
			temporalKeys = append(temporalKeys, d.Key)
		}
	}
	v6AssertContainsStr(t, temporalKeys, "order_date", "order_date should be temporal")
	v6AssertContainsStr(t, temporalKeys, "ship_date", "ship_date should be temporal")

	// Hierarchy: sub_category -> category
	for _, d := range config.Dimensions {
		if d.Key == "sub_category" {
			v6AssertEqual(t, d.Parent, "category", "sub_category should have parent=category")
		}
	}

	// Unit detection
	for _, m := range config.Measures {
		switch m.Key {
		case "revenue":
			v6AssertEqual(t, m.Unit, "currency", "revenue unit")
			if !m.IsCurrency {
				t.Error("revenue should have IsCurrency=true")
			}
		case "shipping_cost":
			v6AssertEqual(t, m.Unit, "currency", "shipping_cost unit")
		case "unit_price":
			v6AssertEqual(t, m.Unit, "currency", "unit_price unit")
		case "quantity":
			v6AssertEqual(t, m.Unit, "units", "quantity unit")
		}
	}

	// Skipped: Order ID
	skippedNames := make([]string, len(config.SkippedColumns))
	for i, s := range config.SkippedColumns {
		skippedNames[i] = s.Column
	}
	v6AssertContainsStr(t, skippedNames, "Order ID", "Order ID should be skipped")

	t.Logf("E-commerce: %d dims, %d measures, %d skipped, currency=%v",
		len(config.Dimensions), len(config.Measures), len(config.SkippedColumns),
		config.Currency != nil && config.Currency.Enabled)
}

// ============================================================================
// 3. HR CSV VALIDATION
// ============================================================================

func TestValidateHRDiscovery(t *testing.T) {
	config, err := DiscoverFromCSV(hrValidationCSV)
	if err != nil {
		t.Fatalf("DiscoverFromCSV failed: %v", err)
	}

	dimKeys := config.DimensionKeys()
	measKeys := config.MeasureKeys()

	// Dimension classification
	expectedDims := []string{"department", "job_title", "level", "location", "employment_status"}
	for _, key := range expectedDims {
		v6AssertContains(t, dimKeys, key, key+" should be a dimension")
	}

	// Measure classification
	expectedMeas := []string{"annual_salary", "bonus_percent", "performance_score"}
	for _, key := range expectedMeas {
		v6AssertContains(t, measKeys, key, key+" should be a measure")
	}

	// Salary unit
	for _, m := range config.Measures {
		if m.Key == "annual_salary" {
			v6AssertEqual(t, m.Unit, "currency", "annual_salary unit")
			if !m.IsCurrency {
				t.Error("annual_salary should have IsCurrency=true")
			}
		}
		if m.Key == "bonus_percent" {
			v6AssertEqual(t, m.Unit, "percent", "bonus_percent unit")
			v6AssertEqual(t, m.DefaultAggregation, "avg", "bonus_percent default agg")
		}
		if m.Key == "performance_score" {
			v6AssertEqual(t, m.Unit, "points", "performance_score unit")
			v6AssertEqual(t, m.DefaultAggregation, "avg", "performance_score default agg")
		}
	}

	// Temporal: hire_date
	for _, d := range config.Dimensions {
		if d.Key == "hire_date" {
			if !d.IsTemporal {
				t.Error("hire_date should be temporal")
			}
		}
	}

	// Skipped: Employee ID, Full Name
	skippedNames := make([]string, len(config.SkippedColumns))
	for i, s := range config.SkippedColumns {
		skippedNames[i] = s.Column
	}
	v6AssertContainsStr(t, skippedNames, "Employee ID", "Employee ID should be skipped")
	v6AssertContainsStr(t, skippedNames, "Full Name", "Full Name should be skipped")

	t.Logf("HR: %d dims, %d measures, %d skipped",
		len(config.Dimensions), len(config.Measures), len(config.SkippedColumns))
}

// ============================================================================
// 4. CROSS-DOMAIN CONSISTENCY
// ============================================================================

func TestValidateSyntheticRecordCount(t *testing.T) {
	datasets := map[string][]byte{
		"Jira":      jiraValidationCSV,
		"Ecommerce": ecommerceValidationCSV,
		"HR":        hrValidationCSV,
	}
	for name, data := range datasets {
		config, err := DiscoverFromCSV(data)
		if err != nil {
			t.Fatalf("%s: DiscoverFromCSV failed: %v", name, err)
		}
		found := false
		for _, m := range config.Measures {
			if m.Key == "record_count" && m.IsSynthetic {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: synthetic record_count measure missing", name)
		}
	}
}

func TestValidateNoEmptyDimensions(t *testing.T) {
	datasets := map[string][]byte{
		"Jira":      jiraValidationCSV,
		"Ecommerce": ecommerceValidationCSV,
		"HR":        hrValidationCSV,
	}
	for name, data := range datasets {
		config, err := DiscoverFromCSV(data)
		if err != nil {
			t.Fatalf("%s: DiscoverFromCSV failed: %v", name, err)
		}
		for _, d := range config.Dimensions {
			if d.Key == "" {
				t.Errorf("%s: dimension with empty key", name)
			}
			if d.DisplayName == "" {
				t.Errorf("%s: dimension %s has empty display name", name, d.Key)
			}
			if len(d.SampleValues) == 0 {
				t.Errorf("%s: dimension %s has no sample values", name, d.Key)
			}
		}
	}
}

// ============================================================================
// HELPERS
// ============================================================================

func v6AssertContains(t *testing.T, slice []string, val string, msg string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			return
		}
	}
	t.Errorf("%s -- %q not found in %v", msg, val, slice)
}

func v6AssertNotContains(t *testing.T, slice []string, val string, msg string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			t.Errorf("%s -- %q should not be in %v", msg, val, slice)
			return
		}
	}
}

func v6AssertContainsStr(t *testing.T, slice []string, val string, msg string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			return
		}
	}
	t.Errorf("%s -- %q not found in %v", msg, val, slice)
}

func v6AssertEqual(t *testing.T, got, want, msg string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", msg, got, want)
	}
}