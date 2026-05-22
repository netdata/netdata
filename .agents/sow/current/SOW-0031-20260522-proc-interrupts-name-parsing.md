# SOW-0031 - Proc Interrupts Name Parsing

## Status

Status: in-progress

Sub-state: investigating and implementing a minimal fix for interrupt dimension names that currently lose name tokens.

## Requirements

### Purpose

Ensure `proc.plugin` keeps meaningful interrupt names when `/proc/interrupts` action names contain `:` or spaces.

### User Request

Fix `/proc/interrupts` parsing so interrupt dimension names reflect the real interrupt/action name instead of only the last token (e.g. not just `00.0_240` or `io5_250`).

### Assistant Understanding

Facts:

- `proc_interrupts.c` opens `/proc/interrupts` with separators `" \t:"`, so `:` in names is split into multiple words.
- The current code builds the display name from only `procfile_lineword(..., words - 1)`, which drops preceding words when names contain spaces.

Inferences:

- The bug is parser/tokenization-related, not charting-related.
- A minimal safe fix should preserve existing CPU counter parsing while improving extracted name tokens.

Unknowns:

- None blocking implementation.

### Acceptance Criteria

- Numeric interrupt dimension names include all trailing name words after CPU columns instead of only the last token.
- Colons in trailing interrupt names are preserved (not split away by procfile separators).
- Existing interrupt counter collection behavior remains unchanged.

## Analysis

Sources checked:

- `.agents/skills/project-writing-collectors/SKILL.md`
- `src/collectors/proc.plugin/proc_interrupts.c`
- `src/libnetdata/procfile/procfile.h`
- `src/libnetdata/procfile/README.md`

Current state:

- Name extraction uses only the final parsed word for numeric interrupts.
- Separator set includes colon, which destroys colon-containing names before extraction.

Risks:

- If name-start heuristics are wrong, dimension names might include extra non-name metadata tokens.
- Changing separators may alter token counts; CPU-column parsing must remain stable.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Root cause 1: `procfile_open(..., " \t:", ...)` tokenizes colon characters globally, so name fragments like `pci:0000:86:00.0` are reduced to trailing pieces.
- Root cause 2: the collector then uses only the last token (`words - 1`) as the interrupt name, which truncates multi-word names.

Evidence reviewed:

- `src/collectors/proc.plugin/proc_interrupts.c` lines around separator setup and `words - 1` usage.
- Procfile tokenization API in `src/libnetdata/procfile/procfile.h` and parser behavior notes in `src/libnetdata/procfile/README.md`.

Affected contracts and surfaces:

- `proc.plugin` interrupt chart dimension names in Netdata dashboards/APIs.
- No schema/API contract changes.

Existing patterns to reuse:

- Existing fixed-size `irr->name` handling and truncation logic in `proc_interrupts.c`.

Risk and blast radius:

- Limited to Linux `proc.plugin` interrupt dimension naming.
- No expected impact to metric values or chart creation flow.

Sensitive data handling plan:

- Use only generic synthetic examples from issue text.
- Do not include customer data, endpoints, or secrets in artifacts.

Implementation plan:

1. Update `/proc/interrupts` separator set to avoid splitting on `:` while keeping CPU counters parsed by whitespace.
2. Replace last-token-only naming with join of trailing tokens so names with spaces are preserved.
3. Keep existing length limits and `_id` suffix behavior.

Validation plan:

- Run targeted grep/manual checks on changed parsing logic.
- Run a targeted compile/build check for the affected C source (or document environment blocker).
- Execute parallel validation (Code Review + CodeQL).

Artifact impact plan:

- AGENTS.md: no update needed (no workflow/policy change).
- Runtime project skills: no update needed (no new collector authoring pattern discovered).
- Specs: no update needed (bug fix within existing behavior contract).
- End-user/operator docs: no update needed (no config/interface change).
- End-user/operator skills: no update needed.
- SOW lifecycle: keep this SOW in `current/` with `in-progress` until validation is complete.

Open-source reference evidence:

- External mirrored OSS references were not required; root cause is directly visible in this repository code.

Open decisions:

- None.

## Implications And Decisions

No user decision required; applying minimal parser behavior fix.

## Plan

1. Modify proc interrupt name parsing to preserve full trailing names and colons.
2. Validate changed behavior with targeted checks and compile validation.
3. Run automated review/security validation and finalize.

## Execution Log

### 2026-05-22

- Created SOW and documented root cause and implementation/validation plan.
- Updated `src/collectors/proc.plugin/proc_interrupts.c` to:
  - stop splitting `/proc/interrupts` tokens on `:`
  - join trailing interrupt name tokens with `_` instead of taking only the last token
  - skip a leading IRQ-type descriptor token (e.g. `*-edge`) when present before the name
- Ran CI-log triage with GitHub Actions MCP:
  - listed recent runs
  - fetched failed job logs for run `26278726985` (`Docs Broken Link Check`) and confirmed it is unrelated to this proc interrupt parser change
  - fetched failed-job logs for this branch build run `26279548573` and confirmed no failed jobs were present (`action_required` state)

## Validation

Acceptance criteria evidence:

- Numeric interrupt names are no longer derived from only the last token:
  - logic now joins trailing tokens (`for w = name_start; ...`) in `src/collectors/proc.plugin/proc_interrupts.c`.
- Colons are preserved in names:
  - `/proc/interrupts` procfile separator changed from `" \t:"` to `" \t"` in the same file.
- Existing CPU counter parsing path remains unchanged:
  - per-CPU value parsing loop over `procfile_lineword(..., c + 1)` is unchanged.

Tests or equivalent validation:

- `cmake -S . -B /tmp/netdata-build -DCMAKE_BUILD_TYPE=RelWithDebInfo` (failed before implementation due environment Go version below required minimum: found `1.24.13`, required `1.26.0`).
- `cmake -S . -B /tmp/netdata-build ... -DENABLE_PLUGIN_GO=OFF ...` (failed due missing system dependency `libelf` in sandbox).
- `git diff --check` (passed after changes).
- Manual parser verification with representative sample lines (Python simulation of updated logic):
  - `mlx5_comp40@pci:0000:86:00.0_240`
  - `nvme_0_io5_250`

Real-use evidence:

- Validated parser outcome on issue-representative `/proc/interrupts` line samples using the same separator and join behavior as implemented in C code.

Reviewer findings:

- `parallel_validation` Code Review flagged boundary/clarity improvements in name-join loop.
- Addressed by tightening underscore insertion bounds and replacing repeated `strlen()` updates with bounded `strnlen()` + `memcpy()` position tracking.

Same-failure scan:

- Searched `src/collectors/proc.plugin/proc_interrupts.c` for old `words - 1` extraction usage; replaced the only affected interrupt name extraction path.

Sensitive data gate:

- No secrets, customer data, private endpoints, or tokens were added to durable artifacts.

Artifact maintenance gate:

- AGENTS.md: no update needed (workflow/policy unchanged).
- Runtime project skills: no update needed (no new cross-task collector authoring rule discovered).
- Specs: no update needed (bug fix preserves intended collector behavior).
- End-user/operator docs: no update needed (no user-facing configuration/API change).
- End-user/operator skills: no update needed.
- SOW lifecycle: still `in-progress` in `current/` until final close decision.

Specs update:

- No spec update needed; this is a parser correctness fix to align behavior with existing intent.

Project skills update:

- No project skill update needed; no new durable process guidance was discovered.

End-user/operator docs update:

- No end-user/operator doc update needed; no configuration or external interface changed.

End-user/operator skills update:

- No end-user/operator skill updates needed; skill behavior/contracts unaffected.

Lessons:

- For `/proc/interrupts`, choosing procfile separators directly affects semantic fidelity of interrupt names; avoid splitting on `:` when names can contain PCI-style addresses.

Follow-up mapping:

- Implemented in this SOW; no deferred follow-up items.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
