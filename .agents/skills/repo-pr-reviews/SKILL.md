---
name: repo-pr-reviews
description: Inspect pull-request comments and reviews, or address them when authorized under the repository review policy — fetch all comments with paranoid pagination, classify by author (AI bot vs human), verify each finding, address it, find similar patterns, reply per-thread, resolve threads, pull SonarCloud findings, check CI before pushing, retrigger AI reviewers (cubic-dev-ai, coderabbitai), and wait for new feedback. Use when the user says "address PR comments", "look at the reviews on PR N", "deal with the bot comments", "there is a coderabbit review", "another review", "iterate on PR N until clean", or anything mentioning PR comments / reviews / cubic / cubic-dev-ai / coderabbit / coderabbitai / copilot / sonar findings on a PR.
---

# PR review handler skill

This skill gathers and verifies PR findings, then applies the authorized handling path below.

## Your role on a PR

Authorization and read-only scope are owned by `AGENTS.md#when-a-sow-is-required`; Git operations by
`AGENTS.md#git-and-pr-workflow`; finding classification, review repetition and stopping by `AGENTS.md#review`.

- **Inspect/report:** requests such as "look at the reviews" gather and verify findings using steps 1-2, then report
  them under step 8. Do not fix source, post replies, resolve threads, mark Sonar findings, or trigger reviewers.
- **Address:** when fixes are authorized, solve the original PR problem and handle verified findings within approved
  scope. Posting replies, resolving threads, changing remote triage state and triggering bots require user
  authorization for those actions; permission to fix code alone does not grant it. Preserve already-granted approval.
  Prepare proposed replies or triage decisions for the user when remote actions are not authorized.
- Fetch all configured finding sources for either path, not just the comments mentioned in the request. The mutation
  steps below apply only to authorized actions. Human-comment handling retains the separate direction requirement
  under "Author classes".

Sources of findings, in priority order:

1. **Human review comments** -- maintainers / devs / community.
2. **AI bot review comments** -- cubic-dev-ai, coderabbitai, etc.
3. **SonarCloud PR findings** -- new code-smell / vulnerability /
   security-hotspot issues introduced by this PR. SonarCloud does NOT
   post these as inline GitHub review-comments; only a QualityGate
   summary is posted to GitHub. The actual findings live behind the
   SonarCloud API and must be pulled explicitly.
4. **CI failures relevant to this PR** -- shellcheck, codeql, build /
   test failures caused by the PR's changes.
5. **Anything else this repo configures** (Codacy, custom workflows, ...).

A finding is "relevant to this PR" if its existence (or its line
location) is plausibly caused by the PR's diff. CI failures unrelated to
this PR (a flaky test on an unrelated module, an infra outage) are NOT
in scope -- note them, surface to the user at the end, do not fix them
here.

The bar is the project's performance, stability, and long-term
maintainability. Don't dismiss findings because they look minor.

## MANDATORY rules

These are non-negotiable. Skipping any of them will cost the user time.

1. **Pagination paranoia.** Do not stop at round numbers. If a fetch returns
   exactly 100 / 200 / 300 items, the round count is suspicious -- GitHub
   pagination defaults to 100, and round-multiples almost always mean there
   is a next page that the previous client missed. Always re-probe with an
   explicit `page=N+1` request. `fetch-all.sh` does this automatically.
2. **Triage every comment.** Verify it and classify it under `AGENTS.md#review`. Handle non-blocking findings under
   `AGENTS.md#scope-discipline-at-every-step` and `AGENTS.md#followup-discipline`; do not silently discard them or
   promote them to shipping blockers merely because a reviewer requested them.
3. **Verify every comment properly.** No shortcuts. Read the code, follow
   the trace, confirm the claim. AI bots produce false positives -- judge
   each one on its merits.
4. **When replies are authorized, reply per-thread, one by one.** No bulk replies. No mechanical "fixed"
   answers. Each thread gets a substantive reply that explains what you
   did or why the comment doesn't apply.
5. **Assess materiality, not phrasing.** Optional improvements receive an explicit disposition; they do not
   independently extend the review cycle (`AGENTS.md#review`).
6. **Explain false positives with evidence.** During authorized fixes, add a source comment only when it clarifies
   otherwise ambiguous intent. Bot confusion alone does not justify changing correct code.
7. **Check CI BEFORE every push, but never WAIT for CI between iterations.**
   Waiting for CI between bot-review cycles destroys throughput -- a CI
   run can take 30+ minutes, and during that time the AI reviewers are
   idle. The right cadence is:
   - Before each push: run `ci-status.sh`. If there are PR-caused failures, fix
     them and bundle into the same push. If checks are still running,
     that's fine -- ignore them and push anyway. The next push triggers
     fresh CI on the new code, which is what we actually care about.
   - After a push: if another round is warranted under `AGENTS.md#review` and trigger comments are authorized,
     re-trigger the selected bots, then use `wait-for-activity.sh` for their feedback.
   - If `wait-for-activity.sh` times out (30 min, no new comments):
     re-check `ci-status.sh`. If checks are still running, that's normal,
     surface to the user. Treat PR-caused failures as blockers; report unrelated failures without fixing them.
8. **Re-trigger selected reviewers when another round is warranted.** Use `AGENTS.md#review` to decide whether to
   repeat a round; do not re-trigger solely to obtain an approval phrase. Trigger comments require authorization.
   - cubic-dev-ai: post a new top-level comment mentioning it
     (`trigger-cubic.sh`).
   - coderabbitai: post a new top-level comment with a command
     (`trigger-coderabbit.sh`; `@coderabbitai review` is incremental,
     `full review` re-reads the whole PR).
   - Copilot: NOT re-triggered. Its billing model means the org normally has
     no credits for it, so a re-request produces nothing and only adds
     latency. `trigger-copilot.sh` is kept for the case where credits exist,
     but it is not part of the loop and Copilot never blocks the exit
     condition. Address any comments it does post like any other AI bot.
9. **Don't loop forever on silent bots.** Some assistants stop responding.
   That's fine. Use `wait-for-activity.sh` with the 30-min timeout and
   move on if nothing changes.
10. **When a bot finds a legit issue, search the WHOLE PR for similar
    issues.** This is the most expensive rule to ignore. AI reviewers
    surface their top 3-7 findings, not the full set. If you fix only the
    ones they pointed at, you'll spend dozens of round-trips discovering
    the rest one at a time. Every round-trip is 30+ minutes of bot
    review latency. **The fix for one issue means a full re-audit of the
    PR for the same class of issue.** Do that before pushing.
11. **Don't trust linters alone -- smoke-test every fix.** Static
    analyzers (shellcheck, etc.) verify a property of the code; they
    don't verify behavior. A "correct per the linter" fix can change
    runtime behavior in subtle ways (e.g. a printf format-string fix
    that stops escape-sequence interpretation, breaking colored output
    that the linter never knew about). After every fix, run the
    affected script (or the smallest invocation that exercises the
    change) and verify the output looks right. "Linter green" is not
    the same as "still works."
12. **Check full-scope review coverage before pushing.** Step 4a applies `AGENTS.md#review`; a push alone does not
    require another review when the current change already has adequate coverage.
13. **Before every authorized push, re-fetch finding sources.** Step 4-pre verifies and dispositions newly arrived
    feedback against the current HEAD; do not leave findings unassessed or assume the fetch prevents later arrivals.

## Author classes -- different handling per class

- **AI bots** (`cubic-dev-ai[bot]`, `coderabbitai[bot]`, `copilot[bot]` and
  variants): handle
  within the authorized path above. Verify every finding; fix, reply and resolve only when those actions are
  authorized. Inspection alone ends with a report.
- **Informational bots** (`sonarqubecloud[bot]`, `github-actions[bot]`,
  `netdata-bot[bot]`): read for signal (e.g. quality
  gate status). They don't usually require a reply.
- **Humans** (developers, maintainers, community): consult the user.
  Maintainer comments matter most -- in this project, we are usually
  contributors, they are the project owners. Do not respond on the user's
  behalf without their direction. Surface human comments to the user with
  a recommendation, then act per their instruction.

## Setup

`gh` CLI authenticated for the repo. Nothing else.

The skill reads `upstream` (or `origin`) from git remotes to derive the
repo slug. Override with `PR_REPO_SLUG=owner/repo` if working cross-repo.

State for each PR is cached under `<repo-root>/.local/audits/pr-reviews/pr-<N>/`:

- `pr.json` -- top-level PR metadata
- `issue-comments.json` -- top-level PR comments (REST)
- `review-comments.json` -- inline review comments (REST)
- `reviews.json` -- review submissions with body (REST)
- `review-threads.json` -- per-thread, with `isResolved` (GraphQL)
- `summary.txt` -- human-readable triage summary
- `FETCH-INCOMPLETE` -- present only while a fetch is running or after one aborted; the snapshot is partial, re-run
  `fetch-all.sh` before reading anything else here

## Workflow

For inspection, gather and verify findings in steps 1-2, then report under step 8. For authorized addressing, apply
steps 3-4, perform only authorized remote actions, and use `AGENTS.md#review` to decide whether steps 5-7 are needed.

### 1a. Fetch all comments (paranoid)

```
bash .agents/skills/repo-pr-reviews/scripts/fetch-all.sh <PR_NUMBER>
```

Tail-prints a `summary.txt` that shows the per-author count and the list of
open review threads. Use this as the input to the rest of the cycle.

### 1b. Fetch SonarCloud PR findings

```
bash .agents/skills/repo-pr-reviews/scripts/fetch-sonar-findings.sh <PR_NUMBER>
```

This script fetches SonarCloud findings that are not posted inline on GitHub:
- `.local/audits/pr-reviews/pr-<N>/sonar-issues.json`
- `.local/audits/pr-reviews/pr-<N>/sonar-hotspots.json`
- a brief summary to stdout (counts by rule and severity).

Requires the same `.env` config the `triage-sonarqube` skill uses
(`SONAR_TOKEN`, `SONAR_HOST_URL`, `SONAR_PROJECT`). If `.env` is missing,
the script prints what's needed and exits.

An empty issue/hotspot list does not prove the quality gate passed. Inspect
`/api/qualitygates/project_status?projectKey=<project>&pullRequest=<PR>` for
failed metric conditions. For duplication, use
`/api/duplications/show?key=<file-component-key>&pullRequest=<PR>`: each
`duplications[].blocks[]` identifies `from`, `size`, and `_ref`; `files`
resolves those references. Preserve test cases when sharing duplicated setup.

### 1c. Note the CI signal as a third source

Run `bash .agents/skills/repo-pr-reviews/scripts/ci-status.sh <PR>` once early to capture which checks are failing
**right now**. You're looking for failures caused by the current PR
(typo in a YAML file you added, a script that doesn't pass shellcheck,
a build that breaks because of the diff). DO NOT fix CI yet -- just note
the failures as input alongside review comments and Sonar findings. They
all get addressed in the same iteration so a single push covers them.

### 2. List open threads (and Sonar findings)

```
bash .agents/skills/repo-pr-reviews/scripts/list-open-threads.sh <PR_NUMBER>          # full bodies
bash .agents/skills/repo-pr-reviews/scripts/list-open-threads.sh <PR_NUMBER> --short  # one line per thread
```

The "short" output is a table: `thread-id | path:line | author`. The full
form prints every comment in each thread.

### 3. For each open thread, ONE AT A TIME

**This is per-thread, not batched.** Do not prepare a list of replies and
fire them in a loop. Do not post all replies first and resolve all later.
Walk one thread at a time:

For thread N:

1. **Read the comment carefully.** What is the bot/dev claiming?
2. **Open the file at the line and verify.** Does the claim hold against
   the current code? Is it valid in context?
3. **Search the whole PR diff (and adjacent code) for the same class of
   issue.** Rule #10 -- this is mandatory. (You only do this sweep once,
   on the first thread of a class -- subsequent threads in the same
   class share the same fix.)
4. **Decide**:
   - If valid -> classify under `AGENTS.md#review`; fix blockers and approved improvements, including similar
     in-scope instances. Record dispositions for non-blocking items under the root scope and follow-up rules.
   - If invalid -> record the evidence; clarify source intent only when warranted under rule 6.
5. **Reply in the thread.**
   ```
   bash .agents/skills/repo-pr-reviews/scripts/reply-thread.sh <PR> <comment-id> "<reply>"
   ```
   `<comment-id>` is the `databaseId` of the FIRST comment in the thread
   (from `review-threads.json` -> `.[].comments.nodes[0].databaseId`).
6. **Resolve the thread immediately after the reply succeeds.** "Succeeds"
   means you saw `posted reply id=...`. Resolving a thread whose reply failed
   hides it from the needs-attention view with nothing written in it, which
   reads to a human as a silently dismissed review.
   ```
   bash .agents/skills/repo-pr-reviews/scripts/resolve-thread.sh <thread-id>
   ```
   `<thread-id>` is the GraphQL node id (`review-threads.json` -> `.[].id`,
   starts with `PRRT_`). Resolving immediately after replying takes the
   thread out of the "needs attention" view; leaving threads open without
   resolution accumulates noise.

Then move to thread N+1. Reply-and-resolve, reply-and-resolve. Never
queue them up.

The reason: the order makes intent visible to humans watching the PR --
they see "agent posted reply, agent resolved" as one motion per thread,
not "agent dumped 14 replies, then dumped 14 resolves". Bulk operations
look mechanical and erode trust in the address pass.

### 3b. Address each Sonar finding

For each issue in `sonar-issues.json` and each hotspot in
`sonar-hotspots.json`:

1. **Read the rule and the message.** What is Sonar claiming?
2. **Open the file at the line and verify.** Does the claim hold against
   the current code?
3. **Search the whole PR diff (and adjacent code) for the same class of
   issue.** Same rule #10 as for review comments. If Sonar flagged one
   instance of S131 (case without default), sweep all case statements.
   If Sonar flagged S2245 (insecure RNG), sweep all `random()` callsites.
4. **Decide**:
   - If valid -> classify under `AGENTS.md#review`; fix blockers and approved improvements, including similar
     in-scope instances. Disposition other findings under the root scope and follow-up rules.
   - If invalid -> record the evidence. When remote triage is authorized, `triage-sonarqube` provides
     `sonar-mark.sh fp <KEY> "<reason>"` to mark it False Positive in SonarCloud. Comments are ASCII-only (Cloudflare).
5. Account for every finding. An open issue count alone is not the stopping condition; use `AGENTS.md#review` and
   report unmet required quality gates explicitly.

For Sonar there is no "thread reply" -- you address the issue with
either a code fix or a `sonar-mark.sh` action. There's nothing to
resolve in GitHub for Sonar findings.

### 4-pre. Before pushing -- refresh and triage findings

Reviewers run in parallel. Multiple bots and humans can be appending
findings WHILE you're addressing the current batch. If you push the
moment your queue is empty, the findings that arrived during this
iteration get attributed to your fresh commit instead of the previous
one -- and on the next round you end up "fixing" findings that no
longer apply because you addressed them implicitly with the next push.
The result: chronic desync, where your commit and the reviewers'
findings are always one round apart.

Before an authorized push, re-fetch all sources (comments, Sonar, CI) and triage new findings against the current
HEAD. Resolve verified blockers before pushing; handle non-blocking items under the root scope/follow-up rules.
This reduces stale work but is not an atomic barrier: new feedback can arrive after the fetch.

```
bash .agents/skills/repo-pr-reviews/scripts/fetch-all.sh <PR_NUMBER>
bash .agents/skills/repo-pr-reviews/scripts/fetch-sonar-findings.sh <PR_NUMBER>
bash .agents/skills/repo-pr-reviews/scripts/ci-status.sh <PR_NUMBER>
```

The `ci-status.sh` line is the third source: a CI failure that is
CAUSED by this PR's changes (added a script that doesn't pass
shellcheck, broke a YAML parse, etc.) is in scope and must be folded
in. CI failures unrelated to this PR are noted, surfaced to the user
at the end, but not fixed here.

If the fresh snapshot contains an unassessed finding, verify and disposition it before pushing. Additional fixes
require relevant validation; repeat review only under `AGENTS.md#review`, not merely because another comment arrived.

### 4a. Before pushing -- verify full-scope review coverage

Ensure the complete current PR diff has independent review coverage under `AGENTS.md#review`. Reuse a completed
review when no condition requiring another round applies; do not launch a reviewer merely because a push is next.
The coordinator owns delegation, validates findings and decides whether a repeat is warranted under the root rule.

A full-scope prompt template:

> Review PR <N> end-to-end on branch <X>, against base <base>. Read AGENTS.md and the active SOW <filename, if any>.
> Check correctness, tests, side effects, security, operations, and similar defects throughout the complete diff.
> For performance-sensitive code, also audit hot-path complexity, allocations, contention and unbounded growth.
> Classify each finding under AGENTS.md Review, with file/line evidence, trigger, consequence and a suggested fix.
> This is read-only: do not edit files, mutate remote or Git state, stop processes, or launch other agents.

Verify reviewer findings before acting. Handle blockers and optional improvements under the root review, scope and
follow-up rules. A reviewer returning no suggestions is not required before proceeding.

### 4b. Before pushing -- check CI for FAILURES (don't wait)

```
bash .agents/skills/repo-pr-reviews/scripts/ci-status.sh <PR_NUMBER>
```

Exit codes:
- `0` -- all green, safe to push
- `2` -- runs in progress -- IGNORE this; push anyway. Waiting for CI
  between iterations destroys throughput. The new push triggers fresh CI
  on the new code, which is what matters.
- `3` -- runs failing -- fix the failures and bundle them into the push.

CI failures unrelated to this PR (a flaky test on a different module, an
infra outage) are NOT in scope for this PR -- note them, surface to the
user, move on. Do not make drive-by fixes here.

### 5. Push, then re-trigger reviewers

When a further review round is warranted under `AGENTS.md#review`, and posting trigger comments is authorized,
run the selected reviewers after the fix commits have been pushed:

```
bash .agents/skills/repo-pr-reviews/scripts/trigger-cubic.sh      <PR_NUMBER>
bash .agents/skills/repo-pr-reviews/scripts/trigger-coderabbit.sh <PR_NUMBER>
```

cubic and coderabbit re-review when mentioned in a new top-level PR comment.
Copilot is deliberately absent: see rule 8. If a new reviewer is selected for repeated review, document its
re-trigger mechanism here; recognizing its comments does not by itself require repeatedly invoking it.

### 6. Wait for new activity

```
bash .agents/skills/repo-pr-reviews/scripts/wait-for-activity.sh <PR_NUMBER>
```

Default timeout 30 min, poll every 30 s. Returns 0 on new activity, 124 on timeout. Use it when awaiting a warranted
review round; a bot's "no new findings" comment is evidence to assess, not a required exit phrase.

What counts as "new activity":
- New issue comment / review comment / review on the PR.
- New commit pushed to the PR head.
- A review thread getting resolved or unresolved (often by a bot saying
  "addressed; resolving" -- without this signal we'd miss thread state
  flips and time out spuriously).

### 7. Loop

Iteration and completion follow `AGENTS.md#review`. Re-fetch and assess new findings when another round is warranted;
do not repeat merely to reach zero open threads, zero optional suggestions, or a particular bot verdict.

- Before concluding, account for findings from every source and report their verified disposition. Required CI,
  quality gates, human decisions and actual merge requirements remain prerequisites for claiming merge readiness.
- A silent reviewer or a wait timeout is not approval. Re-check CI and available feedback, then report any remaining
  coverage or validation limitation; do not loop solely to make the bot respond.
- If required checks are running or human input is pending, report that waiting state without claiming readiness.
  If a required check fails because of this PR, treat it as a blocker. Unrelated failures are reported, not fixed here.

### 8. Final report

When the loop ends, summarize for the user:
- Findings addressed (count by source: review threads, Sonar, CI).
- Any unrelated CI failures observed but not fixed (with check name + URL).
- Any human comments that need their attention.
- Current PR state (mergeable / blocked / awaiting validation or input, decision, head SHA).
- Proposed but unperformed replies or remote actions, remaining optional findings and their dispositions, and any
  review/validation coverage limits.

## Commit message hygiene

Commit messages on the address-the-comments cycle should describe **the
change**, not the reviewer or the cycle:

- BAD: "address copilot comments"
- BAD: "fix bot review feedback"
- GOOD: "scripts: fix dry-run env var name and printf format-string usage"

Never reference an AI tool by name in commit messages or PR bodies. The
work matters; the tool that flagged it does not.

(Comments on the PR are an exception when they're operational mentions
required by the bot itself: `@cubic-dev-ai please review again` is a
direct trigger for that bot, and the trigger script enforces it. Outside
operational triggers, the same rule applies to comments.)

## Replying to bots -- tone

Be substantive but brief. The bot's prompt-text is verbose; your reply
doesn't have to be. Examples:

- For a valid fix: "Fixed in <sha-or-paragraph>: <one-sentence what changed>."
- For a false positive: "False positive -- <one-sentence why>: <evidence
  citation>." Add a code comment if it'll help future reviewers.
- For a partial fix: "Partial -- fixed the immediate case at <line>, but the
  related <other-line> is intentional because <reason>."

## Replying to humans -- consult the user

Maintainer / dev / community comments go to the user FIRST. Your message
should:
1. Quote the relevant part of their comment.
2. State your read of what they're asking for.
3. Propose 1-3 options if it's a design call, or one option if obvious.
4. Wait for the user's decision.

Then act per their direction. Do not respond to humans on the user's
behalf without explicit direction.

## Bot directory

| Bot                          | Role                                       | Re-trigger                                       |
|------------------------------|--------------------------------------------|--------------------------------------------------|
| `cubic-dev-ai[bot]`          | Line-level code review                     | New PR comment mentioning `@cubic-dev-ai`        |
| `coderabbitai[bot]`          | Line-level code review                     | `trigger-coderabbit.sh` (`@coderabbitai review`) -- NEVER `@coderabbit`, that is a different, unrelated GitHub user |
| `copilot[bot]`               | Line-level code review                     | Not re-triggered -- no credits (see rule 8)      |
| `sonarqubecloud[bot]`        | Quality-gate status                        | Auto, on each scan run -- read its issue comment |
| `github-actions[bot]`        | CI status / labels                         | Auto, on each workflow run                        |
| `netdata-bot[bot]`           | Repo automation (labels, etc.)             | Auto                                              |

If a new AI reviewer appears in the project, classify it by adding to
`PR_AI_BOT_RE` in `_lib.sh` so the skill recognizes it.

## Reviewer-specific notes

- **The mention is `@coderabbitai`, never `@coderabbit`.** `@coderabbit` is a
  real, unrelated GitHub user; mentioning it pings a stranger on every
  iteration and never reaches the bot. `trigger-coderabbit.sh` hardcodes the
  correct handle -- do not hand-write the mention. The same care applies to
  `@cubic-dev-ai`. Before posting any comment containing an `@`, check the
  handle against the bot directory above.

- **`coderabbitai[bot]` posts line-level findings**, not just summaries. It was
  originally classified here as informational; it is an AI reviewer and its
  threads need the same verify-reply-resolve treatment as cubic's.
- coderabbit cites external URLs (learn.netdata.cloud, upstream GitHub) as
  evidence. Those citations are often *directionally* right but not
  authoritative for the branch under review -- verify against the source in the
  checkout before acting. Example: it correctly flagged that a Function needs a
  signed-in identity, but the proof is the `HTTP_ACCESS_*` flags in the
  producer, not the doc page it linked.
- Both reviewers will flag a generated page's *content* when the real defect is
  in the generator or the shared template. Fix the producer, regenerate, and say
  so in the reply -- otherwise the same finding returns on the next vendor page.

## Failure modes -- quick diagnosis

| Symptom                                                | Likely cause                                                         |
|--------------------------------------------------------|----------------------------------------------------------------------|
| `fetch-all.sh` returns suspiciously round counts       | Pagination missed pages. Re-run; fetch-all auto-probes when count is a multiple of 100. |
| `fetch-all.sh` aborts with `page N probe FAILED` and leaves `FETCH-INCOMPLETE` in the state dir | `gh` auth or rate limit; the cache is incomplete. Fix `gh auth status`, re-run; do not read the partial dump. |
| A GraphQL helper script fails with `cursor_args[@]: unbound variable` | macOS Bash 3.2 plus `set -u` treats empty array expansion as unbound. Keep `gh api` argument arrays non-empty before expansion or branch the first-page GraphQL call. This affected both `fetch-all.sh` and `wait-for-activity.sh`. |
| `reply-thread.sh` -> 404                               | Wrong comment id (use `databaseId` from `review-threads.json`, not the GraphQL node id). |
| `reply-thread.sh` -> `line N: 2: usage`, yet the thread ends up resolved | The comment id expanded to empty AND the resolve ran anyway. Never chain reply and resolve so that resolve can run after reply fails: run `reply-thread.sh`, confirm it printed `posted reply id=...`, THEN resolve. See the bash-vs-zsh note below. |
| An associative-array lookup (`${MAP[key]}`) is empty in a helper loop | The interactive shell here is zsh, not bash. `declare -A` plus `${MAP[key]}` does not behave the same, so ids silently expand to nothing. Pass literal ids, or drive the loop from `python3` output one line at a time, rather than building a shell map. |
| `resolve-thread.sh` -> "thread not found"              | Used REST id instead of GraphQL node id.                              |
| `trigger-copilot.sh` succeeds but no new review        | Expected. The org has no Copilot review credits, which is why it is not in the loop. Do not wait on it. |
| `trigger-cubic.sh` succeeds but no new review          | cubic ignores comments without an explicit `@cubic-dev-ai` mention. The script always prepends it. |
| `ci-status.sh` exits 3 (failing)                       | Fix PR-caused failures before pushing; report unrelated failures. Exit 3 wins over 2 when checks are also running. |
| `ci-status.sh` exits 2 (running)                       | CI hasn't finished. Push anyway -- waiting on CI between iterations destroys throughput. The next push triggers a fresh CI run on the new code, which is what matters. (See Step 4b.) |
| Bot keeps re-flagging the same line after a fix push   | The bot didn't see the new commit because it wasn't re-triggered.    |
| `wait-for-activity.sh` 124 timeout                     | Check available feedback and CI; use `AGENTS.md#review` and report coverage gaps. Silence or resolved threads alone do not establish readiness. |

## MANDATORY -- keep this skill alive

Capture timing and authorization for operational discoveries follow `AGENTS.md#knowledge-capture`.

Examples of things to capture:
- A new AI reviewer bot that appears in the project (add to the directory + `PR_AI_BOT_RE`)
- A new common false-positive pattern that warrants a clarifying source comment
- A new GitHub API quirk (rate limits, undocumented response shapes, pagination edge cases)
- A retrigger mechanism that changed (e.g. copilot's re-request behavior)
