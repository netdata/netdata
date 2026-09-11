---
name: repo-skill-authoring
description: How to create, edit, slim, split, or review a skill under .agents/skills/ or docs/netdata-ai/skills/, and how to spot skill rot. Use when asked to write a new skill, add or change rules in an existing skill, slim or split a bloated or duplicated skill, run a staleness or rot pass over a skill, or review a skill change; on phrases like "write a skill for X", "slim this skill", "this skill is too long", "the skill is stale or wrong", "skill rot", "skill authoring". Covers the authoring rules (point at the owner document instead of restating it, one owner per fact, symbols and paths rather than line numbers, qualified enforcement claims), the change method (evidence round, numbered options, row-level preservation map, two-lens review), and the rot signals. Not for normalizing the skills layout across CLI tools or bootstrapping the SOW framework, not for PR comment mechanics (repo-pr-reviews), and not for authoring a collector (collectors-authoring).
---

# Skill Authoring

Developer skill for assistants changing this repository's skills. The MUSTs that have another home live in the root
`AGENTS.md` and `.agents/skills/README.md`; this skill points at them and states only the rules and the method that
have no other home. When an owner document and this skill disagree, the owner wins and this file is fixed in the same
change.

## Pick Your Task

| Task | Read |
|---|---|
| create a skill | `./SKILL.md#where-the-rules-live`, `./SKILL.md#authoring-rules`, `./SKILL.md#creating-a-skill`; `./change-method.md#review-round`, `./change-method.md#mechanical-hygiene`, `./change-method.md#close` |
| add or change rules in an existing skill | `./SKILL.md#where-the-rules-live`, `./SKILL.md#authoring-rules`; `./change-method.md#recorded-findings-and-owners`, `./change-method.md#mechanical-hygiene`, `./change-method.md#close` |
| slim, split, or restructure a skill | `./SKILL.md#where-the-rules-live`, `./SKILL.md#authoring-rules`, `./SKILL.md#rot-signals`, then all of `./change-method.md#changing-a-skill`; the method is not optional |
| periodic rot pass | `./SKILL.md#rot-signals` |
| review a skill change | `./change-method.md#review-round`: the two lenses and what each reviewer receives |

## Where The Rules Live

Owner sections, cited by anchor (`.agents/skills/README.md#owner-section-citations`). Read the row for the obligation it
names.

| Owner section | What you must get from it |
|---|---|
| `AGENTS.md#project-skills` | where runtime skills live; the same-PR rule for gap-closing and pointer-fixing updates; the slimming pass every skill change ends with and what it reports; the public-skill convention (audience boundary, symlinks, script shape, token safety, the live how-tos catalog); output/reference skill trees and their no-rename rule; the grouped skills index |
| `AGENTS.md#durable-ai-facing-artifact-formatting` | retrieval structure, requirement words next to the action, prose width, reflow-only commits |
| `AGENTS.md#sensitive-data-in-durable-artifacts`, `.agents/sensitive-data-discipline.md#allowed-alternatives` | the public-artifact assumption and the sanitized-evidence requirement; the `<repo>/`-prefixed path form for prose (an anchor citation stays repo-relative without it) |
| `AGENTS.md#enforcement` | what `.agents/sow/audit.sh` hard-fails on and what the PR gate `.github/workflows/sow.yml` re-checks |
| `AGENTS.md#local-only-working-directory` | where skill-change evidence goes, and how this skill's `<dir>` (the `<subject>` in `./change-method.md#changing-a-skill`) is named |
| `AGENTS.md#clean-end-state-over-less-churn` | the target is recorded before options are generated; the disclosure of what is removed and what is excluded; the reference search when a path is replaced; how coupled cleanup is handled |
| `AGENTS.md#working-with-the-user` | the user-decision format and when a decision is recorded |
| `AGENTS.md#review` | review scope, reproduction evidence, authorized checkpoints, repeat and stop conditions |
| `AGENTS.md#knowledge-capture` | discovery notes, documentation authorization, and coupled skill maintenance |
| `AGENTS.md#when-a-sow-is-required`, `AGENTS.md#followup-discipline`, `AGENTS.md#validation-gate`, `AGENTS.md#artifact-maintenance-gate` | a skill change is non-trivial work with a SOW; how deferred items are tracked; what Validation must hold and what every close records |
| `AGENTS.md#git-and-pr-workflow` | staging, and which git actions need explicit approval (deleting a file among them) |
| `AGENTS.md#open-source-reference-evidence` | the citation form for an owner outside this repository |
| `.agents/skills/README.md#skills`, `.agents/skills/README.md#naming`, `.agents/skills/README.md#areas` | the no-nesting rule; the name form; the area minting rule with its same-change obligation |
| `.agents/skills/README.md#owner-section-citations` | the point-rather-than-restate rule and the anchor form (a MUST) with its slug rules; the marker paragraph for a private owner document, none for a Learn-published one |
| `.agents/skills/README.md#finding-a-skill` | the index is the map; the frontmatter description is the trigger; the cross-reference for a skill serving two areas |

## Authoring Rules

Every requirement below is a MUST unless it says SHOULD or MAY.

Ownership and pointing:

- Search for the owner before writing a fact: the shipped format document, an `ARCHITECTURE.md`, a tool `README.md`,
  a published operator page, a code-tree `README.md`, a sibling skill, a script or workflow. A skill that restates any
  of them rots.
- An owner is a hand-maintained file. A generated page (an opening HTML comment carrying `startmeta` and
  `meta_yaml:`, a `DO NOT EDIT` banner, or a file `integrations-lifecycle` lists as generator output) or a symlink into
  generated output (`git ls-files -s <path>` shows mode `120000`; `readlink <path>`) is never an owner or an authority.
- When the only owner of a fact is a script or a workflow (`.agents/sow/audit.sh`, `.github/workflows/sow.yml`),
  attribute the fact to that file, not to the prose section that names the script: a section that does not state the
  fact cannot be checked against it.
- Cite the repository path, never a bare filename; several documents in this tree share a basename. A one-segment
  path names a repository-root file. Inside a skill, cite a section of its own files as `./<file>.md#<anchor>`.
- An owner outside this repository cannot be an anchor citation: read it from the local mirror (`repo-mirror-sources`)
  and cite it as `owner/repo @ commit` (`AGENTS.md#open-source-reference-evidence`). When the mirror is absent, a
  throwaway shallow clone of the owner's default branch outside the repository serves the same purpose; a checkout
  found elsewhere on the machine is usable only when its worktree is clean (`git status --short` prints nothing) and
  after comparing its commit with upstream (`git ls-remote`), since the commit is what the citation pins and a
  modified checkout would be cited under a commit it does not match. A fact whose only owner is out of repo stays in
  the skill, labelled with the verifying commit, or as unverifiable; never delete what you cannot re-home.
- One owner per fact. When a fact moves into its owner, delete every copy in the same commit and record the owner
  section, not just the file.
- A pointer row carries one clause naming the subject; copying the requirement text creates a second owner.
- A keep-list fact with no owner is a user-owned fork: keep it in the skill, add it to the section of an existing
  code-side document that covers the mechanism, or open a separate change for a new developer document. Never mint a
  second owner inside the skill.
- Developer procedure stays in the skill; a shipped operator document keeps only the operator form.
- Do not transcribe schemas, enum members, finding codes, or code; the reader opens the file.
- Design records rot once the code ships its own document. Propose retiring them, with the relocation target and the
  rejected alternatives; deletion is the user's call; history stays in git. Unshipped design that must survive goes
  to the owning subsystem's scope or architecture document, labelled as not shipped; when none exists, that is the
  user-owned fork above. A rule a skill declares mandatory or durable that a repo-wide owner contradicts is demoted
  the same way: proposed with the superseding owner named, decided by the user.

Citations and claims:

- Cite symbols and paths, never line numbers of this repository; a `path:line` appears only inside an
  `owner/repo @ commit` citation, where the commit pins it (`AGENTS.md#open-source-reference-evidence`). Never promise
  that line numbers track a branch.
- No PR, issue, or SOW identifiers of this repository, no commit hashes of this repository, and no session labels
  (option letters, inventory ids) in a skill. A commit hash appears only inside an `owner/repo @ commit` citation of an
  out-of-repo owner.
- Write each code-dependent sentence from the symbol, not from old skill text or a quick read, and state what a
  consumer relies on rather than how the producer gets there.
- Qualify every "the code enforces" claim with the tool, the mode, and the severity (error or warning); keep enforced
  checks and hand-reviewed rules in separate lists.
- Keep the permissive half of a rule: a MAY clause is a rule, inventoried as its own row and mapped separately from
  its prohibition.
- A documented command runs as written: repository-root paths, where to run it from, the interpreter or environment
  it needs. A launcher's internal path resolution is a separate fact.
- No hard-coded aggregates, counts, or percentages that regeneration or the next slim changes; give the recompute
  command.
- A relocated fact adopts the receiving document's existing convention (requirement words, mood) and names it; a
  developer document written in lowercase indicative keeps that register. This never removes requirement force from
  an AI-facing artifact.
- Tests never pin skill prose.

Structure and routing:

- Every file is routed from `SKILL.md`; a router or index lists only files that exist, checked by hand (a bare
  filename is not a path the audit resolves); no stubs.
- When a skill serves more than one audience, `SKILL.md` routes by audience and a topic file SHOULD serve one. Split
  into two skills only when the audiences differ and the seam cuts no shared step and no facts that change together;
  otherwise one skill with a router.
- The required public catalog shape is owned by `AGENTS.md#project-skills`; capture timing and authorization by
  `AGENTS.md#knowledge-capture`. In a runtime skill an index file in any subdirectory (`how-tos/INDEX.md`,
  `recipes/INDEX.md`) is optional: keep it when `SKILL.md` routes it and every listed file exists, otherwise list the
  files in `SKILL.md`.
- A peer skill gets one routing sentence. A one-way prerequisite MAY be declared (an entry-point skill, a
  read-this-first skill).
- Two skills MUST NOT require each other. When two skills claim one directory, each carries one boundary line.
- The trigger (the frontmatter description) enumerates the phrases users type and carries "not for" clauses toward
  neighbouring skills; the `AGENTS.md` index entry matches it, including every producer, subcommand, or binary name.
- Recommended skeleton, each element with the skill that shows it: frontmatter trigger (all); an owners block with one
  row per owner and anchors where a section is meant (`topology-authoring` as a table, `collectors-snmp-trap-profiles`
  as anchored bullets); a task router (`collectors-prometheus-profiles`); a rule sheet split into what the code
  enforces and rules with no code owner (`collectors-prometheus-profiles`, `topology-authoring`); a workflow section
  (`collectors-prometheus-profiles`, `topology-authoring`) or ordered required-checks lists (`collectors-snmp-profiles`,
  `collectors-snmp-trap-profiles`); validation commands (`collectors-snmp-profiles`, `collectors-snmp-trap-profiles`);
  a how-to list (`collectors-prometheus-profiles`, `topology-authoring`); a one-file rule sheet when there is one
  audience (`collectors-snmp-profiles`, `collectors-snmp-trap-profiles`).

## Creating A Skill

1. Resolve the runtime-vs-public fork before the first file exists (`AGENTS.md#project-skills` has the criteria and
   the audience boundary); a public skill also gets its relative symlink.
2. Pick the area (`.agents/skills/README.md#areas`); name the directory `<area>-<topic>` and set frontmatter `name` to
   the directory name (`.agents/skills/README.md#naming`).
3. Write the trigger with the phrases users type and the "not for" clauses; write `SKILL.md` on the skeleton above; add
   a supporting file only when `SKILL.md` routes it.
4. If the skill writes output, add its row to the table in `AGENTS.md#local-only-working-directory`.
5. Add the index entry under its area in `AGENTS.md#project-skills`, in the bullet form the existing entries use, and,
   when the skill serves two areas, the cross-reference `.agents/skills/README.md#finding-a-skill` asks for.
6. Run the audit and close per `./change-method.md#close`, then one review round with the correctness lens of
   `./change-method.md#review-round` plus a trigger-coverage check: the description names a phrase a user would type
   for every task the router lists, and the index entry names every producer, subcommand, or binary the skill covers.
   A creation has no inventory, so no preservation map and no preservation lens.

## Rot Signals

Run this over a skill periodically and before extending it (an owner document is judged only by being hand-maintained
and current). A cluster of hits is a slim; a single hit is a one-line fix in the same change.

- Line-number citations into this repository; PR, commit, issue, or SOW identifiers; a promise that line numbers track
  a branch. Grep for them; the audit automates only its legacy SOW-identifier check.
- A code-side document now exists for facts the skill states.
- "Authoritative design", "Spec -", Status, Migration Notes, Schema Additions Required, phase history, open review
  questions.
- The same fact in two or more files; transcribed enum members, finding codes, schema fields.
- Unqualified "the loader enforces", "validates", "rejects".
- An index nothing routes, or one listing files that do not exist; stubs; dead paths; two skills requiring each other.
- Hard-coded counts or percentages.
- Index entry and frontmatter disagree; a new producer, subcommand, or flag the trigger does not name.
- No task router: every task opens most of the skill; `SKILL.md` narrative paid on every trigger.
- Audit anchor failures after an owner document edit; a relative link or bare filename that no longer resolves
  (`./change-method.md#recorded-findings-and-owners` says what is verified by hand).
- A generated page or symlink in an authority list.
- An owner section named in prose instead of cited by anchor.

## Maintaining This Skill

Nothing feeds this skill but skill work. A lesson or rot signal learned while changing any skill lands here in the
same SOW, and that SOW's Artifact Maintenance Gate says so. When the method itself changes, a fresh-context agent runs
the method from this skill, `AGENTS.md`, and the README alone; what it misses is the under-specified line, fixed in the
same SOW.
