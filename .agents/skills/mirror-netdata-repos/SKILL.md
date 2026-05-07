---
name: mirror-netdata-repos
description: Maintains a local mirror of Netdata-org source repositories at `${NETDATA_REPOS_DIR}` so AI assistants and developers can do cross-repo grep / code review locally without GitHub API round-trips and rate limits. Ships a vendored sync script (`scripts/sync-netdata-repos.sh`) that updates ~150 repos in two phases (resync existing on default branch, discover and clone new). Safety -- skips repos that have staged or modified changes; otherwise switches to the default branch and recursively updates submodules. Reset-to-default is intentional -- it prevents stale-feature-branch "black hole" repos that confuse cross-repo reasoning. Supports `--repo NAME` (repeatable) to scope to specific repos. Independent from any other repo mirrors this workstation may have. Use when the local mirror is out of date, before a cross-repo grep / review session, when adding a new netdata-org repo (auto-discovered), when an assistant needs cross-repo cognition without `gh` API turnaround.
---

# mirror-netdata-repos

A local mirror of every active Netdata-org source repository,
synced by a vendored bash script. Built for AI assistants
(and humans) that need cross-repo grep, code review, and
pattern lookup without paying GitHub API costs.

## Why this skill exists

Netdata maintains ~150 active source repos across the
`netdata` GitHub org (the agent monorepo, cloud-* services,
ai-agent, charts, helmchart, blogs, dashboards, ...). Routine
work (cross-repo grep, "how does service X handle this?",
pattern lookup, build) needs all of them locally.

Without a local mirror:
- Each cross-repo question hits the GitHub API.
- Searches are paginated and rate-limited.
- An AI assistant cannot pipeline grep results across repos.
- Iteration speed and reasoning depth fall through the floor.

With a local mirror at `${NETDATA_REPOS_DIR}`, all of that is
fast local I/O.

This is a **netdata repos mirror, independent from any other
repo mirrors this workstation may have**. It exists for this
project's cross-repo work; it is not a generic research mirror.

## How it works

The vendored script `scripts/sync-netdata-repos.sh` does two
phases:

### Phase 1 -- update existing repos

For each `.git`-bearing subdirectory under `${NETDATA_REPOS_DIR}`,
sorted by recent activity (cached in `.repo-activity-cache`):

- If staged OR modified files exist -> **skip** with details.
- Else: detect default branch (master / main / develop),
  switch to it (committed feature-branch state survives in
  the branch ref), `git pull`, and `git submodule update
  --init --force --recursive`.

### Phase 2 -- discover and clone new repos

Runs only when:
- no `--repo` flag was given (full sync), AND
- `gh` is available AND authenticated.

Lists `gh repo list netdata --source --no-archived` and clones
any that are not yet in the mirror. The `--source --no-archived`
filter excludes forks and dead repos -- they add no value for
cross-repo grep.

If `gh` is missing or not authenticated, Phase 2 is skipped
with a clear warning. Phase 1 still runs (it uses local `git`
only, no GitHub API).

## Reset-to-default-branch is the feature, not a hazard

Sub-repos in a mirror tend to drift onto stale feature
branches that no one remembers. A repo whose `HEAD` is on
`fix/something-from-six-months-ago` is a **black hole** for
cross-repo reasoning -- the assistant grepping it sees
out-of-date code and reasons wrong.

The only viable fix: always reset to the default branch when
it's safe to do so. The script's safety conditions:

- **Untracked files**: OK; they survive checkout.
- **Staged or modified files**: NOT safe; script skips the
  repo and prints what was found.
- **Unpushed feature-branch commits**: SAFE; the branch ref
  preserves them, no data is lost. Script switches to default
  with a warning summarizing the unpushed commits.

So the rule is: if you have working changes you want to keep,
commit them or stash them before running this. Anything else
the script handles correctly.

## When to run

- **Before any cross-repo grep / review** session.
- **After a long absence** from the workstation (catches up to
  upstream on every repo).
- **When you've just added a new netdata-org repo**: nothing
  to do manually; the next full run picks it up via Phase 2.
- **Periodically** (daily / weekly) to keep the mirror fresh.

There's no automation here; the script is interactive (colored
output, end-of-run summary). Run it on demand.

## Setup

### One-time

1. Pick a directory for the mirror (large; expect 30-50 GB).
2. Set `NETDATA_REPOS_DIR` in `<repo>/.env`:
   ```
   NETDATA_REPOS_DIR="/path/to/your/mirror"
   ```
3. `mkdir -p "$NETDATA_REPOS_DIR"`.
4. Required tools: `git` and `jq`. Install via your package
   manager.
5. For Phase 2 (auto-discovery): install `gh` (the GitHub CLI)
   and run `gh auth login`. SSH access to `git@github.com:netdata/...`
   must work for clones.

### First sync

```bash
# Source the env, run the script.
source <(grep -E '^NETDATA_REPOS_DIR=' <repo>/.env)
.agents/skills/mirror-netdata-repos/scripts/sync-netdata-repos.sh
```

Or with the variable inline:

```bash
NETDATA_REPOS_DIR="/path/to/mirror" \
  .agents/skills/mirror-netdata-repos/scripts/sync-netdata-repos.sh
```

The first run clones every netdata-org source repo. Expect it
to take several minutes; subsequent runs are fast (only
fetch+pull on each repo).

## Common usage

### Sync everything (default)

```bash
.agents/skills/mirror-netdata-repos/scripts/sync-netdata-repos.sh
```

### Sync just one or two repos

```bash
.agents/skills/mirror-netdata-repos/scripts/sync-netdata-repos.sh \
    --repo netdata \
    --repo cloud-frontend
```

`--repo` is repeatable. When any `--repo` is given, Phase 2
(discovery) is skipped -- you asked for specific repos, the
script does not go looking for new ones.

### See help

```bash
.agents/skills/mirror-netdata-repos/scripts/sync-netdata-repos.sh --help
```

Works without `NETDATA_REPOS_DIR` set.

## Reading the output

The script prints colored per-repo progress and ends with a
summary covering:

- **Branches switched to default**: every repo that was on a
  non-default branch and got switched. Inspect this list if
  you had work in progress.
- **Repositories with uncommitted changes (skipped)**: these
  weren't synced. Commit / stash / revert and re-run.
- **Repositories with unpushed commits**: switched to default,
  but you have feature-branch commits that haven't been
  pushed. The branch ref preserves them; push when you're
  ready.
- **Repositories on wrong branch**: tried to switch but
  failed (rare). Manual intervention needed.
- **Repositories that failed to update**: fetch or pull
  failure. Inspect manually.

## Adding a new netdata-org repo

Nothing to do in this skill or its script. Phase 2's
`gh repo list netdata --source --no-archived` discovers any
new netdata-org repo on the next full sync run. The repo
must be:

- Owned by the `netdata` org (not a fork).
- Not archived.

Otherwise it's skipped intentionally.

If you want to mirror a fork or an archived repo (rare),
clone it manually into `${NETDATA_REPOS_DIR}/<name>` and the
next run's Phase 1 will start syncing it.

## Sanitization (what the script checks)

The script refuses to run unsafely. Hard errors (exit 2):

- `NETDATA_REPOS_DIR` not set.
- `NETDATA_REPOS_DIR` set but the directory doesn't exist.
- `git` not in `PATH`.
- `jq` not in `PATH`.

Soft warnings (Phase 2 skipped, Phase 1 still runs):

- `gh` not installed.
- `gh` installed but not authenticated.

## Limitations

- **Org is hardcoded** (`netdata`). This skill is
  netdata-org-specific.
- **Filter is hardcoded** (`--source --no-archived`). To
  mirror forks or archived repos, clone them manually -- the
  script will then sync them in Phase 1.
- **`gh` rate limit**: Phase 2 calls `gh repo list netdata
  --limit 1000` once per run. On a properly-authed `gh` this
  is well within the limit.
- **Submodule `--force --recursive`**: intentional. Cross-repo
  review and most build steps depend on accurate, up-to-date
  submodule state. Local submodule modifications are
  overwritten -- if you have work-in-progress inside a
  submodule, commit it before running.

## Path discipline

This skill follows
`<repo>/.agents/sow/specs/sensitive-data-discipline.md`:

- All references to the mirror directory go through
  `${NETDATA_REPOS_DIR}` (the env key from `.env`).
- The script itself contains no hardcoded user paths.
- The skill content contains no workstation paths.

## See also

- `<repo>/.agents/skills/mirror-netdata-repos/scripts/sync-netdata-repos.sh`
  -- the vendored script.
- `<repo>/.agents/skills/mirror-netdata-repos/how-tos/INDEX.md`
  -- live catalog of how-tos.
- `<repo>/.env` -- where `NETDATA_REPOS_DIR` lives.
