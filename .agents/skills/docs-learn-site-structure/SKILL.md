---
name: docs-learn-site-structure
description: Change, review or troubleshoot Learn publication, docs/.map mapping, page URLs, redirects, sidebars, MDX and ingest/CI behavior. Local site builds require docs-learn-pr-preview; generated integration content uses metadata/integrations skills.
---

# Learn site structure

A page on `learn.netdata.cloud` is a node in this repository's `docs/.map/map.yaml`, rendered by the ingest of the
`netdata/learn` repository into a Docusaurus site that Netlify deploys. This skill states what an author or reviewer
here relies on; the how-to-publish procedure is `docs/.map/README.md`, and the mechanics live in the learn repository.

Apply `AGENTS.md#skill-selection`. Review the affected publication contract and existing evidence; recipe instructions
do not require a new SOW or authorize deletion, publication, ingest or a local preview. Use the task and symptom routes
below, including their dependencies when a change reaches another surface.

Learn-side facts carry the revision at which they were checked. Before relying on a claim affected by the task, inspect
that contract at the relevant available Learn revision and report any evidence gap. `repo-mirror-sources` supplies the
read-only checkout/revision workflow; loading this skill does not request a mirror refresh.

## Owners

| Owner | What it decides |
|---|---|
| `docs/.map/README.md#steps-to-publish` | the publish and unpublish procedure, `meta` fields, path reconstruction, the placeholder node, the local test command |
| `docs/.map/map.schema.json` | node shapes, required fields, the `edit_url` pattern, the `integration_kind` members |
| `docs/.map/validate_map_schema.py` | the two custom map rules (no duplicate `edit_url`; a leaf needs an `edit_url`), hand-run |
| `.github/workflows/trigger-learn-update.yml` | which pushes to `master` here dispatch the learn ingest |
| `.github/workflows/check-markdown.yml` | the PR gate here that regenerates integrations and runs the real ingest with `--fail-links-netdata` |
| `netdata/learn`: `ingest/ingest.py`, `ingest/autogenerateRedirects.py`, `static.toml`, `.github/workflows/ingest.yml`, `README.md`, `AGENTS.md` | everything the site does with a mapped file; facts in this skill are verified against `netdata/learn @ c3a16edd5ee4dc819976ef162c9afaff4b9b968c` and cite the symbol, never a line |
| `.agents/sensitive-data-discipline.md#allowed-alternatives` | how paths into the learn repository are written in committed text (`${NETDATA_REPOS_DIR}/learn/...`; `.agents/ENV.md` documents the key) |

Sibling skills: `integrations-lifecycle` produces the integration pages that ingest splices in through placeholders;
`collectors-metadata-yaml` owns the author-side MDX safety rules for `metadata.yaml` text; `docs-learn-pr-preview`
builds the site locally from a PR and loads this skill first; `repo-mirror-sources` maintains the checkout under
`${NETDATA_REPOS_DIR}` that the learn paths refer to.

## Tasks

| Task | Read |
|---|---|
| review a page, mapping or pipeline change | the affected task/symptom references below and current owner evidence; no recipe execution solely for review |
| add a page | `./recipes/add-doc-page.md`, then `./mapping.md` |
| move a page to another section | `./recipes/move-doc-page.md`, `./redirects.md` |
| rename a page (label or URL) | `./recipes/rename-doc-page.md`, `./mapping.md#file-path-and-slug` |
| delete or unpublish a page | `./recipes/delete-doc-page.md`, `docs/.map/README.md#unpublishing-files` |
| decide where an edit belongs | `./authoring-boundary.md` |
| understand or debug what ran in CI | `./pipeline.md` |
| a mapped page renders empty | `./how-tos/repairing-empty-mapped-pages.md` |
| links from integration descriptions to Learn | `./how-tos/integration-card-description-links.md` |

Symptom to file: page missing on Learn or URL not what was expected: `./mapping.md#the-join-key`; ingest exited 2:
schema, `./mapping.md#what-is-checked-and-by-what`; exited 3: `./redirects.md`; traceback naming
`resolve_publish_path_collisions` or `populate_integrations`: `./mapping.md`; MDX build error or a page cut off:
`./mdx-rules.md`; sidebar order, missing category, or a grid that turned into a page: `./sidebars.md`; old URL 404
or redirect loop: `./redirects.md`; a change vanished from `docs/` in the learn repository: `./authoring-boundary.md`.

## Rules

Enforced by code (the file names the tool and the effect):

- `map.yaml` must validate against `docs/.map/map.schema.json`; ingest exits 2 otherwise
  (`./mapping.md#what-is-checked-and-by-what`).
- A published path or slug collision that involves a non-integration page aborts ingest (`ValueError`);
  integration duplicates are suffixed automatically (`./mapping.md#file-path-and-slug`).
- A redirect catalogue entry that resolves to nothing and is neither covered nor retired aborts ingest with exit 3
  (`./redirects.md`).
- A broken link or anchor in a mapped page fails `.github/workflows/check-markdown.yml` on the PR here, and the
  learn ingest under `--fail-links` (`./pipeline.md#verification-before-merging-here`).
- Every `.md` and `.mdx` under learn `docs/` without `part_of_learn: True`, and every `.json` unconditionally, is
  deleted at the start of each full ingest (`./authoring-boundary.md`).

Hand-reviewed, because no code checks them:

- Every published page has a `map.yaml` node with `label` and `edit_url`; a node whose URL matches no file is
  silently unpublished, and a file without a node is silently skipped (`./mapping.md#the-join-key`).
- Keep `edit_url` unchanged when moving a page; the redirect catalogue is anchored to it (`./redirects.md`).
- A label decides the URL; choose labels that survive `_sanitize_mdx_filename_source` without colliding with a
  sibling (`./mapping.md#file-path-and-slug`).
- Cross-references are repository-relative `.md` paths, never `learn.netdata.cloud` URLs
  (`./mapping.md#links-between-pages`).
- Text that MDX reads as a tag (`<word>`, `<` before a digit, generics) is wrapped in inline code or rephrased
  (`./mdx-rules.md`).
- Run `docs/.map/validate_map_schema.py` before opening a map PR (`./mapping.md#what-is-checked-and-by-what`);
  nothing in CI runs it.
- Learn paths in committed text use the `${NETDATA_REPOS_DIR}/learn/...` form; facts about learn code carry the
  `owner/repo @ commit` they were verified against.
