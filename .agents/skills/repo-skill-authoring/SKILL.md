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
| create a skill | Where The Rules Live, Authoring Rules, Creating A Skill |
| add or change rules in an existing skill | Authoring Rules; then `change-method.md` "Close" |
| slim, split, or restructure a skill | Authoring Rules, then all of `change-method.md`; the method is not optional |
| periodic rot pass | Rot Signals; a cluster of hits becomes a slim, a single hit a one-line fix |
| review a skill change | `change-method.md` "Review Round": the two lenses and what each reviewer receives |

## Where The Rules Live

Owner sections, cited by anchor so the audit catches a renamed heading. Read the row for the obligation it names; a row
names the subject and never copies the requirement text.

| Owner section | What you must get from it |
|---|---|
| `AGENTS.md#project-skills` | where runtime skills live; a gap-closing skill update ships in the PR that exposed it; the slimming pass every skill change ends with (keep every rule, report line counts before and after); the public-skill convention (audience boundary, symlinks, script shape, token safety, the live how-tos catalog); the grouped index, whose entries the audit parses as a bullet whose first token is the backticked skill name followed by a colon |
| `AGENTS.md#durable-ai-facing-artifact-formatting` | retrieval structure, requirement words next to the action, ~120 columns, reflow-only commits kept separate |
| `AGENTS.md#sensitive-data-in-durable-artifacts` | a skill is public even when local; sanitized evidence only |
| `AGENTS.md#enforcement` | what `.agents/sow/audit.sh` hard-fails on: skills layout, `.agents/skills/` paths named in any tracked file, index agreement, owner-section anchors, legacy SOW identifiers, relocated spec paths, the sensitive scan |
| `AGENTS.md#local-only-working-directory` | evidence goes under `.local/audits/<dir>/`; this skill's row uses the name of the skill or SOW topic under change |
| `AGENTS.md#clean-end-state-over-less-churn` | record the target before generating options; disclose what is removed and what is excluded; the reference search when a path is replaced; coupled cleanup is included and disclosed |
| `AGENTS.md#working-with-the-user` | how a user decision is presented: evidence, numbered options, pros and cons, a recommendation |
| `AGENTS.md#review` | the blocker bar, verify findings before acting, red test first, checkpoint commits, one round by default, repeat only after a material change, the recurrence guard |
| `AGENTS.md#when-a-sow-is-required`, `AGENTS.md#followup-discipline`, `AGENTS.md#artifact-maintenance-gate` | a skill change is non-trivial work with a SOW; deferred items are tracked; every close records the artifact gate |
| `AGENTS.md#git-and-pr-workflow` | stage specific files; deleting a file, checking one out, or resetting needs explicit approval |
| `AGENTS.md#open-source-reference-evidence` | an owner outside this repository is cited as `owner/repo @ commit` plus a repository-relative path |
| `.agents/skills/README.md#naming`, `.agents/skills/README.md#areas` | the name form, frontmatter `name` equals the directory, the area minting rule; the no-nesting rule is in the README's opening paragraph |
| `.agents/skills/README.md#owner-section-citations` | the anchor form and slug rules; the marker paragraph for a private owner document, none for a Learn-published one; the anchor check scans skill files only |
| `.agents/skills/README.md#finding-a-skill` | the index is the map; the frontmatter description is the trigger |

## Authoring Rules

Ownership and pointing:

- Search for the owner before writing a fact: the shipped format document, an `ARCHITECTURE.md`, a tool `README.md`,
  a published operator page, a code-tree `README.md`, a sibling skill. A skill that restates any of them rots.
- An owner is a hand-maintained file. A generated page, or a symlink into generated output (check with
  `git ls-files -s` and `readlink`), is never an owner or an authority.
- Cite the repository path, never a bare filename; several documents in this tree share a basename.
- An owner outside this repository cannot be an anchor citation: cite it as `owner/repo @ commit`. A fact whose only
  owner is out of repo stays in the skill, labelled with what verified it or as unverifiable; never delete what you
  cannot re-home.
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
  to the code-side scope document, labelled as not shipped.

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
- No hard-coded counts or percentages that regeneration or the next slim changes; give the recompute command.
- A relocated fact adopts the receiving document's existing convention (requirement words, mood) and names it; a
  developer document written in lowercase indicative keeps that register. This never removes requirement force from
  an AI-facing artifact.
- Tests never pin skill prose.

Structure and routing:

- Every file is routed from `SKILL.md`; a router or index lists only files that exist (a bare name is invisible to the
  audit's path scan); no stubs.
- When a skill serves more than one audience, `SKILL.md` routes by audience and a topic file SHOULD serve one.
- Repo-wide `AGENTS.md` rules are pointed at, never restated. The live how-tos catalog MUST binds public skills only;
  a runtime skill's `how-tos/INDEX.md` is optional: keep it when `SKILL.md` routes it and every listed file exists,
  otherwise list the how-tos in `SKILL.md`.
- A peer skill gets one routing sentence and is never a required reference (that is circular). When two skills claim
  one directory, each carries one boundary line.
- The trigger (the frontmatter description) enumerates the phrases users type and carries "not for" clauses toward
  neighbouring skills; the `AGENTS.md` index entry matches it, including every producer, subcommand, or binary name.
- The shape that has held: frontmatter trigger; an owners table with anchors and clauses; a task router; a rule sheet
  split into what the code enforces and rules with no code owner; workflow; validation commands; the how-to list.
  Precedents: `collectors-snmp-trap-profiles`, `collectors-prometheus-profiles`, `topology-authoring`.

## Creating A Skill

1. Decide runtime or public (`AGENTS.md#project-skills`): a workflow that reads source files, updates schemas, or
   changes collectors is a runtime skill; a public skill teaches operators and gets its relative symlink.
2. Pick the area from `.agents/skills/README.md#areas`, or add its row in the same change; name the directory
   `<area>-<topic>` and set frontmatter `name` to the directory name.
3. Write the trigger with the phrases users type and the "not for" clauses; write `SKILL.md` in the shape above; add a
   supporting file only when `SKILL.md` routes it.
4. If the skill writes output, add its row to the table in `AGENTS.md#local-only-working-directory`.
5. Add the index entry under its area in `AGENTS.md#project-skills`, in the bullet form the audit parses, and a
   cross-reference from any second area it serves.
6. Run `bash .agents/sow/audit.sh`; then one review round with the correctness lens plus a trigger-coverage check
   (there is no inventory yet, so no preservation lens).

## Rot Signals

Run this over a skill before extending it, and periodically. A cluster of hits is a slim; a single hit is a one-line
fix in the same change.

- Line-number citations; PR, commit, issue, or SOW identifiers; a promise that line numbers track a branch (grep for
  them; nothing automates this yet).
- A code-side document now exists for facts the skill states.
- "Authoritative design", "Spec -", Status, Migration Notes, Schema Additions Required, phase history, open review
  questions.
- The same fact in two or more files; transcribed enum members, finding codes, schema fields.
- Unqualified "the loader enforces", "validates", "rejects".
- An index nothing routes, or one listing files that do not exist; stubs; dead paths; a peer skill as a required
  reference.
- Hard-coded counts or percentages.
- Index entry and frontmatter disagree; a new producer, subcommand, or flag the trigger does not name.
- No task router: every task opens most of the skill; `SKILL.md` narrative paid on every trigger.
- Audit anchor failures after an owner document edit; a code-tree relative link whose `../` depth no longer resolves.
- A generated page or symlink in an authority list.
- An owner section named in prose instead of cited by anchor.

## Maintaining This Skill

This skill has no code-side owner; nothing feeds it but skill work. A lesson or rot signal learned while changing any
skill lands here in the same SOW, and that SOW's Artifact Maintenance Gate says so. When the method itself changes, a
fresh-context agent runs the method from this skill, `AGENTS.md`, and the README alone; what it misses is the
under-specified line.
