---
name: triage-sonarqube
description: Inspect, review, or apply authorized triage decisions to SonarCloud issues and security hotspots; also review the Sonar helpers. Use for SonarQube/SonarCloud findings, code smells, vulnerabilities, and quality-gate evidence. Supplied evidence needs no live query; per-finding writes and project-wide policy changes have distinct scopes.
---

# SonarCloud triage skill

Use the requested operation to select the workflow. Loading this skill does not authorize remote transitions,
comments, risk acceptance or changes to analysis policy.

| Task | Route |
|---|---|
| Review supplied findings or helper changes | Inspect the provided evidence and affected source; use the decision matrix as review criteria without credential setup or live requests |
| Inspect current findings | Search only the requested project/PR/rule scope with configured credentials; preserve current source and analysis provenance |
| Apply authorized triage | Verify each finding and its classification, then use the selected-key commands below |
| Change a whole rule family or project policy | Establish evidence and authorization for that complete scope before applying the family/profile procedure |

Scripts use the project configured in `.env` and keep audit artifacts under `<repo>/.local/`. Read only the sections
needed for the task; ordinary finding review does not require profile configuration.

## MANDATORY — keep this skill alive

Capture timing and authorization for operational discoveries follow `AGENTS.md#knowledge-capture`.

Examples of things to capture:
- New rule with a known FP pattern (and the exact comment to use)
- A bulk-FP family that's safe to apply project-wide
- A SonarCloud API quirk (rate limits, undocumented response shapes)
- A new path/issue exclusion that's safer than per-finding marking

## Setup

Setup is for live helper execution. Reuse configured credentials through the helper; never ask for tokens in the
conversation. Offline evidence review does not need setup.

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

No browser tab is required for token-based auth. Tokens can expire or be revoked; verify access when executing.

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

The helpers enforce ASCII-only comments before the network round-trip. This preserves a workaround for observed
403 challenges with non-ASCII bodies; it does not establish a universal current Cloudflare rule.

- Use `--` instead of em-dash (U+2014).
- Use straight quotes `"` `'` instead of smart quotes.

## Workflow

### Step 1 — see what's open

```
bash .agents/skills/triage-sonarqube/scripts/sonar-search.sh summary
```

Prints per-rule counts of open issues and hotspots. Use volume to prioritize inspection. A count or shared rule ID
is not evidence that every finding is false positive, safe, or covered by the same exclusion.

### Step 2 — search for specific rule's findings

Issues:
```
bash .agents/skills/triage-sonarqube/scripts/sonar-search.sh issues --rule cpp:S5827
```

Hotspots:
```sh
bash .agents/skills/triage-sonarqube/scripts/sonar-search.sh hotspots --status=TO_REVIEW \
  | jq '.hotspots[] | select(.ruleKey=="c:S5443")'
```

### Step 3 — triage

Before writing, verify the selected finding against reachable code, its current state and the matrix above. Record
why the classification fits; `FIXED` requires the relevant code change. A request to inspect or review remains read-only.
Existing authorization to apply the verified decisions persists; do not add another approval round for routine execution.

#### Single finding

```
bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh fp     <ISSUE_KEY> "<COMMENT>"
bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh wontfix <ISSUE_KEY> "<COMMENT>"
bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh confirm <ISSUE_KEY> "<COMMENT>"

bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh safe  <HOTSPOT_KEY> "<COMMENT>"
bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh ack   <HOTSPOT_KEY> "<COMMENT>"
bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh fixed <HOTSPOT_KEY> "<COMMENT>"
```

#### Family mode (every open finding for a rule)

```sh
bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh family-fp   <RULE_ID> "<COMMENT>"
bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh family-safe <RULE_ID> "<COMMENT>"
```

Prefer single-key commands for a reviewed subset. Family mode re-enumerates every currently open finding for the
rule; it cannot express a selected path or evidence subset. Use it only when the evidence and user authorization
cover that entire current set. Otherwise apply the already verified keys individually.

Family mode prints matched keys and prompts unless `SONAR_MARK_YES=1` is set. That variable skips the helper prompt;
it does not grant user authorization or extend it to newly discovered findings.

### Step 4 — dry runs

```
SONAR_DRY_RUN=1 bash .agents/skills/triage-sonarqube/scripts/sonar-mark.sh fp KEY "Comment"
```

In dry-run mode, **write** API calls (mark issues, change hotspot status,
add comments) are printed but not executed. **Read** API calls (issue
search, hotspot search used to enumerate findings in family mode) still
run -- otherwise family mode could not show what it would have acted on.

## What this skill does NOT do

These are separate project-policy operations, not effects of per-finding triage. Perform them only when that scope
is authorized:

- **Disable rules**: the API offers `api/qualityprofiles/deactivate_rule`; the SonarCloud UI also exposes Quality Profiles.
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

Before an authorized project-wide change, inspect the effective profile, ownership, inheritance and other projects
using it. Reuse an appropriate editable profile when the approved scope covers its consumers. If the inherited
profile cannot be edited or its other consumers must remain unaffected, the API supports copying it
(`api/qualityprofiles/copy`), editing the copy, and assigning this project (`api/qualityprofiles/add_project`). Verify
the resulting effective profile; do not create a fresh copy automatically on every run.

SonarCloud language keys include:
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
| Missing `.env` for a public PR query   | Try `https://sonarcloud.io/api/issues/search` with `componentKeys`, `pullRequest`, `sinceLeakPeriod=true`, and `statuses=OPEN,CONFIRMED`; anonymous reads have worked for public projects, but availability must be checked; failure is an evidence gap. |
| Token works for issues but not hotspots| Hotspot endpoints have separate auth checks — token must have `Browse` permission |
| Family-mode appears to stop at 500     | Outdated -- `sonar-mark.sh` family-mode now paginates transparently via `sq_paginate`. If you still see truncation, check `sq_paginate`'s array-key recognition list. |
| `falsepositive` transition rejected    | Check current state, available transitions and permissions; do not assume a previously supported transition is still available |
| Hotspot transition rejected            | Re-check current state, available resolutions and permissions before retry |

## Recurring tips

- `api/issues/search` is paged at `ps=500` max. The `sq_paginate` helper
  in `_lib.sh` walks every page until `paging.total`; use it from any
  new script instead of re-implementing the loop.
- PR new-code measures returned by `api/measures/component_tree` are stored
  under `measures[].periods[0].value`, not `measures[].value`. This matters for
  quality-gate metrics such as `new_duplicated_lines`,
  `new_duplicated_lines_density`, and `new_lines`.
- Hotspot `ruleKey` filtering is client-side (search only filters by
  status/project), so the family-mode helper does it with `jq`.
- An issue may be transitioned only between certain states; if you get
  "Cannot do transition from STATUS X to Y", inspect its current state and available transitions before retrying.
- `SONAR_DRY_RUN=1` is the right knob when iterating on comments
  before committing to a bulk operation.
