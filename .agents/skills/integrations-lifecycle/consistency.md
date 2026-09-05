# Collector consistency rule

Any change touching a collector MUST land in one source PR with matching changes to every affected authoritative
artifact (root `AGENTS.md`, "Collector Consistency"):

1. **The code**: the collector implementation files.
2. **`metadata.yaml`**: the integration page driver.
3. **`taxonomy.yaml`**: dashboard table-of-contents placement for the collector's chart contexts.
4. **`config_schema.json`**: the dashboard's DynCfg editor.
5. **The stock `.conf`**: what `/etc/netdata/<plugin>/...` ships.
6. **`health.d/*.conf`**: alert definitions for the collector's metrics.
7. **The authoritative documentation source**: usually `metadata.yaml`; never a generated integration page or umbrella
   page.

A PR may legitimately not touch every file, but because most cross-artifact checks are not enforced by CI, its
description MUST enumerate the relevant artifacts and justify each one left unchanged. Any SHOULD-level exception or
escape hatch the implementation uses MUST be visible in the PR description or design note, not only in a code comment.

Obvious cases: a unit change in code updates `metadata.yaml`; a new option updates the schema, the stock conf, and the
docs; a new metric updates `metadata.yaml`, `taxonomy.yaml`, and the README. Subtle ones: renaming a metric label
changes the alert definition that refers to it; changing a default changes the stock conf example and the documented
default value.

## What is enforced today

- `gen_integrations.py` validates each `metadata.yaml` against its JSON Schema only.
- `gen_taxonomy.py` validates committed `taxonomy.yaml` files, cross-references literal owned contexts and widget
  references against `metadata.yaml`, requires declared dynamic selectors, and emits the gitignored
  `integrations/taxonomy.json`. `check_collector_taxonomy.py` (in `check-markdown.yml`) fails a PR that touches a
  collector `taxonomy.yaml`, adds or removes one, or edits a `metadata.yaml` metrics block without a matching
  `taxonomy.yaml`; prose-only metadata edits do not trigger it, though the global validation still runs.
- `integrations/tests/test_collector_metadata.py` (both integration workflows): a collector named by a
  service-discovery rule documents its auto-detection; prose fields contain no Markdown that breaks the Learn build.
- `collecttest.AssertConfigSchemaMatchesMetadata` (opt-in, per collector test): option descriptions and tabs agree
  between `config_schema.json` and `metadata.yaml`.
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
   title, unit, type, and dimensions): `.agents/skills/project-collector-metadata/metrics.md`.
2. **Chart-context changes have matching `taxonomy.yaml` changes.** Structural `items:` entries and widget `contexts:`
   references name real contexts in the collector's `metadata.yaml`. Dynamic contexts use `type: selector` or
   selector objects with `context_prefix:` or `collect_plugin:` and the corresponding
   `metrics.dynamic_context_prefixes:` or `metrics.dynamic_collect_plugins:` declaration.
3. **Config changes propagate to all four config-related files**: the Go struct field (`config.go`),
   `config_schema.json` (type, default, validation), the stock `.conf` (a representative example), and
   `metadata.yaml` (`setup.configuration.options.list`). The option's `group` and the DynCfg tab MUST name the same
   thing and the option `description` MUST be identical in both files. The rules (tab and group naming, the
   `Tab / Subgroup` form, order, deriving groups from the collector's own keys, the opt-in
   `collecttest.AssertConfigSchemaMatchesMetadata` call) are owned by
   `.agents/skills/project-go-collector-design/config-schema.md` sections 3 and 8; the metadata side of a row by
   `.agents/skills/project-collector-metadata/setup.md`.
4. **Alert changes have matching `metadata.yaml.alerts` entries.** An alert added, removed, or renamed in
   `health.d/<plugin>.conf` changes `metadata.yaml.modules.<m>.alerts[]`. Row content (name, `on:` context, `info`
   verbatim, link, `os`): `.agents/skills/project-collector-metadata/alerts-and-meta.md`.
5. **README handling.** A plugin directory with one integration has a README symlinked to the generated
   `integrations/<slug>.md`, which follows `metadata.yaml`. A directory with several integrations has a hand-written
   README the author updates. `agent_notification` is special: the README itself is the generated artifact (no
   `integrations/` subdirectory).
6. **Generated integration documentation has an explicit delivery route.** The author MUST run the metadata pipeline
   locally for validation and leave committed generated pages unchanged in the source PR;
   `generate-integrations.yml` opens the regeneration PR after the source reaches `master`. `check-markdown.yml`
   regenerates pages for Learn link validation but does not assert a clean diff; an uncommitted regeneration diff
   does not fail the source PR.
7. **Umbrella pages.** Adding or removing a collector, secret store, or discoverer changes
   `src/collectors/COLLECTORS.md`, `SECRETS.md`, or `SERVICE-DISCOVERY.md` respectively. Validate locally; the
   post-merge workflow commits the generated output.
8. **Generated artifacts are outputs, not source.** Files with a `DO NOT EDIT THIS FILE DIRECTLY` or
   `<!--startmeta ... message: "DO NOT EDIT..." -->` banner are regenerated from their sources, never hand-edited.
9. **Gitignored generated catalogs are absent from the PR.** Before opening it, run:

   ```bash
   git status --porcelain |
     rg '^(\?\?|!!| M|M |A |AM) integrations/(integrations\.(js|json)|taxonomy\.json)$' || true
   ```

   The command MUST print nothing. If it names `integrations/integrations.js`, `integrations/integrations.json`, or
   `integrations/taxonomy.json`, remove the local generated artifact from the commit rather than committing it.

## Anti-patterns to flag in review

- "I only changed the code; the docs can be a follow-up PR." No for source artifacts: metadata, schema, stock config,
  alerts, taxonomy, and hand-written docs move with the behavior. Only generated pages use the post-merge route.
- "The integration page on Learn doesn't show my new option." Verify `metadata.yaml` changed and the post-merge
  regeneration PR completed.
- "I edited `integrations/<slug>.md` to fix a description." No: it is generated. Edit `metadata.yaml` and regenerate.
- "I edited `metadata.yaml` for an ibm.d module." No: edit `contexts.yaml`, `config.go`, or `module.yaml` and run
  `go generate`.
- "I changed a default in the stock `.conf` only." Update the `config_schema.json` `default`, the `metadata.yaml`
  option `default_value`, and the authoritative documentation source in lockstep.
- "I added a chart context but skipped `taxonomy.yaml` because the dashboard will discover it." No: add the context
  to a placement or use a declared dynamic selector.
