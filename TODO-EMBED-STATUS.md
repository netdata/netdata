# TODO: Embed Headend Status Updates

## TL;DR
Fix the embed headend UI so users see progress/status updates during agent processing instead of just "Connecting..." with no feedback.

## Current Status
**BACKEND FIX COMPLETED** - Code change done, needs deployment verification.

## Problem Statement
When using the embed UI at `/test-div.html`:
1. User sends a message (e.g., "hello")
2. UI shows "Connecting..." and user sees NO progress for the entire processing time
3. Eventually the response appears, but user was blind throughout

## Root Cause (Identified)
The `handleStatusEvent()` function in `embed-headend.ts` only forwarded `agent_update` events when `taskStatus` was set (which requires agent to call `agent__task_status` tool). Other progress events were IGNORED:
- `agent_started` - not forwarded ❌
- `agent_finished` - not forwarded ❌
- `tool_started` - not forwarded ❌
- `tool_finished` - not forwarded ❌
- `agent_update` without taskStatus - not forwarded ❌

## Fix Applied
**File:** `src/headends/embed-headend.ts` (lines 470-562)

Changed `handleStatusEvent()` to forward ALL progress event types:
- `agent_started` → sends status with "Agent X starting..."
- `agent_update` → sends status with message (taskStatus fields if present)
- `agent_finished` → sends status with "Agent X completed"
- `agent_failed` → sends status with error message
- `tool_started` → sends status with "Calling tool..."
- `tool_finished` → sends status with completion/failure

## Deployment

### Source Files Location
```
/home/costa/src/ai-agent.git/src/headends/embed-test/
├── test-div.html
├── test-widget.html
├── test.css
└── test.js
```

### Build Output
The `npm run build` command:
1. Compiles TypeScript to `/home/costa/src/ai-agent.git/dist/`
2. Copies embed-test files: `cp -r src/headends/embed-test dist/headends/`

### Installation (build-and-install.sh)
Copies everything to `/opt/ai-agent/`:
```
/opt/ai-agent/dist/headends/embed-headend.js     ← compiled TS
/opt/ai-agent/dist/headends/embed-test/          ← test UI files
```

### CRITICAL: Correct Path for Test Files
**The embed-headend uses `import.meta.url` which resolves to the compiled JS location.**

Test files MUST be at:
```
/opt/ai-agent/dist/headends/embed-test/
├── test-div.html
├── test-widget.html
├── test.css
└── test.js
```

**NOT at** `/opt/ai-agent/headends/embed-test/` (wrong - no /dist/)

### Running Services
Port 8817 (gpt-5.2): `neda.service` - PID 4057305
Port 8807 (gpt-oss-20b): Different service - PID 4086259

### Restart Services
```bash
sudo systemctl restart neda.service
# or for the gpt-oss service, find correct service name
```

## Testing

### Test Endpoint
**USE minimax-m2.1 on port 8877:**
```
http://10.20.4.205:8877/test-div.html
```

### Available Embed Ports
| Port | Model | Service |
|------|-------|---------|
| 8817 | openai/gpt-5.2 | neda.service |
| 8807 | nova/gpt-oss-20b | local LLM |
| 8877 | nova/minimax-m2.1 | neda-minimax.service |

### Verify SSE Stream Contains Status Events
```bash
timeout 60 curl -s -N -X POST "http://10.20.4.205:8817/v1/chat" \
  -H "Content-Type: application/json" \
  -d '{"message":"hello"}'
```

Should now see `event: status` entries like:
- `event: status` with `eventType: agent_started`
- `event: status` with `eventType: tool_started`
- `event: status` with `eventType: tool_finished`
- `event: status` with `eventType: agent_finished`

## Pending Tasks

1. [x] Verify /opt/ai-agent/dist/ has updated embed-headend.js
2. [x] Restart the service (neda-minimax on port 8877)
3. [x] Test SSE stream shows status events - **WORKING!**
4. [ ] Verify UI shows status updates during processing

## UI Issues Identified

### Trivial Fixes (DONE):
1. [x] Assistant response background - set to transparent (matches page)
2. [x] Icons verified: copy icon for copy, trash icon for clear (already correct)
3. [x] Accent color changed to Netdata green (#00AB44)
4. [x] Spinner now shows "Working..." with spinning circle, status message below

### Complex Issues (IMPLEMENTED):
5. **Markdown re-rendering** - FIXED with double-buffering
   - Two content divs (buffer-a and buffer-b)
   - Render into hidden buffer, then instant swap
   - Throttled with requestAnimationFrame
   - No more visual jumping

6. **Auto-scroll control** - FIXED
   - Scroll event listener tracks if user scrolled up (50px threshold)
   - scrollToBottom() respects userScrolledUp flag
   - Resets on new message send

## Test Results (2026-01-17)
SSE stream now contains status events:
```
event: status - agent_started (support-request)
event: status - agent_finished (support-request)
event: status - agent_started (support)
event: status - agent_update with taskStatus
```

**Next**: Test UI at http://10.20.4.205:8877/test-div.html

## Future Work
- Process `<response_self_evaluation>` metadata from model responses (separate task)
