# SOW - SNMP Traps PR CI Fixes

## Status

Status: completed

Completed actionable CI/static-analysis fixes for this batch.

Remaining non-code gates:

- `no-working-files` remains intentionally failing because branch-local SOW
  working files are still present and must be removed only before final merge
  preparation.
- `WIP` remains pending because the PR is still draft/WIP.
- Long build/package matrix jobs were still pending at last check; no completed
  branch-caused build/package failure was visible.

## Requirements

### Purpose

Bring the SNMP traps PR closer to merge-ready state by fixing remaining CI and static-analysis issues caused by this branch, while leaving the intentional SOW merge guard untouched until final merge preparation.

### User Request

Fix the rest of the issues after updating the journal SDK to `v0.5.1`.

### Acceptance Criteria

- Flake8 failures caused by this branch are fixed.
- Yamllint failures caused by this branch are fixed.
- Go toolchain `go fmt` / `go fix` check passes locally or remaining diffs are explained with evidence.
- Codacy and Sonar findings caused by this branch are triaged and fixed where actionable.
- SOW `no-working-files` remains explicitly out of this batch; SOW files are not deleted locally.

## Pre-Implementation Gate

Status: in-progress

Problem/root-cause model:

- Current PR CI failures after the SDK `v0.5.1` fix are no longer package/docker Go-version failures.
- Remaining failures are lint/style/toolchain/static-analysis issues in branch-added Python tooling, YAML metadata, Go code formatting/fix output, and external analysis gates.
- The SOW `no-working-files` failure is intentional branch-local merge guarding, not a code defect, and is excluded from this batch by user direction to keep SOW files locally.

Evidence reviewed:

- `gh pr checks 22652 --repo netdata/netdata` reports failures in flake8, yamllint, Go toolchain tests, Codacy, Sonar, and SOW no-working-files.
- `.github/workflows/sow.yml` rejects SOW working files in PR head as a merge guard.
- `AGENTS.md` allows branch-local SOWs during PR work and requires removal only before merge.

Affected contracts and surfaces:

- Python generator tooling under `tools/snmp-traps-profile-gen/`.
- SNMP trap collector taxonomy YAML.
- Go source files touched by `go fmt` / `go fix`.
- Static-analysis results for PR #22652.

Clean-end-state target:

- CI-relevant lint/toolchain/static-analysis findings caused by the branch are fixed with minimal behavior change.
- SOW files remain locally available and are not part of this fix batch.
- Any final merge-only SOW cleanup remains separate.

Existing patterns to reuse:

- Existing Python style constraints from CI flake8.
- Existing YAML style enforced by CI yamllint.
- Existing Go toolchain workflow behavior: `go fmt ./...`, `go fix ./...`, then clean diff.
- Existing PR review and Codacy audit skills for finding triage.

Risk and blast radius:

- Low to medium. Lint fixes should be mechanical, but `go fix` may modify files outside the SNMP trap collector if the Go toolchain finds stale patterns.
- External analysis findings may include false positives; each finding must be verified before changing behavior.

Sensitive data handling plan:

- Use only CI job names, file paths, line numbers, rule IDs, and sanitized summaries.
- Do not write tokens, credentials, private endpoints, device identifiers, SNMP communities, or account metadata to repo artifacts.

Implementation plan:

1. Capture exact remaining CI findings.
2. Fix flake8 and yamllint issues.
3. Run and inspect Go toolchain formatting/fix output; apply required diffs only with evidence.
4. Triage Codacy and Sonar findings; fix actionable branch-caused issues.
5. Run focused validation and commit only source/lint fixes.

Validation plan:

- Re-run flake8 on affected Python files.
- Re-run yamllint on affected YAML file.
- Re-run Go toolchain commands in `src/go`.
- Re-run focused SNMP trap Go tests if Go source changes affect the collector.
- Re-check PR CI after push.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected unless a new static-analysis workflow gotcha is discovered.
- Specs: no product behavior change expected.
- End-user/operator docs and skills: no product behavior change expected.
- SOW lifecycle: keep this SOW branch-local and uncommitted unless takeover is needed.

Open decisions:

- None blocking. User explicitly excluded local SOW deletion from this batch.

## Validation

- Local Python lint:
  `.local/venv-flake8/bin/python -m flake8 tools/snmp-traps-profile-gen/*.py`
  passed.
- Local Python import/syntax:
  `.local/venv-flake8/bin/python -m compileall -q tools/snmp-traps-profile-gen`
  passed.
- Local exact pydocstyle probe for the Codacy-reported rule family:
  `.local/venv-flake8/bin/python -m pydocstyle --select D203,D212,D213 ...`
  passed after converting reported multi-line docstrings to single-line
  summaries.
- Whitespace validation: `git diff --check` passed.
- Pushed commit: `a91bcbe1a7 Fix SNMP trap profile generator docstring lint`.
- PR #22652 static/style checks at last poll:
  - Codacy Static Code Analysis: `SUCCESS`, Codacy API issue count `0`.
  - SonarCloud and SonarCloud Code Analysis: `SUCCESS`.
  - Review workflow: `flake8`, `yamllint`, `shellcheck`, `golangci-lint`
    all `SUCCESS`; `hadolint` and `actionlint` skipped by workflow logic.
  - SOW `sensitive-data`: `SUCCESS`.
  - SOW `no-working-files`: `FAILURE`, intentionally out of this batch.

## Artifact Maintenance Gate

- AGENTS.md: no update needed; workflow and guardrails unchanged.
- Runtime project skills: no update needed; no new reusable workflow beyond
  the already documented Codacy API fallback.
- Specs: no update needed; no product behavior or public contract changed.
- End-user/operator docs: no update needed; style-only generator docstring
  cleanup.
- End-user/operator skills: no update needed; no public/operator workflow
  changed.
- SOW lifecycle: this local SOW records the batch as completed but remains
  untracked and local, consistent with the branch-SOW merge-guard discussion.
