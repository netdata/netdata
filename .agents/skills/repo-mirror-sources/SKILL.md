---
name: repo-mirror-sources
description: Inspect Netdata-org source checkouts under NETDATA_REPOS_DIR, or set up and synchronize that mirror when requested. Use for cross-repo lookup, review, missing sources, mirror freshness, or scoped sync. Source inspection does not require syncing; maintenance can switch branches, pull, clone, and force-update submodules.
---

# Netdata source mirrors

Use existing local source checkouts for cross-repo search and review. This Netdata-org mirror is independent of other
research mirrors. Its path is `${NETDATA_REPOS_DIR}`; configuration is documented in `.agents/ENV.md`.

## Choose the task

| Task | Read / do |
|---|---|
| Inspect, compare, or review source | Use the read-only route below; load relevant domain skills for the assigned lens |
| Establish whether local evidence is current enough | Record checkout revision/state and the question's required revision; do not assume local means latest |
| Set up, refresh, or troubleshoot the mirror | Read [maintenance.md](maintenance.md) before any sync; apply the scope already authorized by the task |
| Understand helper behavior | Read [scripts/sync-netdata-repos.sh](scripts/sync-netdata-repos.sh); its checks are not a general preservation guarantee |
| Capture a reusable mirror procedure | Follow [how-tos/INDEX.md](how-tos/INDEX.md) and `AGENTS.md#knowledge-capture` |

A request for cross-repo grep or review does not authorize sync, checkout, fetch, pull, clone, stash, commit, or push.
A task that already authorizes maintenance needs no repeated permission for the same scope. Prefer named repository
updates for a narrow maintenance request; full organization discovery is a distinct scope.

## Inspect existing source

Resolve only the required configured path; do not print unrelated environment values or load credentials to grep
local files. The helper reads an exported `NETDATA_REPOS_DIR` and does not source `.env` itself. Set it explicitly
from the trusted project configuration when needed; maintenance setup is in `maintenance.md`.

For a known checkout such as `learn`, inspect its state before using it as evidence:

```bash
: "${NETDATA_REPOS_DIR:?Set the configured mirror directory}"
mirror_repo="$NETDATA_REPOS_DIR/learn"
git --no-optional-locks -C "$mirror_repo" rev-parse HEAD
git --no-optional-locks -C "$mirror_repo" symbolic-ref --quiet --short HEAD || true
git --no-optional-locks -C "$mirror_repo" status --short --untracked-files=all --ignore-submodules=none
rg --files "$mirror_repo"
```

Use `rg` on the relevant files after locating them. A blank branch result may mean detached HEAD; a failed Git
command is not a clean-state result. An available source checkout can be nested or use a `.git` file even though the
sync helper only recognizes immediate repositories with `.git` directories.

Record the upstream owner/repository and exact checked commit under `AGENTS.md#open-source-reference-evidence`.
Distinguish working-tree edits from committed source; do not attribute edited contents to an unmodified commit.
Existing sources can answer questions about their recorded revision without a refresh. Check the requested revision
when reviewing a PR or historical behavior; switching to default would discard the intended review context.

If required sources are absent or freshness cannot be established, report that limit and determine whether the task
already authorizes a scoped refresh or acquisition. Do not silently run the full sync to resolve one missing checkout.
A current local timestamp, branch name, or old `origin/HEAD` reference does not prove live upstream freshness.

## Maintenance boundary

The vendored helper performs existing-repository updates and optional organization discovery. It switches branches,
fetches/pulls, force-updates recursive submodules, and writes the mirror activity cache. `--repo` selects existing
repositories and disables discovery; it does not confine every side effect to those repositories.

Read `maintenance.md` for prerequisites, exact selection behavior, safety limits, and post-run checks. Help alone
requires neither configuration nor installed Git tooling:

```bash
.agents/skills/repo-mirror-sources/scripts/sync-netdata-repos.sh --help
```

No scheduling automation is installed by this skill. A long absence, new repository, or periodic freshness requirement
can justify an authorized maintenance task; none makes maintenance a prerequisite to every source read.

## Path discipline

`.agents/sensitive-data-discipline.md#allowed-alternatives` owns path notation. Use `${NETDATA_REPOS_DIR}` for mirror
paths in skills and evidence; do not embed personal workstation paths in this skill or its helper.
