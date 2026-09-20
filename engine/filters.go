package engine

import (
	"strings"
)

// ============================================================================
// FILTERS — Generic Dimension-Based Filtering via RecordView
// ============================================================================
// Single-pass filter: checks ALL dimension constraints per record in one loop.
// Returns a SubView (index list into parent) — zero data copy.
// ============================================================================

// ApplyFilters returns a view of records matching all dimension filters.
// Dimensions are AND-combined; values within a dimension are OR-combined.
// Empty filter = no restriction (returns original view).
func ApplyFilters(view RecordView, filters Filters) RecordView {
	if filters.IsEmpty() {
		return view
	}

	// Pre-build lowercase lookup sets for each dimension filter
	sets := make(map[string]map[string]bool)
	for dim, allowed := range filters.Dimensions {
		if len(allowed) > 0 {
			sets[dim] = toLowerSet(allowed)
		}
	}

	if len(sets) == 0 {
		return view
	}

	// Single pass — record passes if it matches ALL dimension filters
	n := view.Len()
	indices := make([]int, 0, n)
	for i := 0; i < n; i++ {
		pass := true
		for dim, set := range sets {
			// getDimensionValue, not view.Dimension: it knows the VIRTUAL
			// dimensions — "year", derived from "month" — that no adapter
			// defines an accessor for.
			//
			// Grouping has always gone through it; filtering did not, so a
			// dataset could be GROUPED by year and not FILTERED by it. Asking
			// for 2018–2022 read every record's "year" as empty, matched
			// nothing, and answered "no data for that period" over a chart
			// plainly showing those years. TPL staging, 20 Sep.
			//
			// The asymmetry was invisible because the two paths are in
			// different files and only one of them was ever wrong.
			val := strings.ToLower(getDimensionValue(view, i, dim))
			if !set[val] {
				pass = false
				break
			}
		}
		if pass {
			indices = append(indices, i)
		}
	}

	// ── EVIDENCE: filtering is an in-memory scan producing an index list.
	// No query string (SQL or otherwise) is constructed — there is nothing to log
	// as a "query" because none exists. ──
	trace("FILTER (in-memory scan, NO query generated): %d records -> %d matched", n, len(indices))

	return newSubView(view, indices)
}

// toLowerSet converts a string slice to a lowercase lookup set.
func toLowerSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[strings.ToLower(item)] = true
	}
	return set
}