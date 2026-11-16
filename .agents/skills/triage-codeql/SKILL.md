---
name: triage-codeql
description: Inspect, review or triage GitHub Code Scanning alerts, including CodeQL findings; apply verified dismissals when authorized. Not for CodeQL query authoring, CI configuration, Dependabot, or secret scanning.
---

# GitHub Code Scanning Triage

Use the shipped helpers for GitHub REST alert operations. Run commands below from the repository root.
Owner references starting with `./` are relative to this skill directory.

## Pick The Task

| Task | Read |
|---|---|
| List, inspect or review findings | Setup, Inspect, Triage Decisions |
| Apply an authorized dismissal or batch | Setup, Inspect, Triage Decisions, Apply Verified Decisions |
| Diagnose a failed request | Troubleshooting and the linked API/CLI reference |
| Change queries, suites or CI | The workflow and config owners below; this is not an operational triage task |

Authorization and read-only scope follow `AGENTS.md#when-a-sow-is-required`; findings and evidence follow
`AGENTS.md#review`. Inspection stops at the verified report. Applying remote state changes requires authorization
covering those actions; preserve permission already granted for the current scope.

## Owners

- Helpers: `./scripts/codeql-list.sh`, `./scripts/codeql-dismiss.sh`, and `./scripts/_lib.sh` own their arguments,
  output and repository resolution. Use list-helper `--help` for its flags.
- CI suites and upload filters: `.github/workflows/codeql.yml` and its referenced `.github/codeql/` configs.
  Do not infer every language uses the C/C++ `security-extended` configuration.
- API permissions, alert states and operations:
  [GitHub REST code scanning](https://docs.github.com/en/rest/code-scanning/code-scanning).
- Authentication: [gh environment](https://cli.github.com/manual/gh_help_environment).
- Discovery capture: `AGENTS.md#knowledge-capture`.

## Setup

- The helpers need Bash, Git, `gh` and `jq`; summary output also uses `awk`, `sort`, `head` and `column`.
- Authentication belongs to `gh`, through its configured credentials or supported exported environment variables.
  The helpers do not source `.env`. Use `gh auth status` to diagnose missing authentication; use `gh auth login`
  only when setup is needed. Do not request credential values in conversation.
- Check the endpoint's token permissions and the account's repository access separately. A successful listing does
  not establish permission to dismiss.
- Confirm the target: the helpers prefer `upstream`, falling back to `origin` when that remote lookup is absent.
  They are intended for `github.com` remotes; a fork checkout can therefore operate on upstream alerts.

Resolve the same target used by both helpers; this reads Git configuration without displaying the remote URL.
Keep Setup and subsequent examples in the same shell session so they share `codeql_repo`:

```bash
source .agents/skills/triage-codeql/scripts/_lib.sh
codeql_repo="$(gh_require_slug)"
printf '%s\n' "$codeql_repo"
```

## Inspect

The default is open alerts across tools. The compact summary shows only the most frequent rule/severity groups;
use `--raw` for the complete array merged across pages. For a CodeQL-only investigation:

```bash
bash .agents/skills/triage-codeql/scripts/codeql-list.sh --tool=CodeQL
bash .agents/skills/triage-codeql/scripts/codeql-list.sh --tool=CodeQL --raw \
  | jq -r '.[] | [.number, .rule.id, .most_recent_instance.location.path, .html_url] | @tsv'
```

The list helper also supports `--state=` and `--severity=`; choose filters from the requested investigation.
A candidate list is not a dismissal list.

Set `codeql_alert` to a listed alert number, then inspect the alert with the target resolved in Setup:

```bash
gh api "/repos/${codeql_repo:?resolve the repository in Setup}/code-scanning/alerts/${codeql_alert:?set the alert number}"
```

Open the returned `html_url` for available traces. Verify the rule, affected code, reachability and relevant instances
before deciding; a matching rule ID, directory or filename alone does not establish a false positive.

## Triage Decisions

The dismissal helper supports the following reasons; this is its supported subset, not the complete GitHub API enum.

| Helper reason | Evidence needed |
|---|---|
| `false positive` | The reported defect is incorrect: establish the relevant guard, type or unreachable path. |
| `won't fix` | The defect is real and the user accepts leaving the risk unresolved. |
| `used in tests` | The flagged path is confined to test code or fixtures, rather than reachable production behavior. |

Fixing code and having analysis report `fixed` is different from dismissing a finding. Reopening a dismissed alert is
also possible through GitHub or the update endpoint with `state=open`; the dismissal helper does not implement it.
Consult GitHub's alert history when investigating a reopened finding.

## Apply Verified Decisions

Use `./scripts/codeql-dismiss.sh` only for the verified alert number, reason and comment covered by authorization.
Set `codeql_reason` and `codeql_comment` from that decision. Comments SHOULD be short, factual and ASCII;
the helper passes the quoted comment as one API string argument.

```bash
bash .agents/skills/triage-codeql/scripts/codeql-dismiss.sh \
  "${codeql_alert:?set the verified alert number}" \
  "${codeql_reason:?set the verified reason}" \
  "${codeql_comment:?set the supporting explanation}"
```

For a batch, you MUST record the exact verified alert numbers with their reasons and supporting evidence before applying
the single-alert helper to each. You MUST NOT pipe a rule-only search directly into dismissal commands. Re-check the
target and decision if the code or alert has changed since inspection; inspect the returned state after each request.

Apply writes sequentially and stop on errors. Handle throttling according to the response and
[GitHub's rate-limit guidance](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api);
there is no fixed safe request rate, and the helpers do not implement retries.

## Troubleshooting

| Symptom | Check |
|---|---|
| Missing executable | Install the missing Setup dependency through the environment's normal setup. |
| Authentication or access error | Check `gh` authentication, endpoint token permissions and repository access. |
| Alert endpoint returns 404 | Confirm resolved repository, alert number, visibility/access and Code Scanning availability. |
| Empty successful output | Inspect `--raw`, the selected tool/state/severity and the command's exit status. |
| Suspected missing pages | The list helper already uses `--paginate`; inspect raw output rather than the bounded summary. |
| Missing C/C++ findings under `build/` | Inspect the workflow's SARIF upload filter; config `paths-ignore` alone does not explain built analysis. |

For direct calls beyond the helpers, use the linked REST reference and
[gh api manual](https://cli.github.com/manual/gh_api).
Dependabot's GraphQL `vulnerabilityAlerts` is a different data source; it does not retrieve CodeQL findings.
