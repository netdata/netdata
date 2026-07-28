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
	Name    string
	Proves  string
	FixedBy string // PR or commit that fixed it
}

var manifest = map[string]ManifestCase{
	"L0/live-burst": {
		Proves:  "live BEGIN2/SET2 burst round-trips byte-exact without settle discipline (pins #23096 green)",
		FixedBy: "#23096",
	},
	"L0/live-paced": {
		Proves: "legacy spike-3 pacing still works (control for live-burst)",
	},
	"L0/replication": {
		Proves: "replication dialogue round-trips byte-exact incl. gap and anomaly bits",
	},
	"L0/two-children": {
		Proves: "same context from two children answers independently per host",
	},
	"L0/labels": {
		Proves: "CLABEL chart labels reach the query path (group_by=label)",
	},
	"L0/restart": {
		Proves: "fixtures survive daemon restart byte-identical (journal-v2 read path)",
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
		Proves: "receiver teardown with queued replies to a dead child stays crash-free: mid-dialogue disconnect and a 30-cycle disconnect soak",
	},
	"L1/palette": {
		Proves: "tier0 ingestion identity for the edge-data palette: complete, leading/interior-run gaps, trailing short retention, reset (AR and lone-R), anomaly runs, negatives, all-zero, update_every=5",
	},
	"L1/single-point": {
		Proves: "single-point ingestion exact through a wide window; PINS the 1-point-window view expansion (ue 1→2, bucket at t0+2) as a layer-9 seed",
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
		Proves: "the db stores PER-SECOND rates regardless of update_every: a v1 child's raw counters through the parent's rrdset_done yield K*(mul/div)/UE per second (incremental at ue 1/2/5 incl. mul/div scaling; absolute control unscaled)",
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
		Proves:  "a child first connected < one metadata scan cycle (5s) before a graceful restart SURVIVES it: the metasync shutdown path now runs a final host scan, so the fresh host's metadata reaches sqlite regardless of scan phase (was: forgotten — host 404 after boot, dbengine data orphaned)",
		FixedBy: "#23120",
	},
	"L3/families": {
		Proves: "every registry time_group equals its Go oracle over the mixed palette at group 10 (all-gap bucket, anomaly run, reset): average/sum/min/max, extremes, stddev/cv (Welford, sample variance, single-value=0), median + trimmed-median (value-range trim + R-7 quantile), percentile/trimmed-mean (slot-window means with fractional interpolation), ses/des (running state across buckets), incremental-sum (carry), countif (options grammar)",
	},
	"L3/sign-semantics": {
		Proves: "percentile/trimmed-mean walk from the top when a bucket has any negative value; extremes champions by |abs|; pinned over all-negative and mixed-sign fixtures",
	},
	"L3/sparse-buckets": {
		Proves: "single-numeric-value buckets: stddev 0.0 (not null), pass-through for value families, incremental-sum all null (leading bucket loses its seed; empty resets the carry) — pinned current contract",
	},
	"L3/identity-smoothing": {
		Proves: "ses/des at group=1 use the requested points (capped 15) as the smoothing window; incremental-sum at identity is all null — pinned current contract",
	},
	"L3/registry-completeness": {
		Proves: "the FULL time-grouping registry: all 47 accepted name strings (latest included since #23257) answer (20 variants/aliases beyond L3/families, alias==canonical), the complete countif grammar (! !: >: <: <> : == spaces empty), numeric option overrides with clamps (percentile [0,100], trimmed-mean/median [0,50]), unknown names silently parse to average; PINNED QUIRK (rulings batch): bare-number countif options lose their first digit",
	},
	"L3/anomaly-bit-option": {
		Proves: "options=anomaly-bit replaces fetched values with per-point anomaly rates BEFORE time-grouping: 0/100 at tier0 identity, buckets aggregate the rates (average = bucket anomaly %, max = any-anomaly), group-by consumes them as values (sum adds across members, gaps stamp PARTIAL), and tier>0 feeds FRACTIONAL window rates (100*anomaly_count/count)",
	},
	"L3/sum-over-time-volume": {
		Proves: "time_group=sum has two modes: RATE-stored metrics (incremental) multiply each point by its duration — the sum is the VOLUME at any update_every; non-rate metrics sum plainly",
	},
	"CASE-020/sum-over-time-units": {
		Proves: "summing a rate over time produces a volume, but the response units keep the rate form — 'units/s' should become 'units' when time_group=sum integrates a rate",
	},
	"L4/family-tier-matrix": {
		Proves: "every grouping family over FORCED tier1 with 6 windows per bucket equals the fetch-aware oracle (min/max/sum fetch their tier fields, all else the per-window average — avg-of-averages pinned quantitatively with unequal counts); bucket anomaly rates from tier counts; window alignment rounds `before` UP to group multiples",
	},
	"L4/auto-tier-selection": {
		Proves: "with no tier param the planner picks the COARSEST tier whose density is acceptable (>= HALF the wanted points, wanted floored at QUERY_PLAN_MIN_POINTS=10): 1s buckets from tier0, 60s from tier1, and even 3600s buckets from tier1 while it covers (tier2 needs >= 5h windows — 5 x 3600s — or coverage gaps); db.per_tier points-read pinned exclusive; values equal the serving tier's oracle",
	},
	"L4/minmax-absolute-semantics": {
		Proves: "time_group=min returns the value CLOSEST to zero and max the value FURTHEST from zero (min.h/max.h fabs comparisons) — visible only on negative/mixed data; pinned green in L3 sign-semantics + L4 matrix; RULING PENDING (arithmetic min/max would be a behavior change; extremes already provides champion-by-abs)",
	},
	"L4/plan-switching": {
		Proves: "queries spanning tiers with DIFFERENT retention are served by multiple plans: a dedicated daemon with tier0 at the 25MB quota floor rotates its head out (boundary DISCOVERED from db.per_tier, ~19h evicted at 10M samples), a straddling query reads tier1 (head) + tier0 (tail) with per-side oracle values, and a head-only query is served by tier1 alone",
	},
	"L4/three-tier-join": {
		Proves: "three tiers with DIFFERENT retention depths, joined inside one query: every tier at the engine's 25MiB floor and the tiers brought close together (1s/5s/10s) so each fills its own quota from one fixture — tier0 keeps the newest slice, tier1 outlives it, tier2 outlives them both. The whole retained duration is then read at five resolutions from 845s buckets down to 2.9s: no bucket inside the span is ever empty, time never runs backwards, every value stays inside the generator range, and across the resolutions all three tiers contribute (the planner picks the coarsest tier that can supply the requested density, so WHICH tier answers changes with the zoom). Pins that alignment rounds the grid OUTWARD, so the leading buckets can precede retention and are legitimately empty; and that asking for buckets finer than the serving tier is upsampling, not a seam defect",
	},
	"L5/group-by-matrix": {
		Proves: "level-1 group-by, BOTH contracts: every key (selected, dimension, instance, node, label, context, units) x every aggregation (average, min, max, sum, extremes) over a 2-node x 2-instance x 3-dim palette equals the member-enumeration oracle — non-raw converts (average divides, ar/gbc), raw defers (sums undivided, ar accumulated, per-point counts on the wire); PARTIAL stamping and group naming pinned (instance = id@guid, node = machine guid, label = value)",
	},
	"L5/percentage": {
		Proves: "aggregation=percentage with a dimensions selector: non-raw converts n*100/(n+h) per group; raw defers (selected sums + hidden denominator on the wire); group_by=dimension is degenerate (hidden dims group separately — flat 100). percentage-of-instance (the exclusive single-key shorthand) converts EVEN IN RAW mode with no hidden — safe, per-instance groups never span agents",
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
		Proves: "two-pass mechanics oracle over 10 key-chains (incl. cross-key: pass 1 partitions by the UNION of both passes' keys) x 8 agg-chains (incl. mixed) x non-raw/raw: values, PARTIAL propagation, group_by_label[1]; ANOMALY RATE accumulates raw through both passes and divides ONCE by the final group count — inflated by members-per-group for EVERY chain (ar analog of the avg-of-sums family, pinned as current mechanics; rollup SOW evidence); raw pin: point count = number of pass-1 GROUPS, values/ar unconverted",
	},
	"L6/two-pass-percentage": {
		Proves: "percentage as the PASS-2 aggregation: pass 1 runs in SHADOW hidden mode (hidden dims accumulate in per-group shadow buckets), the pct pass folds them into the denominator of their normal group (v*100/(v+h)); an incomplete shadow bucket taints the point PARTIAL through hgbc; raw defers everything — visible sum as value, hidden on the wire, count = visible pass-1 groups",
	},
	"CASE-018/multipass-average": {
		Proves: "AVERAGE at pass 1 of a two-pass group-by feeds pass 2 the group SUMS (the per-group division never happens) — the final value is inflated by ~members-per-group (bug-list item 3 family; fix owned by SOW-20260701-query-rollup-hierarchical-correctness, in planning)",
	},
	"L7/formatters": {
		Proves: "classic v1 formats over a hostile fixture: csv/tsv byte-exact (newest-first default, natural order option, unquoted header cells pinned as current contract), ssv/ssvcomma/array exact row-sum values, csvjsonarray VALID JSON with NUMERIC timestamps (#23115/#23117 pinned), markdown/html/json/datatable/jsonp structure",
	},
	"CASE-022/time-group-latest": {
		Proves:  "time_group=latest works end to end: per-bucket last collected value, empty buckets stay empty, sign preserved without options=absolute and erased with it; points=1 with before at/near now (raw zero or resolved within one update_every of now) anchors the window at the newest stored sample and serves it from the collector cache — zero db reads, the RAW un-quantized double, anomaly rate 0 by design — while the storage path (selected-tier) keeps SN quantization and the engine-generic anomaly rate",
		FixedBy: "#23257",
	},
	"CASE-023/fleet-time-groupings": {
		Proves: "the four fleet time-aggregations and their shared expression grammar: percentage-of-samples (canonical, countif alias) / percentage-of-time / number-of-flaps / number-of-times, each echoing its canonical name and transforming the response units (%/%/flaps/events); gap tokens (nan|null|gap|empty) pull gap slots in for percentage-of-samples, number-of-flaps and number-of-times while an expression without one keeps them invisible there (percentage-of-time always counts uncollected time - CASE-023/percentage-of-time-denominator); previous|last compare against the previous COLLECTED sample so a counter reset is a reboot and the first sample never matches; flaps count observed false->true transitions only, carried across buckets; a gap contributes its SLOT width, not the zero span of QUERY_POINT_EMPTY",
	},
	"CASE-023/tier-estimation": {
		Proves: "ABOVE tier 0 a stored point is min/max/avg over many samples, not a sample: percentage-of-time estimates the share of each stored window that satisfied the condition with the two-point mass model (weight(max) = (avg-min)/(max-min)), which is EXACT for a 0/1 availability signal because there the average IS the fraction of time at 1 — so up% and down% of a mixed window sum to 100 instead of both answering 'never', which is what evaluating the condition on the stored average does; a mixed window counts one flap and at most one occurrence (no ordering survives the rollup); percentage-of-samples keeps its historical tier behaviour",
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
	"CASE-023/countif-bare-number": {
		Proves: "the shared expression parser fixes the bare-number digit swallow (countif.h:78 advances past the operator switch even when no operator matched, so options '5' targets 0) — the API is aligned to health, which has always parsed countif(5) as '=5' (health-config-unittest.c:96)",
	},
	"CASE-024/zoom-into-slow-metrics": {
		Proves: "a metric collected once a minute, once per ten minutes or once an hour still answers when the dashboard zooms BELOW its collection interval: a 60-point request over a window shorter than one sample interval, fully inside the collected span, returns rows that carry the value - a chart that empties out when the user zooms in is indistinguishable from an outage",
	},
	"CASE-019/v1-json-name-escaping": {
		Proves:  "v1 JSON-family formatters (json, jsonp, csvjsonarray, datatable) escape dimension names (was: raw between quotes — a double-quote in a name, or a label value via group_by=label, produced invalid JSON); the objectrows row keys are escaped like the header, and the google flavor (datatable+google_json) escapes the apostrophe of its single-quoted JavaScript labels while keeping the double quote raw",
		FixedBy: "#23216",
	},
	// Layer 10 — the invariants every grouping owes, swept across all of
	// them. The roster comes from the engine's own enum, so a grouping added
	// without a classification fails the sweep by name.
	"L10/roster-is-complete": {
		Proves: "the sweep covers EVERY grouping the engine offers: the roster is parsed from the RRDR_TIME_GROUPING enum and the registry that names each value, so a grouping added to the enum without a line in layer 10's table fails here by name — an unknown time_group silently falls back to `average`, so a missing one would otherwise be tested by accident and pass",
	},
	"L10/no-holes-inside-data": {
		Proves: "every grouping answers in every bucket whose width was collected end to end: a bucket wide enough to hold several samples has something to say about them, and EMPTY says 'no data here', which is what an outage looks like. Swept at tier 0 and tier 1, at bucket widths from 10s to 300s, so a grouping that genuinely needs two samples is never asked for the impossible",
	},
	"L10/buckets-finer-than-stored-data-answer": {
		Proves: "a bucket NARROWER than the stored data still answers, for every grouping: above tier 0 a stored point covers many seconds, so a dashboard drawn finer gives the engine buckets that a single re-delivered point covers on its own. This is where number-of-flaps and number-of-times punched holes — they dropped the repeat entirely, sample count and all. L10/no-holes-inside-data cannot reach it: it deliberately uses buckets wide enough to hold several samples so a grouping needing two of them is not asked for the impossible, and that fairness is exactly what hides this case",
	},
	"L10/order-statistics-stay-in-range": {
		Proves: "a central tendency or order statistic answers WITH the data, never past it: average/min/max/median/trimmed-*/percentile*/extremes/latest/ses stay inside the range of the samples that fed the bucket, at every tier. A value outside it is a number the aggregation invented — from an interpolation it should not have used, or from state another point left behind. sum/incremental-sum/stddev/cv/des are excluded with reasons from what they mean, not from what the engine returns",
	},
	"L10/dimensions-are-independent": {
		Proves: "what a dimension answers does not depend on which dimensions were queried alongside it, for every grouping at every tier: the aggregations carry state across buckets on purpose (the predecessor of a condition, the flap state, the smoothing level) and the engine walks dimensions through ONE grouping instance, resetting between them — a reset that misses a field makes an answer depend on its neighbours and on the order they were walked in",
	},
	"L10/totals-are-exact-across-zoom": {
		Proves: "a TOTAL over a fixed span is the same number at any resolution: the total volume over an hour is a physical quantity and cannot depend on how many columns the chart was drawn with. `sum` breaks this above tier 0 — a stored point is delivered to every bucket it spans and `sum` fetches the WINDOW'S OWN SUM for it (TIER_QUERY_FETCH_SUM), so each bucket is handed the whole window's total and adds it again. On a constant 7 over 1200s (true total 8400) tier 1 reads 8400 at 20 buckets, 25200 at 60, 126000 at 300 and 504000 at 1200 — exactly the zoom factor. options=natural-points does not help. Same fault as the condition groupings had, in an aggregation nobody had written a case for",
	},
	"L10/single-point-buckets-answer": {
		Proves: "a bucket holding ONE collected sample still answers: one bucket per sample interval is the most natural resolution there is and a chart drawn at it must not come back blank. `incremental-sum` returns EMPTY for EVERY bucket there — the carry-over it is built around (flush does `first = last`) is destroyed at startup: the first bucket sets `first` and leaves `last` NAN, flush emits EMPTY and then copies that NAN back over the seed it just captured, so the chain never starts and every later bucket repeats it. One point per bucket is otherwise fully supported by add(), which is what makes this a latch bug rather than a limitation. NOTE: L3/sparse-buckets records the same behaviour as \"pinned current contract\" — the two entries disagree deliberately, and this one states why it is a defect",
	},
	"L10/counts-do-not-inflate-with-zoom": {
		Proves: "an event does not happen twice because the chart was zoomed in: above tier 0 the same stored point is handed to every bucket it spans, and a grouping that counts occurrences may see its total collapse as a rollup loses ordering but may never see it grow. Swept over number-of-flaps and number-of-times at 1/2/3/5 buckets per stored window",
	},
	"L10/totals-exact-over-gaps-and-off-grid": {
		Proves: "a total is exact over a span with HOLES in it and over a span that does not start on the tier grid - the two shapes L10/totals-are-exact-across-zoom never sees, and the two that hid real defects. An unaligned span puts the first stored point ACROSS the first bucket's start, where an apportionment clamped only against what it already accounted for hands the bucket everything from the point's own beginning; and a fixture with holes is the only way to reach a point the engine does NOT trim, because query_interpolate_point() trims a wide point to the bucket end only when the point before it is adjacent and numeric, which after a gap it is not. The same fault as L10/totals-are-exact-across-zoom reaches both shapes: `sum` above tier 0 counting a stored point once per bucket it spans",
	},
	"L10/anomaly-bit-answers-about-rates": {
		Proves: "options=anomaly-bit answers about anomaly RATES, never about the metric: the option replaces the delivered value with the stored window's anomaly rate while min/max/sum/count go on describing the metric, so an aggregation that reaches past the delivered value into those statistics answers in unrelated units. Pinned with a dimension holding 1000000 that is never anomalous - every grouping of its anomaly rate is zero, so any value carrying that magnitude came from the wrong domain",
	},
	"L10/time-shares-stable-across-zoom": {
		Proves: "a share of TIME over a fixed span is the same at every zoom: the share of a window that satisfied a condition is a property of the data and the window, not of how many buckets the window was drawn with. This is the rule percentage-of-time(<previous) broke — 5% of the span at one bucket per stored window against 77% at five. percentage-of-samples is deliberately excluded: it answers about the samples it was handed and a re-delivery is another sample to it",
	},
	"L10/aliases-resolve-to-the-same-grouping": {
		Proves: "an alias is the same grouping, not a similar one: avg/mean, incremental-sum, trimmed-mean, trimmed-median, percentile, rsd/coefficient-of-variation, ema/ewma and countif each answer bit-identically to their canonical name. Nothing else checks the registry's name->implementation mapping, so a copy-paste there would route a name to the wrong aggregation silently",
	},
	"L10/queries-are-deterministic": {
		Proves: "asking twice answers twice the same, for every grouping: the aggregations keep state for the length of a query, and any of it that outlives the query — a static, an arena not cleared, a field create() does not initialise — makes every answer a function of what was asked before it",
	},
	"L10/buckets-are-ordered-and-unique": {
		Proves: "buckets come back in order, once each, for every grouping at every tier and every resolution: a repeated or out-of-order timestamp means the grid walk lost its place and every value after it is attributed to the wrong moment",
	},
	// Layer 11 — the slicing matrix: the knobs that must not change a total.
	"L11/slicing-is-additive": {
		Proves: "cut a window in two and the halves total the whole, across a matrix covering every PAIR of: data shape (dense/gaps/sparse), collection interval (1s/10s), tier (0/1), chart points per stored record (1/3/10), window start offset from the storage grid (0/17/30s) and option flag (none/natural-points/absolute). The split introduces exactly one new edge, so a record straddling it is the one under test - counted in both halves the parts exceed the whole, dropped from both they fall short. Needs no oracle: the engine is compared with itself, three questions at a time. Every slicing defect this corpus has found was triggered by one or two of these knobs together, never three, which is why covering all pairs is the target. NOTE what this does NOT catch: additivity is scale-invariant, so a total inflated by a CONSTANT factor satisfies it (both halves and the whole inflate together). That is L11/totals-match-what-was-pushed's job - the two are complementary and neither is sufficient alone",
	},
	"L11/randomised-slicing": {
		Proves: "the same two slicing properties, on configurations GENERATED rather than listed: window bounds, resolution, tier, grid offset, data shape and option flag drawn at random, then shrunk on failure to the smallest case that still fails. The pairwise matrix covers the combinations someone thought to enumerate, and every slicing defect this corpus has found escaped exactly that way - the aggregation sweep held the alignment still, the alignment tests held the data shape still, the shape tests never turned an option on, and each time the bug sat in the axis that had been pinned. Seeded, so a failure replays exactly with QUERY_CORPUS_SEED",
	},
	"L11/totals-match-what-was-pushed": {
		Proves: "a total equals what the fixture actually pushed into the window - conservation against arithmetic rather than against another query. Two preconditions, both principled and both enforced: the chart points must be at least as wide as the COLLECTION INTERVAL (below that the engine is no longer dividing stored records but manufacturing values between them, which layer 9 owns), and the tier being asked must actually cover the window (a rollup still catching up answers with less than was pushed, which looks like a defect and is not). The precondition is on the collection interval and NOT on the stored record, because at tier 1 a 1-second chart point is still one point per collected sample - and that regime is exactly where sum-over-time multiplies a total by the zoom",
	},
	"L9/virtual-points": {
		Proves: "the virtual-points view oracle is engine-EXACT (fixture/viewpoints.go, the rrd2rrdr_query_execute port): grid boundaries cutting sample intervals serve a linearly interpolated boundary point per line; only freshly fetched points ending inside the line are added whole (a pending straddler shifts to the interpolation anchor WITHOUT re-adding, keeping its original bounds); off-grid charts re-time onto the absolute grid with exact interpolated slots; upsampling serves interpolated sub-ue slots, with the query's FIRST straddler raw — tier0 has no backward plan expansion, so it has no anchor (the CASE-017 asymmetry)",
	},
	"L9/window-normalization": {
		Proves: "a negative `after` is relative to `before` (identical to the absolute equivalent); (0,0) resolves to the ~600s grid-aligned default window ending NOW — NOT the full retention (the reason backdated fixtures settle via explicit windows); time_resampling (v1 gtime) forces the bucket size up",
	},
	"L9/natural-points": {
		Proves: "options=natural-points keeps the db COUNT and spacing with raw sample values, but timestamps still snap onto the absolute ue grid; slot values around region boundaries are the raw sample OR its phase-interpolation toward the next (two-candidate pin; the full natural-mode slot selection is a recorded deferral — the DEFAULT virtual-points mode is oracle-exact)",
	},
	"L9/live-edge": {
		Proves: "queries past NOW on a live chart: the grid derives from the requested `before` (no clamp) — at most ONE bucket-end past now is served, holding the collected tail, or the incomplete tail is trimmed, depending on where now falls against the grid (phase-dependent; envelope-pinned: the series ends within a bucket of now, nothing further into the future)",
	},
	"L9/v2-v3-parity": {
		Proves: "/api/v2/data and /api/v3/data answer identically for identical params (shared api_v23_data_internal) — only the api version field differs",
	},
	"API/selectors": {
		Proves: "the selector surface with VALUE-exact oracles: nodes/instances/dimensions filters and their scope_ counterparts, '!' negation patterns, label key:value patterns (labels + scope_labels); match-ids/match-names dimension modes with id!=name dims (default matches BOTH; each mode excludes the other's namespace; a no-match response is the bare [time] labels row)",
	},
	"API/options-long-tail": {
		Proves: "ms renders epoch-milliseconds; rfc3339 loses to seconds on the v1 formatters (pinned no-op); objectrows emits named row objects; jsonwrap all-dimensions ADDS full_dimension_list/full_chart_list/full_chart_labels while the queried selection stays; tqx wraps datatable in the gviz envelope echoing reqId; tsv-excel == tsv; csv label-quotes quotes the header; v2 minimal-stats drops totals, long-json-keys switches to descriptive keys, group-by-labels flattens the label values into the view",
	},
	"API/row-reductions": {
		Proves: "the single-series formats reduce each row by the requested option: min2max = max-min (0 on single-value rows), min, max, average — exact cells over the formatter fixture (default sum pinned by L7/formatters)",
	},
	"API/fallbacks-and-limits": {
		Proves: "unknown v1 format silently serves json; unknown weights method silently runs ks2; cardinality_limit at 2 folds five dimensions into the remaining bucket and at >= the dimension count folds nothing",
	},
	"W/value": {
		Proves: "weights method=value: per-metric weight = the window average over NATURAL points with the after-INCLUSIVE window (121 points for a 120s span — rulings batch); MULTINODE rollup rows carry the mean of their dimensions; the per-dimension timeframe stats (min/avg/max/sum/count/anomaly_count) are exact; method=value NEVER rank-normalizes",
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
		Proves: "options=percentage (v2/v3 FORCE absolute with it — and with any non-dimension group-by: shares computed over |values|), options=absolute (|v| at fetch), nonzero (drops all-zero dims; self-neutralizes when everything is zero), null2zero (gap cells become 0), cardinality_limit (top N-1 by |view sum| + 'remaining X dimensions' fold of per-row sums)",
	},
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

	mc, ok := manifest[name]
	if !ok {
		t.Fatalf("case %q missing from manifest", name)
	}

	if held {
		return
	}

	brokenMu.Lock()
	broken = append(broken, name)
	brokenMu.Unlock()

	t.Errorf("BROKEN %s: %s", name, mc.Proves)
}

var (
	brokenMu sync.Mutex
	broken   []string
)

// brokenSummary is printed once, after every test has run: the corpus
// answers "what does the query engine get wrong today" and this is that
// answer, in one place.
func brokenSummary() string {
	brokenMu.Lock()
	defer brokenMu.Unlock()

	if len(broken) == 0 {
		return fmt.Sprintf("query contract corpus: all %d contracts hold\n", len(manifest))
	}

	sort.Strings(broken)

	var b strings.Builder
	fmt.Fprintf(&b, "\nquery contract corpus: %d of %d contracts BROKEN\n",
		len(broken), len(manifest))
	for _, name := range broken {
		fmt.Fprintf(&b, "  BROKEN  %s\n", name)
	}
	fmt.Fprintf(&b, "\nEach one is a defect in the query engine, not a test to adjust.\n")
	return b.String()
}
