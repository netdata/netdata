// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 3 completeness — the pre-fleet time-grouping registry
// (src/web/api/queries/query-group-over-time.c): TestLayer3Families
// covers 27 of the 48 pre-fleet accepted name strings; this file covers
// the remaining 21, the complete countif options grammar (countif.h), the
// numeric group-options overrides with their clamps (percentile
// [0,100]; trimmed-mean/median [0,50]), and the silent-fallback
// contract (an unknown time_group name parses to average, never an
// error — time_grouping_parse).
//
// Bare-number and invalid-expression behavior is covered independently by
// CASE-023, so this registry test cannot pass merely because a parser bug
// happens to produce the same value as one of its operator probes.
package corpus

import (
	"testing"
	"time"
)

func TestLayer3RegistryCompleteness(t *testing.T) {
	ch := layer3Canonical("fixture.l3reg")
	pushLiveBurst(t, "l3-reg", guid(68), ch)
	if _, err := td.WaitRetention("l3-reg", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// every registry variant not exercised by TestLayer3Families, each
	// against its parameterized oracle
	t.Run("variants", func(t *testing.T) {
		trackContract(t, "L3/registry-variants")

		for _, name := range []string{
			"trimmed-mean2", "trimmed-mean3", "trimmed-mean5",
			"trimmed-mean10", "trimmed-mean15", "trimmed-mean20",
			"trimmed-median2", "trimmed-median3", "trimmed-median5",
			"trimmed-median10", "trimmed-median15", "trimmed-median20",
			"percentile75", "percentile80", "percentile90",
			"percentile95", "percentile97", "percentile98",
		} {
			t.Run(name, func(t *testing.T) {
				verifyTimeGroup(t, "l3-reg", ch, name, "", 10)
			})
		}
	})

	// registry aliases resolve to the same grouping as a canonical name
	// that TestLayer3Families already proves against the oracle
	t.Run("aliases", func(t *testing.T) {
		trackContract(t, "L3/registry-aliases")

		for _, tc := range []struct{ sent, canonical string }{
			{"incremental_sum", "incremental-sum"},
			{"ewma", "ses"},
			{"coefficient-of-variation", "cv"},
		} {
			t.Run(tc.sent, func(t *testing.T) {
				verifyTimeGroupAs(t, "l3-reg", ch, tgQuery{Name: tc.sent, OracleName: tc.canonical}, 10)
			})
		}
	})

	// the shared expression grammar beyond the spellings TestLayer3Families
	// sends: '<>' is NOT-EQUAL, '>:'/'<:' are >=/<=, ':' and '==' are
	// EQUAL, spaces are skipped around the operator, empty
	// options compare EQUAL against 0.0, and a bare number compares EQUAL
	// against itself (it used to lose its first digit)
	t.Run("countif-grammar", func(t *testing.T) {
		trackContract(t, "L3/registry-countif-grammar")

		for _, opts := range []string{
			">:30", "<:20", "<>1", ":40", "==40",
			"40", "", "  <>  1",
		} {
			t.Run("countif("+opts+")", func(t *testing.T) {
				verifyTimeGroup(t, "l3-reg", ch, "countif", opts, 10)
			})
		}
	})

	// numeric group-options override the named default and clamp:
	// percentile to [0,100], trimmed-mean/median to [0,50]; negative
	// and unparsable values collapse to 0
	t.Run("options-clamping", func(t *testing.T) {
		trackContract(t, "L3/registry-option-clamping")

		for _, tc := range []struct{ name, options string }{
			{"percentile", "10"},
			{"percentile", "150"}, // clamps to 100 → plain average
			{"percentile", "-20"}, // clamps to 0 → single-slot walk
			{"trimmed-mean", "40"},
			{"trimmed-mean", "60"}, // clamps to 50 → narrowest window
			{"trimmed-mean", "-1"}, // clamps to 0 → plain average
			{"trimmed-median", "50"},
		} {
			t.Run(tc.name+"("+tc.options+")", func(t *testing.T) {
				verifyTimeGroup(t, "l3-reg", ch, tc.name, tc.options, 10)
			})
		}
	})

	// unknown names never error: time_grouping_parse silently falls
	// back to average — pinned
	t.Run("unknown-name-fallback", func(t *testing.T) {
		trackContract(t, "L3/registry-unknown-fallback")

		verifyTimeGroupAs(t, "l3-reg", ch, tgQuery{Name: "no-such-grouping", OracleName: "average"}, 10)
	})
}
