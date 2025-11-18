# Headend Thinking Tags TODO

## TL;DR
- Streaming headends (OpenAI & Anthropic compatibility) should bracket every reasoning/thinking block with `\n\n---\n` markers, but the current output only shows the trailing delimiter, leaving the leading separator missing/cropped according to user reports.

## Analysis
- Both `OpenAICompletionsHeadend` and `AnthropicCompletionsHeadend` duplicate a `renderReasoning` closure (`src/headends/openai-completions-headend.ts:395-448`, `src/headends/anthropic-completions-headend.ts:397-448`) that is responsible for formatting progress summaries, turn headers, and reasoning text.
- The renderer keeps a `thinkingSectionOpen` flag scoped to the function invocation and only emits the closing delimiter when `closeThinkingSection` runs (triggered at the start of the next turn, when an update list is rendered, or when a summary is appended). Because we do not close immediately after pushing the thinking text, the closing delimiter may arrive long after the reasoning chunk, whereas the leading delimiter can get skipped entirely depending on how diffs are sliced for SSE output—matching the symptom (“only see the ending `---`).
- The renderer logic is duplicated across the two headends, making it easy for regressions to diverge and hard to test in isolation.
- There is currently no direct unit test to assert the exact Markdown layout for reasoning output, so regressions slip through unnoticed.
- REST & Slack headends simply replay `onThinking` text without any Markdown scaffolding; spec only mandates the Markdown separators for the OpenAI/Anthropic compatibility layers, but we need confirmation if other surfaces must follow suit.

## Decisions (confirmed)
1. Scope is limited to the OpenAI & Anthropic headends (REST already returns plain text and Slack has bespoke progress handling).
2. Reasoning streams must behave as a simple boolean state machine: whenever we emit thinking we ensure the block is open; whenever we emit progress we close it if needed. Opening delimiter **must** be `\n\n---\n`; closing delimiter **must** be `\n---\n\n`.
3. Regression coverage must use Vitest; no new deterministic harness scenarios.

## Plan
1. (Optional but recommended) extract the duplicated reasoning renderer into a shared helper so we only implement the boolean open/close contract once.
2. Update the OpenAI and Anthropic headends so:
   - Thinking deltas call `openThinkingSection()` (which writes `\n\n---\n`) before streaming text, reusing a simple `isThinkingOpen` boolean.
   - Progress/update writers call `closeThinkingSection()` (which writes `\n---\n\n`) before emitting bullets.
   - No buffering occurs; SSE output reflects each event immediately with proper separators.
3. Add Vitest coverage around the helper/renderer to assert exact delimiter placement for single-turn and multi-turn scenarios.
4. Run `npm run lint` and `npm run build` (Vitest tests run via `npm run test:vitest` or a targeted script once implemented).

## Implied Decisions / Assumptions
- Fixing the boolean open/close sequencing (with the exact delimiters) in the shared renderer will resolve the missing/cropped leading separator reports.
- Vitest will remain the sole vehicle for this regression coverage; no harness updates are planned for this issue.

## Testing Requirements
- `npm run lint`
- `npm run build`
- Vitest coverage for the reasoning renderer (exact delimiter placement, single/multi turn, progress interleaving).

## Documentation Updates Needed
- Verify `docs/REASONING.md` already documents the `\n\n---\n` contract; update only if helper introduction changes behavior or clarifies applicability to other headends.
