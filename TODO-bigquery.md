# TODO – Refactor `neda/bigquery.ai`

## TL;DR
- File is ~7.9k lines (~100k tokens) of auto-generated Grafana SQL; huge context and high confusion.
- BigQuery agent must mirror Grafana charts exactly; bigquery.ai now includes the former executive2.ai prompt for parity.
- Need a slimmer, guardrailed prompt that still preserves Grafana-fidelity (canonical formulas/templates) instead of raw dumps.
- **Migration request (Costa):** Replace `bigquery.ai` with `executive2.ai` for production testing, remove executive.ai references, and rename harness/scripts/TODO from *executive* → *bigquery*.
- **Latest (2025-12-22):** Expand user-intent map for signups/customers/spaces/churn, then re-run the full harness with extreme timeouts and debug remaining failures.
- **New (2025-12-26):** Consolidate `neda/bigquery.ai` to reduce token load by removing duplicated rules (freshness, source selection, stat vs timeseries, realized ARR) via surgical edits only.

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
- Prompt duplication hotspots (after full read, 2025-12-26):
  - Data freshness rules repeated in **0.3**, **7.1**, **9.3**, and Output Contract.
  - Source selection / temporal routing repeated in **Domain Model**, **Source Selection Matrix**, **Authority & Scope**, **Entity Cheat Sheet**, and **Question → Table Decision Tree**.
  - Stat vs timeseries + schema maxItems rules repeated in **Understanding user requests**, **Output Contract**, **Core execution rules**, **Known pitfalls**, and **FAQ**.
  - Canonical realized ARR formula repeated in **Domain Model**, **Core execution rules**, **Canonical realized ARR**, **Business Logic 5.14**, **Query Patterns 7.5**, and **FAQ**.
  - `manual_360_bq` access restriction repeated in **0.2**, **1.4**, **9.1**, and **9.8**.
  - Date-window rules (cap today / explicit to_date) repeated in **How to run queries**, **Core execution rules**, and **Known pitfalls**.
- Consolidation applied (2025-12-26): temporal routing centralized under **Source Selection Matrix → Temporal Data Routing (canonical)**; duplicate routing rules trimmed in sections 3.3, 7.2, 7.10, 8.6, 9.10, 9.11; routing reminders compressed.
- Consolidation applied (2025-12-26): **Date Handling Precedence (canonical)** added under core workflow; duplicate date rules trimmed in **How to run queries**, **Core execution rules**, and **Known pitfalls 9.3**.
- Consolidation applied (2025-12-26): **Stat vs Timeseries Decision (canonical)** added; duplicate stat/timeseries and maxItems rules trimmed in **Understanding user requests**, **Output contract**, and **Core execution rules**.
- Consolidation applied (2025-12-27): **metrics_daily_stat_last_not_null (generic)** added with a Stat KPI registry; removed per‑KPI stat templates for AWS ARR/subscriptions, active users, AI credits, trials total, business ARR, business nodes, homelab subs, unrealized ARR latest, virtual nodes, and trial 6+ estimate.
- Consolidation applied (2025-12-27): **metrics_daily_timeseries_basic (generic)** added with a Timeseries KPI registry; removed per‑KPI timeseries templates for Business ARR, AI bundles, and Windows reachable nodes breakdown.
- Consolidation applied (2025-12-27): **metrics_daily_timeseries_spine (generic)** added with a Spine KPI registry; removed `unrealized_arr_timeseries` template and rewired references.
- Consolidation applied (2025-12-27): **metric_growth_pct_timeseries (generic)** converted to registry + rationale; removed growth % alias templates (customers/business nodes/combined nodes) and rewired references to registry entries.
- Consolidation applied (2025-12-27): **segment_snapshot (generic)** added with a snapshot registry; removed per‑template snapshot segment counts/percent templates for spaces and nodes, and rewired references.

## Philosophy (Costa, 2025-12-26)
- Goal is to **educate the agent**, not constrain it as a “dummy.” Avoid “do X because we say so.”
- Prefer **explanations**: “when you need A, do B because X/Y/Z,” with reasoning and context.
- Templates should include **use‑cases, rationale, useful joins, pitfalls, and best practices** so the model can **improvise safely**.
- Guardrails are acceptable only when necessary for correctness, but the primary intent is **understanding‑first** guidance.

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

9. **Prompt difficulty diagnostics in agent output (new)**
   - Context: Costa wants per-test “stress indicators” to spot fragile prompt areas even when numeric outputs pass.
   - 9A: **Hybrid (recommended)** — add a small `diagnostics` object to schemas + prompt guidance for self‑report; compute objective signals (turns/tool errors/timeouts) in the harness logs.
     - Pros: Balanced; subjective + objective; low schema footprint.
     - Cons: Requires schema updates across tests; still some self‑report noise.
   - 9B: **All in schema (self‑report only)** — add fields like `prompt_clarity_pct`, `pivoted`, `confidence_pct`, `notes`, etc., and rely on the model.
     - Pros: Simple to implement; everything in JSON output.
     - Cons: Subjective variance; extra tokens can increase truncation risk.
   - 9C: **Harness‑only diagnostics** — no schema changes; derive difficulty metrics from logs only.
     - Pros: Deterministic; no risk to output length.
     - Cons: No self‑reported “ease”/prompt clarity signal.
   - Recommendation: **9A** (hybrid).

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
- **Diagnostics (2025-12-27)**: Use model self-report for `sql_failures` (do not parse logs); include a diagnostics block in schema and prompt so the agent counts its own failed queries.

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

## Plan agreed (2025-12-28) — “educate, don’t constrain”
Focus on **reasoning cards** (short “why/when/how” explanations) at the template/registry level to improve model understanding without heavy guardrails.
1. **Growth % templates**: explain why we do **not** widen windows, why NULLs are expected at window start, and how that preserves Grafana parity.
2. **Snapshot segment templates**: explain the **segment taxonomy** (trial vs newcomer vs paid), why `ax_trial_ends_at` is the trial signal, and why counts vs node sums must not be mixed.
3. **Barchart snapshot templates**: explain **date bucketing** (e.g., `DATE(cx_arr_realized_at)`), why weekly grouping is incorrect, and how snapshot scope limits dates.
4. **Customer list / top‑N templates**: explain **current vs snapshot** routing, ordering, and why strict filters/limits are required for parity.
5. **“Since only” deltas**: explain how `since X` maps to **two snapshots** (`from_date = X`, `to_date = yesterday`) and why this is required to avoid drifting IDs.

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

### Progress update (2025-12-23)
- Costa requested a full harness re-run. Next action: run the full suite with extreme timeouts using `--continue --jobs 3` (gpt-oss-20b; consider `TEMPERATURE_OVERRIDE=0` if tool-name hallucinations persist).
- Full suite run completed: **51 total, 45 pass, 6 fail**.
  - Failures: `nodes_total_view_percent_snapshot`, `homelab_nodes_growth_pct`, `on_prem_customers`, `windows_reachable_nodes_breakdown`, `realized_arr_kpi_delta`, `active_users`.
  - Logs: `tmp/bigquery-tests/logs/<case>.log`.
- Decision needed (2025-12-23): Next step to resolve the 6 failing cases.
  - A) Inspect the 6 log files and propose prompt/template fixes. Pros: fastest root-cause visibility; Cons: takes manual review time. **Recommendation: A**.
  - B) Re-run only the 6 failing cases to confirm flakiness before any changes. Pros: quick signal on stability; Cons: may repeat failures without new insight.
  - C) Re-run the full suite with a different override (e.g., temperature/model) before investigation. Pros: might reduce tool-name hallucinations; Cons: higher time cost and may mask real issues.
- Log inspection summary (2025-12-23):
  - `nodes_total_view_percent_snapshot`: agent returned fractions (0–1) instead of % (0–100). Likely missing `* 100` or wrong denominator scaling in template.
  - `homelab_nodes_growth_pct`: agent queried outside the requested window to fill LAGs, producing non-null pct values; ref expects nulls inside the 7-day window.
  - `on_prem_customers`: agent returned full timeseries; expected latest only (`ORDER BY date DESC LIMIT 1`).
  - `windows_reachable_nodes_breakdown`: agent returned `data: []` / “Data not available” without running the KPI query; should always run the metrics_daily breakdown.
  - `realized_arr_kpi_delta`: tool call never executed; model exhausted retries with `no_tools` and returned `status: partial` + empty data.
  - `active_users`: agent returned full timeseries; expected latest only (single row).
- New requirement (2025-12-23): **Prompt must explicitly explain entity meanings and entity-linking rules** (e.g., Space vs Customer, how subscriptions/users/nodes connect) so the model can answer arbitrary questions without guessing. This requires updating `neda/bigquery.ai` and relevant docs.
- Added Source Selection Matrix section to `neda/bigquery.ai` with deterministic routing rules and examples (per D12–D14).
- Added top‑100 customers >= $2K ARR harness case to `neda/bigquery-test.sh` (schema + reference SQL + compare function).
- Manual query executed for top‑100 customers >= $2K ARR; results saved to `tmp/top_customers_arr_2k.json` (contains admin contact emails).
- New test request (2025-12-23): add a harness case for **Top 100 customers paying >= $2K ARR** with fields:
  - Primary Contact (space owner/admin), Subscription Renewal Date, Current ARR, Nodes committed, Nodes connected.
  - Renewal date rule: annual = start_date + 1 year; forecast ARR equals current ARR.
  - Question from Costa: can we provide this result manually (needs BigQuery execution).

### Progress update (2025-12-23, latest run)
- Full suite run completed: **52 total, 47 pass, 5 fail**.
  - Failures: `customers_growth_pct`, `on_prem_customers`, `top_customers_arr_2k`, `aws_arr`, `realized_arr_kpi_customer_diff`.
  - Logs: `tmp/bigquery-tests/logs/<case>.log`.
- Root causes (from logs/diffs):
  - `customers_growth_pct`: agent ran extra queries and emitted non-null pct_7 despite null lags; must return template results with NULLs preserved.
  - `on_prem_customers`: agent returned a full timeseries and kept earliest row; must ORDER BY date DESC LIMIT 1 and emit a single row.
  - `top_customers_arr_2k`: harness crashed due to unescaped `$2K` in echo banner (case did not execute).
  - `aws_arr`: agent returned `data: []` without executing KPI SQL; needs explicit AWS ARR routing + hard stop.
  - `realized_arr_kpi_customer_diff`: agent dropped one negative-row result after correct SQL; must return all rows from SQL (expect up to 20 when gains+losses asked).
- Fixes applied (pending re-run):
  - `neda/bigquery-test.sh`: escape `$2K` in the case banner.
  - `neda/bigquery.ai`: add AWS ARR routing, hard-stop on growth % output to preserve NULLs, reinforce on-prem latest-row rule, and require full row retention for ARR delta top-10.
  - `docs/AI-AGENT-GUIDE.md`: record growth % null-preservation and AWS ARR routing requirements.
- Next step: re-run full harness after these edits.

### Progress update (2025-12-24)
- Full suite run completed: **52 total, 45 pass, 7 fail**.
  - Failures: `trial_6plus_nodes_est_value`, `nodes_combined_growth_pct`, `homelab_nodes_growth_pct`, `on_prem_customers`, `windows_reachable_nodes_breakdown`, `top_customers_arr_2k`, `active_users`.
- Root causes (facts from logs):
  - `trial_6plus_nodes_est_value`: model returned full timeseries; expected latest row only.
  - `nodes_combined_growth_pct` / `homelab_nodes_growth_pct`: model widened the window and computed non‑NULL pct_* from extra history; must keep NULL lags within requested window and return template rows only.
  - `on_prem_customers` / `active_users`: model returned timeseries; expected single latest row.
  - `windows_reachable_nodes_breakdown`: model used `spaces_asat_*` reachable nodes by plan, not `metrics_daily` windows metrics (values off by ~10–30x).
  - `top_customers_arr_2k`: tool response exceeded `toolResponseMaxBytes` (15000) causing truncated data; committed_nodes/connected_nodes mismatches.
- Fixes applied (prompt + docs; surgical):
  - Added `top_customers_arr_2k_current` template + routing; clarified paid‑plan classes incl. 45d newcomers; updated customer definition and signup wording.
  - Added `trial_6plus_nodes_est_value_stat_last_not_null` template + routing; added to stat‑only KPI list.
  - Added `windows_reachable_nodes_breakdown_timeseries` template + routing; forbid spaces_asat for Windows breakdown.
  - Added explicit “growth % guard” hard‑stop (no extra queries, no window widening).
  - Increased `toolResponseMaxBytes` in `neda/bigquery.ai` to `120000` for 100‑row customer lists.
- Updated docs (`docs/AI-AGENT-GUIDE.md`) to note larger tool response size requirement.
- Next step: re-run full harness after the prompt edits to confirm 52/52.

### Progress update (2025-12-24, later)
- Implemented **stat schema shape** change: `data` is now a **single object** (no `maxItems:1` arrays) for all stat KPIs.
- Added **data_freshness** to **all schemas** and updated prompt rules to populate it when present.
- Removed schema-indicator phrases from **all test questions** (e.g., “Return JSON with data[…]”), leaving only natural-language asks.
- Updated comparators and harness guards to accept `data` as object or array (for backwards compatibility during transition).
- Lint/build were run before these edits; need another run after the latest changes.

### Progress update (2025-12-24, latest)
- Removed schema-instruction prefix from `run_agent` so prompts are plain user questions.
- Replaced placeholder questions (empty/JSON/dot) with natural language.
- Removed explicit formula hints and internal metric names from questions where possible.

### Progress update (2025-12-24, latest run)
- ✅ Reintroduced schema reinforcement: `run_agent` now embeds the full JSON schema in the question (per D18).
- ✅ Switched `realized_arr_stat` to breakdown+total (uses KPI stat schema/SQL); updated question + comparator.
- ✅ Added schema‑authority rules + stricter freshness placement in `neda/bigquery.ai`.
- ✅ Added `ai_credits_spaces_stat_last_not_null` template + hard‑stop rules; strengthened growth‑% guard.
- ✅ Added hard‑stop rules for on‑prem customers (metrics_daily only), realized ARR delta window, and node‑delta top10 templates.
- ✅ Updated docs (`docs/AI-AGENT-GUIDE.md`) with schema‑authority + realized ARR stat behavior.
- ✅ Full harness run completed: **52 total, 45 pass, 7 fail**.
  - Failures: `unrealized_arr_barchart_snapshot`, `ai_credits_spaces`, `business_nodes_growth_pct`, `on_prem_customers`, `realized_arr_kpi_delta`, `business_nodes_delta_top10`, `top_customers_arr_2k`.
  - Root causes (from logs):
    - `unrealized_arr_barchart_snapshot`: used snapshot date ≠ `to_date` → missing/incorrect rows.
    - `ai_credits_spaces`: used `spaces_asat_*` count instead of metrics_daily `ai_credits_space_count`.
    - `business_nodes_growth_pct`: widened window (started at `to_date-30`), producing non‑NULL pct values.
    - `on_prem_customers`: used manual360 snapshot counting on‑prem/support instead of metrics_daily `onprem_customers`.
    - `realized_arr_kpi_delta`: ran timeseries and computed deltas in‑model; must use delta template.
    - `business_nodes_delta_top10`: did not use the top‑10 SQL template verbatim (filters/order differ).
    - `top_customers_arr_2k`: returned only 10 rows instead of 100.

### Progress update (2025-12-24, latest run #2)
- Ran full harness with `--continue --jobs 3`; local command timed out after ~1h while starting `top_customers_arr_2k`.
- Completed remaining cases individually: `aws_arr`, `aws_subscriptions`, `virtual_nodes`, `active_users` **PASS**; `top_customers_arr_2k` **FAIL**.
- Current failures (4): `realized_arr`, `business_subscriptions`, `homelab_nodes`, `top_customers_arr_2k`.
- Root causes (from logs):
  - `realized_arr`: agent output constant **72978** for each day vs ref **~1.36M**; dry‑run error occurred before a re‑try (likely wrong SQL).
  - `business_subscriptions`: agent used `aws_business_subscriptions` (14) instead of `total_business_subscriptions` (1328).
  - `homelab_nodes`: agent used `watch_towers.spaces_asat_*` reachable sum (7954) instead of `metrics_daily.total_reachable_nodes_homelab` (7775).
  - `top_customers_arr_2k`: model output truncated (`stop_reason=length`), schema validation failed; needs higher output cap or shorter response.
- Non‑fatal noise: repeated toolbox stderr `invalid method prompts/list` on MCP init (doesn’t block queries but pollutes logs).

### Progress update (2025-12-24, latest run #3)
- Implemented D21: `maxOutputTokens` set to **32768** in `neda/bigquery.ai`; docs updated.
- Re-ran `top_customers_arr_2k` only: **still FAILS** with `stop_reason=length` (output_tokens=32768); response truncated mid‑JSON.
- Result: 32k is still insufficient for 100 rows with full admin lists; need a higher cap or output reduction/pagination.

### Progress update (2025-12-24, latest run #4)
- Implemented D22: **limit primary_contact to first 3 admins** per space (prompt + SQL + docs).
- Ran `npm run lint` and `npm run build` (both OK).
- Re-ran `top_customers_arr_2k`: **FAIL** because the tool output was **dropped by context guard** (projected tokens exceeded limit); agent returned `UNKNOWN` row.
- Root cause: bigquery tool output (~10k tokens) + large prompt still exceeds context budget; tool output dropped before final response.

### Decisions needed (2025-12-24) — schema compliance + realized ARR stat shape
1) **Schema compliance enforcement in prompt**
   - Option A: Add a global rule: when a JSON schema is provided, output must **exactly** match it (top-level keys, field names, and shapes); **never** rename SQL aliases; include `data_freshness` only when the schema includes it.
     - Pros: keeps current schemas; fixes most failures from renamed keys and missing `data`.
     - Cons: relies on the model to follow strict rules (still possible drift).
   - Option B: Reintroduce minimal schema hints in harness questions (contrary to the “no schema hints” preference).
     - Pros: strongest compliance signal.
     - Cons: violates the “no schema hints in user questions” requirement.
   - Recommendation: **A** (keeps questions clean while reinforcing schema compliance).
2) **“Latest realized ARR” stat output shape**
   - Option A: Keep **total-only** output (`realized_arr`) for the stat case; add a dedicated total-only template + routing in `neda/bigquery.ai`.
     - Pros: matches the “Realized ARR $” stat expectation; keeps breakdown separate.
     - Cons: adds another template and routing.
   - Option B: Switch the stat case to **breakdown + total** (align with existing realized ARR components stat template), and retire the total-only stat case.
     - Pros: fewer templates; aligns with existing prompt.
     - Cons: changes the current stat contract.
   - Recommendation: **A** (keeps stat total and breakdown as separate, explicit cases).
3) **Output key naming for mismatched KPIs**
   - Option A: Keep current schema keys (e.g., `discounted_arr`, `windows_nodes`, `churn_business_subs`, `to_be_realized`) and enforce alias usage in the prompt.
     - Pros: minimal code churn; aligns with existing harness comparisons.
     - Cons: keys are less natural; model must learn them.
   - Option B: Rename schema + SQL aliases to more descriptive keys (e.g., `business_arr_discount`, `windows_reachable_nodes`, `churned_business_subs`, `unrealized_arr`, `realization_date`).
     - Pros: more human‑readable; aligns with model’s natural outputs.
     - Cons: wider edits (SQL, schemas, comparators, prompt).
   - Recommendation: **A** for now; we can revisit renames after stabilizing.

### Decisions (2025-12-24) — chosen
- D18: Enforce schema compliance in prompt; additionally **embed the full JSON schema inside the user question** to reinforce it. (Costa: “1A but also include the entire schema in the question.”)
- D19: Switch “latest realized ARR” stat to **breakdown + total** (retire total-only stat behavior in routing/tests). (Costa: 2B)
- D20: Keep existing schema key names; enforce alias usage rather than renaming schemas. (Costa: 3A)
- D21: Increase `maxOutputTokens` to **32768** to accommodate 100-row outputs with long admin lists (Costa: “increase it to 32768”).
- D22: For the **top customers >= $2K ARR** list, return **only the first 3 admins per space** (comma-separated, deterministic order) to reduce output size. (Costa: “Return the first 3 admins per space.”)

### Decisions needed (2025-12-23) — new test case
1) **Source tables and fields**
   - Option A: Use `watch_towers.spaces_latest` + `watch_towers.spaces_asat_YYYYMMDD` for ARR + subscription fields; join to app DB for contacts.
     - Pros: aligns with existing KPI patterns; Cons: may need joins for primary contact and subscription dates.
   - Option B: Use `watch_towers.spaces_latest` only, if it already embeds owner/admin and subscription dates.
     - Pros: simpler, fewer joins; Cons: may be incomplete or missing contact fields.
   - Recommendation: **A** unless we confirm B has all required fields.
2) **Definition of “customer”**
   - Option A: Space = customer (1 space = 1 customer).
   - Option B: Stripe customer ID = customer (may span spaces).
   - Recommendation: **A** unless BI prefers Stripe-level consolidation.
3) **Nodes committed vs connected**
   - Option A: `committed_nodes` from subscription record; connected = `ae_reachable_nodes` from spaces snapshot.
   - Option B: Use reachable nodes for both if committed is not available.
   - Recommendation: **A** if committed field exists.
4) **Subscription start date source + renewal rule**
   - Option A: Use `spaces_latest.ca_cur_plan_start_date` as start date; renewal = +1 year if annual, +1 month if monthly.
     - Pros: typed DATE field; already in watch_towers; simple.
     - Cons: assumes it maps to subscription start exactly.
   - Option B: Use `space_active_subscriptions_latest.created_at` as start date; renewal = +1 year if annual, +1 month if monthly.
     - Pros: raw subscription record; likely closest to “start”.
     - Cons: stored as STRING; needs parsing.
   - Option C: Use `spaces_latest.ay_billing_period_start` as start date; renewal based on `space_active_subscriptions_latest.period`.
     - Pros: uses explicit billing period.
     - Cons: string parsing + more joins/logic.
   - Recommendation: **A** unless we see evidence it’s wrong.
5) **Primary contact display format**
   - Option A: Comma-separated admin **emails**.
   - Option B: Comma-separated `Name <email>` when name exists, otherwise email.
   - Recommendation: **B** (more readable, still deterministic).
6) **Paid plan classes for “customer” filter**
   - Option A: Include `Business`, `Homelab`, `Business_45d_monthly_newcomer`, `Homelab_45d_monthly_newcomer`.
   - Option B: Only `Business`, `Homelab` (exclude newcomer variants).
   - Recommendation: **A** (newcomer variants are still paid).

### Decisions locked (2025-12-23) — new test case
- D6: **Source tables**: use `watch_towers.spaces_latest` + app DB joins for contacts/subscription dates (not `spaces_latest` alone).
- D7: **Customer definition**: customer = space with **paid subscription**; a space with Stripe customer ID can still be **Community** and is **not** a customer.
- D8: **Nodes**: committed nodes from subscription; connected nodes from reachable nodes.
- D9: **Owner/primary contact**: `owner` = `admin`. A space may have multiple admins; all are owners. If schema requires a single field, return a **comma-separated list** of all admin contacts.
- D10: **Subscription start date**: use `watch_towers.spaces_latest.ca_cur_plan_start_date` (Option 1A).
- D11: **Missing period**: if subscription period is missing/blank, report `"unknown"` (do not infer).

### New requirement (2025-12-23)
- Prompt must explicitly teach **differences between overlapping data sources** and **rules for which source to use when**, so the model can answer arbitrary questions. The prompt should surface the alternatives (e.g., `spaces_latest` vs `spaces_asat_*` vs `spaceroom_space_active_subscriptions_*`) with a deterministic selection matrix, not hide them.

### Decisions needed (2025-12-23) — source comparison detail in prompt
1) **Where to place the source comparison rules**
   - Option A: Add a new “Source Selection Matrix” section near the top (after Domain Model).
     - Pros: highly visible; deterministic routing.
     - Cons: increases prompt size.
   - Option B: Expand “Entity Cheat Sheet” with per-entity source alternatives + rules.
     - Pros: keeps all entity info together.
     - Cons: less prominent for routing.
   - Recommendation: **A** (clearest for arbitrary questions).
2) **Depth of comparison**
   - Option A: Short table: *source → when to use → caveats* for each overlapping entity (spaces, subscriptions, members, nodes, ARR).
   - Option B: Short table + 2–3 concrete example questions per source.
   - Recommendation: **B** (reduces ambiguity).
3) **Conflict resolution rule**
   - Option A: “watch_towers wins” for ARR/plan/trial truth; raw app_db_replication only for identity/contacts/committed_nodes.
   - Option B: Allow overrides when raw data is fresher and the user explicitly asks for raw.
   - Recommendation: **A** unless you want explicit raw overrides.

### Decisions locked (2025-12-23) — source comparison detail in prompt
- D12: **Placement**: add a “Source Selection Matrix” near the top (after Domain Model).
- D13: **Depth**: include a short table + 2–3 example questions per source.
- D14: **Conflict rule**: `watch_towers` is authoritative for ARR/plan/trial truth; raw app_db_replication only for identity/contacts/committed_nodes unless the user explicitly asks for raw.

### Decisions locked (2025-12-24) — schema enforcement for stat vs timeseries
- D15: **Enforce single-value via schema for stat KPIs** (Option 2). Tests must use strict schemas so timeseries responses fail schema validation. For free-text (no schema), timeseries is acceptable; for schema runs, the model must reduce to a single latest value (not sum unless the question explicitly asks for a sum).
- D16: **Schema shape for stat KPIs**: use `{ data: <object>, notes: [], data_freshness: {...} }` (Option 1A). Avoid `data[]` + `maxItems: 1`.
- D17: **Data freshness field**: add `data_freshness { last_ingested_at, age_minutes, source_table }` to all schema responses to avoid embedding freshness in notes.

### Decisions locked (2025-12-26) — prompt consolidation (single pattern)
- D18: **Consolidate Temporal Data Routing** into a single canonical section (under Source Selection Matrix), transfer and enrich all routing rules there, and replace duplicated routing guidance elsewhere with short pointers. Goal: improve understanding + reduce prompt size via **surgical edits** only.
- D19: **Freshness consolidation**: keep the full freshness query + placement rules only in **0.3** (Non‑Negotiables) as the canonical source; replace **7.1** with a short pointer and update other sections to reference 0.3.

### Stress points roadmap (2025-12-26) — fix one-by-one (no re-runs of external reviewers)
Top 5 stress points (ranked by reliability risk + frequency):
1) **Date handling precedence conflicts** (explicit `to_date` vs `CURRENT_DATE()` vs “include today”).
2) **Stat vs timeseries + schema cap conflicts** (`maxItems:1`, LIMIT 1, post-filter vs template verbatim).
3) **Template verbatim vs adaptation rule tension** (when to adapt vs hard-stop).
4) **Freshness reporting vs schema constraints** (must include vs schema forbids).
5) **Manual baseline cutoff logic drift** (2025-10-01/02/04 rules across templates).

Planned fix order (one per iteration):
P-A: Consolidate and enrich **Date Handling Precedence** into a single canonical block; replace all conflicting rules with pointers. **(Done 2025-12-26)**
P-B: Consolidate **Stat vs Timeseries Decision** into a canonical block (schema-first precedence; eliminate contradictory wording). **(Done 2025-12-26)**
P-C: Consolidate **Template Usage Rule** into a clear decision tree (exact match → verbatim; otherwise adaptation rules with required disclosures). **(Done 2025-12-26)**
P-D: Consolidate **Freshness vs Schema** into an explicit precedence rule with strict fallbacks. **(Done 2025-12-26)**
P-E: Consolidate **Manual Baseline Logic** into a single section with explicit cutoff dates and template references. **(Done 2025-12-26)**

### Decisions needed (2025-12-26) — prompt consolidation (surgical)
1) **Freshness rule consolidation**
   - Option A: Keep the full freshness query + placement rules only in **0.3**; replace **7.1** with a one‑line pointer (“See 0.3”) and remove the duplicate SQL.
     - Pros: single source of truth; saves tokens.
     - Cons: one less visible SQL example in the “Query Patterns” section.
   - Option B: Keep **7.1** as the canonical location; shorten **0.3** to a brief mandate with a pointer.
     - Pros: keeps query examples grouped with patterns.
     - Cons: weakens Non‑Negotiables section.
   - Recommendation: **A** (non‑negotiables should remain canonical).
2) **Stat vs timeseries rule consolidation**
   - Option A: Make **Core execution rules** the canonical place; remove duplicate stat/series bullets from **Understanding user requests**, **Known pitfalls**, and **FAQ** (keep only template‑specific notes).
     - Pros: clear routing; fewer contradictions.
     - Cons: requires careful trimming across multiple sections.
   - Option B: Keep stat/series notes inside each template/FAQ and remove from Core rules.
     - Pros: local context near examples.
     - Cons: more repetition; higher drift risk.
   - Recommendation: **A** (single canonical rule + minimal template reminders).
3) **Source selection/temporal routing consolidation**
   - Option A: Keep **Source Selection Matrix** as canonical; trim overlapping rules from **Authority & Scope**, **Entity Cheat Sheet**, and **Decision Tree** to short pointers.
     - Pros: one deterministic routing table; fewer contradictions.
     - Cons: less self‑contained sections.
   - Option B: Keep **Authority & Scope** canonical; shrink Source Selection Matrix into short examples only.
     - Pros: authority hierarchy stays central.
     - Cons: reduces deterministic routing guidance.
   - Recommendation: **A** (matrix is the most explicit routing aid).
4) **Realized ARR rule consolidation**
   - Option A: Keep the **Canonical realized ARR** section as the single source; replace other mentions with 1‑line references.
     - Pros: reduces drift; shortens repeated formula text.
     - Cons: less “inline” explanation in other sections.
   - Option B: Keep short summary in Domain Model only; remove other references.
     - Pros: keeps business model readable.
     - Cons: loses operational rule location for templates.
   - Recommendation: **A** (operational rule should be canonical).

### Decisions needed (2025-12-26) — ARR routing for ambiguous “current ARR”
**Context:** The prompt has multiple valid ARR concepts (realized, business-only, total+unrealized, per-space). We need a deterministic default for ambiguous asks like “current ARR” with no qualifiers.
1) **Default definition for ambiguous “current ARR”**
   - Option A: **Realized ARR (portfolio)** via `realized_arr_components_stat_last_not_null` (metrics_daily).
     - Pros: aligns with KPI dashboards; already the canonical “realized ARR” definition; avoids per-space summation drift.
     - Cons: excludes unrealized (45d newcomers).
   - Option B: **Total ARR + unrealized** via `total_arr_plus_unrealized_arr_latest`.
     - Pros: includes newcomers; closer to “total potential ARR.”
     - Cons: differs from realized ARR dashboards; can surprise finance reporting.
   - Option C: **Per-space ARR total** via `spaces_latest.bq_arr_discount` (sum across spaces).
     - Pros: uses space-level truth; intuitive for “customer ARR.”
     - Cons: heavy query; not the KPI rollup; can diverge from metrics_daily.
   - Recommendation: **A** (most consistent with existing KPI definitions and reduces ambiguity).

### Decisions needed (2025-12-27) — template family centralization (explain‑why, not rigid rules)
**Context:** Extend the “single template + registry + rationale/pitfalls/joins” pattern beyond stat KPIs. Goal: reduce size while improving model understanding and safe improvisation.
1) **Which template family to centralize next (first pass)?**
   - Option A: **metrics_daily_timeseries_basic** (no date spine; per‑day MAX for asat metrics).  
     - Pros: lots of repetition; low risk; aligns with existing patterns.
     - Cons: still need a separate spine template for growth‑% and null‑preserving series.
   - Option B: **metrics_daily_timeseries_spine** (null‑preserving date spine).  
     - Pros: clarifies include‑today + missing dates; handles growth‑% prerequisites.
     - Cons: slightly higher risk; must be precise about when a spine is required.
   - Option C: **growth_pct_timeseries** registry (already canonical SQL, add registry + rationale/pitfalls).  
     - Pros: improves clarity without touching SQL; low risk.
     - Cons: smaller size reduction.
   - Option D: **snapshot mix/counts** templates (spaces/nodes segment counts & percents).  
     - Pros: reduces duplicated explanations; improves correctness for % math.
     - Cons: limited size reduction.
   - Recommendation: **A first**, then **B**, then **C**.
2) **Allowed operations per family**
   - Option A: Only **metric_name selection + aliases** (strict).  
     - Pros: safest; least ambiguity.
     - Cons: less improvisation.
   - Option B: Allow **simple arithmetic** across compatible metrics (sum/multipliers).  
     - Pros: supports more real‑world questions; aligns with “educate, not constrain”.
     - Cons: needs clear pitfalls/units guidance.
   - Recommendation: **B**, with explicit “compatible units only” guidance.

### Decisions locked (2025-12-27) — template family centralization order + freedom
- D20: **Order** for template family centralization: A → B → C → D (timeseries basic → timeseries spine → growth % registry → snapshot mix/counts).
- D21: **Allowed operations**: Option **B** (allow simple arithmetic across compatible metrics) with explicit unit‑compatibility guidance.

### Decisions needed (2025-12-24) — stat KPI schema shape
1) **Schema shape for single-value KPIs**
   - 1A: Keep root `{ data, notes }`, but make `data` a **single object** (not an array).
     - Pros: minimal disruption; keeps existing `notes`; aligns with “object of what you expect.”
     - Cons: requires comparator updates for stat cases.
   - 1B: Flatten to root object `{ ...fields, notes }` (no `data` wrapper).
     - Pros: simplest shape for strict single values.
     - Cons: bigger refactor (schemas, comparators, prompt contract, and callers).
   - Recommendation: **1A** to keep consistency with existing contract while removing array confusion.

### Decisions needed (2025-12-23) — prompt entity definitions
1) **Entity definitions to codify in the prompt**
   - Option A: Use the existing Netdata domain model plus the new “Customer” definition (paid spaces only) and primary-contact rule (owner/admin).
     - Pros: consistent with current prompt; minimal divergence.
     - Cons: requires careful edits to avoid conflicts with existing wording.
   - Option B: Replace the domain model section with a short, explicit “entity glossary + linking rules” table.
     - Pros: clearer and shorter for the model.
     - Cons: potential loss of context if we remove too much.
   - Recommendation: **B** (clearer, less ambiguous, easier to keep correct).
2) **Documentation sync for prompt changes**
   - Option A: Update `docs/AI-AGENT-GUIDE.md` (and any referenced prompt docs) in the same pass.
     - Pros: complies with doc-sync requirement; avoids drift.
     - Cons: extra edits now.
   - Option B: Defer doc updates until after the new test case lands.
     - Pros: fewer changes now.
     - Cons: violates doc-sync requirement; higher drift risk.
   - Recommendation: **A** (required by repo rules).
3) **Primary contact selection when a single value is required by schema**
   - Option A: If schema needs one field, return a **comma-separated list** of all admins (owners).
     - Pros: faithful to “owner = admin” and captures all owners.
     - Cons: field contains multiple values.
   - Option B: Return a single admin (deterministic ordering).
     - Pros: single value; stable.
     - Cons: drops other owners.
   - Recommendation: **A** per Costa.

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
- P12: After Costa chooses consolidation options, apply **surgical edits** to `neda/bigquery.ai` (remove duplicated rules, replace with short pointers).
- P13: Re-run `npm run lint` + `npm run build` after prompt edits; spot-check a small harness subset to ensure no behavioral regressions.

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
