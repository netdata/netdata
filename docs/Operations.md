# Operations

Monitor, debug, and maintain AI Agent deployments in production environments.

---

## Table of Contents

- [Pages in This Section](#pages-in-this-section) - Complete list of operations topics
- [Quick Diagnostics](#quick-diagnostics) - Fast commands to check agent health
- [Key Directories and Files](#key-directories-and-files) - Important file locations
- [Environment Variables](#environment-variables) - Runtime configuration
- [See Also](#see-also) - Related documentation

---

## Pages in This Section

| Page                                           | Description                                                |
| ---------------------------------------------- | ---------------------------------------------------------- |
| [Logging](Operations-Logging)                  | Structured logging system, severity levels, output formats |
| [Debugging Guide](Operations-Debugging)        | Step-by-step debugging workflow for common issues          |
| [Session Snapshots](Operations-Snapshots)      | Session state capture and post-mortem analysis             |
| [Tool Output Handling](Operations-Tool-Output) | Large response storage and extraction                      |
| [Telemetry](Operations-Telemetry)              | Prometheus metrics, OpenTelemetry traces, OTLP export      |
| [Accounting](Operations-Accounting)            | Token usage and cost tracking                              |
| [Exit Codes](Operations-Exit-Codes)            | Exit code reference for scripting and CI/CD                |
| [Troubleshooting](Operations-Troubleshooting)  | Problem/Cause/Solution reference                           |

---

## Quick Diagnostics

### Validate Configuration

```bash
ai-agent --agent myagent.ai --dry-run
```

Validates configuration without making LLM calls. Shows:

- Parsed frontmatter
- Resolved model chain
- Available tools
- Configuration errors

### Enable Verbose Output

```bash
ai-agent --agent myagent.ai --verbose "test query"
```

Shows turn-by-turn progress:

```
VRB 1.0 → LLM main: openai/gpt-4o messages 3, 1523 bytes
VRB 1.0 ← LLM main: LLM response received [2341ms, tokens: in 1523, out 456]
VRB 1.0 → MCP github:github search_code
VRB 1.0 ← MCP github:github search_code [523ms, 12456 bytes]
FIN 1.0 ← LLM main: requests=2 failed=0, tokens prompt=3046 output=912 cacheR=0 cacheW=0 total=3958
```

### Full Tracing

```bash
ai-agent --agent myagent.ai --trace-llm --trace-mcp "test query"
```

Shows complete request/response payloads for debugging protocol issues.

### Debug Environment Variables

```bash
DEBUG=true CONTEXT_DEBUG=true ai-agent --agent myagent.ai "test query"
```

Enables internal debugging output for:

- AI SDK internals (`DEBUG=true`)
- Context window budget tracking (`CONTEXT_DEBUG=true`)

---

## Key Directories and Files

| Path                             | Purpose                      |
| -------------------------------- | ---------------------------- |
| `~/.ai-agent/sessions/*.json.gz` | Session snapshots (gzipped)  |
| `~/.ai-agent/accounting.jsonl`   | Token/cost accounting ledger |
| `~/.ai-agent/cache.db`           | Response cache database      |
| `/opt/neda/.ai-agent/sessions/`  | Production session snapshots |

### Session Snapshot Filename

Snapshots are named by origin transaction ID:

```
~/.ai-agent/sessions/756b8ce8-3ad8-4a5a-8094-e45f0ba23a11.json.gz
```

Find the latest snapshot:

```bash
ls -lt ~/.ai-agent/sessions/*.json.gz | head -1
```

---

## Environment Variables

| Variable               | Purpose                                   | Default        |
| ---------------------- | ----------------------------------------- | -------------- |
| `DEBUG`                | Enable AI SDK debug output                | `false`        |
| `CONTEXT_DEBUG`        | Enable context window debugging           | `false`        |
| `AI_TELEMETRY_DISABLE` | Disable telemetry collection              | `false`        |
| `HOME`                 | User home directory for config resolution | System default |

### Example: Production Debug Session

```bash
DEBUG=true \
CONTEXT_DEBUG=true \
ai-agent --agent production.ai \
  --verbose \
  --trace-llm \
  "diagnose slow responses" 2> debug.log
```

---

## Production Monitoring Checklist

1. **Enable accounting** for cost tracking
2. **Configure Prometheus** endpoint for metrics
3. **Set up log rotation** for accounting files
4. **Monitor exit codes** in orchestration scripts
5. **Archive snapshots** for post-mortem analysis

---

## See Also

- [specs/logging-overview.md](specs/logging-overview.md) - Logging technical specification
- [specs/snapshots.md](specs/snapshots.md) - Snapshot technical specification
- [specs/telemetry-overview.md](specs/telemetry-overview.md) - Telemetry technical specification
- [skills/ai-agent-session-snapshots.md](skills/ai-agent-session-snapshots.md) - Complete snapshot extraction guide
