# Authoring Rules And Source Ownership

Use this reference for the affected authoring or review question. `SKILL.md` chooses the workflow;
`change-method.md` owns implementation, evidence and review mechanics.

## Where The Rules Live

Owner sections, cited by anchor (`.agents/skills/README.md#owner-section-citations`). Read the row for the obligation it
names.

| Owner section | What you must get from it |
|---|---|
| `AGENTS.md#project-skills` | where runtime skills live; the same-PR rule for gap-closing and pointer-fixing updates; the slimming pass every skill change ends with and what it reports; the public-skill convention (audience boundary, symlinks, script shape, token safety, the live how-tos catalog); output/reference skill trees and their no-rename rule; the grouped skills index |
| `AGENTS.md#durable-ai-facing-artifact-formatting` | retrieval structure, requirement words next to the action, prose width, reflow-only commits |
| `AGENTS.md#sensitive-data-in-durable-artifacts`, `.agents/sensitive-data-discipline.md#allowed-alternatives` | the public-artifact assumption and the sanitized-evidence requirement; the `<repo>/`-prefixed path form for prose (an anchor citation stays repo-relative without it) |
| `AGENTS.md#enforcement` | what `.agents/sow/audit.sh` hard-fails on and what the PR gate `.github/workflows/sow.yml` re-checks |
| `AGENTS.md#local-only-working-directory` | where skill-change evidence goes and how its `<subject>` is named |
| `AGENTS.md#clean-end-state-over-less-churn` | the target is recorded before options are generated; the disclosure of what is removed and what is excluded; the reference search when a path is replaced; how coupled cleanup is handled |
| `AGENTS.md#working-with-the-user` | the user-decision format and when a decision is recorded |
| `AGENTS.md#review` | review scope, reproduction evidence, authorized checkpoints, repeat and stop conditions |
| `AGENTS.md#knowledge-capture` | discovery notes, documentation authorization, and coupled skill maintenance |
| `AGENTS.md#when-a-sow-is-required`, `AGENTS.md#followup-discipline`, `AGENTS.md#validation-gate`, `AGENTS.md#artifact-maintenance-gate` | implementation classification and its SOW exemptions; how deferred items are tracked; what Validation must hold and what every close records |
| `AGENTS.md#git-and-pr-workflow` | staging, and which git actions need explicit approval (deleting a file among them) |
| `AGENTS.md#open-source-reference-evidence` | the citation form for an owner outside this repository |
| `.agents/skills/README.md#skills`, `.agents/skills/README.md#naming`, `.agents/skills/README.md#areas` | the no-nesting rule; the name form; the area minting rule with its same-change obligation |
| `.agents/skills/README.md#owner-section-citations` | the point-rather-than-restate rule and the anchor form (a MUST) with its slug rules; the recommended marker paragraph for a private owner document, none for a Learn-published one |
| `.agents/skills/README.md#finding-a-skill` | the index is the map; the frontmatter description is the trigger; the cross-reference for a skill serving two areas |

## Authoring Rules

Apply the following rules within the task's scope and authorization. Requirements are MUST unless marked SHOULD or MAY.

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
- For an external owner, use the requested or otherwise relevant revision and the citation form in
  `AGENTS.md#open-source-reference-evidence`. Read existing source via
  `.agents/skills/repo-mirror-sources/SKILL.md#inspect-existing-source`; neither default branch nor latest upstream is
  automatically the evidence target. A dirty checkout can supply unchanged committed blobs via `git show`; never
  attribute edited contents to an unmodified commit. Acquire or refresh sources only when needed and authorized.
  If source is inaccessible, preserve the useful claim with its verifying revision or a specific uncertainty; do not
  silently delete it or claim verification. An external citation is not validated by this repository's anchor audit.
- One owner per fact. When a fact moves into its owner, replace duplicate rule text with pointers in the same change
  and record the owner section. File deletion still follows `AGENTS.md#git-and-pr-workflow`.
- A pointer row carries one clause naming the subject; copying the requirement text creates a second owner.
- Keep valid knowledge in its existing home when no other owner exists. Routine placement and factual corrections
  within the approved target do not create an approval gate. Ask only when ownership placement creates a genuine
  scope, audience, design, or destructive fork (`AGENTS.md#plan-before-non-trivial-work`); do not create a second owner.
- Developer procedure stays in the skill; a shipped operator document keeps only the operator form.
- Do not transcribe schemas, enum members, finding codes, or code; the reader opens the file.
- Design records rot once the code ships its own document. Propose retiring them, with the relocation target and the
  rejected alternatives; deletion is the user's call; history stays in git. Unshipped design that must survive goes
  to the owning subsystem's scope or architecture document, labelled as not shipped, or stays in its current home
  until placement is resolved. Correct a contradicted skill rule against its governing owner within authorized scope;
  a real change to purpose, public behavior, or approved requirement remains a user decision.

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
- Keep the permissive half of a rule. For preservation inventories, a MAY clause has its own row and disposition,
  separate from its prohibition; a bounded amendment still checks the affected exception even without a full map.
- A documented command runs as written: repository-root paths, where to run it from, the interpreter or environment
  it needs. A launcher's internal path resolution is a separate fact.
- No hard-coded aggregates, counts, or percentages that regeneration or the next slim changes; give the recompute
  command.
- A relocated fact adopts the receiving document's existing convention (requirement words, mood) and names it; a
  developer document written in lowercase indicative keeps that register. This never removes requirement force from
  an AI-facing artifact.
- Tests never pin skill prose.

Structure and routing:

- Every supporting file is reachable from `SKILL.md`, directly or through a routed reference/index. List only real
  files; check unanchored and bare-filename links explicitly. Do not create placeholder references for future files.
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
- The description names discriminative tasks and representative user phrases, with useful near-miss boundaries.
  Put important review and operation triggers early. Keep exhaustive producer, command and field mappings inside the
  task router or references; the root index is a concise pointer, not another full trigger inventory.
- Invocation and review reference selection follow `AGENTS.md#skill-selection`; route the relevant contracts without
  duplicating that policy or forking domain rules by reviewer lens.
- Recommended skeleton, each element with the skill that shows it: frontmatter trigger (all); an owners block with one
  row per owner and anchors where a section is meant (`topology-authoring` as a table, `collectors-snmp-trap-profiles`
  as anchored bullets); a task router (`collectors-prometheus-profiles`); a rule sheet distinguishing what the code
  enforces from rules with no code owner (`collectors-prometheus-profiles`, `topology-authoring`); a workflow section
  (`collectors-prometheus-profiles`, `topology-authoring`) or ordered required-checks lists (`collectors-snmp-profiles`,
  `collectors-snmp-trap-profiles`); validation commands (`collectors-snmp-profiles`, `collectors-snmp-trap-profiles`);
  a how-to list (`collectors-prometheus-profiles`, `topology-authoring`); a one-file rule sheet when there is one
  audience (`collectors-snmp-profiles`, `collectors-snmp-trap-profiles`).

## Rot Signals

Use these signals in the requested scope and when checking touched content. They are investigation prompts, not proof
of rot or a mandatory full-skill rewrite. A local duplicate or one stale claim can use the bounded amendment path;
structural loss risk uses `./change-method.md#restructure-evidence`. An owner document is judged by its authority and
current applicability, not by whether it follows a skill skeleton.

- Line-number citations into this repository; PR, commit, issue, or SOW identifiers; a promise that line numbers track
  a branch. Grep for them; the audit automates only its legacy SOW-identifier check.
- A code-side document now exists for facts the skill states.
- "Authoritative design", "Spec -", Status, Migration Notes, Schema Additions Required, phase history, open review
  questions.
- The same fact in two or more files; transcribed enum members, finding codes, schema fields.
- Unqualified "the loader enforces", "validates", "rejects".
- An index nothing routes, or one listing files that do not exist; stubs; dead paths; two skills requiring each other.
- Hard-coded counts or percentages.
- Index, description and actual task coverage disagree; important tasks cannot be discovered or unrelated work triggers
  the skill. A new flag alone does not require expanding the global description.
- No task router: every task opens most of the skill; `SKILL.md` narrative paid on every trigger.
- Audit anchor failures after an owner document edit; a relative link or bare filename that no longer resolves
  (`./change-method.md#recorded-findings-and-owners` says what is verified by hand).
- A generated page used as its own authority. Resolve a symlink to its target: a public skill alias to hand-maintained
  content is a valid route; a symlink into generated output is not the source owner.
- An owner section named in prose instead of cited by anchor.
