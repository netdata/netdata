# TODO: Add Retry and Context Info to XML-NEXT Message

## TL;DR
Add retry count ("Retry 2 of 5") and context window percentage ("Your context window is X% full") to the xml-next system notice message.

## Analysis

### Current State
- `renderXmlNextTemplate()` in `src/llm-messages.ts` generates the XML-NEXT message
- Currently displays: "This is turn No {turn} of {maxTurns}."
- Data needed already exists:
  - **Retry info**: `attempts` and `maxRetries` in `session-turn-runner.ts` turn loop
  - **Context %**: `expectedPct` from `ContextGuard.buildMetrics()` in `context-guard.ts`

### Files to Modify
1. `src/xml-transport.ts` - Add fields to `XmlBuildMessagesConfig`
2. `src/xml-tools.ts` - Add fields to `XmlNextPayload`
3. `src/llm-messages.ts` - Add fields to `XmlNextTemplatePayload` and update `renderXmlNextTemplate()`
4. `src/session-turn-runner.ts` - Pass retry/context values when calling `buildMessages()`

## Decisions (Pending User Input)

### 1. Display Format for Retry Info
**Context**: We need to show retry count only when `retryAttempt > 1` (first attempt is not a retry)

**Options**:
- A) Show as: `Retry 2 of 5.` (simple)
- B) Show as: `This is retry attempt 2 of 5.` (more verbose)
- C) Show as: `[Retry 2/5]` (compact tag style)

**Recommendation**: Option A - concise and clear

### 2. Display Format for Context Window
**Context**: The percentage is already computed in `buildMetrics()` as `expectedPct`

**Options**:
- A) Show as: `Your context window is 45% full.`
- B) Show as: `Context usage: 45%`
- C) Show as: `Context: 45% used`

**Recommendation**: Option B - concise and professional

### 3. Placement in Message
**Context**: Current template shows turn info, need to decide where to add new info

**Options**:
- A) After turn info: "This is turn No 3 of 10. Context usage: 45%"
- B) Separate lines: Each on its own line
- C) Combined with turn: "Turn 3/10 | Context: 45% | Retry 2/5"

**Recommendation**: Option B - cleaner, each piece of info on its own line

## Plan

1. Add optional fields to interfaces:
   - `retryAttempt?: number` (1-based, where 1 = first attempt)
   - `maxRetries?: number`
   - `contextPct?: number`

2. Update `renderXmlNextTemplate()` to conditionally display:
   - Retry line only when `retryAttempt > 1`
   - Context line only when `contextPct` is defined

3. Update call site in `session-turn-runner.ts`:
   - Calculate context metrics before calling `buildMessages()`
   - Pass `attempts` (as retryAttempt), `maxRetries`, and `expectedPct` (as contextPct)

## Testing Requirements
- Verify message format when retry = 1 (should NOT show retry line)
- Verify message format when retry > 1 (should show retry line)
- Verify context percentage is displayed correctly
- Build and lint must pass

## Documentation Updates
- docs/specs/tools-xml-transport.md may need update
