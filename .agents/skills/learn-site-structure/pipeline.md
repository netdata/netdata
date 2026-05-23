# Pipeline: ingest, CI, deploy

This document maps the Learn ingest pipeline end to end: every
step `ingest.py` performs, the source repositories it pulls
from, the CI workflow that triggers it, and how the result
gets to `learn.netdata.cloud`.

## The orchestrator: `ingest/ingest.py`

Live entrypoint: `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py`.
The legacy `ingest.js` and `ingest.md` at the learn-repo root
are NOT the orchestrator -- ignore them.

### Argument parsing (`ingest.py:2513-2762`)

| Flag | Purpose |
|---|---|
| `--repos OWNER/REPO:BRANCH ...` | Override the default repo list. |
| `--local-repo NAME:/path ...` | Use a local copy via `shutil.copytree` instead of cloning. |
| `--dry-run` | Skip output writes. |
| `--debug` | Verbose logging. |
| `--docs-prefix DOCS` | Default `docs`; the target directory in learn for published files. |
| `--fail-links` | Exit 1 if broken internal links are detected at the end of the run. |
| `--fail-links-{netdata,helmchart,onprem,asd,grafana,github}` | Per-source-repo fail-on-broken-links. |
| `--gh-token` | GitHub token for clones. |
| `--use_plain_https` | Force HTTPS clone URLs (no SSH). |
| `--ignore-on-prem-repo` | Skip `netdata-cloud-onprem` clone and ignore broken-link references to it. |

### The 16-step flow (`ingest.py:2513-3087`, `__main__`)

1. **Argument parsing**.

2. **Cleanup** (`ingest.py:2766-2768`):
   - `unsafe_cleanup_folders('ingest-temp-folder')` -- wipes
     the work dir.
   - `safe_cleanup_learn_folders('docs')` -- walks
     `${NETDATA_REPOS_DIR}/learn/docs/` and removes every
     `.md` / `.mdx` / `.json` that does NOT carry
     `part_of_learn: True`. The flag is the **opt-in for
     hand-authored files** that should survive between runs.
     Currently only `docs/ask-nedi.mdx`.

3. **Clone all repos** (`ingest.py:2776-2800`) into
   `ingest-temp-folder/<repo>/` with `--depth 1`. Each repo
   tracks the branch declared in `default_repos`
   (`ingest.py:74-105`). `--ignore-on-prem-repo` skips the
   on-prem clone and ignores broken-link references to it.
   `--local-repo X:/path` and `--repos /path/to/repo` use
   `shutil.copytree` for developer ergonomics.

4. **Move + validate map** (`ingest.py:2802-2825`):
   - `ingest-temp-folder/netdata/docs/.map/map.yaml` ->
     `./map.yaml`.
   - Validate against
     `ingest-temp-folder/netdata/docs/.map/map.schema.json`.
   - Build `MAP_SIDEBAR_ORDER` and `MAP_DOC_SCOPE` for sidebar
     position assignment.
   - Schema failure -> exit code 2.

5. **Enumerate markdowns** (`ingest.py:2828-2829`):
   `glob('ingest-temp-folder/**/*.md*')` plus dot-directories
   (`fetch_markdown_from_repo`, `ingest.py:1207-1211`).

6. **Populate integrations** (`ingest.py:2832`): scan all
   markdowns for `INTEGRATION_MARKER`, parse hidden metadata,
   bucket by category, replace each `integration_placeholder`
   row in the map with sorted (by `learn_rel_path`,
   `sidebar_label`) integration rows. Writes the resulting
   tabular map to `ingest/generated_map.yaml`.

7. **Sidebar position assignment** (`ingest.py:2838-2840`):
   `automate_sidebar_position` walks the (now expanded) map
   dataframe and assigns sibling-relative positions in steps
   of 10 within each parent scope, ordered by map traversal.

8. **Inject metadata into source files** (`ingest.py:2842-2889`):
   for each markdown, look up its row by `custom_edit_url` and
   write the hidden metadata block at the top of the file. If
   `learn_status == Published`, compute the destination via
   `create_mdx_path_from_metadata`, store the destination +
   ingestedRepo in `to_publish`, also inject `slug:` and
   `learn_link:` into the file. Otherwise file is dropped.

9. **Path collision check** (`ingest.py:2891-2908`): warn if
   any two `to_publish` entries collide case-insensitively.

10. **Publish each file** (`ingest.py:2916-2919`):
    - `local_to_absolute_links(md_file, to_publish)` --
      convert relative/abs-from-repo-root markdown links into
      GitHub view URLs (rewritten in step 12).
    - `copy_doc(md_file, to_publish[md_file]['learnPath'])` --
      copy temp file to `docs/<...>.mdx` (creating dirs).
    - `sanitize_page(learnPath)` -- runs MDX-escape transforms
      (see `mdx-rules.md`).

11. **Build cross-reference dictionary** (`ingest.py:2925-2927`):
    `add_new_learn_path_key_to_dict` produces, for every
    published file, a map: GitHub-view-link AND
    GitHub-edit-link -> final `/docs/...` URL. Handles the
    duplicate-segment slug trim and explicit `slug:`
    overrides.

12. **Rewrite GitHub links to Learn URLs** (`ingest.py:2929-2930`):
    `convert_github_links` walks every published file. Any
    `https://github.com/netdata/<repo>/blob/.../*.md` in the
    body that maps to a file in `to_publish` is rewritten to
    its final Learn URL. Header anchors are validated; broken
    anchors are accumulated. Links to integration md files
    that aren't in the map fall back to the parent README's
    URL (`ingest.py:2153-2209`). Links to GitHub files that
    exist in the repos but aren't in the map stay as GitHub
    links (intentional -- `file_exists_in_repos` at
    `ingest.py:711-719`). Truly missing targets become
    `UNCORRELATED_LINK_COUNTER` increments.

13. **Generate redirects** (`ingest.py:2932`): see
    `redirects.md`.

14. **Broken-link reporting** (`ingest.py:2940-3022`): print
    broken URLs grouped by repo, broken anchors grouped by
    repo, decide whether to exit 1 based on `--fail-links`
    flags.

15. **Post-processing** (`ingest.py:3030-3065`):
    - Save current `(custom_edit_url -> new_learn_path)`
      mapping to `ingest/one_commit_back_file-dict.yaml` for
      next run's redirect diff.
    - Cleanup `ingest-temp-folder` and `map.yaml`.
    - `get_dir_make_file_and_recurse('./docs')` -- auto-create
      grid pages for any integration directory that lacks an
      overview.
    - `ensure_category_json_for_dirs('docs')` -- write
      `_category_.json` for any directory still without an
      overview page.
    - `normalize_sidebar_positions_by_parent('docs')` --
      assign sibling positions deterministically per parent
      scope.

16. **No git push from the script.** The CI workflow handles
    `git add` / commit / PR.

### How content gets fetched

`ingest.py` does NOT use the GitHub REST API for content. It
clones full repos with `gitpython` (`ingest.py:1101-1137`), so
there's no API rate-limit consideration in the live pipeline.
(Legacy `ingest.js` did use the API; ignore it.)

## The 6 source repositories

Default at `ingest/ingest.py:74-105`:

| Logical repo | Owner | Branch | What it feeds |
|---|---|---|---|
| `netdata` | `netdata` | `master` | The bulk of Learn -- all docs declared in `<repo>/docs/.map/map.yaml`, plus integration metadata generated by the agent build (collectors, exporters, secretstore, alerts, functions). Hosts the canonical map files. |
| `netdata-cloud-onprem` | `netdata` | `master` | On-prem documentation (Netdata Cloud On-Prem category). Skippable via `--ignore-on-prem-repo`. |
| `.github` | `netdata` | **`main`** | `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md` -- only those whose edit URLs appear in `map.yaml`. |
| `agent-service-discovery` | `netdata` | `master` | Service-discovery docs for the agent. |
| `netdata-grafana-datasource-plugin` | `netdata` | `master` | Grafana datasource integration docs. |
| `helmchart` | `netdata` | `master` | Kubernetes Helm chart docs. |

Override at runtime: `--repos OWNER/REPO:BRANCH ...` or
`--local-repo NAME:/path ...` (`ingest.py:2607-2613`).

The legacy `ingest.js` listed only 4 repos (`netdata`, `.github`,
`go.d.plugin`, `agent-service-discovery`). `go.d.plugin` is no
longer pulled (its docs were absorbed into the netdata
monorepo); `netdata-cloud-onprem`,
`netdata-grafana-datasource-plugin`, `helmchart` are new in the
Python pipeline.

## CI: `${NETDATA_REPOS_DIR}/learn/.github/workflows/ingest.yml`

### Triggers (`ingest.yml:2-18`)

- `workflow_dispatch` -- manual.
- `schedule: cron "10 8-23/3 * * *"` -- every 3 hours from
  08:10 to 23:10 UTC (6 runs/day).
- `push` to `master` when `plugins/**`, `src/**`, `static/**`,
  `sidebar.js`, `package.json`, `yarn.lock`,
  `tailwind.config.js`, or any `**/*.md` / `**/*.mdx` changes
  in the LEARN repo (not in source repos -- those trigger
  ingest indirectly via the cron).

### Steps (`ingest.yml:24-101`)

1. Checkout (full history, `fetch-depth: 0`) using
   `secrets.GITHUB_TOKEN`.
2. SSH agent for `secrets.NETDATABOT_SSH_PRIVATE_KEY` (used to
   clone private repos like `.github`).
3. Python 3.10, install
   `${NETDATA_REPOS_DIR}/learn/.learn_environment/ingest-requirements.txt`
   (`pip`, `requests==2.33.0`, `Pillow`, `PyGithub`, `gitpython`,
   `mergedeep`, `pandas`, `numpy`, `retrypy`, `pyyaml`,
   `jsonschema`).
4. **Run `python ingest/ingest.py --fail-links 2>&1`** --
   captures output for issue creation, propagates exit code 1
   (broken links) into a workflow output but does NOT fail
   the workflow on broken links (only on real errors,
   `ingest.yml:65-77`).
5. **Update kickstart checksum** -- fetches
   `https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/kickstart.sh`,
   MD5s it, replaces literal `@KICKSTART_CHECKSUM@` placeholder
   in `docs/Netdata Agent/Installation/Linux/Linux.mdx`.
6. **`peter-evans/create-pull-request@v6.0.1`** -- opens or
   updates a branch named `ingest` with title
   "Ingest New Documentation", labels `ingest, automation`. The
   PR is reviewed and merged manually by the team.
7. If broken links were found, create or update a single open
   GitHub issue with label `broken-links`.

## CI: `${NETDATA_REPOS_DIR}/learn/.github/workflows/daily-learn-link-check.yml`

Daily cron `00 04 * * *` -- runs
`python ingest/check_learn_links.py`, which extracts every
`learn_link:` URL from every `.md` / `.mdx` in `docs/`,
HEAD/GET checks them, exits 1 (failing the workflow) when any
404 is observed.

## Deploy: Netlify

There is **no GitHub-side build/deploy workflow**. Build &
deploy are handled by Netlify:

- README confirms Netlify deploys from `master` automatically.
- Site is `netdata-docusaurus` on Netlify.
- Branches `master`, `staging`, `staging1` each get their own
  deploy (preview URLs).
- Build env pinned by `static.toml:3-4` /
  `netlify.toml [build]`: `NPM_VERSION=10.9.2`,
  `NODE_VERSION=22.14.0`, `NETLIFY_USE_YARN=true`,
  `NODE_OPTIONS=--max_old_space_size=4096`.
- Build command: `yarn build` (Docusaurus default).
- Output dir: `build/`.
- Propagation time: whatever Netlify takes after a master
  commit; redirects ship in `netlify.toml` so they apply at
  the edge immediately upon deploy.

## End-to-end timing

1. Maintainer pushes to `netdata/netdata` master (via merged
   PR).
2. Up to 3 hours later, `${NETDATA_REPOS_DIR}/learn/.github/workflows/ingest.yml`
   cron fires.
3. Ingest opens a PR in the learn repo titled "Ingest New
   Documentation".
4. A maintainer reviews and merges the PR.
5. Netlify auto-deploys `master`. Within minutes, the new
   content is live on `learn.netdata.cloud`.

So end-to-end: 0-3 hours of cron lag + manual review/merge +
minutes of Netlify deploy. Plan accordingly when something
must be live by a deadline -- you can also trigger the ingest
manually via `workflow_dispatch` to skip the cron lag.

## Local testing

From within the learn repo:

```bash
# Install deps once.
python3 -m venv venv && . venv/bin/activate
pip install -r .learn_environment/ingest-requirements.txt

# Test against your local netdata clone.
python3 ingest/ingest.py --local-repo netdata:<repo> --ignore-on-prem-repo --fail-links-netdata

# Then build the site locally.
yarn install
yarn start    # dev server
# OR
yarn build    # full build to ./build
```
