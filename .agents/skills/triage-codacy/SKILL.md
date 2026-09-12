---
name: triage-codacy
description: Inspect, analyze, troubleshoot, or review Codacy findings and local analyzer/API helpers. Use for Codacy CI failures, codacy-analysis-cli, PR issue queries, and markdownlint findings. Supplied evidence and source review need no credentials; live queries and local analysis are separate routes. Writes require user authorization.
---

# Codacy audit skill

Use the operation and available evidence to choose a route:

| Task | Route |
|---|---|
| Review supplied findings or helper changes | Inspect that evidence and affected helper contracts; no credentials, API fetch or analyzer run solely because this skill loaded |
| Run local analysis | `scripts/analyze-local.sh`; no token or `.env` needed; select the relevant tool/scope |
| Fetch current PR issues | `scripts/pr-issues.sh`; configured token required by this script; preserve the response and its commit provenance |
| Diagnose a known CLI/API problem | Select the relevant recipe from `./how-tos/INDEX.md` |
| Validate wrapper changes | Run the offline self-test and `python3 -B .agents/skills/triage-codacy/tests/test_helpers.py` |

Loading this skill does not authorize fixes, pushes, remote issue transitions or analysis-policy changes.

This skill is the fourth in the static-analysis triage family in this repo:
`triage-coverity/`, `triage-sonarqube/`, `triage-codeql/`, `triage-codacy/` share one shape and one set of conventions;
each keeps its own `.local/audits/<dir>/` (root `AGENTS.md`, Local-Only Working Directory).

## MANDATORY -- keep this skill alive

Capture timing and authorization for operational discoveries follow `AGENTS.md#knowledge-capture`.

Examples worth capturing:
- New v3 API endpoint or response-shape detail learned the hard way
- Codacy-side rate-limit signals
- A pattern Codacy mismodels for this codebase (so the next assistant can add a path exclusion or mark it FP)
- A new tool the local CLI gained / lost
- Auth-failure surface (e.g. token type mismatch, expired token signs)

## MANDATORY -- live how-tos catalog

`AGENTS.md#knowledge-capture` governs this catalog. Authorized Codacy recipes live under `how-tos/` and are listed in
`./how-tos/INDEX.md`.

## Scope

In scope:

- Local pre-push analysis via `codacy-analysis-cli` (auto-detects local binary, falls back to docker).
- Read-only PR-issue queries against the v3 API.
- Token-safe wrappers (sentinel-driven no-leak self-test).

The following need a user-authorized task and applicable project tracking. A GitHub issue or SOW alone is not
permission to perform them:

- Write actions (mark issue as false-positive, mark as fixed, modify ignore-patterns).
- Repository-wide backlog triage beyond the selected PR or findings.
- Cross-repo aggregation across the netdata org.

## Required env keys

| Key | Required for |
|---|---|
| `CODACY_TOKEN` | Account API token, header `api-token: <value>`. Required by `pr-issues.sh` and any wrapper that calls `_codacyaudit_run`. NOT required by `analyze-local.sh` (the CLI runs anonymously). |
| `CODACY_HOST` | Defaults to `https://api.codacy.com`. Override only if Codacy moves the API host. |
| `CODACY_PROVIDER` | Defaults to `gh` (GitHub). |
| `CODACY_CLI_VERSION` | Optional, not a secret, and read from the process environment only (`analyze-local.sh` does not source `.env`): `CODACY_CLI_VERSION=1.2.3 analyze-local.sh` pins the docker image tag; defaults to `latest`. |
| `CODACY_ORG` | Defaults to `netdata`. |
| `CODACY_REPO` | Defaults to `netdata`. |

All values except `CODACY_CLI_VERSION` live in `<repo>/.env` (gitignored). See `<repo>/.agents/ENV.md` for setup (where
each value comes from, sample formats, common mistakes).

## Scripts (in scripts/)

| Script | Purpose |
|---|---|
| `_lib.sh` | Helpers (`codacyaudit_*` prefix). Token-safe; ships `codacyaudit_selftest_no_token_leak`. |
| `analyze-local.sh` | Run `codacy-analysis-cli` locally; auto-pick local-binary or docker; write JSON dump under `.local/audits/codacy/`. |
| `pr-issues.sh` | Fetch all Codacy issues for a PR via the v3 API; cluster summary on stdout; full JSON dump on disk. |

## Workflow -- pre-push prevention

```
$ .agents/skills/triage-codacy/scripts/analyze-local.sh
[analyze-local] runner=docker format=json dir=<repo>
[analyze-local] wrote 0 finding(s) to <repo>/.local/audits/codacy/local-<ts>.json
```

For an authorized push, use local analysis to catch relevant findings early. Zero findings establishes only the
completed local run's result: CLI versions, selected tools/files and server-side configuration can differ. Check the
remote gate for the current head before claiming it is green. Verify findings before applying authorized fixes.

Failed analyses are not clean trees: when the dump is not JSON, is JSON of the
wrong shape (not a findings array, an `{issues: [...]}` object, or a SARIF `runs`
document), or the CLI exits non-zero with zero findings, the script exits 4 and
keeps the CLI's stderr in `<dump>.log`. Treat exit 4 as "no
evidence", not as green. If GitHub check-run annotations are empty too, use
`pr-issues.sh` with `CODACY_TOKEN`; without that token, record the evidence gap
and re-check after the next push.

One common local cause is gitignored generated output with restrictive file
permissions. For example, if local scratch output under `.local/` contains files
not readable by the Docker container, Codacy logs `Could not read file` messages
and the saved `.json` dump is plain text. Fix or move the local generated output
before trusting local analyzer output. Preserve unrelated files and permissions when correcting the local cause.

A public Codacy v3 endpoint has exposed PR details without a token when GitHub annotations were empty. Availability
is service-dependent; an authorization failure or unavailable response is an evidence gap, not an empty issue list:


```
curl -fsS \
  "https://api.codacy.com/api/v3/analysis/organizations/gh/netdata/repositories/netdata/pull-requests/<PR>/issues?limit=100"
```

Filter for `.data[] | select(.deltaType == "Added")` to inspect added issues, then verify relevance against the
current head and gate; this filter alone does not establish which findings block it. Treat `commitInfo` fields as sensitive operational
metadata; do not copy names or email addresses into committed artifacts.

Operational gotcha: Codacy's PR issue API can lag behind the GitHub check-run
after a new push. If `pr-issues.sh` still reports findings but the Codacy
check-run for the current head SHA is green, inspect `.commitIssue.commitInfo.sha`
in the dump. Reverify older findings against the current source and current-head gate. An older anchor alone does
not prove a finding is stale or resolved; the same defect may still exist.

To restrict to a single tool (matches what Codacy reported on a CI run):

```sh
.agents/skills/triage-codacy/scripts/analyze-local.sh --tool markdownlint
```

## Workflow -- PR triage

```console
$ .agents/skills/triage-codacy/scripts/pr-issues.sh 22423
[pr-issues] fetching issues for PR #22423 ...
[pr-issues] wrote 0 issue(s) to <repo>/.local/audits/codacy/pr-22423-<ts>.json

No issues on PR #22423.
```

For a PR with findings, the script emits a clustered TSV summary. Default grouping is `--by pattern`; switch to `--by tool`, `--by severity`, `--by file`, or `--by category` for other angles. The JSON dump under `.local/audits/codacy/` carries the full issue payload for follow-up jq queries.

Operational note: large Codacy PR issue arrays must be passed to `jq` via a
temporary file and `--slurpfile`, not `--argjson`, because shell argument-size
limits can fail before `jq` starts.

## Path discipline

This skill follows `<repo>/.agents/sensitive-data-discipline.md`:

- Repo files: repo-relative (`<repo>/src/...`).
- Codacy account / org / repo identifiers: env-keyed.
- `CODACY_TOKEN`: NEVER literal in any committed file; ALWAYS via `${CODACY_TOKEN}` and the `_lib.sh` wrappers.
- Audit dumps: gitignored under `<repo>/.local/audits/codacy/`.

## Related skills

- `.agents/skills/triage-coverity/` -- Coverity Scan (same triage shape).
- `.agents/skills/triage-sonarqube/` -- SonarCloud (same triage shape).
- `.agents/skills/triage-codeql/` -- GitHub Code Scanning / CodeQL (same triage shape).

## Token-safe self-test

After changing wrappers, run the offline self-test (no `.env` or token setup):

```console
$ source .agents/skills/triage-codacy/scripts/_lib.sh
$ codacyaudit_selftest_no_token_leak
PASS: codacyaudit_selftest_no_token_leak
```

The self-test uses synthetic configuration and an in-process transport in a subshell. It drives every public HTTP
wrapper, checks success and both output streams, and preserves caller state. The focused tests also cover failure
status and reflected-secret suppression. This checks local wrapper behavior, not live authentication or service
compatibility. Successful API bodies are forwarded unchanged and may contain private data; review before sharing.
