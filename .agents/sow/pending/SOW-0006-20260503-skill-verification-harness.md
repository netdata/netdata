# SOW-0006 - Skill verification harness

## Status

Status: open

Sub-state: stub. Created 2026-05-03 evening when the user split the verification piece out of SOW-0010 ("the evaluation does not need to be done now"). Depends on SOW-0010 closing first because the harness consumes the seed question lists and how-tos that SOW-0010 lays down.

## Requirements

### Purpose

Build a reusable **skill verification harness** that grades
whether an AI assistant can answer concrete questions about a
running Netdata environment using ONLY the skill's
SKILL.md + per-domain guides + how-tos. The harness is the
acceptance gate for any querying skill in this repo.

The user's framing (verbatim from the 2026-05-03 expansion
message): "use sonnet (not the best model) to query the APIs to
find out if the instructions work. Sonnet should be responding
what it did to find the answer and we should maintain a directory
with how-tos and an index in SKILL.md to help the model figure
out its way through it."

The harness validates the two SOW-0010 skills first, then is
re-used by SOW-0003 (`query-agent-events`), SOW-0004
(`learn-site-structure`), SOW-0005 (`mirror-netdata-repos`), and
SOW-0007 (`integrations-lifecycle`) as their own acceptance gates.

### User Request

> "We need to build a verification system: spawn an agent and
> ask it a question:
> - Node `costa-desktop` (on netdata-cloud in space Netdata People)
> - find its hardware specs
> - which operating system it runs?
> - is it a parent? of how many and which nodes?
> - is it a child and where it stream?
> - does it have any vnodes and which?
> - are there any failed data collection jobs?
> - does it monitor nvidia DCGM? What is the data collection
>   frequency?
> - which is the PID with the biggest memory consumption and in
>   which app group/category I can see in on the dashboard?
> - find the last netdata status file log in the logs
> - and many more
>
> The idea is to use sonnet (not the best model) to query the
> APIs to find out if the instructions work. Sonnet should be
> responding what it did to find the answer and we should
> maintain a directory with how-tos and an index in SKILL.md to
> help the model figure out its way through it.
>
> The list of how-tos should be supposed to be live. Every time
> the assistant is asked to do something and it is not documented
> and it is forced to do an analysis to answer, it should create
> a how-to to help future assistants work faster.
>
> The agent skill and documents and scripts should ensure the
> assistant never sees the cloud api token and agent bearer
> token."
>
> Follow-up: "The evaluation does not need to be done now. But
> the live how-tos inventory and everything else must be done."
> -- so the inventory + token-safety + structural scope stay in
> SOW-0010; the harness itself (this SOW) is deferred.

### Acceptance Criteria

- A `verify/` runner under each verified skill -- e.g.
  `<repo>/docs/netdata-ai/skills/query-netdata-cloud/verify/run.sh`
  -- that:
  1. Reads `<skill>/verify/questions.md` (the seed list shipped
     by the upstream SOW).
  2. For each question, spawns a Sonnet-class assistant with a
     minimal system prompt that points at the skill (SKILL.md +
     `how-tos/INDEX.md` + canonical reference docs).
  3. Captures the assistant's full transcript, the tool calls
     it made, and the final answer.
  4. Records results under
     `<repo>/.local/audits/<skill>/verify/<timestamp>/`.
  5. Grades each answer against `verify/grader.md`.
  6. Reports pass / fail / unanswered counts.
- `verify/grader.md` per skill: rubric covering (a) correctness,
  (b) evidence shown (file:line refs, response keys), (c) no
  exposed tokens / bearers / claim ids in the transcript or
  output, (d) how-to authored when missing.
- An "unanswered question" prompt loop: when Sonnet cannot
  answer, the runner asks (interactively, or via a follow-up
  agent run) for a draft how-to and stages it under
  `<skill>/how-tos/<slug>.md` for human review.
- A self-test mode: the runner can be pointed at any of the
  four SOW-0010+ skills and produces consistent reports. This
  is what makes it reusable across SOWs.
- Sensitive-data gate: every committed file passes the
  pre-commit grep from
  `<repo>/.agents/sow/specs/sensitive-data-discipline.md`. The
  harness MUST scrub captured transcripts of any token / bearer
  / claim_id before persisting them under `.local/audits/`.

## Analysis

Sources to consult during stage 2:

- `<repo>/.agents/sow/done/SOW-0010-20260503-netdata-query-skills-infrastructure.md`
  -- the SKILL.md and how-tos format this SOW must validate.
- the user-global Claude Code config and `claude` CLI invocation patterns (the user
  has documented them in `the user-global Claude Code config (gitignored)` under "Run other
  AI assistants for a second opinion").
- The pattern of the legacy private skills (`coverity-audit`,
  `pr-reviews`) for their `_lib.sh` shape and `_run` wrappers.

Risks:

- Token leakage in transcripts is the highest risk. The harness
  must scrub captured stdout/stderr before writing.
- Cost: running Sonnet over many questions has real cost. The
  harness should default to a small seed-question subset and
  let the operator opt into the full run.
- Flakiness: live API answers are time-dependent (e.g.
  "currently active alerts" varies). The grader rubric must
  accept structural correctness + presence of evidence rather
  than exact byte equality.

## Pre-Implementation Gate

Status: blocked-on-prereq

Depends on SOW-0010 closing (it provides the SKILL.md, the
per-domain guides, the `how-tos/INDEX.md`, the
`verify/questions.md` seed list, and the token-safe wrappers
that the harness must invoke).

Sensitive data handling plan:

- Follows `<repo>/.agents/sow/specs/sensitive-data-discipline.md`.
- Captured transcripts and response bodies live under
  `<repo>/.local/audits/<skill>/verify/<timestamp>/` (gitignored).
- Before persisting any transcript, the harness scrubs anything
  matching the discipline grep (UUIDs, IPv4 literals to specific
  hosts, bearer/cloud-token shaped strings, forbidden absolute
  paths). The scrubbed transcript is what gets stored; the
  unscrubbed in-memory copy is dropped.
- No `.env` keys are required beyond what SOW-0010 already
  defines.

Holding-pattern decisions to record now:

- The harness invokes `claude -p` (Sonnet) per the user's
  documented pattern in `the user-global Claude Code config (gitignored)`. Model selection
  is a runner flag with a default; not a per-question
  hardcode.
- The how-to generation prompt is a separate template under
  `verify/howto-template.md`. Stage 2 implementation defines
  it.

## Implications And Decisions

No new decisions at this stub stage. Will be added when
SOW-0010 closes and stage 2 begins.

## Plan

1. **Wait for SOW-0010 to close.**
2. Stage 2a: read SOW-0010 final deliverables (SKILL.md
   structure, how-tos shape, wrappers).
3. Stage 2b: implement `verify/run.sh` for the cloud skill;
   prove end-to-end on the seed questions.
4. Stage 2c: parameterize `run.sh` so it can target any of the
   SOW-0010+ skills.
5. Stage 2d: implement the how-to generation prompt loop.
6. Stage 2e: implement the transcript scrubber + unit test
   that asserts no token bytes are persisted.
7. Stage 2f: validate end-to-end. Close.

## Execution Log

### 2026-05-03

- Created as a stub during the second SOW-0010 scope expansion.

## Validation

Pending. All sub-fields will be filled at close.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
