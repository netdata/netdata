# Schema reference

Per-field reference for JSON Schemas under
`integrations/schemas/`. Each schema is JSON Schema Draft 7;
cross-refs use `./shared.json#/$defs/...` resolved by
`Registry(retrieve=retrieve_from_filesystem)`
(`gen_integrations.py:163-169`).

Tables below use these column conventions:
- **Field**: dotted path (`a.b.c[].d` for nested arrays).
- **Type**: JSON Schema type or `$ref` indication.
- **Req**: yes / no / conditional (with the condition).
- **Values**: enum values, regex constraints, `minItems`,
  `minLength`.
- **Surface**: which output(s) the field affects (learn /
  www / in-app / alerts / stock / README / none).
- **Notes**: cross-field constraints, special handling in
  `gen_integrations.py`.

If the schema declares a field but no template renders it,
"Surface: none" is recorded; the field is still validated and
serialized into `integrations.js` but never appears anywhere
visible.

`additionalProperties: false` is NOT set on most schemas, so
unknown keys pass through silently. See `gotchas.md`.

## shared.json -- building blocks

Referenced by every other schema for common structures.

### `$defs.id`

Single string field used in many schemas as an identifier.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | string | yes | `minLength: 1` | all | URL-safe identifier; deduplication key in `dedupe_integrations` (`gen_integrations.py:789`). |

### `$defs.instance`

The "what is this thing" descriptor used by every per-integration entry.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `instance.name` | string | yes | -- | learn / www / in-app | Display name. Drives slug for most types. |
| `instance.link` | string | yes | URL | learn / www | Official upstream site. |
| `instance.categories` | array<string> | yes | each must match a `categories.yaml` id | learn / www / in-app | Validated; bogus removed (`gen_integrations.py:899-912`). If none survive, falls back to `categories.yaml` entries flagged `collector_default: true` (`:906-908`). |
| `instance.icon_filename` | string | yes | -- | learn / www / in-app | Path under `${NETDATA_REPOS_DIR}/website/themes/tailwind/static/img/` (icon repo). |
| `instance.variables` | object | no | values: string / int / bool / number | all rendered text | Triggers two-pass Jinja templating; see `pipeline.md`. |

Do not use `instance.variables` or option/default text to build the
short catalog description. For collector-like integrations, the
Monitor Anything table description is extracted from the first
sentence of the generated overview, usually
`overview.data_collection.metrics_description`. See
`description-authoring.md` before writing or reviewing description
fields.

### `$defs.keywords`

Search-keyword array.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `keywords` | array<string> | yes (in most parent schemas) | -- | learn frontmatter, in-app search | Emitted in the `<!--startmeta` block as `keywords: ['k1','k2']`. |

### `$defs.short_setup`

Minimal "Setup" block. Alternative to `full_setup` for
notification-style integrations.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `short_setup.description` | string | yes (when `short_setup` used) | markdown | learn / in-app | Free-form setup text. |

### `$defs.full_setup`

The standard setup block for collectors / exporters /
authentication / secretstore / service_discovery.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `full_setup.prerequisites.list[]` | array<obj> | yes | objects with `title`, `description` | learn / in-app | Rendered as h4 sections in `setup-generic.md`. |
| `full_setup.prerequisites.list[].title` | string | yes | -- | learn / in-app | h4 text. |
| `full_setup.prerequisites.list[].description` | string | yes | markdown | learn / in-app | body. |
| `full_setup.configuration.file.name` | string | yes | -- | learn | Stock conf filename, e.g. `go.d/postgres.conf`. |
| `full_setup.configuration.file.section_name` | string | no | -- | learn | netdata.conf section, e.g. `[plugin:proc]`. |
| `full_setup.configuration.options.description` | string | yes | markdown | learn / in-app | Intro before the options table. |
| `full_setup.configuration.options.folding.title` | string | yes | -- | learn (clean strips) | Folding section title. |
| `full_setup.configuration.options.folding.enabled` | boolean | yes | -- | learn (clean strips) | Whether the section is collapsed by default. |
| `full_setup.configuration.options.list[].name` | string | yes | -- | learn / in-app | Option name (e.g. `dsn`). |
| `full_setup.configuration.options.list[].group` | string | no | -- | learn | Adds a "Group" column when present. |
| `full_setup.configuration.options.list[].description` | string | yes | markdown | learn / in-app | Short description for the table cell. |
| `full_setup.configuration.options.list[].detailed_description` | string | no | markdown | learn (anchor) | When set, table cell becomes a link to a detailed h5 section below. |
| `full_setup.configuration.options.list[].default_value` | string / number / bool | yes | -- | learn / in-app | Default value as displayed in the table. |
| `full_setup.configuration.options.list[].required` | boolean | yes | -- | learn / in-app | Yes/No column. |
| `full_setup.configuration.examples.folding` | $ref `_folding` | no | -- | learn (clean strips) | Folding for the examples block. |
| `full_setup.configuration.examples.list[].name` | string | yes | -- | learn / in-app | Example title. |
| `full_setup.configuration.examples.list[].description` | string | yes | markdown | learn / in-app | Example explanation. |
| `full_setup.configuration.examples.list[].config` | string | yes | YAML string | learn / in-app | Rendered inside a ```` ```yaml ```` fence. |
| `full_setup.configuration.examples.list[].folding` | $ref `_folding_relaxed` | no | -- | learn (clean strips) | Per-example folding override. When absent, defaults to the parent `examples.folding.enabled` (`gen_integrations.py:918-922`). |

### `$defs.troubleshooting`

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `troubleshooting.problems.list[].name` | string | yes | -- | learn / in-app | Rendered as h3. |
| `troubleshooting.problems.list[].description` | string | yes | markdown | learn / in-app | Body. |

The `troubleshooting.md` template adds debug-mode boilerplate
per plugin (e.g. `python.d.plugin`, `go.d.plugin`,
`charts.d.plugin`); see
`integrations/templates/troubleshooting.md:1-86`.

### `$defs._folding`

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `_folding.title` | string | yes | -- | learn (clean strips) | Section title. |
| `_folding.enabled` | boolean | yes | -- | learn (clean strips) | Initial collapsed/expanded state. |

### `$defs._folding_relaxed`

Same as `_folding` but only `enabled` is required; `title`
optional.

## collector.json

Top-level structure: a `plugin_name` plus a `modules:` array
where each module is one collector integration.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `plugin_name` | string | yes | -- | (cascaded into modules) | Auto-copied to each `module.meta.plugin_name` at `gen_integrations.py:381`. |
| `modules` | array<obj> | yes | -- | -- | One entry per integration. |
| `modules[].meta.plugin_name` | string | yes | -- | id / edit_link | Redundant with top-level; both must agree (no enforcement). |
| `modules[].meta.module_name` | string | yes | -- | id / stock conf basename | Matches stock conf section / filename. |
| `modules[].meta.monitored_instance` | $ref `shared.instance` | yes | -- | all | Full instance block; `name` drives slug + sidebar label. |
| `modules[].meta.keywords` | $ref `shared.keywords` | yes | -- | learn / in-app | |
| `modules[].meta.community` | boolean | no | -- | badge color | When true, badge becomes "Community" (`gen_docs_integrations.py:424`). |
| `modules[].meta.related_resources.integrations.list[].plugin_name` | string | yes (in entry) | -- | related-integrations panel | |
| `modules[].meta.related_resources.integrations.list[].module_name` | string | conditional | required if `monitored_instance_name` is set (Draft-7 `dependencies` at `collector.json:61-63`) | related-integrations | See `gotchas.md` for non-obvious dependency semantics. |
| `modules[].meta.related_resources.integrations.list[].monitored_instance_name` | string | no | -- | related-integrations | For cgroups multi-instance disambiguation. |
| `modules[].meta.info_provided_to_referring_integrations.description` | string | yes | markdown | rendered when ANOTHER collector references this one | The "what THIS collector says when referenced from another." |
| `modules[].overview.data_collection.metrics_description` | string | yes | markdown | learn / www / Monitor Anything first-sentence source | The "what we collect" prose. First sentence is the catalog description and must start with an active user-facing phrase such as `Monitor...`, `Collect...`, `Enrich network flows with...`, or `Annotate network flows with...`. Do not start with setup, variables, defaults, limits, or option names. |
| `modules[].overview.data_collection.method_description` | string | yes | markdown | learn / www | The "how we collect" prose. |
| `modules[].overview.supported_platforms.include` | array<string> | yes (may be empty) | platform names | learn (`overview/collector.md:12-26`) | Allow-list. |
| `modules[].overview.supported_platforms.exclude` | array<string> | yes (may be empty) | platform names | learn (`overview/collector.md:12-26`) | Block-list. |
| `modules[].overview.multi_instance` | boolean | yes | -- | learn (`overview/collector.md:28-32`) | Drives the multi-instance sentence. |
| `modules[].overview.additional_permissions.description` | string | yes (may be empty) | markdown | learn (`overview/collector.md:34-36`) | When non-empty, an extra paragraph. |
| `modules[].overview.default_behavior.auto_detection.description` | string | yes | markdown | learn (`overview/collector.md:46-58`) | |
| `modules[].overview.default_behavior.limits.description` | string | yes | markdown | learn (`overview/collector.md:46-58`) | |
| `modules[].overview.default_behavior.performance_impact.description` | string | yes | markdown | learn (`overview/collector.md:46-58`) | |
| `modules[].setup` | $ref `shared.full_setup` | yes | -- | learn / in-app | Rendered through `setup-generic.md` (with sample-`<lang>`-config.md per plugin). |
| `modules[].troubleshooting` | $ref `shared.troubleshooting` | yes | -- | learn / in-app | |
| `modules[].alerts[].name` | string | yes | -- | learn alerts table | |
| `modules[].alerts[].link` | string | yes | URL or repo-relative | learn alerts table | Deep link to the `health.d/<...>.conf` definition. |
| `modules[].alerts[].metric` | string | yes | metric context | learn alerts table | Must match a metric name in `metrics.scopes[].metrics[].name` (NOT enforced). |
| `modules[].alerts[].info` | string | yes | -- | learn alerts table | Short alert description. |
| `modules[].alerts[].os` | string | no | -- | learn alerts table | OS filter. |
| `modules[].metrics.folding` | $ref `_folding` | yes | -- | learn (clean strips) | Folding for the entire metrics section. |
| `modules[].metrics.description` | string | yes | markdown | learn | Intro to the metrics block. |
| `modules[].metrics.availability` | array<string> | yes | -- | metrics table column-set | Defines which "availability" columns the table will have. |
| `modules[].metrics.dynamic_context_prefixes[].prefix` | string | no | `minLength: 1` | taxonomy | Opt-in guardrail for `taxonomy.yaml` `context_prefix:` selectors. |
| `modules[].metrics.dynamic_context_prefixes[].reason` | string | no | `minLength: 1` | taxonomy | Required explanation for each dynamic context prefix. |
| `modules[].metrics.dynamic_collect_plugins[].plugin` | string | no | `minLength: 1` | taxonomy | Opt-in guardrail for `taxonomy.yaml` `collect_plugin:` selectors. |
| `modules[].metrics.dynamic_collect_plugins[].reason` | string | no | `minLength: 1` | taxonomy | Required explanation for each dynamic collect-plugin selector. |
| `modules[].metrics.scopes[].name` | string | yes | -- | learn metrics table | Special: `global` is rewritten to `<instance> instance` at `gen_integrations.py:914-916`. |
| `modules[].metrics.scopes[].description` | string | yes | markdown | learn metrics table | |
| `modules[].metrics.scopes[].labels[].name` | string | yes | -- | learn | Label name. |
| `modules[].metrics.scopes[].labels[].description` | string | yes | -- | learn | |
| `modules[].metrics.scopes[].metrics[].name` | string | yes | metric context | learn metrics table | Chart context (e.g. `postgres.connections`). |
| `modules[].metrics.scopes[].metrics[].availability` | array<string> | no | matches parent `metrics.availability` | metrics table | Drives column ticks (`metrics.md:32-37`). |
| `modules[].metrics.scopes[].metrics[].description` | string | yes | -- | metrics table | Chart title. |
| `modules[].metrics.scopes[].metrics[].unit` | string | yes | -- | metrics table | |
| `modules[].metrics.scopes[].metrics[].chart_type` | string | yes | enum: `line, area, stacked, heatmap` | metrics table | |
| `modules[].metrics.scopes[].metrics[].dimensions[].name` | string | yes | -- | metrics table | |
| `modules[].functions.description` | string | yes (when `functions` present) | markdown | learn Live Data section | Intro. |
| `modules[].functions.list[].id` | string | yes | -- | learn | Function id (matches the agent's Function name). |
| `modules[].functions.list[].name` | string | yes | -- | learn | Display name. |
| `modules[].functions.list[].description` | string | yes | markdown | learn | |
| `modules[].functions.list[].parameters[].id` | string | yes | -- | learn parameters table | |
| `modules[].functions.list[].parameters[].name` | string | yes | -- | learn parameters table | |
| `modules[].functions.list[].parameters[].description` | string | yes | -- | learn parameters table | |
| `modules[].functions.list[].parameters[].type` | string | yes | -- | learn parameters table | |
| `modules[].functions.list[].parameters[].required` | boolean | yes | -- | learn parameters table | |
| `modules[].functions.list[].parameters[].default` | string / number / bool | yes | -- | learn parameters table | |
| `modules[].functions.list[].parameters[].options[].id` | string | yes | -- | learn parameters table | When present, parameter is enum-style. |
| `modules[].functions.list[].parameters[].options[].name` | string | yes | -- | learn | |
| `modules[].functions.list[].parameters[].options[].description` | string | no | -- | learn | |
| `modules[].functions.list[].parameters[].options[].default` | boolean | no | -- | learn | |
| `modules[].functions.list[].returns.description` | string | yes | markdown | learn | |
| `modules[].functions.list[].returns.columns[].name` | string | yes | -- | learn returns table | |
| `modules[].functions.list[].returns.columns[].type` | string | yes | -- | learn returns table | |
| `modules[].functions.list[].returns.columns[].unit` | string | yes | -- | learn returns table | |
| `modules[].functions.list[].returns.columns[].visibility` | string | no | enum: `hidden` | learn returns table | When `hidden`, column is suppressed. |
| `modules[].functions.list[].performance` | string | yes | markdown | learn | Performance characteristics. |
| `modules[].functions.list[].security` | string | yes | markdown | learn | Security considerations. |
| `modules[].functions.list[].availability` | string | yes | markdown | learn | When the function is available. |
| `modules[].functions.list[].prerequisites.list[].title` | string | yes (if prereqs present) | -- | learn | h4 text. |
| `modules[].functions.list[].prerequisites.list[].description` | string | yes (if prereqs present) | markdown | learn | |
| `modules[].functions.list[].require_cloud` | boolean | no | -- | learn functions table | Yes/No column. |

Required at module root: `meta`, `overview`, `setup`,
`troubleshooting`, `alerts`, `metrics`
(`collector.json:611-618`).

Required on `meta`: `plugin_name`, `module_name`,
`monitored_instance`, `keywords`, `related_resources`,
`info_provided_to_referring_integrations` (`collector.json:94-101`).

## taxonomy_collector.json

Sibling authoring file for collector dashboard placement:
`<collector>/taxonomy.yaml`. The schema is intentionally closed
(`additionalProperties: false` plus `x_*` extension keys on core
nodes). `section_id:` is the only accepted section reference in v1;
`section_path:` is rejected.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `taxonomy_version` | integer | yes | `1` | taxonomy | Authoring schema version. |
| `plugin_name` | string | yes | -- | taxonomy | Must match owning `metadata.yaml`. |
| `module_name` | string | yes | -- | taxonomy | Must match owning `metadata.yaml` module. |
| `taxonomy_optout.reason` | string | conditional | `minLength: 1` | taxonomy | Mutually exclusive with `placements`. |
| `inline_dynamic_declarations.dynamic_context_prefixes[]` | array<object> | no | `prefix`, `reason` | taxonomy | For no-metadata plugins only. Fails when sibling metadata exists. |
| `inline_dynamic_declarations.dynamic_collect_plugins[]` | array<object> | no | `plugin`, `reason` | taxonomy | For no-metadata plugins only. |
| `placements[].id` | string | yes | `^[a-z0-9][a-z0-9_.-]*$` | taxonomy | Leaf id under the target section. |
| `placements[].section_id` | string | yes | registered section id | taxonomy | Resolved against `integrations/taxonomy/sections.yaml`. |
| `placements[].title` | string | yes | -- | taxonomy | Multi-node canonical title. |
| `placements[].icon` | string | no | registered icon id | taxonomy | Resolved against `integrations/taxonomy/icons.yaml`. |
| `placements[].families` | boolean / array<string> | no | -- | taxonomy | Preserved for the dashboard TOC consumer. |
| `placements[].items[]` | array | yes | recursive item tree | taxonomy | Ordered TOC tree; strings in structural positions own contexts. |
| `items[].type` | string | conditional | `owned_context`, `group`, `flatten`, `selector`, `context`, `grid`, `first_available`, `view_switch` | taxonomy | Plain strings normalize to `owned_context`. |
| `owned_context.context` | string | yes | real context | taxonomy | Must exist in metadata. |
| `selector.context_prefix[]` | array<string> | conditional | unique | taxonomy | Dynamic selector; requires metadata opt-in. May narrow a declared metadata namespace, e.g. `snmp.device_prof_` under declared `snmp.`. |
| `selector.context_prefix_exclude[]` | array<string> | no | unique | taxonomy | Valid only with same-node `context_prefix`. |
| `selector.collect_plugin[]` | array<string> | conditional | unique | taxonomy | Dynamic selector by `_collect_plugin`; requires metadata opt-in. |
| `context.contexts[]` | array | yes | literal context, unresolved object, or selector object | taxonomy | Widget references; literal references must resolve or carry `unresolved`. |
| `context.chart_library` | string | yes | `bars`, `d3pie`, `dygraph`, `easypiechart`, `gauge`, `groupBoxes`, `number`, `table` | taxonomy | Display widget renderer. |
| `context.group_by[]` | array<string> | no | unique | taxonomy | Widget grouping axes, e.g. `selected`, `dimension`, `label`, `node`, `context`. |
| `context.group_by_label[]` | array<string> | no | unique | taxonomy | Label names used when `group_by` includes `label`. |
| `context.aggregation_method` | string | no | `avg`, `max`, `min`, `sum` | taxonomy | Aggregation method for grouped widgets. |
| `context.selected_dimensions[]` | array<string> | no | unique | taxonomy | Explicit dimensions to show in the widget. |
| `context.dimensions_sort` | string | no | non-empty | taxonomy | FE dimension sort directive, e.g. `valueDesc`. |
| `context.colors[]` | array<string> | no | non-empty strings | taxonomy | Renderer color palette values. |
| `context.layout` | object | no | `left`, `top`, `width`, `height` | taxonomy | Grid coordinates for `grid.items` widgets. |
| `context.table_columns[]` | array<string> | no | unique | taxonomy | Table widget column axes, e.g. `context`, `dimension`. |
| `context.table_sort_by[]` | array<object> | no | `{id, desc}` | taxonomy | Table sort directives. |
| `context.labels` | object | no | string map | taxonomy | Context or dimension display labels. |
| `context.value_range[]` | array<number|null> | no | at least one item | taxonomy | Numeric renderer bounds, usually `[0, null]` or `[0, 100]`. |
| `context.eliminate_zero_dimensions` | boolean | no | -- | taxonomy | Renderer hint to hide all-zero dimensions. |
| `context.context_items[]` | array<object> | no | `{value, label}` | taxonomy | Per-widget context item labels for selector-like UI. |
| `context.post_group_by[]` | array<string> | no | unique | taxonomy | Post-aggregation grouping axes. |
| `context.show_post_aggregations` | boolean | no | -- | taxonomy | FE post-aggregation display toggle. |
| `context.grouping_method` | string | no | non-empty | taxonomy | FE grouping-method override. |
| `context.sparkline` | boolean | no | -- | taxonomy | Render compact sparkline form when supported. |
| `renderer` | object | no | `overlays`, `url_options`, `toolbox_elements`, `x_*` | taxonomy | Renderer-private payload envelope. |
| `placements[].single_node` | object | no | closed field set | taxonomy | Sparse override block; top-level fields are multi-node defaults. |

Item-kind matrix:

| Item kind | Required fields | Allowed children / references | Notes |
|---|---|---|---|
| string shorthand | string value | none | Structural positions only; normalizes to `owned_context`. |
| `owned_context` | `type`, `context` | none | Owns one literal context. |
| `group` | `type`, `id`, `title`, `items` | structural `items` | `id` is stable across title renames. |
| `flatten` | `type`, `id`, `title`, `items` | non-flatten structural `items` | Equivalent to legacy `justGroup`; nested flatten is invalid. |
| `selector` | `type`, `id`, `title`, one of `context_prefix` or `collect_plugin` | none | Owns the resolved selector snapshot. |
| `context` | `type`, `contexts`, `chart_library` | widget `contexts` references | References contexts but does not own them. |
| `grid` | `type`, `id`, `items` | `context`, `first_available`, display `view_switch` | Grid children are display-only. |
| `first_available` | `type`, `items` | `context`, `grid`, display `view_switch` | Alternatives are ordered and display-only. |
| `view_switch` | `type`, `multi_node`, `single_node` | concrete object branches except `flatten` or nested `view_switch` | Branches are whole-body replacements; no string branches. |

For a rich collector example with grids, table widgets, nested groups,
and ownership leaves, read
`src/go/plugin/go.d/collector/mysql/taxonomy.yaml`.

Widget `contexts[]` entries may be:

- a literal context string;
- an unresolved literal reference object:
  `{context, unresolved: {reason, owner, expires}}`, where
  `expires` is `YYYY-MM-DD`;
- a selector reference object with `context_prefix` or
  `collect_plugin`.

Generated output adds `unresolved_references[]` to each placement and
item that aggregates unresolved escape hatches with `context`,
`reason`, `owner`, `expires`, and `item_path`.

## taxonomy_sections.json

Schema for `integrations/taxonomy/sections.yaml`. Sections have
stable opaque `id` values and parentage through `parent_id`.
Moving a section means changing `parent_id`, not editing collector
`taxonomy.yaml` files.

Required fields per section: `id`, `title`, `section_order`,
`status`. Optional fields: `parent_id`, `short_name`, `icon`,
`deprecation`, and `x_*` extensions.

## taxonomy_output.json

Schema for generated `integrations/taxonomy.json`. The artifact
contains `taxonomy_schema_version`, `source`, normalized `sections`,
normalized `placements`, and `opted_out_collectors`. Each placement
preserves the ordered item tree and includes `resolved_contexts`
(owned contexts), `referenced_contexts` (display references), and
`unresolved_references` snapshots for CI/review diffing.

## agent_notification.json

Single object OR array of objects (oneOf).

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id / dedupe | |
| `meta` | $ref `shared.instance` | yes | -- | learn / in-app | `meta.name` drives slug. |
| `keywords` | array<string> | yes | -- | learn frontmatter | |
| `overview.notification_description` | string | yes | markdown | learn (`overview/notification.md`) | The "what gets notified" prose. |
| `overview.notification_limitations` | string | yes (may be empty) | markdown | learn | When non-empty, rendered as `## Limitations`. |
| `global_setup.severity_filtering` | boolean | yes | -- | learn | Sentence in setup template. |
| `global_setup.http_proxy` | boolean | yes | -- | learn | Sentence in setup template. |
| `setup` | oneOf [`shared.short_setup`, `shared.full_setup`] | yes | -- | learn / in-app | Rendered by `setup-generic.md` which handles both shapes. |
| `troubleshooting` | $ref `shared.troubleshooting` | no | -- | learn / in-app | |

## cloud_notification.json

Same shape as `agent_notification.json` minus `overview`
(none required), with `setup` required.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id | |
| `meta` | $ref `shared.instance` | yes | -- | learn / in-app | |
| `keywords` | array<string> | yes | -- | learn frontmatter | |
| `setup` | oneOf [`shared.short_setup`, `shared.full_setup`] | yes | -- | learn / in-app | |
| `troubleshooting` | $ref `shared.troubleshooting` | no | -- | learn / in-app | |

`integrations/cloud-notifications/metadata.yaml` is a single
file containing an ARRAY of these entries (one per
notification destination).

## authentication.json

Same shape as `agent_notification.json` with renamed `overview`
fields for the authentication context.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id | |
| `meta` | $ref `shared.instance` | yes | -- | learn / in-app | |
| `keywords` | array<string> | yes | -- | learn frontmatter | |
| `overview.authentication_description` | string | yes | markdown | learn (`overview/authentication.md`) | |
| `overview.authentication_limitations` | string | yes (may be empty) | markdown | learn | |
| `setup` | oneOf [`shared.short_setup`, `shared.full_setup`] | yes | -- | learn / in-app | |
| `troubleshooting` | $ref `shared.troubleshooting` | no | -- | learn / in-app | |

`integrations/cloud-authentication/metadata.yaml` is a single
file with an array of authentication-method entries.

## logs.json

Single entry OR array. Required: `id`, `meta`, `keywords`,
`overview`.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id | |
| `meta` | $ref `shared.instance` | yes | -- | learn / in-app | |
| `keywords` | array<string> | yes | -- | learn frontmatter | |
| `overview.description` | string | yes | markdown | learn (`overview/logs.md`) | h1 body. |
| `overview.visualization.description` | string | yes | markdown | learn | `## Visualization` section. |
| `overview.key_features.description` | string | yes | markdown | learn | `## Key features` section. |
| `setup.prerequisites.description` | string | yes (when `setup` present) | markdown | learn (`setup-logs.md`) | |

`integrations/logs/metadata.yaml` covers exactly three log
types: `systemd-journal`, `windows-events`, `OpenTelemetry`.

## secretstore.json

Per-backend entries.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id | |
| `meta.kind` | string | yes | -- | slug | **Drives slug** (NOT `meta.name`); matches stock conf filename `/etc/netdata/go.d/ss/<kind>.conf`. |
| `meta.name` | string | yes | -- | learn / in-app | Display name. |
| `meta.link` | string | yes | URL | learn / www | |
| `meta.icon_filename` | string | yes | -- | learn / www / in-app | |
| `keywords` | array<string> | yes | -- | learn frontmatter | |
| `overview.description` | string | yes | markdown | learn (`overview/secretstore.md`) | |
| `overview.limitations` | string | no | markdown | learn | |
| `setup` | $ref `shared.full_setup` | yes | -- | learn / in-app | Rendered via `setup-secretstore.md`. |
| `collector_configs.description` | string | yes | markdown | learn (`collector_configs.md`) | |
| `collector_configs.summary.operand_format` | string | yes | -- | `SECRETS.md` umbrella table | Used by `gen_doc_secrets_page.py` to build the supported-backends table. |
| `collector_configs.summary.example_operand` | string | yes | -- | `SECRETS.md` umbrella table | as above |
| `collector_configs.format.description` | string | no | markdown | learn | |
| `collector_configs.format.syntax` | string | yes | -- | learn | E.g. `${store:<kind>:<name>:<operand>}`. |
| `collector_configs.format.parts.list[].name` | string | yes | -- | learn | |
| `collector_configs.format.parts.list[].description` | string | yes | -- | learn | |
| `collector_configs.examples.list[].name` | string | yes (`minItems: 1`) | -- | learn | |
| `collector_configs.examples.list[].description` | string | yes | -- | learn | |
| `collector_configs.examples.list[].content` | string | yes | -- | learn | Code block. |
| `collector_configs.examples.list[].language` | string | no | language id | learn | Code-fence language; defaults to `text` per schema description but template uses `'yaml'`. |
| `troubleshooting` | $ref `shared.troubleshooting` | yes | -- | learn / in-app | |

## service_discovery.json

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id | |
| `meta.kind` | string | yes | -- | slug | **Drives slug**; matches discoverer registry name and stock conf filename. |
| `meta.name` | string | yes | -- | learn / in-app | |
| `meta.tagline` | string | yes | -- | SD hub table | One-liner shown in the SERVICE-DISCOVERY.md table. |
| `meta.link` | string | yes | URL | learn / www | |
| `meta.icon_filename` | string | yes | -- | learn / www / in-app | |
| `keywords` | array<string> | yes | -- | learn frontmatter | |
| `overview.description` | string | yes | markdown | learn (`overview/service_discovery.md`) | |
| `overview.how_it_works` | string | no | markdown | learn | h3 under Overview. |
| `overview.limitations` | string | no | markdown | learn | |
| `setup` | $ref `shared.full_setup` | yes | -- | learn (`setup-service_discovery.md`) | |
| `services.description` | string | yes | markdown | learn | |
| `services.evaluation.description` | string | no | markdown | learn | |
| `services.evaluation.list[].name` | string | yes | -- | learn | Evaluation criterion. |
| `services.evaluation.list[].description` | string | yes | -- | learn | |
| `services.template_variables.description` | string | no | markdown | learn | |
| `services.template_variables.list[].name` | string | yes (`minItems: 1`) | -- | learn | Discoverer-specific template var name. |
| `services.template_variables.list[].description` | string | yes | -- | learn | |
| `services.template_variables.list[].type` | string | no | -- | learn | |
| `services.examples.description` | string | no | markdown | learn | |
| `services.examples.list[].name` | string | yes (`minItems: 1`) | -- | learn | |
| `services.examples.list[].description` | string | yes | -- | learn | |
| `services.examples.list[].config` | string | yes | -- | learn | YAML code block. |
| `verify.description` | string | no | markdown | learn | |
| `verify.checks.list[].name` | string | yes (`minItems: 1` when `verify` present) | -- | learn | |
| `verify.checks.list[].description` | string | yes | -- | learn | |
| `troubleshooting` | $ref `shared.troubleshooting` | yes | -- | learn / in-app | |

Required at entry root: `id`, `meta`, `keywords`, `overview`,
`setup`, `services`, `troubleshooting` (`service_discovery.json:237-245`).

## deploy.json

Top-level: ARRAY of objects (one per deploy method).

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id | |
| `meta` | $ref `shared.instance` | yes | -- | in-app dialog | `meta.categories` must include a `deploy.*` id. |
| `keywords` | array<string> | yes | -- | in-app search | |
| `install_description` | string | yes | markdown | in-app dialog | |
| `methods[].method` | string | yes | -- | in-app dialog | E.g. `wget`, `curl`, `kubectl`. |
| `methods[].commands[].channel` | string (enum) | yes | enum: `nightly`, `stable` | in-app dialog | |
| `methods[].commands[].command` | string | yes | -- | in-app dialog | May contain custom tags `{% if $showClaimingOptions %}...{% /if %}`; stripped when `clean=True` (`gen_integrations.py:982-985`). |
| `additional_info` | string | yes | markdown | in-app dialog | May contain custom tags. |
| `clean_additional_info` | string | no | markdown | in-app dialog | Clean-variant override; when present, replaces `additional_info` in the `clean_*` branch (`gen_integrations.py:990-992`). |
| `related_resources` | object | yes | -- | (TBD/empty) | Currently unused. |
| `platform_info.group` | string (enum) | yes | enum: `include`, `no_include`, `""` | in-app dialog | `include`/`no_include` cross-ref `distros.yml` to filter the platform table. |
| `platform_info.distro` | string | yes | -- | in-app dialog | Matches `distros.yml` `distro` field. |
| `quick_start` | integer | yes | -- | in-app "Add Nodes" dialog | Sort order. Negative -> hidden. |

Custom tag patterns recognized in `command` / `additional_info`:
`{% if X %}...{% /if %}`, `{%...%}` (regex
`gen_integrations.py:124`). Stripped when generating
`clean=True` outputs.

## exporter.json

Same pattern as `agent_notification.json` with
`overview.exporter_description` and
`overview.exporter_limitations`.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | $ref `shared.id` | yes | -- | id | |
| `meta` | $ref `shared.instance` | yes | -- | learn / in-app | |
| `keywords` | array<string> | yes | -- | learn frontmatter | |
| `overview.exporter_description` | string | yes | markdown | learn (`overview/exporter.md`) | |
| `overview.exporter_limitations` | string | yes (may be empty) | markdown | learn | When non-empty, rendered as `## Limitations`. |
| `setup` | $ref `shared.full_setup` | yes | -- | learn / in-app | |
| `troubleshooting` | $ref `shared.troubleshooting` | yes | -- | learn / in-app | |

## categories.json

Recursive tree definition.

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `id` | string | yes | -- | category lookup | Dotted path, e.g. `data-collection.databases`. |
| `name` | string | yes | -- | navigation | Display name. |
| `description` | string | yes | markdown | navigation | Tooltip / overview. |
| `children` | array<obj> | yes (may be empty) | -- | navigation | Recursive structure. |
| `collector_default` | boolean | no | -- | fallback | When true, this category is the default if a collector's declared categories are all bogus (`gen_integrations.py:906-908`). |

## distros.json

Validates `.github/data/distros.yml`. **NOT actually
enforced** -- `gen_integrations.py:1330` calls `load_yaml`
without passing this schema. See `gotchas.md`.

Top-level keys: `platform_map` (CPU arch -> docker platform
string), `arch_order`, `include[]` (array of platform
descriptors).

Per-platform descriptor fields:

| Field | Type | Req | Values | Surface | Notes |
|---|---|---|---|---|---|
| `distro` | string | yes | regex `^[a-z][a-z0-9]*$` | deploy platform table | |
| `version` | string | yes | regex `^[a-z0-9][a-z.0-9]*$` | deploy platform table | |
| `support_type` | string | yes | enum: `Core`, `Intermediate`, `Community`, `Third-Party`, `Unsupported` | deploy platform table | |
| `notes` | string | yes | -- | deploy platform table | |
| `eol_check` | bool / string | no | -- | deploy build matrix | |
| `bundle_sentry` | bool / string | yes | -- | deploy build matrix | |
| `base_image` | string | no | -- | deploy build matrix | |
| `env_prep` | string | no | -- | deploy build matrix | |
| `jsonc_removal` | string | no | -- | deploy build matrix | |
| `test.ebpf-core` | bool | no | -- | deploy build matrix | |
| `packages.type` | string | no | -- | deploy build matrix | |
| `packages.arches` | array<string> | no | -- | deploy build matrix | |
| `packages.repo_distro` | string | no | -- | deploy build matrix | |
| `packages.alt_links` | array | no | -- | deploy build matrix | |

Required: `distro`, `version`, `support_type`, `notes`,
`bundle_sentry`. Garbage in `distros.yml` produces broken
`platform_info` tables silently.

## Cross-schema notes

- The `<plugin-dir>/metadata.yaml` for collectors uses one of
  two top-level shapes (`gen_integrations.py:381`):
  - **Single-module** -- `plugin_name` and `modules: [<one>]`.
  - **Multi-module** -- `plugin_name` and `modules: [<many>]`.
  Both are validated against the same `collector.json` schema.
  The split single/multi validation in
  `check_collector_metadata.py` is dead code (see `gotchas.md`).

- Schema `shared.json` cross-refs are resolved by
  `Registry(retrieve=retrieve_from_filesystem)` so changes to
  `shared.json` propagate to all consumers immediately.

- `additionalProperties: false` is NOT set on most schemas.
  Unknown keys (`alternative_monitored_instances`,
  `most_popular`) pass through silently into `integrations.js`
  but no template renders them. See `gotchas.md`.

- Validation warnings are FATAL: `fail_on_warnings()`
  (`gen_integrations.py:150-160`) returns 1 on any warning,
  causing CI to fail and aborting doc regeneration.
