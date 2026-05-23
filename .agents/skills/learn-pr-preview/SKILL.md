---
name: learn-pr-preview
description: Use only when the user explicitly asks to build, run, preview, inspect, or validate learn.netdata.cloud locally using the contents of a PR or documentation branch before merge. Do not trigger for ordinary docs edits unless the user asks for a local Learn preview.
---

# learn-pr-preview

Build and inspect a local Learn site from a PR's documentation content without
dirtying the real Learn checkout.

Always load `learn-site-structure` first. If the PR touches `metadata.yaml` or
generated integration pages, also load `integrations-lifecycle`.

## Rules

- Trigger only on an explicit preview/build/inspect request.
- Do not run ingest directly in a dirty Learn worktree.
- Use an isolated preview directory under `/tmp` or the repo's gitignored
  `.local/`.
- Record the Learn branch and commit used for the preview.
- Copy PR source content into an isolated source directory. Prefer committed
  PR content; if validating uncommitted work, copy tracked modified files and
  only intentional untracked docs files after checking `git status --short`.
- Save any preview server PID and kill only that PID when stopping it.
- Treat site-wide warnings as evidence, but separate pre-existing global Learn
  warnings from PR-specific Network Flows or docs warnings.

## Workflow

Set paths:

```bash
REPO_ROOT="$(git rev-parse --show-toplevel)"
PR_NUMBER="<pr-number>"
LEARN_REPO="${NETDATA_REPOS_DIR}/learn"
PREVIEW_ROOT="${TMPDIR:-/tmp}/netdata-learn-preview-pr-${PR_NUMBER}-$(date +%Y%m%d%H%M%S)"
SOURCE_COPY="${PREVIEW_ROOT}/netdata-source"
LEARN_COPY="${PREVIEW_ROOT}/learn"
```

Create isolated copies:

```bash
mkdir -p "${SOURCE_COPY}"
git -C "${REPO_ROOT}" ls-files -co --exclude-standard -z \
  | rsync -a --from0 --files-from=- --ignore-missing-args "${REPO_ROOT}/" "${SOURCE_COPY}/"

git clone --branch "$(git -C "${LEARN_REPO}" branch --show-current)" \
  --single-branch "${LEARN_REPO}" "${LEARN_COPY}"
git -C "${LEARN_COPY}" rev-parse HEAD
```

Install ingest dependencies in the isolated preview:

```bash
python3 -m venv "${PREVIEW_ROOT}/venv"
"${PREVIEW_ROOT}/venv/bin/python" -m pip install --upgrade pip
"${PREVIEW_ROOT}/venv/bin/python" -m pip install \
  -r "${LEARN_COPY}/.learn_environment/ingest-requirements.txt"
```

If the real Learn checkout has compatible `node_modules`, symlink it to avoid a
fresh install:

```bash
ln -s "${LEARN_REPO}/node_modules" "${LEARN_COPY}/node_modules"
```

Run ingest with the PR source:

```bash
cd "${LEARN_COPY}"
"${PREVIEW_ROOT}/venv/bin/python" ingest/ingest.py \
  --local-repo "netdata:${SOURCE_COPY}" \
  --ignore-on-prem-repo \
  --use_plain_https \
  --fail-links-netdata
```

Build with the Netlify-pinned runtime:

```bash
NODE_OPTIONS=--max_old_space_size=4096 \
  npx -y -p node@22.14.0 -p yarn@1.22.22 yarn build
```

Serve the static build for inspection:

```bash
python3 -m http.server 3030 --bind 127.0.0.1 --directory "${LEARN_COPY}/build"
```

Or run it in the background with a PID file:

```bash
python3 -m http.server 3030 --bind 127.0.0.1 --directory "${LEARN_COPY}/build" \
  >"${PREVIEW_ROOT}/http.log" 2>&1 &
echo "$!" > "${PREVIEW_ROOT}/http.pid"
```

Inspect representative pages in a browser. For docs PRs, check:

- the changed hand-authored pages;
- generated integration pages affected by `metadata.yaml`;
- the category index page;
- at least one page that previously failed ingest, MDX, or link checks.

Report:

- Learn branch and commit used;
- ingest command and exit status;
- build command and exit status;
- inspected URLs and HTTP/browser status;
- PR-specific warnings or failures;
- pre-existing global warnings separately.
