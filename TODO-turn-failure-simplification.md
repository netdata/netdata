TL;DR
- Costa proposes simplifying success/failure semantics: a turn should succeed only if it makes at least one non-progress/non-batch tool call or produces a non-empty final report; retries exhaust → session fails; session succeeds only when a non-empty final report exists.
- Current code advances turns after invalid responses, forces success on some failed paths (e.g., final_report tool errors, single-provider final turn), and can synthesize reports that still mark success=false but let the session finish; behavior diverges from the proposed rules.
- Decisions needed on what counts as “non-empty” final reports, whether progress reports can salvage a turn, and whether synthetic final reports should still be generated for observability when the session is marked failed.

Analysis
- Turn iteration (`src/session-turn-runner.ts`): for-loop increments turns regardless of success; when retries exhaust with `invalid_response`, it logs and moves to the next turn instead of failing the session. Final-report tool failures set `turnSuccessful = true` to allow synthetic failure later. Final-turn with a single provider also forces `turnSuccessful = true` after invalid response.
- Turn success trigger today: on a successful attempt (status `success` and no `invalid_response`), turn is marked successful even if no final report exists; the presence of tool calls (even empty results) keeps `lastErrorType` clear, so non-final turns advance after any tool path. Empty-without-tools is retried within the turn; if retries exhaust, the turn still advances with warnings.
- Session success derivation (`finalizeWithCurrentFinalReport`): success is `source !== 'synthetic' && !toolLimitExceeded && (!statusFailure || finalReportToolFailedEver)`. Synthetic reports (max-turns exhaustion, invalid slack payload, final_report tool failure) return `success=false` but still commit a final report. Missing final reports after max turns are synthesized, so sessions rarely exit without a final report object.
- Progress reports are not considered in turn success today; progress tool calls are allowed even when no other tools are called. Progress events (`SessionProgressReporter`) emit `agent_finished` if any final report exists, regardless of success flag.
- Unknown tools or schema-invalid tool calls are dropped during sanitization; the attempt is still treated as a valid tool path (clears invalid_response) because `attemptHasValidToolPath` only checks `sanitizedHasToolCalls` and lack of detected failures, so a turn can succeed without any executed tool call.
- Documentation currently states max-turn cap and synthetic final-report fallback; it does not define “failed turn” or session success purely by final-report presence, so adopting the proposed rules would be a contract change affecting specs and telemetry.

Feasibility (2025-12-05)
- Feasible but high-touch: core loop in `src/session-turn-runner.ts` controls retries, turn advancement, and final report adoption; refactors must keep behavior with providers, context guard, and forced-final flows intact.
- Success gating change (require executed non-progress tool or accepted final report) needs new state to track “executed tool” vs “sanitized but dropped” calls; current `attemptHasValidToolPath` is too permissive.
- Retry exhaustion → session fail requires reworking the post-loop branch that currently logs and continues on `invalid_response`; implementation is straightforward but risks altering collapse/forced-final interactions.
- Logging/slug requirements are additive but need a centralized helper to avoid scattering WRN/ERR emission across the loop; achievable by extracting a failure logger invoked once per failed turn/session.
- Synthetic final report policy changes are localized to `finalizeWithCurrentFinalReport` and the max-turn/final-report-tool failure blocks; feasible with careful metric updates.
- Docs/test updates are mandatory; Phase1 harness additions are doable but will require crafting new fixtures for: unknown tool, progress-only, empty-with-reasoning, final-report schema fail, and max-turn exhaustion.

Risks / watchouts
- Regression surface is large: turn loop touches provider cycling, context guard enforcement, forced final turns, tool selection, and synthetic reports; a small logic slip can cause infinite retries or premature session failure.
- Success criterion change may conflict with existing telemetry/metrics that assume “any non-invalid response advances turn”; dashboards could misclassify failures unless updated in lockstep.
- Forcing session failure on retry exhaustion removes today’s implicit “continue after invalid_response”; may break long-running agents relying on best-effort behavior (speculative, needs confirmation).
- Need to avoid double-logging: new single WRN/ERR per failed turn/session must replace existing scattered logs; missing suppression could spam logs and violate slug taxonomy.
- Progress reporter currently emits `agent_finished` on any final report; must ensure it emits failed status when success conditions are not met to avoid downstream orchestrators treating failures as successes.
- Synthetic report paths must remain for observability but set success=false; risk of user-visible behavior change if consumers relied on synthetic reports being treated as success.

Decisions (locked in by Costa)
1) Final report acceptance: Use format-specific validity checks (e.g., Slack must have messages array; JSON must satisfy schema when provided). Only “accepted/valid/verified” reports count toward success. (Choice B)
2) Progress/batch counting: Progress tool and batch wrapper never count toward turn success. (Choice B)
3) What counts as a tool call for turn success: Any MCP/REST/Subagent tool (excluding progress and batch) that is actually invoked counts, even if execution fails; unknown/invalid/malformed tool calls still require a report. (Choice A)
4) Retry exhaustion: When retries for a turn are exhausted without meeting success criteria, the session fails immediately; turns do not advance. (Choice A)
5) Final report policy: A final report is always returned by the system (even when session marked failed); success still requires an accepted, valid, verified report.

Additional directives
- Every identified tool call (including progress, batch wrapper, unknown/invalid/malformed) must always yield a report entry; no silent drops.
- Turn collapse (`maxTurns = currentTurn + 1`) must never increase maxTurns; only shrink/collapse is allowed to avoid runaway sessions.
- Turn failure observability: emit exactly one WRN log per failed turn with exhaustive context and the complete LLM response (streaming and non-streaming). All failure reasons must be encoded as slugs; tests assert slugs, not free text. A single function must emit this log.
- Session failure observability: emit exactly one ERR log per failed session. Synthetic session reports are generated only on session failure.
- Per-attempt logging (2025-12-05): every failed attempt/retry must emit one WRN turn-failure log (with slugs + raw response) before the next retry begins; when the session ultimately fails, emit a single ERR session-failure log.
- Loop clarity requirement: keep the main turn/session loops thin and readable. Heavy logic (failure classification, logging, success checks, collapse decisions) must be hoisted into focused helpers so the core loops express only the control flow.
- Slug taxonomy: use descriptive slugs (accepted set start: no_tools, empty_response, reasoning_only, text_only, malformed_tool_call, unknown_tool, tool_exec_failed, tool_limit, context_guard, final_report_missing, final_report_invalid_format, final_report_schema_fail, retries_exhausted, provider_error, rate_limited); extensible if needed.
- Failure log payload cap: include full LLM response up to ~128 KB; if truncated, mark `truncated=true` in details. Target is to avoid hitting the cap in normal debugging.

Plan (after decisions)
- Update definitions of turn success/failure in code, aligning retry exhaustion → session failure and turn advancement only on successful turns.
- Adjust turn result processing to require at least one executed non-progress/batch tool call or non-empty final report (per decisions) for turn success; unknown/invalid tool calls should not satisfy success criteria.
- Update session-level success derivation to match the new contract; ensure progress reporter emits `agent_failed` when final report absent or synthetic per decision 5.
- Revisit synthetic final-report generation paths (max turns, tool failures, slack validation) to match session failure semantics and logging/metrics.
- Update tests/phase1 harness to cover: no tools + text, unknown tool, invalid schema, progress-only, max-turn exhaustion, final-report presence/absence.
- Sync docs (SPECS, DESIGN, AI-AGENT-GUIDE) to new definitions and retry semantics.

Implied decisions
- Telemetry/metrics labels may need new outcomes for failed turns/sessions without final reports.
- Need clarity on whether maxTurns should still cap retries if turns no longer advance on failure.

Testing requirements
- `npm run lint`
- `npm run build`
- Phase1 harness scenarios covering new success/failure semantics (add/adjust cases as needed).
- Add Phase1 cases for: progress-only turn (fails), unknown tool call, invalid schema tool call, no-tools text-only, retry exhaustion → session failure, single-provider final-turn invalid response, max-turn exhaustion synthetic failure, slug assertions in failure logs.

Documentation updates required
- Update docs/SPECS.md, docs/IMPLEMENTATION.md, docs/DESIGN.md, and docs/AI-AGENT-GUIDE.md to define failed turn/session rules, retry exhaustion behavior, and any synthetic report policy.


Costa Questions:

Based on this implementation:

Q1: is there any possibility for a runaway session, bypassing maxTurns and maxRetries? My expectation is that there should never be any session that can make more than `maxTurns x maxRetries` llm requests. Do I understand it right?

Q2: is there any possibility for any tool request from the llm, no matter if the tool exists or the requests is valid, to not be added back to the messages with a proper tool result (successful or failed)? My expectation is that every tool request the model makes, should always be answered, even if the tool name is invalid, the tool parameters are invalid, etc. There should be no case where the any tool request made by the llm to be silently removed from the list of tools, or not receiving a valid response for the tool.

Q3: when a turn fails, the model should always receive an ephemeral user message starting with `system notice: ` which should explain in detail what the model did wrong, why the turn did not progress and what the model should do to make progress now. This ephemeral user message should not receive caching tags in anthropic requests. There should be at most 1 such ephemeral message per turn. The ephemeral message is sent once to the model, but it is not added to the messages history permanently (this is why it is ephemeral).
