# Collector consistency rule

Any change touching a collector MUST land in one source PR with matching changes to every affected authoritative
artifact (root `AGENTS.md`, "Collector Consistency"):

1. **The code**: the collector implementation files.
2. **`metadata.yaml`**: the integration page driver (field content: `.agents/skills/collectors-metadata-yaml/`).
3. **`config_schema.json`**: the dashboard's DynCfg editor (form rules:
   `.agents/skills/collectors-go-design/config-schema.md`).
4. **The stock `.conf`**: what `/etc/netdata/<plugin>/...` ships.
5. **`health.d/*.conf`**: alert definitions for the collector's metrics.
6. **The authoritative documentation source**: usually `metadata.yaml`; never a generated integration page or umbrella
   page.

A PR may legitimately not touch every file, but because most cross-artifact checks are not enforced by CI, its
description MUST enumerate the relevant artifacts and justify each one left unchanged. Any SHOULD-level exception or
escape hatch the implementation uses MUST be visible in the PR description or design note, not only in a code comment.

Obvious cases: a unit change in code updates `metadata.yaml`; a new option updates the schema, the stock conf, and the
docs; a new metric updates `metadata.yaml` and the README. Subtle ones: renaming a metric label changes the alert
definition that refers to it; changing a default changes the stock conf example and the documented default value.

## Delivery boundary: source PR versus post-merge PR

This is the one place the boundary is stated; other files point here.

- The source PR contains authoritative inputs (hand-authored `metadata.yaml`, ibm.d `contexts.yaml`/`config.go`/
  `module.yaml`, SNMP profiles and the trap catalogue), generators, schemas, templates, workflows, tests, and
  maintainer contracts.
- Generated documentation is NOT in the source PR: per-integration `<dir>/integrations/<slug>.md`, generated `README.md`
  files and symlinks, the umbrella pages `src/collectors/COLLECTORS.md`, `SECRETS.md`, `SERVICE-DISCOVERY.md`, and
  producer-generated metadata (ibm.d `metadata.yaml`, the NPM catalog `metadata.yaml`). After the source PR merges,
  `.github/workflows/generate-integrations.yml` reruns the producers and generators and opens the
  `integrations-regen` PR ("Regenerate integrations docs") with every derived change; a maintainer reviews and merges
  it. Name that delivery route in the source PR description.
- Exception, generated runtime outputs: ibm.d `contexts/zz_generated_contexts.go` (compilation) and
  `config_schema.json` (the shipped configuration contract) MUST ship in the source PR with the `contexts.yaml` or
  `config.go` change that produced them. Both workflows run `go generate` and fail on drift in those two files
  (step "Verify generated runtime outputs").
- Local regeneration is still mandatory validation: run the pipeline (`integrations/README.md` lists the commands and
  dependencies), inspect the complete generated diff, then leave every generated file unstaged.
- `.github/workflows/check-markdown.yml` regenerates the pages on pull requests to validate Learn ingest and links; it
  does NOT assert that regeneration leaves the checkout clean, so an uncommitted regeneration diff never fails a source
  PR. Earlier guidance that described a clean generated diff as a PR gate was wrong.
- Gitignored catalogs (`integrations/integrations.js`, `integrations/integrations.json`, `integrations/taxonomy.json`)
  and the untracked NPM side report `src/go/plugin/go.d/collector/snmp/npm-catalog/metrics-metadata-gaps.txt` are never
  committed. Before opening the PR run:

  ```bash
  git status --porcelain |
    rg '^(\?\?|!!| M|M |A |AM) integrations/(integrations\.(js|json)|taxonomy\.json)$' || true
  ```

  The command MUST print nothing. If it names a catalog, remove the local generated artifact from the commit rather
  than committing it.

## The collector taxonomy gate

The collector taxonomy (`taxonomy.yaml` next to a `metadata.yaml`, `integrations/gen_taxonomy.py`, the gitignored
`integrations/taxonomy.json`) is a dormant proof of concept. It has no consumer, its design will change, and nothing in
this repository's skills documents how to author it. Its PR gate is still live, though:

- `integrations/check_collector_taxonomy.py --pr-diff <base>...<head>` (step "Check Collector Taxonomy" in
  `check-markdown.yml`) emits fatal `TAX030` when the PR adds or deletes a collector `metadata.yaml` under the
  collector source roots, or changes a module's set of metric contexts (`metrics.scopes[].metrics[].name`) or its
  `metrics.dynamic_*` declarations, and the collector directory has no `taxonomy.yaml`.
- To satisfy it, generate a minimal file from the metadata and commit it with the collector:

  ```bash
  python3 integrations/gen_taxonomy_seed.py <collector-dir>/metadata.yaml --module-name <module> \
    --section-id <id from integrations/taxonomy/sections.yaml> --placement-id <module> --icon <icon id>
  python3 integrations/gen_taxonomy.py --check-only
  ```

  Reseed after adding, removing, or renaming contexts. Do not invest in richer taxonomy layouts.
- Prose-only metadata edits do not trigger the gate.

## What is enforced today

- `gen_integrations.py` validates each `metadata.yaml` against its JSON Schema only (fatal on any warning).
- `integrations/tests/test_collector_metadata.py` (both integration workflows): a collector named by a
  service-discovery rule documents its auto-detection; prose fields contain no Markdown that breaks the Learn build.
- `integrations/tests/test_descriptions.py` (both workflows): the generated page meta descriptions
  (`description-authoring.md`) resolve, validate, and are unique.
- `collecttest.AssertConfigSchemaMatchesMetadata` (opt-in, per collector test): option descriptions and tabs agree
  between `config_schema.json` and `metadata.yaml`.
- The taxonomy gate above, and the ibm.d runtime-output drift gate.
- Not enforced anywhere: metric rows against the code, alert rows against `health.d/*.conf`, option names against the
  schema for collectors that have not opted in, stock conf defaults against the schema. `check-markdown.yml` checks
  that generated links resolve on Learn, not that sources are in sync. `integrations/check_collector_metadata.py` is
  broken and unused (`gotchas.md`). Until such checks exist, these are review-time checks.
- ibm.d modules are the exception: docgen generates `metadata.yaml`, `README.md`, and `config_schema.json` from
  `contexts.yaml`, `config.go`, and `module.yaml`, so those three agree by construction; the stock `.conf` and
  `health.d/<...>.conf` still need manual sync (`ibm-d.md`).

## What reviewers should check

1. **Code changes have matching `metadata.yaml` changes.** A chart, dimension, label, or unit change in the code
   appears in `metadata.yaml`; a renamed metric changes both files. Row content (rows mirror the code's context,
   title, unit, type, and dimensions): `.agents/skills/collectors-metadata-yaml/metrics.md`.
2. **Config changes propagate to all four config-related files**: the Go struct field (`config.go`),
   `config_schema.json` (type, default, validation), the stock `.conf` (a representative example), and
   `metadata.yaml` (`setup.configuration.options.list`). The option's `group` and the DynCfg tab MUST name the same
   thing and the option `description` MUST be identical in both files. The rules (tab and group naming, the
   `Tab / Subgroup` form, order, deriving groups from the collector's own keys, the opt-in
   `collecttest.AssertConfigSchemaMatchesMetadata` call) are owned by
   `.agents/skills/collectors-go-design/config-schema.md` sections 3 and 8; the metadata side of a row by
   `.agents/skills/collectors-metadata-yaml/setup.md`.
3. **Alert changes have matching `metadata.yaml.alerts` entries.** An alert added, removed, or renamed in
   `health.d/<plugin>.conf` changes `metadata.yaml.modules.<m>.alerts[]`. Row content (name, `on:` context, `info`
   verbatim, link, `os`): `.agents/skills/collectors-metadata-yaml/alerts-and-meta.md`.
4. **README handling.** A plugin directory with one integration has a README symlinked to the generated
   `integrations/<slug>.md`, which follows `metadata.yaml`. A directory with several integrations has a hand-written
   README the author updates. `agent_notification` is special: the README itself is the generated artifact (no
   `integrations/` subdirectory).
5. **Umbrella pages.** Adding or removing a collector, secret store, or discoverer changes
   `src/collectors/COLLECTORS.md`, `SECRETS.md`, or `SERVICE-DISCOVERY.md` respectively; delivery boundary above.
6. **Generated artifacts are outputs, not source.** Files with a `DO NOT EDIT THIS FILE DIRECTLY` or
   `<!--startmeta ... message: "DO NOT EDIT..." -->` banner are regenerated from their sources, never hand-edited.
   The umbrella pages carry no banner and are generated all the same.
7. **The taxonomy gate is satisfied** when metric contexts or a collector were added or removed (section above).

## Anti-patterns to flag in review

- "I only changed the code; the docs can be a follow-up PR." No for source artifacts: metadata, schema, stock config,
  alerts, and hand-written docs move with the behavior. Only generated pages use the post-merge route.
- "The integration page on Learn doesn't show my new option." Verify `metadata.yaml` changed and the post-merge
  regeneration PR completed.
- "I edited `integrations/<slug>.md` to fix a description." No: it is generated. Edit `metadata.yaml` and regenerate.
- "I edited `metadata.yaml` for an ibm.d module." No: edit `contexts.yaml`, `config.go`, or `module.yaml` and run
  `go generate`.
- "I changed a default in the stock `.conf` only." Update the `config_schema.json` `default`, the `metadata.yaml`
  option `default_value`, and the authoritative documentation source in lockstep.
