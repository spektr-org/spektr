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
			val := strings.ToLower(view.Dimension(i, dim))
			if !set[val] {
				pass = false
				break
			}
		}
		if pass {
			indices = append(indices, i)
		}
	}

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