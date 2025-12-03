# Cursor

Configure Cursor IDE to access your Netdata infrastructure through MCP.

## Transport Support

Cursor’s MCP client natively supports multiple transports (https://cursor.com/docs/context/mcp):

| Transport | Support | Netdata Version | Notes |
|-----------|---------|-----------------|-------|
| **stdio** | ✅ Fully Supported | v2.6.0+ | Launch Netdata via `nd-mcp` or `npx mcp-remote` |
| **SSE** | ✅ Fully Supported | v2.7.2+ | Configure `type: "sse"` with Netdata SSE endpoint |
| **Streamable HTTP** | ✅ Fully Supported | v2.7.2+ | Configure `type: "streamable-http"` for Netdata HTTP endpoint |
| **WebSocket** | ❌ Not Supported | - | Use the stdio bridge for v2.6.0–v2.7.1 |

## Prerequisites

1. **Cursor installed** - Download from [cursor.com](https://www.cursor.com)
2. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent to get infrastructure level visibility. Your AI Client (running on your desktop or laptop) needs to have direct network access to the Netdata IP and port (usually 19999).
   - **v2.6.0 - v2.7.1**: Only WebSocket transport is available, so launch Netdata through `nd-mcp`
   - **v2.7.2+**: Expose Netdata over SSE or HTTP directly, or continue to use `nd-mcp`
3. **Optional bridge** - `npx mcp-remote@latest` remains useful if you prefer stdio-only setups or want to re-use the same launcher for multiple clients.
4. **Netdata MCP API key loaded into the environment** (recommended) - export it before launching Cursor:
   ```bash
   export ND_MCP_BEARER_TOKEN="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"
   ```
   Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/netdata-ai/mcp/README.md#finding-your-api-key)

## Configuration Methods

Cursor reads MCP definitions from `.cursor/mcp.json` in the workspace root. For user-wide defaults, open Cursor’s Settings and add the same structure to the global config path documented by Cursor (https://cursor.com/docs/context/mcp#configuration-locations).

### Method 1: stdio Bridge (All Netdata versions)

```json
{
  "mcpServers": {
    "netdata": {
      "type": "stdio",
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "ws://YOUR_NETDATA_IP:19999/mcp"
      ]
    }
  }
}
```

### Method 2: Direct SSE (Netdata v2.7.2+)

```json
{
  "mcpServers": {
    "netdata": {
      "type": "sse",
      "url": "https://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      }
    }
  }
}
```

### Method 3: Streamable HTTP (Netdata v2.7.2+)

```json
{
  "mcpServers": {
    "netdata": {
      "type": "streamable-http",
      "url": "https://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      }
    }
  }
}
```

> Cursor supports config interpolation such as `${env:NETDATA_MCP_API_KEY}` or `${workspaceFolder}` inside `command`, `args`, `env`, `url`, and `headers` (https://cursor.com/docs/context/mcp#config-interpolation). Use these to avoid storing secrets in plain text.

After editing `.cursor/mcp.json`, restart Cursor or run “Reload Window” for the new server to appear in **Settings → MCP**.

## Using Netdata in Cursor

### In Chat (Cmd+K)

Reference Netdata directly in your queries:

```
@netdata what's the current CPU usage?
@netdata show me database query performance
@netdata are there any anomalies in the web servers?
```

### In Code Comments

Get infrastructure context while coding:

```python
# @netdata what's the typical memory usage of this service?
def process_large_dataset():
    # Implementation
```

### Multi-Model Support

Cursor's strength is using multiple AI models. You can:

- Use Claude for complex analysis
- Switch to GPT-4 for different perspectives
- Use smaller models for quick queries

All models can access your Netdata data through MCP.

## Multiple Environments

Cursor allows multiple MCP servers but requires manual toggling:

```json
{
  "mcpServers": {
    "netdata-prod": {
      "type": "stdio",
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://prod-parent:19999/mcp"]
    },
    "netdata-dev": {
      "type": "stdio",
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://dev-parent:19999/mcp"]
    }
  }
}
```

Use the toggle in settings to enable only the environment you need.

> ℹ️ Before switching environments, set `ND_MCP_BEARER_TOKEN` to the matching key so the bridge picks up the correct credentials without embedding them in the config file.

## Best Practices

### Infrastructure-Aware Development

While coding, ask about:

- Current resource usage of services you're modifying
- Historical performance patterns
- Impact of deployments on system metrics

### Debugging with Context

```
@netdata show me the logs when this error last occurred
@netdata what was the system state during the last deployment?
@netdata find correlated metrics during the performance regression
```

### Performance Optimization

```
@netdata analyze database query latency patterns
@netdata which endpoints have the highest response times?
@netdata show me resource usage trends for this service
```

## Troubleshooting

### MCP Server Not Available

- Restart Cursor after adding configuration
- Verify JSON syntax in settings
- Check MCP is enabled in Cursor settings

### Connection Issues

- Test Netdata accessibility: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Verify bridge path is correct and executable
- Check firewall allows connection to Netdata

### Multiple Servers Confusion

- Cursor may query the wrong server if multiple are enabled
- Always disable unused servers
- Name servers clearly (prod, dev, staging)

### Limited Functionality

- Ensure API key is included for full access
- Verify Netdata agent is claimed
- Check that required collectors are enabled
