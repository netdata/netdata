# Pipeline: ingest, CI, deploy

What runs, in which repository, when a documentation change merges. Owners: `netdata/learn` `README.md` (sections
"Ingest and process documentation files", "Automated ingest via GitHub Actions", "Deployment"),
`.github/workflows/ingest.yml`, `.github/workflows/daily-learn-link-check.yml`, and `AGENTS.md` in that repository;
in this repository `.github/workflows/trigger-learn-update.yml` and `.github/workflows/check-markdown.yml`;
`docs/.map/README.md#2-test-the-changes` and `docs/.map/README.md#4-merge-the-learn-ingest-pr` for the author's view.
Learn-side facts verified against `netdata/learn @ c3a16edd5ee4dc819976ef162c9afaff4b9b968c`.

## The orchestrator

`ingest/ingest.py` is the entry point (`__main__`); `ingest.js` and `ingest.md` at the repository root describe the
retired Node pipeline and are not read by any workflow. Flags are defined in the `argparse` block; the ones that change
behaviour an author meets:

- `--local-repo <name>:<path>` copies a local checkout (`shutil.copytree`) instead of cloning; `--repos` accepts
  `owner/repo:branch` (changes what is cloned, not the map lookup key, `./mapping.md#the-join-key`) or a local path,
  whose repository is inferred from the directory name by exact match, then by substring in either direction, so a
  fork named unlike its upstream can bind to the wrong repository; use `--local-repo netdata:<path>`.
- `--ignore-on-prem-repo` skips the on-prem clone, adds that repository to the redirect ignore set, and forces plain
  HTTPS cloning.
- `--fail-links`, or one abbreviated flag per repository (`--fail-links-netdata`, `-helmchart`, `-onprem`, `-asd`,
  `-grafana`, `-github`), turns broken links or anchors into exit code 1 at the end of the run (the run completes
  first).
- `--kickstart-checksum` (32 hex characters) is required for a full remote ingest; with a local `netdata` checkout it
  is derived from that checkout's `packaging/installer/kickstart.sh` (`resolve_kickstart_checksum`).
- `--regenerate-grids-only` rebuilds generated outputs from the committed recovery state (`regenerate_grids_only`,
  `load_sidebar_order_state`) without touching sources and without resolving a checksum.
- `--debug` prints the non-empty source files that matched no map row; `--dry-run` and `--docs-prefix` (default
  `docs`, the output directory) exist.

## What a run does, in order

Read `__main__` for the exact sequence; the symbols, in order:

1. `resolve_kickstart_checksum`, before anything is deleted (a no-op under `--regenerate-grids-only`, which then
   runs `regenerate_grids_only` and exits before any cleanup).
2. `unsafe_cleanup_folders` on the temp folder and `safe_cleanup_learn_folders` on `docs/` (`./authoring-boundary.md`).
3. `clone_repo` for every entry of `default_repos` with depth 1, or `shutil.copytree` for `--local-repo`. A clone
   failure is caught and printed; the run continues without that repository.
4. The map is moved out of the `netdata` checkout and validated (`validate_map_schema`, exit `MAP_SCHEMA_EXIT_CODE`),
   then `load_map_sidebar_order` fills `MAP_SIDEBAR_ORDER` (`./sidebars.md`).
5. `fetch_markdown_from_repo` lists every `.md*` file; a dot-directory is searched one level deep only
   (`.github/*.md`), so a page nested deeper under a dot-directory is never found.
6. `populate_integrations` splices integration pages into the map (`./mapping.md`).
7. `automate_sidebar_position`, then per file `insert_and_read_hidden_metadata_from_doc` and
   `create_mdx_path_from_metadata`; `resolve_publish_path_collisions`; `update_metadata_of_file`; the case-only
   collision warning.
8. Per published file `local_to_absolute_links`, `copy_doc`, `sanitize_page` (`./mdx-rules.md`).
9. `add_new_learn_path_key_to_dict` sets each entry's `new_learn_path` from its computed slug (the view and edit link
   dictionary it builds is discarded, and `produce_gh_edit_link_for_repo` returns an unsubstituted format string for
   every repository except `.github`; neither matters, because `convert_github_links` builds its own key and
   normalizes `edit/` to `blob/`, so edit links to published pages are rewritten too).
10. `autogenerateRedirects.main` (`./redirects.md`); a `LegacyRedirectGateError` exits with `REDIRECT_GATE_EXIT_CODE`
    (3) before the catalogue, `netlify.toml`, or the mapping state are written.
11. Broken-link and broken-anchor reports grouped by repository; the exit decision is recorded, not applied yet.
12. `ingest/one_commit_back_file-dict.yaml` (next run's redirect baseline), `apply_kickstart_checksum` (exactly one
    placeholder in the installation page), temp cleanup, `reconcile_generated_outputs` (grids, `_category_.json`,
    `normalize_sidebar_positions_by_parent`, `fix_mermaid_diagram_contrast`, `clean_redirects` and
    `write_netlify_config`),
    `write_sidebar_order_state` (map hash, sibling order, corpus SHA-256, plus a `.sha256` sidecar), removal of the
    temporary map, then exit 1 if a fail flag fired.

Exit codes: 2 schema, 3 redirect gate, 1 broken links under a fail flag; a `ValueError` from
`resolve_publish_path_collisions`, a `RuntimeError` from `get_dir_make_file_and_recurse`, or an `IndexError` from
`populate_integrations` is an uncaught traceback. Nothing in the script commits or pushes.

Source repositories are the keys of `default_repos` (`.github` on `main`, the rest on `master`); the map is read only
from the `netdata` checkout, so a page from any other repository still needs its row in this repository's
`docs/.map/map.yaml`.

## When ingest runs

- This repository dispatches it: `.github/workflows/trigger-learn-update.yml` runs on a push to `master` touching
  `**.mdx?`, `docs/.map/map.yaml`, or `packaging/installer/kickstart.sh` and dispatches the learn workflow `Ingest`.
  In GitHub Actions filter syntax `?` means zero or one of the preceding character, so `**.mdx?` matches `.md` and
  `.mdx`.
- `ingest.yml` in `netdata/learn` also runs on `workflow_dispatch`, on its cron (every third hour between 08:00 and
  23:00 UTC, `schedule:`), and on pushes to its own `master` touching the paths it lists.

So a merged docs PR normally reaches the ingest PR within minutes; the cron is the ceiling, not the expectation.

`ingest.yml` (read the file for the steps): resolves the kickstart checksum from this repository's `master` (the
step fails the workflow when the download or the 32-hex check fails), runs `ingest.py --fail-links`, classifies the
output with `ingest/classify_ingest_result.py` (broken links become an issue labelled `broken-links`, not a failed
workflow; an unclassifiable run fails), verifies the recovery state as a fixed point (snapshot,
`--regenerate-grids-only`, identical snapshot), opens or updates the PR on branch `ingest` (title
"Ingest New Documentation", labels `ingest` and `automation`), and dispatches `rendered-link-integrity.yml` against
that PR. The PR carries the regenerated `docs/`, `netlify.toml`, and the redirect catalogue; a person reviews those
and merges it (`docs/.map/README.md#4-merge-the-learn-ingest-pr`).

Other gates: `daily-learn-link-check.yml` runs `ingest/check_learn_links.py` daily (the copy under `scripts/` is an
unwired duplicate) and fails on any `learn_link:` that returns 404; `generated-output-boundary.yml` refuses hand edits
to generated output in learn PRs; `rendered-link-integrity.yml` renders head and base and diffs the link inventory
(contract in the learn `AGENTS.md`).

## Verification before merging here

- `.github/workflows/check-markdown.yml` (job `check-documentation`) is the PR gate in this repository: it checks out
  `netdata/learn`, regenerates the integration pages, runs `integrations/tests/test_descriptions.py` against the map,
  and runs the real ingest with `--local-repo netdata:<workspace> --ignore-on-prem-repo --fail-links-netdata`. A
  broken link or anchor in a mapped page fails the PR here, before any ingest PR exists.
- Locally: `docs/.map/README.md#2-test-the-changes` has the command; the environment setup is the learn `README.md`
  "Manual ingest via local environment" (Python 3.13, `.learn_environment/ingest-requirements.txt` with
  `--require-hashes`). `docs/.map/validate_map_schema.py` is the hand-run map check (`./mapping.md`).
- A full local build with a browser is `docs-learn-pr-preview`.

## Deploy

Netlify builds and deploys `master` of `netdata/learn`; there is no deploy workflow (site name and preview branches:
learn `README.md`, section "Netlify status"). The build command, publish
directory, and pinned Node and npm versions are the `[build]` table of `static.toml`, copied into the generated
`netlify.toml`; read them there rather than from the learn `README.md`, whose Node pin lags. Redirects ship in
`netlify.toml` and apply at the edge on deploy. There is one live version of the site: no `versioned_docs/` or
`versions.json` exist, and `versioning/remove_edit_links.py` is a manual helper for a snapshot, not an automated step.
