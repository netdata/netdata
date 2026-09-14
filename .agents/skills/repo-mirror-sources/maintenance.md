# Maintain the Netdata source mirror

Read this for authorized setup, synchronization, or sync failure investigation. For source inspection use
`./SKILL.md#inspect-existing-source`. The implementation is `scripts/sync-netdata-repos.sh`; this guide describes its
usable contract and limits, not a guarantee that it preserves arbitrary development work.

## Before running

- Confirm the authorized repositories and the intended source revision. Default-branch maintenance is inappropriate
  for a checkout intentionally holding a PR or historical revision, or currently used by another task.
- Use existing, verified immediate child checkouts with simple repository basenames. The helper does not validate
  origins or reject path traversal, absolute paths, and directory symlinks. Do not pass those as `--repo` names or
  rely on the helper to prove containment.
- Inspect tracked, untracked, ignored, and recursive submodule state before authorizing mutation. Require a valid
  attached HEAD and verify the intended default branch. Valuable work, unknown state, detached commits without a
  durable branch ref, or conflicting files need preservation decisions before maintenance. Do not automatically
  commit, stash, revert, or push just to make a sync proceed; those actions need their own task authorization.
- The helper uses `git submodule update --init --force --recursive`. This can overwrite submodule modifications.
  Top-level dirty checks, especially with configured submodule ignores, do not prove nested safety. A commit alone
  does not establish that a detached submodule commit remains on a named branch. Use this helper only for mirror
  checkouts whose update and recursive submodule replacement are within the authorized maintenance scope.

The helper normally skips staged/modified top-level state and `diff-index` errors. Its invalid-HEAD path does not
provide the same protection, and untracked files are informational rather than a gate; collisions can prevent
checkout/pull. These checks do not replace the preflight above.

## Setup and invocation

1. Choose and create the intended mirror directory; size depends on the repositories and submodules selected.
2. Set `NETDATA_REPOS_DIR` using `.agents/ENV.md`. The project may store it in `.env`; the helper reads only the
   exported environment and does not source that file. Do not source unrelated credentials for this task.
3. `git` and `jq` are required, including for scoped updates. Discovery additionally needs available authenticated
   `gh` and SSH access to the repositories; install/authenticate tools only as needed for authorized setup.

For example, after selecting the actual mirror path:

```bash
export NETDATA_REPOS_DIR='/path/to/mirror'
mkdir -p "$NETDATA_REPOS_DIR"
```

A named update uses existing repositories only:

```bash
.agents/skills/repo-mirror-sources/scripts/sync-netdata-repos.sh \
  --repo netdata \
  --repo cloud-frontend
```

`--repo` is repeatable. Missing names warn and are skipped; they are not cloned. Do not replace a failed named update
with an unrequested full sync. For a missing repository, scope an explicit clone or organization-discovery task.
`NETDATA_REPOS_DIR` may also be supplied inline for one invocation instead of exported.

For an explicitly requested full mirror update and discovery, omit the selector:

```bash
.agents/skills/repo-mirror-sources/scripts/sync-netdata-repos.sh
```

No `gh` or authentication means discovery is skipped with a warning; existing updates can still use Git transport.
They require network access for fetch/pull even though they do not use the GitHub API. `gh auth status` is probed during
initialization even on scoped runs. This helper is an on-demand command with terminal progress, not a scheduler.

## Selection and changes

| Mechanism | Current behavior / limit |
|---|---|
| Existing repositories | Immediate children with `.git` directories; linked worktrees with `.git` files are excluded |
| Update order | Full runs use `.repo-activity-cache`; scoped runs use the requested list |
| Default branch | Cached `origin/HEAD`, then existing `origin/master`, `origin/main`, `origin/develop`; this occurs before fetch and can be stale |
| Unknown default | Skip the repository before checkout, fetch, pull, or submodule updates |
| Failed branch switch | Skip that repository without pulling its current branch |
| Named branch state | Switching preserves its branch ref; this does not protect an unreferenced detached commit |
| Unpushed warning | Compares against an existing cached `origin/<current-branch>` before fetch; absent remote refs and detached commits are not covered |
| Pull | `git fetch origin`, then `git pull origin <selected-branch>`; pull follows the repository's Git configuration, not a guaranteed fast-forward-only policy |
| Submodules | Initialize and force-update recursively; failures appear inline as warnings |
| Discovery | Without `--repo`, authenticated `gh repo list netdata --source --no-archived --limit 1000`; clone missing names over SSH recursively |
| Discovery exclusions | Forks and archived repositories are not automatically discovered; manually cloned ones can participate in existing-repository updates |
| Shared cache | Refreshed across the mirror after both scoped and full runs; newly added directories can miss one cached full-run selection, so use an explicit selector when needed |

`get_last_activity` uses GNU-style `stat -c`; an incompatible platform falls back to zero, so activity sorting may not
reflect recency. Discovery's limit is not proof that every organization repository was listed. The script has no
hardcoded workstation path and does not manage other research mirrors.

## Verify the result

Keep the command output for the task: the helper has no persistent per-run report. Inspect individual messages for
switched branches, skipped dirty state, unpushed commits, unknown or failed default selection, fetch/pull failures,
submodule warnings, and discovery/clone failures. Investigate failures without clearing existing work automatically.

Exit 2 reports configuration/tool or argument errors. Ordinary exit 0 and the green final summary are **not** aggregate
success guarantees: repository, discovery, clone, and submodule failures may be continued or omitted from summary
arrays. Missing scoped names can also end with a success message.

Re-check each requested checkout's branch, commit, working-tree and submodule state; compare against the required
revision or upstream evidence for the task. Report skipped or incomplete results explicitly. Neither “Sync complete”
nor a cached remote reference proves that every requested source and submodule is current.
