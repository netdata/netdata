# TODO-json-repair

## TL;DR
- Add robust LLM tool-arg parsing with jsonrepair-backed fallback and logging of failures/repairs.
- Add schema-driven deep repair for stringified JSON fields (esp. final_report).
- Update final_report schema to support `encoding: raw|base64` and decode path.
- Update contract/docs; add tests for the new parsing pipeline and encoding.

## Analysis
- Current parser: `parseJsonRecord` in `src/utils.ts` heuristically trims, strips code fences, fixes `\xHH`, extracts first JSON object, closes dangling braces. No tests found in repo.
- Sanitization: `sanitizeTurnMessages` logs dropped tool calls but truncates preview; invalid parameters are dropped. No logging of full payloads; no repair visibility.
- Schema validation: AJV used for `agent__final_report` only; other tools not schema-validated. No nested stringified JSON repair today.
- Final report tool: expects `report_content` string; no encoding option; prompts omit base64 guidance.
- Contract lacks explicit section on tool-arg parsing/repair.

## Decisions (need from Costa)
- Accept adding `jsonrepair` dependency (yes agreed).
- Remove old heuristics after new pipeline? Prefer to drop old as long as new handles legacy cases; keep fixtures to confirm.
- Log full raw payload on parse failure (required).
- Warn when repairs applied with step names (required).

## Decisions made
- Proceeded with `jsonrepair` as primary repair step; keep limited legacy helpers (dangling/embedded extraction) behind the new pipeline.
- Added required `encoding` field (`raw`|`base64`) for `agent__final_report`, defaulting to `raw` when omitted but schema advertises the enum.
- Implemented schema-guided repair loop for `content_json` with logging per repaired path.
- Normalize `\\xHH` / `xHH` escapes after repair so batch/stringified payloads (e.g., run-test-122) pass through correctly.
- Treat missing/invalid final_report attempts as “incomplete” and collapse remaining turns even when the payload was auto-repaired (restores run-test-final-report-literal-newlines).

## Plan
1) Add jsonrepair dependency; implement low-level parser: native parse → jsonrepair → parse; expose detailed result (repairs, errors). Log WARN on repair, ERR with full raw on failure.
2) Integrate parser into tool-call sanitization and other call sites; add deep schema repair loop for final_report (iterate failing path, parse if stringified JSON, bounded retries).
3) Extend final_report schema/prompt: add `encoding: 'raw'|'base64'`; decode when base64; ensure AJV validates.
4) Update docs/CONTRACT.md with a dedicated section on tool-request parsing/repair and logging expectations.
5) Add tests (vitest) covering: clean parse, needs jsonrepair, nested stringified JSON repair, base64 content, repeated-path failure stop, and logging/repair flags.
6) Run lint/build/tests.

## Implied decisions
- Keep new parser deterministic and single-path (no configurability).
- Retain legacy heuristics only if needed to pass new fixtures; prefer to remove to reduce complexity.
- Logging: full payload allowed in logs; ensure no truncation on parse-failure logs.

## Testing requirements
- vitest unit tests for parser + final_report decoding/repair.
- Ensure `npm run lint`, `npm run build`, `npm test` pass.

## Documentation updates required
- docs/CONTRACT.md: add parsing/repair guarantees, logging behavior.
- If prompts/docs mention final_report fields, update `AI-AGENT-GUIDE.md` or related tool docs accordingly.
