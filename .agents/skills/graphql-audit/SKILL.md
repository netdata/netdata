---
name: graphql-audit
description: Triage GitHub Code Scanning alerts (CodeQL with security-extended suite) for this repository — list open alerts, dismiss as false positive / won't fix / used in tests, query via GitHub REST + GraphQL. Use when the user asks to "review GitHub security alerts", "check CodeQL findings", "triage code scanning", or anything mentioning Code Scanning, CodeQL, security-extended, or github.com/$repo/security/code-scanning.
---

# GitHub Code Scanning triage skill

This skill drives the GitHub Code Scanning API (REST + GraphQL via `gh`) for
the repository's CodeQL alerts. The repo's CI runs the **security-extended**
CodeQL suite, which surfaces a wider set of findings than the default suite.

The skill operates on whatever repo this checkout points at — it derives the
`owner/repo` from the `upstream` (or `origin`) git remote. Auth uses the
`gh` CLI's stored credentials; no token is needed in `.env`.

## MANDATORY — keep this skill alive

**If you (the agent) discover a new pattern, gotcha, working flow, correction,
or any piece of knowledge while running this skill — update this `SKILL.md`
AND commit it BEFORE proceeding. Knowledge that isn't committed is lost.**

Examples of things to capture:
- A CodeQL rule with a known FP pattern + the canonical comment to use
- A query (GraphQL or REST) that returns useful aggregate views not available in the UI
- An alert format change or new field that started appearing
- A way to bulk-dismiss without hitting REST rate limits

## Setup

### Prerequisite — `gh` CLI authenticated

```
gh auth status
```

Must show authentication for github.com. If not, run `gh auth login`.

The `gh` user needs **write** scope on the repository to dismiss alerts;
read scope is enough for listing.

### .env entries

None required for this skill. `gh` handles auth.

(Optional `GITHUB_TOKEN` could go in `.env` if you ever need to bypass `gh`
and call REST/GraphQL directly via curl, e.g. from a script that runs
without an authenticated `gh` session.)

## Triage decision matrix

GitHub Code Scanning alerts have three lifecycle states: `open`, `dismissed`,
`fixed` (auto-detected when the underlying code changes). Manual triage uses
`dismissed` with one of three reasons:

| Decision         | API value         | When to use                                                        |
|------------------|-------------------|---------------------------------------------------------------------|
| False Positive   | `false positive`  | CodeQL is wrong (path is unreachable, type is wider, guard exists) |
| Won't Fix        | `won't fix`       | Real but acceptable risk; not addressing in this codebase           |
| Used in Tests    | `used in tests`   | Alert is in test fixtures / vendored test code, not production      |

A dismissed alert can be **reopened** later (manually in the UI or via
`PATCH /alerts/<n>` with `state=open`); the dismissal history is preserved.

## Workflow

### Step 1 — see what's open

```
bash .agents/skills/graphql-audit/scripts/codeql-list.sh
```

Default output is a count-by-rule summary for **open** alerts:

```
  42  cpp/integer-overflow|warning
  17  cpp/uninitialized-local|error
  ...
```

Filters:
```
bash .agents/skills/graphql-audit/scripts/codeql-list.sh --state=open --severity=high
bash .agents/skills/graphql-audit/scripts/codeql-list.sh --tool=CodeQL --severity=critical
bash .agents/skills/graphql-audit/scripts/codeql-list.sh --raw          # full JSON
```

### Step 2 — inspect a single alert

```
gh api /repos/<owner>/<repo>/code-scanning/alerts/<n>
```

Or visit `https://github.com/<owner>/<repo>/security/code-scanning/<n>` in
the browser for the full data-flow view.

### Step 3 — dismiss

```
bash .agents/skills/graphql-audit/scripts/codeql-dismiss.sh \
    <alert_number> "false positive" "<short ASCII comment>"
```

Comments are stored verbatim; keep them short, factual, and ASCII.

### Step 4 — bulk dismissal

For a known-FP rule pattern (e.g., `cpp/uninitialized-local` always firing
on a particular header), GitHub does NOT have a REST bulk endpoint. The
working pattern is:

```bash
bash .agents/skills/graphql-audit/scripts/codeql-list.sh --raw \
  | jq -r '.[] | select(.rule.id=="cpp/uninitialized-local") | .number' \
  | while read n; do
        bash .agents/skills/graphql-audit/scripts/codeql-dismiss.sh \
            "$n" "false positive" "FP: rule firing on a stub model not real code"
    done
```

Throttle if you have many — the REST API tolerates ~10 req/s for a single
user.

## What this skill does NOT do

- **CodeQL query authoring**: this skill triages results, not the queries
  that produce them. To suppress a class of FPs at the source, edit
  `.github/codeql/` config or the queries themselves.
- **Workflow management**: enabling/disabling CodeQL runs is in
  `.github/workflows/codeql.yml` — not here.
- **Other Advanced Security features** (secret scanning alerts, dependabot)
  use sibling APIs (`/secret-scanning/alerts`, `/dependabot/alerts`). They
  could be added to this skill if needed.

## REST vs GraphQL

CodeQL alerts are exposed via REST (`/repos/{owner}/{repo}/code-scanning/...`).
GraphQL (`gh api graphql`) is useful for cross-repo queries or reaching
combined data (e.g., alert + commit history in one call):

```
gh api graphql -f query='
  query($owner:String!,$name:String!) {
    repository(owner:$owner, name:$name) {
      vulnerabilityAlerts(first: 100, states:OPEN) {
        nodes { securityAdvisory { ghsaId, severity } }
      }
    }
  }' -F owner=netdata -F name=netdata
```

The above is for **Dependabot** advisories, not CodeQL — CodeQL alerts
remain REST-only at the time of writing.

## Failure modes — quick diagnosis

| Symptom                                  | Likely cause                                            |
|------------------------------------------|---------------------------------------------------------|
| `HTTP 403 Resource not accessible by integration` | gh token lacks `security_events` scope          |
| `HTTP 404` on the alerts endpoint        | Code Scanning not enabled for the repo, or wrong slug   |
| `gh: not found`                          | gh CLI not installed                                    |
| Empty output, no error                   | No alerts in that state — try `--state=dismissed` to confirm gh is reaching the API |
| Pagination cuts off at 100               | Use `--paginate` (already in `codeql-list.sh`)         |
