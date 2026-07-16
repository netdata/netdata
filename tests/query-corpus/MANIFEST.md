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
| CASE-015 (robustness) mid-dialogue disconnect + soak | teardown with queued replies to a dead child stays crash-free | green | n/a | |
| L1/palette | tier0 ingestion identity for the edge-data palette (gaps, resets, anomaly runs, negatives, zeros, ue=5) | green | n/a | |
| L1/single-point | single-point exact through a wide window; pins 1-point-window view expansion (layer-9 seed) | green | n/a | |
| L1/trailing-window | beyond-retention reads return nulls at the fixed epoch | green | n/a | |
| L1/precision | storage_number quantization contract (Go pack/unpack port) | green | n/a | |
| L1/gap-states | #23095 ruling pinned: live phantom / gone after restart / back next iteration | green | n/a | ruling #23095 |
| L2/tier1-palette | tier1 rollup identity for the palette: aligned windows, partial counts, stored-empty windows, fractional anomaly rates, RESET annotation lost at tier1+, float32 write-rounding | green | n/a | |
| L2/whole-chart-absence | never-stored tier windows read like stored-empty ones (null + EMPTY), flanking partial counts correct | green | n/a | |
| L2/sn-vs-original | tiers aggregate ORIGINAL collected doubles, not tier0-quantized values (2^24+1 discriminates) | green | n/a | |
| L2/update-every-5 | tier grid arithmetic scales with update_every (granularity ue×grouping) | green | n/a | |
| L2/tier2 | second-level rollup (3600s windows) over replicated history incl. gap run; tier1 cross-checked on identical data | green | n/a | |
| CASE-016/fresh-host-forgotten-on-restart | child connected <5s before a graceful restart is forgotten (pending host metadata never flushed at shutdown); its dbengine data orphaned | **red** | n/a | |
| L3/families | every time_group equals its Go oracle over the mixed palette at group 10 (incl. stddev/cv Welford, median value-range trim + R-7, percentile/trimmed-mean slot-window means, ses/des running state, incremental-sum carry, countif grammar) | green | n/a | |
| L3/sign-semantics | percentile/trimmed-mean top-walk on negative buckets; extremes champions by abs; all-negative + mixed fixtures | green | n/a | |
| L3/sparse-buckets | single-value buckets: stddev 0.0, pass-through families, incremental-sum all null (pinned contract) | green | n/a | |
| L3/identity-smoothing | ses/des identity window = requested points capped 15; identity incremental-sum all null (pinned contract) | green | n/a | |
| L4/family-tier-matrix | grouping families over forced tier1, 6 windows/bucket, fetch-aware oracles (avg-of-averages pinned); tier-count anomaly rates | green | n/a | |
| L4/auto-tier-selection | auto tier = coarsest with acceptable density (>= half the wanted points, wanted floored at 10); tier2 unreachable under 5h windows with full coverage; per_tier points exclusive; values match the serving tier's oracle | green | n/a | |
| L4/minmax-absolute-semantics | min = closest to zero, max = furthest from zero (fabs comparisons) — RULING PENDING | green | n/a | |
| L4/plan-switching | tier0 quota rotation (dedicated daemon) + straddling query served by tier1 head + tier0 tail (per-side oracles); head-only from tier1 alone | green | n/a | |
| CASE-017/tier-boundary-absorption | tier>0 first bucket keeps out the tier point ending exactly at `after` (was: absorbed pre-window data); tier0 control clean | green | n/a | #23127 |

## Corpus-wide pusher discipline

- CASE-015 forces the harness rule: pusher connections close only AFTER the
  settle barrier confirms retention. Deliberate immediate closes exist only
  inside the CASE-015 red cases.
- The historical spike-3 discipline (first point alone, wait, then burst)
  is NOT needed since #23096 — pinned by L0/live-burst.
