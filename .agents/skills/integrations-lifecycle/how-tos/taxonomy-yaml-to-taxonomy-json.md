# How collector taxonomy becomes `integrations/taxonomy.json`

Question answered: what is the general flow from collector
`metadata.yaml` and `taxonomy.yaml` to the generated dashboard
taxonomy artifact consumed by downstream frontend code?

## Short version

`metadata.yaml` is the metric-context source of truth. Collector
`taxonomy.yaml` files organize those contexts into the dashboard table
of contents. `integrations/gen_taxonomy.py` validates both sides
against the taxonomy registries and schemas, then emits the gitignored
`integrations/taxonomy.json` cross-repo contract.

The implementation details can evolve, but the durable model is:

1. metadata declares what metric contexts exist;
2. taxonomy declares where those contexts belong and which widgets
   reference them;
3. the generator proves the references are valid;
4. the generated JSON carries the normalized section tree, placements,
   recursive items, and context snapshots.

## Inputs

The taxonomy pipeline reads four source classes:

- Collector `metadata.yaml` files. The generator loads collector
  modules through the shared integrations loader and extracts metric
  contexts from `metrics.scopes[].metrics[].name`; see
  `integrations/gen_taxonomy.py:269-276`.
- Collector `taxonomy.yaml` files. These live next to collector
  metadata and use the closed v1 authoring schema
  `integrations/schemas/taxonomy_collector.json`.
- `integrations/taxonomy/sections.yaml`. This registry owns stable
  `section_id` targets and parentage for the generated TOC section
  tree; schema: `integrations/schemas/taxonomy_sections.json`.
- `integrations/taxonomy/icons.yaml`. This registry limits the icon IDs
  sections and placements may reference.

The field-level contract is documented in
`../schema-reference.md`. The contributor workflow is documented in
`../recipes/add-go-collector.md` and
`../recipes/update-collector.md`.

## Metadata indexing

The generator first builds metadata indexes from all known collector
metadata:

- `by_path_module`: matches a `taxonomy.yaml` file to its sibling
  `metadata.yaml` module by path, `plugin_name`, and `module_name`.
- `all_contexts`: sorted global list of known metric contexts, used for
  prefix resolution.
- `contexts_by_plugin`: contexts grouped by plugin name, used for
  `collect_plugin` selectors.
- dynamic selector guardrails from
  `metrics.dynamic_context_prefixes` and
  `metrics.dynamic_collect_plugins`.

The relevant implementation is `integrations/gen_taxonomy.py:286-315`.

This is why `metadata.yaml` is the metric source of truth: a literal
context in taxonomy authoring is valid only if the sibling metadata
module declares it. A taxonomy file can organize and reference metric
contexts; it cannot invent static metric contexts.

## Taxonomy authoring validation

Each collector `taxonomy.yaml` is loaded and validated against the
closed authoring schema before semantic validation. The schema rejects
old or ambiguous shapes such as placement-level `contexts:`,
`section_path:`, and string shorthand in display-only positions.

After schema validation, the generator checks:

- `section_id` exists in `sections.yaml`;
- icon IDs exist in `icons.yaml`;
- literal owned contexts exist in the sibling metadata;
- literal widget references exist in metadata unless they carry the
  explicit `unresolved` escape hatch;
- dynamic selectors are declared by metadata guardrails;
- display widgets reference contexts but do not own them;
- every literal widget reference is owned somewhere else unless it is
  deliberately unresolved.

The matching and semantic validation start in
`integrations/gen_taxonomy.py:745-790`. Selector and literal-reference
validation live around `integrations/gen_taxonomy.py:438-526`.

## Ownership model

The generated artifact separates ownership from display references:

- Structural strings and `type: owned_context` own literal contexts.
- Structural `type: selector` owns the contexts matched by
  `context_prefix` or `collect_plugin`.
- Containers such as `group`, `flatten`, `grid`, `first_available`, and
  `view_switch` aggregate context snapshots from their children.
- `type: context` display widgets reference contexts through
  `contexts:` but do not own them.

Generated items and placements therefore carry:

- `resolved_contexts`: contexts owned by that node after child and
  selector aggregation.
- `referenced_contexts`: contexts referenced by display widgets.
- `unresolved_references`: staged widget references that intentionally
  do not resolve yet, with `reason`, `owner`, `expires`, and
  `item_path`.

The recursive emission logic is in `integrations/gen_taxonomy.py:551-719`.
The FE-facing meaning of the generated fields is documented in
`../in-app-contract.md`.

## Output artifact

The generated artifact is `integrations/taxonomy.json`. It is validated
against `integrations/schemas/taxonomy_output.json` and is intentionally
gitignored.

Top-level shape:

```json
{
  "taxonomy_schema_version": 1,
  "source": {},
  "sections": [],
  "placements": [],
  "opted_out_collectors": []
}
```

Important output concepts:

- `sections[]` is the resolved global section registry.
- `placements[]` is the ordered list of collector-owned TOC placements.
- `placements[].items[]` is the normalized recursive item tree.
- `collector_ids` links a placement back to the integration IDs produced
  from metadata.
- `section_id` is the stable registry handle; `section_path` is the
  resolved path for consumers.

Assembly, deterministic placement sorting, and output schema validation
are handled in `integrations/gen_taxonomy.py:847-883`. Writing is handled
by the generator CLI in `integrations/gen_taxonomy.py:890-920`.

## CI flow

Pull requests run the taxonomy checker from
`.github/workflows/check-markdown.yml`. The checker:

- validates all committed taxonomy sources by building the artifact;
- enforces taxonomy coverage when a PR changes a collector
  `taxonomy.yaml`, adds/removes it, or edits metric-bearing parts of
  `metadata.yaml`;
- runs the taxonomy unit tests.

See `.github/workflows/check-markdown.yml:45-58` and
`integrations/check_collector_taxonomy.py`.

The master regeneration workflow runs `integrations/gen_taxonomy.py` as
part of the integrations regeneration job; see
`.github/workflows/generate-integrations.yml:59-68`. The generated
`taxonomy.json` is still a runtime/downstream contract artifact, not a
committed source file.

## Worked mental model

For a static collector such as MySQL:

1. `metadata.yaml` declares `mysql.queries`.
2. `mysql/taxonomy.yaml` owns `mysql.queries` in a structural item.
3. A summary grid widget may also reference `mysql.queries`.
4. The generated placement includes `mysql.queries` in
   `resolved_contexts` because it is owned, and in
   `referenced_contexts` where the widget uses it.

For a dynamic collector such as SNMP:

1. `metadata.yaml` declares a dynamic namespace such as `snmp.`.
2. `snmp/taxonomy.yaml` may use a narrower selector like
   `snmp.device_prof_` under that declared namespace.
3. Selector items own the matched context snapshot; selector references
   inside widgets reference dynamic contexts without claiming ownership.
4. The generated JSON preserves selector objects so downstream frontend
   code can resolve runtime dynamic contexts cleanly.

## How I figured this out

Files read:

- `integrations/gen_taxonomy.py`
- `integrations/check_collector_taxonomy.py`
- `integrations/schemas/taxonomy_collector.json`
- `integrations/schemas/taxonomy_output.json`
- `.github/workflows/check-markdown.yml`
- `.github/workflows/generate-integrations.yml`
- `../schema-reference.md`
- `../in-app-contract.md`

Commands used during the original analysis:

```bash
rg -n "def module_contexts|def build_metadata_indexes|def process_taxonomy_file|def emit_item|def build_taxonomy" integrations/gen_taxonomy.py
rg -n "gen_taxonomy|check_collector_taxonomy|taxonomy.json|taxonomy.yaml" .github/workflows integrations/README.md .agents/sow/specs/taxonomy.md
```
