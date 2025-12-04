TL;DR
- Investigate why valid-looking final reports from hubspot sub-agent are rejected as "Final report incomplete (status='missing')" and session exits with EXIT-INVALID-RESPONSES-EXHAUSTED.
- Status field was removed from the final report contract, but the runtime still gates acceptance and success/error derivation on `finalReport.status`, causing valid reports to be treated as incomplete.

Analysis
- In xml-final mode the agent expects the model to return the final report wrapped in XML tags `<ai-agent-NONCE-FINAL tool="agent__final_report" format="...">...content...</ai-agent-NONCE-FINAL>`. The xml parser turns that into tool parameters `{ status, report_format, _rawPayload }`.
- The adoption logic for format `sub-agent` accepts only `_rawPayload` (from XML) or `report_content` (native tools) as the content; if both are absent/empty it logs `Final report params (empty sub-agent payload)` and treats status as `missing` (`src/session-turn-runner.ts` ~880-925, 1215-1234).
- The user-shared logs show parameters shaped like `{ tool, format, payload }` instead of `_rawPayload`/`report_content`, so the content is ignored and the final report never commits, triggering retries and `EXIT-INVALID-RESPONSES-EXHAUSTED`.
- The native tool schema still requires `report_format` + `report_content` (`src/tools/internal-provider.ts` ~447-456); a `payload` field is not recognized anywhere.
- Current code paths (FinalReportManager + session-turn-runner) insist on `finalReport.status` being `success|failure` before finishing; with the status field removed from the contract, `getFinalReportStatus` returns `missing`, forcing retries and logging `Final report incomplete (status='missing')` even when content/format are valid (`src/session-turn-runner.ts` 1238-1297, 2205-2225; `src/final-report-manager.ts` 87-113, 180-212).
- Types, telemetry, and metrics still expose/label `finalReport.status`, so the public API and emitted metrics no longer match the contract (AIAgentResult, telemetry labels, headend spans).

Decisions
1) Completion signal (status removed): **Chosen 1.1** — treat presence of a committed final report with non-empty content+format as success; no `status` field required from the model.

2) Telemetry/metrics label for outcomes: **Chosen 2.1** — drop the `status` label entirely; rely on `source` + `synthetic_reason` for alerting.

3) Final-report instructions content: **Chosen 3.2** — keep current concise schema-first `finalReportXmlInstructions`; do not expand with format-specific templates.

Plan
- Validate current sub-agent prompt / tool exposure to ensure it instructs models to send `report_content` (or XML `_rawPayload`) instead of `payload`.
- Decide whether to (a) adjust prompts/instructions for sub-agent final reports or (b) add compatibility handling that maps `payload` → `report_content` when format is `sub-agent`.
- If code changes are chosen, add coverage to prevent regressions (phase1/phase2 harness) and run `npm run lint` + `npm run build`.
- Remove `status` field dependencies: types (AIAgentResult), FinalReportManager, session-turn-runner acceptance logic, telemetry labels, headend spans; replace with derived outcome if option 2 above is chosen.
- Adjust finalization success/error derivation to use presence of final report + source (synthetic vs tool) instead of `status`.
- Update docs/specs and tests to reflect the chosen contract (no status field, outcome derived internally if applicable).

Implied decisions
- None identified yet.

Testing requirements
- Determine appropriate lint/build/test commands after analysis; likely `npm run lint` and `npm run build` at minimum.

Documentation updates required
- TBD based on findings; if validation rules or guidance changes, update relevant docs.
