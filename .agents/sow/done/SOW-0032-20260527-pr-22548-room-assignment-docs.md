# SOW-0032 - Address PR #22548 room-assignment doc review

## Status

Status: completed

Sub-state: review feedback addressed, validated, and ready to commit

## Requirements

### Purpose

Address the requested review feedback on PR #22548 with the smallest doc changes that fully resolve the issues.

### User Request

Fix all comments in review `pullrequestreview-4364714468` on PR #22548, applying suggested changes exactly when present, and keep the Kubernetes example in a details dropdown at the end instead of in the middle of the page.

### Assistant Understanding

Facts:

- PR #22548 changes `docs/netdata-cloud/node-rule-based-room-assignment.md` and `docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md`.
- Copilot review `4364714468` reported three actionable doc issues: non-idempotent Helm command wording, a broken Kubernetes guide link, and wording that incorrectly implies automatic environment labels.
- The user added an extra placement constraint for the new Kubernetes example: it should appear last and inside a details dropdown.

Inferences:

- The docs fixes are limited to the two touched files and should not change unrelated content.
- Equivalent validation for this docs-only change should focus on link correctness, markdown structure, and targeted repository lint/check commands where available.

Unknowns:

- None.

### Acceptance Criteria

- The reviewed docs no longer imply `helm upgrade` works for first-time installs, no longer contain the broken Kubernetes guide path, and no longer claim automatic environment labels provide the room-routing example.
- The Kubernetes example is moved to the end of the room-assignment page inside a details dropdown, per the user instruction.
- Targeted validation confirms the updated links/structure are correct and no new issues were introduced in the edited docs.

## Analysis

Sources checked:

- `AGENTS.md`
- `docs/netdata-cloud/node-rule-based-room-assignment.md`
- `docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md`
- GitHub PR review data for PR #22548 via MCP (`get_reviews`, `get_review_comments`, `get_files`)
- `packaging/installer/methods/kubernetes.md`
- `.agents/skills/codacy-audit/scripts/analyze-local.sh`

Current state:

- The room-assignment doc places the new Kubernetes example in the middle of the page before the operator reference.
- The example uses `helm upgrade -f ...`, which contradicts the "Deploy (or upgrade)" wording for a first install.
- The example links to `/docs/netdata-agent/installation/kubernetes.md`, which does not exist in this repository.
- The host-labels doc adds an environment-routing sentence inside the "Use automatic labels" section, which is misleading because the `environment` label in the example is custom.

Risks:

- Moving the example could reduce discoverability if the title is removed entirely, so the details summary should still clearly name the example.
- Changing doc links must preserve repository-local link style and point to a real source file.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The previous docs change added a useful Kubernetes example but placed it mid-page, used a non-idempotent Helm command for first installs, linked to a nonexistent path, and described a custom-label workflow as if it came from automatic labels. These are documentation accuracy and structure issues identified directly in the changed lines and the PR review.

Evidence reviewed:

- `AGENTS.md`
- `docs/netdata-cloud/node-rule-based-room-assignment.md`
- `docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md`
- `packaging/installer/methods/kubernetes.md`
- GitHub MCP review metadata for PR #22548 (`pullrequestreview-4364714468` and inline review threads)
- Existing docs detail-dropdown patterns in `docs/netdata-cloud/README.md` and `docs/netdata-agent/configuration/README.md`

Affected contracts and surfaces:

- End-user documentation for room assignment and host labels
- Internal markdown links in the edited docs
- PR #22548 review thread resolution

Existing patterns to reuse:

- Existing `<details><summary>...</summary><br/>` usage in repository docs
- Existing repository-local link to `packaging/installer/methods/kubernetes.md`

Risk and blast radius:

- Low risk; docs-only changes in two existing files.
- Main regression risk is breaking markdown structure or links while moving the example block.

Sensitive data handling plan:

- No secrets or private data are needed. Evidence in this SOW is limited to repository paths, PR identifiers, and public review text.

Implementation plan:

1. Update the room-assignment doc to move the Kubernetes example to the end in a details block, fix the Helm command wording, and replace the broken link with a valid repository path.
2. Update the host-labels doc so the room-assignment cross-link refers to custom labels and the explicit `environment` label instead of automatic labels.
3. Run targeted validation and close the SOW with reviewer-handling notes.

Validation plan:

- Re-run targeted searches on the edited docs for the old broken path and misleading wording.
- Run the repository's existing markdownlint path via `analyze-local.sh` if available; record any environment/tooling limitation encountered.
- Review the final diff for markdown structure and placement.

Artifact impact plan:

- AGENTS.md: no update expected; work follows existing instructions.
- Runtime project skills: no update expected; no durable workflow change discovered.
- Specs: no update expected; no product behavior changed.
- End-user/operator docs: update the two affected docs files.
- End-user/operator skills: no update expected; no user-facing skill content changed.
- SOW lifecycle: create this SOW in `current/`, then mark `completed` and move it to `done/` in the same commit as the docs fix.

Open-source reference evidence:

- No external mirrored repository references were needed; repository sources and GitHub PR review data were sufficient.

Open decisions:

- None.

## Implications And Decisions

No additional user decision was required because the user already specified the example placement constraint.

## Plan

1. Edit the two docs files to address each review finding and the placement instruction with minimal text movement.
2. Validate the updated docs with targeted checks and document any tool/environment limitation.
3. Close and move the SOW together with the final changes.

## Execution Log

### 2026-05-27

- Gathered PR #22548 review data via GitHub MCP because the local `gh` CLI is unauthenticated in this environment.
- Confirmed the affected files, reviewed the exact Copilot comments, and inspected the valid Kubernetes install doc path in `packaging/installer/methods/kubernetes.md`.
- Attempted the repository's existing markdownlint validation path via `.agents/skills/codacy-audit/scripts/analyze-local.sh`; the analyzer could not fetch tools from `api.codacy.com` in this sandbox, so targeted structural checks are also required.

## Validation

Acceptance criteria evidence:

- `docs/netdata-cloud/node-rule-based-room-assignment.md` now uses `helm upgrade --install`, links to `/packaging/installer/methods/kubernetes.md`, and places the Kubernetes example at the end inside a `<details>` block.
- `docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md` now describes the room-assignment example under custom host labels and explicitly names the `environment` label.

Tests or equivalent validation:

- `git diff --check`
- Targeted Python assertions verified:
  - removed `/docs/netdata-agent/installation/kubernetes.md`
  - removed `helm upgrade -f override.yml netdata netdata/netdata`
  - moved the example to the end of `node-rule-based-room-assignment.md`
  - moved the cross-link text to the custom-label section in `organize-systems-metrics-and-alerts.md`
- `.agents/skills/codacy-audit/scripts/analyze-local.sh --tool markdownlint --directory ...` attempted for both affected doc trees, but Codacy tool bootstrap failed in this sandbox with `Connect(api.codacy.com:443) ... timeout`, so repository-provided linting could not complete here.

Real-use evidence:

- Verified `packaging/installer/methods/kubernetes.md` exists and matches the new in-repo link target referenced from the room-assignment doc.

Reviewer findings:

- Copilot review `pullrequestreview-4364714468`: addressed all 3 findings:
  - idempotent Helm command
  - broken Kubernetes guide link
  - misleading automatic-label wording
- Holistic local subagent review of the full PR diff reported no additional significant issues.

Same-failure scan:

- Searched the docs tree for the old broken Kubernetes path, the old `helm upgrade -f` command, and the old automatic-label wording; no remaining matches were found.

Sensitive data gate:

- Confirmed the changes and this SOW contain no secrets, credentials, private endpoints, or customer data.

Artifact maintenance gate:

- AGENTS.md: no update needed; existing instructions were followed and no repository-wide workflow rule changed.
- Runtime project skills: no update needed; no durable new PR-review skill behavior or gotcha was discovered.
- Specs: no update needed; this work changes documentation wording only, not product behavior or contracts.
- End-user/operator docs: updated `docs/netdata-cloud/node-rule-based-room-assignment.md` and `docs/netdata-agent/configuration/organize-systems-metrics-and-alerts.md`.
- End-user/operator skills: no update needed; no output/reference skill consumes this wording.
- SOW lifecycle: this SOW is marked `completed` and moved from `current/` to `done/` with the implementation in the same commit.

Specs update:

- No update needed; the product behavior did not change.

Project skills update:

- No update needed; no stable workflow correction needed to be captured in project skills.

End-user/operator docs update:

- Updated the room-assignment doc and host-labels doc to reflect the reviewed corrections and user-requested example placement.

End-user/operator skills update:

- No update needed; no end-user/operator skill references these exact docs sections.

Lessons:

- Existing docs detail blocks are the right pattern for relegating environment-specific examples without interrupting the main conceptual flow.

Follow-up mapping:

- All requested review findings were implemented in this SOW; no follow-up work remains.

## Outcome

Completed. The PR review feedback was addressed with minimal docs-only changes, the Kubernetes example now appears last in a details dropdown, and targeted validation plus a clean-context diff review found no remaining issues in the current PR diff.

## Lessons Extracted

- Keep environment-specific examples at the end of conceptual docs using the repository's existing `<details>` pattern when they would otherwise interrupt the main reference flow.

## Followup

None yet.

## Regression Log

None yet.
