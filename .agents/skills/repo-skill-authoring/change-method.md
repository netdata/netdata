# Changing A Skill

Choose the work that matches the request. A small rule, example, trigger, or local duplicate does not by itself
require a restructure. Use `./authoring-rules.md#authoring-rules` for content requirements; root `AGENTS.md` owns
SOW classification, authorization, review policy and delivery. There is no size target: report line counts, but judge
correctness, retrieval and preservation.

A review-only request uses `./change-method.md#review-round`; it does not authorize edits, staging, commits, a new SOW,
or the operational commands being reviewed. An implementation request follows the applicable SOW gate, including its
fixed-goal approval rule and exemptions (`AGENTS.md#plan-before-non-trivial-work`). Knowledge Capture amendments MAY
reuse the active implementation SOW when they belong to that work.

## Evidence Storage

Use `.local/audits/<subject>/<run>/` for inventories, source checks, preservation maps, review reports and throwaway
tooling (`AGENTS.md#local-only-working-directory`). Name review reports `review-r<n>-<lens>.md` and PR bot reports
`review-bot<n>.md` within that run. Inspect existing destinations before writing or delegating. Choose
an unused run subdirectory and create new evidence files exclusively; never overwrite an earlier run merely because
it used the same standard filename. Explicitly pass each writer its owned output path. Read partial output before
retrying an interrupted assignment; preserve it or deliberately continue the same current-run artifact.

The SOW holds the target, decisions, dispositions and validation summary. Evidence files support it; they are not the
sole durable record of a shipped rule. Read-only work MAY save sanitized local notes without creating an implementation
SOW (`AGENTS.md#knowledge-capture`).

## Bounded Amendment

Use this path when the audience, ownership and workflow remain intact and the change has a locally checkable boundary.
Examples include correcting one claim, improving a trigger, fixing a command, or replacing a local duplicate with its
existing owner pointer.

1. Read the affected clauses, their governing owners, callers and relevant recorded findings. Verify the claim at its
   source; inspect any exception or permissive half that the edit could change.
2. Record the implementation target and validation under the applicable SOW gate. The existing request or approved
   plan covers a fixed goal; ask only for a genuine user-owned fork.
3. Make the amendment and its coupled pointer fixes. Record what changed, its evidence, any intentionally retained
   material, and line counts before and after. A full inventory, two evidence agents and a preservation map are not
   required for this path.
4. Check affected commands, links, routing and source claims safely, then use the review coverage required by the
   user and `AGENTS.md#review`. This path does not automatically require two review lenses.
5. Complete `./change-method.md#mechanical-hygiene` and `./change-method.md#close`.

If the change relocates substantial knowledge, removes a workflow, changes audience boundaries, or makes omissions
hard to detect locally, use restructure evidence for that affected unit. Do not expand a bounded task to the entire
skill solely because unrelated rot is visible; handle independent findings under `AGENTS.md#followup-discipline`.

## Restructure Evidence

For a substantial slim, split or rewrite, first identify candidate owners and incoming references using
`./change-method.md#recorded-findings-and-owners`. Then run two independent read-only evidence passes with fresh
context, scoped to the skill unit and its coupled surfaces. One owns `inventory.md`, the other `staleness.md` in the
new run directory. Give both the scope and owner candidates (possibly empty); they MAY extend that list. Each writes its
assigned file before returning its summary. The coordinator verifies consequential findings; source decides factual
disagreements, and the finer split wins when inventory clauses differ.

The inventory opens with per-file line counts, prose-width observations and provenance from `git log --follow --
<path>`: first and last change dates and commit count. Keep hashes in local evidence, never in the skill. Distinguish
maintained design records from abandoned history. Inventory the changed material as follows:

- Give each rule or hard-won fact a stable per-file ID in document order; qualify repeated basenames by directory.
  Split compound obligations into separate clauses. Give every permissive MAY half its own row.
- Treat a closed list as one row identifying all members and their count. Classify rows as MUST, MUST NOT, SHOULD,
  MAY, fact, enum, pointer, procedure, history, rationale, example or status. Pure narrative and examples MAY be
  summarized in the census instead of receiving preservation rows; do not classify a unique operational fact as
  disposable narrative.
- Record internal duplication, overlap with owner documents and other skills, task clusters, file reachability,
  external references, and CI/audit coupling. Detailed physical-line classification is useful when it explains
  bloat, but is not a prerequisite for every restructure.
- Entire unchanged supporting files MAY be retained as file-level units if their unchanged contents and reachability
  are verified. Expand any changed contract into clause-level rows. Check that reported row counts match actual IDs.

The staleness pass checks claims against source, rather than treating old prose as authority:

- Label checked claims FALSE, STALE, DEAD, TRUE-HIGH-VALUE (otherwise unstated), TRUE-DERIVABLE (available from an
  owner), or UNVERIFIABLE with the specific source gap. Identify Tier-1 claims that would cause wrong work today.
- Check that each proposed authority exists and is hand-maintained. Record missing domain knowledge in a keep list
  with stable `K<n>` IDs for the preservation map.
- For external owners, use the revision and acquisition boundaries in `./authoring-rules.md#authoring-rules`.
  Neither an absent mirror nor an unverifiable claim automatically authorizes synchronization, substitutes latest
  upstream for historical evidence, or requires a user decision.

The coordinator re-verifies every Tier-1 item before implementation and preserves material uncertainty in the record.

## Recorded Findings And Owners

- Inspect findings relevant to the affected unit: accessible GitHub issues, local SOWs/audits, known PR findings and
  sibling references. Record each as fixed, rejected with evidence, or tracked under `AGENTS.md#followup-discipline`.
  Record an inaccessible store as unsearched, not empty. Looking up an issue does not require a full PR-comment
  triage workflow; use `repo-pr-reviews` when that workflow is actually requested.
- Search owner candidates and generated-file indicators using `./authoring-rules.md#authoring-rules`. Check Learn
  publication via `docs/.map/map.yaml` when deciding the private-owner marker recommended by
  `.agents/skills/README.md#owner-section-citations`. Root instructions and the skills README already use inline
  citing sentences; retain that convention. Cite the exact section that states the fact, or its script/symbol owner.
- For changed paths, headings or contracts, search incoming references in root and scoped instructions, sibling
  skills, developer docs, code-tree README files, tests, CI filters and audit globs. Record relevant hits and their
  disposition under `AGENTS.md#clean-end-state-over-less-churn`.
- `.agents/sow/audit.sh` defines the path and anchor forms it checks. Verify other relevant forms explicitly,
  including unanchored links, bare filenames, public-skill paths and links from outside a skill. Check new files
  explicitly: tracked-file scans can miss them before staging. A backticked placeholder or ordinary word does not
  require an indiscriminate repository-wide symbol search.

## Decisions Before Implementation

Record the target first. Use the fixed-goal rule in `AGENTS.md#plan-before-non-trivial-work`; do not manufacture an
options round for a factual correction, routine placement, or a plan already authorized. Keep valid knowledge in its
current home when no better owner exists. An evidence gap calls for qualification and further investigation; ask
only if the unresolved answer materially affects a user-owned decision.

For genuine scope, audience, design, public-contract or destructive forks, present evidence, numbered options and a
recommendation and record the decision before dependent implementation (`AGENTS.md#working-with-the-user`). Existing
file deletion still needs the authorization required by `AGENTS.md#git-and-pr-workflow`.

## Rewrite And Preservation Map

Write to the authoring rules. For a restructure, maintain `preservation-map.md` with one disposition per inventory ID:

- KEPT: where the rule remains reachable.
- CORRECTED: what was wrong and the governing source or symbol supporting the correction.
- OWNER: the exact cited `<path>.md#<anchor>` that now holds the rule.
- DROPPED: evidence that the claim is false, the named owner that supersedes it, or the recorded user decision.

Keep permissive clauses independently accounted for. A range row is allowed only after opening every ID in the range,
with the same verified disposition; never use a range to claim a pre-existing owner. Map every keep-list item to its
new section. Include coupled owner/reference fixes when low-risk and within scope, and disclose them in the SOW.
Relocated facts follow the receiving document's register and citation rules without losing requirement force.

## Creating A Skill

1. Determine the task and audience. Use existing naming/area rules and the public-versus-developer boundary in
   `./authoring-rules.md#where-the-rules-live`. Resolve genuine audience or scope forks; routine placement following
   those rules needs no new approval.
2. Search for existing owners and overlapping skills before writing. Create the entry with `name` and `description`
   frontmatter, a selective task router and only the supporting files the task needs. Use representative discovery
   terms; keep exhaustive mappings in references.
3. Add the output-location row required by `AGENTS.md#local-only-working-directory` when the skill writes output.
   Wire the grouped root index and required cross-area pointers. Public skills use the canonical public directory and
   runtime symlink described by `AGENTS.md#project-skills`; output/reference skill trees retain their own contracts.
4. Validate facts, commands, routing and every new file/link explicitly. Review the complete new skill and its coupled
   wiring under `./change-method.md#review-round`, including representative trigger coverage, then close. No pre-change
   inventory or preservation map is needed when there is no existing material to preserve.

## Review Round

Review the complete active unit and its interactions, not unrelated accumulated branch changes. Give every reviewer
its original request, approved decisions, SOW filename when present, exact diff/base or recorded working-tree state,
owner sources and relevant validation. The old text reveals removed contracts; it is evidence of change, never the
truth authority. Reviewers are read-only and MUST NOT launch more agents or execute the skill's operational workflow.
When an existing change has no inventory or map, derive the affected obligations from its diff and owners. Report
concrete missing evidence; do not require the author to repeat an implementation ceremony solely to supply those files.

Use the coverage required by the user and `AGENTS.md#review`. For a restructure, use two independent fresh-context
lenses, both receiving the complete unit:

- Preservation: checks inventory and keep-list dispositions against reachable new text or cited owner sections,
  including permissive halves and any range rows.
- Correctness: checks changed and retained applicable contracts at their sources, enforcement qualifiers, command
  prerequisites, anchors, invocation boundaries and unintended effects of removals. Assess the whole unit and its
  acceptance criteria, not only individual claims. Record unavailable sources as unverifiable.

For changes to this method, include a fresh-context task walkthrough in the correctness assignment: bounded
amendment, restructure, review-only work, and relevant authorization/evidence edge cases. It is part of the same
review, not an extra review round.

Ask reviewers for a proposed fix per finding. Verify findings before acting and classify blockers versus TODO using
`AGENTS.md#review`. Validate prose commands
through source inspection, syntax checks or isolated fixtures with synthetic data where appropriate. Never run a
remote write, credential flow, destructive command or service operation merely to prove a reviewed example; report
what was checked and what remains unverified. Where feasible, reproduce a concrete defect safely before fixing it
and confirm the same case afterward.

Incorporate user steering promptly. If the review state changes, tell reviewers the precise changes or let them finish
against their recorded state; do not hold the user's request until the round ends. Apply useful in-scope suggestions
and run affected checks. Suggestions alone do not trigger another round. Repeat the complete unit only for a verified
blocker that materially changes behavior or a review missing necessary context/coverage, following the owner's stop
and recurrence rules. Git checkpoints require existing Git authorization; a review never supplies it.

## Mechanical Hygiene

Use `AGENTS.md#durable-ai-facing-artifact-formatting`: wrap rewritten paragraphs in their content commit; a change
that only reflows surviving prose belongs in a separate whitespace-only commit. For reflow tools, preserve fences,
tables, lists and paragraph boundaries, including a dash-space bullet beginning a new paragraph. Assert identical
whitespace-split tokens for reflow-only text; never automatically join short lines.

Count characters, not bytes. From the repository root, pass each touched Markdown file explicitly, including new
untracked files and coupled owners. This example checks this skill; add other affected paths as quoted arguments.
It reports non-table lines over the prose limit; assess fences/frontmatter under the owner rule before changing them.

```bash
python3 - .agents/skills/repo-skill-authoring/SKILL.md \
  .agents/skills/repo-skill-authoring/authoring-rules.md \
  .agents/skills/repo-skill-authoring/change-method.md <<'PY'
import sys
from pathlib import Path
for filename in sys.argv[1:]:
    for number, line in enumerate(Path(filename).read_text().splitlines(), 1):
        if len(line) > 120 and not line.startswith('|'):
            print(f'{filename}:{number}')
PY
```

- Never put a literal HTML comment opener in skill prose or a code span; it can hide subsequent headings from the
  anchor scanner (`.agents/skills/README.md#owner-section-citations`).
- Inspect the complete intended diff before committing. Use `git diff --check HEAD` for tracked/staged changes and
  check new untracked content explicitly. Stage only authorized files; read-only validation does not require staging.
- Handle sensitive-scan hits with the prescribed placeholders and source-accurate wording
  (`AGENTS.md#sensitive-data-in-durable-artifacts`, `.agents/sow/scan-sensitive.sh`).

## Close

Complete required validation and review before marking implementation complete. Run `bash .agents/sow/audit.sh` before
every authorized commit and at close. Its citation/path scans use tracked files: when staging is authorized, include
new files before the audit; otherwise explicitly check those files and report that audit coverage limit. CI does not
replace local validation (`.github/workflows/sow.yml`).

Record before/after line counts, source/symbol checks for affected claims, and the final reference search for renamed
or removed paths/headings. Finish the touched-file slimming pass under `AGENTS.md#project-skills`; that pass does not
itself require a new restructure or review cycle. Complete follow-up mapping and artifact maintenance under
`AGENTS.md#followup-discipline` and `AGENTS.md#artifact-maintenance-gate`, then the SOW lifecycle when applicable.
Capture reusable method lessons under `./SKILL.md#maintaining-this-skill` without adding a second owner for root policy.
