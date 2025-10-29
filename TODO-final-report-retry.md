# TL;DR
- Final-report retries now inject explicit guidance as ephemeral user turns that only affect the next LLM call (system prompt remains untouched).
- Reminder copy is format-aware and no longer references `agent__test-agent2`.

# Analysis
- `pushSystemRetryMessage` queues reminders in `pendingRetryMessages`; they are merged into exactly one LLM request and then dropped, so conversation history stays clean.
- Format-aware helper `buildFinalReportReminder()` centralises reminder wording across all retry paths.
- Phase 1 harness coverage still pending (should assert reminder is surfaced when final report status is missing).

# Decisions
1. Keep reminders ephemeral by staging them separately and only merging into the next LLM request.
2. Generate reminder text from `resolvedFormat`/prompt metadata so instructions match the requested medium.
3. Add regression coverage that inspects the request payload (not the persistent conversation) to verify reminders appear when needed.

# Plan
1. Maintain a per-turn `pendingRetryMessages` queue and merge it into the next `TurnRequest` (clear afterwards).
2. Extend `ConversationMessage.metadata` only if needed for future dedupe; remove stale retry entries from history when merging.
3. Use `buildFinalReportReminder()` (format-aware) in all retry paths instead of hard-coded strings.
4. Add/extend tests to confirm reminders are present in the request payload when final report status is missing and absent afterwards.
5. Verify lint/build/test.

# Implied Decisions
- Tests may need to tap into low-level request tracing to observe ephemeral reminders.

# Testing Requirements
- `npm run lint`
- `npm run build`
- Relevant Phase 1 harness scenario covering final-report retries.

# Documentation Updates
- None anticipated (behavioural fix only). EOF
