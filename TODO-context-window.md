# TODO – Context Window Investigation

## TL;DR
- User (Costa) reports confusing context token (ctx) progression in the Nov 19 session with fireflies agent and wants an explanation.
- Need to inspect provided log excerpt to understand how ctx grew per turn and whether behavior is expected or broken.

## Analysis
- Provided excerpt covers two sessions (requirement extraction + follow-up generation) on Nov 19 23:57–23:58, including token accounting per turn.
- Each turn shows `LLM request prepared` with `tokens: ctx … new … schema … expected …` followed by responses referencing `ctx` again; must interpret how transcripts and conversation history contribute to totals.
- Turn-by-turn ctx jumps: Turn1 7,183 → Turn2 16,239 → Turn3 25,338 → Turn4 34,506 (with MCP transcript 19,513 tokens) → Turn5 84,040 (apparent double counting) → Turn6 114,107 before guard triggered; need to reconcile vs limit 114,432.
- Must correlate MCP transcript payload sizes (76,809 bytes ≈ 19,513 tokens, 69,966 bytes ≈ 17,802 tokens) with ctx increments to see whether transcripts re-injected multiple times.
- Observation: `expected` counts combine `ctx + new + schema`; `ctx` resets down to ~29,998 for turn5 response despite `request prepared` citing 84,040; indicates pruning or dedup by runtime; need explanation referencing existing tokenizer behavior.
- Specification references: `docs/specs/context-management.md` documents `currentCtxTokens`, `pendingCtxTokens`, `newCtxTokens`, and guard math; `TODO-context-guard.md` (Counter Contract) states `currentCtxTokens` must equal the previous turn’s provider-reported `ctx` while `newCtxTokens` holds freshly added prompt/tool tokens until they commit.
- Current implementation (`src/ai-agent.ts:476`) seeds `currentCtxTokens = estimateTokensForCounters(conversation)` during construction, so first-turn `ctx` already contains system + user prompts instead of zero, contradicting the contract and Costa’s requirement that first `ctx` be 0 and the initial prompt live entirely in `new`.

## Decisions Needed
- Confirm whether Costa needs just narrative explanation or also remediation plan (currently assume explanation only); awaiting confirmation if deeper debugging (code changes) required.
- Need clarity if additional logs or metrics (e.g., instrumentation from ai-agent) should be reviewed beyond provided excerpt.
- Confirm target behaviour: first-turn `ctx` must be zero with entire seed prompt counted as `new`; `currentCtxTokens` should update only after provider responses.

## Plan
1. Parse provided log snippet to reconstruct timeline of ctx, transcripts fetched, and guard actions.
2. Explain how ctx is calculated (conversation history + cached schema + pending tool outputs) and why jumps occur (large transcripts appended, history retention 0?).
3. Identify potential anomalies (e.g., transcript tokens counted twice) and flag uncertainties needing more data.
4. Summarize findings for Costa with explicit facts, speculation labels, and potential issues/risks, plus recommended next questions if needed.
5. Review implementation vs. spec to pinpoint why first-turn `ctx` ≠ 0 (currently due to `estimateTokensForCounters` seeding) and outline required fix (seed `currentCtxTokens` at 0, push initial prompt into `new/pending`).

## Implied Decisions
- Treat this as observability analysis task only; no code/documentation changes until Costa requests.
- Assume provided log excerpt is authoritative snapshot; if insufficient, request more logs rather than guessing implementation details.

## Testing Requirements
- None yet (informational analysis only). If debugging proceeds to code changes, will outline lint/build/test steps then.

## Documentation Updates Required
- None identified. If discovery leads to knowledge base changes, will revisit after investigation.
