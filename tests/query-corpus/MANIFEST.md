# Query Corpus Manifest

Human mirror of `manifest.go` (the enforced table). Every case records what
it proves and its expected status on the agent under test. A `red` case pins
a known bug: the suite passes while the bug reproduces and fails loudly when
a fix lands, demanding the flip to `green` with the fixing PR.

| Case | Proves | Agent | Cloud | Fixed by |
|------|--------|-------|-------|----------|
| L0/live-burst | live BEGIN2/SET2 burst round-trips byte-exact without settle discipline | green | n/a | #23096 |
| L0/live-paced | legacy spike-3 pacing still works (control) | green | n/a | |
| L0/replication | replication dialogue round-trips byte-exact incl. gap + anomaly bits | green | n/a | |
| L0/two-children | same context from two children answers independently per host | green | n/a | |
| L0/labels | CLABEL chart labels reach the query path (group_by=label) | green | n/a | |
| L0/restart | fixtures survive daemon restart byte-identical (journal-v2 read path) | green | n/a | |
| CASE-015/live-disconnect-discard | receiver drains delivered live data before honoring HUP — immediate close loses nothing | green | n/a | #23118 |
| CASE-015/replication-disconnect-discard | same drain guarantee on the replication path | green | n/a | #23118 |
| CASE-015/robustness | teardown with queued replies to a dead child stays crash-free (mid-dialogue disconnect + 30-cycle soak) | green | n/a | |
| L1/palette | tier0 ingestion identity for the edge-data palette (gaps, resets, anomaly runs, negatives, zeros, ue=5) | green | n/a | |
| L1/single-point | single-point exact through a wide window; pins 1-point-window view expansion (layer-9 seed) | green | n/a | |
| L1/trailing-window | beyond-retention reads return nulls at the fixed epoch | green | n/a | |
| L1/precision | storage_number quantization contract (Go pack/unpack port) | green | n/a | |
| L1/gap-states | #23095 ruling pinned: live phantom / gone after restart / back next iteration | green | n/a | ruling #23095 |
| L1/incremental-rates | db stores PER-SECOND rates at any update_every (v1 child counters through parent rrdset_done: K*(mul/div)/UE; absolute control unscaled) | green | n/a | |
| L1/resets-overflows | v1 reset/overflow arithmetic: backward step → 0 + RESET; 32-bit wrap stores cap-relative delta (one less than true — quirk, ruling pending); pcent-over-diff pre-pass absorption; long-gap collection reset (no spike, no flag); blend mass conservation | green | n/a | |
| L1/off-grid-timestamps | off-grid pushes: storage keeps pushed times exactly; views re-grid to absolute ue multiples with boundary interpolation (envelope-pinned; exact oracle = layer 9) | green | n/a | |
| L2/tier1-palette | tier1 rollup identity for the palette: aligned windows, partial counts, stored-empty windows, fractional anomaly rates, RESET annotation lost at tier1+, float32 write-rounding | green | n/a | |
| L2/whole-chart-absence | never-stored tier windows read like stored-empty ones (null + EMPTY), flanking partial counts correct | green | n/a | |
| L2/sn-vs-original | tiers aggregate ORIGINAL collected doubles, not tier0-quantized values (2^24+1 discriminates) | green | n/a | |
| L2/update-every-5 | tier grid arithmetic scales with update_every (granularity ue×grouping) | green | n/a | |
| L2/tier2 | second-level rollup (3600s windows) over replicated history incl. gap run; tier1 cross-checked on identical data | green | n/a | |
| L2/update-every-sweep | ue {10,30,60,600,3600}: tier0 identity, tier1 windows on the scaled grid, time-group buckets in both grid modes (absolute-aligned + unaligned); v1 rate contract at ue=10 | green | n/a | |
| CASE-017/tier-boundary-absorption | tier>0 first bucket keeps out the tier point ending exactly at `after` (was: absorbed pre-window data); tier0 control clean | green | n/a | #23127 |
| CASE-016/fresh-host-forgotten-on-restart | child connected <5s before a graceful restart SURVIVES it — the metasync shutdown now flushes pending host metadata regardless of scan phase (was: forgotten, data orphaned) | green | n/a | #23120 |
| L3/families | every time_group equals its Go oracle over the mixed palette at group 10 (incl. stddev/cv Welford, median value-range trim + R-7, percentile/trimmed-mean slot-window means, ses/des running state, incremental-sum carry, countif grammar) | green | n/a | |
| L3/sign-semantics | percentile/trimmed-mean top-walk on negative buckets; extremes champions by abs; all-negative + mixed fixtures | green | n/a | |
| L3/sparse-buckets | single-value buckets: stddev 0.0, pass-through families, incremental-sum all null (pinned contract) | green | n/a | |
| L3/identity-smoothing | ses/des identity window = requested points capped 15; identity incremental-sum all null (pinned contract) | green | n/a | |
| L3/registry-completeness | full time-grouping registry: all 47 names (variants + aliases, latest since #23257), full countif grammar, option clamps, unknown-name fallback to average; pinned quirk: bare-number countif loses its first digit (ruling pending) | green | n/a | |
| L3/anomaly-bit-option | options=anomaly-bit: values become anomaly rates pre-grouping (0/100 tier0, fractional tier>0); buckets aggregate the rates; group-by consumes them as values | green | n/a | |
| L3/sum-over-time-volume | time_group=sum: rate-stored metrics integrate (value x duration = VOLUME at any ue); non-rate metrics sum plainly | green | n/a | |
| CASE-020/sum-over-time-units | volume integration keeps the rate units — "units/s" should become "units" when sum integrates a rate | **red** | n/a | |
| L4/family-tier-matrix | grouping families over forced tier1, 6 windows/bucket, fetch-aware oracles (avg-of-averages pinned); tier-count anomaly rates | green | n/a | |
| L4/auto-tier-selection | auto tier = coarsest with acceptable density (>= half the wanted points, wanted floored at 10); tier2 unreachable under 5h windows with full coverage; per_tier points exclusive; values match the serving tier's oracle | green | n/a | |
| L4/minmax-absolute-semantics | min = closest to zero, max = furthest from zero (fabs comparisons) — RULING PENDING | green | n/a | |
| L4/plan-switching | tier0 quota rotation (dedicated daemon) + straddling query served by tier1 head + tier0 tail (per-side oracles); head-only from tier1 alone | green | n/a | |
| L5/group-by-matrix | level-1 (first-pass) group-by, BOTH contracts: 7 keys x 5 aggregations vs member-enumeration oracle (non-raw converts; raw defers with counts on the wire; PARTIAL on gap rows; naming) | green | n/a | |
| L5/percentage | aggregation=percentage: non-raw n*100/(n+h); raw defers (sums + hidden on wire); dimension key degenerate (flat 100); percentage-of-instance converts even raw (no hidden — per-instance groups never span agents) | green | n/a | |
| L5/statistics | per-group view sts: non-average = row means + row extremes; AVERAGE = weighted pair; raw = untouched (sum, count) — D-B SETTLED | green | n/a | #23097 |
| L5/anomaly-statistics | jsonwrap-v2 per-dim anomaly arrays: view sts arp = mean row ARP, db sts arp = fetched-points rate; stored NAN gaps excluded from both | green | n/a | |
| L5/multi-key-group-by | multi-key group_by tuples: fixed id order (dimension, instance, label, node, context, units) regardless of request order; bare instance id with node; selected / percentage-of-instance collapse rules; avg alias + unknown-aggregation fallback to average | green | n/a | |
| L6/two-pass-matrix | two-pass mechanics: 10 key-chains (union partitioning) x 8 agg-chains x non-raw/raw; ARP accumulates raw, divides once by final group count (inflated every chain — rollup SOW evidence); raw count = pass-1 groups; group_by_label[1]; PARTIAL propagates | green | n/a | |
| L6/two-pass-percentage | percentage at pass 2: shadow hidden buckets at pass 1 fold into the denominator at the pct pass; incomplete shadow bucket taints PARTIAL via hgbc; raw defers (visible sum + hidden on wire, count = visible groups) | green | n/a | |
| CASE-018/multipass-average | AVERAGE at pass 1 feeds pass 2 the group SUMS — final value inflated ~members-per-group (item 3 family; fix owned by the rollup-correctness SOW) | **red** | n/a | |
| L7/formatters | classic v1 formats byte/structure-pinned: csv/tsv CRLF endings, literal "null" gap cells, newest-first default + options=flip, unquoted header cells (current contract), csvjsonarray valid JSON + numeric timestamps (#23115/#23117), ssv/ssvcomma/markdown/html/array/json/datatable/jsonp | green | n/a | |
| CASE-022/time-group-latest | time_group=latest end to end: per-bucket last value, empty buckets empty, sign preserved/erased per options=absolute; points=1 + before at/near now anchors at the newest sample, served from the collector cache (0 db reads, raw un-quantized value, ar=0); storage path keeps quantization + generic ar | green | n/a | #23257 |
| CASE-023/fleet-time-groupings | the four fleet time-aggregations and one shared grammar: percentage-of-samples (canonical, `countif` alias) / percentage-of-time / number-of-flaps / number-of-times, each echoing its canonical name and transforming units (%/%/flaps/events); gap tokens (nan\|null\|gap\|empty) pull gap slots in for `percentage-of-samples`/`number-of-flaps`/`number-of-times` while an expression without one keeps them invisible there (`percentage-of-time` always counts uncollected time); `previous`\|`last` compare against the previous COLLECTED sample (counter reset = reboot; the first sample never matches); flaps count observed false→true transitions only, carried across buckets; a gap contributes its SLOT width, not the zero span of QUERY_POINT_EMPTY | **red** | n/a | |
| CASE-023/tier-estimation | above tier 0 a stored point is min/max/avg over many samples, not a sample: `percentage-of-time` estimates each window's share with the two-point mass model (weight(max) = (avg−min)/(max−min)), EXACT for a 0/1 availability signal because there the average IS the fraction of time at 1 — a mixed window's up% and down% sum to 100 instead of both answering "never", which is what evaluating the condition on the stored average does; a mixed window counts one flap and at most one occurrence (the rollup keeps no ordering); `percentage-of-samples` keeps its historical tier behaviour | **red** | n/a | |
| CASE-023/redelivery | the same stored point handed to several buckets means different things per grouping: `percentage-of-samples` treats a delivery AS a sample and must answer in EVERY bucket (skipping repeats leaves EMPTY holes), while `number-of-times`/`number-of-flaps` count a stored window at most once however many buckets it spans | **red** | n/a | |
| CASE-023/reset-counted-once | one counter reset counts once above tier 0 at any resolution — carrying the PRE-reset peak forward, or falling back to comparing window averages, makes the window after the reset look like it dropped too (measured 2 reboots for 1) | **red** | n/a | |
| CASE-023/nonzero-follows-answer | the condition groupings are judged by their ANSWER under `options=nonzero`: a dimension whose condition never holds is dropped even though every source sample is non-zero, while one with a non-zero answer stays (needs two dimensions — an all-zero result self-neutralizes the option) | **red** | n/a | |
| L4/three-tier-join | three tiers with different retention depths joined in one query: all at the 25MiB floor with 1s/5s/10s granularity so each fills its own quota — tier0 newest, tier1 outlives it, tier2 outlives both. The full retained span reads clean at five resolutions (845s→2.9s buckets): no empty bucket inside the span, monotone time, values in range, and all three tiers contribute across the zoom levels. Pins outward grid alignment (leading buckets precede retention) and upsampling below the serving tier as expected, not seam defects | green | n/a | |
| CASE-024/zoom-into-slow-metrics | a metric collected every 60s / 600s / 3600s still answers when the dashboard zooms BELOW its collection interval — a 60-point request over a window shorter than one sample interval, inside the collected span, returns rows carrying the value | green | n/a | |
| CASE-023/percentage-of-time-denominator | the denominator of `percentage-of-time` is the SELECTED duration, not the collected part of it: one collected second reading 1 followed by 99 uncollected seconds is 1% at `==1` and 99% at `==gap` — answering 100% turns a node that went silent into a healthy one. `percentage-of-samples` keeps the opposite contract and reads 100%, because it answers about the samples it was handed | **red** | n/a | |
| CASE-023/trailing-gaps | a condition that names a gap keeps accounting to the END of the requested window: the engine stops walking a few buckets after a dimension's storage is exhausted and the caller fills the rest with EMPTY — the same answer for every other aggregation, but for `==gap` it silently truncates an outage. A dimension that stops being collected while its chart keeps going must read 100% gap for every remaining bucket | **red** | n/a | |
| CASE-023/gap-weight | a gap counts in stored SLOTS, not seconds: on a 10s metric a 100s hole is ten missing samples, not a hundred — measuring against the query grid (1s) let one missing slot outweigh ten collected ones (measured 90.9% where the fixture is 50%) | **red** | n/a | |
| CASE-023/tier-wide-point | when the view grid is FINER than the stored data — a dashboard zoomed into a window only tier 1 still covers — a stored point is re-delivered to every bucket it spans, carrying its original start and an interpolated value; the share of time answers the SAME estimate in each of those buckets, and one stored window yields at most ONE occurrence and ONE flap, because a re-delivery is the same window seen again, not a second event (counting the repeats inflates an SLO by exactly the zoom factor) | **red** | n/a | |
| CASE-023/tier-anomaly-bit | with `options=anomaly-bit` above tier 0 the value is the stored window's anomaly RATE while min/max still describe the metric, so the condition is answered on the rate itself — a window either satisfied it or it did not (100/0), never a fraction estimated across two unrelated domains; `>=N` and `<N` partition every window | **red** | n/a | |
| CASE-023/countif-bare-number | the shared parser fixes the bare-number digit swallow (countif.h:78 advances past the operator switch even when no operator matched, so options `5` targets 0) — the API is aligned to health, which has always parsed `countif(5)` as `=5` | **red** | n/a | |
| CASE-019/v1-json-name-escaping | v1 json/jsonp/csvjsonarray/datatable escape dimension names (was: raw — a double-quote in a name or label value via group_by=label broke the JSON); objectrows keys escaped like the header; google flavor escapes the apostrophe of its single-quoted labels, double quote stays raw | green | n/a | #23216 |
| L9/virtual-points | the view oracle is engine-exact: boundary interpolation with straddler-as-anchor consumption, off-grid re-timing, upsampled sub-ue slots; the query's first straddler serves raw (tier0 has no backward expansion) | green | n/a | |
| L9/window-normalization | negative after is relative to before; (0,0) = the ~600s default window ending now (NOT full retention); time_resampling/gtime forces bucket size up | green | n/a | |
| L9/natural-points | natural-points = db count/spacing + raw values, timestamps still grid-snapped; boundary slots raw-or-phase-interp (two-candidate pin; full natural-mode oracle deferred) | green | n/a | |
| L9/live-edge | past-now queries: grid from requested before (no clamp); at most one future-stamped tail bucket or a trimmed tail (phase-dependent, envelope-pinned) | green | n/a | |
| L9/v2-v3-parity | /api/v2/data == /api/v3/data for identical params (shared implementation; only the api field differs) | green | n/a | |
| API/selectors | nodes/instances/dimensions/labels filters + scope_ variants, ! negation, label key:value; match-ids/match-names with id!=name dims (default matches both); value-exact via the member oracle | green | n/a | |
| API/options-long-tail | ms, rfc3339 (v1 no-op with seconds, pinned), objectrows, all-dimensions (full_* blocks), gviz tqx envelope, tsv-excel alias, label-quotes, v2 minimal-stats/long-json-keys/group-by-labels | green | n/a | |
| API/row-reductions | ssv min2max/min/max/average exact cells (default sum pinned by L7) | green | n/a | |
| API/fallbacks-and-limits | unknown v1 format → json; unknown weights method → ks2; cardinality_limit 2 folds five dims, >= dim count folds nothing | green | n/a | |
| W/value | weights value = after-INCLUSIVE window average (121 pts/120s — ruling pending); rollups = mean of dims; timeframe stats exact; value never rank-normalizes | green | n/a | |
| W/anomaly-rate-per-metric | per-metric anomaly-rate = true anomaly rates (anomaly bit applied); NONZERO default drops zero weights, explicit options keep them | green | n/a | |
| W/anomaly-rate-multidim | method=anomaly-rate implies the anomaly bit on every path — bare method == explicit option, true rates everywhere (was: multi-dim ranked by value averages) | green | n/a | #23212 |
| W/volume | volume = (hl-bl)/bl x fraction-of-time above/below baseline; equal-averages metrics skipped | green | n/a | |
| W/ks2 | ks2 exact endpoints (identical diffs → 0, one-sided diffs → 1); spread_results_evenly rank normalization pinned via Go port; intermediate KS values deferred (KSfbar port) | green | n/a | |
| L8/post-processing | percentage (implies absolute on v2/v3 — as does any non-dimension group-by), absolute, nonzero (+ self-neutralizing all-zero), null2zero, cardinality_limit fold | green | n/a | |

## Corpus-wide pusher discipline

- CASE-015 established the harness rule: pusher connections close only AFTER
  the settle barrier confirms retention. Deliberate immediate closes exist
  only inside the CASE-015 cases — green since #23118, where they prove the
  drain guarantee (an immediate close loses nothing).
- The historical spike-3 discipline (first point alone, wait, then burst)
  is NOT needed since #23096 — pinned by L0/live-burst.
