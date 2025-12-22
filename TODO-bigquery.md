# TODO – Refactor `neda/bigquery.ai`

## TL;DR
- File is ~7.9k lines (~100k tokens) of auto-generated Grafana SQL; huge context and high confusion.
- BigQuery agent must mirror Grafana charts exactly; bigquery.ai now includes the former executive2.ai prompt for parity.
- Need a slimmer, guardrailed prompt that still preserves Grafana-fidelity (canonical formulas/templates) instead of raw dumps.
- **Migration request (Costa):** Replace `bigquery.ai` with `executive2.ai` for production testing, remove executive.ai references, and rename harness/scripts/TODO from *executive* → *bigquery*.
- **Latest (2025-12-22):** Expand user-intent map for signups/customers/spaces/churn, then re-run the full harness with extreme timeouts and debug remaining failures.

## Analysis (current state)
- Frontmatter: single tool `bigquery`, `maxTurns: 50`, `reasoning: high`, `maxOutputTokens: 16384`; no expected output/schema; uses `${FORMAT}` placeholder.
- Body: brief mission + tool list + tone include, then **178 Grafana panel sections** with full SQL. Many queries reuse Grafana macros (`${__from:date}`, `${__timeGroup}`, `${__interval}`) that will fail if sent directly to BigQuery tool.
- Duplication & noise: multiple variants of ARR/ARR %, nodes, subscriptions, trials; panels repeated with slight tweaks. No prioritization or explanation of when to use which query.
- Data coverage is broad but undocumented: tables span `metrics.metrics_daily`, `data360.space360_daily`, `watch_towers.manual_360_bq`, `arr_forecast`, AWS/Stripe metrics, space churn tables, etc. No glossary of column meanings.
- Workflow gaps: no steps to confirm scope, pick correct date range, or validate table schemas before running heavy queries. Agents have no guardrails against full‑scan or wrong dataset.
- Result confusion: output format not enforced; no requirement to show executed SQL or sanity checks; encourages over-long context and hallucinated queries.
- Fidelity constraint: Costa wants Grafana parity—asking for “Realized ARR” should return exactly the Grafana number. Replacing the file with improvised logic risks divergence.
- bigquery.ai already provides schema-driven querying and templates; executive.ai exists to guarantee chart parity, so we need a strategy that keeps parity while shrinking context.
- Macro usage: 285 Grafana macros remain (`${__to:date}` 165×, `${__from:date}` 120×) that will fail against `execute_sql` unless translated.
- Table concentration: of all backticked tables, top counts are `metrics.metrics_daily` (140 refs), `watch_towers.manual_360_bq` (33), `watch_towers.spaces_latest` (30), `data360.space360_daily` (25); a handful of others appear ≤9 times. Patterns are few; queries are many.
- Metric-name concentration in `metrics_daily` (top examples): arr_business_discount (23), arr_homelab_discount (21), total_business_subscriptions (20), arr_business (19), paid_nodes_business_annual (16), paid_nodes_business_monthly (15), mrr_business_discount (12), trials_total (6), ai_credits_space_revenue (6), churn_subs (6). Indicates a small canonical KPI set repeated with date filters/groupings.
- Snapshot reference: latest Grafana export at `/opt/baddisk/Downloads/Netdata Executive Dashboard-1765988425701.json` (186 panels). Panel order in file is priority; top panels are most important for parity.
- Top-panel focus: first ~30 panels include the key ARR/Trials/Subscriptions/Nodes KPIs; each currently has a unique SQL blob but shares the same tables/metrics and differs mainly by filters/aggregations.
- Testing: BigQuery toolbox is installed/configured; we can execute queries directly to validate parity vs Grafana.
- Related prompt: `neda/bigquery.ai` already contains (1) a fairly complete data-flow overview, (2) Watch Towers field semantics (largely overlapping with `analytics-bi/airflow/dags/watch_towers/README.md`), and (3) KPI definitions + template patterns. This supports a “data dictionary + KPI catalog + examples” approach without re-embedding full Grafana SQL dumps.
- Consistency risk spotted: `neda/bigquery.ai` defines “Realized ARR (discounted)” using `business_arr_discount + homelab_arr_discount + ai_credits_space_revenue`, but later lists canonical `metrics_daily` keys as `arr_business_discount` / `arr_homelab_discount`. This kind of mismatch is a likely cause of wrong queries and must be normalized.

## Decisions needed from Costa
1. **Fidelity vs consolidation**
   - 1A: Keep executive.ai but rewrite as a **canonical “Grafana parity” spec**: for each chart keep the exact formula/query template (parameterized dates, no Grafana macros) and drop bulk SQL dumps.
   - 1B: Eliminate executive.ai; fold parity definitions into bigquery.ai, trusting its templates to stay aligned with Grafana.
   - 1C: Keep full dump as-is to guarantee parity but accept size/confusion.
   - Recommendation: **1A** (preserves parity with far less token load).

2. **Content slimming approach (if 1A)**
   - 2A: Curated catalog of canonical panels (e.g., 15–25 KPIs) with short formulas + table/column lists; move long SQL to separate reference file.
   - 2B: Keep catalog + link to external “sql-library.md” the agent can fetch when needed.
   - 2C: Keep everything inline (status quo, largest).
   - Recommendation: **2A** (can add 2B for traceability if desired).

3. **Execution guardrails**
   - 3A: Enforce 3-step workflow: clarify scope → inspect tables (`list_table_ids`/`get_table_info`) → propose & run a bounded query with date filters + `LIMIT` sanity; always show SQL + row counts.
   - 3B: Softer guidance only.
   - 3C: No workflow enforcement.
   - Recommendation: **3A** to reduce wrong queries/cost.

4. **Scope & specialization**
   - 4A: Keep one executive agent with themed blocks (ARR, subscriptions, nodes, trials) using parity templates.
   - 4B: Split into sub‑agents per theme and expose as tools to an orchestrator; each sub‑agent holds its parity templates.
   - 4C: Keep monolith unchanged.
   - Recommendation: **4A** now; 4B later if we want deeper isolation.

5. **Output contract**
   - 5A: Force `output.format=json` with schema `{summary, insights[], metrics[], sql_used, caveats, parity_check}` to keep responses consistent and auditable.
   - 5B: Keep freeform markdown.
   - 5C: Offer both via user-specified `FORMAT` with a default.
   - Recommendation: **5A** for consistency and downstream consumption.

6. **Prompt cleanup scope (new)**
   - 6A: Keep the current multi-KPI catalog (realized_arr + snapshot KPIs) but tighten wording; no structural deletions yet.
   - 6B: Prune the prompt to only the finalized KPI(s) (right now: realized_arr) and move the rest to a separate scratch file until each is templated/tested; re‑add once validated.
   - 6C: Split into two files: `executive2.ai` (production, only validated KPI templates) and `executive2-scratch.ai` (work-in-progress templates) to keep the main prompt lean during buildout.
   - Recommendation: **6C** (keeps production prompt small and clean while preserving WIP material without deleting it).
   - Status: Implemented. `neda/executive2.ai` now contains only frontmatter, guardrails, and realized_arr templates (including deltas). All other KPIs are preserved in `neda/executive2-scratch.ai`.

7. **Harness stability with gpt-oss-20b tool hallucinations (new)**
   - 7A: Set `temperature: 0` in `neda/executive2.ai` frontmatter.
     - Pros: Less tool-name hallucination; more deterministic JSON/schema compliance.
     - Cons: Lower flexibility for ambiguous asks; might reduce robustness on edge cases.
   - 7B: Keep prompt as-is; override temperature in harness only (e.g., `--override temperature=0`) so production remains unchanged.
     - Pros: Fast to test; doesn’t change prompt behavior for non-harness use.
     - Cons: Harness results may not match production behavior.
   - 7C: Remove explicit “bad tool name” examples to avoid priming (keep only the positive rule “use bigquery__execute_sql”).
     - Pros: May reduce accidental `...commentary/json` tool names.
     - Cons: Less explicit guardrail; could increase other tool-name drift.
   - Recommendation: **7A** (most reliable for harness + production), with 7B as fallback if you want to avoid changing prompt defaults.

8. **Harness execution mode (new)**
   - Requirement: tests must continue after failures, produce a summary at the end, and return exit code 0 (all pass) / 1 (any fail).
   - Requirement: support parallel execution with configurable slots (2–3 default).
   - 8A: Add CLI flags `--continue` / `--fail-fast` and `--jobs N` with default fail-fast + single job.
     - Pros: Explicit, user-friendly; backwards compatible.
     - Cons: More CLI parsing logic.
   - 8B: Use env vars only (`CONTINUE_ON_FAIL=1`, `JOBS=3`) without flags.
     - Pros: Minimal parsing changes.
     - Cons: Less discoverable; not a CLI flag.
   - Recommendation: **8A** (meets the “command line flag” requirement and allows env overrides).

### Decisions locked in (2025-12-21)
- 1A accepted: single canonical realized ARR rule = metrics_daily onprem_arr + static manual360_asat_20251002 baseline for dates ≤ 2025-10-01; no per-day manual360 joins.
- 2A accepted: templates are “verbatim after substituting ISO dates”; remove “no placeholders” ambiguity.
- 3A accepted: default cap at yesterday; include today only if the user explicitly insists. Note added to prompt.
- 4A accepted: manual_360_bq is banned; removed from allowed inventories.
- 5A accepted: only `netdata-analytics-bi.telemetry.production_events_daily` is allowed; `netdata-telemetry.production.*` is forbidden.
- 6A accepted: freshness note must live inside `notes` when a strict schema is provided; no extra top-level keys.

### Decisions captured
- Proceed to build KPI templates first and attach the top 20–30 panels as concrete examples (agreed 2025-12-17).
- Validation harness will use `--schema` (CLI expectedOutput override) so each case enforces JSON output and allows numeric comparisons between agent answers and reference BigQuery results (decision 2025-12-17).
- `watch_towers.manual_360_bq` remains blocked for BigQuery tool dry-run/Drive credentials; keep parity intent but allow skipping spikes in tests until DevOps fixes access.
- **Unification decision (2025-12-18)**: Use `neda/executive2.ai` as the single working BI agent while we build the KPI catalog + parity tests; once stable, migrate the content into `neda/bigquery.ai` and remove `executive*.ai` (after Costa verifies).
- **Parity strategy (2025-12-18)**: Start with **2B** — top ~30 panels per-panel tests, plus 1–2 tests per KPI template, plus sampling for the long tail; reassess after we see how well the KPI catalog generalizes.
- **On-prem “spikes” source (2025-12-18)**: For Grafana parity, prefer `watch_towers.manual360_asat_YYYYMMDD` (native snapshot tables) and the baseline logic used by panel 7 (`manual360_asat_20251002` summed and applied to dates <= 2025-10-01). Avoid relying on the Drive-backed `watch_towers.manual_360_bq` external table in parity templates unless strictly necessary.
- **DevOps re-test request (2025-12-18)**: DevOps updated permissions and asked us to re-test `watch_towers.manual_360_bq` access (both `bq` CLI and BigQuery MCP/toolbox). If the MCP access works, we can re-enable “spikes” parity tests/panels that depend on the Drive-backed external table; if not, we keep skipping them and escalate the specific failing query to DevOps.
- **Proceed with workaround (2025-12-18)**: Even though BigQuery MCP/toolbox still cannot access `watch_towers.manual_360_bq`, continue the refactor by using the snapshot-table workaround (`manual360_asat_YYYYMMDD` + panel-specific baseline logic) and keep building the KPI templates + top-panel parity tests.
- **Test window stability (2025-12-18)**: Default harness window is now “last 7 complete days (ending yesterday)” to avoid `CURRENT_DATE()` rows with missing metrics causing spurious 0/null mismatches.
- **Realized ARR KPI (2025-12-18)**: Implemented template suite (`realized_arr_stat_last_not_null`, `realized_arr_timeseries`, `realized_arr_delta_window`, `realized_arr_customer_diff`, `realized_arr_shares`) plus on-prem baseline rule (static manual360 snapshot <= 2025-10-01). Harness questions now reference template names to avoid improvisation.
- **Context merge plan (2025-12-19)**: Merge bigquery.ai context into executive2.ai while keeping it lean—add freshness note, read-only/allowed datasets, data-flow overview, KPI-definition vs template distinction, and clarify discount/on-prem terminology without expanding template SQL mass. Retain JSON-only output and template-first guardrails.
- **Context merge applied (2025-12-19)**: Executive2 now embeds condensed bigquery.ai policy: freshness query requirement (notes), allowed datasets/read-only, data-flow summary, entity quick-reference, KPI intent mapping, discount naming clarification, and reiteration of template-first + JSON-only rules. On-prem parity remains the static manual360 baseline until told otherwise.
- **KPI template port (2025-12-19)**: All remaining Grafana-aligned KPI templates from scratch promoted into executive2 (business/homelab/on-prem levels & deltas, subscriptions, nodes, AI bundles/credits, trials 6+ est value, professional services, SaaS spaces counts/percent, nodes snapshots, unrealized/ending trials charts). One copy of each template kept; JSON-only contract preserved.

### Panel → template coverage snapshot (top-of-dashboard, 2025-12-18)
Status legend: ✅ in `executive2.ai` (prod), 🟨 in `executive2-scratch.ai` (WIP), ⛔ not mapped yet.
- 7 `$ Realized ARR - Forecasts` – ✅ (realized_arr_timeseries + on-prem baseline)
- 195 `Realized ARR $` – ✅ (realized_arr_components_last_window)
- 196 `Realized ARR %` – ✅ (realized_arr_percent_last_window)
- 91 `Trials total` – 🟨 (trials_total_last_not_null)
- 197 `Trial metrics (6+ nodes est value)` – 🟨 (trial_6plus_nodes_est_value_last_not_null)
- 51 `Total ARR + Unrealized ARR` – 🟨 (total_arr_plus_unrealized_arr_last_not_null)
- 232 `Business professional services sum` – 🟨 (business_plan_services_money_last_not_null)
- 192 `Unrealized ARR` (stat) – 🟨 (unrealized_arr_last_not_null)
- 208 `Unrealized ARR` (barchart) – 🟨 (unrealized_arr_barchart_snapshot)
- 191 `$ Realized ARR` (stat) – ✅ (realized_arr_stat_last_not_null)
- 137 `Realized ARR deltas` – ✅ (realized_arr_deltas_last_not_null)
- 209 `Ending Trial Spaces` – 🟨 (ending_trial_spaces_barchart_snapshot)
- 92 `New Business Subscriptions` – 🟨 (new_business_subscriptions_timeseries)
- 193 `Churned Business Subscriptions` – 🟨 (churned_business_subscriptions_timeseries)
- 215 `AI Bundle metrics` – 🟨 (ai_bundle_metrics_timeseries)
- 216 `Spaces with AI Credits` – 🟨 (ai_credits_spaces_stat)
- 194 row “Top view metrics (Numbers are yesterday…)” – container only
- 50 `Business Subscriptions` – 🟨 (business_subscriptions_last_not_null)
- 56 `Business ARR` – 🟨 (business_arr_discount_last_not_null)
- 55 `Business Nodes` – 🟨 (business_nodes_last_not_null)
- 205 `Windows Reachable Nodes` (stat) – 🟨 (windows_reachable_nodes_stat)
- 198 `SaaS Spaces` – 🟨 (saas_spaces_counts_snapshot)
- 200 `SaaS Spaces Total view` – 🟨 (saas_spaces_percent_snapshot)
- 203 `Nodes Total View` – 🟨 (nodes_total_view_percent_snapshot)
- 139/141/142 (business subs/arr/nodes deltas) – 🟨 (business_subscriptions_deltas_last, business_arr_discount_deltas_last, business_nodes_deltas_last)
- 124/125/127 Homelab subs/ARR/nodes – 🟨 (homelab_subscriptions_last_not_null, homelab_arr_discount_last_not_null, homelab_nodes_last_not_null)
- 204 `Nodes` (stat) – 🟨 (nodes_counts_snapshot)
- 210 `Windows Reachable Nodes` (timeseries) – 🟨 (windows_reachable_nodes_timeseries)
- 199/202 drill-down pies – 🟨 (saas_spaces_drilldown, nodes_drilldown) not yet templated
- 129 `On Prem & Support Subscriptions` – 🟨 (on_prem_customers_last_not_null)
- 57 `On Prem ARR` – 🟨 (on_prem_arr_last_not_null)

Next actions to reach parity: promote 🟨 templates from scratch → prod after harness proof; add mappings for 199/202 pies if needed.

## Ground rules for testing & prompt design (must not forget)

1) **User-level questions only.** Tests must use natural language asks that a user would type (e.g., “show daily churned business subscriptions from X to Y”), never panel numbers, Grafana references, or template names.  
2) **No “cheating” via user prompts.** If the model fails to pick the right SQL, fix the system prompt/templates, not the user question. Template hints in user prompts are prohibited for production realism.  
3) **System prompt responsibility.** The system prompt must clearly map common KPI intents to the cataloged templates without relying on panel IDs. Keep template catalog, but strip Grafana/panel wording from template descriptions.  
4) **Date-window rule (global).** Treat `from` and `to` as inclusive; if `to` is today or future, cap at yesterday unless the user explicitly asks to include today. Apply across templates and harness.  
5) **Validation harness.** Every test case must: (a) run a BigQuery reference query, (b) ask the agent with a natural question, (c) compare JSON outputs. No panel/template identifiers in the user prompt.  
6) **KPI coverage target.** Parity is defined as: for each Grafana panel in scope, there exists a natural-language test that passes against the reference SQL.  
7) **Failure handling.** On mismatch, adjust system prompt/template logic (and, if needed, template catalog) until the natural-language test passes—do not relax the test question.  
8) **Documentation sync.** Any change to prompt contract or date rules must be reflected in `executive2.ai` comments and this TODO.  
9) **Execution realism.** Keep tool usage guardrails (no params; inline dates) and performance safeguards, but they must be invisible to the user-facing questions.

## What we are testing
- Ability of `executive2.ai` (prod prompt) to answer natural-language KPI asks that correspond to Grafana-derived KPIs, without seeing panel IDs.
- Numeric parity with BigQuery reference SQL for each KPI (top panels first, then long tail).
- Correct window handling per the inclusive + cap-today rule.
- Reliable top-10 delta answers (ARR and nodes) using SQL-only delta templates (no in-memory snapshot comparison).
- Correct mapping of "users/customers/spaces signed up" and "churned customers/deleted spaces" per product taxonomy.

## Expected system prompt shape
- Concise guardrails (tool usage, date rule, template-first, avoid macros/params).
- KPI catalog with neutral names/descriptions (no panel numbers), each with exact SQL and expected JSON shape.
- Clear instruction that the model must choose the right template from user intent alone.
- Prominent, early user-intent mapping for signups/customers/spaces/churn (Business/Homelab/Trial/Community).

## Test suite requirements
- One natural-language test per KPI/panel in scope.
- Reference SQL mirrors Grafana logic (parameterized with from/to, no macros).
- Agent called with JSON schema enforcement; outputs compared numerically/structurally.
- No template/panel hints in test user prompts.
- Harness must run with **extreme timeouts** (10h llm/tool) and **continue mode** + **jobs=3** for full-suite runs.
- Use session snapshots (`~/.ai-agent/sessions/{txn}.json.gz`) via `neda/bigquery-snapshot-dump.sh` to debug failing cases.

## Current focus (Dec 22)
- Re-run full harness with `MODEL_OVERRIDE=nova/gpt-oss-20b`, `TEMPERATURE_OVERRIDE=0`, `--continue --jobs 3`, and **10h timeouts**.
- For each failure: extract snapshot via `neda/bigquery-snapshot-dump.sh`, inspect SQL + final JSON, and determine whether the error is **format-only** or **wrong-query**.
- If wrong-query: strengthen prompt rule at the earliest relevant section (avoid scattered exceptions).

### Top panels covered in parity harness (so far)
- Panel 7: `$ Realized ARR - Forecasts` (discounted realized ARR incl. on-prem baseline)
- Panel 195: `Realized ARR $` (components + total)
- Panel 196: `Realized ARR %` (component shares)
- Panel 91: `Trials total` (lastNotNull)
- Panel 197: `Trial metrics` (6+ nodes estimated value, lastNotNull)
- Panel 51: `Total ARR + Unrealized ARR` (lastNotNull)
- Panel 232: `Business professional services sum` (lastNotNull)
- Panel 192: `Unrealized ARR` (business/homelab newcomer ARR, lastNotNull)
- Panel 208: `Unrealized ARR` (barchart; snapshot-based `to_be_realized` grouped by `cx_arr_realized_at`)
- Panel 191: `$ Realized ARR` (stat; lastNotNull of discounted realized ARR incl. on-prem baseline)
- Panel 137: `Realized ARR deltas` (stat; lastNotNull 7/30/90 deltas on realized ARR series)
- Panel 209: `Ending Trial Spaces` (barchart; snapshot-based reachable nodes grouped by `ax_trial_ends_at`)
- Panel 92: `New Business Subscriptions` (timeseries)
- Panel 193: `Churned Business Subscriptions` (timeseries)
- Panel 215: `AI Bundle metrics` (timeseries)
- Panel 216: `Spaces with AI Credits` (lastNotNull)
- Panel 50: `Business Subscriptions` (lastNotNull, stable yesterday semantics)
- Panel 139: `Business Subscriptions deltas` (stat; last 7/30/90 deltas)
- Panel 56: `Business ARR` (last/lastNotNull, stable yesterday semantics)
- Panel 141: `Business ARR discounted deltas` (stat; last 7/30/90 deltas)
- Panel 55: `Business Nodes` (last/lastNotNull, stable yesterday semantics)
- Panel 142: `Business Nodes deltas` (stat; last 7/30/90 deltas)
- Panel 205: `Windows Reachable Nodes` (lastNotNull)
- Panel 198: `SaaS Spaces` (stat; snapshot-based segment counts)
- Panel 200: `SaaS Spaces Total view` (pie; snapshot-based segment % + `check`)
- Panel 203: `Nodes Total View` (pie; snapshot-based reachable nodes % + `check`)
- Panel 204: `Nodes` (stat; snapshot-based reachable nodes totals by segment)
- Panel 210: `Windows Reachable Nodes` (timeseries breakdown by segment)
- Panel 124: `Homelab Subscriptions` (stat; lastNotNull)
- Panel 125: `Homelab ARR` (stat; lastNotNull)
- Panel 127: `Homelab nodes` (stat; lastNotNull)
- Panel 129: `On Prem & Support Subscriptions` (stat; lastNotNull; static `5` baseline <= 2025-10-04)
- Panel 57: `On Prem ARR` (stat; lastNotNull)

### Progress update (2025-12-18)
- Added KPI templates for the remaining missing “top ~30” panels (snapshot-based SaaS segment panels + delta panels + the snapshot barcharts).
- Added `trials_total_last_not_null` template to `executive2.ai` so trials total KPI is covered without panel/template hints.
- Expanded `neda/bigquery-test.sh` to cover these panels and compare numeric outputs vs `bq` reference queries.
- Tightened the prompt to be **template-first** (anti-improvisation) while keeping user prompts natural-language only (no panel IDs, no template hints); routing must be inferred from intent.
- Hardened the harness parsers to accept the agent’s JSON even when it forgets notes (treat a single array output as `{data: [...], notes: []}`) while still flagging missing notes in the prompt contract.
- Added an explicit guardrail: when the user asks for customer won/lost/increase/decrease, the agent must run `realized_arr_customer_diff`, return the customer rows in `data`, and never replace them with prose. If no rows, return `data: []` and note why.
- Clarified date defaults: if the user provides only a start date (“since X”), set `from_date = X`, `to_date = yesterday` (cap rule), and state both dates in notes; never shrink the window.
- Added harness coverage for the “since-only” variant of realized_arr customer diff to verify the implicit `to_date=yesterday` path.
- Ran the harness after the template-hint fixes: `realized_arr` (stat + components) and `total_arr_plus_unrealized_arr` cases pass; full suite was interrupted due to time—need a complete re-run. Manual_360 external table still noted as risky; workaround baseline remains in templates.

### Progress update (2025-12-22)
- Harness run (gpt-oss-20b, `--continue --jobs 3`, 10h cap) completed: **51 total, 40 pass, 11 fail**; exit code 1.
- Failing cases (needs fixes): `ending_trial_spaces_barchart_snapshot`, `business_arr_discount_deltas`, `nodes_combined_growth_pct`, `business_nodes_growth_pct`, `on_prem_customers`, `homelab_subscriptions`, `customers_growth_pct`, `realized_arr_kpi_customer_diff`, `realized_arr_kpi_delta`, `homelab_nodes_delta_top10`, `business_nodes_delta_top10`.
- Observed root causes (facts from logs/diffs):
  - Tool hallucinations: invalid tool names like `bigquery__execute_sqlcommentary` / `bigquery__execute_sqljson` / `agent__task_statusjson` causing retries or partial outputs.
  - Output wrapper violations: model sometimes returns `{status, content_json}` instead of the required `{data, notes}`.
  - `on_prem_customers` returned a 7‑day timeseries instead of latest stat (should be ORDER BY date DESC LIMIT 1).
  - `homelab_subscriptions` off by 1 day (agent picked 2025‑12‑20 vs reference 2025‑12‑21).
  - Growth % cases returned non‑null percent changes where reference is `null` (missing lag); model is “backfilling” instead of keeping nulls.
  - `realized_arr_kpi_customer_diff` missing one side (only 10 rows; should be top 10 gains + top 10 losses).
  - `realized_arr_kpi_delta` uses wrong key names (`delta_*` vs `*_delta`).
  - Node delta top‑10: status labels mismatch (`lost` vs `decrease`), plus missing/extra rows.
- Reliability requirement (Costa): customer/node change questions must use SQL to compute deltas directly (top 10 gains/losses), **not** snapshot diffs in the model; current behavior is unreliable and must be corrected in prompt/templates.
- Next prompt adjustments needed (planned):
  - Strengthen tool name guardrail (positive-only) to stop `...commentary/json` tool calls.
  - Explicitly forbid wrapper outputs; enforce root `{data, notes}` only.
  - Add/clarify templates for latest‑stat queries (homelab subscriptions, on‑prem customers) with `ORDER BY date DESC LIMIT 1`.
  - Add a hard rule: **do not compute missing LAGs**; if LAG is null, return null.
  - Enforce `increase/decrease/no_change` status vocabulary for delta top‑10 templates.
- Enforce “top 10 gains + top 10 losses = 20 rows” for realized ARR customer diff template.
- Fix key naming in realized ARR delta template to match harness (`business_delta`, `total_delta`, etc.).
- Decision needed (2025-12-22): choose how to proceed with the fixes and re-run.
  - 1A: Apply the above prompt/template fixes in one batch, then re-run the full harness.
  - 1B: Review each failing case with Costa first, then fix incrementally.
  - 1C: Pause harness fixes and focus first on redesigning the delta/top‑10 strategy (SQL-first, no snapshot diffs), then re-run.
- Recommendation: 1A (fastest path to regain full pass/fail signal; we can refine after).
  - Status: Costa asked to proceed with prompt improvements (no new decision required).

### Progress update (2025-12-22, later)
- Added snapshot extractor script: `neda/bigquery-snapshot-dump.sh` (parses `~/.ai-agent/sessions/<txn_id>.json.gz` into SQL/tool/final outputs).
- Applied prompt fixes in `neda/executive2.ai` to address failing cases:
  - Stronger mappings in “Understanding user requests” for ending trials, growth %, top‑10 deltas, and ARR window deltas.
  - Explicit “do not change explicit to_date/snapshot_date” rule (duplicate in core rules).
  - Output contract now forbids wrapper objects like `{status, content_json}`.
  - Core rules now enforce: run KPI query after freshness, never filter top‑10 rows, node statuses are increase/decrease only, ARR statuses follow SQL, no helper queries for growth %, and no recomputing/null‑override.
  - Added stat templates: `homelab_subscriptions_stat_last_not_null`, `on_prem_customers_stat_last_not_null`.
  - Added alias templates: `customers_growth_pct_timeseries`, `business_nodes_growth_pct_timeseries`, `nodes_combined_growth_pct_timeseries`.
  - Added `realized_arr_kpi_delta_window` template with correct `*_delta` key names.
  - Reinforced `ending_trial_spaces_barchart_snapshot` must run and not stop after freshness.
  - Reinforced `space_nodes_delta_top10_business` filter to avoid `plan_class LIKE 'Business%'`.
- Not yet re-run harness after these prompt changes.
- Harness run instruction (hard): always execute harness with an extreme timeout so it never gets killed by the CLI (Costa: "I don't want your timeout to ever hit").
- Added **single-SQL** top‑10 delta templates to `neda/executive2.ai` to avoid MCP output truncation and model snapshot‑comparison errors:
  - `space_arr_delta_top10` (top 10 won/increase + top 10 lost/decrease by ARR delta).
  - `space_nodes_delta_top10_business` and `space_nodes_delta_top10_homelab` (top 10 increase/decrease reachable nodes; no trials).
- Harness run (gpt-oss-20b, reasoning disabled) still takes long with `--verbose` and hit the CLI timeout when running the full suite. No mismatches were found in the diff files generated before timeout.
- Fixed `ONLY_CASE` exit behavior in `neda/bigquery-test.sh` (now returns 0 when a matching case runs; errors only if the case name is unknown).
- Re-ran `ONLY_CASE=windows_reachable_nodes` after the fix; case passes and now exits 0.
- Full harness re-run stops at **total_arr_plus_unrealized_arr** with a mismatch:
  - Ref: 1,396,384.251 vs Agent: 1,707,223.2177 (difference ≈ 310,839 = manual360 static baseline).
  - Indicates the agent is incorrectly applying the on-prem static baseline to dates > 2025-10-01, despite the template rule. Need to tighten template enforcement and/or add explicit “baseline must be 0 after 2025‑10‑01” guard in prompt.

### Progress update (2025-12-22, latest)
- Migration applied for production testing:
  - Backed up `neda/bigquery.ai` → `neda/bigquery.ai.old`.
  - Copied `neda/executive2.ai` → `neda/bigquery.ai`.
  - Renamed scripts: `neda/executive-test.sh` → `neda/bigquery-test.sh`, `neda/executive-snapshot-dump.sh` → `neda/bigquery-snapshot-dump.sh`.
  - Renamed TODO: `TODO-executive.md` → `TODO-bigquery.md`.
  - Removed `executive.ai` references in `neda/neda.ai` and `neda/neda-core.md`; expanded bigquery description to include executive-level aggregations.
- Note: `neda/executive2.ai` remains as the working copy for prompt edits; production now uses `neda/bigquery.ai`.
- Full harness re-run completed with extreme timeout: **51 total, 50 pass, 1 fail**.
- Only failing case: `homelab_nodes_delta_top10`. Snapshot review showed the model used INNER JOIN and `ce_plan_class LIKE 'Homelab%'` with concrete `spaces_asat_YYYYMMDD` tables, causing missing/extra rows.
- Prompt fixes applied:
  - Added hard rule for **FULL OUTER JOIN + COALESCE** on delta top‑10 queries.
  - Added hard rule for **exact Homelab filter** (`ce_plan_class = 'Homelab'`; no LIKE).
  - Reinforced join rule globally for top‑10 delta templates.
- Re-ran `--only-case homelab_nodes_delta_top10` after the fix: **PASS**.
- Full-suite re-run still pending after the last prompt tweak; expect 51/51 if no regressions.
- Updated prompt to respect explicit user intent for top‑10 deltas: if the user asks only gains or only losses, return only that side; otherwise return both sides. (Matches “respect user requests”.)
  - Log also shows a malformed tool call attempt (`bigquery__execute_sqljson`) before retrying; reinforce “use only bigquery__execute_sql” in the prompt.
- Updated FAQ in `neda/executive2.ai` to route natural-language “won/lost/increase/decrease” and “nodes add/remove” questions to these templates (no Grafana/panel IDs).
- Extended harness with new cases:
  - `business_nodes_delta_top10`
  - `homelab_nodes_delta_top10`
  - (existing realized ARR customer diff remains the canonical top‑10 ARR delta case).
- Identified harness compare bug: nodes delta cases were using the ARR comparator (null subtraction). Fixed compare call sites to use the correct comparator (needs re-run to verify).
- **Follow‑up fixes & validations:**
  - Updated `case_realized_arr_kpi_customer_diff_sql` to match the **positive/negative** delta semantics (not just won/lost).
  - Hardened `compare_customer_delta_fields` against missing rows (no null subtraction; safe diff).
  - Added explicit “do not shift from_date” rules (global + template) so “since X” uses `from_date = X` exactly.
- Re‑ran targeted harness cases: `business_nodes_delta_top10`, `homelab_nodes_delta_top10`, `realized_arr_kpi_customer_diff`, `realized_arr_kpi_customer_diff_since_only` — all **PASS**.
- Full harness re-run (reasoning disabled) **failed early** at `total_arr_plus_unrealized_arr` due to repeated invalid tool calls (`bigquery__execute_sqlcommentary`) and a final schema-violating response (`status: "partial"` + empty data). This indicates gpt-oss-20b is still hallucinating tool names under load; we need additional guardrails (and/or lower temperature) before the full suite can pass reliably.
- Added harness enhancements to continue after failures and run cases in parallel:
  - CLI flags `--continue` / `--fail-fast` and `--jobs N`.
  - Per-case logs under `tmp/bigquery-tests/logs/<case>.log`.
  - End-of-run summary with exit code (0 all pass, 1 any fail).
  - `--only-case` is supported as a flag (in addition to ONLY_CASE env).
 - Full suite on `gpt-oss-20b` (MODEL_OVERRIDE default) completed: **51 total, 44 pass, 7 fail**.
   - Failures: `business_nodes`, `business_subscriptions_deltas`, `nodes_reachable_growth_pct`, `homelab_nodes_growth_pct`, `on_prem_customers`, `business_nodes_delta_top10`, `active_users`.
   - Root causes observed in logs:
     - Invalid tool names (`bigquery__execute_sqlcommentary`, `bigquery__execute_sqljson`, `agent__task_statuscommentary`) causing no_tools failures.
     - Stat KPIs returning timeseries (active_users, on_prem_customers) instead of LIMIT 1.
     - Growth % window widened to compute lags (nodes_reachable/homelab nodes growth), contrary to template.
     - Node delta status relabeled as won/lost instead of increase/decrease.
     - Business subscriptions deltas swapped last_30_days and last_90_days.
   - Prompt fixes applied in `neda/bigquery.ai`:
     - Added tool-name hard stop (no suffixes), stat-only KPI list, and explicit mappings for business nodes/on-prem customers and growth % routes.
     - Added `business_nodes_stat_last_not_null` and `business_subscriptions_deltas_latest` templates.
     - Reinforced “no window widening” for growth % and “no relabeling” for node delta status.
     - Added explicit mapping and rules for Business subscriptions deltas to preserve column labels.
 - Re-run needed for the 7 previously failing cases after these prompt changes.

## Decisions (2025-12-18)
- D1: Adopt KPI-first template catalog (no per-panel SQL in prompt). Maintain panel parity via harness instantiations. **Accepted (Costa)**.
- D2: Start with KPI `realized_arr` as the first fully parameterized template; must support (a) current value total + per product, (b) change over a window with per-product deltas, (c) customer-level impact (won/lost). **Accepted (Costa)**.

## Decisions (2025-12-21)
- D3: Treat panel IDs/Grafana references as out-of-bounds for prompts and tests; user questions must be natural language only. System prompt must do routing from intent alone. **Accepted (Costa)**.
- D4: Keep `executive2.ai` lean; move unvalidated/WIP KPI templates to `executive2-scratch.ai` while production prompt holds only validated templates. **Accepted (Costa)**.
- D5: Output format enforcement belongs in the user prompt/harness. System prompt should allow multiple formats; JSON/schema rules are provided via user/tooling when needed. **Accepted (Costa)**.

## Critical reliability requirements (must not regress)
- **Primary question #1 (ARR change by customer):** The agent must reliably answer “Top 10 won/increased and top 10 lost/decreased customers between T0 and T1 by ARR change.”
  - **Strategy:** Always use a **single SQL query** that computes deltas from two snapshots and returns top 10 positives + top 10 negatives. Do **not** fetch two snapshots and compare in the model.
- **Primary question #2 (nodes change by space):** The agent must reliably answer “Top 10 paid subscribers adding/removing nodes between T0 and T1.”
  - **Strategy:** Always use a **single SQL query** that computes node deltas in BigQuery and returns top 10 positives + top 10 negatives for **Business** and **Homelab** separately. No trials.
- **Reasoning visibility:** When debugging, run with `--override models=nova/gpt-oss-20b` to capture full reasoning; do not switch default model unless Costa asks.

## Plan (active)
- P0 (migration): backup `neda/bigquery.ai` → `neda/bigquery.ai.old`, copy `neda/executive2.ai` → `neda/bigquery.ai`, remove `executive.ai` references in `neda.ai` / `neda-core.md`, update `neda-code.md` description, rename scripts + TODO to *bigquery*.
- P1: Extract top 20–30 panels from the latest snapshot; map each to a canonical template (macro-free, parameterized dates) while preserving exact Grafana math.
- P2: Validate 3–5 representative top-panel queries via BigQuery toolbox to confirm parity (Realized ARR $, ARR %, Trials total, Business Subscriptions, Nodes).
- P3: Propose prompt rewrite: keep parity templates for top panels; summarize/generalize lower-priority panels into thematic guidance + templates.
- P4: Add guardrails (scope → inspect tables → bounded query) and JSON output schema; integrate into prompt.
- P5: If approved, replace inline SQL dump with compact parity templates + optional external sql-library for traceability; ensure context fits models.
- P6: Run `npm run lint` + `npm run build` after prompt edits; revalidate a small query set.
- P7: Extend `neda/bigquery-test.sh` to store reference/agent JSON, apply schemas per case, and compute simple numeric diffs (start with realized ARR; add trials/subs/nodes next).
- P8: Add canonical template for `realized_arr_percent_timeseries` (component shares) mirroring Grafana logic; update prompt and harness case `realized_arr_percent`.
- P9: Re-run harness with ONLY_CASE filters to stabilize realized_arr_percent; then full suite to ensure no regressions.
- P8: Build a “KPI catalog” (definitions + canonical SQL templates) that is small and test-driven; reference it from `executive2.ai` instead of embedding per-panel SQL. Back it with concrete examples from the top panels.
- P9: Once `executive2.ai` covers the top panels + long-tail templates well, migrate the finalized catalog/workflow into `neda/bigquery.ai`, then delete `neda/executive.ai` and `neda/executive2.ai` **only after Costa confirms**.
- P10: Add a dedicated harness case that queries `watch_towers.manual_360_bq` via BigQuery MCP/toolbox (and a reference `bq` CLI query) to detect Drive-credential/dry-run failures early and to validate DevOps permission changes.
- P11: Re-run harness after comparator fix, starting with `ONLY_CASE=business_nodes_delta_top10` and `homelab_nodes_delta_top10`, then full suite with the larger timeout. If failures persist, add SQL-side ordering/filters or update template routing (not user prompts).

## Implied decisions/assumptions
- SQL should avoid Grafana macros; replace with standard parameters (e.g., `WHERE date BETWEEN @from AND @to`).
- We will maintain a small, maintained library of vetted queries/templates rather than auto-generated dashboards, but those templates must stay numerically equal to Grafana charts.
- Safety: always include date filters and `LIMIT` for diagnostics before full runs.
- Single-agent rule: when a user asks for a “Grafana KPI name” (e.g. Realized ARR), the agent must use the KPI catalog template for that KPI (no improvising from other tables); ad-hoc analyses are allowed but must be explicitly labeled as such.

## Testing requirements
- Run `npm run lint` and `npm run build` after prompt changes (repo standard).
- Manual sanity: execute 2–3 sample BigQuery calls via the agent or tool harness to ensure macros are gone and queries run.
- Maintain the parity harness as the gate: top panel tests must remain green as we expand prompt scope.

## Documentation updates
- If prompt contract/output schema changes, update `docs/AI-AGENT-GUIDE.md` and any README/headend references to `neda/executive.ai`.
- Add/adjust a short “Executive BI agent” section describing workflow and supported metrics.
- When migrating `executive2.ai` into `bigquery.ai`, update docs to reflect the single-agent entrypoint and remove references to `executive*.ai`.
