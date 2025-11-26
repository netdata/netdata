# Reasoning-Only Empty Responses Contract Update

## TL;DR
Add an explicit exception in docs/CONTRACT.md so reasoning-only assistant replies (no content, no tools) are allowed and do not trigger the empty-response retry rule.

## Analysis
- Current contract (docs/CONTRACT.md, section "Empty or Invalid LLM Responses") states that empty content without tool calls must be ignored and retried.
- Implementation (src/session-turn-runner.ts lines ~1018-1054) retries only when the empty reply also lacks reasoning; reasoning-only replies are allowed and added to conversation.
- User confirmed the intended behavior: reasoning-only, tool-free responses should be permitted.

## Decisions (user-approved)
1. Clarify the contract to exempt reasoning-only, content-empty, tool-free replies from the empty-response retry rule.

## Plan
1. Update docs/CONTRACT.md "Empty or Invalid LLM Responses" to document the reasoning-only exception and keep the retry rule for other empty responses.
2. Re-read the section for consistency and wording.
3. No code changes; no tests required.

## Implied Decisions
- None beyond the explicitly approved exception.

## Testing Requirements
- Documentation only; no automated tests required.

## Documentation Updates Required
- docs/CONTRACT.md (section: Empty or Invalid LLM Responses)
