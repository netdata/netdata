# Pipeline

How the generators turn source metadata into the catalog, the pages, and the umbrella pages, and what CI does with them.
Citations name files and symbols, never line numbers. Dependencies and the command list for a local run:
`integrations/README.md`.

## Stages

| Stage | Script | Reads | Writes | Tracked? |
|---|---|---|---|---|
| 0 | `integrations/gen_npm_catalog.py` | SNMP device profiles, the trap-profile catalogue, generator-owned copy | `src/go/plugin/go.d/collector/snmp/npm-catalog/metadata.yaml`; side report `metrics-metadata-gaps.txt` | metadata yes; report untracked and not gitignored |
| 0 | ibm.d `go generate` (`docgen`, `metricgen`) | `contexts.yaml`, `config.go`, `module.yaml` | `metadata.yaml`, `config_schema.json`, `zz_generated_contexts.go`; also writes through the `README.md` symlink into the generated page (`ibm-d.md`) | yes |
| 1 | `integrations/gen_integrations.py` | every source below, `integrations/categories.yaml`, `.github/data/distros.yml`, schemas, templates | `integrations/integrations.js`, `integrations/integrations.json` | gitignored |
| 2 | `integrations/gen_docs_integrations.py` | `integrations.js` only (never the JSON) | one page per integration, README symlinks | yes |
| 3 | `integrations/gen_doc_collector_page.py` | `integrations.js` | `src/collectors/COLLECTORS.md` | yes |
| 4 | `integrations/gen_doc_secrets_page.py` | `integrations.js` | `src/collectors/SECRETS.md` | yes |
| 5 | `integrations/gen_doc_service_discovery_page.py` | `integrations.js` | `src/collectors/SERVICE-DISCOVERY.md` | yes |
| none | `integrations/gen_taxonomy.py` (and `check_collector_taxonomy.py`, validation only) | `taxonomy.yaml` files, `integrations/taxonomy/` | `integrations/taxonomy.json` | gitignored; dormant and not run by CI (`consistency.md`, "The dormant collector taxonomy") |

Stage 0 MUST run before stage 1 whenever its inputs or generators change; both workflows do so, and locally a skipped
stage 0 leaves the tracked NPM metadata stale while every later stage still succeeds. Stages 2 to 5 read the same
`integrations.js`, which is gitignored and absent from a fresh checkout: `gen_docs_integrations.py` fails before
touching the tree when it is missing, and a stale one quietly regenerates pages from old data. Run everything from the
repo root: stages 2 to 5 open `integrations/integrations.js` and write their outputs by paths relative to the current
directory (`gen_integrations.py` alone resolves through `INTEGRATIONS_PATH` and `REPO_PATH` in `_common.py`). Shared
constants and helpers (`AGENT_REPO`, `COLLECTOR_SOURCES`, `METADATA_PATTERN`, `make_id`, `make_validator`, `load_yaml`,
`load_collectors`, `warn`, `fail_on_warnings`) live in `_common.py`, not in `gen_integrations.py`.
`packaging/cmake/Modules/NetdataRenderDocs.cmake` (`render-docs`) wraps stages 1 and 2 for the build system and is not a
substitute for running the scripts.

## Sources and integration types

| `integration_type` | Source YAML (`*_SOURCES`) | Schema | Render keys | Doc mode |
|---|---|---|---|---|
| `collector` | `COLLECTOR_SOURCES` in `_common.py`; each directory root is globbed one level deep (`<root>/*/metadata.yaml`), which is why nested trees appear as their own entries: `src/collectors`, `src/collectors/charts.d.plugin`, `src/collectors/python.d.plugin`, `src/collectors/guides`, `src/go/plugin/go.d/collector`, `src/go/plugin/scripts.d/collector`, `src/go/plugin/ibm.d/modules`, `src/go/plugin/ibm.d/modules/websphere`, plus the single files `src/collectors/ebpf.plugin/ebpfgo.plugin/metadata.yaml`, `src/crates/otel-plugin/metadata.yaml` (the list also names `src/crates/otel-plugin/taxonomy.yaml`, which the `*/metadata.yaml` filter in `get_metadata_entries` ignores) | `collector.json` | `alerts metrics functions overview related_resources setup troubleshooting` | `collector` |
| `flows` | `FLOWS_SOURCES`: `src/crates/netflow-plugin/metadata.yaml` | `flows.json` (`$ref` to `collector.json`) | same as collector | `flows` |
| `device` | `DEVICE_SOURCES`: the NPM catalog `metadata.yaml` (stage 0 output) | `device.json` (`$ref` to `collector.json`) | same as collector | `device` |
| `deploy` | `integrations/deploy.yaml` | `deploy.json` | none (`platform_info.md` template only) | none: lives only in `integrations.js` |
| `exporter` | `src/exporting/*/metadata.yaml` | `exporter.json` | `overview setup troubleshooting` | `exporter` |
| `agent_notification` | `src/health/notifications/*/metadata.yaml` | `agent_notification.json` | `overview setup troubleshooting` | `agent-notification` |
| `cloud_notification` | `integrations/cloud-notifications/metadata.yaml` (one file, an array) | `cloud_notification.json` | `setup troubleshooting` | `cloud-notification` |
| `logs` | `integrations/logs/metadata.yaml` (one file, four entries) | `logs.json` | `overview setup` | `logs` |
| `authentication` | `integrations/cloud-authentication/metadata.yaml` (one file, an array) | `authentication.json` | `overview setup troubleshooting` | `authentication` |
| `secretstore` | `src/go/plugin/agent/secrets/secretstore/backends/*/metadata.yaml` | `secretstore.json` | `overview setup collector_configs troubleshooting` | `secretstore` |
| `service_discovery` | `src/go/plugin/go.d/discovery/sdext/discoverer/*/metadata.yaml` | `service_discovery.json` | `overview setup services verify troubleshooting` | `service_discovery` |

Non-YAML inputs: `integrations/categories.yaml` (validated against `categories.json`); `.github/data/distros.yml`
(loaded with `load_yaml` and NOT validated, although `distros.json` exists; garbage yields silently broken
`platform_info` tables); the 17 schemas under `integrations/schemas/` (`schema-reference.md`); the Jinja templates under
`integrations/templates/` (delimiters below).

## Stage 1: validation

- Each type has a `Draft7Validator` built by `make_validator()` in `_common.py`, which resolves `./shared.json#/$defs/…`
  references through a filesystem `Registry` and registers the custom `format: netdata-balanced-parentheses` used by the
  page-description definition. A generic Draft-7 validator without that format checker silently accepts unbalanced
  parentheses; always validate through `make_validator()`.
- Every `ValidationError` becomes `warn(...)`. Warnings are fatal: `fail_on_warnings()` returns 1 after the run (also
  for duplicate ids, invalid categories, unresolvable `related_resources`), so a single cosmetic warning fails CI.
- Most schemas do not set `additionalProperties: false` (exceptions: the taxonomy schemas, the `distros.json`
  `platform_map` and platform objects, `shared.json`'s `instance.variables` and troubleshooting `errors.list[]` entry,
  and in `collector.json` `profile_coverage` and the two `metrics.dynamic_*` list entries). Unknown keys pass through
  into `integrations.js` and no template renders them (`gotchas.md`).

## Stage 1: rendering

For collectors (`render_collectors`; the other types follow the same shape):

1. `project_prometheus_profile_coverage` attaches the Prometheus profile projection
   (`how-tos/prometheus-profile-metadata.md`).
2. `make_id` builds `<plugin_name>-<module_name>-<instance>` with the instance name's spaces replaced by `_` and case
   kept; flows and device entries get ids the same way.
3. Sort (`sort_integrations`), then `dedupe_integrations` warns on duplicate ids and drops the later one.
4. `related_resources` are resolved by a cascading lookup: plugin plus module plus `monitored_instance_name`, then
   plugin plus module, then plugin alone (only when no module is given, so a module typo is not masked).
5. Categories: invalid ids are dropped with a warning (fatal at the end of the run); only a module whose `categories`
   list is empty falls back to every category flagged `collector_default: true` in `categories.yaml` (today only
   `data-collection.applications`).
6. A `metrics.scopes[].name` of `global` is rewritten to `<monitored_instance.name> instance`.
7. Each key in `*_RENDER_KEYS` is rendered twice through the section template chosen by `get_section_template_name`:
   `clean=False` (the rich variant kept under the key) and `clean=True` (the clean variant kept on a parallel object).
   When `meta.monitored_instance.variables` exists (collectors) or `meta.variables` (other types) the rendered string is
   rendered a second time as a template with `variables=` in context. The trigger is that key test, not any inspection
   of the output.
8. `_src_path`, `_repo`, `_index` are stripped before serialization.

Custom Jinja delimiters (`get_jinja_env`): `[[ ]]` for variables, `[% %]` for statements, `[# #]` for comments, so the
templates can emit the frontend's own `{% details %}`, `{% relatedResource %}`, `{% if $showClaimingOptions %}` and `{{
}}` markers verbatim (`integrations/templates/README.md`). `templates/overview.md` is a dispatcher: one `[% elif
entry.integration_type == '<type>' %][% include 'overview/<type>.md' %]` branch per type that renders an `overview` key
(`cloud_notification` has neither). A type with an `overview` render key but no branch renders an empty overview, and
its pages then fail the description preflight with "Missing description source" unless every entry carries an explicit
description. `templates/setup/sample-*-config.md` hold the per-plugin sample configuration blocks `setup-generic.md`
includes.

## Stage 1: outputs

- `integrations/integrations.js`: the `integrations/templates/integrations.js` shell with a two-line `// DO NOT EDIT
  THIS FILE DIRECTLY` banner and `export const categories = [...]; export const integrations = [...]`, holding the rich
  variant. `convert_local_links` rewrites every `](/` to `](https://github.com/netdata/netdata/blob/master/` first, so a
  metadata link written as `](/src/foo/bar.md)` reaches the dashboard as a GitHub link.
- `integrations/integrations.json`: `{categories, integrations}` with the clean variant, no banner. The dashboard's
  drift check reads it from `raw.githubusercontent.com` (`in-app-contract.md`).

Three content variants therefore exist: rich markers in `.js`, markers stripped in `.json`, and, in the tracked pages,
markers converted to HTML `<details>` by `clean_and_write` in `gen_docs_integrations.py`, which reads the rich keys.

## Stage 2: pages

`gen_docs_integrations.py` (`main`) in order: parse `integrations.js` by splitting on the two `export const` markers
(`read_integrations_js`; missing or malformed input is a hard error before anything is touched); resolve and validate
the meta description of every page across all ten documentation modes (`_validate_complete_description_corpus`, the
contract in `description-authoring.md`); with `--check`, print per-mode counts and stop; otherwise `cleanup()` and
write. Keep that order: the preflight and the input read MUST stay before cleanup so a description defect or a missing
catalog cannot delete the committed tree. `-c plugin/module` scopes cleanup and writes to one collector but still
validates the whole corpus; an unknown collector is an error, never an empty run. CI never uses `-c`.

`cleanup()` removes every `**/integrations` directory it owns, then restores the paths in `PRESERVE_FILES` (today
`src/collectors/ebpf.plugin/integrations/ebpf_dcstat.md`, kept so Learn's redirect catalog keeps resolving after the
dcstat move to ebpfgo). `check-markdown.yml` complements this with a step that deletes the new ebpfgo dcstat page while
the legacy one exists, to avoid a Learn URL collision during ingest. Drop both when the Learn catalog is republished.

Per page (`build_readme_from_integration`, `write_to_file`):

| Type | Sidebar label and slug source | Output | `learn_rel_path` |
|---|---|---|---|
| `collector` | `meta.monitored_instance.name` | `<dir>/integrations/<slug>.md` | derived from `categories[0]` through `generate_category_from_name`, `Data Collection` replaced by `Collecting Metrics/Collectors`; a path under `Network Performance Monitoring/` gets `/Integrations` appended; then `relocate_syslog_chapter` |
| `flows` | same | same | derived, e.g. `Network Performance Monitoring/Network Flows/Flow Protocols` |
| `device` | same | same (1013 pages under the NPM catalog directory) | derived plus `/Integrations`, then `relocate_syslog_chapter` |
| `exporter` | `meta.name` | same | fixed `Exporting Metrics/Connectors` (the branch also derives a path from the category that `main()` never uses) |
| `agent_notification` | `meta.name` | `<dir>/README.md` written directly: no `integrations/` subdirectory, no symlink | derived, `notifications` replaced by `Alerts & Notifications/Notifications` (`.../Agent Dispatched Notifications`) |
| `cloud_notification` | `meta.name` | `integrations/cloud-notifications/integrations/<slug>.md` | same replacement (`.../Centralized Cloud Notifications`) |
| `logs` | `meta.name` | `integrations/logs/integrations/<slug>.md` | fixed `Logs Management/Integrations` |
| `authentication` | `meta.name` | `integrations/cloud-authentication/integrations/<slug>.md` | derived, `authentication` replaced by `Netdata Cloud/Authentication & Authorization/Cloud Authentication & Authorization Integrations` |
| `secretstore` | label `meta.name`; slug `meta.kind` (matches `/etc/netdata/go.d/ss/<kind>.conf`) | `<backend-dir>/integrations/<kind>.md` | fixed `Collecting Metrics/Secrets Management/Secret Stores` |
| `service_discovery` | label `meta.name`; slug `meta.kind` (the discoverer registry name in `sdext/registry.go`; the file-based discoverers ship `go.d/sd/<kind>.conf` under the same name) | `<discoverer-dir>/integrations/<kind>.md` | fixed `Collecting Metrics/Service Discovery/Discoverer` |
| `deploy` | never rendered to a page; lives only in `integrations.js` for the in-app "Add Nodes" dialog | | |

`clean_string` (slug): lowercase, spaces to `_`, `/` to `-`, drop `(`, `)`, `:`. Two names that clean alike in one
directory overwrite each other silently. `relocate_syslog_chapter` maps `Network Performance Monitoring/Syslog from
Network Devices/Integrations` to `OpenTelemetry/Syslog from Network Devices/Integrations`; the matched string is the
category name in `categories.yaml`, so change both together. Every `integration_type` except `device` lands in its own
Learn section through an `integration_placeholder` node in `docs/.map/map.yaml` (`integration_kind` values:
`collectors`, `flows`, `exporters`, `agent_notifications`, `cloud_notifications`, `logs`, `authentication`,
`secretstore`, `service_discovery`); how the `device` pages attach is the Learn side, the `docs-learn-site-structure`
skill.

Every page opens with the block `create_frontmatter` emits (`custom_edit_url` is injected afterwards by
`add_custom_edit_url`), in this order and quoting, followed by the type's message:

```markdown
<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/<dir>/integrations/<slug>.md"   # <dir>/README.md once symlinked, see below
meta_yaml: "https://github.com/netdata/netdata/edit/master/<dir>/metadata.yaml"
sidebar_label: "<display name>"
learn_status: "Published"
learn_rel_path: "<Learn path>"
description: "<50-160 character plain-text description>"
keywords: ['k1', 'k2']                       # only when the source has keywords
message: "DO NOT EDIT THIS FILE DIRECTLY, IT IS GENERATED BY THE COLLECTOR'S metadata.yaml FILE"
endmeta-->
```

The message names the source per type: `COLLECTOR'S`, `FLOWS'`, `NPM CATALOG`, `EXPORTER'S`, `NOTIFICATION'S` (both
notification types), `LOGS'`, `AUTHENTICATION'S`, `SECRETSTORE'S`, `SERVICE DISCOVERY DISCOVERER'S`. Learn ingest reads
this block and matches `custom_edit_url` against `map.yaml`. `create_overview_banner` inserts the Community badge when
the `meta.community` key is present (its value is not read) and the Netdata badge otherwise, before the first `##`.

After all pages are written: `resolve_related_links` rewrites `{% relatedResource id="..." %}name{% /relatedResource %}`
markers into `[name](/path)` using the id-to-path map of the just-written files, falling back to the bare `name` with no
warning when the id is unknown; `make_symlinks` creates `<dir>/README.md -> integrations/<slug>.md` for every directory
that holds exactly one page (multi-integration directories keep their hand-written README) and rewrites
`<dir>/integrations/<slug>.md` self-references in the page to `<dir>/README.md`.

## Stages 3 to 5: umbrella pages

- `gen_doc_collector_page.py` writes `src/collectors/COLLECTORS.md`, the Learn "Monitor anything with Netdata" page.
  Sections are the children of `data-collection` and of `network-performance-monitoring` (Device Metrics, Network Flows,
  SNMP Traps, BGP, Licensing, Topologies, Syslog); the predicate also accepts a top-level `flows` category that no
  longer exists in `categories.yaml`, so that branch is dead and can go. Sort order is "Linux first, Other last"
  (`_get_ordered_sections`). The row description is the first sentence of the rendered `## Overview`
  (`get_integration_description`, `extract_description_from_overview` in `descriptions.py`), falling back to
  `meta.monitored_instance.description`, then `Monitor <name>`; the `description-authoring.md` contract follows from
  that. `_render_tech_navigation` hardcodes the marketing navigation;
  `integrations/tests/test_collector_page_navigation.py` requires every target to be an emitted section anchor or
  `#beyond-the-850-integrations` and pins the exact link list, so a category or heading change updates the mapping and
  the test together, and because no workflow runs that test, run it by hand (`python3 -m unittest
  integrations.tests.test_collector_page_navigation`). The "850+ integrations" header is a literal.
- `gen_doc_secrets_page.py` and `gen_doc_service_discovery_page.py` render `src/collectors/SECRETS.md` and
  `SERVICE-DISCOVERY.md` from the `secretstore` and `service_discovery` entries through `templates/secrets.md` and
  `templates/service_discovery.md` (the two templates `get_section_template_name` never returns); only the backends and
  discoverer tables are dynamic, the rest is static prose in the `SECRETS_PAGE` and `SD_PAGE` dicts. To change static
  prose, edit the script. After changing an NPM or flows category, regenerate and expect the matching section heading
  (for example `### Network Flows`) in `COLLECTORS.md`.
- None of the three umbrella pages carries a DO-NOT-EDIT banner (`COLLECTORS.md` opens with `<!--
  markdownlint-disable-file -->`); a direct edit is overwritten by the next regeneration.

## CI

`.github/workflows/generate-integrations.yml` (push to `master`, `netdata/netdata` only, path-filtered on the sources,
schemas, templates, generators, and tests above, except that `integrations/logs/metadata.yaml` is missing from the
list): stage 0 (`gen_npm_catalog.py`, `go generate` for ibm.d), the "Verify generated runtime outputs" gate (`git diff
--exit-code` on ibm.d `config_schema.json` and `zz_generated_contexts.go`, plus a check that both are tracked for every
module), stage 1, the unit test modules `test_descriptions`, `test_prometheus_profile_docs`, `test_collector_metadata`,
stages 2 to 5, `rm` of the gitignored catalogs and the NPM side report, then `peter-evans/create-pull-request` on branch
`integrations-regen` (label `integrations-update`, title "Regenerate integrations docs", token
`NETDATABOT_GITHUB_TOKEN`), and a Slack notification on failure.

`.github/workflows/check-markdown.yml` (pull requests, path-filtered on Markdown, docs, metadata, `integrations/**`, the
profile and ibm.d producer inputs): checks out the PR (with full history) and `netdata/learn`, installs Learn's
hash-pinned ingest requirements plus `pip.sh`, then runs stage 0, the same runtime-output gate, stage 1, the same three
test modules (`test_descriptions` with `LEARN_INGEST_PATH` set to Learn's ingest script), stages 2 to 5 with the dcstat
step between 2 and 3, and finally `learn/ingest/ingest.py --local-repo netdata:... --ignore-on-prem-repo
--fail-links-netdata`. It fails a PR on generation errors, test failures, or unresolved links; it does NOT compare the
regenerated pages with the committed ones. `integrations/tests/test_collector_page_navigation.py` and `test_taxonomy.py`
exist but no workflow runs them. What the two workflows mean for a source PR is in `consistency.md`, "Delivery
boundary".
