// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import "testing"

// Status is a manifest entry's expected outcome on the agent under test.
type Status string

const (
	// Green means the case must pass: a failure is a regression.
	Green Status = "green"
	// Red means the case pins a KNOWN BUG: it "passes" while the bug
	// reproduces, and demands a manifest flip when a fix lands.
	Red Status = "red"
)

// ManifestCase tracks one corpus case: what it proves and its expected
// agent status. MANIFEST.md mirrors this table for humans.
type ManifestCase struct {
	Name    string
	Proves  string
	Agent   Status
	FixedBy string // PR or commit that flipped it green
}

var manifest = map[string]ManifestCase{
	"L0/live-burst": {
		Proves: "live BEGIN2/SET2 burst round-trips byte-exact without settle discipline (pins #23096 green)",
		Agent:  Green, FixedBy: "#23096",
	},
	"L0/live-paced": {
		Proves: "legacy spike-3 pacing still works (control for live-burst)",
		Agent:  Green,
	},
	"L0/replication": {
		Proves: "replication dialogue round-trips byte-exact incl. gap and anomaly bits",
		Agent:  Green,
	},
	"L0/two-children": {
		Proves: "same context from two children answers independently per host",
		Agent:  Green,
	},
	"L0/labels": {
		Proves: "CLABEL chart labels reach the query path (group_by=label)",
		Agent:  Green,
	},
	"L0/restart": {
		Proves: "fixtures survive daemon restart byte-identical (journal-v2 read path)",
		Agent:  Green,
	},
	"L1/palette": {
		Proves: "tier0 ingestion identity for the edge-data palette: complete, leading/interior-run gaps, trailing short retention, reset (AR and lone-R), anomaly runs, negatives, all-zero, update_every=5",
		Agent:  Green,
	},
	"L1/single-point": {
		Proves: "single-point ingestion exact through a wide window; PINS the 1-point-window view expansion (ue 1→2, bucket at t0+2) as a layer-9 seed",
		Agent:  Green,
	},
	"L1/trailing-window": {
		Proves: "beyond-retention reads return null points at the fixed epoch (no now-trimming)",
		Agent:  Green,
	},
	"L1/precision": {
		Proves: "storage_number quantization contract: engine values equal the Go pack/unpack port (fixture.SNRoundTrip) within JSON print tolerance",
		Agent:  Green,
	},
	"L1/gap-states": {
		Proves: "the three gap-only dimension states per the #23095 working-as-intended ruling: live phantom retention, gone after restart, back on next iteration",
		Agent:  Green, FixedBy: "ruling #23095",
	},
	"L2/tier1-palette": {
		Proves: "tier1 rollup identity for the edge-data palette: aligned windows (partial first — T0 unaligned), interior gaps (partial counts + stored-empty windows), fractional anomaly rates, reset annotation LOST at tier1+ (pages store no flags), float32 sum/min/max write-rounding",
		Agent:  Green,
	},
	"L2/whole-chart-absence": {
		Proves: "never-stored tier windows (whole-chart gap) read identically to stored-empty windows: null + EMPTY annotation, with correct partial counts on the flanking windows",
		Agent:  Green,
	},
	"L2/sn-vs-original": {
		Proves: "tiers aggregate the ORIGINAL collected doubles, not tier0 storage_number-quantized values: 2^24+1 reads 16777220 at tier0 (decimal mantissa step) but f32(16777216)-derived at tier1",
		Agent:  Green,
	},
	"L2/update-every-5": {
		Proves: "tier grid arithmetic scales with chart update_every: granularity ue×grouping, aligned ends, partial first window",
		Agent:  Green,
	},
	"L2/tier2": {
		Proves: "second-level rollup (granularity 3600) over replicated history incl. a gap run, with tier1 cross-checked on identical data across the gap boundary",
		Agent:  Green,
	},
	"L3/families": {
		Proves: "every registry time_group equals its Go oracle over the mixed palette at group 10 (all-gap bucket, anomaly run, reset): average/sum/min/max, extremes, stddev/cv (Welford, sample variance, single-value=0), median + trimmed-median (value-range trim + R-7 quantile), percentile/trimmed-mean (slot-window means with fractional interpolation), ses/des (running state across buckets), incremental-sum (carry), countif (options grammar)",
		Agent:  Green,
	},
	"L3/sign-semantics": {
		Proves: "percentile/trimmed-mean walk from the top when a bucket has any negative value; extremes champions by |abs|; pinned over all-negative and mixed-sign fixtures",
		Agent:  Green,
	},
	"L3/sparse-buckets": {
		Proves: "single-numeric-value buckets: stddev 0.0 (not null), pass-through for value families, incremental-sum all null (leading bucket loses its seed; empty resets the carry) — pinned current contract",
		Agent:  Green,
	},
	"L3/identity-smoothing": {
		Proves: "ses/des at group=1 use the requested points (capped 15) as the smoothing window; incremental-sum at identity is all null — pinned current contract",
		Agent:  Green,
	},
	"L4/family-tier-matrix": {
		Proves: "every grouping family over FORCED tier1 with 6 windows per bucket equals the fetch-aware oracle (min/max/sum fetch their tier fields, all else the per-window average — avg-of-averages pinned quantitatively with unequal counts); bucket anomaly rates from tier counts; window alignment rounds `before` UP to group multiples",
		Agent:  Green,
	},
	"L4/auto-tier-selection": {
		Proves: "with no tier param the planner picks the COARSEST tier whose density is acceptable (>= HALF the wanted points, wanted floored at QUERY_PLAN_MIN_POINTS=10): 1s buckets from tier0, 60s from tier1, and even 3600s buckets from tier1 while it covers (tier2 needs >= 5h windows — 5 x 3600s — or coverage gaps); db.per_tier points-read pinned exclusive; values equal the serving tier's oracle",
		Agent:  Green,
	},
	"L4/minmax-absolute-semantics": {
		Proves: "time_group=min returns the value CLOSEST to zero and max the value FURTHEST from zero (min.h/max.h fabs comparisons) — visible only on negative/mixed data; pinned green in L3 sign-semantics + L4 matrix; RULING PENDING (arithmetic min/max would be a behavior change; extremes already provides champion-by-abs)",
		Agent:  Green,
	},
	"CASE-017/tier-boundary-absorption": {
		Proves: "a tier>0 query whose after equals a stored tier point end keeps that point out of the first bucket (was: absorbed, leaking pre-window data into (after, before] — the backward-expanded storage scan met the inclusive bucket-start check); tier0 control stays clean",
		Agent:  Green, FixedBy: "#23127",
	},
	"CASE-016/fresh-host-forgotten-on-restart": {
		Proves: "a child first connected < one metadata scan cycle (5s) before a graceful restart is forgotten: pending host metadata is not flushed at shutdown (sqlite_metadata.c metasync shutdown path), host 404s after boot, dbengine data orphaned",
		Agent:  Red,
	},
	"CASE-015/live-disconnect-discard": {
		Proves: "receiver drains delivered live data before honoring HUP: a child disconnecting right after writing loses nothing (was: up to the whole burst discarded)",
		Agent:  Green, FixedBy: "#23118",
	},
	"CASE-015/replication-disconnect-discard": {
		Proves: "same drain guarantee on the replication path: a child disconnecting after its final REND loses nothing",
		Agent:  Green, FixedBy: "#23118",
	},
}

// expectAgentStatus applies the manifest contract: a Green case must have
// passed; a Red case must still reproduce its bug — a Red case that stops
// reproducing fails loudly so the manifest gets flipped with the fix.
func expectAgentStatus(t *testing.T, name string, observedPass bool) {
	t.Helper()
	mc, ok := manifest[name]
	if !ok {
		t.Fatalf("case %q missing from manifest", name)
	}
	switch mc.Agent {
	case Green:
		if !observedPass {
			t.Errorf("MANIFEST %s: expected GREEN, observed failing — regression (proves: %s, fixed-by: %s)",
				name, mc.Proves, mc.FixedBy)
		}
	case Red:
		if observedPass {
			t.Errorf("MANIFEST %s: expected RED but the bug no longer reproduces — "+
				"if a fix landed, flip this case to green with its PR (proves: %s)", name, mc.Proves)
		} else {
			t.Logf("MANIFEST %s: still RED as expected (proves: %s)", name, mc.Proves)
		}
	}
}
