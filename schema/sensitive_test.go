package schema

import "testing"

// TestSensitivePIISuppression pins the privacy-critical behaviour that PII
// dimension VALUES are suppressed regardless of cardinality.
//
// This is the regression that a cardinality-based heuristic silently misses:
// a small users table where names REPEAT (low cardinality) must STILL suppress
// name/email/picture values. If this test ever fails, PII is crossing to the AI.
func TestSensitivePIISuppression(t *testing.T) {
	// 6 rows, names deliberately REPEAT → low cardinality (3 distinct names).
	// A cardinality/uniqueness rule would NOT catch these; the name/type policy must.
	// All values below are synthetic placeholders — no real personal data in tests.
	usersCSV := []byte("id,name,email,picture_url,subscription_tier\n" +
		"1,Test User A,usera@example.test,https://example.test/a,free\n" +
		"2,Test User A,usera@example.test,https://example.test/a,free\n" +
		"3,Test User B,userb@example.test,https://example.test/b,pro\n" +
		"4,Test User B,userb@example.test,https://example.test/b,pro\n" +
		"5,Test User C,userc@example.test,https://example.test/c,free\n" +
		"6,Test User C,userc@example.test,https://example.test/c,free\n")

	config, err := DiscoverFromCSV(usersCSV, DiscoverOptions{SampleSize: 1000})
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}

	// Find each dimension and assert PII ones are marked Sensitive with no values.
	dims := map[string]DimensionMeta{}
	for _, d := range config.Dimensions {
		dims[d.Key] = d
	}

	for _, key := range []string{"name", "email", "picture_url"} {
		d, ok := dims[key]
		if !ok {
			// Acceptable if it was skipped entirely (e.g. as unique) — that also
			// means no values cross. Only fail if it's present WITH values.
			continue
		}
		if !d.Sensitive {
			t.Errorf("%q should be marked Sensitive (PII)", key)
		}
		if len(d.SampleValues) != 0 {
			t.Errorf("%q must have NO sample values (PII leak): got %v", key, d.SampleValues)
		}
		// NOTE: sensitive columns remain Groupable — grouping/filtering is a local
		// engine operation on real values; only VALUE TRANSMISSION to the AI is
		// suppressed. So we do NOT assert !Groupable here.
	}

	// subscription_tier is NOT PII → should remain a normal dimension with values.
	if tier, ok := dims["subscription_tier"]; ok {
		if tier.Sensitive {
			t.Errorf("subscription_tier should NOT be marked sensitive")
		}
		if len(tier.SampleValues) == 0 {
			t.Errorf("subscription_tier should retain its sample values (free/pro)")
		}
	}
}