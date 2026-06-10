# SOW - SNMP Traps PR CI Fixes

## Status

Status: in-progress

Reopened for the 2026-06-09 CI batch after the journal SDK `v0.6.3`
update exposed new branch-caused failures.

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

Fix the current PR CI issues after updating the journal SDK to `v0.6.3`.

### Acceptance Criteria

- Flake8 failures caused by this branch are fixed.
- Yamllint failures caused by this branch are fixed.
- Go toolchain `go fmt` / `go fix` check passes locally or remaining diffs are explained with evidence.
- Codacy and Sonar findings caused by this branch are triaged and fixed where actionable.
- SOW `no-working-files` remains explicitly out of this batch; SOW files are not deleted locally.

## Pre-Implementation Gate

Status: in-progress

Problem/root-cause model:

- Current PR CI failures after the SDK `v0.6.3` update are no longer
  package/docker Go-version failures.
- Remaining actionable failures are a Go 1.26 `go fix` diff, a static bundle
  runtime check that cannot find `plugins.d/snmp-trap-profile-gen`, Codacy
  markdownlint findings in the generated SNMP traps integration page, and the
  Sonar quality gate duplication threshold.
- The SOW `no-working-files` failure is intentional branch-local merge guarding, not a code defect, and is excluded from this batch by user direction to keep SOW files locally.
- The 2026-06-10 Go toolchain failures after commit `385bec4619` are not
  another `go fix` diff. All failed Go toolchain shards fail
  `TestDiscoverer_Run/simple_discovery` because `snmputils` returns empty
  `SysInfo.Organization` where the test expects `net-snmp`.
- Root cause: the lazy IANA PEN loader removed the embedded registry and now
  reads `iana-enterprise-numbers.txt` from disk. Source-tree tests in CI do not
  have an installed `/usr/lib/netdata/conf.d/...` registry, and the current
  relative source-tree fallbacks are evaluated from each package test directory,
  so they miss `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/`.

Evidence reviewed:

- `gh pr checks 22652 --repo netdata/netdata` reports failures in flake8, yamllint, Go toolchain tests, Codacy, Sonar, and SOW no-working-files.
- `.github/workflows/sow.yml` rejects SOW working files in PR head as a merge guard.
- `AGENTS.md` allows branch-local SOWs during PR work and requires removal only before merge.
- `gh pr checks 22652 --repo netdata/netdata --watch=false` on 2026-06-09
  reports failures in static builds, Go toolchain tests, Codacy Static Code
  Analysis, SonarCloud Code Analysis, and `no-working-files`.
- Go toolchain log for job `80381870997` shows `go fix ./...` wants to update
  `src/go/cmd/godplugin/main.go` to use `slices.Contains`.
- Static build logs for jobs under run `27222784975` fail in
  `jobs/81-netdata-runtime-check.sh`; the downloaded job log reports
  `/opt/netdata/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` missing.
- Public Codacy PR API reports 75 added markdownlint findings, all in
  `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`.
- SonarCloud Code Analysis summary reports quality gate failure from
  `3.1% Duplication on New Code` against a `<= 3%` threshold; code-scanning
  alerts remain clean.

Affected contracts and surfaces:

- Static installer permission normalization in `netdata-installer.sh`.
- Integration documentation generation in `integrations/gen_docs_integrations.py`.
- Go source files touched by `go fmt` / `go fix`.
- Job manager test structure used to reduce Sonar new-code duplication.
- Static-analysis results for PR #22652.

Clean-end-state target:

- CI-relevant lint/toolchain/static-analysis findings caused by the branch are fixed with minimal behavior change.
- Lazy PEN lookup remains disk-backed and first-use only, but source-tree tests
  can find the committed registry without an installed Netdata tree.
- Static artifacts include the SNMP trap profile generator wherever the runtime
  checker expects it, instead of weakening the runtime check.
- Generated SNMP traps integration documentation is produced in the repository's
  existing markdownlint-compatible style.
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
2. Apply the Go 1.26 `go fix` diff in `src/go/cmd/godplugin/main.go`.
3. Inspect static packaging/build wiring for Go helper commands and install
   `snmp-trap-profile-gen` into the static artifact path expected by
   `packaging/runtime-check.sh`.
4. Inspect the generated SNMP traps integration page and its generator/source
   metadata; fix the source path so regenerated docs are markdownlint clean.
5. Re-check Sonar duplication evidence; reduce actionable branch-caused
   duplication if the duplicated file set can be identified.
6. Run focused validation and commit only source/lint fixes.

Validation plan:

- Re-run flake8 on affected Python files.
- Re-run yamllint on affected YAML file.
- Re-run Go toolchain commands in `src/go`.
- Re-run focused SNMP trap Go tests if Go source changes affect the collector.
- Re-check PR CI after push.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: update required because Sonar PR new-code measure
  values are stored under `periods[0].value`; add a focused how-to for future
  PR duplication-gate triage.
- Specs: no product behavior change expected.
- End-user/operator docs and skills: no product behavior change expected.
- SOW lifecycle: keep this SOW branch-local and uncommitted unless takeover is needed.

Open decisions:

- None blocking. User explicitly excluded local SOW deletion from this batch.

## Validation

- 2026-06-10 PEN source-tree lookup fix:
  - All failed Go toolchain shards for commit `385bec4619` failed
    `TestDiscoverer_Run/simple_discovery`.
  - The test expected `SysInfo.Organization` to resolve to `net-snmp`; CI got
    an empty value because the lazy disk-backed IANA PEN loader could not find
    the committed registry from package test working directories.
  - Implemented a `runtime.Caller` source-file fallback after
    `buildinfo.StockConfigDir`, preserving first-use disk loading and avoiding
    any embedded PEN registry.
  - Added `TestEnterpriseNumbersFilePathFindsSourceTreeRegistry` to prove
    source-tree tests can find the committed registry without an installed
    Netdata tree.
  - `go test -count=1 ./plugin/go.d/pkg/snmputils
    ./plugin/go.d/discovery/sdext/discoverer/snmpsd
    ./plugin/go.d/collector/snmp_traps ./cmd/snmptrapprofilegen` passed.
  - `git diff --check` completed without warnings.
- 2026-06-10 Go 1.26 follow-up:
  - Current PR CI failed the Go toolchain job because `go fix ./...` still
    wanted mechanical changes in:
    - `src/go/cmd/snmptrapprofilegen/main.go` (`maps.Copy`);
    - `src/go/plugin/go.d/collector/snmp_traps/profile_test.go`
      (`strings.CutPrefix`).
  - Applied the `go fix ./...` output locally.
  - Re-ran `go fix ./...` in `src/go`; it completed without producing new
    changes.
  - Re-ran focused tests:
    - `go test -count=1 ./plugin/go.d/collector/snmp_traps
      ./plugin/go.d/collector/snmp_topology ./plugin/agent/jobmgr/funcctl
      ./cmd/godplugin ./pkg/funcapi ./plugin/ibm.d/modules/as400`;
    - `go test -count=1 ./cmd/snmptrapprofilegen`.
  - `git diff --check` completed without warnings.
- Go toolchain:
  `go fix ./...` in `src/go` passed and left no extra diff after applying the
  Go 1.26 fixes.
- Focused Go tests:
  `go test ./cmd/godplugin ./plugin/agent/jobmgr ./plugin/agent/jobmgr/funcctl ./plugin/go.d/collector/snmp_traps ./plugin/ibm.d/modules/as400`
  passed.
- Static installer syntax:
  `bash -n netdata-installer.sh packaging/makeself/install-or-update.sh packaging/runtime-check.sh`
  passed.
- Integration docs:
  `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`
  passed and produced only the intended generated page diff.
- Whitespace validation:
  `git diff --check` passed.
- Static build root cause:
  CI logs showed CMake installed `snmp-trap-profile-gen`, but
  `netdata-installer.sh` chmod-normalized libexec files to `0644` and only
  restored executability for `*plugin`/`*.sh`; the fix restores `0750` for
  `snmp-trap-profile-gen` before the runtime checker runs.
- Codacy:
  public PR API still reports the old 75 markdownlint findings until a new
  branch commit is pushed; local generated output now contains
  `<!-- markdownlint-disable-file -->` from the generator.
- Sonar:
  public quality gate reports only `new_duplicated_lines_density` failing at
  `3.1 > 3`. File-level Sonar data showed a 93-line duplicate block in
  `src/go/plugin/agent/jobmgr/funcctl/controller_test.go`; the test harness was
  refactored into a shared helper, which should drop the PR below the threshold
  after the next Sonar analysis.

## Artifact Maintenance Gate

- AGENTS.md: no update needed; workflow and guardrails unchanged.
- Runtime project skills: updated `.agents/skills/sonarqube-audit/SKILL.md`
  and added `.agents/skills/sonarqube-audit/how-tos/triage-pr-duplication-gate.md`
  with the PR duplication-gate API workflow.
- Specs: no update needed; no product behavior or public contract changed.
- End-user/operator docs: generated SNMP traps integration page now includes
  the generated-doc markdownlint suppression.
- End-user/operator skills: no update needed; no public/operator workflow
  changed.
- SOW lifecycle: this SOW remains active branch-local takeover memory; final
  merge preparation still has to delete active SOW working files.
