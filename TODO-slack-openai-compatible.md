# TL;DR
- Finish Slack progress parsing: preserve empty task_status fields, stop flicker, validate in Slack UI/logs.
- Decide emoji handling and complete mrkdwn fixes (shortcodes vs Unicode, headers vs mrkdwn).
- Fix regression: Slack progress updates overwrite final report after session completion.
- Add/confirm Phase 1 harness coverage for Slack formatting beyond unit tests.
- Update docs for provider changes, Slack formatting, and task_status vs thinking semantics.
- Update npm dependencies and re-run lint/build/phase1 after those changes.

# Analysis
- OpenAI-compatible provider support is wired in code (`src/llm-providers/openai-compatible.ts`, `src/llm-client.ts`, `src/config.ts`, `src/types.ts`).
- `neda/.ai-agent.json` and `/home/costa/.ai-agent/ai-agent.json` now set `nova`, `vllm1`, `vllm2`, `llama.cpp` to `type: openai-compatible`.
- Default temperature is 0.2 in `src/options-resolver.ts` and config defaults (neda config shows `defaults.temperature: 0.2`).
- Slack Block Kit schema now allows section blocks to include both `text` and `fields` (`src/slack-block-kit.ts`, `SLACK_BLOCK_KIT_SCHEMA`).
- Slack Block Kit normalization now:
  - Converts common markdown to mrkdwn (headings, links, **bold**, ***bold***, ~~strike~~, tables to code fences).
  - Does not escape `& < >` in plain_text headers.
  - Flattens nested message arrays and inserts `[invalid block dropped]` placeholders when dropping a message.
- Unit tests + fixture were added for the raw payload from `/tmp/2ea853ad-3d5a-4c7c-b1e0-d0d9823cd7f3.json.gz`.
- Phase 1 harness ran clean (warnings only: lingering handles in harness cleanup).
- Remaining gaps:
  - `status-aggregator.ts` still drops empty task_status parts (filters empty strings).
  - Emoji shortcode handling is not finalized (some shortcodes not rendering; headers are plain_text).
  - Docs not updated to reflect recent Slack/mrkdwn changes and task_status semantics.
  - Dependency updates not done.

# Decisions
## D1) Emoji shortcode handling (required)
Context: Some emoji shortcodes (e.g., :database:, :terminal:) do not render in Slack. Headers are plain_text and do not parse shortcodes.

Options:
1) Convert emoji shortcodes to Unicode in **plain_text only**, keep shortcodes in mrkdwn.
   - Pros: minimal change; preserves Slack custom emoji in mrkdwn; fixes headers.
   - Cons: invalid shortcodes in mrkdwn still render as raw text.
2) Convert emoji shortcodes to Unicode in **both** plain_text and mrkdwn.
   - Pros: consistent rendering everywhere.
   - Cons: loses custom emojis; requires mapping table; risk of wrong/unavailable mappings.
3) Leave shortcodes as-is; enforce prompt guidance to use Unicode in headers.
   - Pros: lowest complexity.
   - Cons: relies on model correctness; current failures persist.

Decision: **Option 3** — leave shortcodes as-is; update prompt guidance to use Unicode in headers.

# Completed (verified in code)
- OpenAI-compatible provider support added and used for nova/vllm/llama.cpp.
- Default temperature set to 0.2.
- Consecutive standalone task_status final-turn threshold set to 5.
- Slack Block Kit prompt guidance added in `llm-messages.ts`.
- Slack Block Kit schema updated to allow section `text` + `fields`.
- Slack Block Kit normalization improvements + placeholder for dropped messages.
- Unit tests + fixture for the 2ea853ad payload.
- Lint/build/Phase1 harness run successfully (warnings only).

# Plan
- Implement D1 decision for emoji handling.
- Make `status-aggregator.ts` preserve empty fields (no filter-shift) and verify Slack progress rendering.
- Stop Slack progress updates from overwriting final report once a session completes.
- Add/confirm Phase 1 harness scenario for Slack formatting (beyond unit tests).
- Update docs: provider config, Slack formatting, task_status vs thinking.
- Update npm dependencies; re-run lint/build/phase1.

# Implied Decisions
- Continue using the Slack Block Kit array as canonical output, accept `{messages:[...]}` as input.
- Use placeholder `[invalid block dropped]` for dropped messages to surface losses.

# Testing Requirements
- Run `npm run lint` and `npm run build` after changes.
- Run `npm run test:phase1` after changes.
- Investigate lingering handles warning in Phase1 harness (child process cleanup).

# Documentation Updates Required
- Provider config/type changes (openai-compatible).
- Slack Block Kit mrkdwn rules + conversion behavior.
- task_status vs thinking semantics and Slack progress parsing.
