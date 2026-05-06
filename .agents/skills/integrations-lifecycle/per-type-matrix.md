# Per-type matrix

One row per integration type. Compact lookup: where the source
YAMLs live, which schema validates them, which sections render,
which template chooses, where the output lands, which surfaces
consume it.

| `integration_type` | Source YAMLs | Schema (`integrations/schemas/`) | RENDER_KEYS (`gen_integrations.py`) | Overview template (`integrations/templates/overview/`) | Setup template (`integrations/templates/`) | Output `.md` location | Surfaces consuming it |
|---|---|---|---|---|---|---|---|
| `collector` | 9 source roots under `src/collectors`, `src/go/plugin/{go.d,scripts.d,ibm.d}/...`, `src/crates/netdata-otel` (see `pipeline.md` for full list) | `collector.json` | `alerts, metrics, functions, overview, related_resources, setup, troubleshooting` (`:71`) | `collector.md` | `setup-generic.md` (with sample-`<lang>`-config.md per plugin) | `<plugin-dir>/integrations/<slug>.md`, slug = `clean_string(meta.monitored_instance.name)` | learn (per-page), in-app (sidebar/search), `src/collectors/COLLECTORS.md`, `integrations.js` |
| `deploy` | `integrations/deploy.yaml` (`:39-41`) | `deploy.json` | n/a -- only `platform_info.md` template | n/a | n/a | NOT written to disk -- embedded only in `integrations.js` | in-app "Add Nodes" dialog (sorted by `quick_start`) |
| `exporter` | `src/exporting/*/metadata.yaml` (`:43`) | `exporter.json` | `overview, setup, troubleshooting` (`:81`) | `exporter.md` | `setup-generic.md` | `src/exporting/<dir>/integrations/<slug>.md` | learn, in-app |
| `agent_notification` | `src/health/notifications/*/metadata.yaml` (`:47`) | `agent_notification.json` | `overview, setup, troubleshooting` (`:87`) | `notification.md` | `setup-generic.md` (handles short `setup.description` form too) | `src/health/notifications/<dir>/README.md` (DIRECT, NOT a symlink, NOT under `integrations/`) | learn, in-app |
| `cloud_notification` | `integrations/cloud-notifications/metadata.yaml` (`:51`) | `cloud_notification.json` | `setup, troubleshooting` (`:93`) | n/a (image-only header) | `setup-generic.md` (mostly short form) | `integrations/cloud-notifications/integrations/<slug>.md`, slug = `clean_string(meta.name)` | learn, in-app |
| `logs` | `integrations/logs/metadata.yaml` (`:55`) | `logs.json` | `overview, setup` (`:98`) | `logs.md` | `setup-logs.md` | `integrations/logs/integrations/<slug>.md`, slug = `clean_string(meta.name)` | learn, in-app |
| `authentication` | `integrations/cloud-authentication/metadata.yaml` (`:59`) | `authentication.json` | `overview, setup, troubleshooting` (`:103`) | `authentication.md` | `setup-generic.md` | `integrations/cloud-authentication/integrations/<slug>.md`, slug = `clean_string(meta.name)` | learn, in-app |
| `secretstore` | `src/go/plugin/agent/secrets/secretstore/backends/*/metadata.yaml` (`:63`) | `secretstore.json` | `overview, setup, collector_configs, troubleshooting` (`:109`) | `secretstore.md` | `setup-secretstore.md` (renders `collector_configs.md` too) | `<backend-dir>/integrations/<slug>.md`, **slug = `clean_string(meta.kind)`** (NOT `meta.name`) | learn (per-backend page), in-app, `src/collectors/SECRETS.md` (umbrella) |
| `service_discovery` | `src/go/plugin/go.d/discovery/sdext/discoverer/*/metadata.yaml` (`:67`) | `service_discovery.json` | `overview, setup, services, verify, troubleshooting` (`:116`) | `service_discovery.md` | `setup-service_discovery.md` (renders `sd-services.md`, `sd-verify.md` too) | `<discoverer-dir>/integrations/<slug>.md`, **slug = `clean_string(meta.kind)`** (NOT `meta.name`) | learn, in-app, `src/collectors/SERVICE-DISCOVERY.md` (umbrella) |
| `categories` | `integrations/categories.yaml` | `categories.json` | n/a -- embedded as `categories` array in `integrations.js` | n/a | n/a | n/a | in-app navigation, learn navigation |
| `distros` | `.github/data/distros.yml` | `distros.json` (declared but **NOT enforced** -- see `gotchas.md`) | n/a -- consumed only by `render_deploy` | n/a | n/a | n/a | feeds `deploy.platform_info` table |
| `shared` | n/a -- referenced by other schemas via `./shared.json#/$defs/...` | n/a | n/a | n/a | n/a | n/a | building block (instance, full_setup, troubleshooting, _folding) |

## Slug rules summary

The slug used in the output filename comes from `clean_string`
(`gen_docs_integrations.py:118-126`):
1. lowercase the source string;
2. replace spaces with `_`;
3. replace `/` with `_`;
4. drop `(`, `)`, `,`, `'`, backtick, `:`.

Source string per type:
- collector: `meta.monitored_instance.name`
- exporter / agent-notification / cloud-notification /
  authentication / logs: `meta.name`
- **secretstore / service_discovery: `meta.kind`** (deliberately
  -- it must match the runtime config filename
  `/etc/netdata/go.d/ss/<kind>.conf` etc.)

Special case at `gen_docs_integrations.py`: if a collector's
`custom_edit_url` is under `/integrations/functions/`, the
output filename uses the function slug from the URL stem
(with `-` -> ` `) instead of `monitored_instance.name` -- to
avoid collisions when many integrations share a "Top Queries"
label.

## Single-integration symlink rule

`gen_docs_integrations.py:make_symlinks` (`:527-544`):

- After all per-integration `.md` files are written, the script
  walks each `<plugin-dir>/integrations/` directory.
- If the directory contains EXACTLY one file
  (`len(list(integrations_dir.iterdir())) == 1` at `:466`),
  the script creates `<plugin-dir>/README.md` as a symlink to
  `integrations/<sole-file>.md`.
- Multi-integration directories do NOT get a top-level
  `README.md` symlink; the parent's hand-written README (if
  any) is left alone.
- The internal `{element}/{symlinks[element]}` references in
  the `.md` body are rewritten to `{element}/README.md` so
  anchors don't break (`:542-544`).

This is the reason a plugin's `README.md` is sometimes a
symlink (e.g. `src/collectors/diskspace.plugin/README.md ->
integrations/disk_space.md`) and sometimes a hand-written
file (e.g. multi-module plugins).

## `agent_notification` is the odd one out

For every other type, the per-integration `.md` lives under
`<dir>/integrations/<slug>.md` and a `README.md` symlink is
made when there is only one integration in that directory.

For `agent_notification`, the script writes the per-integration
file DIRECTLY to `<dir>/README.md` (`:488-496`). No
`integrations/` subdirectory, no symlink. So
`src/health/notifications/email/README.md` IS the generated
artifact, not a symlink. Keep this in mind when checking the
five-file consistency rule (the README.md you would normally
not edit is the same physical file as the generated
integration page).
