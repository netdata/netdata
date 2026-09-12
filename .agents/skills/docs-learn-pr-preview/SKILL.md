---
name: docs-learn-pr-preview
description: Use only when the user explicitly asks to build, run, preview, inspect, or validate learn.netdata.cloud locally using the contents of a PR or documentation branch before merge. Do not trigger for ordinary docs edits unless the user asks for a local Learn preview.
---

# docs-learn-pr-preview

Build and inspect a local Learn site from a PR's documentation content without
dirtying the real Learn checkout.

Always load `docs-learn-site-structure` first. If the PR touches `metadata.yaml` or
generated integration pages, also load `integrations-lifecycle`.

## Rules

- Trigger only on an explicit preview/build/inspect request.
- Do not run ingest directly in a dirty Learn worktree.
- Use an isolated preview directory under `/tmp` or the repo's gitignored
  `.local/`.
- Record the source selection and resolved Netdata and Learn commits used for the preview.
- Copy PR source content into an isolated source directory. Prefer committed
  PR content; if validating uncommitted work, copy tracked modified files and
  only intentional untracked docs files after checking `git status --short`.
- Save any preview server PID and kill only that PID when stopping it.
- Treat site-wide warnings as evidence, but separate pre-existing global Learn
  warnings from PR-specific warnings.

## Workflow

Resolve the requested PR's head commit from that PR's metadata, or resolve the requested documentation branch.
A PR number is a label, not a source selector. Verify the object is available locally; acquire a missing object in
an isolated repository if necessary. Do not substitute the current checkout. For an intentional uncommitted preview,
inspect `git status --short` and choose the working-tree alternative below.

Set paths and create a fresh private run directory. `LEARN_REPO` must identify the selected local Learn checkout;
`LEARN_REF` may be `HEAD`, including a detached HEAD, or another verified local ref.

```bash
REPO_ROOT="$(git rev-parse --show-toplevel)"
SOURCE_REPO="${REPO_ROOT}" # Use the isolated acquisition repository here if the requested object was missing.
SOURCE_REF="<verified-requested-source-commit-or-ref>"
LEARN_REPO="${NETDATA_REPOS_DIR:?set the source mirror root}/learn"
LEARN_REF="HEAD"
LEARN_COMMIT="$(git -C "${LEARN_REPO}" rev-parse --verify --end-of-options "${LEARN_REF}^{commit}")"
mkdir -p "${REPO_ROOT}/.local/audits/learn-pr-preview"
PREVIEW_ROOT="$(mktemp -d "${REPO_ROOT}/.local/audits/learn-pr-preview/run.XXXXXX")"
SOURCE_COPY="${PREVIEW_ROOT}/netdata-source"
LEARN_COPY="${PREVIEW_ROOT}/learn"
SNAPSHOT="${REPO_ROOT}/.agents/skills/docs-learn-pr-preview/scripts/snapshot-source.py"
```

Export exact committed source blobs (requires Python 3.9 or later):

```bash
python3 "${SNAPSHOT}" --repo "${SOURCE_REPO}" --output "${SOURCE_COPY}" --ref "${SOURCE_REF}"
```

For an intentional working-tree preview, use this **instead** of the committed export. Add one
`--include-untracked relative/file` for each explicitly selected nonignored untracked file; omit the option if none.
Use the intended working checkout as `SOURCE_REPO`. Tracked working files, staged additions and tracked deletions
are reflected automatically.

```bash
python3 "${SNAPSHOT}" --repo "${SOURCE_REPO}" --output "${SOURCE_COPY}" --working-tree \
  --include-untracked "<selected-relative-file>"
```

The helper requires a new output and sibling `.manifest.json`. It records source mode, resolved/base commit,
per-file hashes and modes, selected untracked files, indexed paths missing on disk and gitlink pins. Staged deletions
are absent from the index and snapshot; use the inspected status/diff for the complete deletion report. Gitlinks are
recorded, not expanded: if an affected preview input needs submodule content, prepare that pinned content in the isolated copy
and record it before claiming coverage. Unresolved index conflicts and links escaping the snapshot are rejected.
The helper isolates Git subprocesses from inherited repository/index selection; `--repo` selects the source,
including a linked worktree. Keep inputs stable during capture; on failure inspect the new partial output and retry with a fresh run directory.
The helper never restores or cleans the original checkout.

Pin an isolated Learn copy to the resolved commit; this exports committed Learn content, not its local edits:

```bash
git clone --no-hardlinks --no-checkout "${LEARN_REPO}" "${LEARN_COPY}"
git -C "${LEARN_COPY}" checkout --detach "${LEARN_COMMIT}"
git -C "${LEARN_COPY}" rev-parse HEAD
```

Before ingest, prepare affected generated integration pages **inside `SOURCE_COPY`**, following the producer chain
and current-input generation in `integrations-lifecycle`. A metadata-only PR can otherwise preview stale committed
pages. The source manifest describes inputs before this derived generation; record generator commands and results
separately. Keep generated changes out of the original source checkout.

Install ingest dependencies in the isolated preview:

```bash
python3.13 -m venv "${PREVIEW_ROOT}/venv"
"${PREVIEW_ROOT}/venv/bin/python" -m pip install \
  --require-hashes \
  -r "${LEARN_COPY}/.learn_environment/ingest-requirements.txt"
```

Install JavaScript dependencies into `LEARN_COPY` using its lockfile and the runtime selected by its `static.toml`
(see below). If reusing compatible dependencies, copy them into the isolated checkout, then check all links:

```bash
python3 "${SNAPSHOT}" --check-links "${LEARN_COPY}"
```

Do not link to the original checkout's writable `node_modules`. A reused dependency tree must be self-contained;
reinstall into the isolated copy if its links escape. Merely matching a directory name does not establish lockfile
or runtime compatibility.

Run ingest with the PR source:

```bash
cd "${LEARN_COPY}"
"${PREVIEW_ROOT}/venv/bin/python" ingest/ingest.py \
  --local-repo "netdata:${SOURCE_COPY}" \
  --ignore-on-prem-repo \
  --use_plain_https \
  --fail-links-netdata
```

Build with the command and runtime Netlify uses. Both are the `[build]` table of `static.toml` in the Learn checkout
(`command`, and `NODE_VERSION`, `NPM_VERSION`, `NODE_OPTIONS` under `environment`); read them rather than pinning
values here, because the pins move with the site:

```bash
sed -n '/^\[build\]/,/^$/p' "${LEARN_COPY}/static.toml"
PUBLISH_DIR="$(sed -n '/^\[build\]/,/^$/{s/^ *publish *= *"\(.*\)"/\1/p;}' "${LEARN_COPY}/static.toml")"
[ -n "${PUBLISH_DIR}" ] || { echo "no publish key in the [build] table of static.toml" >&2; exit 1; }
```

Run the printed `command` inside `${LEARN_COPY}` with the printed `NODE_OPTIONS` exported and Node and npm matching
`NODE_VERSION` and `NPM_VERSION` (for example through `npx -y -p node@<NODE_VERSION> -p npm@<NPM_VERSION> <command>`).
The output lands in `${LEARN_COPY}/${PUBLISH_DIR}`.

Serve the static build for inspection:

```bash
python3 -m http.server 3030 --bind 127.0.0.1 --directory "${LEARN_COPY}/${PUBLISH_DIR}"
```

Or run it in the background with a PID file:

```bash
python3 -m http.server 3030 --bind 127.0.0.1 --directory "${LEARN_COPY}/${PUBLISH_DIR}" \
  >"${PREVIEW_ROOT}/http.log" 2>&1 &
echo "$!" > "${PREVIEW_ROOT}/http.pid"
```

Inspect representative pages in a browser. For docs PRs, check:

- the changed hand-authored pages;
- generated integration pages affected by `metadata.yaml`;
- the category index page;
- a previously failing ingest, MDX or link-check page, when one exists.

Report:

- requested source and resolved/base commit, source mode and manifest path;
- selected untracked files, tracked deletions, submodule coverage and derived generation, when applicable;
- Learn ref and resolved commit used;
- ingest command and exit status;
- build command and exit status;
- inspected URLs and HTTP/browser status;
- PR-specific warnings or failures;
- pre-existing global warnings separately.
