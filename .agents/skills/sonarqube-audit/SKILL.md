---
name: sonarqube-audit
description: Triage SonarCloud findings (issues, hotspots, code smells, vulnerabilities) for this project — search what's open, mark False Positive / Won't Fix / Confirm / Safe / Acknowledged / Fixed, batch-mark whole rule families. Use when the user asks to "review Sonar findings", "triage SonarCloud", "mark False Positive on Sonar", or anything mentioning sonarqube/sonarcloud, S2259, S5008, code smells, security hotspots, or sonarcloud.io.
---

# SonarCloud triage skill

This skill drives the SonarCloud Web API
(<https://docs.sonarsource.com/sonarcloud/api/>) to enumerate findings and apply
triage decisions: False Positive / Won't Fix / Confirmed for issues; Reviewed
with one of Safe / Acknowledged / Fixed for hotspots. Family-mode lets
you mark every open finding for a rule in one go.

The skill operates on the project configured in `.env` (see Setup). Scripts
auto-detect the repo root and write all artifacts under `<repo-root>/.local/`.

## MANDATORY — keep this skill alive

**If you (the agent) discover a new pattern, gotcha, working flow, correction,
or any piece of knowledge while running this skill — update this `SKILL.md`
AND commit it BEFORE proceeding. Knowledge that isn't committed is lost.**

Examples of things to capture:
- New rule with a known FP pattern (and the exact comment to use)
- A bulk-FP family that's safe to apply project-wide
- A SonarCloud API quirk (rate limits, undocumented response shapes)
- A new path/issue exclusion that's safer than per-finding marking

## Setup

### .env entries

```bash
# SonarCloud
SONAR_TOKEN='<paste your token from https://sonarcloud.io/account/security>'
SONAR_HOST_URL=https://sonarcloud.io
SONAR_PROJECT=<project_key, e.g. netdata_netdata>
SONAR_ORG=<organization_key, e.g. netdata>
```

The token is used as **HTTP Basic auth username with empty password**:
`-u "$SONAR_TOKEN:"` (note the trailing colon).

No browser tab is required — token-based auth is stable across sessions.

## Triage decision matrix

### Issues (Bug, Vulnerability, Code Smell)

| Decision     | API transition  | When to use                                                       |
|--------------|-----------------|--------------------------------------------------------------------|
| Confirm      | `confirm`       | Sonar is right, we're going to fix it                             |
| Won't Fix    | `wontfix`       | Real but acceptable — won't fix (e.g., legacy code being deleted) |
| False Positive | `falsepositive` | Sonar is wrong (guard exists, unreachable, tool model error)    |

### Security Hotspots

Hotspots have a separate state machine. They go from `TO_REVIEW` to
`REVIEWED` with one of three resolutions:

| Resolution    | When to use                                                   |
|---------------|---------------------------------------------------------------|
| `SAFE`        | Hotspot reviewed, code is fine as-is (no risk in context)    |
| `ACKNOWLEDGED`| Risk understood, no immediate action — leave for future review |
| `FIXED`       | Hotspot reviewed and the code was changed to remove the risk  |

## ASCII-only comments — non-negotiable

`api.sonarcloud.io` sits behind Cloudflare, which rejects bodies containing
non-ASCII bytes (em-dashes, smart quotes) with a 403 challenge. The scripts
fail before the network round-trip if non-ASCII is detected.

- Use `--` instead of em-dash (U+2014).
- Use straight quotes `"` `'` instead of smart quotes.

## Workflow

### Step 1 — see what's open

```
bash .agents/skills/sonarqube-audit/scripts/sonar-search.sh summary
```

Prints per-rule counts of open issues + open hotspots. Use this to spot
high-volume rules that are candidates for family-mode bulk marking, and
project-wide quality-profile or exclusion changes.

### Step 2 — search for specific rule's findings

Issues:
```
bash .agents/skills/sonarqube-audit/scripts/sonar-search.sh issues --rule cpp:S5827
```

Hotspots:
```
bash .agents/skills/sonarqube-audit/scripts/sonar-search.sh hotspots --status=TO_REVIEW \
  | jq '.hotspots[] | select(.ruleKey=="c:S5443")'
```

### Step 3 — triage

#### Single finding

```
bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh fp     <ISSUE_KEY> "<COMMENT>"
bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh wontfix <ISSUE_KEY> "<COMMENT>"
bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh confirm <ISSUE_KEY> "<COMMENT>"

bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh safe  <HOTSPOT_KEY> "<COMMENT>"
bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh ack   <HOTSPOT_KEY> "<COMMENT>"
bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh fixed <HOTSPOT_KEY> "<COMMENT>"
```

#### Family mode (every open finding for a rule)

```
bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh family-fp   <RULE_ID> "<COMMENT>"
bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh family-safe <RULE_ID> "<COMMENT>"
```

Family mode prints all matched keys and prompts before acting unless
`SONAR_MARK_YES=1` is set.

### Step 4 — dry runs

```
SONAR_DRY_RUN=1 bash .agents/skills/sonarqube-audit/scripts/sonar-mark.sh fp KEY "Comment"
```

In dry-run mode, **write** API calls (mark issues, change hotspot status,
add comments) are printed but not executed. **Read** API calls (issue
search, hotspot search used to enumerate findings in family mode) still
run -- otherwise family mode could not show what it would have acted on.

## What this skill does NOT do

- **Disable rules**: use `api/qualityprofiles/deactivate_rule` directly, or do
  it in the SonarCloud UI under Quality Profiles.
- **Configure issue exclusions**: use Project Settings -> Analysis Scope ->
  Issue Exclusions in the UI.
- **Rule-tuning audit**: when you want a per-rule KEEP/DISABLE/NARROW decision
  log, document that separately (it's project-wide policy, not per-finding
  triage).

## Project-wide quality profile / exclusion configuration

Effective profile lookup:
```
GET /api/qualityprofiles/search?project=$SONAR_PROJECT&organization=$SONAR_ORG
```

To make project-wide changes (deactivate a rule or override severity):
1. Copy the inherited profile (`api/qualityprofiles/copy`)
2. Make your edits there
3. Assign the project to the new profile (`api/qualityprofiles/add_project`)

This is a one-shot operation per language. SonarCloud language keys are:
`c`, `cpp`, `go`, `javascript`, `py`, `shell`, `plsql`, `docker`, `css`,
`ipynb`, `php` (and others depending on the project). Note the rule-id
namespaces in `api/issues/search` results may differ from the language
keys -- e.g. shell rules use the `shelldre:` prefix, Go rules can use
either `go:` or `godre:` depending on which analyzer fired -- so the
language argument to qualityprofile APIs is the SHORT key (`shell`,
`go`), not the rule-namespace prefix.

Keep a record of profile decisions in a project-local doc under
`.local/audits/sonarqube/`.

## Failure modes — quick diagnosis

| Symptom                                | Likely cause                                                |
|----------------------------------------|-------------------------------------------------------------|
| HTTP 401 / 403 with HTML body          | Token wrong/expired, or Cloudflare blocking non-ASCII       |
| Token works for issues but not hotspots| Hotspot endpoints have separate auth checks — token must have `Browse` permission |
| Family-mode appears to stop at 500     | Outdated -- `sonar-mark.sh` family-mode now paginates transparently via `sq_paginate`. If you still see truncation, check `sq_paginate`'s array-key recognition list. |
| `falsepositive` transition rejected    | Issue is not in `OPEN` or `CONFIRMED` state — check current status |
| Hotspot transition rejected            | Hotspot already in `REVIEWED` state — re-check before retry |

## Recurring tips

- `api/issues/search` is paged at `ps=500` max. The `sq_paginate` helper
  in `_lib.sh` walks every page until `paging.total`; use it from any
  new script instead of re-implementing the loop.
- Hotspot `ruleKey` filtering is client-side (search only filters by
  status/project), so the family-mode helper does it in Python.
- An issue may be transitioned only between certain states; if you get
  "Cannot do transition from STATUS X to Y", it's already past that state.
- `SONAR_DRY_RUN=1` is the right knob when iterating on comments
  before committing to a bulk operation.
