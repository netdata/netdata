---
name: pr-reviews
description: Address pull-request comments and reviews iteratively until the PR is clean — fetch all comments with paranoid pagination, classify by author (AI bot vs human), verify each finding, address it, find similar patterns, reply per-thread, resolve threads, check CI before pushing, retrigger AI reviewers (cubic-dev-ai, copilot), and wait for new feedback. Use when the user says "address PR comments", "look at the reviews on PR N", "deal with the bot comments", "iterate on PR N until clean", or anything mentioning PR comments / reviews / cubic / copilot.
---

# PR review handler skill

This skill iterates a PR through review/comment cycles until there is nothing
left to address.

## Your role on a PR

When this skill is in use, the agent's job is to **bring the PR into
merge-ready shape**: solve the original problem the PR was opened for,
**and** address every legitimate finding the PR has accumulated, from
every source. Reviewers, linters, and CI all matter. Comments are the
loudest source but they are not the only source -- you must proactively
pull findings from every channel that reports on the PR, not wait for
something to surface as a chat message.

Sources of findings, in priority order:

1. **Human review comments** -- maintainers / devs / community.
2. **AI bot review comments** -- cubic-dev-ai, copilot, etc.
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
2. **Accept all comments and address them all.** No exceptions. No "this is
   minor, skip it." The bar is: Netdata's performance, stability, and
   long-term maintainability.
3. **Verify every comment properly.** No shortcuts. Read the code, follow
   the trace, confirm the claim. AI bots produce false positives -- judge
   each one on its merits.
4. **Reply per-thread, one by one.** No bulk replies. No mechanical "fixed"
   answers. Each thread gets a substantive reply that explains what you
   did or why the comment doesn't apply.
5. **Don't dismiss comments because they look minor.** Even style nits
   compound. The goal is for the project to thrive.
6. **Help bots when they're confused.** AI reviewers sometimes flag false
   positives because the surrounding code is ambiguous. Add a short
   comment in the source that clarifies the intent -- it helps the next
   reviewer (human or bot).
7. **Check CI BEFORE every push, but never WAIT for CI between iterations.**
   Waiting for CI between bot-review cycles destroys throughput -- a CI
   run can take 30+ minutes, and during that time the AI reviewers are
   idle. The right cadence is:
   - Before each push: run `ci-status.sh`. If there are FAILURES, fix
     them and bundle into the same push. If checks are still running,
     that's fine -- ignore them and push anyway. The next push triggers
     fresh CI on the new code, which is what we actually care about.
   - After each push: re-trigger the bots, then `wait-for-activity.sh`
     for new comments (NOT for CI).
   - If `wait-for-activity.sh` times out (30 min, no new comments):
     re-check `ci-status.sh`. If checks are still running, that's normal,
     surface to the user. If there are failures, fix and iterate.
8. **Re-trigger AI reviewers explicitly.** They do NOT react to thread
   replies or pushed commits the way humans do.
   - Copilot: re-add as a requested reviewer (`trigger-copilot.sh`).
   - cubic-dev-ai: post a new top-level comment mentioning it
     (`trigger-cubic.sh`).
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
12. **Before every push, spawn a subagent for a holistic PR review.**
    See Step 4a in the workflow. The orchestrator's context is biased
    toward the fixes it just made; a clean-context subagent re-reviewing
    the WHOLE diff is what catches the issues the orchestrator and the
    AI reviewers missed. Skipping this turns each iteration into a
    30-minute round-trip to discover issues that could have been found
    in 2 minutes locally.
13. **Before every push, re-fetch all finding sources one last time.**
    See Step 4-pre. Reviewers post in parallel; if findings arrive
    while you're addressing the current batch, they belong in THIS
    push, not the next one. Without this sync barrier, you and the
    reviewers stay one round out of sync forever -- the next iteration
    is always "fixing" issues that no longer apply.

## Author classes -- different handling per class

- **AI bots** (`cubic-dev-ai[bot]`, `copilot[bot]` and variants): handle
  autonomously. Verify the finding, fix or push back with reasoning, reply
  in-thread, resolve thread.
- **Informational bots** (`sonarqubecloud[bot]`, `github-actions[bot]`,
  `netdata-bot[bot]`, `coderabbitai[bot]`): read for signal (e.g. quality
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

## Workflow

The order is: **gather all findings -> address them per-thread / per-finding
-> check CI for failures the PR caused -> push -> retrigger -> wait -> loop.**

### 1a. Fetch all comments (paranoid)

```
bash .agents/skills/pr-reviews/scripts/fetch-all.sh <PR_NUMBER>
```

Tail-prints a `summary.txt` that shows the per-author count and the list of
open review threads. Use this as the input to the rest of the cycle.

### 1b. Fetch SonarCloud PR findings

```
bash .agents/skills/pr-reviews/scripts/fetch-sonar-findings.sh <PR_NUMBER>
```

SonarCloud findings are NOT delivered as inline GitHub comments -- only a
QualityGate summary is. The actual issue list lives behind the SonarCloud
API. This script writes:
- `.local/audits/pr-reviews/pr-<N>/sonar-issues.json`
- `.local/audits/pr-reviews/pr-<N>/sonar-hotspots.json`
- a brief summary to stdout (counts by rule and severity).

Requires the same `.env` config the `sonarqube-audit` skill uses
(`SONAR_TOKEN`, `SONAR_HOST_URL`, `SONAR_PROJECT`). If `.env` is missing,
the script prints what's needed and exits.

### 1c. Note the CI signal as a third source

Run `bash .agents/skills/pr-reviews/scripts/ci-status.sh <PR>` once early to capture which checks are failing
**right now**. You're looking for failures caused by the current PR
(typo in a YAML file you added, a script that doesn't pass shellcheck,
a build that breaks because of the diff). DO NOT fix CI yet -- just note
the failures as input alongside review comments and Sonar findings. They
all get addressed in the same iteration so a single push covers them.

### 2. List open threads (and Sonar findings)

```
bash .agents/skills/pr-reviews/scripts/list-open-threads.sh <PR_NUMBER>          # full bodies
bash .agents/skills/pr-reviews/scripts/list-open-threads.sh <PR_NUMBER> --short  # one line per thread
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
   - If valid -> fix it AND every similar instance you found.
   - If invalid -> understand why the bot got confused. Often a small
     source comment clarifying the intent will help the next reviewer.
5. **Reply in the thread.**
   ```
   bash .agents/skills/pr-reviews/scripts/reply-thread.sh <PR> <comment-id> "<reply>"
   ```
   `<comment-id>` is the `databaseId` of the FIRST comment in the thread
   (from `review-threads.json` -> `.[].comments.nodes[0].databaseId`).
6. **Resolve the thread immediately after the reply succeeds.**
   ```
   bash .agents/skills/pr-reviews/scripts/resolve-thread.sh <thread-id>
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
   - If valid -> fix it AND every similar instance you found in the
     project where the same reasoning applies (within the diff, plus
     nearby code in files this PR already touches).
   - If invalid -> the corresponding `sonarqube-audit` skill provides
     `sonar-mark.sh fp <KEY> "<reason>"` to mark it False Positive
     directly in SonarCloud. Comments are ASCII-only (Cloudflare).
5. **Repeat until `sonar-issues.json` has zero issues that we caused.**

For Sonar there is no "thread reply" -- you address the issue with
either a code fix or a `sonar-mark.sh` action. There's nothing to
resolve in GitHub for Sonar findings.

### 4-pre. Before pushing -- MANDATORY final-fetch sync barrier

Reviewers run in parallel. Multiple bots and humans can be appending
findings WHILE you're addressing the current batch. If you push the
moment your queue is empty, the findings that arrived during this
iteration get attributed to your fresh commit instead of the previous
one -- and on the next round you end up "fixing" findings that no
longer apply because you addressed them implicitly with the next push.
The result: chronic desync, where your commit and the reviewers'
findings are always one round apart.

The fix: a sync barrier immediately before push. Re-fetch ALL sources
(comments, Sonar, CI) one more time. If ANY new finding has arrived
since you last looked, loop back to step 2 -- address those new
findings in the SAME upcoming push -- then re-fetch again. Only push
when a fresh fetch comes back with no new findings against the current
HEAD. This guarantees you and the reviewers are synchronized.

```
bash .agents/skills/pr-reviews/scripts/fetch-all.sh <PR_NUMBER>
bash .agents/skills/pr-reviews/scripts/fetch-sonar-findings.sh <PR_NUMBER>
bash .agents/skills/pr-reviews/scripts/ci-status.sh <PR_NUMBER>
```

The `ci-status.sh` line is the third source: a CI failure that is
CAUSED by this PR's changes (added a script that doesn't pass
shellcheck, broke a YAML parse, etc.) is in scope and must be folded
in. CI failures unrelated to this PR are noted, surfaced to the user
at the end, but not fixed here.

If `summary.txt` shows any new open thread or `sonar-issues.json` shows
any new issue you haven't addressed yet, **do NOT push**. Loop back to
step 2 and address them first. Then re-run the fetch. Only push when
the fetch is clean.

The same loop applies during the iteration: if you re-fetched while
addressing the previous batch and saw new findings drop in, fold them
into the same push rather than dispatching a half-done batch.

### 4a. Before pushing -- MANDATORY holistic PR review via subagent

This is the most important pre-push step. Skipping it is what makes
review cycles last for hours.

After you have made all the fixes for the current iteration's findings
but BEFORE running `git push`, spawn a subagent to re-review the WHOLE
PR diff (not the small change you just made). The orchestrator's
context is already loaded with the recent fixes; the subagent's clean
context is what gives an honest second look.

Why this is non-negotiable:

- AI reviewers (cubic-dev-ai, copilot, sonarqube) only surface their
  top 3-7 findings. The full set of similar issues remains hidden.
- Each fix can introduce its own new problems (a printf format-string
  fix that breaks color rendering, a portability fix that drops a
  feature, an input-validation fix that rejects valid inputs).
- Without a holistic pre-push review, every iteration takes ~30 min of
  bot review latency just to discover problems the orchestrator could
  have spotted in 2 minutes by re-reading the diff.

How to invoke:

Use the orchestrator's Agent / subagent tool (whatever the harness
provides). Pass the subagent the PR diff (or the list of touched files)
and ask it to:

- Verify each fix in this iteration solves the original finding without
  side effects.
- Sweep the touched files for similar patterns the original findings
  did not point at, but which the same reasoning would flag.
- Sweep the touched files for NEW issues the fixes themselves may have
  introduced (broken behavior, lost features, regressions).
- Report findings as a flat list -- file:line + class + suggested fix.

Then the orchestrator addresses every finding the subagent returns
BEFORE push. Loop the subagent if necessary until it returns a clean
review. Only then proceed to step 4b.

A good subagent prompt template:

> Re-review PR <N> end-to-end. The current diff is on branch <X>; the
> base is <master|...>. Recent fixes addressed: <list>. For the WHOLE
> diff (not just the recent fixes), find:
> 1. Similar patterns to the ones recently fixed that were NOT pointed
>    at by reviewers but where the same reasoning applies.
> 2. Issues the recent fixes may have introduced (regressions, broken
>    behavior, dropped features).
> 3. Anything in the diff that does not match the project's
>    conventions (AGENTS.md, sibling files, the rest of the repo).
> Report a flat list of file:line + class + suggested fix. Be
> exhaustive; do not stop at 3-7 findings.

### 4b. Before pushing -- check CI for FAILURES (don't wait)

```
bash .agents/skills/pr-reviews/scripts/ci-status.sh <PR_NUMBER>
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

After pushing the fix commit(s):

```
bash .agents/skills/pr-reviews/scripts/trigger-copilot.sh <PR_NUMBER>
bash .agents/skills/pr-reviews/scripts/trigger-cubic.sh   <PR_NUMBER>
```

Copilot re-runs when re-requested as a reviewer. cubic re-reviews when
mentioned in a new top-level PR comment.

### 6. Wait for new activity

```
bash .agents/skills/pr-reviews/scripts/wait-for-activity.sh <PR_NUMBER>
```

Default timeout 30 min, poll every 30 s. Returns 0 on new activity, 124 on
timeout. Both bots typically post a "no new findings" comment when they
have nothing left, so the loop ends naturally on a clean PR.

What counts as "new activity":
- New issue comment / review comment / review on the PR.
- New commit pushed to the PR head.
- A review thread getting resolved or unresolved (often by a bot saying
  "addressed; resolving" -- without this signal we'd miss thread state
  flips and time out spuriously).

### 7. Loop

Go back to step 1. Continue until ALL of these are true:

- `fetch-all.sh` reports all review threads resolved.
- `fetch-sonar-findings.sh` reports zero open issues / hotspots that
  this PR introduced (or the remaining ones are explicitly marked FP /
  WontFix).
- The AI bots have posted a "no new findings" or equivalent comment
  after their most recent re-trigger.
- `ci-status.sh` reports no failures caused by this PR (failures
  unrelated to the PR are noted, surfaced to the user, but not fixed).

When `wait-for-activity.sh` times out (30 min):
- Re-run `ci-status.sh`. If checks are still running, surface to the
  user and stop -- the PR is in a clean intermediate state.
- If there are CI failures attributable to the PR, treat them as a new
  finding and iterate (commit -> ci-check -> push -> retrigger -> wait).
- If the failures are unrelated, note them in the final report.

### 8. Final report

When the loop ends, summarize for the user:
- Findings addressed (count by source: review threads, Sonar, CI).
- Any unrelated CI failures observed but not fixed (with check name + URL).
- Any human comments that need their attention.
- Current PR state (mergeable / blocked, decision, head SHA).

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
| `copilot[bot]`               | Line-level code review                     | Re-add as requested reviewer (`gh pr edit`)      |
| `sonarqubecloud[bot]`        | Quality-gate status                        | Auto, on each scan run -- read its issue comment |
| `github-actions[bot]`        | CI status / labels                         | Auto, on each workflow run                        |
| `netdata-bot[bot]`           | Repo automation (labels, etc.)             | Auto                                              |

If a new AI reviewer appears in the project, classify it by adding to
`PR_AI_BOT_RE` in `_lib.sh` so the skill recognizes it.

## Failure modes -- quick diagnosis

| Symptom                                                | Likely cause                                                         |
|--------------------------------------------------------|----------------------------------------------------------------------|
| `fetch-all.sh` returns suspiciously round counts       | Pagination missed pages. Re-run; fetch-all auto-probes when count is a multiple of 100. |
| `reply-thread.sh` -> 404                               | Wrong comment id (use `databaseId` from `review-threads.json`, not the GraphQL node id). |
| `resolve-thread.sh` -> "thread not found"              | Used REST id instead of GraphQL node id.                              |
| `trigger-copilot.sh` succeeds but no new review        | Reviewer was already requested -- script removes-then-adds to force a fresh run. If still nothing, copilot may be quota-limited; wait. |
| `trigger-cubic.sh` succeeds but no new review          | cubic ignores comments without an explicit `@cubic-dev-ai` mention. The script always prepends it. |
| `ci-status.sh` exits 2 (running)                       | CI hasn't finished. Push anyway -- waiting on CI between iterations destroys throughput. The next push triggers a fresh CI run on the new code, which is what matters. (See Step 4b.) |
| Bot keeps re-flagging the same line after a fix push   | The bot didn't see the new commit because it wasn't re-triggered.    |
| `wait-for-activity.sh` 124 timeout                     | Bots are silent -- could be done, could be quota-limited. Check `summary.txt`; if all threads resolved, you're done. |

## MANDATORY -- keep this skill alive

If you (the agent) discover a new pattern, gotcha, working flow, correction,
or any piece of knowledge while running this skill -- update this `SKILL.md`
AND commit it BEFORE proceeding. Knowledge that isn't committed is lost.

Examples of things to capture:
- A new AI reviewer bot that appears in the project (add to the directory + `PR_AI_BOT_RE`)
- A new common false-positive pattern that warrants a clarifying source comment
- A new GitHub API quirk (rate limits, undocumented response shapes, pagination edge cases)
- A retrigger mechanism that changed (e.g. copilot's re-request behavior)
