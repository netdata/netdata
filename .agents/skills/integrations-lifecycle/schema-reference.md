# Schema reference

What each JSON Schema under `integrations/schemas/` validates, the entry shape it expects, and the behavior a reader
cannot see in the schema file itself. Field-by-field types, enums, and required lists are read from the schema file;
this document does not transcribe them. Collector field content is `.agents/skills/collectors-metadata-yaml/`;
validation mechanics (`make_validator`, the custom format, fatal warnings, non-strict schemas) are in `pipeline.md`.

## Shared definitions: `shared.json`

Referenced by most other schemas as `./shared.json#/$defs/<name>`, so an edit here changes every consumer at once.

- `page_description`: the explicit page meta description (50 to 160 characters, trimmed plain text, `format:
  netdata-balanced-parentheses`; the pattern rejects any leading hyphen). Used by `instance.description`,
  `secretstore.meta.description`, and `service_discovery.meta.description`. Contract: `description-authoring.md`.
- `id`: the deduplication key (`dedupe_integrations`).
- `instance`: name, link, categories, icon, optional `description` and `variables`. `name` drives the slug, the sidebar
  label, and the id; `categories` must name `categories.yaml` ids (fallback behavior: `pipeline.md`); `icon_filename` is
  a filename in the website repository's icon directory; `variables` triggers the second Jinja pass.
- `keywords`: emitted into the `<!--startmeta` block.
- `short_setup` and `full_setup`: the two setup shapes; `setup-generic.md` renders both. In `full_setup`, an option's
  `detailed_description` turns the table cell into a link to an `h5` section below the table, `group` adds a Group
  column, and for collectors only `render_collectors` defaults a per-example `folding` to the parent
  `examples.folding.enabled`.
- `troubleshooting`: `errors.list[]` (`error`, `cause`, `fix` required; `when`, `source` optional; the entry is closed
  with `additionalProperties: false`) rendered as `### Known Errors`, and the legacy `problems.list[]` (both fields
  optional) rendered as `### Other Problems`. The template adds `### Diagnostics` for `go.d.plugin`, `python.d.plugin`,
  and `charts.d.plugin` collectors and `### Test Notification` for agent notifications. Entry content:
  `.agents/skills/collectors-metadata-yaml/troubleshooting.md`.
- `_folding` and `_folding_relaxed` (title optional).

## Collector-shaped schemas

### `collector.json`

Top level: `plugin_name`, optional `profile_coverage`, and `modules[]`, one module per integration. The top-level
`plugin_name` is copied onto every `module.meta.plugin_name` by `load_collectors`, so the two agree by construction.
Required at the module root: `meta`, `overview`, `setup`, `troubleshooting`, `alerts`, `metrics`; on `meta`:
`plugin_name`, `module_name`, `monitored_instance`, `keywords`, `related_resources`,
`info_provided_to_referring_integrations`.

Behavior not visible in the schema:

- `related_resources.integrations.list[]`: a Draft-7 `dependencies` keyword makes `module_name` required whenever
  `monitored_instance_name` is set. It is the only `dependencies` use in the repository's schemas. Resolution is the
  cascading lookup in `pipeline.md`; an unresolvable reference is a fatal warning.
- `info_provided_to_referring_integrations.description` is rendered on OTHER pages that reference this module.
- `alerts[].metric` is not checked against `metrics.scopes[].metrics[].name`.
- `metrics.dynamic_context_prefixes[]` and `metrics.dynamic_collect_plugins[]` are read only by the dormant taxonomy
  tooling (`consistency.md`, "The dormant collector taxonomy"); nothing renders them.
- `profile_coverage.modules.<meta.id>[]` is allowed only in the Prometheus collector's metadata file, and
  `metrics.profile_coverage` is a generated in-memory projection that must never be authored
  (`how-tos/prometheus-profile-metadata.md`).
- `functions.list[].parameters[].default` is a string only. `returns.columns[].visibility` is rendered as a table cell
  by `templates/functions.md`; the `hidden` value does not suppress the column.
- `additionalProperties: false` is set only on `profile_coverage`, the two `metrics.dynamic_*` list entries, and (via
  `shared.json`) `instance.variables` and the troubleshooting `errors.list[]` entry, so unknown module keys pass through
  (`gotchas.md`, `alternative_monitored_instances`).

### `flows.json` and `device.json`

Tiny schemas that `$ref` `collector.json`: NetFlow, IPFIX, sFlow and flow-enrichment entries (`integration_type: flows`)
and the generated NPM catalog entries (`integration_type: device`) validate as collectors. Fork the schema only when
type-specific fields diverge.

## Thin schemas

Each is a single entry or an array of entries (`oneOf`); `id`, `meta`, and `keywords` are required everywhere; `meta` is
`shared.instance` except for `secretstore.json` and `service_discovery.json`, which define their own `$defs.meta`
(`kind`, `name`, `link`, `icon_filename`, `description`, plus `tagline` for discoverers; no `categories`, no
`variables`), so a `shared.json` edit does not reach those two. `setup` is `oneOf [short_setup, full_setup]` unless
stated.

- `exporter.json`: `overview.exporter_description` (required) and `exporter_limitations` (required, may be empty,
  rendered as `## Limitations` when non-empty); `setup` is `full_setup`; `troubleshooting` optional.
- `agent_notification.json`: `overview.notification_description` and `notification_limitations` (same pattern); optional
  `global_setup` whose two booleans (`severity_filtering`, `http_proxy`) are required only when the object is present;
  `troubleshooting` optional.
- `cloud_notification.json`: no `overview`; `setup` required; `troubleshooting` optional; the same optional
  `global_setup` as agent notifications. `integrations/cloud-notifications/metadata.yaml` is one file holding the whole
  array.
- `authentication.json`: `overview.authentication_description` and `authentication_limitations`; `troubleshooting`
  optional. `integrations/cloud-authentication/metadata.yaml` is one file holding the array.
- `logs.json`: `overview.description`, `overview.visualization.description`, `overview.key_features.description`;
  `setup.prerequisites.description` renders through `setup-logs.md`. The schema has a defect: a key literally named
  `required` sits inside `setup.properties`, so `setup` itself is optional and `prerequisites` is not required inside it
  (a present `prerequisites` still needs its `description`). `integrations/logs/metadata.yaml` holds four entries
  (systemd journal, Windows events, macOS unified logs, OpenTelemetry).
- `secretstore.json`: `meta.kind` (the slug, matching `/etc/netdata/go.d/ss/<kind>.conf`), optional `meta.description`
  (page description), `overview.description` and optional `limitations`, `setup` (`full_setup`, rendered by
  `setup-secretstore.md`), `collector_configs` (its `summary.operand_format` and `summary.example_operand` feed the
  `SECRETS.md` table; `examples.list[].language` defaults to `text` in the schema but the template uses `yaml`),
  `troubleshooting` required.
- `service_discovery.json`: `meta.kind` (the slug and discoverer registry name), `meta.tagline` (the
  `SERVICE-DISCOVERY.md` table one-liner), optional `meta.description`, `overview.description` with optional
  `how_it_works` and `limitations`, `setup` (`full_setup` only, rendered by `setup-service_discovery.md`), `services`
  (required: `description`, `template_variables`, `examples`; those two lists carry `minItems: 1`, `evaluation` has no
  minimum), optional `verify.checks.list[]`, `troubleshooting` required.

## Catalog and platform schemas

- `deploy.json`: an array of deploy methods for the in-app "Add Nodes" dialog; never rendered to a page.
  `methods[].commands[].command` and `additional_info` may carry the frontend's `{% if $showClaimingOptions %}...{% /if
  %}` tags, stripped by `CUSTOM_TAG_PATTERN` in the clean variant; `clean_additional_info` replaces `additional_info`
  there. `quick_start` orders the dialog; negative hides. `platform_info.group` (`include`, `no_include`, empty) and
  `distro` cross-reference `distros.yml` (`in-app-contract.md`).
- `categories.json`: the recursive `id`/`name`/`description`/`children` tree with the optional `collector_default` flag
  (fallback semantics in `pipeline.md`).
- `distros.json`: describes `.github/data/distros.yml` (`platform_map`, `arch_order`, `include[]` platforms with
  `distro`, `version`, `support_type` enum, `notes`, `bundle_sentry` required) but is never consulted by the generator
  (`gotchas.md`). Its platform object is closed and `packages.type` and `packages.arches` are required inside
  `packages`.
- `taxonomy_collector.json`, `taxonomy_sections.json`, `taxonomy_output.json`: the dormant collector taxonomy
  (`consistency.md`, "The dormant collector taxonomy"). Closed schemas read only by `gen_taxonomy.py`.
