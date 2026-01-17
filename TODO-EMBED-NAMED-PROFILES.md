# Embed Headend Named Profiles

## TL;DR
Refactor embed headend to support named profiles via `--embed name:port` CLI format, allowing multiple distinct embed configurations.

## Current State
- `--embed <port>` accepts only a port number
- All embed headends share the same config from `embed: { ... }` in JSON
- Cannot have different `allowedAgents` or CORS for different ports

## Target State
- `--embed name:port` format (e.g., `--embed support:8090`)
- Each name maps to a profile in JSON config
- Different profiles can have different `allowedAgents`, CORS, rate limits, etc.

## Example

**CLI:**
```bash
ai-agent --embed support:8090 --embed sales:8091
```

**JSON (.ai-agent.json):**
```json
{
  "embed": {
    "support": {
      "allowedAgents": ["support"],
      "corsOrigins": ["*.support.example.com"]
    },
    "sales": {
      "allowedAgents": ["sales"],
      "corsOrigins": ["*.sales.example.com"]
    }
  }
}
```

## Analysis

### Files to Modify

1. **src/types.ts**
   - Change `embed?: EmbedHeadendConfig` to `embed?: Record<string, EmbedHeadendConfig>`
   - Remove `port` field from `EmbedHeadendConfig` (unused)
   - Remove `enabled` field (presence in config = enabled)

2. **src/cli.ts**
   - Update `--embed` option parser to accept `name:port` format
   - Change `embedPorts: number[]` to `embedTargets: Array<{name: string, port: number}>`
   - Update `runHeadendMode` to pass profile name to EmbedHeadend
   - Update validation to require `name:port` format

3. **src/headends/embed-headend.ts**
   - Update constructor to accept profile name
   - Look up config by profile name
   - Update `id` and `label` to include profile name

4. **docs/Headends-Embed.md**
   - Update CLI examples
   - Update configuration examples
   - Update configuration options table

5. **docs/specs/headend-embed.md**
   - Update spec to match new format

6. **docs/skills/ai-agent-guide.md**
   - Update embed config example

## Decisions

1. **CLI format:** `--embed name:port` (colon separator, like `--mcp http:8801`)
2. **Backwards compatibility:** None - always require `name:port` format
3. **Port in JSON:** No - port always comes from CLI

## Plan

- [x] Update types.ts - change embed config to Record<string, ...>
- [x] Update cli.ts - parse name:port format
- [x] Update embed-headend.ts - accept profile name, lookup config
- [x] Update documentation
- [x] Build and lint
- [x] Deploy to neda (production)

## Neda Deployment

Completed:
1. Added `embed.support` profile to `neda/.ai-agent.json`
2. Ran `./build-and-install.sh && sudo neda/neda-setup.sh`
3. Updated all 8 systemd service files with `--embed support:PORT`:
   - neda.service: 8807
   - neda-gpt.service: 8817
   - neda-codex.service: 8827
   - neda-gemini.service: 8837
   - neda-haiku.service: 8847
   - neda-sonnet.service: 8857
   - neda-glm.service: 8867
   - neda-minimax.service: 8877
4. Ran `systemctl daemon-reload`
5. Restarted all services - all active
6. Verified all embed ports are listening

## Status: COMPLETE
