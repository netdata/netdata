# Gotchas

Every surprise, dead-code reference, hardcoded marketing
anchor, custom Jinja delimiter, undocumented behavior, and
edge case the integrations pipeline carries today. Read this
before assuming the code does the obvious thing.

## Dead / broken code in the pipeline

### `integrations/check_collector_metadata.py` is broken

- File path: `integrations/check_collector_metadata.py`.
- Line 8 imports `SINGLE_PATTERN`, `MULTI_PATTERN`,
  `SINGLE_VALIDATOR`, `MULTI_VALIDATOR` from `gen_integrations`.
- **None of those names exist in `gen_integrations.py`
  today.** The current names are `METADATA_PATTERN` (single
  pattern) and `COLLECTOR_VALIDATOR` (single validator).
- Therefore: any attempt to run
  `python3 integrations/check_collector_metadata.py <path>`
  exits with `ImportError`.
- It is referenced nowhere actionable: no workflow under
  `.github/workflows/`, no script under `packaging/cmake/`.
  Only mentioned in `integrations/README.md` style prose.
- Its body has a second bug: `'{ check_path } is a valid
  collector metadata file.'` (line 84) -- the f-string `f`
  prefix is missing, so the literal `{ check_path }` would be
  printed even if the imports worked.
- **Treat this file as dead code.** Do NOT rely on it.
  Followup: SOW will track repair-or-delete.

### `gen_doc_service_discovery_page.py` is NOT in CI

- File exists, runs correctly when invoked manually.
- Workflow `.github/workflows/generate-integrations.yml`
  runs only `gen_integrations.py`, `gen_docs_integrations.py`,
  `gen_doc_collector_page.py`, `gen_doc_secrets_page.py`
  (`generate-integrations.yml:48-63`).
- Workflow `.github/workflows/check-markdown.yml` has the
  same gap.
- Consequence: `src/collectors/SERVICE-DISCOVERY.md` drifts
  from source `metadata.yaml` until a developer manually
  runs `python3 integrations/gen_doc_service_discovery_page.py`.
- Followup: SOW will track adding the script to both
  workflows.

### `integrations/schemas/distros.json` is unused

- Schema declared and well-formed.
- `gen_integrations.py:1330` calls `load_yaml(DISTROS_FILE)`
  WITHOUT validation.
- `DEPLOY_VALIDATOR` is for `deploy.yaml` only, NOT for
  `distros.yml`.
- Garbage in `.github/data/distros.yml` produces broken
  `platform_info` tables silently.
- Followup: SOW will track wiring `distros.json` into
  `gen_integrations.py:1330`.

## Custom Jinja delimiters

`gen_integrations.py:233-238` configures Jinja with custom
delimiters:

| Default | This pipeline |
|---|---|
| `{{ ... }}` | `[[ ... ]]` |
| `{% ... %}` | `[% ... %]` |
| `{# ... #}` | `[# ... #]` |

Why: so that templates can pass `{% details %}`,
`{% relatedResource %}`, `{% if $showClaimingOptions %}`,
`{{ ... }}` markers through to the rendered output verbatim
(those markers are the cloud-frontend's renderer's syntax,
not Jinja's).

Documented in `integrations/templates/README.md:12-15` (which
itself is partly stale -- see below).

## Two-pass templating with `meta.variables`

`gen_integrations.py:930-934`. When a metadata entry declares
`meta.monitored_instance.variables` (collectors) OR
`meta.variables` (other types), the FIRST pass renders the
section; if the rendered output still contains
`[[ variables.foo ]]` markers, a SECOND Jinja pass is run
with `variables=...` in context.

**Divergence**: collectors look up
`monitored_instance.variables`; exporters and notifications
look up `meta.variables`. Same goal, different lookup path.
A wart.

## `{% relatedResource %}` two-pass resolution

Pass 1: templates emit literal
`{% relatedResource id="..." %}name{% /relatedResource %}`
markers. See `integrations/templates/overview/collector.md:42`
and `integrations/templates/related_resources.md:5`.

Pass 2: `gen_docs_integrations.py:resolve_related_links`
(`:56-78`) runs AFTER all per-integration `.md` files are
written. It replaces the markers with `[name](/path)` markdown
links using a global `id_to_path` map built from the
just-written files.

**Silent fallback**: if the marker's `id` is not found in the
map, the marker is replaced with bare `name` text (no link).
No warning.

## `clean=False` vs `clean=True` divergence

Every render keys section is rendered TWICE
(`gen_integrations.py:805-947` for collectors, similar for
others):

- `clean=False` -- preserves `{% details %}` markers; goes
  into `integrations.js` for the cloud-frontend dashboard.
- `clean=True` -- strips folding/details markers; goes into
  `integrations.json` AND into the per-integration `.md`
  files via `gen_docs_integrations.py:50-52`.

Consumers:

- Cloud-frontend reads `.js` -> rich variant with markers.
- Per-integration `.md` files (committed, viewed on Learn /
  GitHub) -- clean variant.
- `gen_doc_collector_page.py`, `gen_doc_secrets_page.py`,
  `gen_doc_service_discovery_page.py`,
  `gen_docs_integrations.py` all parse `integrations.js`,
  NOT `integrations.json` -- so they see the rich variant
  but emit the clean variant downstream.

## Slug rules diverge by integration type

- Most types: slug = `clean_string(meta.name)` ->
  `<plugin-dir>/integrations/<slug>.md`.
- **Secretstore: slug = `clean_string(meta.kind)`**
  (`gen_docs_integrations.py:640`). The kind matches the
  runtime config filename `/etc/netdata/go.d/ss/<kind>.conf`.
- **Service-discovery: slug = `clean_string(meta.kind)`**
  (`gen_docs_integrations.py:655`). Same reason -- the kind
  is the discoverer registry name.
- **Collector custom_edit_url under `/integrations/functions/`**:
  filename uses the function slug from the URL stem (with `-`
  -> ` `) instead of `monitored_instance.name`. Avoids
  collisions when many integrations share a "Top Queries"
  label.

`clean_string` rules (`gen_docs_integrations.py:118-126`):
1. lowercase;
2. spaces -> `_`;
3. `/` -> `_`;
4. drop `(`, `)`, `,`, `'`, backtick, `:`.

So `Apache Kafka` -> `apache_kafka`. `Citrix/NetScaler` ->
`citrix_netscaler`. Note that these characters are stripped
silently; if two source names collide post-cleanup, one
overwrites the other (not warned about).

## `make_id` allows uppercase

`gen_integrations.py:768`:
`monitored_instance.name.replace(' ', '_')`.

So `Apache Kafka` becomes id segment `Apache_Kafka` and full
id `go.d.plugin-kafka-Apache_Kafka`. Mixed case preserved;
only spaces translated. Not URL-safe in the strict sense
(uppercase).

## Schemas are NOT strict

`additionalProperties: false` is NOT set on most schemas. Two
known undocumented fields that pass through silently:

- `alternative_monitored_instances` -- seen in
  `src/go/plugin/go.d/collector/postgres/metadata.yaml:21`.
- `most_popular` -- seen in ibm.d-generated `metadata.yaml`
  and elsewhere.

Neither is in `collector.json`. They appear in
`integrations.js` but no template renders them. They are
harmless but misleading -- maintainers may assume they do
something.

## `global` scope renamed to `<instance> instance`

`gen_integrations.py:914-916`. Many `metadata.yaml` files
declare `metrics.scopes:` with `name: global`. The renderer
rewrites this in-place to `<monitored_instance.name> instance`
before templating. The original file stays as `global`.

## Default-categories fallback

`gen_integrations.py:906-908`. If a collector's declared
categories are all bogus (none match `categories.yaml`), the
renderer falls back to ALL categories with
`collector_default: true` from `categories.yaml`. Currently
only `data-collection.applications` (`categories.yaml:50-52`)
is so flagged. So a typo in a collector's categories silently
parks the integration under "Applications".

## `agent_notification` writes README directly

For every type EXCEPT `agent_notification`, the per-integration
`.md` lives under `<dir>/integrations/<slug>.md` and a
`README.md` symlink is made when there is exactly one
integration.

For `agent_notification`, the script writes the per-integration
file DIRECTLY to `<dir>/README.md` (`gen_docs_integrations.py:488-496`).
No `integrations/` subdirectory, no symlink. So
`src/health/notifications/email/README.md` is the generated
artifact, NOT a hand-written README. The
`<!--startmeta` banner is the giveaway.

## Symlink only fires when exactly one integration

`gen_docs_integrations.py:466`:
`len(list(integrations_dir.iterdir())) == 1`. If a directory
has multiple integrations (rare), no top-level `README.md`
symlink is created -- the parent's existing README is left
alone (or absent).

## Hardcoded marketing anchors that don't exist

`gen_doc_collector_page.py:_render_tech_navigation` (`:424-493`)
writes navigation links like:
`#cloud-provider-managed`, `#kubernetes`, `#search-engines`,
`#freebsd`, `#message-brokers`, `#linux-systems`,
`#windows-systems`, `#macos-systems`.

None of those category IDs exist in `categories.yaml`. The
real categories are `databases`, `web-servers-and-proxies`,
`containers-and-vms`, `operating-systems`, `networking`,
`cloud-and-devops`, `hardware-and-sensors`, `applications`,
`storage`, `synthetic-testing`. Several of the hardcoded
anchors point at non-existent sections in `COLLECTORS.md`.

The header text "850+ integrations" is also a baked literal.

## Static prose baked into Python scripts

`gen_doc_secrets_page.py:20-203`: the `SECRETS_PAGE` dict
contains the bulk of `SECRETS.md`. Only the "Supported
Secretstore Backends" table is dynamic.

`gen_doc_service_discovery_page.py:21-257`: the `SD_PAGE` dict
contains the bulk of `SERVICE-DISCOVERY.md`. Only the
discoverer table is dynamic.

`gen_doc_collector_page.py`: the marketing header in
`_render_tech_navigation` is hardcoded.

To change the static prose on any of these umbrella pages,
edit the Python script and commit.

## Scoped regen via `-c plugin/module`

`gen_docs_integrations.py:578-583`. Allows scoped cleanup +
regen:

```bash
python3 integrations/gen_docs_integrations.py -c go.d/snmp
```

NOT used by CI (CI always does full regen). Useful for fast
local iteration.

## `templates/README.md` is partly stale

`integrations/templates/README.md:30-31` mentions
`setup-generic.md`, `setup-logs.md`, `setup-secretstore.md` as
the per-type setup templates, but `setup-service_discovery.md`
was added later and is not mentioned. Not pipeline-impacting,
just out-of-date docs.

## Umbrella pages have NO DO-NOT-EDIT banner

`src/collectors/COLLECTORS.md`, `src/collectors/SECRETS.md`,
`src/collectors/SERVICE-DISCOVERY.md` all open with marketing
headers (e.g. `# Monitor anything with Netdata`). None has a
`<!--startmeta` block, none has any DO-NOT-EDIT comment.

A maintainer who edits these files directly will have their
edits silently overwritten on the next CI run (for
COLLECTORS.md and SECRETS.md). For SERVICE-DISCOVERY.md the
absence of CI wiring means manual edits stick until someone
runs the script -- giving a false sense that hand-editing is
acceptable.

## Edge case in `build_path`

`gen_docs_integrations.py:81-90` assumes `meta_yaml` URL
starts with `https://github.com/netdata/...`. For forks /
non-`netdata/netdata` sources, it would produce wrong paths.
The pipeline assumes `AGENT_REPO = 'netdata/netdata'`
everywhere (`gen_integrations.py:15`).

## `convert_local_links` rewrites all `](/...)` links

`gen_integrations.py` runs `convert_local_links` on the
`integrations.js` output, rewriting any `](/...)` link in
rendered text to absolute
`https://github.com/netdata/netdata/blob/master/...`. This
applies to body links inside per-integration content. So
metadata authors writing `](/src/foo/bar.md)` get a GitHub
link in the dashboard, not a local-relative link.

## `dependencies` in `collector.json:61-63`

Draft-7 JSON Schema `dependencies` keyword: when
`monitored_instance_name` is set on a
`related_resources.integrations.list[]` entry,
`module_name` becomes required. Correct semantics, but
non-obvious -- nothing else in the schemas uses
`dependencies`, and no commentary explains it.

## `fail_on_warnings` makes ALL warnings fatal

`gen_integrations.py:150-160`. Any single validation warning
-- duplicate id, invalid category, missing related
integration -- causes `fail_on_warnings()` to return 1, which
fails CI. Even cosmetic issues block the regeneration PR.

Warnings are deduplicated by file path; the failure message
lists each warned file.

## Cloud-notifications and authentication metadata are single-file arrays

Most types have one `metadata.yaml` per integration directory.
Two exceptions:

- `integrations/cloud-notifications/metadata.yaml` -- ONE
  file containing an ARRAY of cloud-notification entries.
- `integrations/cloud-authentication/metadata.yaml` -- ONE
  file containing an ARRAY of authentication-method entries.

The `_load_*_file` functions handle both shapes via
`if 'id' in data` branches (single entry vs array).

## ibm.d websphere subdirectories

`gen_integrations.py:35` adds
`src/go/plugin/ibm.d/modules/websphere` to `COLLECTOR_SOURCES`
separately. That's because `websphere/{jmx,mp,pmi}/` are
sub-modules each with their own `metadata.yaml`,
`module.yaml`, `contexts/`, etc. The default
`src/go/plugin/ibm.d/modules` glob would not catch them at
the right depth.

## `pip.sh` and the cmake module must stay in sync

`integrations/pip.sh` is a 2-line script:
`pip install jsonschema referencing jinja2 ruamel.yaml`. The
same four packages are listed at
`packaging/cmake/Modules/NetdataRenderDocs.cmake:21`. Both
must be updated together if the dep set changes (commented
inline in `pip.sh`).

## Logo contrast analysis makes outbound HTTP calls

`gen_integrations.py:1647-1681` annotates `<img src="https://(www\.)?netdata\.cloud/img/...">`
tags with `data-integration-logo`, `data-logo-contrast-light`,
`data-logo-contrast-dark`, `data-logo-contrast-confidence`
after fetching each logo and analyzing its luminance. The
result is cached per-URL within a single run. CI runs may
trip over rate limits or transient network errors; per-request
timeout is hardcoded.
