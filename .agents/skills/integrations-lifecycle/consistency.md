# Collector consistency rule

Any change touching a collector MUST land in one source PR with matching changes to every affected authoritative
artifact (root `AGENTS.md`, "Collector Consistency"):

1. **The code**: the collector implementation files.
2. **`metadata.yaml`**: the integration page driver (field content: `.agents/skills/collectors-metadata-yaml/`). A
   sibling `taxonomy.yaml` is not part of this list ("The dormant collector taxonomy" below).
3. **`config_schema.json`**: the dashboard's DynCfg editor (form rules:
   `.agents/skills/collectors-go-design/config-schema.md`).
4. **The stock `.conf`**: what `/etc/netdata/<plugin>/...` ships.
5. **`health.d/*.conf`**: alert definitions for the collector's metrics; the integration page's alert table is derived
   from them and the contexts `metadata.yaml` declares.
6. **The authoritative documentation source**: usually `metadata.yaml`; never a generated integration page or umbrella
   page.

A PR may legitimately not touch every file, but because most cross-artifact checks are not enforced by CI, its
description MUST enumerate the relevant artifacts and justify each one left unchanged. Any SHOULD-level exception or
escape hatch the implementation uses MUST be visible in the PR description or design note, not only in a code comment.

Obvious cases: a unit change in code updates `metadata.yaml`; a new option updates the schema, the stock conf, and the
docs; a new metric updates its authoritative metadata/docs inputs, while generated README content follows the delivery
boundary below. Subtle ones: renaming a metric label changes the alert definition that refers to it; changing a default
changes the stock conf example and the documented default value.

## Delivery boundary: source PR versus post-merge PR

This is the skill's one statement of the boundary; other skill files point here. `integrations/README.md` carries the
short contributor-facing version.

- The source PR contains authoritative inputs (hand-authored `metadata.yaml`, ibm.d `contexts.yaml`/`config.go`/
  `module.yaml`, SNMP profiles and the trap catalogue, `src/health/health.d`), generators, schemas, templates,
  workflows, tests, and maintainer contracts.
- Generated documentation is NOT in the source PR: per-integration `<dir>/integrations/<slug>.md`, generated `README.md`
  files and symlinks, the umbrella pages `src/collectors/COLLECTORS.md`, `SECRETS.md`, `SERVICE-DISCOVERY.md`, and
  producer-generated metadata (ibm.d `metadata.yaml`, the NPM catalog `metadata.yaml`). After the source PR merges,
  `.github/workflows/generate-integrations.yml` reruns the producers and generators and opens the `integrations-regen`
  PR ("Regenerate integrations docs") with every derived change; a maintainer reviews and merges it. Name that delivery
  route in the source PR description.
- Exception, generated runtime outputs: ibm.d `contexts/zz_generated_contexts.go` (compilation) and `config_schema.json`
  (the shipped configuration contract) MUST ship in the source PR with the `contexts.yaml` or `config.go` change that
  produced them. Both workflows run `go generate` and fail on drift in those two files (step "Verify generated runtime
  outputs"). `go generate` also writes through the module's `README.md` symlink into the generated integration page;
  leave that page unstaged (`ibm-d.md`).
- Validate locally with `gen_integrations.py`, `gen_docs_integrations.py --check`, and the unit tests
  (`integrations/README.md` lists commands and dependencies). Use `how-tos/preview-collector-page.md` for rendered
  collector prose inspection in scratch, or its non-collector route for other integration types. Full regeneration or convergence checks belong in an isolated source
  copy containing current inputs. Preserve pre-existing modified and untracked generated files; keeping outputs out of
  the commit does not require discarding them. Never use a blanket restore to clean up validation.
- `.github/workflows/check-markdown.yml` regenerates the pages on pull requests to validate Learn ingest and links; it
  does NOT assert that regeneration leaves the checkout clean, so an uncommitted regeneration diff never fails a source
  PR. Earlier guidance that described a clean generated diff as a PR gate was wrong.
- Gitignored catalogs (`integrations/integrations.js`, `integrations/integrations.json`, `integrations/taxonomy.json`)
  and the untracked NPM side report `src/go/plugin/go.d/collector/snmp/npm-catalog/metrics-metadata-gaps.txt` are never
  committed. Before opening the PR run:

  ```bash
  git diff --cached --name-only --diff-filter=ACMRT -- \
    integrations/integrations.js integrations/integrations.json integrations/taxonomy.json \
    src/go/plugin/go.d/collector/snmp/npm-catalog/metrics-metadata-gaps.txt
  ```

  The command MUST print nothing. If it names a catalog or the side report, exclude that artifact from the staged
  change while preserving its local contents under the repository Git rules. Unstaged or untracked reports can remain
  for inspection; this check is about the proposed commit, not cleanup of the working tree.

## The dormant collector taxonomy

`taxonomy.yaml` files next to some `metadata.yaml` files, `integrations/gen_taxonomy.py`, `gen_taxonomy_seed.py`,
`check_collector_taxonomy.py`, `integrations/taxonomy/`, the three `taxonomy_*.json` schemas, and
`integrations/tests/test_taxonomy.py` are an early implementation of a collector dashboard taxonomy that is unused
today, will change substantially, and is kept in the tree for that later work. No workflow runs any of it (removed
2026-09-06). Do not author, extend, or seed `taxonomy.yaml` for a new collector; leave the existing files alone unless
the redesign touches them; `metrics.dynamic_collect_plugins` in `metadata.yaml` is read only by this tooling, and
`metrics.dynamic_context_prefixes` also feeds the catalogue's `contexts` export.

## What is enforced today

- `gen_integrations.py` validates each `metadata.yaml` against its JSON Schema (fatal on any warning) and every page
  meta description (`pipeline.md`, "Stage 1: outputs").
- `integrations/tests/test_collector_metadata.py` (both integration workflows): a collector named by a service-discovery
  rule has nonempty auto-detection text, with explicit exceptions; selected prose fields pass common Markdown-pattern
  checks. This is not factual verification or a complete MDX build.
- `integrations/tests/test_descriptions.py` (both workflows): the generated page meta descriptions
  (`description-authoring.md`) resolve, validate, and are unique.
- `integrations/tests/test_health_alerts.py` (both workflows, which also run on `src/health/health.d/**` changes): the
  alert ownership rule and its invariants (the metadata skill's `alerts-and-meta.md`, section 1; the test's `UNOWNED`
  names are the only owner-less alerts besides Agent alerts, and an entry that gains an owner fails). It also pins the
  `alerts` export parser to `health.d`, requires every stock alert to set `class` and `type` from the vocabulary in
  `health_alerts.py` and a `component` without case or plural spelling variants, keeps that vocabulary equal to the
  `src/health/REFERENCE.md` tables, and requires every `calc`, `warn` and `crit` variable on a documented target to
  be a documented dimension name (also with a `_raw` or `_last_collected_t` suffix), an alert name, a listed chart or
  host variable (`CHART_VARIABLES`, `HOST_VARIABLES`) or a `context.dimension` reference, checked only when that
  context is documented; health built-ins and the `green`/`red` a rule sets are exempt.
- `integrations/tests/test_context_owners.py` (both workflows): the `contexts` export rules (`in-app-contract.md`);
  every declared `dynamic_context_prefixes` entry is exported, and no exported prefix covers a context another module
  documents outside that module's longer prefixes.
- `collecttest.AssertConfigSchemaMatchesMetadata` (opt-in, per collector test): option descriptions and tabs agree
  between `config_schema.json` and `metadata.yaml`.
- `collecttest` artifact checks (opt-in, per collector test;
  `.agents/skills/collectors-go-framework-v2/chart-template.md#tests`): metric rows against the chart template, and
  stock alert targets, units, lookup dimensions and the dimensions their expressions read against the documented
  metrics.
- The ibm.d runtime-output drift gate ("Delivery boundary" above).
- Not enforced anywhere: metric rows against the code and option names against the schema for collectors that have
  not opted in, stock conf defaults against the schema. `check-markdown.yml` checks that generated links resolve on
  Learn, not that sources are in sync. `integrations/check_collector_metadata.py` is broken and unused
  (`gotchas.md`). Until such checks exist, these are review-time checks.
- ibm.d modules are the exception: docgen generates `metadata.yaml` and `config_schema.json` from `contexts.yaml`,
  `config.go`, and `module.yaml`, so those agree by construction; the stock `.conf` and `health.d/<...>.conf` still need
  manual sync (`ibm-d.md`).

## What reviewers should check

These checks describe the resulting artifacts. For producer-generated metadata, inspect authoritative inputs and
current generated validation evidence; the delivery boundary does not require that metadata in the source commit.
Review follows `AGENTS.md#skill-selection` and does not itself regenerate or publish outputs.

1. **Code changes have matching `metadata.yaml` changes.** A chart, dimension, label, or unit change in the code appears
   in `metadata.yaml`; a renamed metric changes both files. Row content (rows mirror the code's context, title, unit,
   type, and dimensions): `.agents/skills/collectors-metadata-yaml/metrics.md`.
2. **Config changes propagate to all four config-related files**: the Go struct field (`config.go`),
   `config_schema.json` (type, default, validation), the stock `.conf` (a representative example), and `metadata.yaml`
   (`setup.configuration.options.list`). The option's `group` and the DynCfg tab MUST name the same thing and the option
   `description` MUST be identical in both files. The rules (tab and group naming, the `Tab / Subgroup` form, order,
   deriving groups from the collector's own keys, the opt-in `collecttest.AssertConfigSchemaMatchesMetadata` call) are
   owned by `.agents/skills/collectors-go-design/config-schema.md` sections 3 and 8; the metadata side of a row by
   `.agents/skills/collectors-metadata-yaml/setup.md`.
3. **Alerts keep an owner.** An alert added, removed, or retargeted in `health.d` reaches the pages by derivation: its
   `on:` context is declared in the owning collector's `metadata.yaml` metrics, or the alert is listed by name where
   the context cannot be declared. An integration whose monitored instance lacks the data an alert reads excludes it
   by name (`excluded_alerts`). Rule: `.agents/skills/collectors-metadata-yaml/alerts-and-meta.md` section 1.
4. **README handling.** A plugin directory with one integration has a README symlinked to the generated
   `integrations/<slug>.md`, which follows `metadata.yaml`. A directory with several integrations has a hand-written
   README the author updates. `agent_notification` is special: the README itself is the generated artifact (no
   `integrations/` subdirectory).
5. **Umbrella pages.** Adding or removing a collector, secret store, or discoverer changes
   `src/collectors/COLLECTORS.md`, `SECRETS.md`, or `SERVICE-DISCOVERY.md` respectively; delivery boundary above.
6. **Generated artifacts are outputs, not source.** Files with a `DO NOT EDIT THIS FILE DIRECTLY` or `<!--startmeta ...
   message: "DO NOT EDIT..." -->` banner are regenerated from their sources, never hand-edited; so are the three
   umbrella pages, which carry no banner (`pipeline.md`).

## Anti-patterns to flag in review

- "I only changed the code; the docs can be a follow-up PR." No for source artifacts: metadata, schema, stock config,
  alerts, and hand-written docs move with the behavior. Only generated pages use the post-merge route.
- "The integration page on Learn doesn't show my new option." Verify `metadata.yaml` changed and the post-merge
  regeneration PR completed.
- "I edited `integrations/<slug>.md` to fix a description." No: it is generated. Edit `metadata.yaml` and regenerate.
- "I edited `metadata.yaml` for an ibm.d module." No: edit `contexts.yaml`, `config.go`, or `module.yaml` and run `go
  generate`.
- "I changed a default in the stock `.conf` only." Update the `config_schema.json` `default`, the `metadata.yaml` option
  `default_value`, and the authoritative documentation source in lockstep.
