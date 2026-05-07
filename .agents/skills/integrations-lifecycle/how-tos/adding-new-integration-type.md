# How-to: Add a new `integration_type`

**One-line summary:** Adding a new top-level `integration_type` (peer of `collector`, `logs`, `exporter`, etc.) requires changes in eight places — schema, pipeline, templates, categories, map.yaml, and downstream consumers.

This how-to was written when adding `flows` (NetFlow / IPFIX / sFlow) as a new integration type, on the basis that flows are a different data type from metrics (just like logs are) and deserve their own Learn section and in-app sidebar entry.

## When this applies

You're adding a new fundamental data type or operational concern that:

- Has its own schema (the existing types' schemas don't fit cleanly).
- Should appear in its own Learn section (not buried under `Collecting Metrics/Collectors/<sub-category>`).
- Should appear in its own in-app sidebar entry (not as a sub-category of an existing one).
- Has multiple integration entries that share a common shape.

Examples that would qualify:

- `flows` (NetFlow / IPFIX / sFlow) — flow record collection.
- `topology` (when implemented as a dedicated catalog) — topology graph sources.

Examples that would NOT qualify (use an existing type instead):

- A new database collector — use `integration_type: collector` with `categories: [data-collection.databases]`.
- A new alert channel — use `integration_type: agent_notification` or `cloud_notification`.

## What to change

### 1. JSON schema (`integrations/schemas/<type>.json`)

Clone the closest existing schema and trim/adjust:

- For data-bearing types (something that produces records / metrics / events): start from `collector.json`. Trim fields that don't apply (e.g., flows have no `metrics` or `alerts` sections in the traditional sense).
- For thin types (a config target with overview + setup + troubleshooting): start from `logs.json` or `exporter.json`.

The schema field set drives validation. Anything not in the schema is rejected as an unknown property when `additionalProperties: false` is set on the relevant object.

### 2. Pipeline registration (`integrations/gen_integrations.py`)

There are typically 3-5 places that need updating:

- `COLLECTOR_SOURCES` / equivalent type-specific source list (or add a new constant `FLOW_SOURCES`).
- Schema dispatch (the function that picks which schema to validate against based on file location or `integration_type`).
- The render-keys table — which sections (overview, setup, troubleshooting, alerts, metrics, ...) the type renders.
- The categorisation in `integrations.js` — making sure the new type ends up in the flat `integrations` array with a correct `integration_type` field.

### 3. Pipeline rendering (`integrations/gen_docs_integrations.py`)

Add a branch for the new type in the main loop. The branch typically:

- Calls a new `mode="<type>"` variant of `build_readme_from_integration()`.
- Sets the `learn_rel_path` for the type. Two patterns exist:
  - **Hardcoded** (like `exporter`, `secretstore`, `service_discovery`): one fixed path for all entries of this type.
  - **Derived** (like `collector`, `logs`, `agent_notification`, `cloud_notification`, `authentication`): walk the category tree in `integrations.js`, then `replace()` the top-level category name with the Learn section prefix.
- Calls a new `mode="<type>"` variant of `write_to_file()` that writes the per-integration `.md` to its conventional location.
- Optionally invokes the symlink helper if the integration directory pattern matches single-integration plugins (`make_symlinks`).

Cite `gen_docs_integrations.py` line numbers for each existing type (logs, exporter, etc.) when adding a new branch — they are the authoritative templates.

### 4. Render templates (`integrations/templates/`)

Add a `<type>.md` overview template under `integrations/templates/overview/` (or alongside the other type-specific templates). Reuse `setup-generic.md` if your type's setup is generic; otherwise add a `setup-<type>.md`.

Templates use the customised Jinja2 delimiters `[[ ]]` and `[% %]` (see `gotchas.md`). Cargo-cult from the closest existing type's templates and adjust.

### 5. Top-level category (`integrations/categories.yaml`)

Add a top-level category node for the new type. For a flat structure (like `logs`):

```yaml
- id: <type>
  name: <Display Name>
  description: ""
  children: []
```

For a sub-categorised structure (like `data-collection`):

```yaml
- id: <type>
  name: <Display Name>
  description: ""
  children:
    - id: <type>.<sub>
      name: <Sub Display>
      description: ""
      children: []
```

Default to flat. Sub-categories only justify themselves when the count grows (collectors are sub-categorised because there are 488 of them; logs has 3 entries and stays flat).

### 6. Source metadata declarations

For each new integration of the type, the source `metadata.yaml` (or equivalent) carries:

- `integration_type: <type>`
- `categories: [<type>]` (or `[<type>.<sub>]` for the sub-categorised case)
- The other fields the schema requires.

If the type uses a single shared metadata file (like `integrations/logs/metadata.yaml`), declare all entries there. If the type uses per-plugin or per-module metadata files (like collectors), each lives next to its own source.

### 7. Learn site map (`docs/.map/map.yaml`)

Add a section node with an `integration_placeholder`:

```yaml
- meta:
    label: <Display Name>
  items:
    - type: integration_placeholder
      integration_kind: <kind>
```

The `<kind>` value is consumed by Learn's ingest to filter pages by their `learn_rel_path`. Keep it singular or plural to match existing conventions (`logs`, `collectors`, `exporters`, `agent_notifications`, `cloud_notifications`, `authentication`, `secretstore`, `service_discovery`).

If the type has hand-authored content alongside the catalogue entries (like the rich `docs/network-flows/` documentation that pairs with the `flows` integration type), add explicit `meta.edit_url` rows for those pages and put the `integration_placeholder` at the appropriate position in the sub-tree.

### 8. Downstream consumers

Three downstream repos may need touching, in roughly decreasing likelihood:

- **`netdata/website`** (`~/src/netdata/website`): the daily `update-integrations.yml` workflow renders marketing cards from `integrations.json`. Cards usually appear automatically. But pages that reference the type explicitly (FAQ entries, solution pages) may need rewriting if the new type changes the story (e.g., "we don't do flows" → "we do flows natively").
- **`netdata/learn`** (`~/src/netdata/learn`): no PR usually needed. Learn ingest reads `map.yaml` + the `<!--startmeta-->` markers in the generated `.md` files. Sidebar regenerates automatically. ~3 hours after netdata-repo merge.
- **`netdata/dashboard/cloud-frontend`** (`~/src/dashboard/cloud-frontend`): no PR usually needed. The catalog rendering in `src/domains/integrations/` is fully data-driven from `integrations.js`. New `integration_type` values appear automatically. Special UI (a dedicated tab elsewhere in the dashboard) is a separate concern.

## Verification checklist

After all the changes:

1. `python3 integrations/gen_integrations.py` — exits 0, regenerates `integrations.js` and `integrations.json`. The new type appears in the flat list, in the new top-level category, with the correct count.
2. `python3 integrations/gen_docs_integrations.py` — exits 0, generates per-integration `.md` files. Each has `<!--startmeta-->` with the correct `learn_rel_path`.
3. Manual inspection of one generated `.md` — frontmatter complete (`custom_edit_url`, `meta_yaml`, `sidebar_label`, `learn_status: Published`, `learn_rel_path`, `keywords`, `message: "DO NOT EDIT..."`).
4. `grep` the categories tree in `integrations.json` — the new top-level node exists with the new entries.
5. Open `learn.netdata.cloud` after the next ingest cycle (~3 hours after merge) — confirm the new section renders in the sidebar.
6. Open the in-app integrations catalog — confirm the new top-level filter appears in the sidebar with the correct entries.

## What to commit, and where

- New schema + pipeline branch + template + categories.yaml + map.yaml + skill update — one PR in the netdata repo.
- Source `metadata.yaml` for the new integrations — same PR if they live in the netdata repo (most types do).
- Website FAQ / solution-page corrections — separate PR in the website repo.
- No PR needed in learn or cloud-frontend (auto-discovers).

## How I figured this out

Spawned an Explore subagent to:

- Read the `<!--startmeta-->` blocks of one example file per existing integration type.
- Trace `learn_rel_path` computation through `gen_docs_integrations.py` (lines 199-393).
- Locate `integration_kind` placeholders in `docs/.map/map.yaml`.
- Confirm `deploy` has no Learn rendering branch.

Then wrote this how-to before implementing flows so the future work can follow the recipe instead of re-deriving it.
