---
name: codacy-audit
description: Codacy Cloud workflow for this repository -- run Codacy's analyzers locally before `git push` (mirrors what Codacy CI runs), and fetch/cluster Codacy issues for any PR via the v3 API. Use when the user mentions Codacy, "codacy analysis", `codacy-analysis-cli`, "codacy issues on PR", "fix codacy CI", "codacy markdownlint findings", or any Codacy gate failing on a netdata-org PR. Ships scripts analyze-local.sh (docker/binary runner for codacy-analysis-cli) and pr-issues.sh (paginated v3 issue fetch + group-by tool/pattern/severity/file). Token-safe -- CODACY_TOKEN never reaches assistant-visible stdout. Read-only by design in the current SOW; write actions (mark FP, mark fixed) are deferred.
---

# Codacy audit skill

Drives Codacy Cloud for `netdata/netdata`:

1. **Pre-push prevention** -- run the same analyzers Codacy CI runs, locally, before `git push`. Collapses the "push -> wait minutes -> see findings -> fix -> push again" loop into one push.
2. **Read-only PR triage** -- list Codacy issues for any PR, cluster by tool / pattern / severity / file, drop the JSON dump under `<repo>/.local/audits/codacy/`.

This skill is the fourth in the static-analysis triage family in this repo:
`coverity-audit/`, `sonarqube-audit/`, `graphql-audit/`, `codacy-audit/`. Same shape, same conventions, same artifact directory.

## MANDATORY -- keep this skill alive

If you (the assistant) discover a new pattern, gotcha, working flow, correction, or any operational knowledge while running this skill -- update this `SKILL.md` AND commit it BEFORE proceeding. Knowledge that isn't committed is lost.

Examples worth capturing:
- New v3 API endpoint or response-shape detail learned the hard way
- Codacy-side rate-limit signals
- A pattern Codacy mismodels for this codebase (so the next assistant can add a path exclusion or mark it FP)
- A new tool the local CLI gained / lost
- Auth-failure surface (e.g. token type mismatch, expired token signs)

## MANDATORY -- live how-tos catalog

Each concrete question that requires non-trivial analysis (multiple wrapper calls, jq pipelines, cross-referencing other skills) MUST become a how-to under `how-tos/<slug>.md` AND get an entry in `how-tos/INDEX.md` BEFORE the task is reported complete. Skipping this means the next assistant repeats the analysis from scratch.

## Scope (current SOW)

In scope:

- Local pre-push analysis via `codacy-analysis-cli` (auto-detects local binary, falls back to docker).
- Read-only PR-issue queries against the v3 API.
- Token-safe wrappers (sentinel-driven no-leak self-test).

Out of scope (deferred to a future SOW):

- Write actions (mark issue as false-positive, mark as fixed, modify ignore-patterns).
- Master-backlog triage on the 31,425+ open issues.
- Cross-repo aggregation across the netdata org.

## Required env keys

| Key | Required for |
|---|---|
| `CODACY_TOKEN` | Account API token, header `api-token: <value>`. Required by `pr-issues.sh` and any wrapper that calls `_codacyaudit_run`. NOT required by `analyze-local.sh` (the CLI runs anonymously). |
| `CODACY_HOST` | Defaults to `https://api.codacy.com`. Override only if Codacy moves the API host. |
| `CODACY_PROVIDER` | Defaults to `gh` (GitHub). |
| `CODACY_ORG` | Defaults to `netdata`. |
| `CODACY_REPO` | Defaults to `netdata`. |

All values live in `<repo>/.env` (gitignored). See `<repo>/.agents/ENV.md` for setup (where each value comes from, sample formats, common mistakes).

## Scripts (in scripts/)

| Script | Purpose |
|---|---|
| `_lib.sh` | Helpers (`codacyaudit_*` prefix). Token-safe; ships `codacyaudit_selftest_no_token_leak`. |
| `analyze-local.sh` | Run `codacy-analysis-cli` locally; auto-pick local-binary or docker; write JSON dump under `.local/audits/codacy/`. |
| `pr-issues.sh` | Fetch all Codacy issues for a PR via the v3 API; cluster summary on stdout; full JSON dump on disk. |

## Workflow -- pre-push prevention

```
$ .agents/skills/codacy-audit/scripts/analyze-local.sh
[analyze-local] runner=docker format=json dir=<repo>
[analyze-local] wrote 0 finding(s) to <repo>/.local/audits/codacy/local-<ts>.json
```

Run this before `git push`. If it returns 0 findings, the Codacy gate on the PR will be green (modulo Codacy server-side patterns the local CLI doesn't bundle). If it returns findings, fix them locally first.

To restrict to a single tool (matches what Codacy reported on a CI run):

```
$ .agents/skills/codacy-audit/scripts/analyze-local.sh --tool markdownlint
```

## Workflow -- PR triage

```
$ .agents/skills/codacy-audit/scripts/pr-issues.sh 22423
[pr-issues] fetching issues for PR #22423 ...
[pr-issues] wrote 0 issue(s) to <repo>/.local/audits/codacy/pr-22423-<ts>.json

No issues on PR #22423.
```

For a PR with findings, the script emits a clustered TSV summary. Default grouping is `--by pattern`; switch to `--by tool`, `--by severity`, `--by file`, or `--by category` for other angles. The JSON dump under `.local/audits/codacy/` carries the full issue payload for follow-up jq queries.

Operational note: large Codacy PR issue arrays must be passed to `jq` via a
temporary file and `--slurpfile`, not `--argjson`, because shell argument-size
limits can fail before `jq` starts.

## Path discipline

This skill follows `<repo>/.agents/sow/specs/sensitive-data-discipline.md`:

- Repo files: repo-relative (`<repo>/src/...`).
- Codacy account / org / repo identifiers: env-keyed.
- `CODACY_TOKEN`: NEVER literal in any committed file; ALWAYS via `${CODACY_TOKEN}` and the `_lib.sh` wrappers.
- Audit dumps: gitignored under `<repo>/.local/audits/codacy/`.

## Related skills

- `.agents/skills/coverity-audit/` -- Coverity Scan (same triage shape).
- `.agents/skills/sonarqube-audit/` -- SonarCloud (same triage shape).
- `.agents/skills/graphql-audit/` -- GitHub Code Scanning / CodeQL (same triage shape).

## Token-safe self-test

Before trusting wrappers in a long-running session, run the self-test:

```
$ source .agents/skills/codacy-audit/scripts/_lib.sh
$ codacyaudit_load_env
$ codacyaudit_selftest_no_token_leak
PASS: codacyaudit_selftest_no_token_leak
```

The self-test sets `CODACY_TOKEN` to a sentinel UUID, drives every public wrapper, captures stdout, and asserts the sentinel never appears. Run after editing `_lib.sh` or any wrapper.
