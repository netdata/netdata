# SOW-20260622-fix-pr-22781-ram-retention-docs - Fix PR #22781: ram/alloc mode retention docs

## Status

Status: in-progress

Sub-state: Rewriting CONFIGURATION.md section per ilyam8 review feedback.

## Requirements

### Purpose

Fix the documentation in PR #22781 to comply with the "Document at the User's Altitude, Not the Implementation's" standard. The current draft exposes implementation internals (ring buffers, slot counts, page alignment) that users cannot observe or act on.

### User Request

Review and fix PR #22781. Address the ilyam8 review feedback. Also incorporate the context from the merged PR #22676 (which surfaces a similar pipeline issue with implementation-level content).

### Assistant Understanding

Facts:

- PR #22781 is on branch `docs/netdata-agent-retention-config-ram-mode`, currently in draft.
- Reviewer ilyam8 flagged the content as over-explaining implementation: ring buffers, slot counts, page alignment.
- ilyam8 proposed: `Effective retention time ≈ retention × update_every`, with two examples.
- ilyam8 tagged @stelfrag to confirm the formula.
- Source code confirms the formula: `rrddim_mem.c:293` shows `metric_duration = entries × update_every_s` once the buffer is full, and `entries ≈ retention` (with minor page-alignment rounding for `ram` mode, exact for `alloc`). Formula is correct.
- PR #22676 was merged on 2026-06-19. Its changes to README.md (chart navigation, space/time percentage explanations) are in a different section from what #22781 touches (the `ram` row cross-reference). No rebase needed.
- No destructive or irreversible actions in this SOW.

Inferences:

- The page-alignment note ("`3600 becomes 4096 slots`") is implementation detail; the `≈` in the formula adequately covers this for users.
- The restart-required note and the Parent–Child note are user-actionable and must be kept.

### Acceptance Criteria

- `src/database/CONFIGURATION.md` section "RAM and ALLOC Mode Retention" contains no: "ring buffer", "slot", "entries", "page", "alignment".
- Formula `≈ retention × update_every` is present and correct per source code.
- Table has 3 columns: `retention`, `update every`, effective retention (no "Ring-buffer slots" column).
- Restart-required note preserved.
- Parent–Child note preserved.
- README.md cross-reference anchor `#ram-and-alloc-mode-retention` still resolves to the section heading.

## Analysis

Sources checked:

- `src/database/CONFIGURATION.md` (current branch state — 173 lines, section at lines 15–57)
- `src/database/ram/rrddim_mem.c:293` — formula: `metric_duration = min(counter, entries) × update_every_s`
- PR #22781 review: ilyam8 comment + suggestion
- PR #22676 diff (merged): changes different section of README.md, no conflict

Current state:

- Section violates "Document at the User's Altitude" with: ring-buffer terminology (line 17), "slot" count language (lines 28, 32, 36, 43), page-alignment note (lines 45–49), table column "Ring-buffer slots" (line 38).

Risks:

- Low: pure documentation change with no behavior modification.

## Pre-Implementation Gate

Status: ready

Problem/root-cause: Pipeline generated implementation-altitude docs. ilyam8 review identified the violation.

Evidence: Lines 17–49 of `src/database/CONFIGURATION.md` on this branch.

Clean end state: Section rewritten at user altitude. PR ready for re-review.

Removed-redundant: The page-alignment :::note block (lines 45–49), the "slot count" column in the table, the implementation explanation in lines 28–32.

Excluded-coupled: None.

Reference search: N/A — no path or contract replaced.

Implementation plan:
1. Rewrite lines 15–57 of CONFIGURATION.md.
2. Verify anchor in README.md still matches.
3. Commit and push.
4. Update PR title and description.
5. Reply to ilyam8.

Validation plan: grep for forbidden words; verify formula; check table columns; verify anchor.

Artifact impact: CONFIGURATION.md only. No spec, skill, or test changes needed.

Sensitive data: None.
