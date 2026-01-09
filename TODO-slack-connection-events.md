# TODO: Log All Slack Socket Mode Connection Events

## TL;DR

Add event handlers to capture and log Slack SocketMode connection lifecycle events (connected, disconnected, reconnecting, error) for observability.

## Review Request (2026-01-09)

Review the final implementation in `src/headends/slack-headend.ts` to confirm:

- All 7 events are implemented
- Severity levels match the agreed policy
- Code follows existing patterns
- No remaining issues or risks
- Implementation is complete and correct

## Analysis

### Current State

The `@slack/socket-mode` library emits these events:

- `connecting` - Before attempting connection
- `connected` - After `hello` message from Slack
- `reconnecting` - Before retry attempt
- `disconnecting` - When `disconnect()` called
- `disconnected` - After connection closes
- `authenticated` - After WSS URL retrieved
- `error` - WebSocket error

**Current behavior**: None of these events are captured in ai-agent. Only the library's internal WARN logs appear:

```
[WARN] socket-mode:SlackWebSocket:11 A pong wasn't received from the server before the timeout of 5000ms!
```

### Gap

No structured logging of:

- Connection establishment
- Disconnection events
- Reconnection attempts
- Errors

## Implementation Plan

### Location

`src/headends/slack-headend.ts` - **before** `await this.slackApp.start();` (around line 207)

### Changes Required

1. Access the SocketModeClient from `slackApp.receiver.client` (NOT `slackApp.client`)
2. Add event handlers for all connection lifecycle events
3. Log each event with appropriate severity using the existing `this.log()` method

### Correct Access Path

```typescript
// WRONG (from initial plan):
const socketModeClient = (slackApp as any).client; // This is WebClient!

// CORRECT:
const receiver = (slackApp as any).receiver;
const socketModeClient = receiver.client; // This is SocketModeClient
```

### Events to Handle

| Event           | Severity | Message                          | Payload                              |
| --------------- | -------- | -------------------------------- | ------------------------------------ |
| `connecting`    | VRB      | `Slack connecting`               | none                                 |
| `authenticated` | VRB      | `Slack authenticated`            | `resp` (AppsConnectionsOpenResponse) |
| `connected`     | VRB      | `Slack connected`                | none                                 |
| `reconnecting`  | VRB      | `Slack reconnecting...`          | none                                 |
| `disconnecting` | VRB      | `Slack disconnecting`            | none                                 |
| `disconnected`  | VRB      | `Slack disconnected`             | optional error                       |
| `error`         | ERR      | `Slack socket error: ${message}` | Error object                         |

### Verified Event Payloads

From `@slack/socket-mode` source code (`SocketModeClient.js`):

- `connecting` - NO payload
- `authenticated` - `resp: AppsConnectionsOpenResponse` (contains `url`)
- `connected` - NO payload (just signals `hello` received)
- `reconnecting` - NO payload
- `disconnecting` - NO payload
- `disconnected` - Optional error (README says `(error)`; current SDK emits no args for some paths)
- `error` - Error object

### Code Structure

```typescript
// Before line 207: await this.slackApp.start();
this.attachSocketModeEventHandlers();

// New private method:
private attachSocketModeEventHandlers(): void {
  const receiver = (this.slackApp as unknown as { receiver?: { client?: unknown } })?.receiver;
  if (receiver === undefined || !('client' in receiver)) {
    this.log('Socket mode receiver not available for event handlers', 'WRN');
    return;
  }
  const client = receiver.client as { on: (event: string, handler: (...args: unknown[]) => void) => void };

  client.on('connecting', () => {
    this.log('Slack connecting', 'VRB');
  });

  client.on('authenticated', (resp: unknown) => {
    // AppsConnectionsOpenResponse contains { url, ... }
    this.log('Slack authenticated', 'VRB');
  });

  client.on('connected', () => {
    this.log('Slack connected', 'INFO');
  });

  client.on('reconnecting', () => {
    this.log('Slack reconnecting...', 'INFO');
  });

  client.on('disconnecting', () => {
    this.log('Slack disconnecting', 'VRB');
  });

  client.on('disconnected', () => {
    this.log('Slack disconnected', 'INFO');
  });

  client.on('error', (err: unknown) => {
    const message = err instanceof Error ? err.message : String(err);
    this.log(`Slack socket error: ${message}`, 'ERR');
  });
}
```

### Risks / Edge Cases

1. Handlers attached **after** `start()` will miss the initial `connecting` / `authenticated` / `connected` events.
2. `authenticated` payload includes the WSS URL (sensitive) — do **not** log it.
3. `disconnected` can include an error per SDK README; handler should accept optional error.
4. `receiver` is private in Bolt types; runtime access requires a safe cast + guard.

### Decisions Made

1. **Handler placement**: Attached before `slackApp.start()` to capture initial connect events.
2. **`disconnected` payload handling**: Log optional error message with WRN severity when error present.
3. **Error severity**: `ERR` for socket `error` events, `WRN` for `disconnected` with error.

### Testing Requirements

1. Verify logs appear on connection
2. Verify logs appear on disconnection
3. Verify logs appear on reconnection
4. Verify logs appear on errors

### Documentation Updates

- Update `docs/AI-AGENT-GUIDE.md` if it mentions Slack connection handling
- Update `docs/SESSION-SNAPSHOTS.md` if session snapshots capture connection events

## Implementation Status

**COMPLETED** (2026-01-08)

- Added `attachSocketModeEventHandlers()` method to `SlackHeadend` class
- Method is called after `slackApp` is instantiated and before `start()`
- All 7 events are now logged: connecting, authenticated, connected, reconnecting, disconnecting, disconnected, error
- Uses VRB severity for lifecycle events (connecting, authenticated, connected, reconnecting, disconnecting)
- Uses WRN severity for disconnected with error
- Uses ERR severity for error events
- Lint and build pass

## Review Plan

1. Re-open `src/headends/slack-headend.ts` and verify all event handlers are present.
2. Check severity levels against the agreed mapping.
3. Compare style with adjacent logging patterns in the same file.
4. Identify risks or missing coverage (payload handling, duplicate handlers, lifecycle ordering).
5. Produce a code review report with findings and recommendations.

### Events Implemented

| Event           | Severity | Message                                      |
| --------------- | -------- | -------------------------------------------- |
| `connecting`    | VRB      | Slack connecting                             |
| `authenticated` | VRB      | Slack authenticated                          |
| `connected`     | VRB      | Slack connected                              |
| `reconnecting`  | VRB      | Slack reconnecting...                        |
| `disconnecting` | VRB      | Slack disconnecting                          |
| `disconnected`  | VRB/WRN  | Slack disconnected[: {error}] (WRN if error) |
| `error`         | ERR      | Slack socket error: {message}                |

## Can This Be Done?

**Yes.** The implementation is straightforward:

1. Access the SocketModeClient from `slackApp.receiver.client`
2. Attach event listeners using `.on()`
3. Log using existing `this.log()` method

Estimated effort: 1-2 hours including testing.
