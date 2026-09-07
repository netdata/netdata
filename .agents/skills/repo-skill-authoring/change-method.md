# Changing A Skill

The method for editing, slimming, splitting, or restructuring an existing skill. Past slims lost rules through
inventories that hid sub-clauses and MAY halves, preservation maps with range rows, facts relocated from prose instead
of the symbol, and unqualified enforcement claims; each step below closes one of those gaps. The rules the rewrite must
satisfy are in `./SKILL.md#authoring-rules`. There is no size target: the end state is defined by those rules, and
line counts are reported, not aimed at.

Where things go: the SOW is opened first (`planning`; the evidence round is analysis, not implementation) and holds
the decisions, the recorded target, dispositions, and validation evidence; the evidence files named below, the review
reports (`review-r<n>-preservation.md`, `review-r<n>-correctness.md`; a PR bot round is `review-bot<n>.md`), and any
throwaway tooling written for the change go under `.local/audits/<subject>/`, where `<subject>` is the directory name
of the skill under change (`AGENTS.md#local-only-working-directory`).

## Evidence Round

Before proposing anything, run two independent read-only passes with fresh context (subagents or external
assistants; one writes `inventory.md`, the other `staleness.md`), each given the skill directory, the owner candidates
from a first sweep of `./change-method.md#recorded-findings-and-owners` (an empty list is acceptable; the passes extend
it), and the output format below, and each writing its output file before its summary turn. If a pass dies, read its
file before re-running; a dying pass has usually written it. When the passes disagree on a claim, the code decides:
re-verify it yourself; when they disagree on how to split a rule, take the finer split.

`inventory.md`, the preservation baseline, opens with a census: per-file line counts, lines over 120 columns,
provenance (`git log --follow -- <path>`: first and last change dates and commit count; hashes belong in this local
evidence, never in the skill). A maintained design record and an abandoned one get different options. Then:

- one row per rule or hard-won fact, with a stable id per file (`<file>-R<n>` in document order, the stem
  directory-qualified when basenames repeat); narrative and examples are counted in the classification only, and
  dropping them needs no row; a compound bullet is split into sub-clauses, one row each;
- the permissive half of a rule (MAY) is its own row, separate from its prohibition;
- a closed list (enum, token vocabulary) is one row naming its members and their count;
- a kind per row: MUST, MUST NOT, SHOULD, MAY, fact, enum, pointer, procedure, history, rationale, example, status;
- aggregate sections: line classification as per-file line counts per bucket (rules; hard-won facts, stated nowhere
  else; restatement, stated in a named owner; narrative; examples), internal duplication, ownership overlap with
  code-side documents, cross-skill overlap, task clusters (which files each task opens), reachability (what `SKILL.md`
  routes), structural facts (external references, CI coupling, audit coupling);
- the reported row count equals the enumerated ids; fix a mismatch before the map is written.

`staleness.md`, truth against the code:

- every checkable claim labelled FALSE, STALE, DEAD, TRUE-HIGH-VALUE (stated nowhere else), TRUE-DERIVABLE (the reader
  can get it from the owner), or UNVERIFIABLE (say why);
- a Tier-1 list: claims that would cause wrong work today;
- code facts the skill lacks; an authority check (every named authority exists and is hand-maintained); a keep list
  with its own stable ids (`K<n>`), mapped to inventory ids in the preservation map.
- When an owner lives in another repository and its mirror is absent on the machine, its claims are UNVERIFIABLE and
  whether to touch them at all is a numbered user decision in the options round; a verified out-of-repo fact is
  labelled `owner/repo @ commit`.

Then re-verify every Tier-1 item yourself against the code before presenting anything.

## Recorded Findings And Owners

- Collect the findings already recorded against the skill and triage each before proposing scope: fixed here, or
  rejected with evidence; close the entry. Stores: GitHub issues (`gh issue list --search "<skill name>"`,
  `AGENTS.md#followup-discipline`), the local stores `.local/sow/` and `.local/audits/<subject>/followups.md`, past PR
  bot reviews (fetched as `repo-pr-reviews` describes), and a tree-wide grep for the skill's name (sibling skills carry
  findings in prose).
- Owner search: the candidate owners and the generated-file check in `./SKILL.md#authoring-rules`; the Learn status of
  each candidate via `docs/.map/map.yaml` decides the marker paragraph
  (`.agents/skills/README.md#owner-section-citations`; repository instruction files such as the root `AGENTS.md` and
  the skills README carry the citing sentence inline instead); an owner in another repository is read from the local
  mirror and cited as `./SKILL.md#authoring-rules` says.
- Reference search for every file you may rename or delete (`AGENTS.md#clean-end-state-over-less-churn`): root
  `AGENTS.md`, sibling skills, `src/**/AGENTS.md`, developer documents, CI path filters and test discovery, code-tree
  README links, tests, audit globs. The skills-layout section of `.agents/sow/audit.sh` says which path and anchor
  forms it resolves; everything else is verified by hand, in particular links without an anchor (`./`, sibling, and
  subdirectory forms), bare filenames, path strings under `docs/netdata-ai/skills/`, and links from files outside a
  skill.

## Options Round

Record the clean-end-state target first (`AGENTS.md#clean-end-state-over-less-churn`), then present numbered options
with a recommendation (`AGENTS.md#working-with-the-user`), one numbered decision per fork: scope (slim in place, split
per the test in `./SKILL.md#authoring-rules`, keep); each keep-list cluster with no owner; each file deletion (the
approval `AGENTS.md#git-and-pr-workflow` requires, staged later with `git rm <path>`); each self-declared rule to
demote; each unverifiable out-of-repo cluster. No step plan before the evidence round. The user decides scope and
design.

## Rewrite And Preservation Map

Write to the authoring rules. Keep `preservation-map.md` up to date as you write:

- one row per inventory id with a disposition: KEPT (where), CORRECTED (what was wrong, at which symbol), OWNER (the
  `<path>.md#<anchor>` the skill now cites), DROPPED (the reason: false against the code at a named symbol, superseded
  by a named owner, or a numbered user decision);
- a range row only when every id in the range was opened, and never for a disposition that claims a pre-existing
  owner: leaks found in past review rounds hid inside such ranges;
- every keep-list item maps to the section that now holds it.

Relocating a fact into a shipped document follows the citation and register rules in `./SKILL.md#authoring-rules`. A
coupled fix in an owner document (a dangling reference, a one-word correction) is included when low-risk and
disclosed.

## Review Round

One reviewer per lens, fresh context, full scope, the working tree as it is at round start, one proposed fix per
finding, CRITICAL vs TODO. The blocker bar, verify-before-acting, the one-round default, and the recurrence guard are
`AGENTS.md#review`.

- Preservation lens: receives `inventory.md`, the keep list, and `preservation-map.md`; checks that every row and keep
  item is reachable from the new skill or the cited owner section; flags dropped permissive clauses and range rows.
- Correctness lens: receives the new skill text, the code, and the owner documents, never the pre-change skill text;
  checks every fact at its symbol, every enforcement qualifier, every command as written, every anchor; marks claims
  whose owner is unavailable on the machine as unverifiable.

Probe every fix wrong-to-right and on the normal case (for prose: run the documented commands and re-resolve the
cited anchors and paths); hold user edit requests until the round closes, or tell the reviewers to review the tree as
it is. A PR bot review is triaged the same way: fix real issues, add no machinery for unlikely edge cases; the
mechanics are in `repo-pr-reviews`.

Recurrence seen in skills: findings clustering in relocated prose written from documents, and in descriptions of what
a tool does. The class fix is contract-only text with a symbol per sentence, or a pointer at the tool, not another
round of patches.

## Mechanical Hygiene

The width and reflow-commit rules are `AGENTS.md#durable-ai-facing-artifact-formatting`: a rewritten paragraph is
wrapped in its content commit; surviving prose at another width is rewrapped in a separate whitespace-only commit. The
tooling rules that have bitten, for the throwaway scripts:

- rewrap whole paragraphs with fence-, table-, and list-aware logic; a line starting with a dash and a space (a
  bullet) begins a paragraph;
- assert that the whitespace-split token stream is unchanged after every rewrap; never auto-join short lines;
- count characters, not bytes: BSD `awk length` counts bytes, so use Python; re-run the width check on every touched
  file, including files outside the skill. From the repository root, after staging new files (it checks every
  changed markdown file and reports every non-table line; judge fenced and frontmatter hits by the owner rule instead
  of rewrapping them):

  ```bash
  python3 -c 'import sys
  for f in sys.argv[1:]:
      for n, l in enumerate(open(f), 1):
          if len(l.rstrip("\n")) > 120 and not l.startswith("|"): print(f"{f}:{n}")' $(git diff --name-only --diff-filter=d HEAD -- '*.md')
  ```

- never write an HTML comment opener literally in prose or a code span inside a skill: the headings after it stop
  being scanned (`.agents/skills/README.md#owner-section-citations`);
- `git diff HEAD` before each commit;
- on a sensitive-scan hit, use the bracketed placeholders `AGENTS.md#sensitive-data-in-durable-artifacts` prescribes
  and phrase around the keyword; `.agents/sow/scan-sensitive.sh` defines the hits.

## Close

- `bash .agents/sow/audit.sh` before every commit and at close, after `git add` of every new file (its citation and
  path scans read tracked files); nothing in CI replaces it (`.github/workflows/sow.yml` is the PR gate).
- Line counts before and after per touched file; a symbol sweep (grep the tree for every backticked identifier and
  path the skill cites); the reference search re-run for every file or heading removed or renamed; the slimming pass
  over every touched skill file (`AGENTS.md#project-skills`).
- Follow-ups tracked and the artifact gate recorded (`AGENTS.md#followup-discipline`,
  `AGENTS.md#artifact-maintenance-gate`); a lesson about the method goes into `./SKILL.md#maintaining-this-skill`.
