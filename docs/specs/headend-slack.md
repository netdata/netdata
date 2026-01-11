# Slack Headend

## TL;DR
Slack Socket Mode integration supporting mentions, DMs, channel posts with routing, concurrency limits, and session management.

## Source Files
- `src/headends/slack-headend.ts` - Full implementation
- `src/server/slack.js` - initSlackHeadend helper
- `@slack/bolt` - External Slack SDK
- `src/server/session-manager.ts` - Session orchestration

## Headend Identity
- **ID**: `slack:socket`
- **Kind**: `slack`
- **Label**: `Slack Socket Mode`

## Configuration

### SlackHeadendOptions
```typescript
interface SlackHeadendOptions {
  agentPaths: string[];           // Agent config paths
  loadOptions: LoadAgentOptions;  // Agent loading options
  verbose?: boolean;              // Enable verbose logging
  traceLLM?: boolean;             // Trace LLM calls
  traceMCP?: boolean;             // Trace MCP interactions
  traceSdk?: boolean;             // Trace SDK operations
  traceSlack?: boolean;           // Trace Slack API calls
}
```

### Slack Config (from agent frontmatter)
```typescript
interface SlackConfig {
  botToken: string;        // Slack bot token
  appToken: string;        // Slack app token
  signingSecret?: string;  // Signing secret for verification
  historyLimit?: number;   // Max history messages (default: 100)
  historyCharsCap?: number; // Max chars in history (default: 100000)
  updateIntervalMs?: number; // Update interval (default: 2000)
  mentions?: boolean;      // Enable @mentions (default: true)
  dms?: boolean;           // Enable DMs (default: true)
  openerTone?: SlackOpenerTone; // Response tone
}
```

## Routing

### CompiledRoute
```typescript
interface CompiledRoute {
  channelNamePatterns: RegExp[];
  channelIdPatterns: RegExp[];
  agentPath: string;
  engage: Set<'mentions' | 'channel-posts' | 'dms'>;
  promptTemplates?: {
    mention?: string;
    dm?: string;
    channelPost?: string;
  };
  contextPolicy?: {
    channelPost?: 'selfOnly' | 'previousOnly' | 'selfAndPrevious';
  };
}
```

### Resolution Flow
```typescript
resolveRoute(args): Promise<RoutingResolution | undefined> {
  // Match channel name/id patterns
  // Check engage set for message type
  // Apply deny rules
  // Return session manager and config
}
```

## Construction

**Location**: `src/headends/slack-headend.ts:107-118`

```typescript
constructor(options) {
  this.agentPaths = options.agentPaths;
  this.loadOptions = options.loadOptions;
  this.verbose = options.verbose;
  this.traceLLM = options.traceLLM;
  this.traceMCP = options.traceMCP;
  this.traceSdk = options.traceSdk;
  this.traceSlack = options.traceSlack;
  this.closed = this.closeDeferred.promise;
  this.telemetryLabels = { ...getTelemetryLabels(), headend: this.id };
  this.label = 'Slack Socket Mode';
}
```

## Startup Flow

**Location**: `src/headends/slack-headend.ts:132-200`

1. **Validate configuration**:
   - Require at least one agent path
   - Load primary agent
   - Extract Slack tokens

2. **Initialize agents**:
   - Load each agent path
   - Create session managers
   - Build routing resolver

3. **Create Slack App**:
   ```typescript
   const slackApp = new App({
     token: slackBotToken,
     appToken: slackAppToken,
     socketMode: true,
     logLevel: traceSlack ? LogLevel.DEBUG : LogLevel.WARN,
   });
   ```

4. **Configure handlers**:
   - Set history limits
   - Configure update intervals
   - Enable/disable mentions and DMs
   - Set opener tone

5. **Initialize Slack integration**:
   ```typescript
   initSlackHeadend({
     sessionManager: defaultSessions,
     app: this.slackApp,
     historyLimit,
     historyCharsCap,
     updateIntervalMs,
     enableMentions,
     enableDMs,
     systemPrompt,
     verbose,
     openerTone,
     resolveRoute: this.resolveRoute,
     acquireRunSlot: () => this.slackLimiter.acquire(),
     registerRunSlot: (session, runId, release) => {
       this.registerRunRelease(session, runId, release);
     },
   });
   ```

6. **Create slash command route**

## Concurrency Management

### ConcurrencyLimiter
**Location**: Instantiated at class level

```typescript
private readonly slackLimiter = new ConcurrencyLimiter(10);
```

Limits: 10 concurrent Slack runs

### Slot Registration
```typescript
private readonly runReleases = new Map<string, () => void>();

registerRunRelease(session, runId, release) {
  this.runReleases.set(runId, release);
}
```

Tracks active runs for cleanup.

## Agent Caching

### LoadedAgentRecord
```typescript
interface LoadedAgentRecord {
  loaded: LoadedAgent;
  sessions: SessionManager;
}

private readonly agentCache = new Map<string, LoadedAgentRecord>();
private readonly loaderCache = new LoadedAgentCache();
```

Caches:
- Loaded agent configurations
- Session managers per agent
- User label lookups

## Message Types

### Mentions
- Bot @mentioned in channel
- Enabled by default
- Uses mention prompt template

### DMs
- Direct messages to bot
- Enabled by default
- Uses dm prompt template

## Business Logic Coverage (Verified 2025-11-16)

- **Routing matrix**: `compileRouting` supports `default`, `rules`, and `deny` blocks with glob-style channel IDs/names plus `engage` filters (`mentions`, `channel-posts`, `dms`); deny rules short-circuit resolution so sensitive channels can block the bot entirely (`src/headends/slack-headend.ts:538-670`).
- **Prompt templating**: Route entries can specify `promptTemplates` per channel engagement type so mentions/DMs can inject extra context before the agent run, matching the behavior described in `docs/SPECS.md` (`src/headends/slack-headend.ts:570-612`).
- **Opener tones**: `parseOpenerTone` supports `random`, `cheerful`, `formal`, and `busy` preset openers, ensuring multi-channel deployments have consistent “starting…” cues (`src/headends/slack-headend.ts:182-210, 373-390`).
- **Slash command fallback**: The headend registers an extra route (REST headend if present, or ad-hoc HTTP server) for `/ai-agent` slash commands with signature verification and slack signing secret enforcement (`src/headends/slack-headend.ts:98-305`).
- **Run-slot cleanup**: Each run stores a release callback keyed by Slack `runId`; aborts or completion events call `release` so the `ConcurrencyLimiter` never leaks slots even if Slack closes Socket Mode connections abruptly (`src/headends/slack-headend.ts:210-270`).

### Channel Posts
- Messages in monitored channels
- Requires routing configuration
- Context policy controls history

## Opener Tones

**Location**: parseOpenerTone method

```typescript
type SlackOpenerTone = 'random' | 'cheerful' | 'formal' | 'busy';
```

Controls initial response style.

## Slash Commands

**Location**: createSlashCommandRoute method

Provides REST route for slash command integration when combined with REST headend.

## Shutdown Handling

**Location**: handleShutdownSignal method

```typescript
handleShutdownSignal() {
  if (this.stopping) return;
  this.stopping = true;
  // Release all active runs
  // Stop Slack app
  // Close deferred promise
}
```

Cleanup:
1. Set stopping flag
2. Release run slots
3. Stop Slack app
4. Resolve closed promise

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `agentPaths` | Available agent configurations |
| `botToken` | Slack bot authentication |
| `appToken` | Socket mode app token |
| `historyLimit` | Max messages fetched |
| `historyCharsCap` | Max history size |
| `updateIntervalMs` | Status update frequency |
| `mentions` | Enable @bot mentions |
| `dms` | Enable direct messages |
| `openerTone` | Response personality |

## Telemetry

**Labels added**:
- `headend: slack:socket`
- All base telemetry labels

## Logging

**Via context.log**:
- Startup progress
- Route resolution
- Error conditions
- Shutdown events

## Events

**Headend lifecycle**:
- `closed`: Promise resolving on termination
- With reason ('error' or graceful)
- With error details if applicable

## Invariants

1. **Socket mode required**: Uses @slack/bolt socket mode
2. **Primary agent required**: At least one agent path
3. **Token validation**: Both bot and app tokens required
4. **Concurrency limit**: Max 10 concurrent runs
5. **Session isolation**: Each agent gets own session manager
6. **Graceful shutdown**: Cleanup on abort signal

## Undocumented Behaviors

1. **Per-channel agent routing**:
   - Default routes, deny rules, channel patterns
   - Glob pattern matching for channel names/IDs
   - Per-route promptTemplates and contextPolicy
   - Location: `src/headends/slack-headend.ts:579-654`

2. **Slash command support**:
   - Full support with signature verification
   - Fallback HTTP server when not registered to REST headend
   - Location: `src/headends/slack-headend.ts:267-311, 484-496`

3. **User label lookup and caching**:
   - Caches display name, real name, email
   - Fetches from Slack API on miss
   - Location: `src/headends/slack-headend.ts:449-472`

4. **Opener tones**:
   - Options: random, cheerful, formal, busy
   - Generates varied response openers
   - Location: `src/headends/slack-headend.ts:openerTone`

5. **Agent preloading**:
   - Loads all routed agents at startup
   - Validates agent configs early
   - Creates SessionManager per agent path

6. **Live message updates**:
   - updateIntervalMs for progressive rendering
   - Uses Slack `chat.update` API
   - Thread timestamp tracking

7. **History character cap**:
   - historyCharsCap limits total history size
   - Prevents context overflow

8. **Block Kit output support**:
   - 'slack-block-kit' format produces native Slack blocks
   - Rich formatting preserved

## Test Coverage

**Phase 2**:
- Agent loading
- Route compilation
- Concurrency limiting
- Startup sequence
- Shutdown cleanup

**Gaps**:
- Multi-workspace scenarios
- Route matching edge cases
- History fetching errors
- Rate limiting behavior
- Opener tone variations
- Slash command verification

## Troubleshooting

### Bot not responding
- Check token validity
- Verify socket mode enabled
- Review routing rules
- Check mentions/dms enabled

### Wrong agent responding
- Review route patterns
- Check channel name matching
- Verify engage sets

### Messages truncated
- Check historyCharsCap
- Review historyLimit
- Verify update intervals

### Concurrency blocked
- Check slackLimiter limit
- Verify run releases
- Review active sessions

### Shutdown hanging
- Check stopRef propagation
- Verify signal listeners
- Review run cleanup
