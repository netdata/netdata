# SOW-0040 - SNMP Traps Artifact Hygiene Before Runtime Work

## Status

Status: completed

Sub-state: completed; SOW-0035 remains pending/open and was not activated.

## Requirements

### Purpose

Make the SNMP trap branch safe to continue by removing local/private details and audit-blocking patterns from committed SOWs, specs, and generator documentation before any runtime trap receiver work starts.

The purpose is hygiene only. This SOW must not change the trap profile schema, regenerated profile pack, taxonomy, runtime design, or pending implementation SOW scope.

### User Request

The user accepted the recommendation to clean durable artifact hygiene issues before starting SOW-0035, and asked for a brief SOW-0035 summary before SOW-0035 starts.

### Assistant Understanding

Facts:

- The worktree was clean before this SOW.
- The SOW audit reported a critical verdict because durable artifacts contain sensitive-data pattern hits and local mirror-path evidence.
- Additional grep checks found local workstation paths and private model endpoint details in generator docs/code comments.
- SOW-0035 remains pending and must not be activated by this cleanup.

Inferences:

- Most flagged examples are public upstream examples or placeholders, but committed durable artifacts should still use redacted/placeheld examples where possible to keep the audit gate green and avoid teaching unsafe patterns.
- Generator defaults should not point at a private endpoint; operators/developers should pass explicit OpenAI-compatible endpoint settings.

Unknowns:

- None blocking. Any unresolved audit warning after cleanup will be classified with file/line evidence before further work.

### Acceptance Criteria

- SOW audit no longer reports critical sensitive-data or open-source-reference hygiene failures for the SNMP trap artifacts touched by this branch.
- `tools/snmp-traps-profile-gen/` contains no local workstation path, private endpoint, or private auth-file reference in committed docs/code defaults.
- SOW-0032/SOW-0033 evidence no longer records workstation mirror absolute paths where the project requires upstream-style evidence or sanitized local-corpus language.
- SOW-0035 remains `Status: open` in `.agents/sow/pending/`.
- A brief SOW-0035 summary is provided to the user before SOW-0035 starts.

## Analysis

Sources checked:

- `.agents/sow/audit.sh`
- `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`
- `sync-docs-specs-skills` global skill instructions
- `tools/snmp-traps-profile-gen/README.md`
- `tools/snmp-traps-profile-gen/classify.py`
- `.agents/sow/current/SOW-0032-20260522-snmp-trap-comparative-analysis.md`
- `.agents/sow/done/SOW-0033-20260523-snmp-mib-mechanical-extraction.md`
- `.agents/sow/done/SOW-0034-20260523-snmp-profile-llm-enrichment.md`
- `.agents/sow/specs/snmp-traps/*.md`

Current state:

- The profile pack itself locally validates as structurally clean: no bad categories, bad severities, non-MIB-qualified names, empty varbinds, dangling varbind references, or duplicate trap names.
- The runtime receiver is not implemented yet; no SNMP trap collector/runtime files exist under the collector/runtime source trees.
- Hygiene findings are in durable artifacts and generator defaults/docs, not in shipped profile YAML data.

Risks:

- Over-redaction can remove useful upstream evidence. Mitigation: keep source/path context and replace only sensitive/local values with placeholders.
- Editing completed SOWs can disturb historical narrative. Mitigation: preserve meaning and update only hygiene-sensitive text.
- Changing generator defaults can break a documented local shortcut. Mitigation: use explicit endpoint arguments and neutral localhost defaults.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The branch is not ready for SOW-0035 because durable artifacts include local/private environment details and audit-blocking sensitive-data patterns. These came from research notes, local generation shortcuts, and upstream example snippets copied into specs.

Evidence reviewed:

- `./.agents/sow/audit.sh` reported critical sensitive-data pattern findings and local mirror-path evidence warnings.
- `rg` found local workstation paths, private model endpoints, and private auth-file references in `tools/snmp-traps-profile-gen/README.md` and `classify.py`.
- `rg` found placeholder/example credentials, public/non-private IP examples, and email examples in SNMP trap comparative spec files.

Affected contracts and surfaces:

- SOW lifecycle artifacts under `.agents/sow/current/` and `.agents/sow/done/`.
- SNMP trap comparative specs under `.agents/sow/specs/snmp-traps/`.
- Generator developer/operator documentation and defaults under `tools/snmp-traps-profile-gen/`.
- No runtime code or profile pack behavior.

Existing patterns to reuse:

- Project sensitive-data discipline: use placeholders such as `[REDACTED_SECRET]`, `[PRIVATE_ENDPOINT]`, and documentation-safe IP ranges.
- Project open-source reference evidence rule: do not write workstation absolute mirror paths as evidence.
- Trap-profile generator remains explicit-argument driven for endpoint selection.

Risk and blast radius:

- Low runtime blast radius: no runtime code path changes.
- Medium documentation risk: specs must remain useful after redaction.
- Security/privacy risk if not fixed: local/private endpoint and secret-like examples remain in commit-ready artifacts.

Sensitive data handling plan:

- Do not add raw secrets, bearer tokens, SNMP communities, private endpoints, private auth paths, personal home paths, customer data, or non-private customer-identifying IPs to any durable artifact.
- Replace unsafe values with placeholders (`[REDACTED_SECRET]`, `[PRIVATE_ENDPOINT]`, `[DOC_EXAMPLE_IP]`) or documentation ranges (`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`) where examples are needed.
- Cite file paths and line numbers, not sensitive values, in this SOW and final report.

Implementation plan:

1. Sanitize generator README and classifier defaults/comments so committed guidance is endpoint-neutral and private-path-free.
2. Redact or replace sensitive-pattern examples in SNMP trap specs while preserving the implementation point each example illustrates.
3. Sanitize SOW-0032/SOW-0033/SOW-0034 local/private evidence references without changing their completed outcome.
4. Re-run targeted greps and the full SOW audit; repeat until clean or classify remaining findings with evidence.

Validation plan:

- `rg` for local paths, private endpoints, private auth references, secret-like examples, and workstation mirror paths.
- `./.agents/sow/audit.sh`
- `git status --short --branch`
- Confirm SOW-0035 remains pending/open.

Artifact impact plan:

- AGENTS.md: not expected; no project-wide rule changes.
- Runtime project skills: not expected; no workflow change beyond following existing hygiene rules.
- Specs: expected redactions in `.agents/sow/specs/snmp-traps/`.
- End-user/operator docs: expected generator README cleanup.
- End-user/operator skills: not expected; no public skill changed.
- SOW lifecycle: this cleanup SOW will close before SOW-0035 starts; SOW-0035 remains pending/open.

Open-source reference evidence:

- No new mirrored-repository source investigation is required. This work sanitizes existing evidence references.

Open decisions:

- None. The user already approved cleanup before SOW-0035; no product/runtime design decision is needed for hygiene redaction.

## Implications And Decisions

- User decision recorded: clean durable artifact hygiene before starting SOW-0035.
- User constraint recorded: provide a brief SOW-0035 summary before SOW-0035 starts.

## Plan

1. Clean generator documentation/defaults.
2. Clean SOW/spec evidence and example values.
3. Validate with targeted greps and the full SOW audit.
4. Summarize SOW-0035 for user confirmation before activating it.

## Execution Log

### 2026-05-25

- Created cleanup SOW before modifying durable artifacts.
- Sanitized generator README and classifier defaults/comments to remove workstation-local paths, private endpoint examples, and private auth-file references.
- Sanitized SOW-0032/SOW-0033/SOW-0034 evidence to remove workstation mirror paths, personal-name references, and private endpoint details while preserving the completed narrative.
- Sanitized SNMP trap comparative specs to replace secret-like examples, public/non-documentation IP examples, email examples, and SNMP community examples with placeholders or documentation-safe wording.
- Confirmed SOW-0035 remains in `.agents/sow/pending/` with `Status: open`.

## Validation

Acceptance criteria evidence:

- `./.agents/sow/audit.sh` reports no sensitive-data patterns across 119 durable artifact files.
- `./.agents/sow/audit.sh` reports mirrored repository evidence uses durable citations across checked SOW files.
- Targeted grep for workstation-local paths, private model endpoints, private auth-file references, raw example tokens, raw SNMP community examples, raw example passwords, raw email examples, and personal-name references in scoped SOW/spec/tool files returned no matches, excluding the bundled IANA registry snapshot.
- SOW-0035 remained `.agents/sow/pending/SOW-0035-20260525-snmp-traps-foundation-mvp.md` with `Status: open`.

Tests or equivalent validation:

- `python3 -m py_compile tools/snmp-traps-profile-gen/classify.py`
- `./.agents/sow/audit.sh`

Real-use evidence:

- Not applicable; hygiene-only SOW. No runtime trap receiver behavior changed.

Reviewer findings:

- Not required. Changes are redactions/default cleanup only, with no runtime design decision.

Same-failure scan:

- Targeted `rg` over `.agents/sow/current`, `.agents/sow/pending`, the touched done SOWs, `.agents/sow/specs/snmp-traps`, and `tools/snmp-traps-profile-gen` found no remaining scoped matches for the cleaned patterns.

Sensitive data gate:

Passed for the audit scope: no raw secrets, bearer tokens, SNMP communities, customer identifiers, personal data, non-documentation public IPs, private endpoints, or workstation-local paths remain in the scoped durable artifacts.

The bundled IANA PEN registry remains unchanged because it is authoritative public input data for the generator, not assistant-authored evidence or operator documentation.

Artifact maintenance gate:

- AGENTS.md: unchanged; existing rules already required this cleanup.
- Runtime project skills: unchanged; no workflow rule changed.
- Specs: updated SNMP trap specs under `.agents/sow/specs/snmp-traps/` to sanitize example values.
- End-user/operator docs: updated `tools/snmp-traps-profile-gen/README.md` to use neutral setup and endpoint examples.
- End-user/operator skills: unchanged; no public skill behavior changed.
- SOW lifecycle: this SOW is marked completed and moved to `.agents/sow/done/`; SOW-0035 remains pending/open.

Specs update:

- Updated only hygiene-sensitive examples/evidence. No product contract, profile schema, taxonomy, or runtime behavior changed.

Project skills update:

- No update needed; `project-snmp-trap-profiles-authoring` already contains the applicable profile/generator rules.

End-user/operator docs update:

- `tools/snmp-traps-profile-gen/README.md` updated to document generic venv setup and explicit endpoint configuration without private defaults.

End-user/operator skills update:

- No update needed; no public operator skill changed.

Lessons:

- Hygiene audits should include targeted greps for completed SOWs and tooling files, because the default SOW sensitive scan focuses on pending/current/spec files.

Follow-up mapping:

- Existing non-project skill classification warnings from `./.agents/sow/audit.sh` are unrelated to this SNMP trap artifact cleanup and are not changed here. They remain existing framework warnings, not blockers for SOW-0035 artifact hygiene.

## Outcome

Durable artifact hygiene is cleaned enough to proceed to the requested SOW-0035 summary/confirmation step. Runtime SNMP trap support has not started.

## Lessons Extracted

Hygiene-only SOWs must still validate completed SOWs and generator tooling, not only pending/current SOWs.

## Followup

None.

## Regression Log

None yet.
