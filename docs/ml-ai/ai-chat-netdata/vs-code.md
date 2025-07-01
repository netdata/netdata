# VS Code

Configure Visual Studio Code extensions to access your Netdata infrastructure through MCP.

## Available Extensions

### Continue (Recommended)

The most popular open-source AI code assistant with MCP support.

### Cline

Autonomous coding agent that can use MCP tools.

## Prerequisites

1. **VS Code installed** - [Download VS Code](https://code.visualstudio.com)
2. **MCP-compatible extension** - Install from VS Code Marketplace
3. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
4. **`nd-mcp` program available on your desktop or laptop** - This is the bridge that translates `stdio` to `websocket`, connecting your AI Client to your Netdata Agent or Parent. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
5. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Continue Extension Setup

### Installation

1. Open VS Code
2. Go to Extensions (Ctrl+Shift+X)
3. Search for "Continue"
4. Install the Continue extension
5. Reload VS Code

### Configuration

1. Open Command Palette (Ctrl+Shift+P)
2. Type "Continue: Open config.json"
3. Add Netdata MCP configuration:

```json
{
  "models": [
    {
      "title": "Claude 3.5 Sonnet",
      "provider": "anthropic",
      "model": "claude-3-5-sonnet-20241022",
      "apiKey": "YOUR_ANTHROPIC_KEY"
    }
  ],
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"
      ]
    }
  }
}
```

Replace:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `YOUR_ANTHROPIC_KEY` - Your Anthropic API key

### Usage

Press `Ctrl+L` to open Continue chat, then:

```
@netdata what's the current CPU usage?
@netdata show me memory trends for the last hour
@netdata are there any anomalies in the database servers?
```

## Cline Extension Setup

### Installation

1. Search for "Cline" in Extensions
2. Install and reload VS Code

### Configuration

1. Open Settings (Ctrl+,)
2. Search for "Cline MCP"
3. Add configuration:

```json
{
  "cline.mcpServers": [
    {
      "name": "netdata",
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"
      ]
    }
  ]
}
```

### Usage

1. Open Cline (Ctrl+Shift+P → "Cline: Open Chat")
2. Cline can autonomously:
   - Analyze performance issues
   - Create monitoring scripts
   - Debug based on metrics

Example:

```
Create a Python script that checks Netdata for high CPU usage and sends an alert
```

## Multiple Environments

### Workspace-Specific Configuration

Create `.vscode/settings.json` in your project:

```json
{
  "continue.mcpServers": {
    "netdata-prod": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://prod-parent:19999/mcp?api_key=PROD_NETDATA_MCP_API_KEY"]
    }
  }
}
```

### Environment Switching

Different projects can have different Netdata connections:

- `~/projects/frontend/.vscode/settings.json` → Frontend servers
- `~/projects/backend/.vscode/settings.json` → Backend servers
- `~/projects/infrastructure/.vscode/settings.json` → All servers

## Advanced Usage

### Custom Commands

Create custom VS Code commands that query Netdata:

```json
{
  "commands": [
    {
      "command": "netdata.checkHealth",
      "title": "Netdata: Check System Health"
    }
  ]
}
```

### Task Integration

Add Netdata checks to tasks.json:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Check Production Metrics",
      "type": "shell",
      "command": "continue",
      "args": ["--ask", "@netdata show current system status"]
    }
  ]
}
```

### Snippets with Metrics

Create snippets that include metric checks:

```json
{
  "Check Performance": {
    "prefix": "perf",
    "body": [
      "// @netdata: Current ${1:CPU} usage?",
      "$0"
    ]
  }
}
```

## Extension Comparison

| Feature | Continue | Cline | Codeium | Copilot Chat |
|---------|----------|-------|---------|--------------|
| MCP Support | ✅ Full | ✅ Full | ❓ Check | ❓ Future |
| Autonomous Actions | ❌ | ✅ | ❌ | ❌ |
| Multiple Models | ✅ | ✅ | ❌ | ❌ |
| Free Tier | ❌ | ❌ | ✅ | ❌ |
| Open Source | ✅ | ✅ | ❌ | ❌ |

## Troubleshooting

### Extension Not Finding MCP

- Restart VS Code after configuration
- Check extension logs (Output → Continue/Cline)
- Verify JSON syntax in settings

### Connection Issues

- Test Netdata: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Check bridge is executable
- Verify network access from VS Code

### No Netdata Option

- Ensure `@netdata` is typed correctly
- Check MCP server is configured
- Try reloading the window (Ctrl+R)

### Performance Problems

- Use local Netdata Parent for faster response
- Check extension memory usage
- Disable unused extensions

## Best Practices

### Development Workflow

1. Start coding with infrastructure context
2. Check metrics before optimization
3. Validate changes against production data
4. Monitor impact of deployments

### Team Collaboration

Share Netdata configurations:

- Commit `.vscode/settings.json` for project-specific configs
- Document which Netdata Parent to use
- Create team snippets for common queries
