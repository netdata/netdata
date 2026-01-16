# Operations & Debugging

Monitor, debug, and troubleshoot AI Agent deployments.

---

## Pages in This Section

| Page | Description |
|------|-------------|
| [Debugging Guide](Operations-Debugging) | Step-by-step debugging workflow |
| [Logging](Operations-Logging) | Structured logging system |
| [Session Snapshots](Operations-Snapshots) | Session capture and analysis |
| [Tool Output Handles](Operations-Tool-Output) | Large response handling |
| [Telemetry](Operations-Telemetry) | Metrics and tracing |
| [Accounting](Operations-Accounting) | Cost and usage tracking |
| [Exit Codes](Operations-Exit-Codes) | Exit code reference |
| [Troubleshooting](Operations-Troubleshooting) | Common issues and solutions |

---

## Quick Diagnostics

### Check Agent Status

```bash
ai-agent --agent test.ai --dry-run
```

### Verbose Output

```bash
ai-agent --agent test.ai --verbose "test query"
```

### Full Tracing

```bash
ai-agent --agent test.ai --trace-llm --trace-mcp "test query"
```

### Debug Environment

```bash
DEBUG=true CONTEXT_DEBUG=true ai-agent --agent test.ai "test query"
```

---

## Key Files

| File | Purpose |
|------|---------|
| `~/.ai-agent/sessions/*.json.gz` | Session snapshots |
| `~/.ai-agent/cache.db` | Response cache |
| Accounting JSONL | Token/cost tracking |

---

## See Also

- [specs/LOGS.md](specs/LOGS.md) - Detailed logging guide
- [skills/ai-agent-session-snapshots.md](skills/ai-agent-session-snapshots.md) - Snapshot extraction
