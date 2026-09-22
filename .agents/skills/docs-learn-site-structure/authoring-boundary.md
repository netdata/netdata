# Authoring boundary

Where a change is made so that it survives the next ingest. The learn repository's own `AGENTS.md` ("Change
discipline", "Static build gate contract") owns the hand-edit rules for that repository and its
`generated-output-boundary.yml` check enforces them on its PRs; this file states the consequences for an author
working here. Verified against `netdata/learn @ c3a16edd5ee4dc819976ef162c9afaff4b9b968c`, `ingest/ingest.py`.

## Owned by ingest in `netdata/learn`; never edited by hand

- `docs/**`: `safe_cleanup_learn_folders` deletes every `.md` and `.mdx` whose metadata lacks `part_of_learn: True`
  and every `.json`, unconditionally, at the start of each full run. Today the only surviving file is
  `docs/ask-nedi.mdx`, which the site root redirects to (`./redirects.md`).
- `netlify.toml` (regenerated; `static.toml` is the hand-written input), `ingest/generated_map.yaml`,
  `ingest/one_commit_back_file-dict.yaml`, `ingest/generated_sidebar_order.json` and its `.sha256` sidecar, generated
  grid pages and `_category_.json` files (`./sidebars.md`).
- `LegacyLearnCorrelateLinksWithGHURLs.json` is appended by ingest and hand-edited only for the unpublish step in
  `docs/.map/README.md#unpublishing-files`.

Everything else in that repository (`static.toml`, `docusaurus.config.js`, `src/`, `static/`, `ingest/*.py`,
`config/`, workflows) is hand-maintained there; a change to site behaviour, styling, or the ingest itself is a learn
PR. A page that must live in the learn repository sets `part_of_learn: True` in its frontmatter; ingest never matches,
copies, or sanitizes it, so the map and MDX rules do not apply to it; only the end-of-run reconciliation touches it
(sidebar position, sibling `_category_.json`, Mermaid contrast).

## Where documentation is edited

- Content published from this repository: edit the `.md` here; publication, position, label, description, and
  keywords come from its node in `docs/.map/map.yaml` (`docs/.map/README.md#1-edit-mapyaml`). The file's path has no
  routing effect (`./mapping.md`).
- Content from another source repository in `default_repos` (`netdata-cloud-onprem`, `.github`,
  `agent-service-discovery`, `netdata-grafana-datasource-plugin`, `helmchart`): edit it there; its node still lives in
  this repository's `map.yaml`, which is the only map ingest reads.
- Integration pages (collector, exporter, notification, and similar `integrations/*.md` files) are generated from
  `metadata.yaml` by the integrations pipeline and reach Learn through placeholders; edit the metadata, not the page
  (`integrations-lifecycle`).
- Sidebar order: reorder rows in `map.yaml` (`./sidebars.md`).

## Review flags

- A change under learn `docs/` without `part_of_learn: True`, or to `netlify.toml`, `_category_.json`, or
  `sidebars.js`: lost or ineffective at the next run.
- A hand edit to a generated integration page in this repository: overwritten by the next regeneration
  (`integrations-lifecycle`).
- A new page without a `map.yaml` node: silently unpublished (`./mapping.md#the-join-key`).
