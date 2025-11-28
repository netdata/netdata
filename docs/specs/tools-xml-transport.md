# XML Tool Transport

## TL;DR
Opt-in transport (`tooling.transport=xml|xml-final`) that replaces native tool_calls with XML tags (xml) or keeps native tool_calls while requiring the final report over XML (xml-final). Each turn advertises XML-NEXT (nonce, slots, schemas, final-report + optional progress) and optionally XML-PAST (last turn tool responses). Tags are parsed by substring; valid tags map to existing tools with the same orchestrator, budgets, and accounting. CLI override: `--tooling-transport native|xml|xml-final` beats config/frontmatter when present.

## Modes
- `native` (default): provider sees tools; standard tool_calls.
- `xml`: provider tool list hidden; model must call tools via XML tags. Progress uses XML.
- `xml-final`: provider tools stay native (tool_calls); only final-report travels via XML tag. Progress follows tools transport (native).

## Turn Messages
- **XML-NEXT (ephemeral)**: user notice with nonce, tool schemas, slot templates, progress slot (xml only), final-report slot; counts toward context like any user message. In `xml-final`, XML-NEXT also reminds the model that non-final tools must be called via native tool_calls.
- **XML-PAST (permanent)**: prior turn tool results (slot, tool, status, duration, request/response); suppressed in xml-final; bounded to last turn.

## Slots & Validation
- Nonce per session: `ai-agent-<8hex>` (fixed across all turns).
- Slots: `NONCE-0001...` up to `maxToolCallsPerTurn` for regular tools (incrementing across turns); dedicated `NONCE-FINAL` slot; `NONCE-PROGRESS` for xml mode only.
- Valid tag: matching nonce, declared slot, allowed tool, non-empty content. Others ignored. In `xml-final`, allowed tools via XML are restricted to `agent__final_report`; non-final tools must use native tool_calls; progress uses native tool_calls.
- Status enum for final-report: `success|failure|partial` (extracted from XML tag attributes); payload format must match the agent's expected format.

## Execution Flow
1) Build XML-NEXT/PAST each turn; toolChoice is only forced when tools are present. In `xml`, provider tool definitions are hidden; in `xml-final`, provider tool definitions remain visible for native tool_calls.
2) Model emits XML tags; parser extracts tags into toolCalls. Native tool_calls are ignored in `xml` but **preserved** in `xml-final` (merged with the parsed XML final-report).
3) Orchestrator executes tools unchanged (`source: xml` in accounting); final-report uses `agent__final_report`.
4) Tool responses recorded into XML-PAST for next turn (xml only); final-report ends session (no response message).

## Progress
- In `xml` mode: shares XML transport via `NONCE-PROGRESS` slot.
- In `xml-final` mode: follows tools transport (native tool_calls), not suppressed.

## Context Guard
- Schemas/slots contribute via XML-NEXT text; provider tools list omitted so schemaCtxTokens=0 in xml modes; guard still runs on messages/tool outputs.

## Error Handling
- Invalid/mismatched tags: ignored.
- Payload JSON parsing/validation still done at orchestrator (same repair/validation path as native).
- Over-budget tool outputs replaced with `(tool failed: context window budget exceeded)` as usual.

## Accounting & Telemetry
- Tool accounting: `source: xml`, `command` = real tool name; final-report `command: agent__final_report_xml`.
- Metrics unchanged; toolChoice=auto recorded in provider options.

## Final Report Processing (3-Layer Architecture)
When processing XML final reports, the system uses a 3-layer architecture:
1. **Layer 1 (Transport)**: Extract wrapper fields (`status`, `report_format`) from XML tag attributes; extract raw payload from tag content via `_rawPayload` internal field.
2. **Layer 2 (Format Processing)**: Process payload based on expected format:
   - `sub-agent`: Opaque blob passthrough (no validation, no parsing)
   - `json`: Parse JSON, validate against schema if provided
   - `slack-block-kit`: Parse JSON, expect messages array
   - Text formats (`text`, `markdown`, `tty`, `pipe`): Use raw payload directly
3. **Layer 3 (Final Report)**: Construct clean final report object with status, format, content, and optional metadata.

This separation ensures wrapper fields (transport metadata) never pollute the payload content.

## Test Requirements (planned)
- Parser chunking/nonce/slot/tool validation
- XML-NEXT/PAST rendering (xml vs xml-final)
- Final-report precedence in XML
- Progress via XML (xml only) and native in xml-final
- Native tool_calls ignored in xml but preserved in xml-final; non-final XML tags rejected in xml-final
- Accounting `source: xml`
- Phase 1 harness scenarios covering: xml happy path, xml-final hybrid (native tools + XML final), invalid tag/nonce ignored, and XML-PAST rotation across turns
- 3-layer final report processing for each format type
