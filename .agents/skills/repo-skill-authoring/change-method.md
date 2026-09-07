# Changing A Skill

The method for editing, slimming, splitting, or restructuring an existing skill. It exists because past slims each
lost rules through the same gaps: inventories that hid sub-clauses and MAY halves, preservation maps with range rows,
facts relocated from prose instead of from the symbol, and enforcement claims nobody qualified. Every step below
closes one of those gaps. Evidence lives under `.local/audits/<subject>/` (`AGENTS.md#local-only-working-directory`);
throwaway tooling written for a change (width check, rewrap, symbol sweep) lives there too. The rules the rewrite must
satisfy are in `SKILL.md` "Authoring Rules". There is no size target: the end state is defined by those rules, and
line counts are reported, not aimed at.

## Evidence Round

Before proposing anything, run two independent read-only passes with fresh context (subagents or external
assistants), each given the skill directory, the owner candidates found so far, and the output format below, and each
writing its output file before its summary turn. If a pass dies, read its file before re-running; it usually finished
the file first.

`inventory.md`, the preservation baseline, opens with a census: per-file line counts, lines over 120 columns,
provenance (`git log --follow -- <path>`). A maintained design record and an abandoned one get different options.
Then:

- one row per rule with a stable id per file; a compound bullet is split into sub-clauses, one row each;
- the permissive half of a rule (MAY) is its own row, separate from its prohibition;
- a closed list (enum, token vocabulary) is one row naming its members and their count;
- a kind per row: MUST, MUST NOT, SHOULD, MAY, fact, enum, pointer, procedure, history, rationale, example, status;
- aggregate sections: line classification (rules, hard-won facts, restatement, narrative, examples), internal
  duplication, ownership overlap with code-side documents, cross-skill overlap, task clusters (which files each task
  opens), reachability (what `SKILL.md` routes), structural facts (external references, CI coupling, audit coupling);
- the reported row count equals the enumerated ids; fix a mismatch before the map is written.

`staleness.md`, truth against the code:

- every checkable claim labelled FALSE, STALE, DEAD, TRUE-HIGH-VALUE (stated nowhere else), TRUE-DERIVABLE (the reader
  can get it from the owner), or UNVERIFIABLE (say why);
- a Tier-1 list: claims that would cause wrong work today;
- code facts the skill lacks; an authority check (every named authority exists and is hand-maintained); a keep list
  with stable ids.

Then re-verify every Tier-1 item yourself against the code before presenting anything.

## Recorded Findings And Owners

- Collect the findings already recorded against the skill (GitHub issues per `AGENTS.md#followup-discipline`, the
  local stores under `.local/`, past PR bot reviews) and triage each before proposing scope: fixed here, or rejected
  with evidence. Close the entry.
- Owner search: the candidate owners and the generated-file check in `SKILL.md` "Authoring Rules"; the Learn status of
  each candidate via `docs/.map/map.yaml` (a private owner document gets the marker paragraph of
  `.agents/skills/README.md#owner-section-citations`, a Learn-published one none); an owner in another repository is
  read from the local mirror and cited as `SKILL.md` "Authoring Rules" says.
- Reference search for every file you may rename or delete (`AGENTS.md#clean-end-state-over-less-churn`): root
  `AGENTS.md`, sibling skills, `src/**/AGENTS.md`, developer documents, CI path filters and test discovery, code-tree
  README links, tests, audit globs. The audit checks repository-relative `.agents/skills/` paths in tracked files
  (`CHANGELOG.md` excluded), `../` links written as markdown links or `source` lines inside skill files, and anchor
  citations inside skill files. Everything else is verified by hand: `./`, sibling, and subdirectory links, bare
  filenames, `docs/netdata-ai/skills/` paths named outside a skill file, and the hop depth of a relative link from a
  non-skill file.

## Options Round

Record the clean-end-state target first (`AGENTS.md#clean-end-state-over-less-churn`), then present numbered options
with a recommendation (`AGENTS.md#working-with-the-user`): slim in place, split, keep; and, for every keep-list fact
with no owner, the fork in `SKILL.md` "Authoring Rules". No step plan before the evidence round. The user decides
scope and design.

## Rewrite And Preservation Map

Write to the authoring rules. Keep `preservation-map.md` up to date as you write:

- one row per inventory id with a disposition: KEPT (where), CORRECTED (what was wrong, at which symbol), OWNER (the
  `<path>.md#<anchor>` the skill now cites), DROPPED (the reason: false against the code at a named symbol, superseded
  by a named owner, or a numbered user decision);
- a range row only when every id in the range was opened, and never for a disposition that claims a pre-existing
  owner: every leak the topology slim's review rounds found hid inside such a range;
- every keep-list item maps to the section that now holds it.

Relocating a fact into a shipped document follows the citation and register rules in `SKILL.md` "Authoring Rules". A
coupled fix in an owner document (a dangling reference, a one-word correction) is included when low-risk and
disclosed.

## Review Round

One reviewer per lens, fresh context, full scope, the working tree as it is at round start, one proposed fix per
finding, CRITICAL vs TODO. The blocker bar, verify-before-acting, the one-round default, and the recurrence guard are
`AGENTS.md#review`.

- Preservation lens: receives `inventory.md`, the keep list, and `preservation-map.md`; checks that every row and keep
  item is reachable from the new skill or the cited owner section; flags dropped permissive clauses and range rows.
- Correctness lens: receives the code and the owner documents, never the old skill text; checks every fact at its
  symbol, every enforcement qualifier, every command as written, every anchor.

Verify every finding against the code before acting; probe every fix wrong-to-right and on the normal case; hold user
edit requests until the round closes, or tell the reviewers to review the tree as it is. A PR bot review is triaged
the same way: fix real issues, add no machinery for unlikely edge cases; the mechanics are in `repo-pr-reviews`.

Recurrence seen in skills: findings clustering in relocated prose that was written from documents. The class fix is
contract-only text with a symbol per sentence, not another round of patches.

## Mechanical Hygiene

The width and reflow-commit rules are `AGENTS.md#durable-ai-facing-artifact-formatting`. The tooling rules that have
bitten, for the throwaway scripts under `.local/audits/<subject>/`:

- rewrap whole paragraphs with fence-, table-, and list-aware logic; a line starting with `- ` begins a paragraph;
- assert that the whitespace-split token stream is unchanged after every rewrap; never auto-join short lines;
- count characters, not bytes (Python `len`, not `awk length`); re-run the width check on every touched file,
  including files outside the skill;
- `git diff HEAD` before each commit;
- the sensitive scanner treats the word community (also with an `snmp`, `ro`, `rw`, or `string` affix) followed by a
  colon or an equals sign and a value of three or more characters as an SNMP credential unless the value is a
  placeholder such as `[REDACTED_SECRET]`; phrase around it.

## Close

- `bash .agents/sow/audit.sh` before every commit and at close, after `git add` of every new file: it is the only local
  check for the items `AGENTS.md#enforcement` lists, and CI re-runs only the sensitive scan.
- Line counts before and after per touched file; a symbol sweep of every cited identifier and path; the reference
  search re-run; the slimming pass over every touched skill (`AGENTS.md#project-skills`).
- Follow-ups tracked and the artifact gate recorded (`AGENTS.md#followup-discipline`,
  `AGENTS.md#artifact-maintenance-gate`); a lesson about the method goes into `SKILL.md` "Maintaining This Skill".
