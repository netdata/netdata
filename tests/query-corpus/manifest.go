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
	"L1/incremental-rates": {
		Proves:     "the db stores PER-SECOND rates regardless of update_every: a v1 child's raw counters through the parent's rrdset_done yield K*(mul/div)/UE per second (incremental at ue 1/2/5 incl. mul/div scaling; absolute control unscaled)",
		Components: []string{"rates-ue1-2-5", "rate-ue10"},
	},
	"L1/resets-overflows": {
		Proves: "parent rrdset_done reset/overflow arithmetic over the v1 wire: implausible backward step → zero increment + SN_FLAG_RESET; plausible 32-bit wrap reconstructs cap-relative delta — ONE LESS than the true modulo delta (cap 0xFFFFFFFF, pinned quirk, rulings batch); percentage-of-incremental-row pre-pass absorbs the reset (0% + RESET, survivors split 100%); a silence beyond the gap threshold resets the collection (no spike from the across-gap delta, no RESET, null gap rows); sub-second sample offsets BLEND adjacent sample rates across stored rows with exact mass conservation",
	},
	"L1/off-grid-timestamps": {
		Proves: "samples pushed OFF the absolute ue grid: STORAGE keeps the pushed timestamps exactly (retention proves it), while every VIEW re-grids to absolute ue multiples and serves boundary-INTERPOLATED values (envelope-pinned; the exact virtual-point oracle is layer-9 work)",
	},
	"L2/tier1-palette": {
		Proves: "tier1 rollup identity for the edge-data palette: aligned windows (partial first — T0 unaligned), interior gaps (partial counts + stored-empty windows), fractional anomaly rates, reset annotation LOST at tier1+ (pages store no flags), float32 sum/min/max write-rounding",
	},
	"L2/whole-chart-absence": {
		Proves: "never-stored tier windows (whole-chart gap) read identically to stored-empty windows: null + EMPTY annotation, with correct partial counts on the flanking windows",
	},
	"L2/sn-vs-original": {
		Proves: "tiers aggregate the ORIGINAL collected doubles, not tier0 storage_number-quantized values: 2^24+1 reads 16777220 at tier0 (decimal mantissa step) but f32(16777216)-derived at tier1",
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
	"CASE-017/tier-boundary-absorption": {
		Proves:  "a tier>0 query whose after equals a stored tier point end keeps that point out of the first bucket (was: absorbed, leaking pre-window data into (after, before] — the backward-expanded storage scan met the inclusive bucket-start check); tier0 control stays clean",
		FixedBy: "#23127",
	},
	"CASE-016/fresh-host-forgotten-on-restart": {
		Proves:  "best-effort graceful-restart regression: a freshly connected child remains queryable after restart. The bounded 10-second attempt prevents unbounded timing drift but exceeds the ordinary 5-second metadata scan, so it does not isolate the final shutdown scan from periodic persistence (was: fresh host forgotten after boot, dbengine data orphaned)",
		FixedBy: "#23120",
	},
	"L3/families": {
		Proves: "the listed pre-fleet time-grouping families equal their Go oracles over the mixed palette at group 10 (all-gap bucket, anomaly run, reset): average/sum/min/max, extremes, stddev/cv (Welford, sample variance, single-value=0), median + trimmed-median (value-range trim + R-7 quantile), percentile/trimmed-mean (slot-window means with fractional interpolation), ses/des (running state across buckets), incremental-sum (carry), countif (finite-numeric options)",
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
	"L3/registry-completeness": {
		Proves: "the complete pre-fleet time-grouping registry: all 48 accepted name strings (latest included since #23257) answer (21 variants/aliases beyond L3/families, alias==canonical), the documented countif operator aliases (!= <> >: <: : ==), surrounding spaces in otherwise valid finite-numeric expressions and the zero-length default, numeric option overrides with clamps (percentile [0,100], trimmed-mean/median [0,50]), and unknown names silently parse to average; CASE-023 independently proves the four fleet groupings, bare operands and invalid-expression handling",
	},
	"L3/anomaly-bit-option": {
		Proves:     "options=anomaly-bit replaces fetched values with per-point anomaly rates BEFORE time-grouping: 0/100 at tier0 identity, buckets aggregate the rates (average = bucket anomaly %, max = any-anomaly), group-by consumes them as values (sum adds across members, gaps stamp PARTIAL), and tier>0 feeds FRACTIONAL window rates (100*anomaly_count/count)",
		Components: []string{"option", "tier-rates"},
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
	"CASE-020/mixed-rate-gauge-units": {
		Proves: "grouping a rate and gauge with the same resulting volume units is independent of metric encounter order: both orders produce exact volume units and labels, never the first contributor's source units",
	},
	"CASE-020/badge-sum-units": {
		Proves: "the badge endpoint renders both the exact sum value and its transformed volume units for rates, while average and gauge results keep their correct units",
	},
	"CASE-020/badge-filtered-metric-units": {
		Proves: "badge unit detection uses the same archived and dimension-filtered metric set as the query: selecting an archived rate while a live gauge shares the chart reports the rate's integrated volume and units",
	},
	"L4/family-tier-matrix": {
		Proves: "the 17 listed pre-fleet grouping families are served exclusively from forced tier1 and, over 6 windows per bucket, equal the fetch-aware oracle (min/max/sum fetch their tier fields, all else the per-window average — avg-of-averages pinned quantitatively with unequal counts); exact anomaly rates come from tier counts, EMPTY annotations match null placement, every numeric annotation is clear, and window alignment rounds `before` up to group multiples",
	},
	"L4/auto-tier-selection": {
		Proves: "with no tier param the planner picks the COARSEST tier whose density is acceptable (>= HALF the wanted points, wanted floored at QUERY_PLAN_MIN_POINTS=10): 1s buckets from tier0, 60s from tier1, and even 3600s buckets from tier1 while it covers (tier2 needs >= 5h windows — 5 x 3600s — or coverage gaps); db.per_tier points-read is pinned exclusive, and values, anomaly rates and exact EMPTY/numeric annotations equal the serving tier's oracle",
	},
	"L4/minmax-absolute-semantics": {
		Proves:     "time_group=min returns the value CLOSEST to zero and max the value FURTHEST from zero (min.h/max.h fabs comparisons) — visible only on negative/mixed data; pinned green in L3 sign-semantics + L4 matrix; RULING PENDING (arithmetic min/max would be a behavior change; extremes already provides champion-by-abs)",
		Components: []string{"tier0-min", "tier0-max", "tier1-min", "tier1-max"},
	},
	"L4/plan-switching": {
		Proves:     "queries spanning tiers with DIFFERENT retention are served by multiple plans: a dedicated 11M-sample fixture rotates tier0 at the 25MB quota floor while tier1 keeps the head; the discovered boundary is checked with per-side values, tier1-only controls, exact availability, authoritative fine-tier event/flap rows, and event totals bounded to raw truth plus one crossing coarse representative per seam",
		Components: []string{"seam", "head-only", "condition-groupings"},
	},
	"L4/three-tier-join": {
		Proves: "three tiers with different retention depths join inside one query: tier0 keeps the newest slice, tier1 outlives it and tier2 outlives both. Five zoom levels pin coverage, default newest-first raw wire order, canonical ascending order, range and outward alignment across both seams; forced tiers pin exact controls, while automatic seams pin exact availability, authoritative finer-tier event/flap rows, and event totals bounded to raw truth plus one crossing coarse representative per seam",
	},
	"L5/group-by-matrix": {
		Proves: "level-1 group-by, BOTH contracts: every key (selected, dimension, instance, node, label, context, units) x every aggregation (average, min, max, sum, extremes) over a 2-node x 2-instance x 3-dim palette returns the exact unique t0+1..t0+60 grid and equals the member-enumeration oracle — non-raw converts (average divides, ar/gbc), raw defers (sums undivided, ar accumulated, per-point counts on the wire and no hidden schema field); PARTIAL stamping and group naming pinned (instance = id@guid, node = machine guid, label = value)",
	},
	"L5/percentage": {
		Proves: "aggregation=percentage with a dimensions selector returns the exact unique t0+1..t0+60 grid: non-raw converts n*100/(n+h) per group; raw defers with a result-wide hidden field whose cell is the finite hidden sum when the group has hidden contributors and null otherwise; group_by=dimension is degenerate (hidden dims group separately — flat 100). percentage-of-instance (the exclusive single-key shorthand) converts EVEN IN RAW mode with no hidden — safe, per-instance groups never span agents",
	},
	"L5/statistics": {
		Proves:  "per-group view statistics (D-B SETTLED, #23097 verified numerically): non-average aggregations average over view ROWS (mean plotted value, row-extreme min/max); AVERAGE keeps the weighted (pre-division sum, contributions) pair; raw keeps (sum, count) untouched for the cloud",
		FixedBy: "#23097",
	},
	"L5/anomaly-statistics": {
		Proves: "jsonwrap-v2 per-dimension anomaly arrays: view.dimensions.sts.arp = mean of the plotted rows' anomaly rates, db.dimensions.sts.arp = anomaly rate of the fetched db points; stored NAN gap points are excluded from BOTH counts",
	},
	"L5/multi-key-group-by": {
		Proves: "multi-key group_by: groups are attribute TUPLES, ids join in the FIXED engine order (dimension, instance, label, node, context, units) regardless of request order; instance drops @node when node is in the mask; selected and percentage-of-instance collapse rules; avg alias; unknown aggregation silently parses to average",
	},
	"L6/two-pass-matrix": {
		Proves:     "two-pass oracle over 10 key-chains (including cross-key union partitioning) x 7 aggregation chains with no average boundary x non-raw/raw: pass-2 values, PARTIAL propagation and group_by_label[1]; non-raw point and final view-statistics anomaly metadata is weighted by the raw metric contributors beneath pass-1 groups, while raw keeps the anomaly numerator and the old Agent-Cloud prior-group count because the same field can be a Cloud-rewritten average divisor; the result has no hidden schema field; a live-edge fixture proves that a raw-contributor decline inside one pass-1 group retains the incomplete final row, marks it PARTIAL, and does not shorten the request timestamp grid",
		Components: []string{"matrix", "live-edge-partial-row"},
	},
	"L6/two-pass-average-boundary": {
		Proves:     "sum→average needs separate denominators across two passes: non-raw value divides by contributing pass-1 groups, while anomaly rate remains weighted by the raw metric contributors beneath those groups; raw mode deliberately leaves both numerators undivided and reports the prior-pass group count required by the old Agent-Cloud average merge contract. Class C average→sum stability separately preserves the released prior-group anomaly divisor without ruling on average composition",
		Components: []string{"sum-to-average", "average-to-sum-held"},
	},
	"L6/two-pass-percentage": {
		Proves:     "percentage as the pass-2 aggregation: pass 1 runs in shadow hidden mode, the percentage pass folds hidden sums into each normal group's denominator (v*100/(v+h)), and an incomplete shadow bucket taints PARTIAL; non-raw anomaly metadata remains weighted by visible raw metric contributors, while raw mode defers value conversion, declares a result-wide hidden field with finite sums or null cells, and preserves the visible prior-pass group count. Class C stability controls for percentage→sum and percentage-of-instance→sum separately prove that complete rows remain non-PARTIAL without ruling on percentage pooling or weighting",
		Components: []string{"sum-to-percentage", "percentage-to-sum-held", "percentage-of-instance-to-sum-held"},
	},
	"CASE-018/multipass-average": {
		Proves: "with average at pass 1, pass 2 consumes each finalized pass-1 group average: [dimension,average]→[selected,average] equals the mean of those group averages, not the mean of unfinalized group sums",
	},
	"L7/formatters": {
		Proves: "classic v1 formats over a hostile fixture: csv/tsv byte-exact (newest-first default, natural order option, unquoted header cells pinned as current contract), ssv/ssvcomma/array exact row-sum values, csvjsonarray VALID JSON with NUMERIC timestamps (#23115/#23117 pinned), markdown/html structure, and strict JSON/JSONP/datatable row schemas with exact fixture-derived timestamps, values and gaps",
	},
	"CASE-022/time-group-latest": {
		Proves:  "time_group=latest works end to end: per-bucket last collected value, empty buckets stay empty, sign preserved without options=absolute and erased with it. A one-point explicit window containing the newest stored sample uses the collector cache with zero db reads, the RAW un-quantized double and anomaly rate 0 by design, while its row timestamp remains on the request-derived grid. The API's before=0 database-end sentinel retains its existing newest-sample grid for alert-style queries, including a v1 natural-points update_every=10 fixture whose newest timestamp is off cadence. The storage path (selected-tier) keeps SN quantization and the engine-generic anomaly rate. CASE-034 separately proves across 60-second fixtures that near-now explicit and relative LATEST queries cannot derive their public grids from stored timestamps",
		FixedBy: "#23257",
	},
	"CASE-023/fleet-time-groupings": {
		Proves: "the four fleet time-aggregations and their shared expression grammar: percentage-of-samples (canonical, countif alias) / percentage-of-time / number-of-flaps / number-of-times, each echoing its canonical name and transforming the response units (%/%/flaps/events); gap tokens (nan|null|gap|empty) pull gap slots in for percentage-of-samples, number-of-flaps and number-of-times while an expression without one keeps them invisible there (percentage-of-time always counts uncollected time - CASE-023/percentage-of-time-denominator); previous|last compare against the previous COLLECTED sample so a counter reset is a reboot and the first sample never matches; flaps count observed false->true transitions only, carried across buckets; a gap contributes its SLOT width, not the zero span of QUERY_POINT_EMPTY",
	},
	"CASE-023/expression-grammar-and-state": {
		Proves: "the shared expression parser accepts every documented operator spelling, bare operands, gap aliases and previous|last; only an absent option or a zero-length whole expression defaults to ==0 at all four entry points, while whitespace-only, incomplete, malformed and non-finite expressions are rejected; predecessor and flap state survive gaps and bucket flushes",
	},
	"CASE-023/mcp-surface": {
		Proves: "a protocol-valid MCP lifecycle advertises all four condition groupings, the countif alias and string time_group_options; exact nonzero numeric and gap-operand calls prove option forwarding, canonical echo, percentage-of-samples versus percentage-of-time behavior across a cadence change, units, JSON2 schema, timestamps, anomaly rates and annotations, while missing, blank, non-string, incomplete, malformed and non-finite required expressions return -32602 Invalid Params with structured INVALID_PARAMS details",
	},
	"CASE-023/tier-estimation": {
		Proves: "ABOVE tier 0 a stored point is min/max/avg over many samples, not a sample: percentage-of-time estimates the share of each stored window that satisfied the condition with the two-point mass model (weight(max) = (avg-min)/(max-min)). For a 0/1 availability signal at a steady collection cadence, the average is exactly the fraction of elapsed time at 1, so up% and down% of a mixed window sum to 100 instead of both answering 'never'. If cadence changes inside one stored interval, the average is sample-weighted instead; that separate contract is pinned by CASE-023/cadence-change-availability. A mixed window counts one flap and at most one occurrence because ordering does not survive the rollup; percentage-of-samples keeps its historical tier behaviour",
	},
	"CASE-023/cadence-change-availability": {
		Proves:     "when collection cadence changes, percentage-of-time keeps exact wall-time availability at tier 0. At tiers 1 and 2, a stored interval containing both cadences exposes only min/max/avg/count, so the approved two-point estimator is necessarily sample-weighted: the test derives that estimate from the fixture ledger and pins it separately from the exact steady-cadence contract",
		Components: []string{"slows-down", "speeds-up"},
	},
	"CASE-023/historical-gap-slots-after-cadence-change": {
		Proves: "after a metric speeds up from every 10 seconds to every second, gaps in its old history retain their historical slot weight. One missing old slot counts once, not ten times because the chart's latest cadence is one second. Asserted exactly at forced tiers 0, 1 and 2",
	},
	"CASE-023/tier-resolution-matrix": {
		Proves: "all four condition groupings keep fixture-derived answers across forced tiers 0, 1 and 2 while downsampling, reading at storage resolution and upsampling, including a 10-second metric upsampled from tier0; exact rows cover gaps, changing neighboring rollup averages and interpolation, non-binary estimates, event de-duplication and counter predecessor state, with strict db.per_tier source proof",
	},
	"CASE-023/redelivery": {
		Proves: "the same stored point handed to several result buckets means different things to different groupings: percentage-of-samples treats a delivery AS a sample and must answer in EVERY bucket (skipping repeats leaves EMPTY holes where a value used to be), while number-of-times and number-of-flaps must count a stored window at most once however many buckets it was delivered into. Counted-once and answered-everywhere are separate contracts and both hold: a bucket a wide point covers on its own carries no occurrence but is still a zero, not EMPTY - skipping the repeat entirely punches holes into a chart wherever the user zooms past the stored resolution",
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
		Proves: "the denominator of percentage-of-time is the SELECTED duration, not the collected part of it: one collected second reading 1 followed by 99 seconds with nothing collected is 1% at `==1` and 99% at `==gap`, because uncollected time is time the condition did not hold - answering 100% would turn a node that went silent into a perfectly healthy one. percentage-of-samples keeps the opposite contract and reads 100%, because it answers about the samples it was handed",
	},
	"CASE-023/trailing-gaps": {
		Proves: "a condition that names a gap keeps accounting to the END of the requested window: the query engine stops walking a few buckets after a dimension's storage is exhausted and lets the caller fill the rest with EMPTY, which is the same answer for every other aggregation but silently truncates an outage for `==gap` - a dimension that stops being collected while its chart keeps going must read 100% gap for every remaining bucket, not for eleven of them",
	},
	"CASE-023/window-outside-retention": {
		Proves: "the limit of the trailing-gap contract: a window that does not touch the dimension's retention at all. A node silent for three days, asked what share of the last hour it was unreachable, must answer 100% - the same answer it gives for the tail of a window it partly covers. Metric selection is what stands in the way: a metric whose retention misses the window is dropped because it can only answer 'nothing here', which is true for every other aggregation and exactly backwards for one that accounts for uncollected time - dropping it turns a total outage into an empty chart",
	},
	"CASE-023/gap-weight": {
		Proves: "a gap counts in stored SLOTS, not seconds: percentage-of-samples weighs uncollected time against the collection interval, so on a 10s metric a 100s hole is ten missing samples and not a hundred - measuring it against the query grid (1s for an ordinary query) would let one missing slot outweigh ten collected ones",
	},
	"CASE-023/tier-wide-point": {
		Proves: "when the view grid is FINER than the stored data — a dashboard zoomed into a window only tier 1 still covers — a stored point is re-delivered to every bucket it spans, carrying its original start and an interpolated value; the share of time answers the SAME estimate in each of those buckets, and one stored window yields at most ONE occurrence and ONE flap, because a re-delivery is the same window seen again, not a second event (counting the repeats inflates an SLO by exactly the zoom factor)",
	},
	"CASE-023/tier-anomaly-bit": {
		Proves: "with options=anomaly-bit above tier 0 the value is the stored window's anomaly RATE while min/max still describe the metric, so the condition is answered on the rate itself — a window either satisfied it or it did not (100/0), never a fraction estimated across two unrelated domains; >=N and <N partition every window",
	},
	"CASE-024/zoom-into-slow-metrics": {
		Proves: "a metric collected once a minute, once per ten minutes or once an hour still answers when the dashboard zooms BELOW its collection interval: a 60-point request over a window shorter than one sample interval, fully inside the collected span, returns rows that carry the value - a chart that empties out when the user zooms in is indistinguishable from an outage",
	},
	"CASE-025/carry-survives-gaps": {
		Proves: "sum assigns every stored record to result rows by exact time overlap, including an interior settlement-only row, a following truly empty row, and the final partial row before retention ends. Varying record values make the creditor identity observable: the exact per-row Class-A oracle fails if a whole record is paid to one row, interpolation changes its value, a remainder is dropped, or a neighboring record pays it, even when the whole-window total is unchanged",
	},
	"CASE-025/anomaly-bit-not-blended": {
		Proves: "a bucket lying entirely inside one stored window reports THAT window's anomaly rate, un-blended. options=anomaly-bit answers about the anomaly RATE, so sum's seconds-owed arithmetic is skipped for it - and the other half of the boundary machinery must not reach it either. A bucket carved inside a fully-anomalous window contains no sample from the window before it, so blending would report the metric as less anomalous than every sample under the bucket actually was. Asserted under average, min, max and sum over a hard 0 -> 100 step on a stored window boundary: all three buckets read 100, and a blended 33/67/100 would be the step smeared backwards into seconds it never touched. Records the ruling that refuted a review finding which had assumed the blended answer was the correct one",
	},
	"CASE-026/anomaly-rate-covers-the-paid-seconds": {
		Proves: "on tier 0, each 35-second row carries the exact sum or average value, anomaly rate of raw sample timestamps in `(row_start,row_end]`, and RESET annotation belonging to that same row. Alternating all-healthy/all-anomalous rows expose fetched-record or interpolation-based metadata, while the last reset sample pins the final five-second sum share, ARP and RESET to the settlement row; later rows are exactly empty",
	},
	"CASE-026/totals-survive-a-plan-switch": {
		Proves: "a sum total does not change because the answer had to be assembled from two tiers. sum carries across rows the seconds a record still owes, and a plan switch is the one moment the record stream itself jumps - the engine reads ahead into the next tier and may keep or discard either side. A carry dropped or double-paid there moves the total by up to one stored record, a whole minute of data above tier 0. Asserted over a discovered, rotated tier0 head at four zooms from 1s to 300s buckets; an exact two-row seam covers a first-row join and an exact three-row seam covers a carried coarse record plus a trailing fine-tier control row. A sparse control adds 120-second bursts separated by 1,920-second true DBENGINE page holes: forced tier0 and tier1 queries prove the exact fine burst and its overlapping coarse rollup, while two automatic 10-second grid phases place the fine page boundary on both adjacent row grids, leave every hole exactly empty, conserve every retained sample and report exact fine-tier anomaly membership once fine retention begins. A separate automatic tail seam proves tier1 ends at t0+220 while retained tier0 resumes at t0+2021: three 700-second rows must be exactly 100000, EMPTY and 61000, so a coarse record stops at its stored end and the first point on the later fine page is not lost. Seventeen 120-second rows over the same storage shape additionally hold the expired coarse record in executor state: the first row is 100000, all fifteen storage-hole rows are exactly EMPTY with empty metadata, and the final fine row is 61000, proving zero overlap never becomes a numeric zero or stale metadata. Partial higher-tier anomaly metadata is not claimed because a stored rollup cannot locate its constituent timestamps",
	},
	"CASE-027/incremental-sum-conserves-across-zoom": {
		Proves: "time_group=incremental-sum answers how much a value changed in a bucket, so the buckets of a window telescope and add up to the first-to-last rise at every resolution. At forced tier 0 the exact one-row-per-sample grid has one opening null followed by 59 rows of +7 with zero ARP/PA, while finer and coarser controls preserve the same total; a one-sample flush must carry its real baseline rather than overwrite it with an unset last value",
	},
	"CASE-028/rate-with-gaps-totals-what-was-measured": {
		Proves: "a rate metric with holes in it totals the seconds that were MEASURED, on every tier. Above tier 0 a stored record carries a sum, a count and a wall-clock width, and where seconds under it were never collected the width and the measured time differ - using the width invents volume for time nobody watched and makes the answer a property of retention. The matrix separates the candidate arithmetics: update_every 1 AND 10 (at 1 second, sum x interval is indistinguishable from sum alone), tier 1 AND tier 2 (whose own strides differ by sixty), and gapped plus no-gap controls. Every query spans whole stored records and uses bucket counts that divide the window exactly, so the oracle has no edge estimate or rounded query span",
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
		Proves:     "an explicit absolute virtual-points query has one immutable public timestamp grid, independent of values, collection cadence, gaps, retention coverage, requested tier option, and time aggregation. The historical matrix pins view.after, view.before, view.update_every, row count, and every wire timestamp for aligned and unaligned points=1/7 plus aligned points=60 across dense update_every=1 and 10, gapped, and partial-retention hosts; automatic and requested tiers 0/1/2; and average, sum, and latest on /api/v3/data. A representative aligned one-point query pins /api/v2/data on every fixture shape. Near-now aligned and unaligned explicit absolute plus unaligned relative LATEST points=1 queries across two Agents with different newest stored timestamps must keep request/clock-derived grids; the unaligned requests additionally prove collector-cache service with zero storage reads. A near-live six-row matrix proves that losing the final collected sample leaves an EMPTY cell on the same complete grid instead of truncating its timestamp. Historical and hot-edge values are deliberately ignored because value fixes must preserve this timestamp contract",
		Components: []string{"dense-ue1", "dense-ue10", "gapped-ue1", "partial-ue1", "hot-edge-data-independence", "near-live-partial-data"},
	},
	"CASE-035/transition-volume-slowing-down": {
		Proves: "a metric slowing from update_every 1 to 10 preserves exact fixture-measured rate volume in the complete historical control row and the next row containing both cadences. Asserted at forced tiers 1 and 2 against a raw tier0 control. These forced-tier queries do not cover an automatic plan switch during a cadence change",
	},
	"CASE-035/transition-volume-speeding-up": {
		Proves: "the update_every 10 to 1 mirror preserves exact fixture-measured rate volume in the complete historical control row and the next row containing both cadences, at forced tiers 1 and 2 against a raw tier0 control. These forced-tier queries do not cover an automatic plan switch during a cadence change",
	},
	"CASE-036/absolute-across-plan-seam": {
		Proves: "options=absolute applies to the point read from the incoming tier plan: a negative flat line stays exactly positive in every 1-second row across an automatic tier1-to-tier0 seam, with tier1-only and tier0-only controls and strict source-tier evidence",
	},
	"CASE-037/rate-volume-across-three-tier-cadence-query": {
		Proves: "one exact one-second sum query crosses automatic tier2-to-tier1 and tier1-to-tier0 plan seams, then crosses an update_every 1-to-10 transition where the rate also changes. Every returned row and the complete total come from the fixture ledger, and db.per_tier must prove all three tiers contributed. Forced-tier mixed-cadence records remain covered separately by CASE-035",
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
		Proves: "every grouping answers every exact bucket across fully collected data at tiers 0 and 1, with 10s, 60s, 300s and 600s buckets; EMPTY says 'no data here', so no single-point defect is accepted as an excuse for these multi-sample buckets",
	},
	"L10/buckets-finer-than-stored-data-answer": {
		Proves: "a bucket NARROWER than the stored data still answers, for every grouping: above tier 0 a stored point covers many seconds, so a dashboard drawn finer gives the engine buckets that a single re-delivered point covers on its own. This is where number-of-flaps and number-of-times punched holes — they dropped the repeat entirely, sample count and all. L10/no-holes-inside-data cannot reach it: it deliberately uses buckets wide enough to hold several samples so a grouping needing two of them is not asked for the impossible, and that fairness is exactly what hides this case",
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
		Proves: "cutting a window in two preserves the total across a pairwise matrix of dense/gap/sparse data, 1s/10s cadence, tiers 0/1, 1/3/10 points per record, offsets 0/17/30 and none/natural-points/absolute; ordinary and absolute queries must use the requested resolution, natural-points may declare its own complete grid, every query has nonempty exact wire/canonical coverage, and only the actual fixture content of the one record crossing the midpoint plus wire-print epsilon is allowed",
	},
	"L11/randomised-slicing": {
		Proves: "seeded generated cases materialize window, tier duration and bucket count from their axes, require additivity and conservation coverage for every shape×cadence×tier combination after polling the requested tier's retention prerequisite, and greedily shrink failures to a locally minimal case under the enumerated simplifications; QUERY_CORPUS_SEED replays the sequence",
	},
	"L11/totals-match-what-was-pushed": {
		Proves: "after polling every shape×cadence×tier retention prerequisite, each combination whose buckets are at least one collection interval wide totals the fixture's exact collected-sample sum; only the deduplicated fixture contents of records crossing the two outer edges plus wire-print epsilon are allowed, with positive fixture content requiring numeric response coverage",
	},
	"L9/virtual-points": {
		Proves:     "for the covered default-mode tier0 fixtures, the source-derived virtual-point selection oracle models whole-point inclusion and boundary interpolation over a preconstructed stored-point stream; exact cases cover grid-cut intervals, off-grid identity and upsampling including the first unanchored straddler",
		Components: []string{"interpolated-buckets", "off-grid-identity", "upsampling"},
	},
	"L9/window-normalization": {
		Proves:     "a negative `after` is relative to `before` (identical to the absolute equivalent); (0,0) resolves to the ~600s grid-aligned default window ending NOW with an exact empty dimension-id array — NOT the full retention (the reason backdated fixtures settle via explicit windows); time_resampling (v1 gtime) forces the bucket size up",
		Components: []string{"relative-window", "default-relative-window", "time-resampling"},
	},
	"L9/natural-points": {
		Proves: "options=natural-points keeps the db count and spacing with raw sample values while timestamps snap onto the absolute update-every grid; boundary slots are pinned to the raw sample or its phase interpolation toward the next as a bounded two-candidate contract",
	},
	"L9/live-edge": {
		Proves: "queries whose explicit `before` lies past NOW first shift both endpoints to the query-time clock, then retain the complete normalized timestamp grid without data-dependent truncation; grid rows wholly before retention remain explicit EMPTY rows",
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
		Proves: "the single-series formats reduce each row by the requested option: min2max = max-min (0 on single-value rows), min, max, average — exact cells over the formatter fixture (default sum pinned by L7/formatters)",
	},
	"API/fallbacks-and-limits": {
		Proves:     "unknown v1 format silently serves json; unknown weights method silently runs ks2; cardinality_limit at 2 folds five dimensions into the remaining bucket and at >= the dimension count folds nothing",
		Components: []string{"fallback-pins", "cardinality-limit-sweep"},
	},
	"W/value": {
		Proves:     "weights method=value: per-metric weight = the window average over NATURAL points with the after-INCLUSIVE window (121 points for a 120s span — rulings batch); strict MULTINODE rows contain exactly one instance, context and node rollup with the mean of their dimensions and the required index/null layout; the per-dimension timeframe stats (min/avg/max/sum/count/anomaly_count) are exact; method=value NEVER rank-normalizes",
		Components: []string{"multi-node", "never-spreads"},
	},
	"W/anomaly-rate-per-metric": {
		Proves: "weights method=anomaly-rate on the PER-METRIC path (no context selector) applies the anomaly bit: raw weights are the true window anomaly rates; the NONZERO default (no options= given) drops zero-weight results, any explicit options= keeps them",
	},
	"W/anomaly-rate-multidim": {
		Proves:  "method=anomaly-rate implies the anomaly bit on EVERY path: the bare method and the explicit options=anomaly-bit are equivalent, both returning true anomaly rates through the multi-dimensional path (was: the bare method ranked by plain value averages there while per-metric and MCP forced the bit)",
		FixedBy: "#23212",
	},
	"W/volume": {
		Proves: "weights method=volume: weight = (highlight-baseline)/baseline x the fraction of highlight time above/below the baseline average (countif); metrics with EQUAL window averages are skipped entirely",
	},
	"W/ks2": {
		Proves: "weights method=ks2 exact endpoints: identical consecutive-diff distributions weigh exactly 0, fully one-sided diff distributions with n*d^2>=18 weigh exactly 1 (KSfbar special cases); spread_results_evenly rank normalization pinned via the Go port (unique-value slots, ties share a slot); intermediate KS probabilities are a recorded deferral (KSfbar port)",
	},
	"L8/post-processing": {
		Proves:     "options=percentage (v2/v3 FORCE absolute with it — and with any non-dimension group-by: shares computed over |values|), options=absolute (|v| at fetch), nonzero (drops all-zero dims; self-neutralizes when everything is zero), null2zero (gap cells become 0), cardinality_limit (top N-1 by |view sum| + 'remaining X dimensions' fold of per-row sums)",
		Components: []string{"post-processing", "nonzero-all-zero", "cardinality-limit"},
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
