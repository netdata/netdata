// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
)

// ManifestCase describes one corpus case. It records what the case proves
// and, once a fix has landed, which PR delivered it.
//
// It deliberately records NO expected outcome. A contract either holds or
// it does not, and the only thing that can answer that is running it. A
// hardcoded "we know this one is broken" would make a broken engine report
// success, and the whole point of this corpus is to name what is broken.
type ManifestCase struct {
	Proves     string
	Cloud      string   // defaults to n/a
	FixedBy    string   // PR or commit that fixed it
	Components []string // required independent test scopes; empty means one scope
}

var manifest = map[string]ManifestCase{
	"L0/live-burst": {
		Proves:  "a live BEGIN2/SET2 burst sent without historical spike-3 pre-burst pacing preserves exactly the fixture dimensions, timestamps, storage-number-quantized values, gaps, anomaly rates and annotations after the retention barrier (pins #23096 green)",
		FixedBy: "#23096",
	},
	"L0/live-paced": {
		Proves: "legacy spike-3 pacing still works (control for live-burst)",
	},
	"L0/replication": {
		Proves: "a replication dialogue preserves exactly the fixture dimensions, timestamps, storage-number-quantized values, gaps, anomaly rates and annotations",
	},
	"L0/two-children": {
		Proves: "same context from two children answers independently per host",
	},
	"L0/labels": {
		Proves: "CLABEL chart labels reach the query path (group_by=label)",
	},
	"L0/restart": {
		Proves: "fixtures produce the same typed fixture-derived readback after daemon restart and journal-v2 replay",
	},
	"CASE-015/live-disconnect-discard": {
		Proves:  "receiver drains delivered live data before honoring HUP: a child disconnecting right after writing loses nothing (was: up to the whole burst discarded)",
		FixedBy: "#23118",
	},
	"CASE-015/replication-disconnect-discard": {
		Proves:  "same drain guarantee on the replication path: a child disconnecting after its final REND loses nothing",
		FixedBy: "#23118",
	},
	"CASE-015/robustness": {
		Proves:     "receiver teardown with queued replies to a dead child stays crash-free: mid-dialogue disconnect and a 30-cycle disconnect soak",
		Components: []string{"mid-dialogue", "disconnect-soak"},
	},
	"L1/palette": {
		Proves: "tier0 ingestion identity for the edge-data palette: complete, leading/interior-run gaps, trailing short retention, reset (AR and lone-R), anomaly runs, negatives, all-zero, update_every=5",
	},
	"L1/single-point": {
		Proves: "single-point ingestion is exact through a window wider than retention: the value stays at its stored timestamp and surrounding rows stay empty. CASE-034 separately pins the public API timestamp grid, including one-point queries",
	},
	"L1/trailing-window": {
		Proves: "beyond-retention reads return null points at the fixed epoch (no now-trimming)",
	},
	"L1/precision": {
		Proves: "storage_number quantization contract: engine values equal the Go pack/unpack port (fixture.SNRoundTrip) within JSON print tolerance",
	},
	"L1/gap-states": {
		Proves:  "the three gap-only dimension states per the #23095 working-as-intended ruling: live phantom retention, gone after restart, back on next iteration",
		FixedBy: "ruling #23095",
	},
	"L1/storage-backend-gap-state": {
		Proves:     "every tier-0 storage backend normalizes the same numeric-gap-numeric stream identically: exact request-derived timestamp grids, EMPTY only on gaps, PARTIAL on mixed SUM rows, database statistics that exclude gaps, and the same answers from DBENGINE Gorilla and raw pages both hot and after restart plus RAM and ALLOC child storage",
		Components: []string{"dbengine-gorilla-hot", "dbengine-gorilla-restart", "dbengine-raw-hot", "dbengine-raw-restart", "ram", "alloc"},
	},
	"L1/incremental-rates": {
		Proves:     "the db stores PER-SECOND rates regardless of update_every: a v1 child's raw counters through the parent's rrdset_done yield K*(mul/div)/UE per second (incremental at ue 1/2/5 incl. mul/div scaling; absolute control unscaled)",
		Components: []string{"rates-ue1-2-5", "rate-ue10"},
	},
	"L1/reset-implausible-backward-step": {
		Proves: "an implausible incremental-counter backward step contributes zero rate mass and stamps exactly one RESET row",
	},
	"L1/reset-plausible-32bit-wrap": {
		Proves: "a plausible 32-bit counter wrap reconstructs the cap-relative delta of 99 (the established one-less-than-modulo quirk) and stamps exactly one RESET row",
	},
	"L1/reset-percentage-row": {
		Proves: "percentage-of-incremental-row absorbs one dimension's reset as a zero contribution with RESET while the surviving dimension takes the remaining 100% without RESET",
	},
	"L1/reset-after-gap": {
		Proves: "a silence beyond the collection-gap threshold restarts an incremental counter cleanly, leaving null gap rows without an across-gap rate spike or RESET annotation",
	},
	"L1/subsecond-rate-blending": {
		Proves:     "a paced special sample is blended into at most two adjacent stored rows within its endpoint range",
		Components: []string{"implausible-backward-step", "plausible-32bit-wrap", "pcent-over-diff-reset"},
	},
	"L1/off-grid-timestamps": {
		Proves: "samples pushed OFF the absolute ue grid: STORAGE keeps the pushed timestamps exactly (retention proves it), while every VIEW re-grids to absolute ue multiples and serves boundary-INTERPOLATED values (envelope-pinned; the exact virtual-point oracle is layer-9 work)",
	},
	"L2/tier1-complete": {
		Proves: "complete tier1 rollups preserve the fixture's aligned numeric values and exact window geometry",
	},
	"L2/tier1-interior-gaps": {
		Proves: "tier1 interior gaps produce PARTIAL numeric windows and EMPTY stored-gap windows at their exact aligned positions",
	},
	"L2/tier1-anomaly-rate": {
		Proves: "tier1 rollups report the exact fractional anomaly rate derived from anomaly_count divided by count",
	},
	"L2/tier1-reset-flags": {
		Proves: "tier1 pages do not retain tier0 RESET annotations because the higher-tier page format stores no point flags",
	},
	"L2/tier1-float32-fields": {
		Proves: "tier1 sum, minimum and maximum fields match the ARRAY_TIER1 float32 write-rounding contract",
	},
	"L2/partial-wide-point": {
		Proves: "one partial higher-tier record projected into six finer result rows marks every numeric derived row exactly PARTIAL while preserving the requested timestamp grid; PARTIAL is evidence state, not a one-time event on the source record",
	},
	"L2/whole-chart-absence": {
		Proves: "never-stored tier windows (whole-chart gap) read identically to stored-empty windows: null + EMPTY annotation, with correct partial counts on the flanking windows",
	},
	"L2/tier0-storage-number-quantization": {
		Proves: "tier0 reads the value 2^24+1 through the established storage_number decimal-mantissa quantization path",
	},
	"L2/tier-rollup-original-values": {
		Proves: "higher tiers aggregate the original collected double for 2^24+1 before applying the tier page's float32 field encoding, rather than aggregating the tier0 storage_number value",
	},
	"L2/update-every-5": {
		Proves: "tier grid arithmetic scales with chart update_every: granularity ue×grouping, aligned ends, partial first window",
	},
	"L2/tier2": {
		Proves: "second-level rollup (granularity 3600) over replicated history incl. a gap run, with tier1 cross-checked on identical data across the gap boundary",
	},
	"L2/update-every-sweep": {
		Proves: "ue {10,30,60,600,3600} over the backdated v2 protocol: tier0 identity, tier1 windows on the scaled grid (gran = ue x 60, absolute alignment, partial counts, stored-empty family, fractional anomaly rates), time-group buckets in BOTH grid modes (default: bucket ends snap to absolute multiples of group x ue with `after` rounded UP; unaligned: grid anchored at `after`); paced v1 rate contract extended to ue=10",
	},
	"L2/historical-tier-grouping": {
		Proves:     "a persisted higher-tier point keeps its original complete or PARTIAL state after the configured tier grouping changes: a dense count-4 rollup remains clean after grouping 4 to 8, and a gapped count-4-of-8 rollup remains PARTIAL after grouping 8 to 4; legacy V1 pages cannot satisfy this because they do not store the historical slot invariant",
		Components: []string{"complete-4-to-8", "partial-8-to-4"},
	},
	"L2/v1-rollup-count-65536": {
		Proves: "complete higher-tier rollups containing 65,536 source samples remain numeric after persistence and restart: two consecutive records isolate the stored uint16 count wrap from the separate singleton-page cadence ambiguity, and forced tier 1 projects one record onto an exact 256-by-256-second request grid with value 256 in every clean row and min/avg/max exactly 1",
	},
	"CASE-017/tier-boundary-absorption": {
		Proves:  "a tier>0 query whose after equals a stored tier point end keeps that point out of the first bucket (was: absorbed, leaking pre-window data into (after, before] — the backward-expanded storage scan met the inclusive bucket-start check); tier0 control stays clean",
		FixedBy: "#23127",
	},
	"CASE-016/fresh-host-forgotten-on-restart": {
		Proves:  "best-effort graceful-restart regression: a freshly connected child remains queryable after restart. The bounded 10-second attempt prevents unbounded timing drift but exceeds the ordinary 5-second metadata scan, so it does not isolate the final shutdown scan from periodic persistence (was: fresh host forgotten after boot, dbengine data orphaned)",
		FixedBy: "#23120",
	},
	"L3/family-values": {
		Proves: "the listed pre-fleet time-grouping families equal their independent Go value oracles over complete, gapped, anomalous and reset-bearing buckets",
	},
	"L3/family-annotations": {
		Proves: "every listed pre-fleet time-grouping family preserves the exact EMPTY, PARTIAL and RESET annotation expected from the source palette",
	},
	"L3/sign-semantics": {
		Proves: "percentile/trimmed-mean walk from the top when a bucket has any negative value; extremes champions by |abs|; pinned over all-negative and mixed-sign fixtures",
	},
	"L3/sparse-buckets": {
		Proves: "single-numeric-value buckets: stddev 0.0 (not null), pass-through for value families, incremental-sum measures each bucket against the one before it, so only the opening bucket - which has no predecessor - is null (a bucket holding a single sample hands that sample forward as the next bucket's baseline; an empty bucket keeps the baseline it was given)",
	},
	"L3/identity-smoothing": {
		Proves: "ses/des at group=1 use the requested points (capped 15) as the smoothing window; incremental-sum at identity answers every bucket but the first: one sample per bucket is the shape the carry exists for",
	},
	"L3/registry-variants": {
		Proves: "every accepted pre-fleet time-grouping registry name answers with its canonical implementation",
	},
	"L3/registry-aliases": {
		Proves: "each documented pre-fleet time-grouping alias produces the same answer as its canonical name",
	},
	"L3/registry-countif-grammar": {
		Proves: "countif accepts every documented operator alias plus surrounding spaces and its zero-length default expression",
	},
	"L3/registry-option-clamping": {
		Proves: "numeric grouping options honor overrides and clamp percentile to [0,100] and trimmed mean or median to [0,50]",
	},
	"L3/registry-unknown-fallback": {
		Proves: "an unknown time-grouping name retains the established silent fallback to average",
	},
	"L3/anomaly-bit-identity": {
		Proves: "options=anomaly-bit replaces tier0 identity values with each point's exact 0% or 100% anomaly rate",
	},
	"L3/anomaly-bit-buckets": {
		Proves: "time grouping consumes anomaly-bit rates as values, so average returns the bucket anomaly percentage and max reports whether any point was anomalous",
	},
	"L3/anomaly-bit-group-by": {
		Proves: "group-by consumes anomaly-bit results as values, sums rates across members and marks groups with missing contributors PARTIAL",
	},
	"L3/anomaly-bit-tier-rates": {
		Proves: "above tier0, anomaly-bit exposes each stored window's fractional 100*anomaly_count/count rate as the query value",
	},
	"L3/sum-over-time-volume": {
		Proves:     "time_group=sum has two modes: RATE-stored metrics (incremental) multiply each point by its duration — the sum is the VOLUME at any update_every; non-rate metrics sum plainly",
		Components: []string{"rate-volume", "gauge-plain-sum"},
	},
	"CASE-020/sum-over-time-units": {
		Proves: "time_group=sum integrates a stored rate into volume and reports the corresponding volume units: 'units/s' becomes 'units'",
	},
	"CASE-020/units-across-query-surfaces": {
		Proves: "db metadata keeps each metric's stored units, while sum transforms a rate's result units from rate to volume consistently in view metadata, per-dimension result metadata and labels for group_by=dimension, units and dimension,units; gauges and non-sum groupings keep their source units",
	},
	"CASE-020/query-surface-values": {
		Proves: "average and sum preserve the exact fixture-derived numeric result across group_by=dimension, units and dimension,units for both rate and gauge metrics, independently of result-unit rendering",
	},
	"CASE-020/badge-rate-sum-value": {
		Proves: "a one-point badge SUM over four complete incremental-rate samples includes all four source intervals and returns their exact integrated volume",
	},
	"CASE-020/badge-gauge-sum-value": {
		Proves: "a one-point badge SUM over four complete absolute samples includes all four source values and returns their exact total",
	},
	"CASE-020/badge-rate-sum-units": {
		Proves: "a badge SUM over a rate-only RRDSET renders the integrated volume unit by removing the source unit's trailing '/s'",
	},
	"CASE-020/badge-gauge-sum-units": {
		Proves: "a badge SUM over an absolute-only RRDSET preserves the RRDSET's common units",
	},
	"CASE-020/badge-mixed-algorithm-sum-units": {
		Proves: "a badge SUM over a mixed incremental-and-absolute RRDSET preserves the RRDSET's common units even when the dimension filter selects only its rate dimension",
	},
	"L4/family-tier-source": {
		Proves: "every forced family-tier query is served exclusively from tier1 and returns only the requested source dimension",
	},
	"L4/family-tier-grid": {
		Proves: "every forced tier1 family query returns the exact aligned six-bucket view grid, including upward rounding of before to grouping boundaries",
	},
	"L4/family-tier-values": {
		Proves: "all 17 pre-fleet grouping families match the fetch-aware tier1 value oracle across complete, partial and empty stored windows",
	},
	"L4/family-tier-anomaly-rates": {
		Proves: "every forced tier1 grouping reports the exact bucket anomaly rate derived from its stored anomaly and contributor counts",
	},
	"L4/family-tier-annotations": {
		Proves: "every forced tier1 grouping marks null stored windows EMPTY and numeric windows with stored gaps PARTIAL",
	},
	"L4/auto-tier-choice": {
		Proves: "without a tier parameter the planner chooses the coarsest tier with acceptable density for the one-second, sixty-second and coarse-bucket fixtures",
	},
	"L4/auto-tier-grid": {
		Proves: "automatic-tier queries return exactly the requested view grid and source dimension at each tested density",
	},
	"L4/auto-tier-values": {
		Proves: "automatic-tier query values equal the gap-aware oracle for the tier selected at each tested density",
	},
	"L4/auto-tier-anomaly-rates": {
		Proves: "automatic-tier query rows retain the exact anomaly rate of the selected tier's contributing points",
	},
	"L4/auto-tier-annotations": {
		Proves: "automatic-tier query rows mark empty results EMPTY and numeric results with stored gap evidence PARTIAL",
	},
	"L4/minmax-absolute-semantics": {
		Proves:     "time_group=min returns the value CLOSEST to zero and max the value FURTHEST from zero (min.h/max.h fabs comparisons) — visible only on negative/mixed data; pinned green in L3 sign-semantics + L4 matrix; RULING PENDING (arithmetic min/max would be a behavior change; extremes already provides champion-by-abs)",
		Components: []string{"tier0-min", "tier0-max", "tier1-min", "tier1-max"},
	},
	"L4/plan-switching": {
		Proves:     "queries spanning tiers with DIFFERENT retention are served by multiple plans: a dedicated 11M-sample fixture rotates tier0 at the 25MB quota floor while tier1 keeps the head; the discovered boundary is checked with per-side values, tier1-only controls, exact availability, authoritative fine-tier event/flap rows, and event totals bounded to raw truth plus one crossing coarse representative per seam",
		Components: []string{"seam", "head-only", "condition-groupings"},
	},
	"L4/three-tier-join-grid": {
		Proves: "one query joins three retention depths across both tier seams with exact coverage, outward alignment, newest-first wire order and canonical ascending rows at five zoom levels",
	},
	"L4/three-tier-condition-groupings": {
		Proves: "condition groupings preserve exact availability and at least one non-vacuous authoritative finer-tier event or flap under a crossing coarse record in forced tier regions and across both automatic three-tier seams",
	},
	"L5/group-by-grid": {
		Proves: "every level-1 group-by key and aggregation returns the exact unique t0+1 through t0+60 row grid in raw and non-raw modes",
	},
	"L5/group-by-naming": {
		Proves: "level-1 group-by returns exactly the fixture-derived groups with canonical selected, dimension, instance, node, label, context and units names",
	},
	"L5/group-by-values": {
		Proves: "level-1 group-by values match the member-enumeration oracle for average, min, max, sum and extremes in raw and non-raw modes",
	},
	"L5/group-by-anomaly-metadata": {
		Proves: "level-1 group-by anomaly rates use exact contributor weighting in non-raw mode and retain their accumulated numerator in raw mode",
	},
	"L5/group-by-partial-empty": {
		Proves: "level-1 group-by preserves null groups and marks a numeric row PARTIAL exactly when fewer than all group members contributed",
	},
	"L5/group-by-raw-schema": {
		Proves: "raw level-1 group-by carries exact per-point contributor counts without a hidden field, while non-raw output carries neither count nor hidden state",
	},
	"L5/percentage-grid": {
		Proves: "percentage aggregation returns the exact unique t0+1 through t0+60 grid for every tested grouping and raw mode",
	},
	"L5/percentage-nonraw": {
		Proves: "non-raw percentage converts each selected and hidden group to n*100/(n+h), preserving null when the selected side has no contributor",
	},
	"L5/percentage-raw-hidden": {
		Proves: "raw percentage defers conversion by carrying the selected sum as value and the finite hidden sum, or null without a hidden contributor, in the hidden field",
	},
	"L5/percentage-group-by-dimension": {
		Proves: "group_by=dimension keeps hidden dimensions in separate filtered groups, so each selected dimension percentage remains exactly 100% in raw and non-raw modes",
	},
	"L5/percentage-of-instance": {
		Proves: "percentage-of-instance converts per-instance percentages even in raw mode and never emits a hidden field because its groups cannot span agents",
	},
	"L5/statistics-weighted-average": {
		Proves:  "AVERAGE view statistics retain the pre-division sum and contributor count so each group's reported average is contributor-weighted",
		FixedBy: "#23097",
	},
	"L5/statistics-row-aggregations": {
		Proves:  "non-average group view statistics report the mean plotted row value and the exact minimum and maximum plotted rows",
		FixedBy: "#23097",
	},
	"L5/statistics-raw-sum-count": {
		Proves:  "raw group view statistics preserve the exact accumulated sum and contributor count for Cloud merging",
		FixedBy: "#23097",
	},
	"L5/gap-only-db-average-is-null": {
		Proves: "a dimension whose selected database points are all gaps reports null min/avg/max database statistics, not a numeric zero average that falsely represents observed data",
	},
	"L5/anomaly-statistics": {
		Proves: "jsonwrap-v2 per-dimension anomaly arrays: view.dimensions.sts.arp = mean of the plotted rows' anomaly rates, db.dimensions.sts.arp = anomaly rate of the fetched db points; stored NAN gap points are excluded from BOTH counts",
	},
	"L5/multi-key-group-by": {
		Proves: "multi-key group_by: groups are attribute TUPLES, ids join in the FIXED engine order (dimension, instance, label, node, context, units) regardless of request order; instance drops @node when node is in the mask; selected and percentage-of-instance collapse rules; avg alias; unknown aggregation silently parses to average",
	},
	"L6/two-pass-grouping": {
		Proves: "two-pass grouping partitions pass 1 by the union of both requested keys and returns exactly the fixture-derived final groups across all tested key chains",
	},
	"L6/two-pass-grid": {
		Proves: "every two-pass matrix query returns all 60 expected rows on the exact t0+1 through t0+60 grid",
	},
	"L6/two-pass-values": {
		Proves: "two-pass values match the pass-chain oracle for seven no-average aggregation chains in raw and non-raw modes",
	},
	"L6/two-pass-point-anomaly": {
		Proves: "non-raw two-pass point anomaly rates are weighted by raw metric contributors while raw points retain the anomaly numerator",
	},
	"L6/two-pass-view-anomaly": {
		Proves: "two-pass view anomaly statistics equal the mean of the finalized per-row anomaly rates for every eligible non-raw chain",
	},
	"L6/two-pass-partial-empty": {
		Proves: "two-pass rows are EMPTY without contributors and PARTIAL exactly when an underlying member or prior-pass group is missing",
	},
	"L6/two-pass-raw-schema": {
		Proves: "raw two-pass output carries the prior-pass group count and no hidden field, while non-raw output omits both",
	},
	"L6/two-pass-live-edge-trimming": {
		Proves: "a near-live contributor decline trims the unstable final two-pass row at the established partial-data cutoff while preserving the complete prefix grid",
	},
	"L6/two-pass-live-edge-values": {
		Proves: "a trimmed live-edge two-pass query preserves the exact values of every complete row before the contributor decline",
	},
	"L6/two-pass-live-edge-annotations": {
		Proves: "the rows surviving live-edge partial-data trimming retain exact EMPTY and complete annotations without exposing the removed PARTIAL suffix",
	},
	"L6/two-pass-average-boundary": {
		Proves:     "sum→average needs separate denominators across two passes: non-raw value divides by contributing pass-1 groups, while anomaly rate remains weighted by the raw metric contributors beneath those groups; raw mode deliberately leaves both numerators undivided and reports the prior-pass group count required by the old Agent-Cloud average merge contract",
		Components: []string{"sum-to-average"},
	},
	"L6/two-pass-average-held-boundary": {
		Proves: "average→sum preserves the released prior-pass-group anomaly divisor without ruling on the deferred average-composition contract",
	},
	"L6/two-pass-percentage": {
		Proves:     "percentage as the pass-2 aggregation: pass 1 runs in shadow hidden mode, the percentage pass folds hidden sums into each normal group's denominator (v*100/(v+h)), and an incomplete shadow bucket taints PARTIAL; non-raw anomaly metadata remains weighted by visible raw metric contributors, while raw mode defers value conversion, declares a result-wide hidden field with finite sums or null cells, and preserves the visible prior-pass group count",
		Components: []string{"sum-to-percentage"},
	},
	"L6/two-pass-percentage-held-boundary": {
		Proves: "percentage→sum preserves the released prior-pass-group anomaly divisor and complete annotation without ruling on percentage pooling or weighting",
	},
	"L6/two-pass-percentage-of-instance-held-boundary": {
		Proves: "percentage-of-instance→sum preserves the released prior-pass-group anomaly divisor and complete annotation without ruling on percentage pooling or weighting",
	},
	"CASE-018/multipass-average": {
		Proves: "with average at pass 1, pass 2 consumes each finalized pass-1 group average: [dimension,average]→[selected,average] equals the mean of those group averages, not the mean of unfinalized group sums",
	},
	"L7/format-csv": {
		Proves: "v1 CSV is byte-exact for newest-first and natural row order, including the established unquoted header cells",
	},
	"L7/format-tsv": {
		Proves: "v1 TSV is byte-exact over the formatter fixture's timestamps, values and gaps",
	},
	"L7/format-ssv": {
		Proves: "v1 SSV returns the exact row-reduced values for the formatter fixture",
	},
	"L7/format-ssvcomma": {
		Proves: "v1 SSV-comma returns the exact comma-separated row-reduced values for the formatter fixture",
	},
	"L7/format-csvjsonarray": {
		Proves: "v1 csvjsonarray is valid JSON with numeric timestamps and exact fixture-derived rows",
	},
	"L7/format-markdown": {
		Proves: "v1 Markdown emits the expected table structure and exact fixture-derived cells",
	},
	"L7/format-html": {
		Proves: "v1 HTML emits the expected table structure and exact fixture-derived cells",
	},
	"L7/format-array": {
		Proves: "v1 array format returns the exact row-reduced values and gaps for the formatter fixture",
	},
	"L7/format-json": {
		Proves: "v1 JSON has the strict expected row schema with exact fixture timestamps, values and gaps",
	},
	"L7/format-datatable": {
		Proves: "v1 datatable has the strict expected schema with exact fixture timestamps, values and gaps",
	},
	"L7/format-jsonp": {
		Proves: "v1 JSONP wraps the strict expected JSON rows with exact fixture timestamps, values and gaps",
	},
	"CASE-022/latest-name-echo": {
		Proves:  "time_group=latest is accepted and echoed as the canonical requested grouping",
		FixedBy: "#23257",
	},
	"CASE-022/latest-bucket-values": {
		Proves:  "LATEST returns the last collected value in each multi-sample bucket on the exact requested grid",
		FixedBy: "#23257",
	},
	"CASE-022/latest-empty-buckets": {
		Proves:  "LATEST identity rows preserve numeric values and leave buckets without a collected sample exactly EMPTY",
		FixedBy: "#23257",
	},
	"CASE-022/latest-absolute": {
		Proves:  "options=absolute applies to collector-cache LATEST values by erasing negative signs without forcing a storage read",
		FixedBy: "#23257",
	},
	"CASE-022/latest-collector-cache": {
		Proves:  "a one-point explicit LATEST window containing the newest sample uses zero database reads, preserves the raw unquantized value and anomaly rate zero, and stays on the request-derived grid",
		FixedBy: "#23257",
	},
	"CASE-022/latest-before-zero-v3": {
		Proves:  "v3 LATEST preserves before=0 as the database-end sentinel and returns the newest collector-cache sample on its established newest-sample grid",
		FixedBy: "#23257",
	},
	"CASE-022/latest-before-zero-v1": {
		Proves:  "v1 natural-points LATEST with before=0 restores an off-cadence newest stored timestamp rather than rounding it away",
		FixedBy: "#23257",
	},
	"CASE-022/latest-selected-tier-storage": {
		Proves:  "selected-tier LATEST uses storage, preserves tier0 storage_number quantization and negative signs, and reports the engine-derived anomaly rate",
		FixedBy: "#23257",
	},
	"CASE-023/fleet-grouping-echo": {
		Proves: "all four fleet condition groupings and the countif alias echo their canonical grouping names",
	},
	"CASE-023/percentage-of-samples": {
		Proves: "percentage-of-samples answers exact sample shares for numeric, gap and previous-sample expressions across bucket boundaries",
	},
	"CASE-023/percentage-of-time": {
		Proves: "percentage-of-time answers exact duration shares for numeric, gap and previous-sample expressions across bucket boundaries",
	},
	"CASE-023/number-of-times": {
		Proves: "number-of-times counts exact numeric, gap and previous-sample occurrences while never counting the first sample as previous",
	},
	"CASE-023/number-of-flaps": {
		Proves: "number-of-flaps counts only observed false-to-true condition transitions and carries condition state across buckets and gaps",
	},
	"CASE-023/gap-slot-width": {
		Proves: "a condition-matching gap contributes its stored slot width rather than the zero duration of an EMPTY query point",
	},
	"CASE-023/fleet-grouping-units": {
		Proves: "fleet condition groupings expose their canonical result units: percentages, events or flaps",
	},
	"CASE-023/expression-grammar-and-state": {
		Proves: "the shared expression parser accepts every documented operator spelling, bare operands, gap aliases and previous|last, rejects malformed and non-finite expressions, and preserves predecessor and flap state across gaps and bucket flushes",
	},
	"CASE-023/expression-default-zero": {
		Proves: "an absent, empty or whitespace-only condition means ==0 for every condition grouping, while every operator without an operand applies that operator to numeric zero on the V1, V2 and V3 data APIs",
	},
	"CASE-023/mcp-protocol-lifecycle": {
		Proves: "the MCP initialize response uses the requested protocol version, reports non-empty server identity and tool capability metadata, returns a valid session, and accepts the initialized notification",
	},
	"CASE-023/mcp-query-tool-schema": {
		Proves: "the MCP query_metrics tool schema advertises all four condition groupings, the countif alias and string-valued time_group_options with each grouping and its zero-default behavior documented",
	},
	"CASE-023/mcp-valid-result-schema": {
		Proves: "valid MCP query_metrics calls return the exact JSON2 labels, point schema, single-row shape and numeric point-field types",
	},
	"CASE-023/mcp-valid-query-units": {
		Proves: "valid MCP condition queries report their canonical percentage, event or flap units consistently at view and dimension scope",
	},
	"CASE-023/mcp-valid-query-echo": {
		Proves: "valid MCP condition queries echo the canonical grouping name and exact expression, including canonicalizing countif to percentage-of-samples",
	},
	"CASE-023/mcp-valid-query-timestamps": {
		Proves: "valid one-point MCP condition queries return the exact requested bucket endpoint as an RFC3339 timestamp",
	},
	"CASE-023/mcp-valid-query-values": {
		Proves: "valid MCP numeric and gap expressions return the exact percentage, occurrence and flap values, including cadence-sensitive sample and time denominators",
	},
	"CASE-023/mcp-valid-query-anomaly-rates": {
		Proves: "valid MCP condition queries preserve the exact anomaly rate of their selected source dimension independently of the grouped value",
	},
	"CASE-023/mcp-valid-query-annotations": {
		Proves: "valid MCP condition queries preserve the exact RESET point-annotation bitmap independently of the grouped value",
	},
	"CASE-023/mcp-invalid-options": {
		Proves: "non-string, malformed and non-finite MCP condition expressions return structured -32602 INVALID_PARAMS errors",
	},
	"CASE-023/mcp-default-zero-options": {
		Proves: "missing, empty and whitespace-only MCP conditions mean ==0, while an operator without an operand applies to zero, for every condition grouping and the countif alias",
	},
	"CASE-023/tier-estimation-source": {
		Proves: "each forced higher-tier condition query reads exclusively from the requested stored tier and returns only the selected dimension",
	},
	"CASE-023/tier-estimation-percentage-of-time": {
		Proves: "higher-tier percentage-of-time uses the approved min/max/average two-point mass estimator for steady-cadence stored windows",
	},
	"CASE-023/tier-estimation-number-of-flaps": {
		Proves: "a mixed higher-tier stored window contributes one flap because its interior ordering is unavailable",
	},
	"CASE-023/tier-estimation-number-of-times": {
		Proves: "a mixed higher-tier stored window contributes at most one condition occurrence because its interior ordering is unavailable",
	},
	"CASE-023/tier-estimation-percentage-of-samples": {
		Proves: "higher-tier percentage-of-samples preserves its stored-window sample-estimation behavior independently of the time-share estimator",
	},
	"CASE-023/cadence-change-availability-tier0": {
		Proves:     "when collection cadence changes, tier-0 percentage-of-time keeps exact wall-time availability across both the old and new sample intervals",
		Components: []string{"slows-down", "speeds-up"},
	},
	"CASE-023/cadence-change-availability-higher-tiers": {
		Proves:     "at tiers 1 and 2, a stored interval containing both cadences retains the original record duration and exposes the approved sample-weighted min/max/average two-point availability estimate",
		Components: []string{"slows-down", "speeds-up"},
	},
	"CASE-023/historical-gap-slots-after-cadence-change": {
		Proves: "after a metric speeds up from every 10 seconds to every second, gaps in its old history retain their historical slot weight. One missing old slot counts once, not ten times because the chart's latest cadence is one second. Asserted exactly at forced tiers 0, 1 and 2",
	},
	"CASE-023/tier-resolution-source": {
		Proves: "the tier-resolution matrix reads exclusively from each forced tier and returns only its selected fixture dimension",
	},
	"CASE-023/tier-resolution-percentage-of-time": {
		Proves: "percentage-of-time matches the fixture-derived duration estimator across forced tiers while downsampling, reading at storage resolution and upsampling",
	},
	"CASE-023/tier-resolution-percentage-of-samples": {
		Proves: "percentage-of-samples matches the fixture-derived sample estimator across forced tiers and repeated delivery resolutions",
	},
	"CASE-023/tier-resolution-number-of-times": {
		Proves: "number-of-times preserves exact gap, numeric and previous-counter occurrence behavior across forced tiers and repeated delivery resolutions",
	},
	"CASE-023/tier-resolution-number-of-flaps": {
		Proves: "number-of-flaps preserves exact transition behavior across forced tiers and repeated delivery resolutions",
	},
	"CASE-023/tier-resolution-slow-metric-upsampling": {
		Proves: "a ten-second tier0 metric upsampled into five-second rows stays numeric in every covered row without inventing flaps, occurrences or counter drops",
	},
	"CASE-023/redelivery-samples-everywhere": {
		Proves: "percentage-of-samples treats every delivery of a wide stored point as a sample and therefore answers every covered result bucket",
	},
	"CASE-023/redelivery-counted-once": {
		Proves: "number-of-times and number-of-flaps count one stored window at most once even when it is delivered into several result buckets",
	},
	"CASE-023/redelivery-zero-not-empty": {
		Proves: "a bucket covered only by a repeated wide point remains a numeric zero when no occurrence or flap belongs there, rather than becoming EMPTY",
	},
	"CASE-023/reset-counted-once": {
		Proves: "one counter reset counts once above tier 0, at any resolution: carrying the PRE-reset peak forward makes the window after the reset look like it dropped too, and re-delivering the reset window compares it against the maximum its own first delivery stored - both turn one reboot into several",
	},
	"CASE-023/previous-survives-redelivery": {
		Proves: "a counter that only climbs reports NO time below its predecessor, at any zoom: above tier 0 `<previous` is decided from the window's minimum against the PREVIOUS window's maximum, and by the time a wide window is re-delivered to the next bucket it spans that maximum has already advanced to the window's own - so re-deciding a repeat asks 'is this window's minimum below its own maximum', which is true of every window that moved at all, and percentage-of-time then reports a reboot in every bucket after the first",
	},
	"CASE-023/previous-drop-at-every-zoom": {
		Proves: "the mirror image: one real counter restart covers the SAME share of the span however finely the stored window is cut - the window it happened in counts as time below the predecessor for its whole duration, not just for the first bucket it was delivered into. Replaying the first delivery's verdict is what makes this and CASE-023/previous-survives-redelivery true at once; a repeat that re-decides itself reports the reset in the buckets that follow it and, once the post-reset floor is carried forward, stops reporting it in its own",
	},
	"CASE-023/nonzero-follows-answer": {
		Proves: "the condition groupings answer a question ABOUT the samples, so options=nonzero judges them by the ANSWER: a dimension whose condition never holds is dropped even though every source sample is non-zero, while a dimension with a non-zero answer stays",
	},
	"CASE-023/percentage-of-time-denominator": {
		Proves: "percentage-of-time divides by the full selected duration, so one collected matching second followed by 99 uncollected seconds answers 1% match and 99% gap",
	},
	"CASE-023/percentage-of-samples-denominator": {
		Proves: "percentage-of-samples divides only by delivered samples, so one matching collected sample remains 100% despite 99 seconds without another sample",
	},
	"CASE-023/trailing-gaps": {
		Proves: "retention overlap admits a specific RRDSET instance to the query, after which a condition that names a gap keeps accounting to the END of the requested window: an instance with real samples at the start and no samples later reads 100% gap for every remaining bucket, not for only the first eleven; an instance that continues collecting in the same context is the retention control",
	},
	"CASE-023/leading-gap-chronology": {
		Proves: "when an instance starts collecting near the end of an overlapping query, its leading uncollected interval is delivered before its first real sample: number-of-flaps(==gap) stays zero instead of inventing a numeric-to-gap transition by moving the leading gap to the row suffix",
	},
	"CASE-023/window-outside-retention": {
		Proves: "an RRDSET instance whose retention does not overlap the requested window is omitted even when another instance of the same context does overlap and the grouping names gaps; the mixed-context result contains only the overlapping instance, and explicitly selecting only the expired instance returns no result instead of synthesizing an all-gap series",
	},
	"CASE-023/gap-weight": {
		Proves: "a gap counts in stored SLOTS, not seconds: percentage-of-samples weighs uncollected time against the collection interval, so on a 10s metric a 100s hole is ten missing samples and not a hundred - measuring it against the query grid (1s for an ordinary query) would let one missing slot outweigh ten collected ones",
	},
	"CASE-023/tier-wide-point-source": {
		Proves: "a fine-grid query over tier1 reads the expected wide stored point repeatedly with its original interval and interpolated delivered values",
	},
	"CASE-023/tier-wide-point-time-share": {
		Proves: "percentage-of-time returns the same stored-window estimate in every finer result bucket spanned by one wide tier point",
	},
	"CASE-023/tier-wide-point-number-of-times": {
		Proves: "one wide stored tier point contributes at most one occurrence across all finer result buckets to which it is re-delivered",
	},
	"CASE-023/tier-wide-point-number-of-flaps": {
		Proves: "one wide stored tier point contributes at most one flap across all finer result buckets to which it is re-delivered",
	},
	"CASE-023/tier-anomaly-bit": {
		Proves: "with options=anomaly-bit above tier 0 the value is the stored window's anomaly RATE while min/max still describe the metric, so the condition is answered on the rate itself — a window either satisfied it or it did not (100/0), never a fraction estimated across two unrelated domains; >=N and <N partition every window",
	},
	"CASE-024/zoom-into-slow-metrics": {
		Proves: "a metric collected once a minute, once per ten minutes or once an hour still answers when the dashboard zooms BELOW its collection interval: a 60-point request over a window shorter than one sample interval, fully inside the collected span, returns rows that carry the value - a chart that empties out when the user zooms in is indistinguishable from an outage",
	},
	"CASE-025/carry-survives-gaps": {
		Proves: "sum assigns every stored record to result rows by exact time overlap, including numeric rows that also overlap explicit gap records, a following truly empty row, and the final partial value row before retention ends. Varying record values make the creditor identity observable, while every numeric row containing stored gap evidence is exactly PARTIAL: the per-row Class-A oracle fails if a whole record is paid to one row, interpolation changes its value, a remainder is dropped, a neighboring record pays it, or gap evidence is lost, even when the whole-window total is unchanged",
	},
	"CASE-025/anomaly-bit-not-blended": {
		Proves: "a bucket lying entirely inside one stored window reports THAT window's anomaly rate, un-blended. options=anomaly-bit answers about the anomaly RATE, so sum's seconds-owed arithmetic is skipped for it - and the other half of the boundary machinery must not reach it either. A bucket carved inside a fully-anomalous window contains no sample from the window before it, so blending would report the metric as less anomalous than every sample under the bucket actually was. Asserted under average, min, max and sum over a hard 0 -> 100 step on a stored window boundary: all three buckets read 100, and a blended 33/67/100 would be the step smeared backwards into seconds it never touched. Records the ruling that refuted a review finding which had assumed the blended answer was the correct one",
	},
	"CASE-026/anomaly-rate-covers-the-paid-seconds": {
		Proves: "on tier 0, each 35-second row carries the exact sum or average value, anomaly rate of raw sample timestamps in `(row_start,row_end]`, and RESET annotation belonging to that same row. Alternating all-healthy/all-anomalous rows expose fetched-record or interpolation-based metadata, while the last reset sample pins the final five-second sum share, ARP and RESET to the settlement row; later rows are exactly empty",
	},
	"CASE-026/totals-survive-a-plan-switch": {
		Proves: "a sum total does not change because the answer had to be assembled from two tiers. sum carries across rows the seconds a record still owes, and a plan switch is the one moment the record stream itself jumps - the engine reads ahead into the next tier and may keep or discard either side. A carry dropped or double-paid there moves the total by up to one stored record, a whole minute of data above tier 0. Asserted over a discovered, rotated tier0 head at four zooms from 1s to 300s buckets; an exact two-row seam covers a first-row join and an exact three-row seam covers a carried coarse record plus a trailing fine-tier control row. A sparse control adds 120-second bursts separated by 1,920-second true DBENGINE page holes: forced tier0 and tier1 queries prove the exact fine burst and its overlapping coarse rollup, while two automatic 10-second grid phases place the fine page boundary on both adjacent row grids, leave every hole exactly empty, conserve every retained sample and report exact fine-tier anomaly membership once fine retention begins. A separate automatic tail seam proves tier1 ends at t0+220 while retained tier0 resumes at t0+2021: three 700-second rows must be exactly 100000, EMPTY and 61000, while a 2100-second one-row grid beginning one second later must be exactly 161000 and consume both tiers, so output-row width cannot make the planner skip the fine suffix. Seventeen 120-second rows over the same storage shape additionally hold the expired coarse record in executor state: the first row is 100000, all fifteen storage-hole rows are exactly EMPTY with empty metadata, and the final fine row is 61000, proving zero overlap never becomes a numeric zero or stale metadata. A second sparse seam leaves exactly one retained fine point buffered after its incoming storage handle is exhausted; a one-row query must still add it to the earlier numeric rollup, retain stored-gap PARTIAL evidence and read exactly two tier1 records plus one tier0 point. A third sparse seam gives the crossing coarse record and first fine point exactly the same start: three rows remain 6000/27000/27000 and raw source statistics remain count 60/sum 60000 rather than counting both representations. Constant-source seam queries also require public and raw database statistics to stay algebraically coherent: min=average=max=1000 and sum=1000*count even when the output value uses only a clipped coarse prefix. Partial higher-tier anomaly metadata is not claimed because a stored rollup cannot locate its constituent timestamps",
	},
	"CASE-027/incremental-sum-conserves-across-zoom": {
		Proves: "time_group=incremental-sum answers how much a value changed in a bucket, so the buckets of a window telescope and add up to the first-to-last rise at every resolution. At forced tier 0 the exact one-row-per-sample grid has one opening null followed by 59 rows of +7 with zero ARP/PA, while finer and coarser controls preserve the same total; a one-sample flush must carry its real baseline rather than overwrite it with an unset last value",
	},
	"CASE-028/rate-with-gaps-totals-what-was-measured": {
		Proves: "a rate metric with holes in it totals the seconds that were MEASURED, on every tier, with every row exact. Above tier 0 a stable-cadence record's count and recovered gap_count partition its nominal slots: sum x duration/(count+gap_count) integrates measured time without inventing volume. The matrix separates update_every 1/10, tiers 0/1/2, two exact zooms, and gapped/no-gap controls; every gapped numeric row is exactly PARTIAL and every dense row is clean. Windows span whole stored records, so the oracle has no edge estimate or rounded query span",
	},
	"CASE-028/partial-and-off-grid-rate-windows": {
		Proves: "a window that CUTS stored records still totals what those seconds hold. The aligned matrix is what makes CASE-028's oracle exact, and it means a partial record is never asked for - but a window starting and ending inside records is the ordinary case for a dashboard. Asserted on a rate with no holes, where the part of a record inside the window is countable from the fixture rather than estimated, at update_every 1 and 10, on tier 0 and tier 1, over windows that start mid-record, end mid-record, and cover exactly 181 samples",
	},
	"CASE-029/tier0-slow-metric-totals-at-every-zoom": {
		Proves: "sum assigns each row only its overlap share of a wider stored record: at tier 0, a metric collected every ten seconds has exact values on 10s, 5s, 2s and 1s rows without zoom inflation; two complete 36,000-second tier-2 records are independently sliced into exact 60-second rows with the same dense total and zero anomaly rate",
	},
	"CASE-030/interval-change-slowing-down": {
		Proves: "a metric changing how often it is collected does not rewrite the volume of complete, no-gap historical records. This separates the interval represented by the queried records from the metric's current interval - identical numbers until the interval changes, which is why a uniform fixture cannot detect current-metadata arithmetic. Asserted on four whole tier2 records in the middle of the first collection phase, also a whole number of tier1 records and samples, at forced tiers 0, 1 and 2. This does not claim that a gapped higher-tier record preserves its historical interval: CASE-028 proves sum/count/span cannot expose that. Mutation-proved: reading the metric's CURRENT tier-0 interval fails both directions at all three tiers by 10x",
	},
	"CASE-030/interval-change-speeding-up": {
		Proves: "the mirror of CASE-030/interval-change-slowing-down: history collected every ten seconds keeps its volume after the metric moves to once a second. Asserted separately so a run names which direction broke, and so one failure cannot be counted twice. Same three tiers and the same mutation proof as its mirror",
	},
	"CASE-031/rate-volume-across-an-automatic-seam": {
		Proves: "a dense constant rate's exact whole-window volume is unchanged whether tier 0, tier 1, or both answer. The rotated tier0 head proves tier1-only history, while exact all-tier presence vectors and 60-second/1-second zoom totals exercise read-ahead and the automatic plan switch. No persistent source-tier representation, exact per-row ownership, or tier1-tier2 seam is claimed",
	},
	"CASE-032/reset-annotation-is-not-redelivered": {
		Proves: "a RESET is annotated exactly once, on the result row containing the reset sample's timestamp. At tier 0 the last healthy reset sample of a 10-second metric is asserted under average and sum on 5-second upsampling, 30-second downsampling, and a nondividing 35-second grid whose owning row ends 20 seconds after retention; every row has exact ARP 0 and no extra annotation bits",
	},
	"CASE-033/anomaly-rate-counts-samples-in-the-row": {
		Proves:     "a result row's anomaly rate describes its source evidence. At tier 0, samples anomalous at +50, +60 and +80 pin exclusive-start membership to exact ARP [0, 100/3, 50, 100/3] under average, max and sum, excluding the row-start and future samples. Across an automatic tier1-to-tier0 seam, an all-anomalous dimension remains ARP 100 on every exact numeric row, proving read-ahead must not lose or reclassify coarse-tier metadata when the active plan changes",
		Components: []string{"tier0-row", "plan-seam-source"},
	},
	"CASE-034/api-timestamp-grid-is-immutable": {
		Proves:     "an explicit absolute virtual-points query has one immutable public timestamp grid, independent of values, collection cadence, gaps, retention coverage, requested tier option, and time aggregation, except for the established near-live partial-contributor trimming contract. The historical matrix pins view.after, view.before, view.update_every, row count, and every wire timestamp for aligned and unaligned points=1/7 plus aligned points=60 across dense update_every=1 and 10, gapped, and partial-retention hosts; automatic and requested tiers 0/1/2; and average, sum, and latest on /api/v3/data. A representative aligned one-point query pins /api/v2/data on every fixture shape. Near-now aligned and unaligned explicit absolute plus unaligned relative LATEST points=1 queries across two Agents with different newest stored timestamps must keep request/clock-derived grids on both /api/v3/data and /api/v1/data; the unaligned requests additionally prove collector-cache service with zero storage reads. Historical and hot-edge values are deliberately ignored because value fixes must preserve this timestamp contract",
		Components: []string{"dense-ue1", "dense-ue10", "gapped-ue1", "partial-ue1", "hot-edge-data-independence"},
	},
	"CASE-034/near-live-partial-data-is-trimmed": {
		Proves: "within twice the maximum collection interval of the live edge, a decline in contributing dimensions trims the response at the first incomplete row instead of publishing unstable partial aggregates. A complete two-dimension control keeps all six requested rows; when one dimension is delayed by two samples, both columns end after the last four complete rows while view.after/view.before/view.update_every remain request-derived. DEBUG metadata reports max_update_every=20, expected_after=before-20, and the exact data-dependent cutoff",
	},
	"CASE-035/completed-rollup-keeps-original-cadence": {
		Proves:     "when collection cadence changes, every fully completed higher-tier rollup is persisted with its original page cadence. Separate fixtures pin both internal states: a completed point already delayed in last_completed_point and a boundary-ending point still held in virtual_point. Two complete pre-transition SUM rows at forced tiers 1 and 2 in both the 1-to-10 and 10-to-1 directions preserve exact fixture-measured rate x collection interval volume; exact-boundary cases also pin two complete rows on the reset new-cadence grid twice, so false gap caching cannot hide the first new page on a repeated query. Every request timestamp is exact and unchanged. The separate transition-volume contracts own the active mixed-cadence row that V1 cannot represent exactly",
		Components: []string{"buffered-slows-down-tier1", "buffered-slows-down-tier2", "buffered-speeds-up-tier1", "buffered-speeds-up-tier2", "boundary-slows-down-tier1", "boundary-slows-down-tier2", "boundary-speeds-up-tier1", "boundary-speeds-up-tier2"},
	},
	"CASE-035/transition-volume-slowing-down": {
		Proves: "a metric slowing from update_every 1 to 10 preserves exact fixture-measured rate volume in the higher-tier row containing both cadences. The two-row forced-tier 1 and 2 queries retain one independently pinned homogeneous control to avoid one-point boundary policy; CASE-035/completed-rollup-keeps-original-cadence owns that control. This contract keeps the tier2-width raw tier0 control; CASE-035/tier0-page-boundary-keeps-every-sample owns the identical tier1-width raw control. These forced-tier queries do not cover an automatic plan switch during a cadence change",
	},
	"CASE-035/transition-volume-speeding-up": {
		Proves: "the update_every 10 to 1 mirror preserves exact fixture-measured rate volume in the higher-tier row containing both cadences. The two-row forced-tier 1 and 2 queries retain one independently pinned homogeneous control to avoid one-point boundary policy; CASE-035/completed-rollup-keeps-original-cadence owns that control. This contract keeps the tier2-width raw tier0 control; CASE-035/tier0-page-boundary-keeps-every-sample owns the identical tier1-width raw control. These forced-tier queries do not cover an automatic plan switch during a cadence change",
	},
	"CASE-035/tier0-page-boundary-keeps-every-sample": {
		Proves:     "a DBENGINE tier-0 query crossing adjacent pages with different collection cadences returns every stored sample exactly once. The 10-to-1 direction pins the first ten fine-page samples so advancing by the old ten-second cadence cannot skip nine of them; the 1-to-10 mirror proves the fix does not duplicate or shift the first coarse-page sample. Exact SUM rows come from the fixture's per-sample rate-times-duration ledger, and the public timestamp grid is unchanged",
		Components: []string{"slows-down", "speeds-up"},
	},
	"CASE-036/absolute-across-plan-seam": {
		Proves: "options=absolute applies to the point read from the incoming tier plan: a negative flat line stays exactly positive in every 1-second row across an automatic tier1-to-tier0 seam, with tier1-only and tier0-only controls and strict source-tier evidence",
	},
	"CASE-037/rate-volume-across-three-tier-cadence-query": {
		Proves: "one exact one-second sum query crosses automatic tier2-to-tier1 and tier1-to-tier0 plan seams, then crosses an update_every 1-to-10 transition where the rate also changes. Every returned row and the complete total come from the fixture ledger, and db.per_tier must prove all three tiers contributed. Forced-tier mixed-cadence records remain covered separately by CASE-035",
	},
	"CASE-038/higher-tier-only-rate-volume": {
		Proves: "archived dense and alternating-gap incremental metrics with no tier-0 retention or pre-restart live/cache state still return exact fixture-measured rate volume from retained tier 1. Forced-tier and automatic queries require zero tier-0 retention and reads, covering tier-1 retention and reads, the exact requested timestamp grid, and exact PA: dense rows clean, every gapped numeric row PARTIAL",
	},
	"CASE-019/v1-json-name-escaping": {
		Proves:  "v1 JSON-family formatters (json, jsonp, csvjsonarray, datatable) escape dimension names (was: raw between quotes — a double-quote in a name, or a label value via group_by=label, produced invalid JSON); the objectrows row keys are escaped like the header, and the google flavor (datatable+google_json) escapes the apostrophe of its single-quoted JavaScript labels while keeping the double quote raw",
		FixedBy: "#23216",
	},
	// Layer 10 — the invariants every grouping owes, swept across all of
	// them. The roster comes from the engine's own enum, so a grouping added
	// without a classification fails the sweep by name.
	"L10/roster-is-complete": {
		Proves: "the sweep classifies every requestable grouping declared by the explicitly paired source tree: the roster is parsed from RRDR_TIME_GROUPING and its name registry, so a declared grouping without a layer-10 rule fails by name instead of silently falling back to average",
	},
	"L10/no-holes-inside-data": {
		Proves: "every grouping answers every exact bucket across fully collected data at tiers 0 and 1, with 10s, 60s, 300s and 600s buckets; only incremental-sum may leave its opening bucket empty because the query has no predecessor, and every later bucket must answer",
	},
	"L10/buckets-finer-than-stored-data-answer": {
		Proves: "a bucket NARROWER than stored data still answers for every grouping after the optional opening incremental-sum bucket: above tier 0 a stored point covers many seconds, so a dashboard drawn finer gives the engine buckets that a single re-delivered point covers on its own; dropping those repeats must not create interior holes",
	},
	"L10/order-statistics-stay-in-range": {
		Proves: "at tiers 0 and 1, every numeric average/min/max/median/trimmed-*/percentile*/extremes/latest answer stays inside the exact fixture-derived source envelope of its bucket, while SES stays inside the cumulative source envelope required by its carried state; every grouping/tier/dimension combination must produce numeric coverage",
	},
	"L10/dimensions-are-independent": {
		Proves: "at tiers 0 and 1, every grouping returns the same complete points for a dimension alone and alongside its neighbours — value, anomaly rate and annotations — with exact grids and nonempty numeric coverage",
	},
	"L10/totals-are-exact-across-zoom": {
		Proves: "a TOTAL over a fixed span is the same number at any resolution: the total volume over an hour is a physical quantity and cannot depend on how many columns the chart was drawn with. `sum` breaks this above tier 0 — a stored point is delivered to every bucket it spans and `sum` fetches the WINDOW'S OWN SUM for it (TIER_QUERY_FETCH_SUM), so each bucket is handed the whole window's total and adds it again. On a constant 7 over 1200s (true total 8400) tier 1 reads 8400 at 20 buckets, 25200 at 60, 126000 at 300 and 504000 at 1200 — exactly the zoom factor. options=natural-points does not help. Same fault as the condition groupings had, in an aggregation nobody had written a case for",
	},
	"L10/single-point-buckets-answer": {
		Proves: "a bucket holding ONE collected sample still answers: one bucket per sample interval is the most natural resolution there is and a chart drawn at it must not come back blank. The mechanism is `incremental-sum`'s carry - flush hands a bucket's last sample forward as the next bucket's baseline - and the defect was that a bucket holding only the opening sample leaves `last` unset, so flush copied that unset value back over the seed it had just captured and the chain never started. One point per bucket is fully supported by add(), which is what made it a latch bug rather than a limitation",
	},
	"L10/counts-do-not-inflate-with-zoom": {
		Proves: "an event does not happen twice because the chart was zoomed in: above tier 0 the same stored point is handed to every bucket it spans, and a grouping that counts occurrences may see its total collapse as a rollup loses ordering but may never see it grow. Swept over number-of-flaps and number-of-times at 1/2/3/5 buckets per stored window",
	},
	"L10/totals-exact-over-gaps-and-off-grid": {
		Proves: "a total is exact over a span with HOLES in it and over a span that does not start on the tier grid - the two shapes L10/totals-are-exact-across-zoom never sees, and the two that hid real defects. An unaligned span puts the first stored point ACROSS the first bucket's start, where an apportionment clamped only against what it already accounted for hands the bucket everything from the point's own beginning; and a fixture with holes is the only way to reach a point the engine does NOT trim, because query_interpolate_point() trims a wide point to the bucket end only when the point before it is adjacent and numeric, which after a gap it is not. The same fault as L10/totals-are-exact-across-zoom reaches both shapes: `sum` above tier 0 counting a stored point once per bucket it spans",
	},
	"L10/anomaly-bit-answers-about-rates": {
		Proves: "options=anomaly-bit answers about anomaly rates, never about the metric: a never-anomalous dimension holding 1000000 must return the same complete points as an otherwise identical zero-valued control at tiers 0 and 1, including legitimate EMPTY placement for undefined grouping results",
	},
	"L10/time-shares-stable-across-zoom": {
		Proves: "a share of TIME over a fixed span is the same at every zoom: the share of a window that satisfied a condition is a property of the data and the window, not of how many buckets the window was drawn with. This is the rule percentage-of-time(<previous) broke — 5% of the span at one bucket per stored window against 77% at five. percentage-of-samples is deliberately excluded: it answers about the samples it was handed and a re-delivery is another sample to it",
	},
	"L10/aliases-resolve-to-the-same-grouping": {
		Proves: "every paired-source alias resolves to its canonical grouping with exact request echo, source tier, dimensions, grid, numeric coverage and complete point equality (value, anomaly rate and annotations)",
	},
	"L10/queries-are-deterministic": {
		Proves: "asking twice returns exact validated shapes, numeric coverage and complete point equality for every grouping at tier 1; state cannot outlive its query",
	},
	"L10/buckets-are-ordered-and-unique": {
		Proves: "raw wire rows and canonical columns contain the complete response-derived grid exactly once and in order for every grouping at tiers 0 and 1 with points=7, 60 and 300; the nondivisible 7-point request is checked against the expanded view grid and may not omit the first requested bucket rather than being assumed to have seven rows",
	},
	// Layer 11 — the slicing matrix: the knobs that must not change a total.
	"L11/slicing-is-additive": {
		Proves: "when independently normalized subquery grids share at most the one stored record at their cut, slicing the released query window preserves the total modulo that record's exact fixture content across dense/gap/sparse data, 1s/10s cadence, tiers 0/1, zoom, offsets and query options; every shape×cadence×tier has a non-vacuous eligible case",
	},
	"L11/randomised-slicing": {
		Proves: "seeded generated cases materialize window, tier duration and bucket count from their axes, honor released endpoint and one-point grids, require additivity when subquery normalization shares at most one stored record and conservation for every shape×cadence×tier combination, and greedily shrink failures to a locally minimal case; QUERY_CORPUS_SEED replays the sequence",
	},
	"L11/totals-match-what-was-pushed": {
		Proves: "after polling every shape×cadence×tier retention prerequisite, each combination whose buckets are at least one collection interval wide totals the fixture's exact collected-sample sum; only the deduplicated fixture contents of records crossing the two outer edges plus wire-print epsilon are allowed, with positive fixture content requiring numeric response coverage",
	},
	"L9/virtual-points": {
		Proves:     "for the covered default-mode tier0 fixtures, the source-derived virtual-point selection oracle models whole-point inclusion and boundary interpolation over a preconstructed stored-point stream; exact cases cover grid-cut intervals, off-grid identity and upsampling including the first unanchored straddler",
		Components: []string{"interpolated-buckets", "off-grid-identity", "upsampling"},
	},
	"L9/window-normalization": {
		Proves:     "a negative `after` is relative to `before` (identical to the absolute equivalent); (0,0) resolves to the ~600s grid-aligned default window ending NOW with an exact empty dimension-id array — NOT the full retention (the reason backdated fixtures settle via explicit windows); an explicit future window shifts both endpoints to the query-time clock without changing its duration; time_resampling (v1 gtime) forces the bucket size up",
		Components: []string{"relative-window", "default-relative-window", "future-explicit-window", "time-resampling"},
	},
	"L9/natural-points-grid": {
		Proves: "options=natural-points keeps the database point count and spacing while snapping result timestamps onto the absolute update-every grid",
	},
	"L9/natural-points-values": {
		Proves: "natural-points preserves raw sample values and keeps boundary slots within the documented raw-sample or phase-interpolated two-candidate contract",
	},
	"L9/live-edge-empty-outside-retention": {
		Proves: "live-edge grid rows wholly before retention remain explicit EMPTY rows rather than being removed from the response",
	},
	"L9/v2-v3-parity": {
		Proves: "/api/v2/data and /api/v3/data answer identically for identical params (shared api_v23_data_internal) — only the api version field differs",
	},
	"API/selectors": {
		Proves:     "the selector surface with VALUE-exact oracles: nodes/instances/dimensions filters and their scope_ counterparts, '!' negation patterns, label key:value patterns (labels + scope_labels); match-ids/match-names dimension modes with id!=name dims (default matches BOTH; each mode excludes the other's namespace; a no-match response is the bare [time] labels row)",
		Components: []string{"selectors", "match-modes"},
	},
	"API/options-long-tail": {
		Proves:     "ms renders epoch-milliseconds; rfc3339 loses to seconds on the v1 formatters (pinned no-op); objectrows emits strict named rows with exact fixture timestamps/values; jsonwrap all-dimensions keeps exactly the selected dimension and adds the exact full dimension/chart/automatic-label string-pair sets; tqx wraps datatable in the gviz envelope echoing reqId; tsv-excel == tsv; csv label-quotes quotes the header; v2 minimal-stats drops totals, long-json-keys switches to descriptive keys, group-by-labels flattens the label values into the view",
		Components: []string{"timestamps", "v1-json-shapes", "google-viz", "format-aliases", "label-quotes", "v2-shapes"},
	},
	"API/row-reductions": {
		Proves: "the single-series formats reduce each row by the requested option: min2max = max-min (0 on single-value rows), min, max and average, with default sum pinned by the corresponding L7 format contracts",
	},
	"API/fallback-unknown-format": {
		Proves: "an unknown v1 format retains the established silent fallback to JSON",
	},
	"API/fallback-unknown-weights-method": {
		Proves: "an unknown weights method retains the established silent fallback to ks2",
	},
	"API/cardinality-limit-sweep": {
		Proves: "cardinality_limit=2 folds five dimensions into the remaining bucket, while a limit at or above the dimension count folds nothing",
	},
	"W/value": {
		Proves:     "weights method=value: per-metric weight = the window average over NATURAL points with the after-INCLUSIVE window (121 points for a 120s span — rulings batch); strict MULTINODE rows contain exactly one instance, context and node rollup with the mean of their dimensions and the required index/null layout; the per-dimension timeframe stats (min/avg/max/sum/count/anomaly_count) are exact; method=value NEVER rank-normalizes",
		Components: []string{"multi-node", "never-spreads"},
	},
	"W/anomaly-rate-per-metric-values": {
		Proves: "the per-metric anomaly-rate weights path applies anomaly-bit and returns each metric's true window anomaly rate",
	},
	"W/anomaly-rate-per-metric-nonzero-default": {
		Proves: "the implicit NONZERO weights default drops zero anomaly-rate metrics, while any explicit options value keeps them",
	},
	"W/anomaly-rate-multidim": {
		Proves:  "method=anomaly-rate implies the anomaly bit on EVERY path: the bare method and the explicit options=anomaly-bit are equivalent, both returning true anomaly rates through the multi-dimensional path (was: the bare method ranked by plain value averages there while per-metric and MCP forced the bit)",
		FixedBy: "#23212",
	},
	"W/volume-equal-baseline-skip": {
		Proves: "volume weighting omits a metric whose highlight and baseline window averages are equal",
	},
	"W/volume-formula": {
		Proves: "volume weight equals the relative highlight-versus-baseline average change multiplied by the highlight-time share on the matching side of the baseline average",
	},
	"W/ks2-raw-endpoints": {
		Proves: "ks2 assigns exact raw weights 0 to identical consecutive-difference distributions and 1 to fully one-sided distributions meeting the endpoint threshold",
	},
	"W/ks2-spread-normalization": {
		Proves: "spread_results_evenly rank-normalizes ks2 weights by unique-value slots while tied raw weights share one slot",
	},
	"L8/percentage-post-processing": {
		Proves: "options=percentage computes per-row shares over absolute values and forces absolute semantics for v2/v3 and non-dimension groupings",
	},
	"L8/absolute-post-processing": {
		Proves: "options=absolute replaces each fetched numeric value with its magnitude before later query processing",
	},
	"L8/nonzero-post-processing": {
		Proves: "options=nonzero removes dimensions whose selected result rows are all numeric zero",
	},
	"L8/null2zero-post-processing": {
		Proves: "options=null2zero converts gap cells to numeric zero without altering non-gap values",
	},
	"L8/nonzero-all-zero": {
		Proves: "nonzero filtering self-neutralizes when every selected dimension is all-zero so the result is not emptied",
	},
	"L8/cardinality-limit": {
		Proves: "cardinality limiting keeps the top N-1 dimensions by absolute view sum and folds every remaining per-row value into one named remainder column",
	},
}

const defaultContractComponent = "contract"

type contractObservation struct {
	evaluated bool
	broken    bool
}

type contractLedger struct {
	mu      sync.Mutex
	results map[string]map[string]contractObservation
}

func newContractLedger() *contractLedger {
	return &contractLedger{results: make(map[string]map[string]contractObservation)}
}

func requiredContractComponents(mc ManifestCase) []string {
	if len(mc.Components) == 0 {
		return []string{defaultContractComponent}
	}
	return mc.Components
}

func validateContractComponent(name, component string) error {
	mc, ok := manifest[name]
	if !ok {
		return fmt.Errorf("case %q missing from manifest", name)
	}

	for _, required := range requiredContractComponents(mc) {
		if component == required {
			return nil
		}
	}

	return fmt.Errorf("case %q has no component %q", name, component)
}

func (l *contractLedger) record(name, component string, held, skipped bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// A clean skip did not evaluate the contract. A test that failed before
	// it skipped still produced a real broken observation and must not vanish.
	if skipped && held {
		return
	}

	components := l.results[name]
	if components == nil {
		components = make(map[string]contractObservation)
		l.results[name] = components
	}

	observation := components[component]
	observation.evaluated = true
	if !held {
		observation.broken = true
	}
	components[component] = observation
}

func (l *contractLedger) register(name, component string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	components := l.results[name]
	if components == nil {
		components = make(map[string]contractObservation)
		l.results[name] = components
	}
	if _, ok := components[component]; !ok {
		components[component] = contractObservation{}
	}
}

// trackContract records the result of an ordinary Go test as one corpus
// contract. Register it before any assertion so Fatal and Skip are visible.
func trackContract(t *testing.T, name string) {
	t.Helper()
	trackContractComponent(t, name, defaultContractComponent)
}

// trackContractComponent records one required scope of a contract that is
// proven by more than one independent test.
func trackContractComponent(t *testing.T, name, component string) {
	t.Helper()

	if err := validateContractComponent(name, component); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		contractResults.record(name, component, !t.Failed(), t.Skipped())
	})
}

// registerContract reserves a computed contract verdict before shared work.
// The later assertContract call evaluates it; an earlier Fatal or Skip leaves
// the reservation incomplete instead of allowing a default-true verdict.
func registerContract(t *testing.T, name string) {
	t.Helper()
	registerContractComponent(t, name, defaultContractComponent)
}

func registerContractComponent(t *testing.T, name, component string) {
	t.Helper()

	if err := validateContractComponent(name, component); err != nil {
		t.Fatal(err)
	}

	contractResults.register(name, component)
}

// assertContract records the verdict for one corpus case.
//
// A contract either holds or it does not. A broken one fails here - on
// master, on a feature branch, whether or not anyone already knew about
// it. There is no "known broken, and therefore fine": that state makes a
// broken query engine report success, and this corpus exists to name what
// is broken, not to keep a list of exceptions.
//
// Every break is also collected for the end-of-run summary, so a run tells
// you the whole set at once instead of one line per test.
func assertContract(t *testing.T, name string, held bool) {
	t.Helper()

	if err := validateContractComponent(name, defaultContractComponent); err != nil {
		t.Fatal(err)
	}

	contractResults.record(name, defaultContractComponent, held, false)
	if held {
		return
	}

	t.Errorf("BROKEN %s: %s", name, manifest[name].Proves)
}

var contractResults = newContractLedger()

type contractRunSummary struct {
	evaluated  int
	broken     []string
	incomplete []string
}

func (l *contractLedger) summarize(cases map[string]ManifestCase) contractRunSummary {
	l.mu.Lock()
	defer l.mu.Unlock()

	var summary contractRunSummary
	for name, mc := range cases {
		complete := true
		broken := false
		for _, component := range requiredContractComponents(mc) {
			observation, ok := l.results[name][component]
			if !ok || !observation.evaluated {
				complete = false
				if len(mc.Components) == 0 {
					summary.incomplete = append(summary.incomplete, name)
				} else {
					summary.incomplete = append(summary.incomplete, name+"/"+component)
				}
			}
			if observation.broken {
				broken = true
			}
		}
		if complete {
			summary.evaluated++
		}
		if broken {
			summary.broken = append(summary.broken, name)
		}
	}

	sort.Strings(summary.broken)
	sort.Strings(summary.incomplete)
	return summary
}

// contractSummary is printed once, after every test has run. complete is true
// only when every manifest contract and each of its required scopes ran.
func contractSummary(includeIncompleteDetails bool) (report string, complete bool) {
	summary := contractResults.summarize(manifest)
	return formatContractSummary(summary, len(manifest), includeIncompleteDetails)
}

func formatContractSummary(summary contractRunSummary, total int, includeIncompleteDetails bool) (report string, complete bool) {
	complete = len(summary.incomplete) == 0

	var b strings.Builder
	if complete && len(summary.broken) == 0 {
		fmt.Fprintf(&b, "query contract corpus: all %d contracts hold\n", total)
		return b.String(), true
	}

	if !complete {
		fmt.Fprintf(&b, "\nquery contract corpus: %d of %d contracts fully evaluated; %d required scope(s) did not run\n",
			summary.evaluated, total, len(summary.incomplete))
	}

	if len(summary.broken) > 0 {
		if complete {
			fmt.Fprintf(&b, "\nquery contract corpus: %d of %d contracts BROKEN\n",
				len(summary.broken), total)
		} else {
			fmt.Fprintf(&b, "query contract corpus: %d contract(s) reported BROKEN\n",
				len(summary.broken))
		}
		for _, name := range summary.broken {
			fmt.Fprintf(&b, "  BROKEN  %s\n", name)
		}
		fmt.Fprintln(&b, "\nEach one is a defect in the query engine, not a test to adjust.")
	} else {
		fmt.Fprintln(&b, "query contract corpus: no evaluated contract reported broken")
	}

	if !complete && includeIncompleteDetails {
		fmt.Fprintln(&b, "\nRequired contract scopes not run:")
		for _, name := range summary.incomplete {
			fmt.Fprintf(&b, "  NOT RUN  %s\n", name)
		}
	}

	return b.String(), complete
}
