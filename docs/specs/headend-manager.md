# Headend Manager

## TL;DR
Orchestrator for multiple network service endpoints (headends). Handles lifecycle, fatal error propagation, and graceful shutdown.

## Source Files
- `src/headends/headend-manager.ts` - Full implementation (161 lines)
- `src/headends/types.ts` - Headend interface definitions

## Data Structures

### HeadendManagerOptions
```typescript
interface HeadendManagerOptions {
  log?: HeadendLogSink;
  onFatal?: (event: HeadendFatalEvent) => void;
  shutdownSignal: AbortSignal;
  stopRef: { stopping: boolean };
}
```

### HeadendFatalEvent
```typescript
interface HeadendFatalEvent {
  headend: Headend;
  error: Error;
}
```

### HeadendContext
```typescript
interface HeadendContext {
  log: (entry: LogEntry) => void;
  shutdownSignal: AbortSignal;
  stopRef: { stopping: boolean };
}
```

## Manager Class

**Location**: `src/headends/headend-manager.ts:36-160`

```typescript
class HeadendManager {
  private readonly headends: Headend[];
  private readonly log: HeadendLogSink;
  private readonly onFatal?: (event) => void;
  private readonly shutdownSignal: AbortSignal;
  private readonly stopRef: { stopping: boolean };
  private readonly active: Set<Headend>;
  private readonly watchers: Promise<void>[];
  private fatalResolved: boolean;
  private fatalDeferred: Deferred<HeadendFatalEvent | undefined>;
  private stopping: boolean;
}
```

## Lifecycle Operations

### Construction
**Location**: `src/headends/headend-manager.ts:48-54`

```typescript
constructor(headends, opts) {
  this.headends = headends;
  this.log = opts.log ?? noopLog;
  this.onFatal = opts.onFatal;
  this.shutdownSignal = opts.shutdownSignal;
  this.stopRef = opts.stopRef;
}
```

### describe()
**Location**: `src/headends/headend-manager.ts:56-58`

```typescript
describe(): HeadendDescription[] {
  return this.headends.map((h) => h.describe());
}
```

Returns descriptions of all managed headends.

### startAll()
**Location**: `src/headends/headend-manager.ts:60-65`

```typescript
async startAll(): Promise<void> {
  await this.headends.reduce(async (prev, headend) => {
    await prev;
    await this.startHeadend(headend);
  }, Promise.resolve());
}
```

Sequential startup to ensure order.

### stopAll()
**Location**: `src/headends/headend-manager.ts:67-82`

```typescript
async stopAll(): Promise<void> {
  this.stopping = true;
  const stops = Array.from(this.active).map(async (headend) => {
    await headend.stop();
  });
  await Promise.allSettled(stops);
  await Promise.allSettled(this.watchers);
  if (!this.fatalResolved) {
    this.fatalResolved = true;
    this.fatalDeferred.resolve(undefined);
  }
}
```

Concurrent shutdown of all active headends.

### waitForFatal()
**Location**: `src/headends/headend-manager.ts:84-86`

```typescript
waitForFatal(): Promise<HeadendFatalEvent | undefined> {
  return this.fatalDeferred.promise;
}
```

Blocks until fatal error or graceful shutdown.

## Individual Headend Management

### startHeadend()
**Location**: `src/headends/headend-manager.ts:88-108`

```typescript
async startHeadend(headend): Promise<void> {
  const desc = headend.describe();
  const ctx: HeadendContext = {
    log: (entry) => {
      // Enrich with headendId and remoteIdentifier
      this.log(enrichedEntry);
    },
    shutdownSignal: this.shutdownSignal,
    stopRef: this.stopRef,
  };
  await headend.start(ctx);
  this.active.add(headend);
  this.watchers.push(this.watchHeadend(headend));
}
```

Context provided:
- Log function (enriched)
- Shutdown signal
- Stop reference

### watchHeadend()
**Location**: `src/headends/headend-manager.ts:110-128`

```typescript
async watchHeadend(headend): Promise<void> {
  const event = await headend.closed;
  this.active.delete(headend);
  if (event.reason === 'error') {
    this.registerFatal(headend, event.error);
  } else if (!event.graceful && !this.stopping) {
    this.registerFatal(headend, new Error('headend stopped unexpectedly'));
  }
}
```

Monitors closed promise for:
- Error termination → fatal
- Unexpected stop → fatal
- Graceful stop → ignore

## Fatal Error Handling

### registerFatal()
**Location**: `src/headends/headend-manager.ts:130-136`

```typescript
registerFatal(headend, error): void {
  if (this.fatalResolved) return;
  this.fatalResolved = true;
  const event = { headend, error };
  this.onFatal?.(event);
  this.fatalDeferred.resolve(event);
}
```

First fatal wins:
1. Set resolved flag
2. Invoke onFatal callback
3. Resolve deferred promise

## Log Emission

**Location**: `src/headends/headend-manager.ts:138-154`

```typescript
emit(headend, severity, direction, message, fatal = false): void {
  const entry: LogEntry = {
    timestamp: Date.now(),
    severity,
    turn: 0,
    subturn: 0,
    direction,
    type: 'tool',
    remoteIdentifier: `headend:${headend.kind}`,
    fatal,
    message: `${desc.label}: ${message}`,
    headendId: info.id,
  };
  this.log(entry);
}
```

Standardized log format for headend events.

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `log` | Log sink function |
| `onFatal` | Fatal error callback |
| `shutdownSignal` | Abort signal for shutdown |
| `stopRef` | Stopping flag reference |

## Telemetry

**Events tracked**:
- Headend start success
- Headend stop (graceful/error)
- Fatal error registration
- Watcher completion

## Logging

**Severity levels**:
- `WRN`: Stop failure
- `ERR`: Fatal error (with fatal=true)

**Remote identifier**: `headend:{kind}`

## Events

**Fatal event emission**:
1. Headend closes with error
2. Manager creates HeadendFatalEvent
3. Invokes onFatal callback
4. Resolves fatalDeferred

## Invariants

1. **Sequential start**: Headends start one by one
2. **Concurrent stop**: All headends stop in parallel
3. **First fatal wins**: Only first fatal event registered
4. **Active tracking**: Set maintains running headends
5. **Watcher cleanup**: All watchers awaited on stop
6. **Context isolation**: Each headend gets own context

## Business Logic Coverage (Verified 2025-11-16)

- **Sequential startup, concurrent teardown**: `startAll` reduces over headends to prevent port collisions, while `stopAll` awaits both `active` headends and `watchers` to guarantee watchers cannot fire after shutdown (`src/headends/headend-manager.ts:60-124`).
- **Fatal deduplication**: `fatalResolved` guard ensures the first `registerFatal` wins; `waitForFatal` resolves with either that event or `undefined` when `stopAll` completes without errors (`src/headends/headend-manager.ts:84-135`).
- **Graceful stop propagation**: `stopRef` is passed into every headend context; CLI `SIGINT` toggles it so headends can stop accepting new requests even before `stopAll` runs (`src/headends/headend-manager.ts:48-72`).
- **Unexpected exits**: `watchHeadend` treats non-graceful closes as fatal even without explicit errors, emitting synthesized `headend stopped unexpectedly` exceptions so operators are alerted (`src/headends/headend-manager.ts:110-128`).

## Test Coverage

**Phase 2**:
- Sequential startup
- Concurrent shutdown
- Fatal error propagation
- Log enrichment
- Active set management

**Gaps**:
- Multiple concurrent fatals
- Partial startup failure
- Signal propagation testing
- Long-running watcher scenarios

## Troubleshooting

### Headend not starting
- Check headend.start() implementation
- Verify context parameters
- Review sequential ordering

### Fatal not registered
- Check if already resolved
- Verify closed promise behavior
- Review error vs graceful stop

### Stop hanging
- Check Promise.allSettled completion
- Verify watcher promises resolve
- Review headend.stop() implementation

### Log not appearing
- Check log sink function
- Verify headendId enrichment
- Review message formatting
