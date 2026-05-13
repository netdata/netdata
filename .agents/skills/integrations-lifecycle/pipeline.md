# Pipeline

This document maps the integrations pipeline end to end -- every
script, every input, every output, every CI workflow. All path
citations are repo-relative; line citations refer to the file at
HEAD of `master` at the time this skill was last updated.

## The four-stage pipeline

```
[ YAML sources ]
         |
         v
+------------------------+
| gen_integrations.py    |  (orchestrator, validator, renderer)
+------------------------+
         |
         v   reads
+------------------------+
| integrations.js (gitignored)
| integrations.json (gitignored)
+------------------------+
         |
         v
+--------------------------------+
| gen_docs_integrations.py        |  (per-integration .md files)
+--------------------------------+
         |
         v
+--------------------------------+
| gen_doc_collector_page.py       |  (src/collectors/COLLECTORS.md)
+--------------------------------+
         |
         v
+--------------------------------+
| gen_doc_secrets_page.py         |  (src/collectors/SECRETS.md)
+--------------------------------+
         |
         v
+--------------------------------+
| gen_doc_service_discovery_page.py |  (src/collectors/SERVICE-DISCOVERY.md)
+--------------------------------+   NOT in CI today -- see gotchas.md
```

All four downstream scripts read the **same** `integrations.js`
(or its data inside; details below). They run sequentially but
do not cross-talk.

## Stage 1 -- `gen_integrations.py` (orchestrator)

Repo path: `integrations/gen_integrations.py`.

### Inputs

- **Categories**: `integrations/categories.yaml` (validated
  against `integrations/schemas/categories.json` at
  `gen_integrations.py:344-355`).
- **Distros**: `.github/data/distros.yml` (loaded via
  `load_yaml` at `gen_integrations.py:1330` -- WITHOUT
  validation; the `distros.json` schema exists but is not
  consulted -- see `gotchas.md`).
- **Per-integration `metadata.yaml`** files matched by
  `METADATA_PATTERN = '*/metadata.yaml'`
  (`gen_integrations.py:25`) under nine collector source roots:

| Root | Integration types served |
|---|---|
| `src/collectors` | C plugins (apps, cgroups, diskspace, ebpf, freebsd, idlejitter, macos, proc, slabinfo, statsd, systemd-journal, tc, timex, xenstat, log2journal, charts.d, python.d) |
| `src/collectors/charts.d.plugin` | shell-based charts.d collectors |
| `src/collectors/python.d.plugin` | Python collectors (am2320, etc.) |
| `src/collectors/guides` | tutorial-style content |
| `src/go/plugin/go.d/collector` | the Go collector tree (the bulk) |
| `src/go/plugin/scripts.d/collector` | scripts.d (shell) |
| `src/go/plugin/ibm.d/modules` | ibm.d collectors (db2, mq, etc.) |
| `src/go/plugin/ibm.d/modules/websphere` | websphere/{jmx,mp,pmi}/ subcollectors -- listed separately because they are 1 level deeper |
| `src/crates/netdata-otel` | the OTEL Rust crate's collector metadata |

- **Exporters**: `src/exporting/*/metadata.yaml`
  (`gen_integrations.py:43`).
- **Agent notifications**:
  `src/health/notifications/*/metadata.yaml` (`:47`).
- **Cloud notifications**:
  `integrations/cloud-notifications/metadata.yaml` (`:51`).
- **Logs**: `integrations/logs/metadata.yaml` (`:55`).
- **Authentication**:
  `integrations/cloud-authentication/metadata.yaml` (`:59`).
- **Secretstore**:
  `src/go/plugin/agent/secrets/secretstore/backends/*/metadata.yaml`
  (`:63`).
- **Service discovery**:
  `src/go/plugin/go.d/discovery/sdext/discoverer/*/metadata.yaml`
  (`:67`).
- **Deploy**: `integrations/deploy.yaml` (`:39-41`).

- **Schemas**: `integrations/schemas/*.json` -- loaded on demand
  via `Registry(retrieve=retrieve_from_filesystem)`
  (`gen_integrations.py:163-169`). Each integration type has its
  own `Draft7Validator` instance (`:171-219`).

- **Templates**: `integrations/templates/**` -- Jinja env at
  `gen_integrations.py:230-241`. Custom delimiters: `[[ ]]`
  for variables and `[% %]` for control statements (so that
  the template can pass through embedded `{% ... %}` and
  `{{ ... }}` markers untouched). See `gotchas.md`.

### Validation behavior

For each integration type, `gen_integrations.py` runs a
`Draft7Validator.validate(...)` call (e.g. `:350`, `:372`,
`:399`, `:437`, `:485`, `:533`, `:581`, `:629`, `:677`, `:725`).
On any `ValidationError`, the script calls `warn(...)`.
**Warnings are fatal**: `fail_on_warnings()` (`:150-160`)
returns 1, causing the CI workflow to fail and abort doc
regeneration.

The validator IS strict about declared properties; it is NOT
strict about extra properties (no `additionalProperties: false`
on collector.json). Unknown keys (`alternative_monitored_instances`,
`most_popular`) pass through silently. They appear in
`integrations.js` but no template renders them. See `gotchas.md`.

### Rendering behavior

For each integration type, the script:

1. Loads the YAML(s).
2. Validates each entry against the type's JSON Schema.
3. Calls `make_id` (collectors only -- `:766`,
   `f'{plugin}-{module}-{instance}'`).
4. Computes `edit_link` from `_src_path` (`:777`).
5. Sorts by id/path/index.
6. Calls `dedupe_integrations` (`:789`); duplicate ids yield
   warnings.
7. Renders every section listed in `*_RENDER_KEYS` (`:71-122`)
   through Jinja, storing the result back on the item under
   that key. Sections come from the type's schema (e.g.
   `COLLECTOR_RENDER_KEYS = ['alerts', 'metrics', 'functions',
   'overview', 'related_resources', 'setup',
   'troubleshooting']` at `:71`).
8. Each section is rendered TWICE -- with `clean=False` (rich
   variant for the JS / cloud-frontend output) and `clean=True`
   (clean variant for the JSON / GitHub-rendered `.md`
   output). Both variants are kept in parallel `clean_*`
   lists.
9. Strips internal-only keys (`_src_path`, `_repo`, `_index`)
   before serialization.

### Two-pass templating with `meta.variables`

When a metadata entry declares
`meta.monitored_instance.variables` (collectors) or
`meta.variables` (other types), the FIRST pass produces
markdown that may still contain `[[ variables.foo ]]` markers.
The renderer detects this with a regex and performs a SECOND
Jinja pass over the rendered string with `variables=...` in
context (`:930-934`). This lets metadata authors inject
runtime-style placeholders into rendered text.

**Divergence**: collectors look up
`monitored_instance.variables`; exporters and notifications
look up `meta.variables` directly. Same goal, different lookup
path -- a known wart.

### Outputs

Two files are written (`gen_integrations.py:1311-1325`):

- `integrations/integrations.js` -- assembles the
  `integrations/templates/integrations.js` Jinja shell with
  `categories=...` and `integrations=...` JSON, then runs
  `convert_local_links` to rewrite any `](/...)` in the body to
  absolute GitHub URLs at `https://github.com/netdata/netdata/blob/master/...`.
  The first 2 lines are a banner:
  ```
  // DO NOT EDIT THIS FILE DIRECTLY
  // It gets generated by integrations/gen_integrations.py in the Netdata repo
  ```
  The body is `export const categories = [...]; export const
  integrations = [...]`.
- `integrations/integrations.json` -- pure JSON with the
  `clean` variant of `{categories, integrations}`. No banner.

Both are gitignored (`.gitignore:159-160`). They are produced
fresh on every run; in CI, the workflow `rm`s them after the
downstream scripts read them so they are NOT included in the
auto-PR.

### Commands a maintainer runs locally

```bash
cd <repo>
./integrations/pip.sh         # installs jsonschema referencing jinja2 ruamel.yaml
python3 integrations/gen_integrations.py
```

Run from the repo root. The script depends on relative paths
hard-coded in `gen_integrations.py:11-37`.

## Stage 2 -- `gen_docs_integrations.py`

Repo path: `integrations/gen_docs_integrations.py`.

### Inputs

- `integrations/integrations.js` -- the script parses it by
  string-splitting on `export const categories = ` and
  `export const integrations = ` (`:129-140`). It does NOT
  read `integrations.json`.

### Outputs

For each integration entry, the script writes either a
`<plugin-dir>/integrations/<slug>.md` file or a
`<plugin-dir>/README.md` file (depending on type). The full
mapping per integration type is in `per-type-matrix.md`. Slug
rules are in `artifacts-and-banners.md`.

After writing, the script:

1. Calls `resolve_related_links()` (`:56-78`) to convert
   `{% relatedResource id="..." %}name{% /relatedResource %}`
   markers (left in by `templates/overview/collector.md:42`
   and `templates/related_resources.md:5`) into
   `[name](/path)` markdown links. **Two-pass resolution**:
   the markers are present in pass 1; they get rewritten in
   pass 2 after every file is written so the id-to-path map
   is complete. If the id is not found, the marker is
   replaced with bare `name` text (silent fallback).

2. Calls `make_symlinks(symlink_dict)` (`:527-544`) to symlink
   `<plugin-dir>/README.md -> integrations/<sole-file>.md`
   when the directory holds exactly one integration. Only
   fires when `len(list(integrations_dir.iterdir())) == 1`
   (`:466`). Multi-integration directories are NOT
   symlinked.

3. Cleans the corresponding `**/integrations` directories
   BEFORE writing (`:19-41`), so removed integrations vanish
   from the tree.

### Scoped regen

The script accepts `-c plugin/module` to scope cleanup and
regen to one collector (`:578-583`). Useful locally:

```bash
python3 integrations/gen_docs_integrations.py -c go.d/snmp
```

NOT used by CI; CI always runs without `-c` (full regen).

## Stage 3 -- `gen_doc_collector_page.py`

Repo path: `integrations/gen_doc_collector_page.py`.

Reads `integrations/integrations.js` (`:38-47`). Walks the
category tree; the "section-level" categories are normally
children of `data-collection`, plus the top-level `flows`
category (`:82-86`). `flows` is deliberately included because
Network Flows entries cover both flow protocols and enrichment
inputs, so the Monitor Anything page must list them together
under a `Network Flows` section instead of dropping them into
`Other`.

Writes `src/collectors/COLLECTORS.md` (committed). This is the
"Monitor anything with Netdata" umbrella marketing page that
lists every collector and Network Flows integration in tabular
form, grouped by section. The write path is
`generate_collectors_md()` (`:565-584`), which renders the
header plus dynamic tables and atomically replaces the file.

### Notable behaviors

- Sort order: "Linux first, Other last" (`:285-301`); Network
  Flows follows its position in `integrations/categories.yaml`
  because it is treated as a section.
- Description extraction: `extract_description_from_overview`
  reads `## Overview` body, uses the first sentence (`:143-183`);
  falls back to `meta.monitored_instance.description`; final
  fallback `Monitor <name>`. Because this text becomes the
  Monitor Anything table description, the first sentence of the
  overview must describe the integration itself, not a setting,
  variable, default, limit, or troubleshooting detail. See
  `description-authoring.md`.
- Slug for table links: `to_slug(display_name)` -- lowercase,
  spaces to `_`, `/` to `-`, strips parentheses (`:213-215`).
- Hardcoded marketing anchors: `_render_tech_navigation`
  (`:424-493`) writes `#cloud-provider-managed`, `#kubernetes`,
  `#search-engines`, `#freebsd`, `#message-brokers`, etc.
  Several of these category IDs do NOT exist in
  `categories.yaml` -- some links go to non-existent anchors.
  See `gotchas.md`. Header literal "850+ integrations" is also
  baked in.

## Stage 4 -- `gen_doc_secrets_page.py`

Repo path: `integrations/gen_doc_secrets_page.py`.

Reads `integrations/integrations.js` (`:213-217`), filters
entries where `integration_type == 'secretstore'`, builds the
"Supported Secretstore Backends" table from each backend's
`meta.kind`, `meta.name`, `collector_configs.summary.{operand_format,
example_operand}`, and renders via
`integrations/templates/secrets.md`. Writes
`src/collectors/SECRETS.md` (committed, `:358`).

The bulk of `SECRETS.md` is **static content baked into the
script** (`SECRETS_PAGE` dict, `:20-203`). Only the backends
table is dynamic. To change the static prose, edit the script.

## Stage 5 -- `gen_doc_service_discovery_page.py`

Repo path: `integrations/gen_doc_service_discovery_page.py`.

Mirror of the secrets stage for service discovery. Reads
`integrations.js`, filters
`integration_type == 'service_discovery'`, renders via
`integrations/templates/service_discovery.md`. Writes
`src/collectors/SERVICE-DISCOVERY.md` (committed, `:382`).
Most content is static (`SD_PAGE` dict, `:21-257`).

**KNOWN GAP**: this stage is NOT wired into the
`generate-integrations.yml` workflow. CI does not run it. The
file in tree drifts from metadata.yaml until a developer runs
the script manually (or a future PR adds it to CI). See
`gotchas.md` and the SOW followups.

## CI workflow 1 -- `generate-integrations.yml`

Repo path: `.github/workflows/generate-integrations.yml`.

### Triggers

- `push` to `master` filtered by paths
  (`generate-integrations.yml:6-25`):
  - `**/metadata.yaml` (every collector / exporter / notification
    metadata)
  - `integrations/templates/**`
  - `integrations/schemas/**`
  - `integrations/categories.yaml`, `integrations/deploy.yaml`
  - `integrations/cloud-notifications/metadata.yaml`,
    `integrations/cloud-authentication/metadata.yaml`
  - the four older Python scripts (NOT
    `gen_doc_service_discovery_page.py` -- the gap)
- `workflow_dispatch` -- manual.

### Concurrency

- `integrations-${{ github.ref }}`, `cancel-in-progress: true`.

### Repo gate

- `if: github.repository == 'netdata/netdata'` -- forks do NOT
  trigger this workflow.

### Steps

1. `actions/checkout@v6` (depth 1, recursive submodules).
2. `apt install python3-venv` + `./integrations/pip.sh` to
   install Python deps.
3. `python3 integrations/gen_integrations.py`.
4. `python3 integrations/gen_docs_integrations.py`.
5. `python3 integrations/gen_doc_collector_page.py`.
6. `python3 integrations/gen_doc_secrets_page.py`.
7. **NOT** `gen_doc_service_discovery_page.py` -- gap.
8. `rm -rf go.d.plugin virtualenv integrations/integrations.js
   integrations/integrations.json` -- prevents the auto-PR from
   committing the runtime artifacts.
9. `peter-evans/create-pull-request@v8` -- branch
   `integrations-regen`, label `integrations-update`, title
   `Regenerate integrations docs`, token
   `NETDATABOT_GITHUB_TOKEN`. Reviewed and merged manually.
10. Slack failure notification on master failures.

## CI workflow 2 -- `check-markdown.yml`

Repo path: `.github/workflows/check-markdown.yml`.

### Triggers

- `pull_request` filtered by paths:
  - `**/*.md`, `**/*.mdx`
  - `docs/**`, `**/metadata.yaml`, `integrations/**`

### Steps

1. Checkout PR branch and the `netdata/learn` repo.
2. Install Python deps (`./integrations/pip.sh`).
3. Run `gen_integrations.py`, `gen_docs_integrations.py`,
   `gen_doc_collector_page.py`, `gen_doc_secrets_page.py`
   (same gap on SD page generator).
4. Run `learn/ingest/ingest.py --local-repo netdata:...
   --ignore-on-prem-repo --fail-links-netdata`
   (`check-markdown.yml:64-69`) -- validates that all
   generated markdown links resolve through Learn's ingest
   pipeline.

This workflow validates but does NOT auto-commit. It acts as
a gate on PRs. A failure here means a PR cannot merge until
the metadata or links are fixed.

## CMake target -- `render-docs`

Repo path: `packaging/cmake/Modules/NetdataRenderDocs.cmake`.

A developer-facing convenience target. When wired up by the
build system, it runs the same generator chain (with
`gen_integrations` + `gen_docs_integrations` only by default).
Useful for local validation. NOT a substitute for running the
scripts directly during active development.

## End-to-end: a single PR's flow

1. Developer edits `src/go/plugin/go.d/collector/foo/metadata.yaml`
   (and the four other consistency-rule files: `config_schema.json`,
   stock conf, `health.d/foo.conf`, `README.md`).
2. Developer runs locally:
   ```bash
   ./integrations/pip.sh
   python3 integrations/gen_integrations.py
   python3 integrations/gen_docs_integrations.py -c go.d/foo
   python3 integrations/gen_doc_collector_page.py
   python3 integrations/gen_doc_secrets_page.py
   ```
3. Developer commits the regenerated `integrations/foo.md`,
   the symlinked `README.md` (if applicable), and the updated
   `src/collectors/COLLECTORS.md` if the collector list
   changed.
4. PR is opened. `check-markdown.yml` runs, regenerates the
   same files in CI, and validates Learn ingest. If the dev's
   committed files differ from CI's regen, the PR fails.
5. Reviewer checks the five-file consistency.
6. PR merges. `generate-integrations.yml` triggers on master,
   regenerates everything, and opens an `integrations-regen`
   PR if anything is now stale (typically nothing, because the
   dev already committed the regen). Maintainer merges.
7. Cloud-frontend's own CI (in
   `${NETDATA_REPOS_DIR}/dashboard/cloud-frontend/`) runs
   `gen_integrations.py` against the new master and copies
   `integrations.js` into its source. See `in-app-contract.md`.
8. Learn's ingest pulls the new `integrations/foo.md` on its
   3-hourly schedule. See the `learn-site-structure` skill.

## End-to-end: Monitor Anything / `COLLECTORS.md`

1. `metadata.yaml` entries declare
   `meta.monitored_instance.categories`.
2. `python3 integrations/gen_integrations.py` validates those
   categories against `integrations/categories.yaml`, renders the
   integration content, and writes the runtime
   `integrations/integrations.js` catalog.
3. `python3 integrations/gen_doc_collector_page.py` reads
   `integrations/integrations.js`, groups integrations by Monitor
   Anything section, and writes `src/collectors/COLLECTORS.md`.
4. `check-markdown.yml` runs the same generator before Learn
   ingest on PRs, so broken generated `COLLECTORS.md` content
   (for example, unresolved links) blocks the PR. It does not
   diff-check that the committed `COLLECTORS.md` file is fresh.
5. `generate-integrations.yml` runs the same generator after
   metadata changes land on `master` and opens the
   `integrations-regen` PR if committed generated artifacts drift.
6. Learn ingests `src/collectors/COLLECTORS.md` as the
   "Monitor anything with Netdata" page.

For Network Flows specifically, keep the top-level `flows`
category handling in `gen_doc_collector_page.py`. Without it,
NetFlow / IPFIX / sFlow and enrichment entries will not appear
as a coherent `Network Flows` section on Monitor Anything.
