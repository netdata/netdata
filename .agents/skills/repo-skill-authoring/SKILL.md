---
name: repo-skill-authoring
description: How to create, edit, slim, split, or review a skill under .agents/skills/ or docs/netdata-ai/skills/, and how to spot skill rot. Use when asked to write a new skill, add or change rules in an existing skill, slim or split a bloated or duplicated skill, run a staleness or rot pass over a skill, or review a skill change; on phrases like "write a skill for X", "slim this skill", "this skill is too long", "the skill is stale or wrong", "skill rot", "skill authoring". Covers the authoring rules (point at the owner document instead of restating it, one owner per fact, symbols and paths rather than line numbers, qualified enforcement claims), the change method (evidence round, numbered options, row-level preservation map, two-lens review), and the rot signals. Not for normalizing the skills layout across CLI tools or bootstrapping the SOW framework, not for PR comment mechanics (repo-pr-reviews), and not for authoring a collector (collectors-authoring).
---

# Skill Authoring

Developer skill for assistants changing this repository's skills. The MUSTs live in the root `AGENTS.md` and
`.agents/skills/README.md`; this skill points at them and states only the rules and the method that have no other
home. When an owner document and this skill disagree, the owner wins and this file is fixed in the same change.

## Pick Your Task

| Task | Read |
|---|---|
| create a skill | Where The Rules Live, Authoring Rules, Creating A Skill; `change-method.md` "Mechanical Hygiene" |
| add or change rules in an existing skill | Authoring Rules; `change-method.md` "Recorded Findings And Owners", "Mechanical Hygiene", "Close" |
| slim, split, or restructure a skill | Authoring Rules, then all of `change-method.md`; the method is not optional |
| periodic rot pass | Rot Signals |
| review a skill change | `change-method.md` "Review Round": the two lenses and what each reviewer receives |

## Where The Rules Live

Owner sections, cited by anchor so the audit catches a renamed heading. Read the row for the obligation it names; a row
names the subject and never copies the requirement text.

| Owner section | What you must get from it |
|---|---|
| `AGENTS.md#project-skills` | where runtime skills live; the same-PR rule for gap-closing and pointer-fixing updates; the slimming pass every skill change ends with and what it reports; the public-skill convention (audience boundary, symlinks, script shape, token safety, the live how-tos catalog); the grouped skills index |
| `AGENTS.md#durable-ai-facing-artifact-formatting` | retrieval structure, requirement words next to the action, prose width, reflow-only commits |
| `AGENTS.md#sensitive-data-in-durable-artifacts` | a skill is public even when local; sanitized evidence only |
| `AGENTS.md#enforcement` | what `.agents/sow/audit.sh` hard-fails on and what the PR gate `.github/workflows/sow.yml` re-checks. Facts the audit script itself owns: its path and anchor scans read tracked files (`git grep`, `git ls-files`), the anchor collector reads only files under `.agents/skills/` and `docs/netdata-ai/skills/`, and index entries resolve to directories only when written as a bullet whose first token is the backticked skill name followed by a colon, inside the block that ends at the `Public skills (` line |
| `AGENTS.md#local-only-working-directory` | evidence goes under `.local/audits/<dir>/`; this skill's row names the directory after the skill or SOW topic under change |
| `AGENTS.md#clean-end-state-over-less-churn` | the target is recorded before options are generated; the disclosure of what is removed and what is excluded; the reference search when a path is replaced; how coupled cleanup is handled |
| `AGENTS.md#working-with-the-user` | the user-decision format and when a decision is recorded |
| `AGENTS.md#review` | the blocker bar, what to do with a finding before acting, red test first, checkpoint commits, when a round repeats, the recurrence guard |
| `AGENTS.md#when-a-sow-is-required`, `AGENTS.md#followup-discipline`, `AGENTS.md#artifact-maintenance-gate` | a skill change is non-trivial work with a SOW; how deferred items are tracked; what every close records |
| `AGENTS.md#git-and-pr-workflow` | staging, and which git actions need explicit approval (deleting a file among them) |
| `AGENTS.md#open-source-reference-evidence` | the citation form for an owner outside this repository |
| `.agents/skills/README.md#naming`, `.agents/skills/README.md#areas` | the name form, frontmatter `name` equals the directory, the area minting rule and its same-change obligation; the no-nesting rule is in the README's opening paragraph |
| `.agents/skills/README.md#owner-section-citations` | pointing at a fact's owner rather than restating it is a MUST there; the anchor form and slug rules; the marker paragraph for a private owner document, none for a Learn-published one |
| `.agents/skills/README.md#finding-a-skill` | the index is the map; the frontmatter description is the trigger; the cross-reference for a skill serving two areas |

## Authoring Rules

Every bullet is a MUST unless it says SHOULD or MAY.

Ownership and pointing:

- Search for the owner before writing a fact: the shipped format document, an `ARCHITECTURE.md`, a tool `README.md`,
  a published operator page, a code-tree `README.md`, a sibling skill. A skill that restates any of them rots.
- An owner is a hand-maintained file. A generated page (generator banner, or listed as a generator's output) or a
  symlink into generated output (`git ls-files -s <path>` shows mode `120000`; `readlink <path>`) is never an owner or
  an authority.
- Cite the repository path, never a bare filename; several documents in this tree share a basename.
- An owner outside this repository cannot be an anchor citation: read it from the local mirror (`repo-mirror-sources`)
  and cite it as `owner/repo @ commit` (`AGENTS.md#open-source-reference-evidence`). A fact whose only owner is out of
  repo stays in the skill, labelled with what verified it and when, or as unverifiable; never delete what you cannot
  re-home.
- One owner per fact. When a fact moves into its owner, delete every copy in the same commit and record the owner
  section, not just the file.
- A pointer row carries one clause naming the subject; copying the requirement text creates a second owner.
- A keep-list fact with no owner is a user-owned fork: keep it in the skill, add it to the section of an existing
  code-side document that already covers the mechanism, or open a separate change for a new developer document.
  Present the options; never mint a second owner inside the skill.
- Developer procedure stays in the skill; a shipped operator document keeps only the operator form.
- Do not transcribe schemas, enum members, finding codes, or code; the reader opens the file.
- Design records rot once the code ships its own document. Propose retiring them, with the relocation target and the
  rejected alternatives; deletion is the user's call; history stays in git. Unshipped design that must survive goes
  to the owning subsystem's scope or architecture document, labelled as not shipped; when none exists, that is the
  user-owned fork above.

Citations and claims:

- Cite symbols and paths, never line numbers; never promise that line numbers track a branch.
- No PR, commit, issue, or SOW identifiers and no session labels (option letters, inventory ids) in a skill.
- Write each code-dependent sentence from the symbol, not from old skill text or a quick read, and state what a
  consumer relies on rather than how the producer gets there.
- Qualify every "the code enforces" claim with the tool, the mode, and the severity (error or warning); keep enforced
  checks and hand-reviewed rules in separate lists.
- Keep the permissive half of a rule: a MAY clause is a rule, inventoried and preserved on its own.
- A documented command runs as written: repository-root paths, where to run it from, the interpreter or environment
  it needs. A launcher's internal path resolution is a separate fact.
- No hard-coded aggregates, counts, or percentages that regeneration or the next slim changes; give the recompute
  command.
- A relocated fact adopts the receiving document's existing convention (requirement words, mood) and names it; a
  developer document written in lowercase indicative keeps that register. This never removes requirement force from
  an AI-facing artifact.
- Tests never pin skill prose.

Structure and routing:

- Every file is routed from `SKILL.md`; a router or index lists only files that exist (a bare name is invisible to the
  audit's path scan); no stubs.
- When a skill serves more than one audience, `SKILL.md` routes by audience and a topic file SHOULD serve one.
- The live how-tos catalog MUST binds public skills only; a runtime skill's `how-tos/INDEX.md` is optional: keep it
  when `SKILL.md` routes it and every listed file exists, otherwise list the how-tos in `SKILL.md`.
- A peer skill gets one routing sentence. A one-way prerequisite MAY be declared (an entry-point skill, a
  read-this-first skill); two skills never require each other. When two skills claim one directory, each carries one
  boundary line.
- The trigger (the frontmatter description) enumerates the phrases users type and carries "not for" clauses toward
  neighbouring skills; the `AGENTS.md` index entry matches it, including every producer, subcommand, or binary name.
- Recommended skeleton, each element with the skill that shows it: frontmatter trigger (all); an owners block with
  anchors and one clause per owner (`topology-authoring` as a table, `collectors-snmp-trap-profiles` as bullets); a
  task router (`collectors-prometheus-profiles`); a rule sheet split into what the code enforces and rules with no
  code owner (`collectors-prometheus-profiles`, `topology-authoring`); a workflow (all); validation commands
  (`collectors-snmp-trap-profiles`); a how-to list (`collectors-prometheus-profiles`, `topology-authoring`); a one-file
  rule sheet when there is one audience (`collectors-snmp-profiles`, `collectors-snmp-trap-profiles`).

## Creating A Skill

1. Resolve the runtime-vs-public fork before the first file exists (`AGENTS.md#project-skills` has the criteria and
   the audience boundary); a public skill also gets its relative symlink.
2. Pick the area (`.agents/skills/README.md#areas`); name the directory `<area>-<topic>` and set frontmatter `name` to
   the directory name (`.agents/skills/README.md#naming`).
3. Write the trigger with the phrases users type and the "not for" clauses; write `SKILL.md` on the skeleton above; add
   a supporting file only when `SKILL.md` routes it.
4. If the skill writes output, add its row to the table in `AGENTS.md#local-only-working-directory`.
5. Add the index entry under its area in `AGENTS.md#project-skills`, in the form the audit resolves (Where The Rules
   Live, the `AGENTS.md#enforcement` row), and the cross-reference `.agents/skills/README.md#finding-a-skill` asks for.
6. `git add` the new files (the audit's path and anchor scans read tracked files; nothing in CI runs the audit), run
   `bash .agents/sow/audit.sh`, then one review round with the correctness lens plus a trigger-coverage check (there
   is no inventory yet, so no preservation lens).

## Rot Signals

Run this over a skill periodically and before extending it. A cluster of hits is a slim; a single hit is a one-line fix
in the same change.

- Line-number citations; PR, commit, issue, or SOW identifiers; a promise that line numbers track a branch (grep for
  them; only legacy four-digit SOW identifiers are automated, by the audit).
- A code-side document now exists for facts the skill states.
- "Authoritative design", "Spec -", Status, Migration Notes, Schema Additions Required, phase history, open review
  questions.
- The same fact in two or more files; transcribed enum members, finding codes, schema fields.
- Unqualified "the loader enforces", "validates", "rejects".
- An index nothing routes, or one listing files that do not exist; stubs; dead paths; two skills requiring each other.
- Hard-coded counts or percentages.
- Index entry and frontmatter disagree; a new producer, subcommand, or flag the trigger does not name.
- No task router: every task opens most of the skill; `SKILL.md` narrative paid on every trigger.
- Audit anchor failures after an owner document edit; a relative link the audit does not check that no longer
  resolves (`change-method.md` "Recorded Findings And Owners" lists what it checks).
- A generated page or symlink in an authority list.
- An owner section named in prose instead of cited by anchor.

## Maintaining This Skill

This skill has no code-side owner; nothing feeds it but skill work. A lesson or rot signal learned while changing any
skill lands here in the same SOW, and that SOW's Artifact Maintenance Gate says so. When the method itself changes, a
fresh-context agent runs the method from this skill, `AGENTS.md`, and the README alone; what it misses is the
under-specified line, fixed in the same SOW.
