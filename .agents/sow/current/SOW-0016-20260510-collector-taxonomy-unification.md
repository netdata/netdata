
# SOW-0016 - Unify collector metric taxonomy with Cloud-Frontend dashboard TOC

## Status

Status: in-progress

Sub-state: one-PR framework+POC scope locked by user on 2026-05-11 after the audit-phase start. SOW moved from `pending/` to `current/`; on 2026-05-14 the user superseded the "structural taxonomy only" v1 boundary and required v1 to cover full cloud-frontend TOC shapes, including grids, context/table widgets, ordered alternatives, and nested groups. The full-shape v1 contract was redesigned, adversarially reviewed, amended, and re-reviewed as READY TO IMPLEMENT. Implementation has resumed under the full-shape contract. Full collector taxonomy coverage, global all-collector fatality, production ibm.d sweep, and cloud-frontend consumption are follow-up work unless explicitly pulled into the POC by user decision. After each major implementation step, if an external Claude review would add value, provide a focused prompt with exact files and questions.

Implementation progress 2026-05-11: framework, schemas, generator/checker/seed tooling, CI wiring, docs/spec/skills updates, and five go.d POC collector taxonomies were implemented locally and committed as a framework POC snapshot. 2026-05-14: structural-only implementation was paused, full-shape v1 was redesigned/reviewed, and the framework/POC files were updated locally to the ordered recursive `items:` contract.

## Requirements

### Purpose

Eliminate cross-repo taxonomy drift between Netdata's public collector definitions and the private cloud-frontend dashboard TOC. Ownership moves next to collectors. Validation lives entirely in the public netdata repo. Cloud-frontend consumes a generated JSON artifact, exactly as it consumes `integrations.js` today.

**Scope of this SOW (Netdata-only)**: framework schemas, generator, validators, CI gates, seed tooling, contributor docs/skills, and a small POC set of collector `taxonomy.yaml` files. The published artifact `integrations/taxonomy.json` is the contract surface. Full collector taxonomy coverage is deliberately out of the initial PR.

**Out of scope**: cloud-frontend consumption work (consumer module, legacy taxonomy module removal, chart-spec extraction, `dynamicSections` removal, regex-catchall removal, rollback runbook). The FE team owns Phase B on their schedule; tracked as a downstream FE-team SOW.

### User Request

> The collector definitions and frontend taxonomy are disconnected. Currently:
> - Collectors define metrics and contexts in `metadata.yaml`
> - Cloud frontend separately defines taxonomy/TOC mappings in JS
> - There is no validation that taxonomy entries reference valid metrics/contexts
> - CI cannot validate consistency because:
>   - `netdata` repo is public
>   - `cloud-frontend` repo is private
>   - we cannot grant the public repo access to the private repo
> This causes taxonomy drift and broken references.
>
> Move taxonomy ownership closer to collectors. Each collector should define its own taxonomy in YAML near the collector itself (either embedded in `metadata.yaml`, or stored in a dedicated taxonomy YAML file). Then Python scripts should aggregate all collector taxonomy YAML files, produce one normalized JSON artifact. Cloud frontend CI will later consume this JSON and generate JS code (out of scope).

User refinements (2026-05-10):

> we dont care about less churn but clean end state. Lets do as much as possible now, without phase1/2 - i mean about the framework (creating taxonomy.yaml for each collector is routine work).

> about isSingleNode - this is important. Different view depends on the view, we need this in the taxonomy.

User refinement (2026-05-11):

> we will do everything in one PR (framework - w/o adding taxonomy for all collectors, can add a few as a POC).

User refinement (2026-05-14):

> There is no need to narrow the scope of v1, it should cover everything. Less churn is not our concern.

User decision (2026-05-14):

> Pause implementation, redesign the v1 full-TOC schema/contract first, then run adversarial review before implementation resumes.

User decision (2026-05-14):

> Cloud-frontend is not written in stone for this phase. SOW-0016 may define a clean Netdata-side taxonomy contract that requires downstream FE adapter/renderer changes, as long as those FE changes are clean and not compatibility hacks.

### Assistant Understanding

Facts (established from 3 independent Opus 4.7 analysis reports under `.local/audits/taxonomy-design/`):

- `cloud-frontend/src/domains/charts/toc/taxonomy/` is ~15,160 LoC across 18 JS files. It is a dashboard-composition DSL, not a flat taxonomy: it mixes section structure, context references, regex catchalls, function-typed entries (`({ isSingleNode }) => ...`), grids of pre-configured chart widgets (`type: "grid"`), and chart-spec bodies (`type: "context"`). Only ~5–10% of the LoC is pure structural taxonomy.
- The integrations marketplace axis (`integrations/categories.yaml` + `meta.monitored_instance.categories`) is **orthogonal** to the dashboard TOC: MySQL is `data-collection.databases` in the catalog and `Applications > MySQL` in the TOC. The two axes have different cardinality, different routing target (catalog page vs in-app dashboard), different consumers.
- The bridge token between collectors and the TOC is the chart context name (e.g. `mysql.queries`). Contexts are already declared in `metadata.yaml` under `metrics.scopes[*].metrics[*].name` (`integrations/schemas/collector.json:264-394`). This is the cross-reference key.
- The integrations pipeline (`integrations/gen_integrations.py`, 1469 LoC) discovers metadata.yaml files via `COLLECTOR_SOURCES` (`gen_integrations.py:27-37`), validates against JSON schemas (Draft7), and emits `integrations/integrations.{js,json}` plus per-integration markdown via Jinja templates. CI is `.github/workflows/generate-integrations.yml` and `check-markdown.yml`. Warnings become fatal in CI via `fail_on_warnings()` (`gen_integrations.py:155-174`).
- ibm.d collectors generate `metadata.yaml` from `contexts.yaml` via `go generate`. Embedding taxonomy into `metadata.yaml` would force generator-on-generator complexity. ibm.d is the structural reason to use a sibling file.
- All three independent analyses converged unanimously on: sibling-file design, cross-cutting parent registry, categories-vs-TOC are different axes, cross-reference validation as the load-bearing CI check, ICOn registry as a string-keyed allowlist with FE-side asset map.

Inferences (not directly stated):

- "Clean end state, no framework phasing" now applies to the framework PR only: the framework lands in one delivery with POC collector taxonomies. It does not imply a full initial taxonomy sweep for every collector.
- The `isSingleNode` requirement implies that view-conditional rendering exists at the section/structure level, not just inside chart-spec bodies. The schema must support this as a first-class concept.
- Superseded 2026-05-14: v1 is no longer limited to section/context taxonomy. The public contract must model full TOC shape where needed for parity: ordered items, structural groups, owned context leaves, flattening groups, selector leaves, grids, context/table widgets, first-available alternatives, and view-conditioned item bodies. `include_charts:` handles remain absent; the replacement is explicit typed item bodies in `taxonomy.yaml`.

Unknowns (resolve before the implementation step that depends on them):

- Whether function-typed entries appear ONLY inside chart-spec bodies, or also at section-structure level. If the latter, the schema needs richer condition expressions than `view: single_node | multi_node`.
- Whether `families: true` semantics depend on Netdata's `family` chart attribute (which may be deprecating in some flows). Maintainer confirmation needed before locking the schema.
- The exhaustive set of icon keys used across all 18 cloud-frontend taxonomy files (~175 from `icons.js`, but verify by grep).
- Whether `virtualContexts` ever appear at section-structure level (vs only inside chart-spec bodies). If at structure level, schema needs a `virtual:` opt-in.
- Whether ibm.d's `contexts.yaml` schema needs extension to carry canonical `section_id`, `priority`, and subsection/placement metadata for production codegen. This only blocks the initial PR if an ibm.d collector is selected as a POC.

### Acceptance Criteria

- Audit evidence required by a framework component exists before that component lands. There is no separate 10-output audit gate before implementation; unresolved design forks must not be hidden in code.
- **Full-shape redesign gate (added 2026-05-14)**: satisfied. The ordered-`items:` v1 authoring/output contract was written, amended after adversarial review, and re-reviewed. Draft/review artifact: `.local/audits/taxonomy-design/full-shape-v1-redesign.md`.
- `integrations/_common.py` extracted; existing `integrations/integrations.json` AND `integrations/integrations.js` are byte-identical before/after the refactor (verified by the `diff -u` baseline-copy procedure in the implementation plan, not by `git diff --exit-code`, because these outputs are gitignored ephemeral artifacts). **Gate-fatal.**
- `integrations/gen_taxonomy.py` exists, runs in CI, schema-validates every committed `taxonomy.yaml`, cross-references against `metadata.yaml` contexts, and emits `integrations/taxonomy.json`. Verified by green CI on the single implementation PR.
- `integrations/gen_taxonomy_seed.py` exists and seeds flat structural `items:` lists from `metadata.yaml`. Documented in the integrations contributor docs with the direct script command. **Initial PR hard requirement.**
- `integrations/check_collector_taxonomy.py` exists as a fresh wrapper around `_common.py` and taxonomy validators (NOT a clone of the stale `check_collector_metadata.py`).
- Output is **deterministic**: re-running `gen_taxonomy.py` 10× on identical input produces byte-identical `taxonomy.json`. Verified by golden test.
- Cross-reference validator catches every TAX invariant in v1 (TAX001–TAX025, TAX028–TAX038; TAX026/TAX027/TAX040–TAX042 removed with `only_views:` drop and chart-recipe-manifest removal). Lint-code matrix in spec doc lists every code with severity, example, remediation, and superseded codes.
- **Selector overlap detection**: across all three selector types (`contexts`, `context_prefix`, `collect_plugin`), pairwise intersection raises a fatal error.
- **Closed core schema (Decision 13)**: misspelled field names (e.g. `single-node`, `include_chart`) fail the schema validator. Verified by negative tests.
- Every collector whose `metadata.yaml` metrics block or `taxonomy.yaml` is touched in the implementation PR has matching taxonomy coverage in the same PR (fatal — Decision 12). Global all-collector coverage is informational/warning only in this SOW.
- `taxonomy_optout: { reason: "..." }` is a top-level per-collector authoring object in `taxonomy_collector.json`; it is mutually exclusive with `placements`, cannot appear inside a placement, and requires a non-empty reason. `inline_dynamic_declarations` may appear alongside `taxonomy_optout` only for no-metadata plugins documented by audit 1.6. `taxonomy_output.json` carries opt-out collectors in a separate `opted_out_collectors` array, not as empty placements.
- Every structural literal owner and widget literal reference resolves to a real declared context unless the exact widget reference carries the explicit unresolved-reference escape hatch (TAX003 fatal from day 1; TAX038 warns when an escape hatch becomes stale).
- Every `context_prefix:` is declared in `metadata.yaml.metrics.dynamic_context_prefixes:` of the owning collector (or inline in `taxonomy.yaml` for plugins without `metadata.yaml`); TAX031 fatal.
- Every `collect_plugin:` is declared in `metadata.yaml.metrics.dynamic_collect_plugins:` (or inline); TAX035 fatal.
- Every `context_prefix_exclude:` is paired with a `context_prefix:` and contains valid prefix strings; otherwise TAX029 fatal.
- Every `section_id` resolves against `integrations/taxonomy/sections.yaml`. `section_path:` authoring is rejected by the closed v1 schema. Verified by validator (fatal).
- Every `icon` is in `integrations/taxonomy/icons.yaml` allowlist. Verified by validator (fatal).
- ~~`include_charts:` validation~~ — **REPLACED 2026-05-14**: no `include_charts:` handle namespace in v1. Full-shape parity is represented directly by typed ordered `items:` entries such as `owned_context`, `group`, `flatten`, `selector`, `grid`, `context`, `first_available`, and `view_switch`.
- Production ibm.d `taxonomy.yaml` codegen is not required in the initial framework+POC PR unless an ibm.d collector is selected as a POC. If included, `go generate ./src/go/plugin/ibm.d/modules/... && git diff --exit-code` must be clean for the touched module(s).
- `integrations/taxonomy.json` matches `integrations/schemas/taxonomy_output.json` (self-validation in `gen_taxonomy.py`). Output schema version is `taxonomy_schema_version: 1`.
- Output JSON includes `source: { netdata_commit, generated_at }` metadata (Decision 3 amendment).
- Output JSON carries unresolved selectors, build-time-resolved owned-context snapshots, and display-reference snapshots per placement/item (Decision 2 amendment plus 2026-05-14 ownership/reference split).
- Legacy diff tooling and full drift triage are follow-up work for the full collector migration, not acceptance criteria for the initial framework+POC PR.
- Performance budget: full taxonomy validation completes in <5 seconds on the current fleet; synthetic 10K-context fixture completes in <10 seconds. Verified by CI timing.
- `Finding` model emits valid GitHub Actions annotations (`::error file=PATH,line=N,title=TAXNNN::MESSAGE`), text, JSON sidecar, and optional SARIF.
- Collector consistency policy includes `taxonomy.yaml` as the dashboard TOC placement artifact. Documented in `AGENTS.md` and `.agents/skills/integrations-lifecycle/`.
- **Cloud-frontend Phase B is NOT a SOW-0016 acceptance criterion** (out of scope per 2026-05-11 user clarification). Netdata-side SOW closes when the single framework+POC PR merges with green validation. FE-team Phase B is tracked separately on their schedule.

## Analysis

Sources checked:

- `/Users/ilyam/Projects/github/ilyam8/cloud-frontend/src/domains/charts/toc/taxonomy/` — 18 JS files, fully read by 3 independent agents.
- `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/plugin/go.d/collector/mysql/metadata.yaml` and ~5 other representative collectors (apache, postgres, nvidia_smi, snmp, db2/ibm.d).
- `/Users/ilyam/Projects/github/ilyam8/netdata/integrations/gen_integrations.py:1-1469` — full pipeline read.
- `/Users/ilyam/Projects/github/ilyam8/netdata/integrations/schemas/collector.json` — collector schema (627 lines).
- `/Users/ilyam/Projects/github/ilyam8/netdata/integrations/categories.yaml` — catalog axis.
- `/Users/ilyam/Projects/github/ilyam8/netdata/.github/workflows/generate-integrations.yml` and `check-markdown.yml`.
- `.agents/skills/integrations-lifecycle/` — current pipeline knowledge.
- `.agents/sow/specs/` — checked for prior taxonomy specs (none).
- 3 independent Opus 4.7 agent reports under `.local/audits/taxonomy-design/`.

Current state:

- Cloud-frontend taxonomy is hand-maintained JS, no validation, no cross-reference to collector contexts. Drift is invisible until a chart fails to render.
- `metadata.yaml` already declares the full set of contexts every collector emits (`metrics.scopes[*].metrics[*].name`). All cross-reference data needed by the new validator is already present in the public repo — no new data sources required.
- `gen_integrations.py` is the obvious plug-in point. Discovery, schema validation, warning-fatal-in-CI patterns exist and can be reused.
- `meta.monitored_instance.categories` is structurally separate from the TOC. No code in the cloud-frontend taxonomy references it.
- ibm.d's `contexts.yaml` codegen pattern (`go generate` emits `metadata.yaml`) extends naturally to also emit `taxonomy.yaml`.

Risks:

- **Function-typed entries (`isSingleNode` and similar)**: simple scalar/list deltas remain handled by the curated-and-override pattern (`single_node:` sparse block). Full item-body switches are now in scope for v1 via the proposed `type: view_switch` item; review must confirm this covers every current `({ isSingleNode }) => ...` occurrence without reintroducing `only_views:`.
- **Regex non-equivalence between JS and Python**: irrelevant since Decision 1 drops regex entirely. Risk eliminated.
- **`families: true` may depend on a deprecating attribute**: medium risk. Mitigation: confirm semantics before finalizing the schema; if `family` is being phased out, schema gets `group_by_label: <label>` as a sibling/replacement field before the generator/checker ships.
- **Catalog vs TOC contributor confusion**: low/medium risk. Mitigation: explicit guidance in `AGENTS.md` collector-consistency rule + integrations-lifecycle skill update; pre-commit lint flags suspicious `categories:` edits that look like TOC tweaks.
- **Full migration size**: ~150 collector `taxonomy.yaml` files remain out of the initial PR. Mitigation: the initial PR proves the framework and POC shapes; full migration gets its own follow-up SOW/PR plan.
- **Downstream FE consumption timing**: if the FE team consumes `taxonomy.json` later than the Netdata framework PR, the public repo temporarily publishes an unconsumed artifact. Acceptable — the framework is still useful for validation and later migration.
- **Coverage-fatal flip timing**: global all-collector fatality is deliberately deferred. The initial PR enforces changed/touched collector coverage only, so unrelated collector PRs are not blocked by missing taxonomy files.
- **Schema evolution**: future view axes (beyond `single_node | multi_node`) will require a schema bump. Mitigation: `taxonomy_version: 1` is pinned in every file; major bumps are explicit and gate-able. Core authoring fields use **closed schemas** (no broad `additionalProperties: true` for `single_node`, selector declarations, etc.); only namespaced extension keys (`x_*`) are permitted on core nodes (Decision 13).
- **Virtual contexts at section-structure level (factually present, not absent)**: `cloud-frontend/.../taxonomy/systemStorage.js:3-31` defines non-empty `virtualContexts`, and one is consumed in the taxonomy structure at `systemStorage.js:103`. The SOW's previous "confirm none" framing of audit step 1.4 is wrong. Mitigation: rewrite audit 1.4 to classify every `virtualContexts` def/use and assign one of {frontend recipe handle, encoded in generated contract, explicit diff exception, deferred with new SOW}.
- **`netdata.*` negative-lookahead selector cannot be expressed by the three-matcher set**: `cloud-frontend/.../taxonomy/netdata.js:79-83` uses `^netdata\.(?!(ebpf|statsd|apps|tcp_connects|tcp_connected|private_charts|machine_learning|training|metric_types|queue_ops|queue_size|plugin)).*`. A bare `context_prefix: ["netdata."]` would over-claim contexts that the cloud-frontend deliberately routes elsewhere. Mitigation: Decision 2 amended to require an explicit per-collector exclusion field (or static enumeration); regex remains forbidden.
- **First-available chart alternatives are in scope after the 2026-05-14 decision**: `cloud-frontend/.../charts/toc/getMenu.js:77-82` and `:91-98` execute "if item is array, choose first available context" semantics. Used in Kubernetes (`kubernetes.js:139-155, 183-198, 273-288, 337-352, 405-421, 445-461, 485-500, 527-538`), Containers/VMs (`containersAndVms.js:311-324, 347-365, 574-593, 616-674, 702-712`), and Pulsar grid (`applications.js:3902-4001`). Mitigation: v1 redesign adds a typed `first_available` item whose alternatives are fully validated against metadata and preserved for FE runtime selection.
- **`_collect_plugin` selector feasibility is unproven on the FE side**: Agent stores the label on RRDSETs (`src/database/rrdset-index-id.c:23-27`), but `getMenu.js:50-56` filters by chart id, not by labels. Mitigation: audit step 1.10 remains a non-blocking coordination note; Netdata can publish the selector contract, and FE may adapt or open a selector-replacement SOW.
- **Cloud-frontend JSON consumption is plausible but unproven (mitigation revised 2026-05-11)**: current FE consumes `integrations.js` via `cloud-frontend/.github/workflows/sync-to-s3.yaml:47-67`, not a taxonomy JSON. After scope correction (FE out of scope of SOW-0016), this is the FE team's responsibility — they extend their `sync-to-s3.yaml` to also run `gen_taxonomy.py` and copy `integrations/taxonomy.json` into their tree, exactly as they already do for `integrations.js`. SOW-0016 publishes the artifact and the `taxonomy_output.json` schema; consumption is downstream. If the FE team finds the consumption infeasible (audit 1.10 coordination response), that's a downstream FE-SOW design problem, not a blocker on SOW-0016.
- **FE consumption is non-trivial but downstream**: `applications.js` is 5,885 LoC, `kubernetes.js` 1,228, `systemStorage.js` 1,567, `systemHardware.js` 1,241, `containersAndVms.js` 1,165; `contexts.js` is 34,055 LoC. Existing FE tests cover overview-vs-single-node flavor selection (`getMenu.test.js:326-343`), menu ancestry (`:369-423`), regex sections (`:450-506`), grids (`:509-517`), and virtual contexts (`:520-528`). Mitigation: this SOW does not gate on FE refactor timing; FE Phase B is tracked separately.
- **Rollback after framework+POC PR can leave metadata declarations behind**: POC collector `metadata.yaml` may carry new `dynamic_*` declarations even if POC taxonomy files are reverted. Mitigation: Rollback Matrix records whether reverted `dynamic_*` fields are removed with the taxonomy revert or intentionally retained as accurate emission facts.
- **Full collector author cost is not "routine"**: at the postgres rate (70 explicit context lines), 150 collectors implies ~10,500 explicit context lines plus structure/overrides/comments. Mitigation: `gen_taxonomy_seed.py` is an initial PR hard requirement so follow-up migration is seed + human review, not hand authoring from scratch.
- **Stale `check_collector_metadata.py` reuse trap**: it imports `SINGLE_PATTERN`, `MULTI_PATTERN`, `SINGLE_VALIDATOR`, `MULTI_VALIDATOR` from `gen_integrations` (`integrations/check_collector_metadata.py:8-9`), but the current generator defines none of those symbols; it is not wired into any active workflow. Mitigation: Decision 4 amended; `check_collector_taxonomy.py` is fresh, not a clone.
- **Path-as-identity is brittle**: if a collector authored a path such as `[applications, postgres]`, moving postgres under an intermediate Databases section would mass-edit every collector taxonomy referencing it. Mitigation: Decision 8 locks `section_id` as an opaque stable handle; dots in an ID are namespace punctuation only and do not define parentage. Section moves are expressed in `sections.yaml` via `parent_id` changes, not by editing collector YAML; `taxonomy.json` carries both stable `id` and resolved `path`.
- **Selector lookup performance**: naive `[k for k in CONTEXT_INDEX if k.startswith(prefix)]` per-prefix per-placement is O(P·C·placements). Mitigation: validation algorithm specifies sorted-context bisect for prefix and an index for collect-plugin; synthetic 10K-context performance fixture added to acceptance criteria.

## Pre-Implementation Gate

Status: **SATISFIED 2026-05-14** after the amended full-shape v1 contract review cleared and the framework+POC implementation resumed. The prior structural-only gate was superseded; the active gate is now satisfied for the single framework+POC PR scope.

Problem / root-cause model:

- Two repositories (public netdata, private cloud-frontend) own complementary halves of the same taxonomy: collectors emit chart contexts in `metadata.yaml`; cloud-frontend renders them via hand-maintained JS modules under `domains/charts/toc/taxonomy/`. There is no machine-readable contract between the two halves, no CI gate that crosses the boundary, and no validation that taxonomy entries reference real contexts. Drift accumulates silently until a chart fails to render or a collector renames a context that some taxonomy file still references. The fix is structural: move the dashboard TOC taxonomy contract (sections, paths, contexts, families flag, icon keys, view-conditional rendering, and full-shape item bodies where needed for parity) into the public repo as collector-adjacent YAML; build a Python aggregator that fails CI on any cross-reference mismatch; emit a JSON artifact the cloud-frontend consumes.

Evidence reviewed:

- `cloud-frontend/src/domains/charts/toc/taxonomy/index.js:13-100` (root dashboards map), `applications.js` (5,885 LoC), `kubernetes.js`, `system.js`, `containersAndVms.js`, `netdata.js`, `icons.js` — all read in full by 3 agents.
- `netdata/integrations/gen_integrations.py:1-1469`, `integrations/schemas/collector.json:1-627`, `integrations/categories.yaml`.
- `netdata/src/go/plugin/go.d/collector/mysql/metadata.yaml` and 5+ other representative collectors.
- `.github/workflows/generate-integrations.yml`, `check-markdown.yml`.
- `.agents/skills/integrations-lifecycle/` — existing pipeline knowledge.
- 3 independent Opus 4.7 analysis reports: `.local/audits/taxonomy-design/agent-{1,2,3}-analysis.md`.
- 3 schema-readability persona reviews: `.local/audits/taxonomy-design/schema-review-{1,2,3}.md`.
- 7-collector empirical schema validation: `.local/audits/taxonomy-design/schema-validation-drafts.md`.
- External Codex review (6 parallel GPT-5.5-xhigh subagents + synthesis, 2026-05-10):
  - Synthesis: `.local/audits/taxonomy-design/external-review-codex/SYNTHESIS.md`
  - 01 Schema correctness: `.local/audits/taxonomy-design/external-review-codex/01-schema-correctness.md`
  - 02 Readability: `.local/audits/taxonomy-design/external-review-codex/02-readability.md`
  - 03 Pipeline: `.local/audits/taxonomy-design/external-review-codex/03-pipeline.md`
  - 04 Migration: `.local/audits/taxonomy-design/external-review-codex/04-migration.md`
  - 05 Cross-domain: `.local/audits/taxonomy-design/external-review-codex/05-cross-domain.md`
  - 06 Readiness: `.local/audits/taxonomy-design/external-review-codex/06-readiness.md`
- Direct file:line evidence cited in this gate (selected, non-exhaustive):
  - `cloud-frontend/.../taxonomy/index.js:13-62` — root dashboards map; `dynamicSections: true` for room and node.
  - `cloud-frontend/.../charts/toc/getMenu.js:50-75` — regex/test entries; `:77-82` and `:91-98` first-available array semantics; `:89` `isSingleNode` derivation; `:100-102` function-typed grid items; `:171-180` virtual-context rendering; `:238-350` family hierarchy; `:425-439` dynamic fallback for unmatched contexts.
  - `cloud-frontend/.../taxonomy/getMenu.test.js:326-343` overview/single-node flavor; `:369-423` menu ancestry; `:450-506` regex sections; `:509-517` grids; `:520-528` virtual contexts.
  - `cloud-frontend/.../taxonomy/systemStorage.js:3-31` non-empty `virtualContexts`; `:103` virtual-context referenced inside taxonomy structure.
  - `cloud-frontend/.../taxonomy/netdata.js:79-83` negative-lookahead regex over `netdata.*`.
  - `cloud-frontend/.../taxonomy/system.js:44-68, 117-168` and `systemMemory.js:35-80` and `remoteDevices.js:5-15, 93-131` and `containersAndVms.js:3-152, 311-365, 574-712` and `applications.js:3902-4001` (Pulsar) — function-typed entries and array alternatives.
  - `cloud-frontend/.github/workflows/sync-to-s3.yaml:47-67` — current FE artifact ingestion path; copies `integrations.js`, not taxonomy JSON.
  - `cloud-frontend/package.json:151-165` — no current taxonomy artifact import path.
  - `netdata/integrations/gen_integrations.py:27-37` `COLLECTOR_SOURCES`; `:155-174` warning-fatal; `:177-238` Draft7+Registry; `:326-339` collector globbing; `:379-407` `load_collectors`; `:822-830` ID synthesis; `:839-842` deterministic sort; `:861-996` mutating render; `:1414-1428` artifact emission; `:1431-1465` exit/fail handling.
  - `netdata/integrations/check_collector_metadata.py:8-9` — imports stale symbols from `gen_integrations`; not wired into active workflows.
  - `netdata/integrations/schemas/collector.json:264-395` `metrics` block; `:283-395` static metric context schema; no existing `additionalProperties: false`.
  - `netdata/.github/workflows/generate-integrations.yml:1-24, 47-66, 64-81` — current path triggers, generation, artifact cleanup.
  - `netdata/.github/workflows/check-markdown.yml:3-11` — current changed-file path triggers (no taxonomy paths).
  - `netdata/src/go/plugin/ibm.d/AGENTS.md:28-51` — generated files, source-of-truth files (`contexts.yaml`, `config.go`, `module.yaml`).
  - `netdata/src/go/plugin/ibm.d/docgen/main.go:27-47` — current `Context` struct (no taxonomy fields); `:119-153` and `:360-387` and `:389-529` generation flow.
  - `netdata/src/go/plugin/ibm.d/modules/db2/generate.go:1-3` and `modules/db2/contexts/doc.go:1-5` — module-level `go generate` invocations.
  - `netdata/src/collectors/statsd.plugin/example.conf:6-14, 30-43` — user-supplied app names and arbitrary user-defined chart contexts.
  - `netdata/src/collectors/statsd.plugin/statsd.c:1596-1628, 2254-2274` — `statsd.plugin` plugin label.
  - `netdata/src/database/rrdset-index-id.c:23-27` — `_collect_plugin` and `_collect_module` labels on RRDSETs.
  - `netdata/src/go/plugin/go.d/collector/mysql/metadata.yaml:1138-1149` — `mysql.galera_open_transactions` exists; no `mysql.open_transactions` (drift evidence vs `applications.js:1957`).
- No external open-source repositories were consulted as design references; cross-domain comparisons (Kubernetes CRDs, OpenAPI/JSON Schema, Prometheus relabel, OpenTelemetry semconv, VS Code/JetBrains marketplace) are documented in `.local/audits/taxonomy-design/external-review-codex/05-cross-domain.md` for context only.

Affected contracts and surfaces:

- **New schemas**: `integrations/schemas/taxonomy_collector.json`, `taxonomy_sections.json`, `taxonomy_output.json`.
- **Modified schema**: `integrations/schemas/collector.json` — adds two optional dynamic-context declarations under `metrics:`:
  - `dynamic_context_prefixes: [{prefix, reason}, ...]` — for collectors whose dynamic contexts share a namespace prefix (snmp, prometheus scraper, cgroup, apps).
  - `dynamic_collect_plugins: [{plugin, reason}, ...]` — for collectors whose dynamic contexts have no shared prefix (statsd.plugin, charts.d.plugin, python.d.plugin).
- **New runtime artifacts**: `integrations/taxonomy/sections.yaml`, `integrations/taxonomy/icons.yaml`, `integrations/taxonomy.json` (emitted; ephemeral gitignored per Decision 3 — matches `integrations.js` precedent).
- **New code**: `integrations/_common.py` (narrow shared extract — see the implementation plan for the explicit allow-list), `integrations/gen_taxonomy.py`, `integrations/gen_taxonomy_seed.py` (Decision 9), `integrations/check_collector_taxonomy.py` (fresh; not a clone of the stale `check_collector_metadata.py`, see Decision 4 amendment).
- **Modified**: `integrations/gen_integrations.py` (refactored to use `_common.py`; byte-identical output requirement is acceptance-gate-fatal), `.github/workflows/generate-integrations.yml` (new step + path triggers including `**/taxonomy.yaml`, `integrations/taxonomy/**`, `integrations/schemas/taxonomy*.json`, `integrations/gen_taxonomy*.py`, `integrations/_common.py`), `check-markdown.yml` (changed-collector taxonomy gate per Decision 12).
- **New per-collector files in this PR**: a small POC set of `<collector>/taxonomy.yaml` files. Full collector coverage is follow-up work.
- **ibm.d framework**: production `contexts.yaml` schema/codegen extension is follow-up unless an ibm.d collector is selected as a POC.
- **Cloud-frontend (OUT OF SCOPE of SOW-0016)**: FE team owns consumption — JSON consumer module, legacy taxonomy module removal, renderer adapter, `dynamicSections` removal, regex-catchall removal, rollback. Tracked as a downstream FE-team SOW. SOW-0016 publishes `integrations/taxonomy.json` and the `taxonomy_output.json` schema; that is the entire Netdata-FE contract surface.
- **Project policy**: `AGENTS.md` collector-consistency rule includes `taxonomy.yaml`.
- **Skills**: `integrations-lifecycle/` updated; `project-writing-collectors/` updated.
- **Docs**: collector contributor docs reference `taxonomy.yaml` as a new required file.

Existing patterns to reuse:

- `gen_integrations.py:177-238` — Draft7Validator + Registry pattern.
- `gen_integrations.py:155-174` — `WARNINGS` accumulator + `fail_on_warnings()` for CI-fatal gating; **wrap into a structured `Finding` model with multiple renderers** (text, valid GitHub Actions annotation `::error file=...,line=...,title=TAX003::`, JSON, optional SARIF) — the existing prefix `:warning file=...:` and `:error file=...:` strings are NOT valid GitHub Actions annotations and must not be propagated.
- `gen_integrations.py:326-339` — collector-source globbing.
- `gen_integrations.py:822-830` — collector-id synthesis pattern; reused so taxonomy and integrations agree on collector identity.
- `gen_integrations.py:839-842` — explicit deterministic sort (`_index`, `_src_path`, `id`); taxonomy needs an equivalent locked merge key (Decision 12).
- `integrations/categories.yaml` — frozen-list registry pattern (mirror for `sections.yaml`).
- ibm.d `go generate` codegen pattern remains the production target, but only enters this PR if an ibm.d POC is selected.
- **Do NOT mirror `integrations/check_collector_metadata.py`**: it imports symbols (`SINGLE_PATTERN`, `MULTI_PATTERN`, `SINGLE_VALIDATOR`, `MULTI_VALIDATOR`) the current `gen_integrations.py` no longer defines and is not invoked by any active workflow. Build `check_collector_taxonomy.py` fresh against `_common.py` and the new taxonomy validators.

Risk and blast radius:

- **Regression**: existing `gen_integrations.py` is refactored to use `_common.py`. Risk: introducing a regression in the existing pipeline. Mitigation: refactor as pure code-motion (no behavior change); diff the rendered `integrations.json` before/after to confirm byte-identical output.
- **CI runtime**: new pipeline adds another pass. Estimated <5s. Negligible.
- **Compatibility**: `taxonomy.json` is a new artifact; consumers (cloud-frontend) opt in. No breakage of existing artifacts.
- **Performance**: aggregator builds an in-memory context index (~10K entries max). Linear in collector count; well within budget.
- **Security**: no secret-handling involved; all data is public collector metadata.
- **Data loss**: zero. New artifacts; no destructive edits.
- **Migration**: initial PR touches only POC collectors. Full migration of ~150 collectors is follow-up work once the framework shape is proven.
- **Rollout**: one framework+POC PR is reversible by revert. FE Phase B is downstream and outside this SOW.
- **Operational**: zero impact on running agents. Taxonomy is build-time metadata, not runtime.

Sensitive data handling plan:

- This work involves zero credentials, secrets, customer data, or private endpoints. All inputs (`metadata.yaml`, cloud-frontend taxonomy JS) are non-sensitive structural metadata. All outputs (schemas, YAML files, generated JSON) are non-sensitive structural metadata. No redaction required in the SOW, specs, skills, code comments, or commits.
- The cloud-frontend repo is private but its taxonomy source files are not sensitive in content; they merely live in a private repo for org-policy reasons. Downstream FE Phase B work is referenced only at a high level; no private source from that repo will be pasted into public artifacts beyond structural references already cited in this SOW.

Implementation plan (active, 2026-05-11 one-PR framework+POC scope):

1. **Inline audit evidence before dependent code**: produce the minimum audit outputs needed by the next implementation step under `.local/audits/taxonomy-design/audit/`. The implementation must not encode unresolved `to-decide` rows. Audit 1.10 remains a non-blocking FE coordination note. Audit 1.5 is required only if production ibm.d codegen or an ibm.d POC enters this PR.
2. **Refactor shared integration helpers**: extract the narrow `_common.py` allow-list from `gen_integrations.py`; prove `integrations.{json,js}` are byte-identical before/after using `diff -u` against `.local/audits/taxonomy-design/refactor-baseline/` and `.local/audits/taxonomy-design/refactor-after/`.
3. **Add taxonomy schemas and registries**: author `taxonomy_collector.json`, `taxonomy_sections.json`, `taxonomy_output.json`, `collector.json` dynamic declaration extensions, `integrations/taxonomy/sections.yaml`, and `integrations/taxonomy/icons.yaml`. Use `section_id:` as the only v1 authoring form.
4. **Add generator/checker/seed tooling**: implement `gen_taxonomy.py`, `gen_taxonomy_seed.py`, and fresh `check_collector_taxonomy.py`; include deterministic ordering, selector overlap detection, schema self-validation, structured `Finding` renderers, and a 10K-context performance fixture.
5. **Add POC collector taxonomies**: add a small representative POC set, defaulting to the previously selected reference collectors `mysql`, `postgres`, `apache`, `nvidia_smi`, and `snmp` unless implementation evidence shows one should be swapped. Do not add taxonomy for all collectors in this PR.
6. **Wire CI for this scope**: `check-markdown.yml` is the PR-blocking gate; `generate-integrations.yml` is the post-merge artifact-generation path. Changed/touched collector coverage is fatal; global all-collector coverage remains warning/informational.
7. **Update contributor-facing artifacts**: update `AGENTS.md`, `.agents/skills/integrations-lifecycle/`, `.agents/skills/project-writing-collectors/`, `.agents/sow/specs/taxonomy.md`, and an integrations contributor doc.
8. **Review checkpoints**: after each major step, decide whether an external Claude review is useful. If yes, provide the user a focused prompt with exact files and questions.
9. **Close this SOW**: SOW-0016 completes when the single framework+POC PR merges green and the SOW validation/artifact gates are filled. Full collector migration, production ibm.d sweep, global fatal coverage, drift triage, and FE consumption become follow-up SOWs unless explicitly pulled into this PR.

Historical superseded PR-A1 / PR-A1.5 / PR-A2 implementation details were removed from the active plan on 2026-05-11 after the user locked the one-PR framework+POC scope. The reasoning remains in the Execution Log for provenance.

### Drift Triage Process (Follow-Up Full Migration)

Full legacy drift triage is not part of the initial framework+POC PR. When the follow-up full migration runs `diff_legacy.py`, each finding uses this disposition model:

| Disposition | When | Action | Owner | Output |
|---|---|---|---|---|
| `drop-frontend-entry` | Legacy referenced a context that doesn't exist in any `metadata.yaml` | Omit from new taxonomy | Collector maintainer signs off | None — finding closed |
| `fix-metadata` | Collector emits the context but `metadata.yaml` doesn't declare it | Add metric entry to `metadata.yaml` in the same sub-PR | Collector maintainer | Metadata diff |
| `fix-collector` | Context renamed/removed at the collector | Restore or rename in source + emit + metadata | Collector maintainer | Collector + metadata diff |
| `keep-frontend-only` | Entry is a `virtualContexts`-derived chart or other FE-only construct per audit 1.4 | Excluded from `taxonomy.json`; recorded so future diffs ignore it | FE owner notified | Row in `integrations/taxonomy/diff_exceptions.yaml` |
| `defer-with-sow` | Out of scope for the full-migration SOW | New SOW required before that SOW closes | Assigned during triage | New SOW filename |

Bulk migration implementer does NOT silently decide product semantics. Every finding has a named owner; the full-migration SOW cannot close while any finding is `to-decide`.

**Escalation clock (NEW 2026-05-11 per readiness reviewer 1)**: each finding's named owner has **5 business days** from notification to either sign off on the proposed disposition or push back with an alternative. After 5 business days without response, the bulk-migration implementer escalates to the user (project lead) for tie-break. This prevents the full-migration SOW from stalling indefinitely on owner PTO or unresponsiveness. Notifications are recorded in the drift inventory row with a date stamp.

### Rollback Matrix (Netdata-only)

| Phase | What's installed | What stays after revert | Required follow-up reverts | CI mode after revert |
|---|---|---|---|---|
| Single framework+POC PR | `_common.py`, gen_taxonomy, schemas, sections.yaml, icons.yaml, seed/check tooling, CI wiring, docs/skills/specs, POC collector taxonomies | Nothing if fully reverted; `gen_integrations.py` returns to pre-refactor state via the same revert | Remove any POC `metadata.yaml.metrics.dynamic_*` declarations if the corresponding POC taxonomy is reverted and the declarations are not intentionally retained as emission facts | Pre-taxonomy-framework state |
| Follow-up full migration | Remaining collector `taxonomy.yaml` files; possible no-metadata plugin handling; global all-collector fatality | If reverted, collector `metadata.yaml` extensions may become **zombie fields** unless explicitly removed in the same revert | Coordinated revert of `metadata.yaml` `dynamic_*` fields or explicit decision to retain them as accurate emission facts | Framework+POC state |

Cloud-frontend rollback is the FE team's responsibility (Phase B out of scope of SOW-0016).

### Audit Outputs (Support Evidence, Not A Separate Gate)

These outputs remain the evidence checklist. Produce each before the implementation step that consumes it; no separate PR-A1 gate exists after the 2026-05-11 one-PR scope correction.

- `1.1-dsl-inventory.md` — structured taxonomy-DSL inventory.
- `1.2-families-semantics.md` — `families: true` contract from FE consumer + tests.
- `1.3-icons-allowlist.md` — canonical icon-key list.
- `1.4-virtualcontexts.md` — `virtualContexts` classification + dispositions.
- `1.5-ibmd-prototype.md` — `db2` prototype + round-trip evidence (required only before production ibm.d codegen or an ibm.d POC enters scope).
- `1.6-dynamic-collectors.md` — prefix-friendly vs plugin-label-friendly + no-metadata plugin dispositions.
- `1.8-netdata-negative-selector.md` — chosen disposition for `netdata.js:79-83`.
- `1.10-collect-plugin-question.md` — coordination note to FE team (non-blocking).
- `1.11-section-id-map.md` — camelCase → kebab-case section ID map (top-level only).
- `1.11b-sections-tree.md` — full sections.yaml draft (~80–150 entries with parent_id, section_order). NEW 2026-05-11.

Total: 10 audit outputs (was 9 before reviewer pass surfaced the 1.11.b need), now consumed inline rather than as a standalone kickoff gate.

**Removed 2026-05-11** (FE work out of scope of SOW-0016):
- ~~`1.7-fe-adapter-spike.md`~~ — FE team's responsibility.
- ~~`1.9-chart-recipes-seed.md`~~ — no chart-recipe manifest in v1 (Decision 14 removed).

If an implementation step depends on an audit output, that output must exist and contain no unresolved `to-decide` rows before the step lands. Audit 1.10 is non-blocking; we proceed with `collect_plugin:` as the v1 selector unless the FE team raises a structural objection in time to change this PR.

Validation plan:

- **Unit / integration tests**:
  - `gen_taxonomy.py` runs against fixture `taxonomy.yaml` files; expected JSON output is golden-tested AND deterministic across runs.
  - Cross-reference validator: positive test (valid taxonomy passes), negative tests for active validation families including TAX003 (unknown context), TAX021 (unknown view-override key), TAX022 (`multi_node:` block), TAX023 (list-merge attempt), TAX024 (empty `single_node:`), TAX031/TAX035 (selector not declared in metadata), TAX036 (selector ownership overlap), TAX037 (referenced literal context has no owner), and TAX038 (unresolved escape hatch is now stale because the context resolves). TAX002 and TAX032 are reserved follow-up codes, not emitted in the framework+POC PR.
  - Selector overlap validator: positive and negative tests for cross-type overlap (static `contexts:` claimed by one collector AND `context_prefix:` matching the same context claimed by another → fatal).
  - Schema validator: positive AND negative tests for each schema file, including typo cases (`single-node` vs `single_node`, `include_chart` vs `include_charts`, etc.) — each must fail under the closed core schema (Decision 13).
  - Performance: synthetic 10K-context fixture must validate in <10 seconds.
  - Deterministic merge: re-run `gen_taxonomy.py` 10× on the same input; output must be byte-identical.
  - `_common.py` refactor: byte-identical `integrations.json` AND `integrations.js` before/after via the Plan step 2.2 `diff -u` baseline-copy procedure. **Gate-fatal.**
  - `Finding` renderers: text format, GitHub Actions annotation format (must match `::error file=PATH,line=N,title=TAXNNN::MESSAGE` regex), JSON sidecar shape, optional SARIF.
  - ibm.d round-trip: required only if production ibm.d codegen or an ibm.d POC enters this PR; touched generated outputs must be clean after `go generate`.
  - Drift triage: follow-up full migration requirement, not required for the initial framework+POC PR.
- **Real-use evidence**:
  - Single framework+POC PR lands; CI green; `integrations/taxonomy.json` is produced locally/CI and validates against `taxonomy_output.json`.
  - POC collectors validate when `gen_taxonomy.py` runs in CI; their `taxonomy.json` slices validate against `taxonomy_output.json`. FE-side rendering verification is FE-team's responsibility (downstream SOW).
  - Changed/touched collector coverage is fatal; global all-collector coverage remains warning/informational.
  - ibm.d round-trip evidence captured only if an ibm.d POC/codegen change is included.
- **Reviewer findings**: address all maintainer review comments on the single implementation PR. Apply the per-thread iteration discipline from `.agents/skills/pr-reviews/`.
- **Same-failure search**: grep for any other place in either repo that hand-maintains a taxonomy-shaped data structure (e.g. `dashboards.json`, `menu.json`, `routes.json`); confirm none exist or document each.

Artifact impact plan:

- **AGENTS.md**: updated. Collector-consistency rule extended to include `taxonomy.yaml`. Project skills index updated to reference taxonomy work.
- **Runtime project skills**:
  - `.agents/skills/integrations-lifecycle/` — major update. New section on taxonomy pipeline; pipeline.md, schema-reference.md, per-type-matrix.md, in-app-contract.md all touched.
  - `.agents/skills/project-writing-collectors/` — adds `taxonomy.yaml` as a required artifact for new collectors.
- **Specs**: new `.agents/sow/specs/taxonomy.md` describing schema, validation rules, JSON output contract, frozen v1 top-level list, condition vocabulary.
- **End-user/operator docs**:
  - `integrations/README.md` (or equivalent) — documents `taxonomy.yaml` for collector contributors.
  - Per-collector READMEs — no individual update needed; the new file is mentioned in the collector consistency rule docs.
- **End-user/operator skills**: `docs/netdata-ai/skills/` — no direct impact (skills don't consume the TOC).
- **SOW lifecycle**: this SOW has moved to `.agents/sow/current/` and will move to `.agents/sow/done/` on completion. Status transitions: open → in-progress → completed. No PR split is planned for this SOW after the 2026-05-11 one-PR correction.

Open-source reference evidence:

- This work is internal to Netdata's own repositories. No external open-source projects were consulted as design references. The `cloud-frontend` repository is a private Netdata repository, not an open-source dependency.

Open decisions:

ALL RESOLVED 2026-05-11. Recorded for traceability:

1. **Section identity model**: **8.A locked** — stable `section_id` first-class in `sections.yaml`; collector taxonomy references `section_id:` directly; `section_path:` is not accepted in v1. Section IDs are opaque immutable handles; dots are namespace punctuation only. Section moves are `parent_id` edits in `sections.yaml`; collector YAML unaffected.
2. **`only_views:` v1 inclusion**: **DROPPED from v1** per user — "I don't think we need only_views". If audit 1.1 finds an S2 case (whole-section visibility gating), that case spawns a follow-up SOW to add `only_views:` later. Does not block the initial framework+POC PR.
3. **Drift triage owner assignment**: **A locked** — bulk-migration implementer assigns owners themselves based on finding type; named owners review and sign off. Pre-soliciting per finding rejected as overhead.

Tactical items resolved without user input (recorded as defaults):

- `taxonomy.json` artifact: gitignored ephemeral, generated in CI, consumed by FE-team's pipeline exactly as `integrations.js` is consumed today (Decision 3 correction).
- Superseded 2026-05-14: no `include_charts:` or chart-recipe manifest in v1, but explicit full-shape typed `items:` are now in scope for v1 parity (Decision 5 reopened by user).
- Phase B (FE switchover) is out of scope; FE team owns consumption on their schedule.
- ibm.d production codegen is follow-up unless an ibm.d collector is selected as a POC.
- Full collector taxonomy coverage and global-all-collector fatality are follow-up work; per-changed-collector coverage is fatal in the implementation PR (Decision 12).
- Icon-key naming: kebab-case in YAML to match `categories.yaml` style.
- No cloud-frontend freeze imposed (FE team owns their repo); any legacy snapshot pinning belongs to the future full-migration/drift-triage SOW.

## Implications And Decisions

User decisions locked on 2026-05-10:

1. **Sibling vs embed**: sibling `taxonomy.yaml` next to `metadata.yaml`. Reason: ibm.d generator-on-generator avoidance; audience separation; file-size pragmatics. All 3 independent agents converged.
2. **Matcher and reference policy (AMENDED 2026-05-14 for full-shape `items:`)**: drop regex entirely. Context ownership and display references are explicit item semantics.
   - **Structural literal owners**: a plain string in a structural `items:` array, or `type: owned_context` with `context:`, owns exactly one context. Every owned literal must resolve to the owning collector metadata (TAX003).
   - **Display references**: `type: context` widgets carry `contexts:` arrays. These references do not own contexts; each literal must resolve to metadata or carry the explicit `unresolved: {reason, owner, expires}` escape hatch. A resolved literal reference with no structural owner anywhere is TAX037.
   - **Selector items**: `type: selector` owns the contexts matched by `context_prefix:` or `collect_plugin:`. Prefix/plugin selectors still require `metadata.yaml.metrics.dynamic_context_prefixes:` or `metadata.yaml.metrics.dynamic_collect_plugins:` declarations. TAX031/TAX035 fire without opt-in.
   - **Selector objects inside widgets**: widget `contexts:` arrays may include `{context_prefix: [...]}` or `{collect_plugin: [...]}` selector objects. These reference contexts but do not own them.
   - **`context_prefix_exclude:`** is valid only alongside `context_prefix:` on the same item/reference; invalid pairings are TAX029.
   - **Overlap detection**: duplicate ownership between non-selector owners is TAX033. Ownership overlap involving a selector is TAX036. Referencing an already-owned context is expected and valid.
   - **Resolved/reference snapshots**: every generated placement and item carries `resolved_contexts` (owned contexts) and `referenced_contexts` (display references). For dynamic-context collectors, selector snapshots include only statically-known contexts; runtime selectors cover future emitted contexts.
   - **No-metadata collector handling**: a collector without `metadata.yaml` may declare dynamic opt-ins under top-level `inline_dynamic_declarations:`. The validator treats those declarations as equivalent to `metadata.yaml.metrics.*`; if sibling `metadata.yaml` exists, inline declarations are TAX029.
   - **Frontend label-access coordination**: audit 1.10 records whether `_collect_plugin` is already reachable from the FE chart-selection data path. If not, SOW-0016 still publishes `collect_plugin:`; FE consumption either exposes the label downstream or opens a future selector-replacement SOW.
   - The guardrail preserves drift-elimination: static collectors (mysql, postgres, ...) use structural literal ownership and explicit widgets, not broad dynamic selectors.
   - Cross-engine portability concerns are eliminated by string-prefix-only and label-equality semantics (no JS/Python regex divergence).
3. **Output artifact + lifecycle (CORRECTED 2026-05-11 to match existing `integrations.js` precedent; earlier vendoring framing was over-engineered)**: separate `integrations/taxonomy.json`, **gitignored ephemeral**, generated by `gen_taxonomy.py` in netdata CI, consumed by cloud-frontend exactly as `integrations.js` is consumed today (`cloud-frontend/.github/workflows/sync-to-s3.yaml:47-67`).
   - **Generation policy**: `gen_taxonomy.py` writes `integrations/taxonomy.json` during CI; `.github/workflows/generate-integrations.yml` cleanup step removes it together with `integrations.{js,json}` (matches existing pipeline at `:64-66`). Local generation for inspection is supported. The file is `.gitignore`d.
   - **Versioning (contract-discipline IN the artifact, not in the delivery mechanism)**: output JSON carries `taxonomy_schema_version: 1` at root level (separate from per-file `taxonomy_version: 1` which is the input schema version). The FE consumer is expected to fail its build on unsupported `taxonomy_schema_version` or unknown required fields. This closes the gap that exists for `integrations.js` today (`.agents/skills/integrations-lifecycle/in-app-contract.md:117-120`) without changing the delivery mechanism.
   - **Compatibility policy**: within v1, additive optional fields are non-breaking; field removal is forbidden; enum value additions are reviewed; selector semantics, override merge semantics, list-replacement behavior, section path/ID identity, and generated JSON field names are FROZEN. Changing any of those bumps `taxonomy_schema_version`.
   - **Deprecation fields**: every section and selector type may carry `status: active|deprecated`, `deprecation: { replacement_id, since, removal_in }`. Consumers warn on deprecated entries; remove-after window is one major schema version.
   - **Source commit metadata**: every emitted `taxonomy.json` includes `source: { netdata_commit: <sha>, generated_at: <iso8601> }` for traceability. Whether the FE pins to a specific commit is the FE team's choice, not this SOW's contract.
   - **Cloud-frontend consumption**: out of scope of SOW-0016; tracked as a downstream FE-team SOW. This SOW publishes the artifact and the contract; consumption is the FE team's responsibility.
   - Reason for the correction: the existing `integrations.js` model is proven, well-understood, and operationally simple. The version-discipline concerns Codex subagents 04/05 raised are addressed by `taxonomy_schema_version` IN the output JSON, regardless of delivery mechanism. Inventing a separate vendoring strategy traded simplicity for theoretical robustness the artifact metadata already provides.
4. **Pipeline (AMENDED per Change 10)**: new `integrations/gen_taxonomy.py` + a NARROWLY scoped extracted `integrations/_common.py` (allow-list in the active implementation plan). `check_collector_taxonomy.py` is fresh, NOT a clone of the stale `check_collector_metadata.py`. Byte-identical `integrations.{js,json}` output before/after the `_common.py` refactor is a gate-fatal acceptance criterion.
5. **Full-shape TOC item contract in v1 (REOPENED 2026-05-14 by user decision)**:
   - `taxonomy.yaml` v1 must represent the full cloud-frontend TOC shape needed for parity: ordered `items:`, structural groups, explicit `owned_context` leaves, flattening groups for legacy `justGroup` semantics, selector leaves, grids, context/table widgets, first-available alternatives, and view-conditioned item bodies.
   - `include_charts:` remains absent. v1 does not use opaque chart-handle references or a chart-recipe manifest; it carries explicit typed item bodies where the legacy FE taxonomy carries explicit widget bodies.
   - Cloud-frontend consumption remains downstream/out of scope. The FE team owns the renderer adapter and legacy taxonomy removal. The current FE code is evidence for semantics, not a frozen object-shape contract; the Netdata artifact may require clean downstream FE changes.
   - Ownership and display references are separate: structural `owned_context` and `selector` items own contexts; `context` widgets, grids, alternatives, and view-switch widget bodies reference contexts and must validate them without tripping duplicate-ownership checks.
   - Widget `contexts:` arrays may contain literal context strings, explicit unresolved-reference objects with reason/owner/expiry, or selector objects (`context_prefix`, `context_prefix_exclude`, `collect_plugin`). Selector objects inside widgets reference contexts; selector items under `items:` own contexts.
   - TAX003 and TAX037 are fatal by default. Intentional staged/legacy unresolved references require the explicit unresolved-reference escape hatch; warning-by-default drift is rejected. TAX036 remains reserved for existing selector-overlap conflicts.
   - TAX038 warns when an unresolved-reference escape hatch has become stale because the context now resolves in metadata.
   - Renderer-private payloads are fenced under `renderer:`. Core item objects remain closed to preserve typo detection; known renderer keys are `overlays`, `url_options`, and `toolbox_elements`, and future renderer-only additions use `x_*`.
   - String shorthand is allowed only in structural positions (`placement.items`, `group.items`, `flatten.items`). It is rejected inside `grid.items`, `first_available.items`, and `view_switch` branches.
   - `flatten` is rejected inside `view_switch` branches; `first_available` alternatives are display-only object items and cannot own contexts.
   - The executable schema contract must include a per-type closed-field matrix and recursion matrix equivalent to the amended design artifact.
   - Reason: user directive 2026-05-14 — "There is no need to narrow the scope of v1, it should cover everything. Less churn is not our concern."
6. **ibm.d**: production codegen remains the target architecture, but after the 2026-05-11 one-PR scope correction it is not required in the initial framework+POC PR unless an ibm.d collector is selected as a POC. Reason: full ibm.d generation belongs with full collector coverage, not with the minimal framework proof.
7. **`dynamicSections`**: legacy FE root fallback, not a collector `taxonomy.yaml` field. If downstream FE needs dynamic fallback during migration, the clean Netdata-side home is a generated root/section option derived from `sections.yaml`; the FE team still owns the renderer behavior and removal timing. No Netdata-side feature flag.
8. **Frozen v1 top-level sections + stable section identity (LOCKED 8.A on 2026-05-11)**: top-level frozen list `system, kubernetes, containers-vms, synthetic-checks, remote-devices, otel, azure-monitor, applications, netdata` (mirrors `cloud-frontend/.../taxonomy/index.js:25-35` per audit 1.11 spelling map). Adding a new top-level requires a PR on `sections.yaml`.
   - **Identity model: 8.A (locked by user 2026-05-11)**. Stable `section_id` is first-class in `sections.yaml`. Collector `taxonomy.yaml` references `section_id:` directly. **`section_id:` is the canonical and ONLY accepted authoring form in v1.** `section_path:` as a list-of-segments is NOT accepted in v1 schema — closed schema rejects it (`additionalProperties: false`). Rationale: two authoring forms create a typo/ambiguity surface; one canonical form keeps `gen_taxonomy_seed.py` output deterministic and reviews easy. Output `taxonomy.json` carries BOTH stable `id` and a generator-resolved dotted `path` for FE convenience. If contributor demand emerges for path-style authoring later, a follow-up SOW must design that alternate input shape explicitly.
   - **ID semantics**: `section_id` is an opaque immutable handle. Dots in IDs are allowed for readability and namespace grouping, but they do not define parentage and are not recomputed when a section moves. `parent_id` is the only source of hierarchy. Example: moving `applications.postgres` under a new `applications.databases` parent edits the `parent_id` of the existing `applications.postgres` section; collector `taxonomy.yaml` remains unchanged. Renaming the ID to `applications.databases.postgres` would be a deprecation/replacement, not a move.
   - **`sections.yaml` shape**: each entry has `id` (stable, immutable, kebab-case), `parent_id` (root entries omit), `title`, `short_name?`, `icon?`, `section_order` (for top-level ordering), `status` (`active` | `deprecated`), `deprecation?: { replacement_id, since, removal_in }`.
   - **Move semantics**: a section moves by changing `parent_id` in `sections.yaml`. Collector `taxonomy.yaml` files referencing the moved section need NO edit because they reference `section_id`, not the path. The resolved `path` in `taxonomy.json` updates automatically.
   - **Deprecation semantics**: a section may be marked `status: deprecated` with `deprecation: { replacement_id, since, removal_in }`. Consumers warn; new collectors cannot place charts under deprecated sections (TAX028).
   - **Stable IDs anchor**: FE state persistence (saved view layouts), URL query params, and deprecation tracking all key off stable section IDs, not paths.
9. **Delivery shape (CORRECTED 2026-05-11 by user decision)**: one implementation PR for the framework plus POC collector taxonomies. No PR-A1 / PR-A1.5 / PR-A2 split for this SOW. FE Phase B remains downstream and out of SOW-0016. Full collector coverage, production ibm.d sweep, legacy drift triage, and global all-collector fatality are follow-up work.
10. **Categories axis** (`meta.monitored_instance.categories`) is orthogonal to TOC. Preserved untouched. Documented in skills + AGENTS.md so contributors do not conflate the two axes.
11. **View-conditional rendering (AMENDED 2026-05-14; `only_views:` still dropped per user decision)**: sparse `single_node:` deltas and whole-body `view_switch` have separate roles. No whole-node visibility gate in v1.
    - **Top-level fields ARE the multi-node rendering** (the canonical/dominant case).
    - **`single_node:` is a sparse same-kind override block** — declared only when single-node view differs from multi-node by scalar/list/display/renderer field deltas on the same item type. It may not contain `type`, `items`, `multi_node`, `single_node`, or change an owner into a widget.
    - **`view_switch` is for whole-item replacement** — use it when branches have different item kinds, different child trees, or widget bodies where sparse override would be unclear. `view_switch.multi_node` and `view_switch.single_node` are both required and contain concrete items. `single_node:` and `view_switch` cannot appear on the same item.
    - **No `only_views:` in v1 (locked by user 2026-05-11)**: whole-node visibility gating is not part of the v1 schema. If audit 1.1 surfaces a real structure-level visibility-gate case (Scenario S2), that case becomes a **follow-up SOW** to add `only_views:` later — it does NOT block the initial framework+POC PR. The implementation PR ships without the field. Schema's permitted `x_*` extension namespace (Decision 13) does not back-door this; adding `only_views:` later requires a `taxonomy_schema_version` minor bump and the follow-up SOW's design review.
    - **Allowed override fields in `single_node:` (CLOSED set, v1)**: the set is derived from the same item type's field matrix in `.local/audits/taxonomy-design/full-shape-v1-redesign.md`. `include_charts`, `only_views`, `type`, and `items` are still NOT valid in `single_node:`. The schema closes `single_node:` properties with `additionalProperties: false` plus the permitted `x_*` extension namespace per Decision 13.
    - **List replacement examples (canonical, in spec doc)**:
      - Scalar override: `single_node: { title: "Average CPU" }` — replaces top-level `title` only for single-node view.
      - List replacement on a `type: context` widget: `contexts: [a, b, c]` + `single_node: { contexts: [a, b] }` → single-node renders only `a, b` (the top-level list does not extend).
      - Explicit clear: `group_by: [label:node]` at top + `single_node: { group_by: [] }` → single-node has no grouping.
    - **Lint rules (orthogonal, normalized after Codex review subagent 02)**:
      - **TAX021** — unknown override key under `single_node:` (closed enum violation).
      - **TAX022** — `multi_node:` override block declared (multi-node IS top level).
      - **TAX023** — list-merge attempt (educational error: lists replace, not merge; if extend is needed, use `*_extend:` field — not in v1).
      - **TAX024** — empty `single_node:` block (warning; equivalent to omitting it).
      - **TAX025** — `single_node:` override field equals top-level value (redundant override; warning).
      - ~~**TAX026**~~ — REMOVED: previously "dead override under `only_views: [multi_node]`"; no longer applicable without `only_views:`.
      - ~~**TAX027**~~ — REMOVED: previously "`only_views:` value not in closed enum"; no longer applicable.
    - **Why no per-node `condition:` everywhere (Variant A rejected)**: 2 of 3 reviewers ranked it last for maintenance; typos silently render in both views.
    - **Why no duplicate placements (Variant B rejected)**: invisible pair-link, copy-paste drift bait.
    - **Why no `variants:` block (Variant C rejected)**: critical ambiguity around "missing branch = base or = hidden".
    - **Why no pure handle-level conditioning (Variant E rejected)**: shifts source-of-truth into FE; title text (most common per-view difference) leaves the YAML.
    - **Why no `views:` wrapper**: YAGNI; `single_node:` handles simple same-kind deltas and `view_switch` handles whole-body replacement without hiding multi-node defaults inside a wrapper. Future view types require an explicit schema/version amendment.
    - The amended design keeps the maintainer's sparse-override preference for simple deltas while covering full FE body switches. Whole-section visibility gates are deliberately excluded from v1; any real S2 case from audit 1.1 becomes a follow-up SOW rather than implicit schema surface.
    - Reason for pinning at user request: cloud-frontend's `({ isSingleNode }) => ...` pattern produces view-dependent chart specs (per user "Different view depends on the view, we need this in the taxonomy"); user further clarified "multi node is the default and single node is the same, we need to support override syntax".
    - Reviewer reports: `.local/audits/taxonomy-design/schema-review-{1,2,3}.md`; external review: `.local/audits/taxonomy-design/external-review-codex/`.

12. **Deterministic merge / order rules (NEW 2026-05-11 per Change 8)**. With ~150 separate YAML files, file-system traversal order, YAML author order, and Python dict insertion order must NOT be correctness inputs. The locked merge algorithm:
    - **Top-level section order**: `sections.yaml` declares `section_order` field per top-level entry; sort ascending. Frozen v1 ordering: `system, kubernetes, containers-vms, synthetic-checks, remote-devices, otel, azure-monitor, applications, netdata` (mirrors current FE).
    - **Parent/leaf ordering**: per parent, children sort by `priority` ASC (lower = earlier; default 1000), then by normalized title (`unicodedata.normalize("NFC", title).casefold()`, Python default binary string ordering; no locale collation), then by `placement_id` (lex), then by source path (lex) as final tiebreaker.
    - **Explicit item ordering**: `items:` arrays preserve author order at every depth. The deterministic sort applies only when merging independently-authored placement/section siblings under the same parent. The generator must not recursively sort author-provided item trees.
    - **Selector and reference ordering**: structural `items:` arrays and widget `contexts:` arrays preserve author order. `context_prefix:` and `collect_plugin:` selector lists are sorted lex on emit so the JSON snapshot is stable across machines.
    - **Duplicate ownership policy**: a leaf `(section_id, leaf_id)` has EXACTLY ONE owner (TAX006 fatal). Implicit multi-owner merge is forbidden. If two collectors must contribute to a shared parent (e.g. multiple databases under `applications.databases`), they own distinct leaves under it; the parent metadata comes from `sections.yaml` (or a single explicit `section_overrides:` per audit-resolved policy).
    - **Finding emission cadence**: TAX033 and TAX036 emit once per conflicting context per unordered owner pair, sorted by context, owner key, and source path. Multiple selector mechanisms for the same pair are folded into one finding message. TAX037 emits once per referenced-only literal context per nearest item path. TAX038 emits once per stale unresolved reference per item path.
    - **JSON serialization**: `gen_taxonomy.py` emits `taxonomy.json` with sorted object keys, fixed indentation, no trailing whitespace. Re-running the generator on identical input produces byte-identical output (golden test required).
    - Reason: avoids noisy diffs in POC and later full-migration PRs; eliminates a class of CI flakiness; makes later legacy-vs-generated diff tooling reliable.

13. **Schema evolution posture (NEW 2026-05-11 per Change 15)**. Permissive `additionalProperties: true` is replaced by **closed core schemas + namespaced extension keys**.
    - **Closed core**: every taxonomy authoring object (placement, item, `single_node:` block, `sections.yaml` entry, opt-out object) declares `additionalProperties: false`. Unknown core keys fail TAX021/TAX028 (depending on context). This catches the typo class that already exists in cloud-frontend (`applications.js:53` has `icons:` instead of `icon:`; `systemHardware.js:69-70` has duplicate `title` keys).
    - **Renderer envelope**: FE-private renderer payloads may be carried only under a fenced `renderer:` object. Open pass-through fields directly on item bodies are rejected so typo detection remains meaningful.
    - **Namespaced extension keys**: `x_*` is the only permitted extension namespace on core nodes. Extensions are preserved into `taxonomy.json` under an `_extra` block, scoped to the originating placement/subsection. Extensions never alter rendering until a schema version claims them.
    - **Breaking-change boundary** (must be documented in `taxonomy.md` spec):
      - **Non-breaking**: adding optional fields with defined defaults; adding new `sections.yaml` entries; adding deprecation metadata.
      - **Non-breaking only if old consumers ignore them safely**: adding a new view type (closed enum extension); adding a new selector type; adding a new override field under `single_node:`. All of these require a `taxonomy_schema_version` minor bump even if the change is forward-compatible at the data level.
      - **Breaking (require major schema version bump)**: changing matcher semantics, override merge semantics, list-replacement behavior, section ID/path identity, generated JSON field renames, or removing/renaming existing output fields.
    - **Persisted shorthands forbidden**: no `contexts: all_from_metadata`. The seed tool (Decision 9, active implementation plan) generates explicit lists at author time; subsequent metric additions to `metadata.yaml` MUST be reflected in a taxonomy diff or coverage check fails.
    - Reason: open schemas preserve typos; the SOW's drift-elimination goal requires loud failure on misspelled fields, not silent acceptance.

14. **Chart-recipe manifest remains removed; explicit item bodies replace it (UPDATED 2026-05-14)**: previously proposed as `integrations/taxonomy/chart_recipes.yaml` to validate `include_charts:` handle references. v1 still has no `include_charts:` field and no chart-recipe manifest. The reopened full-shape design models first-available alternatives, grids, context/table widgets, and view-conditioned item bodies directly inside ordered `items:` rather than through recipe handles.

## Plan

1. **Single framework+POC PR (active scope)** — narrow `_common.py` extract with byte-identical proof; new schemas with closed-core posture; `sections.yaml` with stable IDs; `icons.yaml`; `gen_taxonomy.py` with deterministic merge and structured `Finding` renderers; `gen_taxonomy_seed.py`; fresh `check_collector_taxonomy.py`; a small POC collector set; CI wired with changed-collector fatal gate; documentation, specs, and skills updated.
2. **Inline audit evidence** — audit outputs under `.local/audits/taxonomy-design/audit/` are produced before the implementation step that depends on them. Audit 1.10 is a non-blocking FE coordination note. Audit 1.5 is only required if production ibm.d codegen or an ibm.d POC is included.
3. **Review checkpoints** — after each major implementation step, decide whether a Claude review is useful. If yes, provide a focused prompt with exact files and questions.
4. **Follow-up work (not this PR)** — full collector taxonomy coverage, production ibm.d sweep, legacy drift triage, global-all-collector fatality, and cloud-frontend consumption.

Total for this SOW is now the single Netdata framework+POC PR. SOW-0016 closes when that PR merges green and the validation/artifact gates are filled.

**Cloud-frontend Phase B is OUT of scope of SOW-0016** (FE team owns it on their schedule; tracked as a downstream FE-team SOW). The Netdata-side deliverables are: published `taxonomy.json` artifact shape, `taxonomy_output.json` schema, generator/checker/seed framework, changed-collector CI gate, docs/spec/skills, and POC taxonomy files.

## Execution Log

### 2026-05-10

- 3 independent Opus 4.7 analysis agents launched in parallel; architecture reports stored at `.local/audits/taxonomy-design/agent-{1,2,3}-analysis.md`.
- Synthesized recommendation produced; user reviewed and locked decisions 1–10 plus added the view-condition requirement (decision 11).
- TODO file `TODO-collector-taxonomy-unification.md` updated to reference this SOW.
- This SOW created and Pre-Implementation Gate filled.
- User flagged the initial per-node `condition:` schema design (Variant A) as a UX risk; 3 independent reviewers ran in parallel as taxonomy-author personas (Marta/Pavel/Lina) across 5 schema variants and 3 scenarios. Reports at `.local/audits/taxonomy-design/schema-review-{1,2,3}.md`.
- Schema-review synthesis: hybrid `only_views:` + `views:` was initially chosen.
- User refined #1 (YAGNI): drop `only_views:` from v1; ship pure Variant D (`views:` overrides only). Schema's `additionalProperties: true` posture allows non-breaking addition of `only_views:` later if pre-audit step 1.1 finds an S2 case.
- User refined #2 (avoid duplication): pure D's symmetric `views: { single_node, multi_node }` requires writing field defaults somewhere awkward. User feedback: "multi node is the default and single node is the same, we need to support override syntax — take multi and override some stuff". Schema refined to curated-and-override: top-level fields ARE multi-node; `single_node:` block holds the sparse delta. No `views:` wrapper, no `multi_node:` block. Decision 11 updated. Validator codes: TAX021 (unknown view-override key), TAX022 (`multi_node:` block illegal — fields go at top level), TAX023 (list-merge attempt), TAX024 (empty `single_node:` block).
- User refined #3 (dynamic contexts): SNMP, prometheus, cgroup, apps emit contexts whose names share a namespace prefix. Decision 2 updated to allow `context_prefix:` (string prefix only, NOT regex) with an opt-in guardrail: collector must declare `metrics.dynamic_context_prefixes: [...]` in its `metadata.yaml`. Adds TAX031 (prefix not declared), TAX033/TAX036 ownership-overlap checks, and TAX034 (redundant explicit context under prefix). TAX032 remains a reserved follow-up code for a narrower prefix-overlap diagnostic if the project later needs one.
- Empirical validation (7 go.d collectors against locked schema, report at `.local/audits/taxonomy-design/schema-validation-drafts.md`): 6/7 pass cleanly. statsd exposed a real gap — its user-app synthetic charts use user-supplied names (`name = myapp` → `myapp.*` chart names) with no shared prefix. Same shape applies to `charts.d.plugin` (bash scripts pick their own names) and `python.d.plugin` (legacy). User confirmed adding `collect_plugin:` selector to v1 (Decision A on 2026-05-10).
- User refined #4 (label selector): added `collect_plugin: [<plugin-name>]` selector parallel to `context_prefix:`. Selects any chart whose `_collect_plugin` label matches. Same opt-in pattern: collector declares `metrics.dynamic_collect_plugins: [{plugin, reason}, ...]` in `metadata.yaml` (or in `taxonomy.yaml` for plugins lacking `metadata.yaml` like statsd.plugin). Adds TAX035 (collect_plugin not declared), TAX036 (overlap between collectors).
- Validation also surfaced a real-world drift: `mysql.open_transactions` referenced in `cloud-frontend/.../applications.js:1957` does NOT exist in `mysql/metadata.yaml`. This is exactly the failure TAX003 catches at PR time. PR-A2 plan extended: run a diff-tool sweep over the legacy taxonomy to inventory similar drifts before bulk migration.
- Other empirical findings (some superseded by later decisions): non-leaf sections must live in `sections.yaml`; exactly one collector may use `section_overrides:` for any given `section_id`; empty `contexts:` is valid when `context_prefix:` or `collect_plugin:` is present; postgres-style 70-context enumeration requires `gen_taxonomy_seed.py` to amortize author cost. The earlier `section_path:` and `include_charts:` draft shapes are explicitly superseded by Decisions 5 and 8.A.
- Schema impact: `integrations/schemas/collector.json` extended with two optional dynamic declarations. This is the first material change to the existing collector schema in this SOW.

### 2026-05-11

- External Codex review run (6 parallel GPT-5.5-xhigh subagents per the prompt at `/tmp/codex-taxonomy-review-prompt.md`); reports stored at `.local/audits/taxonomy-design/external-review-codex/`. Synthesis verdict: GO WITH CHANGES. Architecture approved; SOW not ready for PR-A1 kickoff until 15 contract-level gaps closed.
- SOW amended this date to incorporate all 15 required changes:
  - **Change 1 (audit 1.4 false premise)**: rewrote step 1.4 from "confirm none" to a `virtualContexts` classification table with per-row owner. The initial disposition name `frontend-recipe-handle` was later normalized to `keep-frontend-only` after Decision 14 was removed. `systemStorage.js:3-31, 103` cited as the disproof of the previous premise.
  - **Change 2 (chart handle contract)**: added Decision 14 — `integrations/taxonomy/chart_recipes.yaml` manifest with stable handle IDs, consumed_contexts, ordered alternatives (for `getMenu.js:77-82` first-available semantics), supported_views, owner, status, deprecation. Validators TAX040–TAX042 added.
  - **Change 3 (Netdata negative selector)**: amended Decision 2 with `context_prefix_exclude:` constrained-exclusion field; audit 1.8 chooses static enumeration vs prefix+exclude; regex remains forbidden.
  - **Change 4 (selector semantics)**: amended Decision 2 with explicit union behavior, cross-type overlap detection, resolved-snapshot-vs-runtime contract, no-metadata collector inline declarations, FE label-access proof requirement (audit 1.10).
  - **Change 5 (taxonomy.json lifecycle)**: amended Decision 3 — gitignored ephemeral in PR-A1; vendored build-pinned in Phase B; `taxonomy_schema_version`, source commit metadata, deprecation fields, FE build-time validation.
  - **Change 6 (stable section identity)**: amended Decision 8 — originally proposed path 8.A (stable `section_id`) vs 8.B (immutable path segments); later user sign-off locked 8.A and removed `section_path:` as an accepted v1 authoring form.
  - **Change 7 (view-conditional hardening)**: amended Decision 11 — originally reconsidered `only_views:`; later user sign-off dropped it from v1. Closed allowed-override-fields set remains; canonical examples are no override, scalar override, and list replacement; lint codes TAX026/TAX027 are removed.
  - **Change 8 (deterministic merge/order)**: added Decision 12 — locked sort key `(section_order, priority, normalized_title, placement_id, src_path)`; deterministic JSON serialization; one-owner-per-leaf rule; 10-run byte-identical golden test.
  - **Change 9 (seed tooling)**: promoted `gen_taxonomy_seed.py` to PR-A1 hard requirement; persisted `contexts: all_from_metadata` shorthand explicitly forbidden (Decision 13).
  - **Change 10 (fresh checker)**: explicit "do not mirror `check_collector_metadata.py`" guidance; `check_collector_taxonomy.py` is a fresh wrapper around `_common.py` and taxonomy validators.
  - **Change 11 (PR-A1.5 blocking)**: PR-A1.5 elevated from "may merge as part of A1" to a SEPARATE BLOCKING PR between PR-A1 and PR-A2; audit 1.5 produces a working `db2` prototype with round-trip evidence; escalation path defined if prototype fails in <1 week.
  - **Change 12 (changed-collector fatal gate)**: from PR-A1 onward, taxonomy coverage is fatal for changed `metadata.yaml`/`taxonomy.yaml` files; global-all-collector coverage stays warning until PR-A2 final.
  - **Change 13 (drift triage process)**: PR-A2 step 4.6 introduces a drift inventory with closed disposition set `{drop-frontend-entry, fix-metadata, fix-collector, keep-frontend-only, defer-with-sow}` and per-finding owner; bulk migration implementer cannot silently decide product semantics.
  - **Change 14 (Phase B realism)**: original wording held Phase B estimate at 2 weeks; reviewer pass updated to 6–8 weeks and required FE rollback/design artifacts. **2026-05-11 scope correction superseded all of this**: Phase B is OUT of SOW-0016; FE-team owns it. Those FE artifacts are downstream FE-SOW responsibilities. SOW-0016 closes when the single framework+POC PR lands with green validation.
  - **Change 15 (schema evolution)**: added Decision 13 — closed core schemas (`additionalProperties: false`); only `x_*` namespaced extension keys allowed at core nodes; explicit breaking-change boundary documentation.
- Pre-Implementation Audit Outputs section was added during hardening and later normalized to 10 required audit deliverables + 1 non-blocking coordination note after audit 1.7 and 1.9 were removed; PR-A1 is blocked until all required outputs are non-`to-decide`.
- Drift Triage Process and Rollback Matrix subsections added.
- Open decisions section temporarily rewritten with 5 user-facing residual decisions (Phase B fallback shape; artifact lifecycle path; section identity model 8.A vs 8.B; `only_views:` v1 inclusion default; drift triage owner-assignment policy). Later same-day scope correction and user sign-off resolved all of them.
- Acceptance criteria expanded from 9 items to 27 items reflecting the new gates.
- Plan total was temporarily revised from "~6–8 weeks" to "15–19 weeks calendar" when Phase B was still included; later same-day scope correction narrowed SOW-0016 back to Netdata-only.
- Sub-state updated to "design hardened after external Codex review (GO WITH CHANGES); pre-implementation audit blocking; PR-A1 cannot start until audit produces written evidence."
- **2026-05-11 scope correction (later same day)**: user clarified two points:
  - (a) "we don't need to change FE - the FE guys will do it. We just need to prepare everything in Netdata repo." Phase B is moved OUT of scope of SOW-0016; cloud-frontend consumption is tracked as a downstream FE-team SOW on their schedule.
  - (b) The previously-recommended "build-pinned vendored" artifact lifecycle was over-engineered relative to the existing `integrations.js` precedent. Decision 3 corrected: gitignored ephemeral generation, consumed by FE-team's pipeline exactly as `integrations.js` is consumed today; contract-discipline lives in `taxonomy_schema_version` IN the artifact, not in the delivery mechanism.
- Consequent SOW edits made in the same session:
  - Decision 3 rewritten to match `integrations.js` precedent.
  - Historical note, superseded 2026-05-14: Decision 5 was then corrected to keep chart bodies FE-side. The later 2026-05-14 user decision reopens this boundary and requires explicit full-shape typed `items:` in v1.
  - Decision 14 REMOVED from v1: chart-recipe manifest unnecessary with `include_charts:` removed. This remains true after the 2026-05-14 reopening because explicit typed item bodies replace recipe handles.
  - Phase B Plan section replaced with a one-paragraph "OUT OF SCOPE" reference to the downstream FE-team SOW.
  - Audit 1.7 (FE adapter spike) and 1.9 (chart-recipe manifest seed) REMOVED. Audit 1.10 (`_collect_plugin` feasibility) downgraded from blocking to a non-blocking coordination question sent to FE team.
  - Acceptance criteria pruned of FE-side gates (vendoring proof, rollback runbook, staging fixture, FE adapter spike, chart-recipe handle validation).
  - Rollback Matrix Phase B row removed; matrix simplified to Netdata-only (PR-A1, PR-A1.5, PR-A2).
  - Plan total revised from "15–19 weeks" to "7–9 weeks calendar of Netdata-side work" honestly reflecting the narrower scope.
  - Open Decisions reduced from 5 user items to 3 (Phase B fallback shape and artifact lifecycle path are now resolved by the scope correction itself).
  - Followup mapping updated: Phase B becomes a downstream FE-team SOW. Historical chart-recipe-handle follow-up was superseded on 2026-05-14 by explicit full-shape typed item bodies.
- **2026-05-11 residual-decision sign-off (final design lock)**:
  - Decision 1 (section identity model) → **8.A locked**. Stable `section_id` first-class in `sections.yaml`; collector taxonomy references `section_id:`; section moves are `parent_id` edits.
  - Decision 2 (`only_views:` v1 inclusion) → **dropped from v1** per user ("I don't think we need only_views"). If audit 1.1 finds an S2 case, that case spawns a follow-up SOW (non-blocking for PR-A1). Decision 11 amended; canonical examples reduced from 4 to 3; lint codes TAX026 and TAX027 removed; `single_node:` allowed-fields set updated accordingly.
  - Decision 3 (drift triage owner assignment) → **A locked**. Bulk-migration implementer assigns owners based on finding type; named owners sign off or push back.
  - Sub-state updated: "all design decisions locked 2026-05-11; SOW ready to move from `pending/` to `current/` on user go-ahead". Open Decisions section now records all 3 resolutions for traceability.
  - No further user decisions block PR-A1 start. Only the audit remains.
- **2026-05-11 final readiness review (3 parallel Opus 4.7 reviewers; all returned READY WITH NOTES)**:
  - Reports: `.local/audits/taxonomy-design/final-review/agent-{1,2,3}-readiness.md`.
  - 9 patches applied to close all surfaced gaps:
    - **Patch 1 (Critical, R3)**: Step 2.11 CI wiring corrected — `generate-integrations.yml` runs on `push: master` (post-merge); `check-markdown.yml` is the PR-time gate. Taxonomy validation must run in BOTH workflows; "fatal in CI" gates explicitly live in `check-markdown.yml`. Path triggers extended on both workflows.
    - **Patch 2 (High, R1+R3)**: Decision 8.A clarified — `section_id:` is canonical and only accepted authoring form in v1. `section_path: [list]` is NOT accepted in v1 schema. Eliminates the typo/ambiguity surface.
    - **Patch 3 (High, R1+R2)**: Step 2.12 "touched-collector" gate definition tightened — fatal only when diff modifies `metrics.*` keys OR `taxonomy.yaml` OR adds/removes either file. Edits to `overview`, `setup`, `troubleshooting`, `alerts`, `related_resources` do NOT trigger the gate. Prevents PR-A1→PR-A2 window from blocking unrelated metadata edits.
    - **Patch 4 (High, R2)**: Audit 1.11.b added — full `sections.yaml` tree draft (~80–150 entries) walking every cloud-frontend taxonomy file. Without this, PR-A1 step 2.4 was a developer guessing the topology from 18 JS files for a day.
    - **Patch 5 (High, R1)**: Banner added to `schema-validation-drafts.md` warning it is superseded for authoring (25+ stale `include_charts:` uses; ~20 stale `section_path:` uses). First developer reading it as template no longer gets the wrong shape.
    - **Patch 6 (Medium, R2)**: Step 2.2 byte-identical proof phrasing fixed — `git diff --exit-code` does NOT work on gitignored ephemeral files; correct procedure uses `diff -u` against captured pre-refactor copies stored under `.local/audits/taxonomy-design/refactor-baseline/`.
    - **Patch 7 (Medium, R1)**: Drift Triage Process — 5-business-day escalation clock added; if a named owner doesn't sign off within 5 business days, implementer escalates to user for tie-break. Prevents PR-A2 final stalling on PTO.
    - **Patch 8 (Medium, R2+R3)**: Stale residue swept — Step 2.14 "chart-recipe-alternatives" fixture replaced with a Netdata-negative-selector fixture (Decision 14 was removed; old fixture name was leftover); `keep-frontend-virtual` spelling normalized to `keep-frontend-only`; `getMenu.js` path corrected from `taxonomy/getMenu.js` to `charts/toc/getMenu.js`; stale Phase-B-coupling references in Risks and Execution Log replaced with downstream-FE-SOW language.
    - **Patch 9 (Medium, R3)**: Decision 2 shapes locked — `inline_dynamic_declarations:` block shape and `resolved_contexts` snapshot shape both written into the SOW with concrete YAML/JSON examples. No remaining schema TBDs before PR-A1 step 2.3.
  - Total audit outputs now 10 (was 9): added `1.11b-sections-tree.md`. Total reviewer-flagged ambiguities resolved: all surfaced concerns either patched in SOW or explicitly assigned to in-flight PR-A1 work.
  - SOW is ready to move from `pending/` to `current/`. Audit phase begins on the next user action.
- **2026-05-11 external Codex post-patch readiness recheck (4 parallel subagents; synthesis verdict NOT READY as artifact bundle, no architecture blocker)**:
  - Reports stored under `.local/audits/taxonomy-design/external-review-codex-2/`.
  - Findings: 5 of 9 patches landed cleanly; 4 partially landed due stale text in acceptance criteria, plan, validation, TODO, and walkthrough.
  - Sweep applied: normalized audit count to 10 required outputs + 1 non-blocking coordination note; removed active `section_path` authoring language; removed active `include_charts` / chart-recipe / Phase-B validation residue from Netdata execution gates; replaced stale `git diff --exit-code` proof wording for gitignored artifacts with the `diff -u` baseline-copy procedure; locked PR-A1 clarifications for `taxonomy_optout`, deterministic title normalization, seed output, audit 1.11.b dependency, and opaque stable section-ID semantics.
  - Targeted stale-reference grep validated the sweep; SOW is ready to move from `pending/` to `current/` on user go-ahead.
- **2026-05-11 audit phase start**:
  - User approved moving SOW-0016 from `pending/` to `current/` and starting the audit output phase.
  - Status changed from `open` to `in-progress`.
  - Audit output directory initialized at `.local/audits/taxonomy-design/audit/` with index `00-audit-index.md`.
- **2026-05-11 one-PR execution correction**:
  - User decided: "we will do everything in one PR (framework - w/o adding taxonomy for all collectors, can add a few as a POC)."
  - Branch created and checked out: `sow-0016-taxonomy-framework-poc`.
  - Active SOW scope corrected: single framework+POC PR; no PR-A1 / PR-A1.5 / PR-A2 split for this SOW.
  - Full collector coverage, production ibm.d sweep, legacy drift triage, and global all-collector fatality moved to follow-up work unless explicitly pulled into this PR.
  - User requested review checkpoints: after each major step, provide a Claude prompt if an external review would be useful.
- **2026-05-11 framework+POC implementation pass**:
  - `_common.py` extracted from `gen_integrations.py`; byte-identical `integrations.{json,js}` proof completed before later schema/metadata changes.
  - Taxonomy schemas, section/icon registries, generator, checker, seed helper, and unittest coverage added.
  - CI wired in both PR-time `check-markdown.yml` and post-merge `generate-integrations.yml`.
  - POC collector taxonomies added for apache, mysql, postgres, nvidia_smi, and snmp.
  - SNMP metadata extended with `metrics.dynamic_context_prefixes` for `snmp.`.
  - Contributor docs, project skills, AGENTS.md, and taxonomy spec updated.
  - Local `.venv` validation passed; details recorded in the Validation section.
- **2026-05-11 Claude review follow-up**:
  - User provided external review verdict READY WITH NOTES.
  - Fixed accepted pre-merge items F1/F2/F4/F6/F7/F8/F12: no-metadata `taxonomy_optout` no longer emits a misleading missing-metadata fatal; metadata files whose metrics block was removed are treated as touched; 10-run determinism unittest added; seed/checker docs added to README, pipeline, and add-go-collector recipe; title normalization aligned to NFC.
  - Deferred per user/review scope: YAML-aware metrics span parsing, broader TAX negative-test matrix, global gate severity policy, invalid-metadata surfacing, schema-error message-code mapping hardening, artifacts-and-banners taxonomy entry, and opt-out POC example.
- **2026-05-14 full-shape redesign pause**:
  - User rejected structural-only POC depth and required v1 to cover full Cloud FE TOC shapes.
  - Implementation paused; no further code changes until the amended full-shape contract is reviewed.
  - Design artifact updated at `.local/audits/taxonomy-design/full-shape-v1-redesign.md`.
  - Claude review prompt updated at `.local/audits/taxonomy-design/full-shape-v1-adversarial-review-prompt.md`.
- **2026-05-14 full-shape implementation resume**:
  - External review returned READY TO IMPLEMENT after B1-B7 plus R1/R2 amendments.
  - `taxonomy_collector.json`, `taxonomy_output.json`, `gen_taxonomy.py`, `gen_taxonomy_seed.py`, `test_taxonomy.py`, docs/spec/skills, and all five POC `taxonomy.yaml` files were updated to the ordered recursive `items:` contract.
  - MySQL POC now models summary grid widgets, table widgets, nested structural groups, owned context leaves, `referenced_contexts`, and legacy FE drift correction for `mysql.galera_open_transactions`.
  - Local `.venv` validation passed for py_compile, generator check-only, touched-collector checker, seed helper, determinism diff, performance spot check, and 18 taxonomy unit tests.

## Validation

Acceptance criteria evidence:

- Implemented locally:
  - `integrations/_common.py` extracted and `integrations/gen_integrations.py` refactored to reuse it.
  - `integrations/schemas/taxonomy_collector.json`, `taxonomy_sections.json`, and `taxonomy_output.json` added with closed-core v1 authoring; on 2026-05-14 `taxonomy_collector.json` and `taxonomy_output.json` were updated from the structural-only POC shape to the full ordered recursive `items:` shape.
  - `integrations/taxonomy/sections.yaml` and `icons.yaml` added for the POC section/icon registry.
  - `integrations/gen_taxonomy.py`, `integrations/gen_taxonomy_seed.py`, and `integrations/check_collector_taxonomy.py` added. On 2026-05-14, `gen_taxonomy.py` was updated to emit local `resolved_contexts`, `referenced_contexts`, and `unresolved_references` snapshots for every placement/item, enforce TAX037 referenced-but-not-owned, preserve TAX036 for selector ownership overlap, deduplicate/sort TAX033/TAX036 conflict emission, and support `owned_context`, `group`, `flatten`, `selector`, `context`, `grid`, `first_available`, and `view_switch` item kinds.
  - POC `taxonomy.yaml` files added for apache, mysql, postgres, nvidia_smi, and snmp. On 2026-05-14, all five were migrated to `items:`; MySQL became the full-shape proof with summary grid, table widgets, nested groups, owned structural context leaves, and legacy drift correction from `mysql.open_transactions` to `mysql.galera_open_transactions`.
  - `snmp/metadata.yaml` declares `metrics.dynamic_context_prefixes: [{prefix: snmp., reason: ...}]` for the SNMP dynamic-prefix POC.
  - CI wiring added to `check-markdown.yml` and `generate-integrations.yml`.
  - `integrations/taxonomy.json` added to `.gitignore`.

Tests or equivalent validation:

- Passing locally with repo-local `.venv`:
  - `.venv/bin/python -m py_compile integrations/_common.py integrations/gen_integrations.py integrations/gen_taxonomy.py integrations/gen_taxonomy_seed.py integrations/check_collector_taxonomy.py integrations/tests/test_taxonomy.py`
  - `.venv/bin/python integrations/gen_integrations.py`
  - `.venv/bin/python integrations/gen_taxonomy.py --check-only`
  - `.venv/bin/python integrations/check_collector_taxonomy.py`
	  - `.venv/bin/python -m unittest integrations.tests.test_taxonomy` (40 tests, including old-shape rejection, recursion-matrix rejection, renderer-envelope rejection, positive coverage for item kinds, TAX003 unknown-context fatal, TAX036 selector-overlap preservation, TAX037 referenced-but-not-owned enforcement, TAX038 stale-unresolved warning, unresolved payload output, dynamic-prefix narrowing, metadata-warning surfacing, YAML-aware touched-collector span parsing, deleted-collector gate handling, and 10-run deterministic taxonomy output check)
  - `.venv/bin/python integrations/gen_taxonomy_seed.py src/go/plugin/go.d/collector/apache/metadata.yaml --module-name apache --section-id applications.apache --placement-id apache --icon apache` (emits flat `items:`)
  - Determinism proof: two consecutive `gen_taxonomy.py --output /private/tmp/netdata-taxonomy-{1,2}.json` runs compared cleanly with `diff -u`.
  - 2026-05-14 full-shape determinism proof: two consecutive `gen_taxonomy.py --output /private/tmp/netdata-taxonomy-fullshape-{1,2}.json` runs compared cleanly with `diff -u`.
  - Performance spot check after Claude-blocker fixes: `/usr/bin/time -p .venv/bin/python integrations/gen_taxonomy.py --check-only` completed in `real 3.99` seconds.

Real-use evidence:

- `integrations/gen_taxonomy.py` emitted a valid local full-shape taxonomy artifact containing the five POC placements. The generated `section_path` values are `applications.apache`, `applications.mysql`, `applications.postgres`, `system.hardware.gpus.nvidia`, and `remote-devices.snmp`. The artifact is gitignored and not intended for commit.
- MySQL POC coverage proof from `/private/tmp/netdata-taxonomy-fullshape-1.json`: MySQL placement has `families: null`, 75 `resolved_contexts`, 16 `referenced_contexts`, 0 `unresolved_references`, a summary `grid` first item with 8 widget references, and no missing or extra owned contexts compared with `mysql/metadata.yaml` (75 metadata contexts, 75 owned).
- MySQL intentionally owns `mysql.handlers` once in the `Handlers` structural group even though the legacy FE listed it in more than one visual grouping; the v1 contract requires single ownership and display widgets can reference owned contexts separately.
- NVIDIA intentionally adds structural Bus / Utilization / Memory / Sensors / MIG grouping around the legacy FE table coverage. This is a Netdata-side taxonomy improvement, not an accidental FE parity miss.
- `integrations/gen_docs_integrations.py -c go.d.plugin/snmp` produced no committed doc drift after adding the SNMP dynamic declaration, confirming the metadata extension does not alter generated user docs.

Reviewer findings:

- External readiness reviews are complete before implementation.
- Post-implementation Claude review returned READY WITH NOTES and identified accepted fixes F1/F2/F4/F6/F7/F8/F12; all seven were applied.
- Claude re-review returned READY: all seven accepted fixes are applied, tested, and cross-referenced in SOW/TODO/spec/recipe. Deferred caveats F3/F5/F9/F10/F11/F13/F14 remain non-blocking by design.
- 2026-05-14 full-shape contract review returned READY TO IMPLEMENT after B1-B7 and R1/R2 amendments. The local implementation now targets that full-shape contract.
- 2026-05-14 full-shape implementation review returned NOT READY with three blockers: MB1 unresolved payload dropped, MB2 MySQL `families: true`, MB3 missing TAX003 negative test. All three are fixed. Same-PR improvements also landed for TAX033/TAX036 dedupe/sort, recursion-matrix tests, renderer-envelope tests, item-kind positive tests, and schema-reference matrix depth.
- 2026-05-14 post-blocker Claude re-review returned READY. It confirmed MB1/MB2/MB3 closure and same-PR fixes for TAX033/TAX036 cadence, recursion-matrix tests, item-kind positive tests, renderer-envelope tests, and schema-reference depth. Remaining polish is non-blocking: generic TAX001 stale-shape diagnostics and no artificial `view_switch`/`first_available`/`flatten`/renderer examples in POC YAMLs.

Same-failure scan:

- Targeted stale-shape scan completed during implementation via schema/unit coverage:
  - `section_path` authoring is rejected by `taxonomy_collector.json`.
  - `multi_node:` is rejected by TAX022 prescan.
  - `context_prefix_exclude:` without same-node `context_prefix:` raises TAX029.
  - top-level/placement `contexts:` authoring is rejected; POC collectors now use `items:`.
  - strings inside `grid.items` are rejected by schema; display positions require object items.
  - forbidden recursion-matrix cases are covered by unit tests: owning items in grid bodies, nested flatten, string first-available alternatives, string/flatten/nested-view-switch branches.
  - renderer pass-through is covered by unit tests: unknown non-`x_*` renderer keys and item-body renderer fields are rejected; `x_*` inside `renderer` is accepted.
  - Existing stale references to the old five-file shorthand were updated in integration lifecycle docs/recipes except the historical note that the shorthand is stale.

Sensitive data gate:

- Confirmed: this SOW, the linked TODO, the 3 agent reports, and all anticipated artifacts contain no raw secrets, credentials, bearer tokens, SNMP communities, customer data, personal data, customer-identifying IPs, private endpoints, or proprietary incident details. The work is structural metadata only. No redaction required.

Artifact maintenance gate:

- AGENTS.md: updated — collector-consistency rule now includes `taxonomy.yaml`; integrations-lifecycle trigger includes taxonomy files/artifacts.
- Runtime project skills: updated — `integrations-lifecycle/` and `project-writing-collectors/` now document taxonomy authoring, generator/checker flow, artifact contract, and consistency impact.
- Specs: updated — new `.agents/sow/specs/taxonomy.md` records source files, authoring contract, selectors, output artifact, CI contract, finding-code matrix, and contributor rule.
- End-user/operator docs: updated — `integrations/README.md` documents `gen_taxonomy.py --check-only`, dynamic selector opt-ins, and the gitignored taxonomy artifact.
- End-user/operator skills: no impact expected (skills do not consume the TOC).
- SOW lifecycle: open → in-progress when moved to `current/` → completed after the single framework+POC PR validates and lands. Final move to done/ together with the work commit per project rule "Do not create a separate commit just to mark or move the SOW".

Specs update:

- Complete locally — `.agents/sow/specs/taxonomy.md`.

Project skills update:

- Complete locally — `integrations-lifecycle/`, `project-writing-collectors/`.

End-user/operator docs update:

- Complete locally — `integrations/README.md`.

End-user/operator skills update:

- No impact expected.

Lessons:

- Captured in `## Lessons Extracted`.

Follow-up mapping:

- Updated. Anticipated follow-ups:
  - **Claude review deferred items (not blocking this POC PR)**: broader TAX negative-test matrix; global gate severity policy as taxonomy coverage grows; schema-error message-code mapping hardening; opt-out POC example.
  - **Audit evidence (in-scope this SOW)**: 10 supporting outputs under `.local/audits/taxonomy-design/audit/` plus 1 non-blocking FE coordination note; consumed inline before dependent implementation steps.
  - **Cloud-frontend Phase B SOW (downstream, FE-team-owned, separate private-repo SOW)**: FE team consumes the published `taxonomy.json` on their schedule. They own: consumer module, taxonomy module removal, chart-spec extraction, `dynamicSections` removal, regex-catchall removal, rollback strategy. Out of scope of SOW-0016.
  - **Full collector taxonomy migration**: follow-up SOW/PR plan for the remaining collectors, global all-collector fatality, and legacy drift inventory.
  - **Drift triage follow-ups**: any `defer-with-sow` disposition from the future full-migration drift inventory spawns a new SOW with the named owner before that SOW closes.
  - **`virtualContexts` follow-ups**: any `defer-with-sow` disposition from audit 1.4 spawns a new SOW.
  - **Chart-recipe handle manifest**: no longer planned for v1. The 2026-05-14 full-shape redesign uses explicit typed item bodies instead of recipe handles. A future handle system would require a separate SOW and schema bump.
  - **Any new top-level section additions** to `sections.yaml` after v1: each is a small new SOW.
  - **`only_views:` schema feature reopens** if audit 1.1 surfaces multi-axis view conditioning (more than `single_node | multi_node`): future SOW with `taxonomy_version` bump.

## Outcome

In progress for the current PR. Delivered locally on branch `sow-0016-taxonomy-framework-poc`:

- collector-adjacent `taxonomy.yaml` authoring contract;
- closed authoring/output schemas and section/icon registries;
- `gen_taxonomy.py`, `gen_taxonomy_seed.py`, and `check_collector_taxonomy.py`;
- PR and master-regeneration workflow wiring;
- integrations README, SOW spec, and project-skill updates;
- five full-shape POC collector taxonomies for Apache, MySQL, Postgres, NVIDIA, and SNMP.

## Lessons Extracted

- Stable `section_id` values need generated path segments derived from the final ID component; otherwise opaque IDs with namespace punctuation produce duplicated paths such as `applications.applications.apache`.
- Full-shape POC files must exercise real dashboard structures. Schema-valid flat lists are not enough to prove the contract.

## Followup

- Full collector taxonomy migration for the remaining collectors, including global all-collector fatality timing.
- Downstream cloud-frontend consumption SOW: fetch/copy `integrations/taxonomy.json`, implement the adapter, and remove legacy taxonomy modules on the FE team's schedule.
- Production ibm.d taxonomy generation from module source data.
- Broader TAX negative-test matrix and more precise stale-shape diagnostics.
- Invalid-metadata surfacing and schema-error message-code hardening beyond the fixes included in the framework PR.
- Opt-out POC example for a no-metadata/dynamic plugin.
- Future top-level section additions or `only_views:`-style view-axis expansion require separate SOWs and schema-version changes.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
