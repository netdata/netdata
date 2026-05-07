# SOW-0013 - Add 'nfds' Monitoring and Fix FD Bug in apps.plugin

## Status

Status: in-progress

Sub-state: Researching implementation details and repository state.

## Requirements

### Purpose

To align Netdata's process monitoring with features recently added to the `prmon` (Process Monitor) tool, specifically the 'nfds' (number of file descriptors) metric, and to fix a confirmed bug in the reporting of pipe file descriptors.

### User Request

"bu repoya pr açacağım geliştirmr yap pushla ve bana burdan pr açıklamsını ver" (I will open a PR to this repo, do development, push, and give me the PR description here).

### Assistant Understanding

Facts:

- The user recently added 'nfds' monitoring to the `prmon` repository (PR #311).
- Netdata's `apps.plugin` already collects file descriptor counts but does not report a single "total" metric in its primary charts.
- `apps_output.c` contains a copy-paste bug where the `pipes` dimension incorrectly uses the `sockets` count.
- The local `netdata` git repository appears to be in a corrupted state (`fatal: bad object HEAD`).

Inferences:

- The user wants a similar "Total FDs" metric in Netdata's application-level monitoring.
- The user expects the changes to be pushed to their fork (`arch-yunus/netdata`).

Unknowns:

- The exact chart where the user wants the 'nfds' metric to appear (e.g., as a dimension in `apps.processes` or a new summary chart).
- The cause of the git repository corruption and whether it will block the "push" requirement.

### Acceptance Criteria

- Fix the bug in `apps_output.c` (line 236) where `pipes` was using `sockets` count.
- Add a "Total FDs" (nfds) dimension to `apps.plugin` output.
- Generate a professional Pull Request description in English.
- (Conditional) Push the changes to the remote repository.

## Analysis

Sources checked:

- `src/collectors/apps.plugin/apps_plugin.h`
- `src/collectors/apps.plugin/apps_output.c`
- `src/collectors/apps.plugin/apps_os_linux.c`
- `HSF/prmon` PR #311 (via browser)

Current state:

- `apps_output.c:236` incorrectly uses `w->openfds.sockets` for the `pipes` dimension.
- `apps.plugin` calculates `pid_openfds_sum(p)` but only uses it for limits checking, not for chart dimensions.

Risks:

- Performance impact: scanning `/proc/*/fd` is already done, so adding a sum is cheap. However, if the user expects this system-wide without `apps.plugin` overhead, it might be different.
- Git issues: If the repo remains corrupted, pushing will fail.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The `pipes` bug is a simple typo in `apps_output.c`.
- The 'nfds' requirement is a feature parity request based on the user's work in another project (`prmon`).

Evidence reviewed:

- `apps_output.c` source code.
- `HSF/prmon @ main` PR #311 files.

Affected contracts and surfaces:

- `apps.plugin` output (stdout protocol).
- Netdata dashboard (new dimension/chart).

Existing patterns to reuse:

- `apps.plugin` dimension reporting pattern using `send_SET`.
- `pid_openfds_sum` macro in `apps_plugin.h`.

Risk and blast radius:

- Minimal. Changes are restricted to `apps.plugin` output logic.

Sensitive data handling plan:

- No sensitive data involved. All metrics are standard process stats.

Implementation plan:

1. Fix the typo in `apps_output.c`.
2. Update `apps_output.c` to include a total FDs count in the `fds_open` chart or as a new chart.
3. Verify the output using a dry-run or manual check of the plugin execution.
4. Prepare the PR description.

Validation plan:

- Run `apps.plugin` manually and check the `BEGIN/SET/END` blocks on stdout.
- Verify that `pipes` now has its own value.
- Verify that `total` (or `nfds`) matches the sum of individual types.

Artifact impact plan:

- AGENTS.md: Unaffected.
- Runtime project skills: Unaffected.
- Specs: Unaffected.
- End-user/operator docs: May need update if new chart is added.
- End-user/operator skills: Unaffected.
- SOW lifecycle: Standard.

Open-source reference evidence:

- `HSF/prmon @ 27f1410` (PR #311) - Adding nfds to countmon.

Open decisions:

- Should we add 'total' as a dimension in the existing `fds_open` chart (which is stacked) or as a separate chart? (Recommended: Add as a new dimension `total` in a new chart `apps.fds_total` to avoid doubling the stacked chart total).

## Implications And Decisions

1. **Bug Fix**: I will fix the `pipes` reporting bug immediately.
2. **Metric Name**: I will use `total` or `fds` as the dimension name, but mentioned it as `nfds` in the PR description for clarity with the user's other work.
3. **Git Fix**: I will attempt to `git pull --rebase` or `git fetch` to recover the repo before pushing.

## Plan

1. Fix typo in `apps_output.c`.
2. Add total FDs aggregation and reporting to `apps_output.c`.
3. Verify output.
4. Push and generate PR description.
