# Collector Taxonomy

Collector chart taxonomy is authored in public repo source files and
generated into a dashboard-consumable JSON artifact.

## Source Files

- Collector taxonomy authoring file:
  `<collector>/taxonomy.yaml`, sibling to `metadata.yaml`.
- Section registry:
  `integrations/taxonomy/sections.yaml`.
- Icon registry:
  `integrations/taxonomy/icons.yaml`.
- Schemas:
  `integrations/schemas/taxonomy_collector.json`,
  `integrations/schemas/taxonomy_sections.json`,
  `integrations/schemas/taxonomy_output.json`.
- Generator:
  `integrations/gen_taxonomy.py`.
- Touched-collector checker:
  `integrations/check_collector_taxonomy.py`.
- Seed helper:
  `integrations/gen_taxonomy_seed.py`.

## Authoring Contract

`taxonomy.yaml` v1 is a closed schema. Unknown core keys fail
validation; extension keys must be prefixed with `x_`.

Required top-level fields:

- `taxonomy_version: 1`
- `plugin_name`
- `module_name`
- either `placements` or `taxonomy_optout`, not both

Each placement requires `id`, `section_id`, `title`, and `items`.
`section_id` is the only accepted v1 section reference.
`section_path` is rejected in authoring files. Section IDs are stable
opaque handles; hierarchy is defined by `parent_id` in
`sections.yaml`.

`items:` is an ordered recursive tree. The allowed item kinds are:

- context-string shorthand such as `mysql.queries`; this is an owning
  structural context leaf and normalizes to `type: owned_context`;
- `type: owned_context` with one literal `context`;
- `type: group` with stable hand-authored `id`, `title`, and nested
  structural `items`;
- `type: flatten`, the structural equivalent of the legacy FE
  `properties.justGroup` behavior;
- `type: selector` with one selector mechanism;
- `type: context`, a display widget that references contexts and
  requires `contexts` plus `chart_library`;
- `type: grid`, `type: first_available`, and `type: view_switch` for
  dashboard widget composition.

Strings are allowed only in structural positions: placement `items`,
`group.items`, and `flatten.items`. Grid bodies, first-available
alternatives, and view-switch branches must use explicit object
items. Nested `flatten` under `flatten.items` is rejected. The
recursion matrix is exhaustive: unlisted container/item combinations
are invalid.

`single_node:` is a sparse same-kind delta only. It may override
display, selector, or renderer fields allowed on the same item type.
It may not contain `type`, `items`, `multi_node`, `single_node`, or
change an owning item into a display widget. Whole-body single-vs-
multi differences use `type: view_switch`; `view_switch` and sparse
`single_node` are mutually exclusive on the same item.

Renderer-private payloads live only under `renderer:`. Current known
keys are `renderer.overlays`, `renderer.url_options`, and
`renderer.toolbox_elements`; future renderer-only additions must use
`x_*` under `renderer`. `toolbox_elements`, `overlays`, and
`url_options` are not valid item-body siblings.

## Selectors And Context References

Every literal context owned by `owned_context` or referenced by a
`context` widget must exist in the owning collector's `metadata.yaml`
under `metrics.scopes[].metrics[].name`, unless the exact reference
uses the explicit unresolved-reference escape hatch:

```yaml
contexts:
  - context: mysql.future_context
    unresolved:
      reason: staged downstream rollout
      owner: cloud-frontend
      expires: "2026-08-01"
```

Dynamic collectors must opt in from metadata:

```yaml
metrics:
  dynamic_context_prefixes:
    - prefix: snmp.
      reason: SNMP profiles emit vendor-specific contexts at runtime.
  dynamic_collect_plugins:
    - plugin: statsd.plugin
      reason: statsd synthetic charts are operator-defined.
```

`type: selector` owns the contexts it resolves from `context_prefix`
or `collect_plugin`. Selector objects inside a widget `contexts:`
array reference contexts but do not own them. `context_prefix_exclude`
is valid only on the same item/reference that also has
`context_prefix`. A `context_prefix:` value may narrow a declared
metadata dynamic namespace; for example, a collector that declares
`dynamic_context_prefixes: [{prefix: snmp., ...}]` may use
`context_prefix: [snmp.device_prof_]` in taxonomy authoring.

`collect_plugin:` selects by Agent `_collect_plugin` label. It is for
dynamic contexts that do not share a stable context-name prefix.

## Output Artifact

`integrations/gen_taxonomy.py` emits gitignored
`integrations/taxonomy.json`:

- `taxonomy_schema_version`
- `source.netdata_commit`
- `source.generated_at`
- normalized `sections`
- normalized `placements`
- `opted_out_collectors`

Each placement and item includes:

- `resolved_contexts`: contexts owned by structural strings,
  `owned_context`, and selector items.
- `referenced_contexts`: contexts referenced by display widgets,
  grids, first-available alternatives, and view-switch widget
  branches.
- `unresolved_references`: explicit unresolved-reference escape
  hatches with `context`, `reason`, `owner`, `expires`, and
  `item_path`. This is the durable signal that a widget reference is
  intentionally unresolved instead of accidentally missing. `expires`
  uses `YYYY-MM-DD`.

The snapshots are deterministic for identical repository input and
preserve author item order for the recursive tree.

## CI Contract

Pull requests run `integrations/check_collector_taxonomy.py` from
`.github/workflows/check-markdown.yml`. The checker:

- validates all committed `taxonomy.yaml` files;
- validates the generated artifact shape;
- fails when a PR adds/removes a collector `taxonomy.yaml`;
- fails when a PR edits a collector `metadata.yaml` metrics block
  without a sibling `taxonomy.yaml`.

The master regeneration workflow runs `gen_taxonomy.py` and removes
the gitignored artifact during cleanup.

## Finding Codes

Active v1 codes:

| Code | Severity | Meaning |
|---|---|---|
| TAX001 | fatal | Schema/load failure or missing matching metadata. |
| TAX002 | reserved | Reserved for a future empty-effective-node lint; current empty authoring shapes fail schema/load validation as TAX001. |
| TAX003 | fatal | Literal context is not declared by the owning collector metadata. |
| TAX006 | fatal | Duplicate section or placement ownership key. |
| TAX021 | fatal | Invalid `single_node` override key or shape. |
| TAX022 | fatal | `multi_node:` used outside `type: view_switch`. |
| TAX023 | fatal | List-merge syntax such as `*_extend` used; v1 lists replace. |
| TAX024 | warning | Empty `single_node:` block. |
| TAX025 | warning | `single_node` override equals the top-level value. |
| TAX028 | fatal | Unknown/deprecated section, unknown icon, or invalid section authoring shape. |
| TAX029 | fatal | Invalid dynamic declaration location or `context_prefix_exclude` usage. |
| TAX030 | fatal | Touched collector needs `taxonomy.yaml` coverage. |
| TAX031 | fatal | `context_prefix` used without metadata opt-in. |
| TAX032 | reserved | Reserved for a future narrower prefix-overlap diagnostic; current selector ownership overlap conflicts emit TAX036. |
| TAX033 | fatal | Resolved context owned by more than one placement. |
| TAX034 | warning | Literal context is redundant because a prefix already covers it. |
| TAX035 | fatal | `collect_plugin` used without metadata opt-in. |
| TAX036 | fatal | Selector overlap conflict across collector/type boundaries. |
| TAX037 | fatal | Literal context is referenced by a widget but not owned by any structural item. |
| TAX038 | warning | `unresolved` escape hatch is stale because the context now resolves. |

Removed v1 codes:

- TAX026 and TAX027 were removed with `only_views`.
- TAX040 through TAX042 were removed with chart-recipe manifests.

## Contributor Rule

Collector context changes and taxonomy changes move together. A PR
that adds, removes, or renames chart contexts must update
`metadata.yaml` and `taxonomy.yaml` in the same change unless the
collector uses a declared dynamic selector that covers the context.
