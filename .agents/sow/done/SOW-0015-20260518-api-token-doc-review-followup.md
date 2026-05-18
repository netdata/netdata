# SOW-0015 - Clarify API token endpoint guidance after PR review

## Status

Status: completed

Sub-state: Docs clarification applied, validated, and ready to close with the review-thread reply.

## Requirements

### Purpose

Address the open PR review feedback on the API token documentation so the Cloud and local endpoint guidance is accurate and easier to read.

### User Request

Address the actionable new PR review comments on `docs/netdata-cloud/authentication-and-authorization/api-tokens.md`, especially the feedback about misleading v3/v1 wording and table readability.

### Assistant Understanding

Facts:

- The open actionable review thread says the current wording conflates room-scoped Netdata Cloud v3 endpoints with local Agent v3 endpoints.
- The same thread says `/api/v1/charts` is still available on local Agents, so the docs should say deprecated/EOL rather than unsupported.
- The changed page is mapped in `docs/.map/map.yaml`, so it is a Learn-published doc.

Inferences:

- The docs should separate Cloud-scoped and local-Agent endpoint paths clearly enough that readers do not substitute one surface for the other.
- The common-endpoints table likely needs a clearer layout to satisfy the "prettify the tables" request.

Unknowns:

- None that block implementation.

### Acceptance Criteria

- `docs/netdata-cloud/authentication-and-authorization/api-tokens.md` clearly distinguishes Netdata Cloud room-scoped v3 endpoints from local Agent v3 endpoints, and no longer claims `/api/v1/charts` is unsupported on local Agents.
- Targeted docs validation is run and recorded, and the actionable PR review thread is answered with the addressing commit hash.

## Analysis

Sources checked:

- `.agents/skills/learn-site-structure/SKILL.md`
- `.agents/sow/specs/sensitive-data-discipline.md`
- `docs/.map/map.yaml`
- `docs/netdata-cloud/authentication-and-authorization/api-tokens.md`
- `docs/netdata-ai/skills/query-netdata-cloud/query-metrics.md`
- `docs/netdata-ai/skills/query-netdata-agents/query-metrics.md`
- `docs/netdata-ai/skills/query-netdata-agents/query-nodes.md`
- `.github/workflows/check-markdown.yml`

Current state:

- The common-endpoints table currently lists only the room-scoped Cloud v3 paths.
- The caution block says `/api/v1/charts` is "deprecated and no longer supported" and points to `/api/v3/contexts`, which reads like a Cloud replacement even though that unscoped path is an Agent endpoint.
- The note below still says v3 is recommended for Cloud, but does not explain the local Agent v3 path family.

Risks:

- Incorrect wording here can send users to the wrong endpoint family and cause avoidable 404 responses or confusion about what still works on local Agents.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The current doc update improved Netdata Cloud v3 visibility, but it documented only the Cloud room-scoped v3 paths in the table and then referenced the unscoped Agent `/api/v3/contexts` path in explanatory text. That mixed two different surfaces, and the wording about `/api/v1/charts` overstated the deprecation as removal.

Evidence reviewed:

- `docs/netdata-cloud/authentication-and-authorization/api-tokens.md`
- `docs/netdata-ai/skills/query-netdata-cloud/query-metrics.md`
- `docs/netdata-ai/skills/query-netdata-agents/query-metrics.md`
- `docs/netdata-ai/skills/query-netdata-agents/query-nodes.md`
- `.github/workflows/check-markdown.yml`

Affected contracts and surfaces:

- The Learn-published API token documentation page.
- Reader guidance for Cloud versus local Agent REST paths.
- Markdown table rendering/readability on the published doc page.

Existing patterns to reuse:

- The query-netdata-cloud and query-netdata-agents docs already separate room-scoped Cloud v3 paths from local Agent v3 paths.
- Existing tables in this doc already use aligned pipe-table formatting.

Risk and blast radius:

- Low risk and docs-only, limited to one published documentation page plus its SOW tracking artifact.

Sensitive data handling plan:

- No secrets or tenant identifiers are needed. Use only placeholders such as `{spaceID}` and `{roomID}` that are already public documentation patterns.

Implementation plan:

1. Update the common-endpoints table to separate Cloud and local Agent endpoint families more clearly.
2. Rewrite the caution/note text so deprecation and replacement guidance is accurate for both Cloud and local Agent readers.
3. Run targeted markdown validation and record the outcome.

Validation plan:

- Run targeted markdownlint-style validation for the touched docs area.
- Re-read the rendered markdown source around the changed table/note blocks.
- Search the file for the misleading `/api/v1/charts` and `/api/v3/contexts` wording pattern after the edit.

Artifact impact plan:

- AGENTS.md: no update expected; this is page-specific content, not workflow guidance.
- Runtime project skills: no update expected; existing skills already document the correct Cloud/Agent split.
- Specs: no update expected; no product contract changes, only a docs correction.
- End-user/operator docs: update `docs/netdata-cloud/authentication-and-authorization/api-tokens.md`.
- End-user/operator skills: no update expected; no skill content changes are required for this docs correction.
- SOW lifecycle: create this SOW in `current/`, complete it if validation succeeds, and move it to `done/` with the final docs commit.

Open-source reference evidence:

- External mirrored repositories were not needed; the relevant source of truth is already in this repository's docs and skills.

Open decisions:

- None.

## Implications And Decisions

No user decisions were required; the review feedback is specific enough to implement directly.

## Plan

1. Validate the current docs baseline in the touched area.
2. Apply a small wording/table-only fix in `docs/netdata-cloud/authentication-and-authorization/api-tokens.md`.
3. Re-run targeted validation, update this SOW, and close the review thread with the addressing commit.

## Execution Log

### 2026-05-18

- Opened this SOW after reviewing the active PR feedback and the Learn publication context for the touched doc page.
- Updated `docs/netdata-cloud/authentication-and-authorization/api-tokens.md` to separate Netdata Cloud room-scoped v3 endpoints from local Agent v3 endpoints in the common-endpoints table.
- Rewrote the `/api/v1/charts` caution and the surrounding note so they describe local-Agent backward compatibility and the Cloud/local v3 split accurately.
- Ran targeted validation with `git diff --check`, repeated the existing markdownlint wrapper for the touched docs area, and searched the file for the misleading wording pattern called out in review.

## Validation

Acceptance criteria evidence:

- The common-endpoints table now includes both room-scoped Netdata Cloud v3 paths and local Agent v3 paths, removing the ambiguous jump from Cloud rows to the unscoped `/api/v3/contexts` path.
- The caution block now says `/api/v1/charts` remains available on local Agents for backward compatibility and points readers to the correct Cloud and local replacements.
- The note block now explains that Cloud uses room-scoped v3 POST endpoints while local Agents use unscoped v3 endpoints.

Tests or equivalent validation:

- `git diff --check` -- passed with no output.
- `.agents/skills/codacy-audit/scripts/analyze-local.sh --tool markdownlint --directory /home/runner/work/netdata/netdata/docs/netdata-cloud/authentication-and-authorization` -- wrapper executed twice, but Codacy tool discovery timed out reaching `api.codacy.com`; no doc-specific markdown findings were returned. This is an external-environment limitation, not a content failure in the edited file.
- `rg 'deprecated and no longer supported|Currently, Netdata Cloud is not exposing the stable API|/api/v1/charts.*no longer supported' docs/netdata-cloud/authentication-and-authorization/api-tokens.md` -- no matches after the edit.
- `rg 'no longer supported.*\`/api/v1/charts\`|/api/v3/contexts\` instead' docs` -- no remaining matches for the misleading phrasing pattern in repo docs.

Real-use evidence:

- Re-read the rendered markdown source for the edited section and confirmed the table/note flow now presents the Cloud and local Agent endpoint families side by side with explicit methods and surfaces.

Reviewer findings:

- Addressed the open Copilot review feedback relayed in PR thread `3257827313`: clarify the Cloud vs local Agent v3 replacement guidance and stop claiming `/api/v1/charts` is unsupported.

Same-failure scan:

- Searched the updated file and the `docs/` tree for the exact misleading `/api/v1/charts` wording pattern; no remaining matches were found.

Sensitive data gate:

- No secrets, tokens, tenant identifiers, or private paths were added. The doc continues to use public placeholder forms such as `{spaceID}` and `{roomID}` only.

Artifact maintenance gate:

- AGENTS.md: no update needed; no workflow or repository-wide guidance changed.
- Runtime project skills: no update needed; the existing query skills already documented the correct endpoint split and were used as sources.
- Specs: no update needed; no product/API contract changed, only the prose on this doc page.
- End-user/operator docs: updated `docs/netdata-cloud/authentication-and-authorization/api-tokens.md`.
- End-user/operator skills: no update needed; no public skill behavior or instructions changed.
- SOW lifecycle: status set to `completed`; file will be moved to `.agents/sow/done/` in the same commit as the docs fix.

Specs update:

- No spec update needed because this was a correction to existing docs wording, not a behavioral or contract change.

Project skills update:

- No runtime project skill update needed; the existing skills were already correct and served as the evidence base for the fix.

End-user/operator docs update:

- Updated `docs/netdata-cloud/authentication-and-authorization/api-tokens.md` to clarify endpoint families and improve the common-endpoints table.

End-user/operator skills update:

- No end-user/operator skill update needed; none of the skill content changed.

Lessons:

- When documenting Netdata Cloud v3 endpoints, explicitly show the local Agent v3 equivalents nearby; otherwise later explanatory text can accidentally mix the two surfaces.

Follow-up mapping:

- Implemented in this SOW; no follow-up SOW is needed.

## Outcome

Completed: the API token docs now distinguish Cloud room-scoped v3 endpoints from local Agent v3 endpoints and describe `/api/v1/charts` as deprecated-but-still-available on local Agents.

## Lessons Extracted

- Keep Cloud-scoped and Agent-local API families in the same comparison table when both are referenced below; that prevents ambiguous replacement guidance.

## Followup

None yet.

## Regression Log

None yet.
