# MCP HTTP Timeout Fix

## TL;DR
**FIXED**: Node.js HTTP server's default 300-second request timeout was killing long-running MCP tool calls. Set `server.requestTimeout = 0` to disable it and let MCP client control timeouts.

**FIXED**: Changed log severity from 'VRB' to 'ERR' for canceled sessions to make them visible in production logs.

## Issues

### 1. Log Severity Too Low
**File**: `src/ai-agent.ts:2738`
**Current**: Logs session cancellation as `'VRB'` (verbose)
**Expected**: Should log as `'ERR'` to make it visible in production logs
**Impact**: Canceled sessions are invisible in logs without verbose mode enabled

### 2. Abort Signal Delay (59 seconds!)
**Files**: `src/headends/mcp-headend.ts:802-807` and `:376`
**Current**: HTTP connection close detection doesn't propagate to tool execution abort signal
**What happens**:
1. Claude Code times out at 300s and closes HTTP connection
2. `req.on('close')` fires but only aborts the limiter's abortController (line 805)
3. MCP SDK abort signal (`extra.signal` at line 376) is NOT triggered
4. Tool execution continues for ~59 seconds until MCP SDK detects closed connection
5. Finally aborts with error `[20]: This operation was aborted`

**Evidence from logs**:
```
14:31:24 - Claude Code: HTTP connection dropped, fetch failed
14:32:23 - Neda: WRN LLM response failed [TIMEOUT]: [20]: This operation was aborted
           (59 seconds later!)
```

## Root Cause

There are TWO abort controllers:
1. **HTTP-level AbortController** (line 802) - detects connection close via `req.on('close')`
2. **MCP SDK AbortSignal** (line 376, `extra.signal`) - controls tool execution

The HTTP-level controller is only used for limiter acquisition, NOT for aborting running tools.

## Analysis

### Current HTTP Request Flow
1. Create HTTP-level `abortController` (line 802)
2. Set up `req.on('close')` listener to abort it (line 807)
3. Use it only for limiter acquisition (line 809)
4. Parse request body and route to MCP SDK
5. MCP SDK calls tool handler with `extra.signal`
6. Tool handler spawns agent session with `extra.signal` as abort signal

### The Disconnect
- The `req.on('close')` abort controller (line 805) is never connected to the MCP SDK's tool execution abort signal (line 376)
- The MCP SDK only detects disconnection when it tries to send a response or receives explicit cancellation
- This causes the ~59 second delay (possibly TCP timeout, or when SDK tries to flush response)

## Proposed Fix

### Option 1: Monitor HTTP Connection in Tool Handler (Least Invasive)
Add HTTP connection monitoring to the tool registration handler:

```typescript
// In registerTools, around line 360-380
server.registerTool(normalized, toolConfig, async (rawArgs, extra) => {
  const abortSignal = extra.signal;
  const stopRef = { stopping: false };

  // NEW: Also monitor HTTP connection for this request
  const httpReq = (extra as any).httpRequest;  // If SDK exposes it
  if (httpReq) {
    httpReq.on('close', () => {
      stopRef.stopping = true;
      // Log the early detection
      this.log('HTTP connection closed during tool execution', 'WRN');
    });
  }

  abortSignal.addEventListener('abort', () => { stopRef.stopping = true; }, { once: true });
  // ... rest of handler
});
```

**Issues**:
- MCP SDK might not expose the HTTP request in `extra`
- Doesn't actually trigger the abort signal, just sets stopRef

### Option 2: Create Combined Abort Signal (Better)
Combine the HTTP-level abort controller with MCP SDK's abort signal:

```typescript
// Around line 802-820
const httpAbortController = new AbortController();
if (limiter !== undefined) {
  closeListener = () => {
    if (!httpAbortController.signal.aborted) {
      this.log('HTTP connection closed, aborting pending operations', 'WRN');
      httpAbortController.abort();
    }
  };
  req.on('close', closeListener);
}

// Later, when handling the request, combine signals
// Need to find where the MCP SDK creates its abort signal for tools
```

**Issues**:
- Need to understand MCP SDK internals to inject our abort signal
- Might not be possible without SDK changes

### Option 3: Use StreamableHTTPServerTransport Correctly (Investigate)
The `StreamableHTTPServerTransport` should already handle this. Need to check:
1. Does the SDK properly propagate HTTP connection close to tool abort signals?
2. Is there a configuration we're missing?
3. Is this a bug in the MCP SDK?

**Action**: Review MCP SDK source code for StreamableHTTPServerTransport

### Option 4: Periodic Connection Check (Workaround)
Add periodic HTTP connection liveness check in the session runner:

```typescript
// In AIAgentSession.run(), add periodic check
setInterval(() => {
  if (httpConnectionClosed) {
    this.canceled = true;
    this.toolsOrchestrator?.cancel();
  }
}, 1000);  // Check every second
```

**Issues**:
- Hacky workaround, not a proper fix
- Adds polling overhead

## Recommended Approach

1. **Quick fix (today)**: Change log severity from VRB to ERR
2. **Investigation**: Check if MCP SDK version has known issues with abort signal propagation
3. **Proper fix**: Either fix in MCP SDK or work around by monitoring HTTP connection separately

## Implementation Plan

### Step 1: Fix Log Severity (Immediate)
**File**: `src/ai-agent.ts:2738`
```typescript
// Change from:
this.log({ severity: 'VRB', ... }, { opId: finOp });

// To:
this.log({ severity: 'ERR', ... }, { opId: finOp });
```

### Step 2: Add WRN When HTTP Connection Closes (Quick Win)
**File**: `src/headends/mcp-headend.ts:804-806`
```typescript
closeListener = () => {
  if (!abortController.signal.aborted) {
    this.log('HTTP connection closed during request processing', 'WRN', false, 'request');
    abortController.abort();
  }
};
```

This won't fix the delay but will at least log when the connection closes.

### Step 3: Investigate MCP SDK (Before Major Changes)
- Check MCP SDK GitHub issues for similar problems
- Review StreamableHTTPServerTransport source code
- Test with different MCP SDK versions
- Check if there's a way to pass custom abort signal to the transport

### Step 4: Implement Proper Fix (TBD based on investigation)

## Testing

After fix, verify:
1. Log shows ERR when session is canceled
2. When HTTP client times out, abort happens within ~1 second (not 59 seconds)
3. LLM requests are canceled immediately when connection drops
4. No resource leaks or hanging sessions

## Questions for Investigation

1. Why does the MCP SDK take 59 seconds to detect the closed connection?
2. Is there a TCP timeout involved (default Node.js socket timeout is 0/infinite)?
3. Does the StreamableHTTPServerTransport have a heartbeat or keepalive mechanism?
4. Can we configure the transport to detect disconnections faster?
5. Is this behavior the same for other MCP transport types (SSE, WS)?

---

## RESOLUTION (2025-11-19)

### Root Cause Identified

**Node.js HTTP server has default `requestTimeout = 300000ms` (5 minutes)**

```javascript
// Node.js defaults:
server.timeout = 0              // No socket timeout
server.keepAliveTimeout = 5000  // 5s
server.headersTimeout = 60000   // 60s
server.requestTimeout = 300000  // 5 minutes ← THIS WAS THE CULPRIT
```

### What Was Happening

1. ai-agent creates HTTP server without configuring `requestTimeout`
2. Server inherits Node.js default: 300 seconds (5 minutes)
3. After 300s, Node.js **automatically closes the request**
4. Claude Code sees "fetch failed"
5. Server-side MCP SDK doesn't immediately detect closed connection
6. ~59 seconds later, when tool tries to continue, abort is detected
7. All in-flight LLM requests fail with `[20]: This operation was aborted`

### Fixes Applied

**1. Disabled HTTP request timeout** (src/headends/mcp-headend.ts:615, 661)
```typescript
server.requestTimeout = 0;  // Let MCP client control timeouts
```

**2. Changed log severity** (src/ai-agent.ts:2738)
```typescript
severity: 'ERR'  // Was 'VRB', now visible in production logs
```

### Impact

- Long-running agent sessions (>5 minutes) will now work correctly
- MCP client can control timeout via its own mechanisms
- Aborted sessions now log as ERR instead of VRB (visible without verbose mode)

### Testing Needed

- Verify long-running tools (>5 minutes) complete successfully
- Verify ERR log appears when connection is actually dropped
- Check if WS transport has similar issues (probably not, as it's streaming)
