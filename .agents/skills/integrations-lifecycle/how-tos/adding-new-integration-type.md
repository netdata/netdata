# How-to: add a new `integration_type`

**Summary:** a new top-level `integration_type` (a peer of `collector`, `logs`, `exporter`) touches the schema, two
generators, the templates, `categories.yaml`, the source metadata, Learn's `map.yaml`, and possibly downstream repos.
`flows` (NetFlow, IPFIX, sFlow) and `device` (the NPM catalog) are the most recent additions and the models to copy.

## When this applies

A new fundamental data type or operational concern that has its own schema, deserves its own Learn section and in-app
sidebar entry, and has several entries sharing one shape. A new database collector is `integration_type: collector` with
a `data-collection.databases` category; a new alert channel is an `agent_notification` or `cloud_notification`.

## What to change

1. **Schema** (`integrations/schemas/<type>.json`). If the entries are collector-shaped, do what `flows.json` and
   `device.json` do: a five-line schema that `$ref`s `collector.json`, forked only when fields diverge. For a thin type
   (overview, setup, troubleshooting) clone `logs.json` or `exporter.json`.
2. **`gen_integrations.py`**: a `<TYPE>_SOURCES` list, a `<TYPE>_VALIDATOR` from `make_validator`, a
   `<TYPE>_RENDER_KEYS` list, a `load_<type>` and `render_<type>` pair, and the two concatenations in `main()` (rich and
   clean). Render keys are a downstream contract, not a docs choice: every section the website or cloud-frontend treats
   as content must be emitted as a Markdown string (`../in-app-contract.md`). When the schema reuses `collector.json`,
   default to the collector render keys.
3. **`gen_docs_integrations.py`**: a `mode` branch in `build_readme_from_integration` (frontmatter, message, sections,
   `learn_rel_path`), a branch in `main()` that dispatches the type, and, if the output location differs from
   `<dir>/integrations/<slug>.md`, a `write_to_file` mode. Add the type to `DOCUMENTATION_TYPES` in `descriptions.py` so
   the description preflight covers it. Two `learn_rel_path` styles exist: fixed (exporter, logs, secretstore, service
   discovery) or derived from the first category through `generate_category_from_name` (collector, flows, device,
   notifications, authentication); `../pipeline.md` has the table.
4. **Templates** under `integrations/templates/`: an `overview/<type>.md` file AND its `elif` branch in the dispatcher
   `overview.md` (without the branch the overview renders empty and the description preflight fails with "Missing
   description source"); unless `setup-generic.md` fits, a `setup-<type>.md`; custom delimiters `[[ ]]` and `[% %]`
   (`../pipeline.md`). Copy the closest type's templates.
5. **`integrations/categories.yaml`**: the type's category node. Flat by default; `collector` is sub-categorised because
   of its size, `logs` is flat. `flows` and `device` live under the existing `network-performance-monitoring` tree
   instead of a new top-level node; `gen_doc_collector_page.py` treats that tree's children as Monitor Anything
   sections.
6. **Source metadata**: `integration_type: <type>` and `categories` on every entry, one shared file (like logs) or one
   file per source directory (like collectors).
7. **Learn** (`docs/.map/map.yaml`): a section holding an `integration_placeholder` node with an `integration_kind`. The
   kind is a fixed bucket name Learn's ingest knows, not the `learn_rel_path`; the existing values are `collectors`,
   `flows`, `exporters`, `agent_notifications`, `cloud_notifications`, `logs`, `authentication`, `secretstore`, and
   `service_discovery` (keep their singular/plural style). Learn's ingest buckets pages by a fixed kind list
   (`.agents/skills/docs-learn-site-structure/mapping.md`, "Integration placeholders"; the list quoted there predates
   `flows` and `service_discovery`), so a new kind is a Learn-repo change too; `device` has no placeholder and attaches
   through the NPM chapter nodes. Map authoring and companion hand-written pages: the `docs-learn-site-structure` skill
   (`mapping.md`).
8. **Downstream**: `netdata/website` renders cards from `integrations.json` automatically, but FAQ and solution pages
   that describe the old story may need edits, and a new top-level category or section shape needs the website build run
   or emulated with the Hugo version pinned in its `netlify.toml`. Cloud-frontend is data-driven, but its content tabs
   (`src/domains/integrations/components/content/integration/tabs.js`), its Markdown renderer
   (`src/components/markdown/useRenderableTree.js`), and `scripts/checkIntegrations.js` assume Markdown strings under
   the standard keys, so inspect the generated `integrations.js` shape against them before merging.

## Verification

1. `python3 integrations/gen_integrations.py` exits 0; the new type appears in the flat list with its category.
2. `python3 integrations/gen_docs_integrations.py --check` counts the new mode; a full run writes pages whose
   `<!--startmeta` block matches the block in `../pipeline.md` in full (every key, order, quoting, the type's message).
3. Every standard content key is a string in `integrations.json`, not an object or array.
4. `python3 integrations/gen_doc_collector_page.py` if the type should appear on Monitor Anything.
5. After merge: the Learn sidebar shows the section after the next ingest; the in-app catalog shows the new filter with
   non-blank tabs.

Commit the schema, generators, templates, categories, `map.yaml`, source metadata, and this skill's tables in one PR in
this repository; website corrections go in their own PR there, and a new `integration_kind` in the Learn repository.

## How I figured this out

Traced `learn_rel_path` per mode through `build_readme_from_integration` in `integrations/gen_docs_integrations.py`,
read the `<!--startmeta` block of one generated page per type, listed the `integration_kind` values in
`docs/.map/map.yaml`, read the fixed-kind bucketing in `docs-learn-site-structure/mapping.md`, and confirmed `deploy`
has no documentation mode in `descriptions.py`. The `flows` and `device` additions are the worked examples
(`integrations/schemas/flows.json`, `device.json`, `FLOWS_SOURCES`, `DEVICE_SOURCES`).
