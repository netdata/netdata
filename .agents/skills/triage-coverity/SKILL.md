---
name: triage-coverity
description: Inspect or review Coverity Scan defects and saved CID bundles; fetch live findings or apply verified triage decisions when requested. Use for Coverity triage helpers, not general scan-build or CI setup.
---

# Coverity Scan Triage

Review defects against current source and use the shipped helpers for the project's unofficial browser JSON API.
Commands run from the repository root; references starting with `./` are relative to this skill directory.

## Pick The Task

| Task | Load |
|---|---|
| Review saved defects or explain a CID | `./review.md`; current source and the saved bundle |
| Prepare a local review bundle | Local Evidence below and `./scripts/prepare-defect.sh` |
| Fetch live tables or defect details | `./operations.md#session-setup` and `./operations.md#fetch-and-resolve` |
| Apply verified classifications | Apply Decisions below, `./review.md#verdicts` and `./operations.md` |
| Change or review a helper | The helper, its callers and `./scripts/_lib.sh`; relevant reference sections |

Authorization follows `AGENTS.md#when-a-sow-is-required`; review scope and evidence follow `AGENTS.md#review`.
A request to inspect or review does not authorize remote triage updates or source fixes. Reuse authorization already
provided for the current scope. Loading this skill MUST NOT start live authentication, keepalive or remote updates.

Use the requested CID set and review approach. Ask only for material missing scope or model preferences; do not ask
again for decisions already made. A local bundle or source review needs neither cookies nor a keepalive. A small batch
SHOULD use a proportionate review. The skill does not prescribe a model pipeline: manual, single-model and
multiple-model reviews remain valid under project policy and the user's chosen tools. For an authorized multiple-model
review, idea generation, source analysis and independent verification MAY be separate roles.

## Local Evidence

Use `.local/audits/coverity/` for this workflow. Helpers detect the repository root through Git; fetch helpers also
accept explicit output paths. Keep each defect's notes, reports, proposed comment, fix draft and validation evidence
under `triage/<scope>/cid-<N>/`, rather than at the repository root.

The local bundler reads `raw/<scope>-all.json` and `details/<scope>/cid-<N>.json` (with a legacy
`details/cid-<N>.json` fallback). Set `coverity_cid` to the requested positive CID and `coverity_scope` to its local
scope name, such as `outstanding`:

```bash
bash .agents/skills/triage-coverity/scripts/prepare-defect.sh \
  "${coverity_cid:?set the CID}" "${coverity_scope:?set the local scope}"
```

It creates `defect-summary.json` (table row), `defect-details.json` (trace/checker/CWE data), `source-context.c`
(roughly 150 lines around the main event), and `TODO.md` (notes). Existing files are retained; `--force` MAY regenerate
these outputs when replacement is intended. Scope defaults to `outstanding` when omitted from this helper.
Coverity's `displayFile` and main-event line are hints: inspect the current function when refactoring has moved it.

## Apply Decisions

Use `./review.md#verdicts` to choose a supported verdict and record the CID, source evidence and intended attributes.
Before an authorized update, confirm the configured project and current defect/classification. For other local
scope names (`dismissed`, `fixed`, `unclassified`, etc.), the finalize helper warns and still posts; it does not verify
that the old classification disagrees or that the update is authorized.

Set `coverity_verdict` and `coverity_comment_file` from the verified decision. Comments MUST be ASCII: use `--` and
straight quotes. The update helper rejects non-ASCII before posting; the historical WAF observations are in
`./operations.md#diagnose-failures`.

```bash
bash .agents/skills/triage-coverity/scripts/finalize-defect.sh \
  "${coverity_cid:?set the CID}" "${coverity_verdict:?set the verified verdict}" \
  "${coverity_scope:?set the local scope}" "${coverity_comment_file:?set the comment file}"
```

A confirmed bug defaults to **Bug / Fix Required**. The optional fifth argument is a fix commit SHA; supply it only
when evidence establishes that the fix was submitted. A merely local commit is not submission evidence. For true-bug
verdicts, this argument selects **Fix Submitted** and appends `Fix commit: <sha>` to the comment. The helper trusts
the caller's reference; it does not check submission or whether the fix addresses the CID.

False-positive and cosmetic verdicts keep their Ignore action. `CODE_GONE` and `NEEDS_HUMAN` print a skip message
and exit without authentication or an update. The attribute enum, JSON value types and low-level explicit-attribute
interface belong to `./scripts/update-triage.sh`; verdict/action and impact/severity mapping belong to
`./scripts/finalize-defect.sh`. Follow `./operations.md#update-details` for severity inputs and response verification.

## Maintaining This Skill

Run `python3 .agents/skills/triage-coverity/scripts/test_finalize.py` for isolated verdict-to-payload checks. It needs
Python 3, Bash, Git and jq; real helper code runs in a disposable repository with synthetic settings and a fake curl.
No live account is used. Shell helpers also need a `bash -n` check when changed.

Operational discovery capture follows `AGENTS.md#knowledge-capture`. Preserve useful view/endpoint behavior, session
and WAF failure clues, analyzer-model exceptions and attribute mappings in the relevant reference or helper owner.
Separate local client guarantees from observations that need confirmation against the current service.
