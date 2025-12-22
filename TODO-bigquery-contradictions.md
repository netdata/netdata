# TODO — Resolve executive2.ai contradictions

## TL;DR
- Remaining blocker: customers_growth_pct still fails because pct_7 is populated instead of NULL when the lag is outside the window. Growth% template hardened; needs re-run.
- Critical user ask: reliably list top/worst customers by ARR (or nodes) between T0–T1 using correct diff logic (no snapshot hacks). Must be 100% reliable for “customers at risk” workflows.
- New work: document/implement robust ARR and node change per-space diff strategy; add harness coverage for those questions and the “Growth metrics” section panels.

## Analysis (facts from executive2.ai)
1) On-prem baseline cutoff mismatch **(fixed in executive2.ai)**
   - Canonical rule: baseline uses expiry_date > 2025-10-01 AND start_date <= 2025-10-01 (applied to dates <= 2025-10-01).
   - Templates updated; previous 2025-10-02 mismatch removed.
   - Risk addressed.

2) Per-customer ARR guidance conflict **(clarify everywhere)**
   - Decision: per-customer ARR = bq_arr_discount from spaces_asat_* only (Grafana dashboards use this; no space_override add-on).
   - Action: ensure routing/FAQ/templates reflect this single rule; remove any residual override wording.

3) MAX vs SUM wording ambiguity on metrics_daily **(clarify wording)**
   - Rule: use MAX(IF(...)) to pivot within a single run_date row (one value per metric per day). Use SUM/AVG when aggregating across dates/entities.
   - Action: align pitfall wording to match.

## Decisions required from Costa (resolved)
1. Cutoff for manual360 baseline: **Use 2025-10-01** (expiry_date > 2025-10-01 AND start_date <= 2025-10-01 applied to dates <= 2025-10-01).
2. Per-customer ARR source: **Use bq_arr_discount only** (Grafana dashboard SQL reads bq_arr_discount from spaces_asat_*; no space_override references found in `dashboard/`).
3. MAX vs SUM guidance: **Clarify** that MAX(IF(...)) is correct for pivoting per run_date in metrics_daily; SUM/AVG are for aggregating across dates/dimensions.

## Proposed plan (updated)
1) Fix growth% case: re-run ONLY_CASE=customers_growth_pct after latest template hardening; if still wrong, add explicit instruction to skip helper queries and keep pct_* NULL when lag < from_date.
2) Define and document canonical per-space diff strategy for ARR and nodes (timeseries-based, not snapshot pair), covering won/lost lists with sorting and caps (top 10) and handling manual on-prem baseline correctly.
3) Add harness cases for ARR customer diff (won/lost by ARR) and nodes won/lost (business + homelab), modeled on Grafana “Daily ARR changes by space” and node breakdown panels; ensure schema compliance.
4) Sweep executive2.ai to ensure per-customer ARR = bq_arr_discount only (no overrides), and clarify MAX vs SUM guidance (MAX for pivot per day; SUM/AVG for aggregation across dates/entities).
5) Expand parity: port top 5–10 panels per Grafana section (excluding “Other Space metrics” and “old retired tables”), add harness coverage as we go. Keep repetition of core rules for retention; no Grafana mentions in prompt.
6) Re-run full `./neda/bigquery-test.sh` (default window) once new diff logic and growth% fix land; regenerate refs only if SQL truly changes.

## New user requirements (Dec 22)
- Agent must answer reliably: “ARR change between T0 and T1; top 10 customers won/lost by ARR delta” using robust timeseries diff (not brittle snapshot comparison).
- Similar requirement for nodes: “Top 10 paid subscribers adding/removing nodes between T0 and T1.”
- Use `--override models=nova/gpt-oss-20b` for reasoning inspection when debugging; default model can stay unless we decide to switch in frontmatter.
- Do **not** instruct the model away from timeseries; avoid snapshot-only comparisons that miss wins/losses.

## Testing
- Immediate: `OUT_DIR=tmp/bigquery-tests TIMEOUT=1800 ONLY_CASE=customers_growth_pct ./neda/bigquery-test.sh` after template fix.
- After diff-strategy implementation: add new harness cases for ARR won/lost and nodes won/lost; run full suite.

## New scope (Dec 22)
- Expand parity to the important Grafana sections (all except “Other Space metrics” and “old retired tables”) by porting the first 5–10 panels of each section, then iterating deeper until size becomes an issue.
- Add harness cases for each newly targeted panel so parity stays test-backed.
- Keep output format/schema compliance strict; favor reusing existing templates (stat/timeseries/growth/snapshot) to limit prompt growth.

## Testing
- Full bigquery-test.sh with default window (last 7 complete days) after changes.
- Focus cases: realized_arr*, realized_arr_percent*, customer_diff, total_arr_plus_unrealized_arr.
