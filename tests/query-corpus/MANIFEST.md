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
| L3/registry-completeness | full time-grouping registry: all 46 names (variants + aliases), full countif grammar, option clamps, unknown-name fallback to average; pinned quirk: bare-number countif loses its first digit (ruling pending) | green | n/a | |
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
| CASE-019/v1-json-name-escaping | v1 json/jsonp/csvjsonarray/datatable emit dimension names unescaped — a double-quote in a name (or label value via group_by=label) breaks the JSON; json2 escapes properly | **red** | n/a | |
| L9/virtual-points | the view oracle is engine-exact: boundary interpolation with straddler-as-anchor consumption, off-grid re-timing, upsampled sub-ue slots; the query's first straddler serves raw (tier0 has no backward expansion) | green | n/a | |
| L9/window-normalization | negative after is relative to before; (0,0) = the ~600s default window ending now (NOT full retention); time_resampling/gtime forces bucket size up | green | n/a | |
| L9/natural-points | natural-points = db count/spacing + raw values, timestamps still grid-snapped; boundary slots raw-or-phase-interp (two-candidate pin; full natural-mode oracle deferred) | green | n/a | |
| L9/live-edge | past-now queries: grid from requested before (no clamp); at most one future-stamped tail bucket or a trimmed tail (phase-dependent, envelope-pinned) | green | n/a | |
| L9/v2-v3-parity | /api/v2/data == /api/v3/data for identical params (shared implementation; only the api field differs) | green | n/a | |
| API/selectors | nodes/instances/dimensions/labels filters + scope_ variants, ! negation, label key:value; match-ids/match-names with id!=name dims (default matches both); value-exact via the member oracle | green | n/a | |
| API/fallbacks-and-limits | unknown v1 format → json; unknown weights method → ks2; cardinality_limit 2 folds five dims, >= dim count folds nothing | green | n/a | |
| W/value | weights value = after-INCLUSIVE window average (121 pts/120s — ruling pending); rollups = mean of dims; timeframe stats exact; value never rank-normalizes | green | n/a | |
| W/anomaly-rate-per-metric | per-metric anomaly-rate = true anomaly rates (anomaly bit applied); NONZERO default drops zero weights, explicit options keep them | green | n/a | |
| CASE-021/multidim-anomaly-rate | the multi-dim weights path (any context selector; v1 default method) never applies the anomaly bit — "anomaly-rate" ranks by plain value averages | **red** | n/a | |
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
