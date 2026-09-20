package engine

// Virtual dimensions on the FILTER path.
//
// "year" is derived from "month" — no adapter defines an accessor for it.
// Grouping has always resolved it through getDimensionValue; filtering read
// view.Dimension directly, so a dataset could be GROUPED by year and not
// FILTERED by it.
//
// TPL staging, 20 Sep: "show only from 2018 to 2022" answered "I can't see any
// data for that period" over a chart plainly showing those years. Every
// record's "year" read as empty, so nothing matched.
//
// The asymmetry survived because the two paths live in different files and
// only one of them was ever wrong.

import "testing"

func yearsView() RecordView {
	return NewSliceView([]Record{
		{Dimensions: map[string]string{"month": "Mar-2018", "category": "Expense"},
		 Measures: map[string]float64{"amount": 100}},
		{Dimensions: map[string]string{"month": "Jul-2021", "category": "Expense"},
		 Measures: map[string]float64{"amount": 200}},
		{Dimensions: map[string]string{"month": "Jan-2026", "category": "Expense"},
		 Measures: map[string]float64{"amount": 400}},
	})
}

func TestApplyFilters_FiltersOnTheVirtualYearDimension(t *testing.T) {
	out := ApplyFilters(yearsView(), Filters{
		Dimensions: map[string][]string{"year": {"2018", "2021"}},
	})

	if out.Len() != 2 {
		t.Fatalf("matched %d records, want 2 — year resolved as empty", out.Len())
	}
	if total := SumMeasure(out, "amount"); total != 300 {
		t.Errorf("total = %.0f, want 300", total)
	}
}

func TestApplyFilters_YearRangeExcludesWhatIsOutsideIt(t *testing.T) {
	// The half that matters as much: a year filter must LEAVE OUT the years it
	// does not name. Matching everything would look like it worked.
	out := ApplyFilters(yearsView(), Filters{
		Dimensions: map[string][]string{"year": {"2026"}},
	})

	if out.Len() != 1 || SumMeasure(out, "amount") != 400 {
		t.Errorf("matched %d records totalling %.0f, want 1 and 400",
			out.Len(), SumMeasure(out, "amount"))
	}
}

func TestApplyFilters_YearCombinesWithOtherDimensions(t *testing.T) {
	// AND across dimensions, as every other filter does.
	out := ApplyFilters(yearsView(), Filters{
		Dimensions: map[string][]string{
			"year":     {"2018", "2021", "2026"},
			"category": {"Expense"},
		},
	})
	if out.Len() != 3 {
		t.Errorf("matched %d, want 3", out.Len())
	}
}

func TestApplyFilters_UnknownYearMatchesNothing(t *testing.T) {
	out := ApplyFilters(yearsView(), Filters{
		Dimensions: map[string][]string{"year": {"1999"}},
	})
	if out.Len() != 0 {
		t.Errorf("matched %d, want 0", out.Len())
	}
}

func TestApplyFilters_RealDimensionsStillWork(t *testing.T) {
	// getDimensionValue falls through to view.Dimension for anything that is
	// not virtual, so nothing that worked before may stop.
	out := ApplyFilters(yearsView(), Filters{
		Dimensions: map[string][]string{"month": {"Jul-2021"}},
	})
	if out.Len() != 1 || SumMeasure(out, "amount") != 200 {
		t.Errorf("month filter broke: %d records, %.0f", out.Len(), SumMeasure(out, "amount"))
	}
}