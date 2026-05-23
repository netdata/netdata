# Preview a documentation PR locally

Question: how do you build Learn locally from a PR's docs content and inspect
it in a browser before merging?

Use an isolated preview directory. Do not run ingest directly in a dirty Learn
checkout because ingest cleans and regenerates `docs/`.

## Evidence

- `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py:2632` defines `--local-repo`.
- `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py:2790` cleans the ingest temp
  folder and `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py:2793` cleans the
  Learn `docs/` tree before publishing.
- `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py:2808` copies a local source repo
  into the ingest temp folder when `--local-repo netdata:<path>` is used.
- `${NETDATA_REPOS_DIR}/learn/.learn_environment/ingest-requirements.txt:1`
  lists the Python dependencies for ingest.
- `${NETDATA_REPOS_DIR}/learn/package.json:8` defines the Docusaurus build
  script.
- `${NETDATA_REPOS_DIR}/learn/netlify.toml:5` pins the Netlify runtime to
  Node `22.14.0`, Yarn, and `NODE_OPTIONS=--max_old_space_size=4096`.

## Procedure

1. Pick the PR source repo and the Learn checkout:

   ```bash
   REPO_ROOT="$(git rev-parse --show-toplevel)"
   PR_NUMBER="<pr-number>"
   LEARN_REPO="${NETDATA_REPOS_DIR}/learn"
   PREVIEW_ROOT="${TMPDIR:-/tmp}/netdata-learn-preview-pr-${PR_NUMBER}-$(date +%Y%m%d%H%M%S)"
   SOURCE_COPY="${PREVIEW_ROOT}/netdata-source"
   LEARN_COPY="${PREVIEW_ROOT}/learn"
   ```

2. Copy the PR source into an isolated directory:

   ```bash
   mkdir -p "${SOURCE_COPY}"
   git -C "${REPO_ROOT}" ls-files -co --exclude-standard -z \
     | rsync -a --from0 --files-from=- --ignore-missing-args "${REPO_ROOT}/" "${SOURCE_COPY}/"
   ```

   This includes tracked files and intentional untracked files that are not
   ignored by Git, while still excluding ignored build and scratch output.

3. Clone the local Learn checkout into the preview directory:

   ```bash
   git clone --branch "$(git -C "${LEARN_REPO}" branch --show-current)" \
     --single-branch "${LEARN_REPO}" "${LEARN_COPY}"
   git -C "${LEARN_COPY}" rev-parse HEAD
   ```

4. Install ingest dependencies:

   ```bash
   python3 -m venv "${PREVIEW_ROOT}/venv"
   "${PREVIEW_ROOT}/venv/bin/python" -m pip install --upgrade pip
   "${PREVIEW_ROOT}/venv/bin/python" -m pip install \
     -r "${LEARN_COPY}/.learn_environment/ingest-requirements.txt"
   ```

5. Reuse Learn `node_modules` when compatible:

   ```bash
   ln -s "${LEARN_REPO}/node_modules" "${LEARN_COPY}/node_modules"
   ```

6. Run ingest with the PR content and fail on broken links from `netdata`:

   ```bash
   cd "${LEARN_COPY}"
   "${PREVIEW_ROOT}/venv/bin/python" ingest/ingest.py \
     --local-repo "netdata:${SOURCE_COPY}" \
     --ignore-on-prem-repo \
     --use_plain_https \
     --fail-links-netdata
   ```

7. Build with the Netlify-pinned runtime:

   ```bash
   NODE_OPTIONS=--max_old_space_size=4096 \
     npx -y -p node@22.14.0 -p yarn@1.22.22 yarn build
   ```

8. Serve and inspect:

   ```bash
   python3 -m http.server 3030 --bind 127.0.0.1 --directory "${LEARN_COPY}/build"
   ```

   Open changed pages, generated integration pages, and the affected category
   index. Confirm HTTP 200, the expected H1, no 404 page, and no MDX/runtime
   error.

## Reporting

Report these facts:

- Learn branch and commit used.
- Ingest command and exit status.
- Build command and exit status.
- Browser-inspected URLs and status.
- PR-specific warnings/failures.
- Pre-existing site-wide warnings separately.

## How I Figured This Out

Read `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py`,
`${NETDATA_REPOS_DIR}/learn/package.json`,
`${NETDATA_REPOS_DIR}/learn/netlify.toml`, and
`${NETDATA_REPOS_DIR}/learn/.learn_environment/ingest-requirements.txt`.
Validated the flow by building an isolated Learn preview from a Network Flows
documentation PR, running ingest with `--fail-links-netdata`, running
`yarn build`, serving the static build, and checking representative pages in a
browser.
