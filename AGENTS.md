# AGENTS.md

## Goals

This repository is the Netdata Agent codebase: a large, multi-language, multi-platform monolith serving production
monitoring, troubleshooting, data collection, alerting, storage, streaming, cloud integration, packaging, and
documentation workflows.

Work here MUST prioritize root-cause understanding, correctness, performance, maintainability, portability,
security, and consistency with existing project conventions.

Critical rules:

1. You MUST find the root cause of a problem before offering a solution. Patching without understanding the
   problem is not allowed.
2. Before patching code, you MUST understand the codebase and the implications of the change: what else is
   affected, what else uses this code.
3. Do not duplicate code. Check whether similar code already exists and reuse it.

## Requirement Language

This repository uses RFC-style requirement language. Durable AI-facing artifacts (instruction files, specs, skills)
SHOULD use these words when documenting enforceable rules:

- **MUST** / **REQUIRED**: mandatory. Violating work is not acceptable unless the user explicitly changes the
  requirement.
- **MUST NOT**: prohibited.
- **SHOULD** / **RECOMMENDED**: expected default. Deviate only with evidence and explain the trade-off.
- **MAY** / **OPTIONAL**: allowed, not required.

## Working With The User

Roles:

- **User**: purpose, scope decisions, design forks, risk acceptance, destructive approvals, final product judgment.
- **Assistant**: investigation, evidence, implementation, tests or equivalent validation, reviews, documentation,
  memory updates, concise reporting. The assistant proposes and investigates; the user approves.

Communication:

- Do your homework before asking questions or requesting decisions. Check every related aspect and possibility so
  questions are informed and to the point.
- Never write walls of text unless asked. Be simple, direct, lean, ordered by importance: give the full picture
  first, start from the high level, let the user ask for details.
- Never agree with the user when the facts contradict their understanding. You MUST always describe the risks and
  implications of their decisions clearly. You are helpful when you reveal the truth accurately, not when you agree.
- Ask the user only for irreducible product, design, or risk decisions.

When a user decision is needed:

1. Present concrete evidence: files and lines, or source references.
2. Provide numbered options.
3. Explain pros, cons, implications, and risks of each.
4. Recommend one option with reasoning.
5. Record the decision in the SOW before implementation. For the goal/plan approval round, the bar is the
   Approval bar (Development Principles, Definitions).

## Development Principles

These principles are mandatory for every task. Code is cheap to add and expensive to live with: a larger diff that
removes debt beats a smaller one that preserves it.

### Definitions

- **Work categories**: defined under "When A SOW Is Required". Trivial changes and read-only work are exempt from
  the implementation record-the-target, disclosure, and reference-search rules below; the clean-end-state preference
  still applies to changes. Read-only work may use local tracking when the user requests it.
- **Approved scope**: the union of (a) the issue or user request, (b) the SOW Purpose and Acceptance Criteria as
  recorded in the SOW, and (c) the migration/contract surface they imply.
- **Coupled item**: code, config, docs, or tests the current change makes redundant or leaves inconsistent (a
  replaced path, its callers, its tests).
- **Independent work**: new work is genuinely independent only if ALL hold: (a) the approved clean end state is
  complete and correct without it, (b) it is not a coupled item or a remaining reference recorded under the target,
  (c) it has its own separable acceptance criteria.
- **User-owned fork**: competing designs, a public-contract change, a destructive or irreversible step, or unclear
  scope.
- **Approval bar** (used by every gate): the user explicitly accepts a trade-off, goal, or plan that you stated in
  your own words (what stays redundant or partial, and why). A bare "ok" or "sounds good" to a one-sided pitch is
  not approval.
- **Mandatory pause**: you MUST present the evidence, STOP, and obtain explicit user approval (Approval bar) before
  proceeding, before requesting non-draft review, and before marking the work complete. It is triggered whenever the
  delivered state would fall short of the recorded target for any reason other than approved staged delivery, and by
  any user-owned fork.
- **Default on doubt**: investigate uncertainty before classifying work. Suspected low-risk coupling follows the
  explicit include-and-disclose rule under "Clean End State Over Less Churn". Ask when a material scope, contract,
  or risk fork remains unresolved. If an intended change's risk remains unclear, treat the change as non-trivial;
  read-only investigation follows "When A SOW Is Required".

### Clean End State Over Less Churn

- You MUST recommend and deliver the clean end state: the structure the codebase SHOULD have once the approved
  scope is fully delivered, including removing the code, config, docs, and tests the change makes redundant. Not
  the smallest diff. You MUST NOT relabel the smallest working diff as "the clean end state".
- Record the target: before generating options, record the clean end state in the SOW. For staged work each stage's
  SOW records its own target, and the stages together MUST reach the full target. Any option that does not match
  the recorded target is a non-clean state and triggers the Mandatory pause.
- Open design decision: when the clean end state is itself the user's design decision, do not invent a fixed
  target. Record a provisional target plus the open question and resolve it with the user first.
- The recorded target MUST account for affected items:
  - (i) removed or migrated: redundant code, config, docs, tests, and their replacement when applicable;
  - (ii) intentionally retained: items and references that remain valid in the delivered state, with their rationale;
  - (iii) deferred or excluded: every coupled item omitted from the delivered state, with its reason, scope source,
    and required user approval and follow-up tracking.
  Excluding an in-scope or coupled item without recording it is silent scope-narrowing and is prohibited. Retention
  under (ii) MUST NOT disguise unfinished migration work; such work belongs under (iii) and the Mandatory pause.
- You are not the scope authority:
  - Coupled cleanup is in scope. You MUST NOT reclassify in-scope or coupled work as "independent" or "out of
    scope" to avoid it, and you MUST NOT silently drop it.
  - When you only suspect coupling and including it is low-risk and confined to what you are changing, include it
    and disclose it rather than stopping to ask.
  - Pause for the user only when inclusion would expand the blast radius, change a user-visible contract, or the
    boundary is itself a genuine scope fork.
- Touch the mess you touch: when code you modify already contains adjacent duplication, dead code, or a clear
  pre-existing defect, you SHOULD clean it as part of this work rather than build on it, provided the cleanup is
  low-risk and confined to the code you are already modifying. Cleanup reaching into unrelated code is independent
  work (see "Scope Discipline At Every Step"). If you leave adjacent mess in place, record why under disclosure (iii).
- Reference search (when replacing a path or altering a contract):
  - You MUST run and record in the SOW (command and result) a search for remaining references to the replaced path
    or contract. Search construction sites and prefixes too, not only literal final names; identifiers here are often
    built dynamically (for example via `fmt.Sprintf`).
  - Every relevant search hit MUST have a disposition under (i), (ii), or (iii), or the target is incomplete.
    Legitimate retained references, such as negative regression tests mentioning a retired identifier, belong under
    (ii). Record and explain unrelated search matches. An item you did not search for counts as silent scope-narrowing.
  - A repository-wide search cannot prove safety for consumers outside this repo (Netdata Cloud, exporters,
    streaming, ML, the docs pipeline). Renaming a shipped public contract is a user-owned breaking decision routed
    through the Mandatory pause, not something the search clears.
- Allowed exceptions (pause conditions, not auto-routes). Recommend a non-clean route ONLY for:
  - (a) technically impossible: impossible to implement correctly at all, NOT within a preferred diff size;
  - (b) a concrete, evidenced safety risk: a named hazard such as data loss or a security/production-stability
    regression, NOT "a larger diff is riskier";
  - (c) confirmed by the user as outside the approved scope;
  - (d) accepted by the user, through the Mandatory pause, as an in-scope partial to ship now.
- Exception evidence: for (a)/(b) you MUST cite specific evidence (file/line, failure class, or test) and route
  through the Mandatory pause; you do not self-certify "unsafe". For (d) track the remainder per Followup Discipline
  with why deferral is acceptable and when it lands; repeatedly shipping partials is debt accumulation, not delivery.
- Never valid: unsupported claims of risk reduction, review convenience, smaller diff, and issue staging do not
  justify a non-clean route and MUST NOT be relabeled "unsafe" or "independent". The evidenced-hazard exception
  above remains available with the required evidence and user approval.
- Re-evaluation checkpoints: at the completion of each planned step, before opening or updating a PR, and before
  marking a SOW completed, you MUST re-evaluate the written changes against the recorded target AND check for drift
  outside the approved scope. You SHOULD also re-evaluate whenever you pause to report progress. Do not keep a
  compromise only because it already exists in the branch.
- Staged delivery (mechanics: "Umbrella And Step SOWs"):
  - Allowed ONLY when every stage is an in-scope decomposition of one approved clean end state and the stages
    together reach it.
  - The approval recorded for the staged plan covers the intermediate states, so an approved stage does not
    re-trigger the pause.
  - Every later stage MUST be tracked per Followup Discipline before an earlier stage merges; "a later stage will
    finish it" with no tracked item is not acceptable.
- Re-ground staged designs: a design recorded during planning or an earlier stage MAY have drifted from the code a
  previous stage produced. Before implementing a later stage you MUST re-verify its design against the current
  code and record the correction in the SOW. Never implement against stale assumptions.
- Deferral check: before recommending deferral, check the issue, SOW, acceptance criteria, and affected migration
  scope. Silence or ambiguity MUST NOT be read as permission to defer. If those sources do not clearly place the
  work outside the target, treat it as in-scope and either complete it or pause for a user decision.

### Plan Before Non-Trivial Work

- Non-trivial work MUST start with a plan recorded in the SOW before any implementation-file change and before any
  implementation-equivalent action: migrations, deletions, pushes, non-draft PRs, external-state mutations via tools.
- Human-owned goal: the desired end state MUST be created with or approved by the user. You MUST NOT finalize the
  goal unilaterally.
- End state first: you MUST establish the desired end state, including its acceptance criteria, before planning
  the steps. The goal drives the work, not a first diff. If you cannot state the end state, keep investigating;
  do not start against an unknown target. A user-owned design decision follows the open-design-decision rule under
  "Clean End State Over Less Churn".
- Decompose: split the work into ordered steps, each with its own clean end state and acceptance criteria, each
  building on the previous one. A single coherent step is valid when the work is atomic; do not invent artificial
  sub-steps.
- Large or vague deliverables: keep refining the plan until every step has a clean end state and acceptance
  criteria. Do not start implementation while steps are still unclear.
- Reachability: the plan MUST reach the desired end state through its steps or produce evidence that it is not
  achievable. An unachievable goal is a pause condition for a user decision, not a silent partial result.
- Approval is for goal-decisions, not work categories:
  - The goal-approval round fires ONLY when the end state is a user-owned fork. Then the whole plan (end state plus
    step breakdown) MUST be explicitly approved by the user (Approval bar) before implementation: state what is
    being accepted and get confirmation. If the user rejects or edits the plan, revise and re-seek approval. The
    SOW stays `planning` until approval is recorded, then becomes `ready`.
  - Fixed end state: non-trivial work whose end state is already fixed by the request, an existing project skill,
    or an established repository pattern (a clear bug fix, a metadata/docs edit with no contract change, a collector
    skeleton and wiring fixed by its authoring skill) still needs a recorded plan and the Pre-Implementation Gate,
    but the request IS the recorded approval.
    - No separate approval round is held, and the same approval satisfies the resume re-check below and the
      progress rule ("SOW Lifecycle").
    - A collector's Function surface, vnode/host-scope design, and new public config options remain user-owned
      forks.
- Approval persists; re-check on resume: before continuing an `in-progress` or `paused` SOW you did not personally
  take through this gate (takeover, handoff), you MUST confirm the SOW records explicit approval of the current
  goal and plan. If it does not, or the plan changed materially since, treat the SOW as `planning` and re-obtain
  approval.

### Scope Discipline At Every Step

- Documentation capture during authorized implementation is part of that task under Knowledge Capture, even when
  the discovered gap is not required to complete the code change. This does not authorize additional implementation
  or changes to product behavior or contracts.
- New work that fails the Independent work definition, or that you are unsure about, is coupled: handle it under
  "Clean End State Over Less Churn" (do the low-risk part and disclose it; pause only for a genuine fork).
- Genuinely independent work: do NOT silently bundle it. Submit it as a separately authorized PR first and rebase
  after it merges, or track it per Followup Discipline. Do NOT fold it into this SOW's steps.

## SOW System

Project SOW status: initialized

The line above is the audit marker; do not edit it. This project uses a local Statement of Work (SOW) system. It is
self-contained: normal SOW work MUST NOT depend on `~/.agents`, `~/.AGENTS.md`, global skills, global templates, or
global scripts. Use this `AGENTS.md`, the local SOW, project-local specs, and project-local skills.

### Storage Model

SOWs and specs are local-only working memory, never committed:

- SOW working files live under `.agents/sow/q/**`, specs under `.agents/sow/specs/**`. Both are gitignored
  (`/.agents/sow/q`, `/.agents/sow/specs`) and MUST NOT be committed to any branch. Specs may be re-introduced to
  git later, reorganized, as a deliberate decision; until then treat them as local memory.
- Committed framework files: `.agents/sow/SOW.template.md`, `.agents/sow/audit.sh`, `.agents/sow/scan-sensitive.sh`,
  `.agents/sow/worktree-link.sh`.
- Durable knowledge needed for the approved deliverable belongs in project skills, docs, code, and tests (and specs
  once re-introduced), not solely in the SOW body. Other discoveries follow Knowledge Capture.
- Queues under `.agents/sow/q/`. Move the file as its state changes; a queue move is normal lifecycle, not deletion.

  | Queue | Meaning | Status values inside |
  |---|---|---|
  | `pending/` | not started: no gate, no branch | `planning` stubs |
  | `current/` | in flight; a paused SOW stays here | `planning`, `ready`, `in-progress`, `paused` |
  | `done/` | finished | `completed` |

- Legacy `active/`: retired. Fold any `.agents/sow/q/active/` files into `current/` (or `done/` if completed);
  nothing reads `active/` any more, and `worktree-link.sh` does this fold whenever it runs.
- Deletion guard: assistants MUST NOT remove a SOW working file from the local checkout (`rm`, patch delete hunks,
  editor deletes, or any equivalent) unless the user explicitly asks to discard it. A completed SOW MAY stay in
  `done/` as local history or be deleted at the user's request. Never delete one without the user asking.
- Worktrees and setup: SOW working memory is per-developer, not per-worktree. Run `.agents/sow/worktree-link.sh`:
  - once on a fresh clone (it creates the queues), after creating a git worktree, and after updating an old checkout
    to this model;
  - in the origin checkout it creates the queues, folds legacy top-level queue dirs into `q/`, and folds a leftover
    `q/active/` into `current/`;
  - in a linked worktree it first moves worktree-local `q/`, `specs/`, and `.local/` content into the origin, then
    symlinks `.agents/sow/q`, `.agents/sow/specs`, `.local`, and `.env` to the origin checkout (a worktree that
    already has its own real `.env` keeps it);
  - it is idempotent, never loses data on a name collision, re-points a symlink whose origin moved, and refuses to run
    in a worktree whose origin checkout is not yet on this model (it prints how to update it).
- Legitimate local SOW files require no commit-for-handoff or remove-before-merge step. The CI guard described under
  Enforcement rejects working files that were accidentally committed; it does not require deleting local memory.

### When A SOW Is Required

Read-only investigation, explanation, and review require no implementation SOW or branch unless the user requests
tracking. They MAY save local evidence and sanitized notes under "Knowledge Capture". They MUST NOT change source,
tracked documentation, Git state, or remote state without separate authorization. When changes are authorized,
classify those changes below before implementation; a request for review alone does not authorize fixes.

Create or reuse a SOW for non-trivial changes:

- feature work; bug fixes with behavioral impact; refactors; migrations; regressions;
- documentation or content changes with product/business impact; spec hygiene; project skill changes;
- process changes; collector changes; packaging, install, or deployment changes;
- PR review iteration or static analysis triage that changes source, docs, or project policy;
- any intended change with unclear risk.

Trivial work needs no SOW: typo fixes; formatting-only changes; mechanical renames with no behavior change; simple
low-risk search/replace (still grep the old token to confirm no call site is missed).

Default on doubt applies to intended changes: unresolved risk means non-trivial, not that read-only analysis itself
requires an implementation gate.

### Required First Checks

Before non-trivial work:

1. Read the in-flight SOWs under `.agents/sow/q/current/` and `pending/`. Discover other in-flight work through
   open PRs and issues, not through `master`.
2. Read relevant specs under `.agents/sow/specs/`.
3. Select and load applicable runtime skills using "Skill Selection" below (index under Project Skills).
4. Inspect code, docs, tests, and existing project instructions as ground truth.
5. For non-trivial work the goal and plan are user-owned: see "Plan Before Non-Trivial Work".

### SOW Lifecycle

- Create new SOWs from `.agents/sow/SOW.template.md` (project-local; MAY be customized). The template is the schema
  of a SOW: its `##` headings carry `<!-- sow:... -->` tags saying which SOW kinds require each section (explained at
  the top of the template), and `audit.sh` derives its structural check from them, so adding or renaming a section
  needs no audit change. Two field labels are additionally pinned in `audit.sh` (`Sensitive data handling plan:`,
  `Sensitive data gate:`); renaming either in the template means updating `audit.sh` in the same change. The audit
  checks presence only; every required section MUST still be filled. Create the file in
  `pending/` when the work is queued but not started, or directly in `current/` when starting now; move it from
  `pending/` to `current/` when you begin filling the Pre-Implementation Gate.
- Filename: `SOW-YYYYMMDD-{slug}.md`, creation date plus descriptive slug. There is no sequential counter because it
  cannot be allocated safely across parallel branches.
- State lives in the file's `Status:` field:
  - `planning`: analysis or decisions incomplete; implementation blocked.
  - `ready`: Pre-Implementation Gate complete and, where the goal-approval round applies, goal and plan approved;
    implementation can start.
  - `in-progress`: implementation underway. Set it with the first implementation-file change.
  - `paused`: intentionally stopped; may resume on the branch.
  - `completed`: validated and durable memory transferred. The successful terminal status.
- Content hygiene: an active SOW is a current-state handoff, not an append-only transcript.
  - When a plan, assumption, or decision is superseded, replace it with the current truth. Keep prior history only
    when it explains a current constraint, approval, or rejected alternative.
  - Preserve user approvals, durable evidence, and material checkpoints; consolidate repeated review rounds and
    remove duplicated analysis.
  - The execution log SHOULD record state transitions, deviations, and validation results. It SHOULD NOT reproduce
    the conversation or every review nit.
  - Before completion, prune stale history and verify another contributor can determine the current target,
    remaining work, decisions, and evidence without reconstructing chronology.
- One SOW at a time: several SOWs MAY be in flight in `current/`, but a branch executes one SOW at a time and never
  executes multiple SOWs as one batch. If work overlaps, coordinate through the open PRs and issues, merge or
  consolidate branches before implementation, or split into separate SOWs and complete one before starting the next.
- Progress reports and Re-evaluation checkpoints are not stop points. Once a SOW is in progress with its approval
  recorded, continue until it is delivered, failed with evidence, blocked on a real user decision or approval, or
  superseded by newer user instructions.
- Completion of a standalone or step SOW, when the authorized deliverable is complete and any changes are ready to
  merge (umbrellas: see "Umbrella And Step SOWs"):
  1. Finish authorized implementation, documentation capture under Knowledge Capture, validation, and follow-up mapping.
  2. Transfer durable knowledge needed for the approved deliverable into project skills, docs, code, and tests (and
     specs once re-introduced). Document reusable discoveries from implementation under Knowledge Capture. Discoveries
     from tracked answer-only work MAY remain sanitized local notes without authorizing guide edits. The SOW MUST NOT
     be the sole record of a delivered project contract.
  3. Set `Status: completed` and move the file to `.agents/sow/q/done/` (unless the user asks to discard it).

### Umbrella And Step SOWs

Staged delivery uses one umbrella SOW plus one SOW per step (each step a mergeable deliverable).

- Naming: umbrella `SOW-YYYYMMDD-<family>-umbrella.md`; steps `SOW-YYYYMMDD-<family>-NN-<step>.md` with a two-digit
  ordinal. All files of one initiative share the `<family>` slug; each keeps its own creation date.
- Links by filename, never by queue path (paths go stale when files move). Each step records
  `Umbrella: SOW-YYYYMMDD-<family>-umbrella` in Requirements. The umbrella keeps a `## Steps` table: ordinal, step
  SOW filename, status, PR.
- Content split: the umbrella's Pre-Implementation Gate holds the full clean end state, the decomposition (as its
  implementation plan), and the open decisions; its `## Implications And Decisions` holds every user decision; its `##
  Steps` section holds the step table and the cross-step follow-up mapping. An umbrella does not need the Workflow
  Friction, Validation, or Artifact Maintenance Gate sections (the schema does not require them for umbrellas): its
  gate's `Validation plan:` and `Artifact impact plan:` describe what the steps will do, and friction is recorded in the
  step SOWs. A step holds its own gate and Validation and cites umbrella decisions by number. A step whose end state is
  fixed by the approved decomposition needs no new approval round.
- Placement and status: the umbrella moves to `current/` when you begin filling its gate and stays there until the last
  step completes. Its status follows the normal ladder: `planning` until the decomposition is approved (its gate
  `ready`), `ready` once approved with no step in flight yet, `in-progress` while any step is in flight, and `completed`
  once the last step is complete and the cross-step follow-up mapping in `## Steps` is resolved; then move it to
  `.agents/sow/q/done/`. `paused` on an umbrella means the initiative itself is parked, not "waiting on steps". Steps
  follow the normal queue rules; the assistant updates the umbrella's `## Steps` table whenever a step changes status or
  gains a PR.
- One SOW at a time applies to steps. The umbrella is never executed and does not count.

### Pre-Implementation Gate

For non-trivial work, implementation MUST NOT begin until the SOW's `## Pre-Implementation Gate` section records
`Gate status: ready` and the SOW's top-level `Status:` is `ready` or `in-progress`.
The `Gate status:` line takes only `blocked` (gate incomplete),
`ready`, or `needs-user-decision` (blocked on a user-owned fork); it is a different key from the SOW `Status:` so the
two are never confused. Before changing implementation files, or before continuing an existing SOW that lacks the
section, fill the gate. Gate `ready` additionally requires the goal/plan approval of "Plan Before Non-Trivial Work"
where that round applies. Trivial changes and read-only work retain the exemptions under "When A SOW Is Required".

The gate MUST fill every field of the template's `## Pre-Implementation Gate` section (the template is the SOW schema,
see "SOW Lifecycle"; field semantics live in its placeholders). Placeholders such as `TBD`, `N/A`, or "to be checked
later" are invalid unless the SOW explains why the item truly does not apply. If the gate exposes an unknown that
investigation cannot resolve, stop and ask the user before implementation.

### Git Worktrees

Assistants MUST NOT create git worktrees on their own. Create one only when the user explicitly asks for it or
approves it, then run `.agents/sow/worktree-link.sh` (see Storage Model).

### Git And PR Workflow

- Work on a branch per SOW created off local `master`; never commit to `master` directly.
- Stage only the specific files you changed. Never `git add -A`: the working copy holds untracked files that MUST
  NOT be committed.
- Never `git checkout <file>`, `git reset`, delete files, or rewrite history without explicit user approval. Undo a
  change by editing it out, not by checking the file out.
- Commit and push only when the user asks or explicitly approves those operations in the plan. Approval to implement
  a fixed goal is not implicit approval of Git operations subsequently added to the plan. Checkpoint commits and
  squashing under "Review" remain subject to these authorization rules.
- Commit messages and PR bodies describe the change. A PR body links the follow-up issues tracked from its SOW.

### Local SOW Parking

- Users MAY keep private paused, abandoned, or not-yet-public SOW drafts under `<repo-root>/.local/sow/`. It is
  gitignored and outside the project SOW lifecycle. Use it to preserve work locally without creating a public or
  team-visible GitHub issue yet.
- Parked SOWs are private memory only: not durable project memory, not visible to other contributors, and not
  acceptable as the only tracking for work that must coordinate a team, block a merge, or survive across machines.
- Eligibility for private deferred-work tracking and the required user acceptance are defined under Followup
  Discipline. Local discovery notes follow Knowledge Capture and are not approval to defer in-scope work.
- Active implementation work still MUST use the `.agents/sow/q/` queues.

### Review

Review findings are leads until verified against the shipped code and its contracts.

- A review request alone is read-only, as defined under "When A SOW Is Required". The fix requirements below apply
  only when implementation is authorized.
- A shipping blocker MUST identify a production-reachable trigger, the violated contract or invariant, the
  concrete consequence, and supporting code or test evidence.
- Failing required validation or an unmet explicit acceptance criterion is also a shipping blocker, regardless of
  the reviewer's severity label.
- An unreachable defensive scenario, optional refactor, style preference, or speculative future risk MUST NOT be
  promoted to a blocker. Reject it with evidence, or track it separately when it has independent value.
- Optional test expansion, documentation polish, and maintainability suggestions without a concrete current defect
  MUST NOT extend the review cycle by themselves.
- Reproduce a reviewer-reported bug as a FAILING automated regression test before fixing it when feasible. If
  automation is impractical, record why and use concrete observable evidence, such as an executable reproducer,
  build failure, hardware observation, or incorrect documented behavior. After the fix, rerun the same test or
  reproducer and confirm the failure no longer occurs. Record the passing result or non-reproduction evidence;
  if verification is unavailable or incomplete, record the gap instead of claiming success. Speculation alone is
  not reproduction.
- Multi-round review: when Git operations are authorized under "Git And PR Workflow", checkpoint-commit each
  validated change (specific files only) before its review and squash at PR time only if history rewriting is
  explicitly approved. Otherwise review the working-tree diff, preserve a record of the reviewed state, and report
  that no checkpoint commit or squash was performed. A review requirement never grants Git authorization.
- Recurrence: if findings keep clustering in one subsystem for ~2-3 rounds, stop patching individual cases, name the
  missing invariant, and propose one class-level fix as a user decision.
- Obtaining a review: the coordinating assistant spawns independent, full-scope reviewers with clean context, or runs
  the external assistants the user names. Delegated reviewers MUST NOT launch other agents; include this restriction
  in each review assignment. For performance-sensitive code at least one reviewer MUST carry an explicit hot-path
  performance lens. Record each reviewer and its findings under Validation in the SOW when one exists; otherwise
  report the findings and review scope directly to the user.
- One complete review round is the default. Repeat the same full scope only when a verified shipping blocker
  required a material change to shipped implementation or behavior, or when the prior review could not assess the
  complete change.
- Stop when no verified shipping blocker remains. Reviewer unanimity, exact readiness phrases, and zero optional
  suggestions are NOT required. Nits alone MUST NOT keep a review cycle open.

### Followup Discipline

"Deferred" is not a terminal outcome. Before a SOW can close, every valid deferred item MUST be implemented,
explicitly rejected as not worth doing with evidence, or tracked as follows:

- Use a GitHub issue linked from the SOW or PR unless the private-tracking exception below applies.
- In-scope remainders, merge dependencies, and work needed for team coordination MUST have a GitHub issue linked
  from the SOW or PR. Tracking does not replace the user approval required to deliver a partial clean end state.
- Genuinely independent personal/local work MAY use a parked SOW under `<repo-root>/.local/sow/` only when the user
  explicitly accepts private tracking and the item does not block merge, require team coordination, or need to
  survive across machines. Record that acceptance, the reason, and the path in the active SOW.
- A local discovery note from Knowledge Capture preserves an observation; it is not an accepted implementation
  commitment or permission to defer a coupled item. Triage it under these rules if it becomes a deferred work item.

Pre-close, search the SOW for `defer|later|follow-up|future|TODO|pending` and map every hit to implemented,
rejected, or tracked.

### Regressions

A regression is broken behavior discovered after a SOW's work merged, where the original claimed outcome is no
longer true. Completed SOWs are not on `master`, so a regression is new work:

1. Open a new local SOW under `.agents/sow/q/current/`.
2. In `## Requirements`, link the prior work: `Regresses: PR #NNNNN`, plus any known commit, spec, issue, or test
   evidence.
3. Run the normal Pre-Implementation Gate and Validation.
4. Update the relevant spec, skill, doc, code, or test so durable memory reflects current reality.

Do not resurrect or mutate a prior SOW.

### Validation Gate

A SOW cannot be completed until every field of the template sections required for its kind holds evidence (tag
legend at the top of the template): for a standalone or step SOW, `## Validation` and `## Artifact Maintenance Gate`;
for an umbrella, every step completed and the cross-step follow-up mapping in `## Steps` resolved. The template is the
authoritative list of what Validation records; the semantics of each field live in its placeholder. Generic "N/A" is
invalid.

### Artifact Maintenance Gate

Every standalone or step SOW close MUST record, per durable artifact class, what was updated or the evidence-backed
reason no update was needed (an umbrella records nothing here: its gate's `Artifact impact plan:` covers the initiative
and each step's Artifact Maintenance Gate records the actual updates):

- `AGENTS.md`: workflow, responsibilities, local framework, project-wide guardrails.
- Runtime project skills: every non-symlink directory under `.agents/skills/` (`<area>-<topic>/SKILL.md` and its
  topic files) and `.agents/skills/README.md`, HOW to work here.
- Specs: `.agents/sow/specs/`, WHAT the project does.
- End-user/operator docs: README, docs site, runbooks, published guides, help text, other human-facing docs.
- End-user/operator skills: output/reference skills copied or consumed outside normal repo work.
- SOW lifecycle: local-only SOW under `.agents/sow/q/`, durable memory transferred, deferred work tracked as GitHub
  issues or explicitly accepted private tracking per Followup Discipline, regressions handled as new linked SOWs.

This is an assistant responsibility. If a SOW changes behavior, docs, specs, commands, schemas, defaults, workflows,
examples, or operating procedure, the assistant MUST update every affected artifact in the same SOW or record the
evidence-backed reason it is unaffected.

### Enforcement

- `.agents/sow/audit.sh`: local consistency audit for SOW rules, the local-only queue/spec layout, framework files,
  the skills layout, and sensitive data. `.agents/sow/scan-sensitive.sh` is the shared scanner it and CI use. Hard
  failures:
  - the marker line under "SOW System"; the exact CRITICAL sensitive-data sentence; legacy `SOW-NNNN` references;
    relocated spec paths; missing or untracked framework files; a `q/` or `specs/` path that is not gitignored;
  - a `current/` SOW with a missing or invalid `Status:`; a committed SOW or spec working file;
  - skills: no area table under `## Areas` in `.agents/skills/README.md`; a skill directory that lacks `SKILL.md`, is
    not named `<area>-<topic>` with a listed area, or whose frontmatter `name` differs from its directory; a public
    skill symlink that does not point into `docs/netdata-ai/skills/` or does not resolve; a skill directory missing
    from the `AGENTS.md` skills index, or an index entry with no directory; a `.agents/skills/` path named in a tracked
    file, or a relative `../` path in a skill file, that does not exist; an owner-section citation `path.md#anchor` in a
    skill file whose file or heading (GitHub slug) does not exist; a failed reference scan;
  - a sensitive-data hit in the committed durable artifacts it scans (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`,
    `.agents/ENV.md`, the framework files, `.agents/skills/**`, `.agents/skill-verification/**`).
  Advisory: in-flight SOW files under `q/current/` are checked for the template's required sections for their kind,
  the `Sensitive data handling plan:` label in every SOW, and `Sensitive data gate:` in non-umbrella SOWs; a missing
  section or label there is a warning. It does not scan SOW working files or specs for secrets.
- `.github/workflows/sow.yml` rejects pull requests that commit SOW working files or specs: anything tracked under
  `.agents/sow/q/**`, `.agents/sow/specs/**`, or a legacy top-level `.agents/sow/{active,pending,current,done}/`.
  A hit means a file was force-added and MUST be removed before merge. It also scans changed instruction, skill, and
  framework files, including canonical public skills under `docs/netdata-ai/skills/**`, for raw sensitive data.
- These checks are guards, not substitutes for the Validation Gate. The assistant still owns the scoped knowledge
  transfer required by SOW Lifecycle before merge.

## Durable Artifacts

### Sensitive Data In Durable Artifacts

SOWs, specs, documentation, project skills, agent instructions, and code comments are durable artifacts. Treat them
as public even when they are local-only, unless a repository-specific policy explicitly says otherwise; no scanner
covers SOWs and specs, so redaction there is your responsibility.

CRITICAL: Never write raw sensitive data to durable artifacts. This includes passwords, API keys, bearer tokens,
SNMP communities, private keys, connection strings with embedded credentials, session cookies, community member
names, customer names, customer identifiers, personal data, non-private IP addresses that can identify customers,
private endpoints, account IDs, and proprietary incident details.

Write only sanitized evidence:

- placeholders such as `[REDACTED_SECRET]`, `[CUSTOMER]`, `[ACCOUNT]`, `[PRIVATE_ENDPOINT]`;
- stable aliases such as `customer-a` only when the real mapping is not stored in the repository;
- file paths, line numbers, command names, schema fields, or error classes instead of the sensitive values;
- summarized logs and traces with minimal redacted snippets.

If sensitive data is required to continue, stop and ask the user for a secure handling path. If found in a durable
artifact, sanitize it before any commit. If already committed, tell the user and do not rewrite history without
explicit approval.

### Durable AI-Facing Artifact Formatting

Applies to `AGENTS.md`, specs, runtime project skills, public/operator skills, SOW templates, instruction bridge
files, and other docs written so future AI agents can execute repository rules correctly.

- Structure for retrieval: headings, short sections, labeled bullets, numbered procedures, so humans and agents find
  the exact rule quickly.
- No dense multi-rule paragraphs. A paragraph with several requirements, exceptions, or branches becomes bullets or
  a table.
- Tables only for matrices or comparisons with short cells. Bullets for rules, workflows, checklists, exceptions.
- Put RFC-style requirement words (`MUST`, `MUST NOT`, `SHOULD`, `MAY`) next to the action they govern. Do not hide
  mandatory behavior in explanatory prose.
- Prefer labeled bullets for operational guardrails (`Target`, `Exception handling`, `Validation`, `Failure mode`).
  A guardrail with several requirements is a labeled parent bullet with one requirement per sub-bullet.
- One durable idea per bullet. When a bullet needs several sentences, the first states the rule; the rest give
  evidence, rationale, or examples.
- Precision over brevity. Formatting is for readability, never for weakening contracts or removing evidence.
- Wrap markdown prose at ~120 columns (SHOULD), not 80. Code blocks, tables, and generated files keep their own
  formats. Keep reflow-only (whitespace) changes in separate commits from content changes.

### Open-Source Reference Evidence

When evidence in a SOW or a skill comes from another open-source repository, cite the upstream repository and the
checked commit, never the workstation path:

```text
owner/repo @ commit
relative/path/inside/repo:line
```

Resolve `owner/repo` from the repository remote, record the checked commit, keep paths relative to the upstream
root. Never write absolute paths into SOW or skill evidence.

### Specs

Specs are memory of WHAT this project does (location and local-only status: Storage Model). The source tree and public
documentation remain the primary ground truth, and specs capture durable decisions, cross-cutting rules, and area
contracts as they are worked. Durable contracts that must be shared with the team right now belong in project skills,
docs, code, and tests, not in specs.

- Layout stays flat until scale proves hierarchy is needed. Use `<domain>-<topic>.md`, one durable contract or
  cross-cutting rule per file, organized by contract ownership, not by repository path. Update
  `.agents/sow/specs/README.md` (a flat index; create it if missing) in the same change.
- Update specs when shipped work changes product behavior, public contracts, collector behavior, APIs and schemas,
  data formats, alerting semantics, packaging or deployment behavior, operational guarantees, or known edge cases.
- Specs describe current reality, not aspiration. If specs and code disagree, record the discrepancy in the active
  SOW and resolve or track it.

### Knowledge Capture

- Reusable findings supported by evidence and not already documented MUST be captured. Prefer improving an existing
  guide over creating a duplicate. Routine lookups, speculation, and one-off incident details do not require a recipe.
- For answer-only investigation or review, deliver the requested answer without starting documentation work. Record
  a reusable discovery in a sanitized local note under `<repo-root>/.local/audits/<subject>/followups.md`, using the
  existing skill audit directory when one applies. Include the finding, supporting evidence, and proposed owning
  guide. If no writable local workspace is available, include that sanitized follow-up in the response instead.
- Report reusable documentation discoveries briefly in the answer-only final response, with the proposed update.
  Obtain authorization before implementing those documentation changes; do not delay the requested answer while
  awaiting it. A local note does not replace notifying the user.
- During authorized implementation, assistants MUST update missing or outdated documentation for reusable,
  evidence-backed discoveries made while doing the work, even when the documentation is not required for the code
  change. This capture is part of the task and needs no separate authorization. Keep affected guides, skills, and
  catalogs consistent before completing the work; describe the updates in the final report.
- Documentation work arising from answer-only questions requires separate authorization. Documentation capture
  records observed behavior; it does not authorize additional implementation or new product contracts.
- Local notes are private evidence, not shared project contracts or automatic follow-up commitments. Accepted
  deferred work follows Followup Discipline. Commit, push, and publication requirements never grant authorization
  to perform those actions.
- Developer skills that give capture instructions MUST point to this section for timing and authorization.
- Public/operator skills MUST carry a self-contained operator-facing version because they can be used outside this
  checkout.
- Public/operator skill catalogs SHOULD point to their skill-local rule rather than repeat it.
- Preserve operator/developer audience boundaries.

### Skill Selection

Apply skill selection to implementation, investigation and read-only review, including work exempt from a SOW.

- Select skills by the requested operation and affected contract, using the grouped index and frontmatter
  descriptions. Read each applicable entry, then expand its routed references for the task. A matching subject word
  alone does not select every authoring, querying and triage workflow for that subject.
- For a review, load the domain contracts needed to assess the change even when an entry describes them as authoring
  guidance. Use them as review criteria; an implementation procedure is not a request to perform it. Review-only
  work retains the exemptions and action boundaries in "When A SOW Is Required".
- A reviewer lens changes evidence emphasis and reference depth, not the applicability of correctness contracts.
  Do not maintain duplicate domain rules for separate lenses. If relevance is uncertain, inspect the candidate entry
  and decide from the changed behavior and reachable dependencies; do not load every supporting file by default.
- Load required shared invariants and prerequisites before specialized guidance. Domain task routers determine which
  contracts apply; a reviewer MAY follow another reference when evidence exposes a relevant dependency. Skill text
  is a source map, not proof that a claim is current: verify affected claims at their owners.
- Loading a skill does not authorize its writes, credential setup, deployments, generation, commits or remote
  operations. For read-only review, consult existing design and validation evidence and report concrete gaps;
  do not create implementation artifacts merely because the authoring workflow requests them. Existing user
  authorization for the actual task still applies.

Review assignment scope, complete-change coverage and stop conditions remain in "Review". Provide the assigned
operation, complete review scope, approved contracts, applicable skill paths/sections and available evidence so a
specialist can select depth without guessing the task. One reviewer still assesses the complete unit and interactions.

### Project Skills

Project skills are memory of HOW to work here.

- Runtime input skills MUST live under `.agents/skills/<area>-<topic>/SKILL.md` and follow `.agents/skills/README.md`
  (areas, naming, frontmatter `name`); the public skill symlinks are exempt (Public skill convention below);
  `.agents/sow/audit.sh` enforces it. Skill Selection defines when to load them.
- Output/reference skills may also exist under product documentation or generated skill directories. Do not rename,
  shorten, or change their descriptions only to satisfy runtime discovery. Update them when their related
  public/operator workflow changes.
- Skill updates required by Knowledge Capture during authorized implementation MUST ship in the same PR. Discoveries
  from answer-only tasks follow its local-note and authorization rules.
- Every change to a skill MUST end with a slimming pass over the touched files: remove restatements, merged-in
  duplicates, and rules that now live elsewhere, keeping every rule (a removed directive is moved or superseded by a
  recorded decision, never dropped). Skills accrete bloat with each update; report line counts before and after.
- How to create, edit, slim, split, or review a skill, and the rot signals to watch for: the runtime skill
  `repo-skill-authoring`. It cites sections of this file and of `.agents/skills/README.md` by heading anchor, so a
  heading rename in either fails the audit until that skill is updated.

Public skill convention (`docs/netdata-ai/skills/`):

- Shape: `docs/netdata-ai/skills/<skill-name>/SKILL.md`, optional supporting `<topic>.md` files, optional
  `scripts/`. Frontmatter has `name` and `description`; the description is the trigger text and MUST identify
  discriminative tasks and representative user phrases. Keep exhaustive command and field mappings in references.
- Audience: operators and end-users. Public skills MAY teach querying Netdata Cloud or Agents, inspecting
  metrics/logs/topology/alerts, and running safe operational commands. They MUST NOT contain developer-contract
  validation, schema migration plans, producer authoring workflows, UI adapter work, aggregator implementation
  notes, SOW handoff instructions, fixture maintenance, PR-review tasks, or codebase-internal recipes.
- Developer-facing skills MUST live under `.agents/skills/` (naming: the runtime-skill rule above). A workflow that
  reads source files, updates schemas, validates fixtures, changes collectors/producers, or coordinates
  frontend/backend/aggregator code is a project developer skill, not a public one.
- Skill verification harness inputs (seed questions, grader rubrics, runner scripts, transcript-generation prompts)
  live under `.agents/skill-verification/<skill>/`, never under `docs/netdata-ai/skills/<skill>/`. Shared cross-skill
  invocation cases live under `.agents/skill-verification/invocation/`; that directory is not a runtime skill.
- Each public skill is reachable from `.agents/skills/<skill-name>` via a relative symlink
  (`.agents/skills/<name>` -> `../../docs/netdata-ai/skills/<name>`). From the repository root, create a new link with
  `ln -s ../../docs/netdata-ai/skills/<name> .agents/skills/<name>`. Inspect an existing path instead of overwriting it;
  replacement follows Git And PR Workflow. Verify the target with `readlink .agents/skills/<name>` and verify that
  `.agents/skills/<name>/SKILL.md` resolves to a file.
- Scripts MUST follow the existing `_lib.sh` shape: `set -euo pipefail`; ANSI colors as real ESC bytes via
  `$'\033[...]'`; `<prefix>_repo_root` via `git rev-parse --show-toplevel`; `<prefix>_load_env` sourcing `<repo>/.env`
  with `: "${VAR:?}"` validation; `<prefix>_audit_dir` creating the skill's `<repo>/.local/audits/<dir>/` (Local-Only
  Working Directory); masked-token `<prefix>_run` / `<prefix>_run_read` wrappers.
- Credential setup MUST be explicit in the task workflow: source the library, then call its environment loader when
  live execution needs it. Sourcing a library or reviewing a recipe MUST NOT itself load `.env` or query a service.
  Public request wrappers use the configured environment; do not assume every call reloads it.
- Helpers handling credential bytes MUST use internal names (`_skill_*`) and keep those bytes out of displayed output.
  Use the owner's supported caller-local assignment mechanism; when an internal helper emits credential JSON, its
  caller MUST capture it privately. Do not require Bash namerefs on shells that lack them. Public wrappers MUST NOT
  log authentication material; successful response bodies may contain private service data and require review before
  sharing. Response forwarding is not a universal redaction guarantee.
- Token-handling libraries MUST provide offline synthetic verification of their credential handoff and masking paths,
  including stdout and stderr. A live request with an invalid sentinel is not an offline self-test; a failed or skipped
  request is not proof that a wrapper works. Use the skill's focused tests and documented supported shell; preserve
  caller state by invoking mutable self-tests in an isolated shell. Validation does not authorize real credential setup.
- How-tos catalog: each public skill ships `how-tos/INDEX.md`. Capture timing and authorization follow Knowledge
  Capture. When an authorized change adds or updates a how-to, keep its catalog consistent. Operator recipes stay
  in public skills; developer recipes belong in the matching `.agents/skills/` project skill and its index, if any.
- Skills without an operator audience (for example `triage-coverity`, `triage-sonarqube`, `triage-codeql`,
  `repo-pr-reviews`) stay under `.agents/skills/<name>/` with no public counterpart.

Skills index (runtime input under `.agents/skills/`, grouped by area; `.agents/skills/README.md` owns the area list
and the rule for adding one; each skill's frontmatter description is the authoritative trigger, this list is a pointer):

- Collectors. START HERE: `collectors-authoring`.
  - `collectors-authoring`: authoring, modifying or reviewing collectors across plugin families; shared identity,
    missing-data, lifecycle, cost and cardinality contracts, then selective framework/domain routes
  - `collectors-go-design`: go.d contract design/review and `config_schema.json` forms; selective operator-surface,
    identity, state/mutation and design-evidence guidance
  - `collectors-go-framework-v2`: implementing, migrating or reviewing a go.d V2 collector; `CollectorV2`,
    `metrix.CollectorStore`, `ChartTemplateYAML`/`charts.yaml`, `charttpl`, `chartengine`, V2 host scopes, V2 tests
  - `collectors-metadata-yaml`: what every collector `metadata.yaml` field says and how it reads: overview,
    permissions, auto-detection (including service discovery), limits and cost, prerequisites, option rows, examples,
    the known-errors troubleshooting catalog, metrics scopes, alerts, identity and keywords; a page that reads as a
    wall of text or claims something false
  - `collectors-prometheus-profiles`: creating, reviewing, validating, proving, iterating, or installing Prometheus
    chart profiles; exporter dashboard design, selector/relabel/fallback policy, coverage and cardinality, stock proof
    artifacts, live verification, the authoring scripts
  - `collectors-snmp-profiles`: authoring/review of SNMP profiles, typed rows, ddsnmp parsing and profile-format docs;
    requires MIB `MAX-ACCESS`/`ACCESS` checks and index-derived extraction for `not-accessible` INDEX objects
  - `collectors-snmp-trap-profiles`: authoring/review of trap profiles and `metrics:`/`charts:` rules, the trap
    `profile-format.md`, the generator `src/go/cmd/snmptrapprofilegen` (shipped as `snmp-trap-profile-gen`), stock
    pack regeneration and compression, category/severity taxonomy changes
  - Also relevant: `integrations-lifecycle` (the pipeline that turns `metadata.yaml` into pages) and
    `health-alert-authoring` (alerts on a collector's contexts).
  - Also relevant: `triage-snmp-diagnostics` (offline SNMP evidence investigations).
- Integrations.
  - `integrations-lifecycle`: implementation/review of metadata validation and integrations pipeline; `integrations/`
    generators, templates, generated outputs, `COLLECTORS.md`/`SECRETS.md`/`SERVICE-DISCOVERY.md`; ibm.d
    `contexts.yaml` and the NPM catalog generator; the collector-consistency rule
  - Also relevant: `collectors-metadata-yaml` (what the fields say) and `docs-learn-site-structure` (where the
    generated pages land).
- Health.
  - `health-alert-authoring`: authoring, adapting, or reviewing health alerts and templates in
    `src/health/health.d/*.conf`; lookup/calc/warn/crit, lifecycle, routing, health-config tests
- Topology.
  - `topology-authoring`: creating, changing or reviewing a topology producer (`topology:network-connections`,
    `topology:streaming`, `topology:snmp`, vSphere, `topology:cato_networks`, or a new one) and its
    `netdata.topology.v1` payload: actors, links, evidence, correlation rules, presentation, modals, overlays,
    validation, and the aggregator contract a producer relies on
- Tests.
  - `tests-query-corpus`: running, extending or reviewing `tests/query-corpus/`; fixtures, oracles, red/green cases for
    query-engine bugs, formatter byte-pins, validating a fix branch
- Packaging.
  - `packaging-static-installer`: building, testing, reviewing or troubleshooting the static self-extracting installer
    (`netdata-<arch>-latest.gz.run`) under `packaging/makeself/`
- Docs.
  - `docs-learn-site-structure`: changing, reviewing or troubleshooting docs publication on `learn.netdata.cloud`;
    `docs/.map/map.yaml`, its schema and `validate_map_schema.py`; why a Learn page is missing or wrong; ingest exit
    codes; MDX escape rules; redirects; the PR gate here (`check-markdown.yml`) and what runs after a docs PR merges
    (`trigger-learn-update.yml`, then the learn `ingest.yml`); Netlify deploy
  - `docs-learn-pr-preview`: only when the user explicitly asks to build, preview, or validate `learn.netdata.cloud`
    locally from a PR or docs branch; loads `docs-learn-site-structure` first
  - Also relevant: `integrations-lifecycle` (generated integration pages are published on Learn).
- Triage.
  - `triage-snmp-diagnostics`: offline SNMP evidence investigations and diagnostics-tool/interpretation reviews
    with `src/go/tools/snmp-diagnostics`;
    topology devices/links, metrics, BGP, licensing, slow collection, discovery and lifecycle failures; `list`,
    `validate`, `summary`, `inspect-device`, `inspect-link`, and `replay`. Not live queries, traps, or
    collector/profile authoring
  - `triage-coverity`: Coverity Scan defect triage
  - `triage-sonarqube`: inspect/review SonarCloud findings and helpers, or apply authorized selected-finding decisions;
    family and project-policy actions require scope covering all affected findings/projects
  - `triage-codeql`: GitHub Code Scanning / CodeQL triage
  - `triage-codacy`: inspect or review Codacy findings/helpers, run local analysis, or fetch PR issues; supplied
    evidence needs no live fetch. Write actions require user authorization and applicable project tracking.
  - `triage-agent-events`: investigating crashes, panics, or fatals across the fleet via the agent-events namespace;
    `AE_*` fields; 23h dedup; journal multi-value `selections` filters. Bug-investigation tool, not generic logs
  - Also relevant: `repo-pr-reviews` (pulls SonarCloud findings for a PR).
- Repo.
  - `repo-pr-reviews`: focused PR-comment inspection or complete PR triage; addressing findings when authorized
  - `repo-mirror-sources`: read-only inspection of Netdata-org source checkouts at `${NETDATA_REPOS_DIR}`;
    authorized mirror setup/sync, `--repo` scoping, and maintenance limits
  - `repo-skill-authoring`: creating, amending, restructuring or reviewing skills; selective authoring rules and owner
    references, bounded amendments, preservation evidence for restructures, read-only review and rot checks

Public skills (canonical under `docs/netdata-ai/skills/<name>/`, symlinked at `.agents/skills/<name>`):

- `query-netdata-cloud`: querying the Netdata Cloud REST API: metrics, logs (systemd-journal), alerts, generic Functions
  on a node
- `query-netdata-agents`: querying Agents directly on port 19999, including auto-minting per-agent bearer tokens from a
  Cloud token
- `query-snmp-traps`: SNMP trap logs via Cloud or Agent: entries, severities, categories, senders, dedup summaries,
  `TRAP_*` fields, `TRAP_JSON` searches

Output/reference skill trees, updated when the related public/operator workflow changes: `docs/netdata-ai/skills/`
(Netdata AI skill artifacts) and `src/ai-skills/` (generated or source AI skill artifacts, when present).

## Repository Rules

### Collector Consistency

When working on collectors, runtime behavior, metrics, charts, configuration, alerts, and authoritative documentation
sources MUST stay consistent in the source PR. Generated integration and umbrella documentation is
validated locally, then committed by the post-merge generated-artifact PR. Checklist and CI notes:
`.agents/skills/integrations-lifecycle/consistency.md`.

### Validation Commands

There is no full-project command matrix. Use the narrowest existing command that validates the changed subsystem,
and do not claim full-project validation from it. Local helper scripts such as `install.sh` may exist in a working
copy; inspect before use and do not assume they are tracked project interfaces.

### Go Test Style

- Prefer table-driven tests using `map[string]struct{}` keyed by case name when cases share setup and assertion
  shape. Map keys beat a `name` field in `[]struct{}`: names stay prominent and order-independent.
- Use separate test functions only when setup or assertions differ materially.
- Tests SHOULD compare a complete expected struct or map with the actual result instead of asserting individual
  fields, so missing and unexpected values are checked.
- Keep separate assertions for distinct behavior, such as errors, call counts, and input ownership.
- Focused comparisons MAY be used when unrelated or nondeterministic fields make whole-value comparison
  inappropriate.

### C Code

- Compilers and libcs: gcc, clang, glibc, musl.
- `libnetdata.h` includes everything in libnetdata (a couple of exceptions); do not include individual headers.
- `z`-suffixed allocators (`mallocz`, `reallocz`, `callocz`, `strdupz`, ...) call `fatal()` on failure, exiting the
  process. `freez()` accepts NULL.
- Reusable, generic, module-agnostic code goes to libnetdata.
- Doubly linked lists use the `DOUBLE_LINKED_LIST_*` macros.
- JSON: json-c for parsing, `buffer_json_*` for manual generation.

### Naming Conventions

- "Netdata Agent" (capitalized) for the product; "`netdata`" (lowercase, code-formatted) for the process.
- See `docs/DICTIONARY.md` for precise terminology.

### Local-Only Working Directory

`/.local/` at the repo root is gitignored and reserved for per-user runtime artifacts: audit reports, fetched API
data, scratch notes, queue files, intermediate triage decisions. Nothing under it is committed; treat it as
ephemeral between users and machines, not a shared source of truth.

Skill output SHOULD default to `<repo-root>/.local/audits/<dir>/`; a skill that ships a `_lib.sh` pins its `<dir>`
there. The directories predate the area-prefixed skill names and are kept as they are, so per-user caches survive
renames:

| Skill | `.local/audits/<dir>/` | Holds |
|---|---|---|
| `triage-coverity` | `coverity/` | raw fetches, per-defect details, triage |
| `triage-sonarqube` | `sonarqube/` | finding queues, FP templates |
| `triage-codacy` | `codacy/` | local analysis output, PR issue fetches |
| `triage-codeql` | `graphql/` | Code Scanning fetches and dismissals |
| `triage-agent-events` | `query-agent-events/` | fetched event batches |
| `triage-snmp-diagnostics` | `snmp-diagnostics/` | private bundle inspections, replay output, and incident reports |
| `docs-learn-pr-preview` | `learn-pr-preview/` | private isolated source snapshots, manifests, Learn builds and server logs |
| `repo-pr-reviews` | `pr-reviews/` | per-PR comment and review caches |
| `collectors-prometheus-profiles` | `prometheus-profiles/` | captured exposition dumps |
| `collectors-snmp-trap-profiles` | `snmp-trap-profiles/` | private generator artifacts and selected classification caches |
| `repo-skill-authoring` | `<subject>/` | inventory, staleness, preservation map, review reports, and throwaway tooling of a skill change |
| `query-netdata-agents` (public) | `query-netdata-agents/` | output of the agent-query wrappers and the bearer cache |
| `query-netdata-cloud` (public) | `query-netdata-cloud/` | saved Cloud API responses from its how-tos |
| `query-snmp-traps` (public) | `query-snmp-traps/` | saved trap query results from its how-tos |

A new runtime skill picks a `<dir>` equal to its topic; a public skill uses its skill name; `repo-skill-authoring` is
the exception: its `<subject>` is the directory name of the skill under change (distinct from that skill's own pinned
directory) or the SOW topic. All record the row here.

### Per-User Secrets

`/.env` at the repo root is gitignored and holds per-user secrets and endpoint configuration consumed by skill
scripts: API tokens, session cookies, project keys. Never commit secrets; never hard-code tokens in scripts.

- Setup: copy `<repo>/.env.template` to `<repo>/.env` and fill in the keys you need.
- Reference: `<repo>/.agents/ENV.md` is the single canonical guide to every key (what it is, where to find it, format,
  common mistakes, which skills need it). When a script errors with `<KEY> is empty`, check it there.
