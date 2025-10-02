# Cursor

Configure Cursor IDE to access your Netdata infrastructure through MCP.

## Transport Support

Cursor currently supports stdio-based MCP servers only:

| Transport | Support | Netdata Version | Use Case |
|-----------|---------|-----------------|----------|
| **stdio** (via nd-mcp bridge) | ✅ Fully Supported | v2.6.0+ | Local bridge to WebSocket |
| **stdio** (via npx remote-mcp) | ✅ Fully Supported | v2.7.2+ | Alternative bridge with HTTP/SSE support |
| **Streamable HTTP** | ❌ Not Supported | - | Use npx remote-mcp bridge |
| **SSE** (Server-Sent Events) | ❌ Not Supported | - | Use npx remote-mcp bridge |
| **WebSocket** | ❌ Not Supported | - | Use nd-mcp bridge |

> **Note:** Cursor only supports stdio-based MCP servers. For HTTP/SSE connections to Netdata v2.7.2+, you must use a bridge like npx remote-mcp. For older Netdata versions (v2.6.0 - v2.7.1), use the nd-mcp bridge with WebSocket.

## Prerequisites

1. **Cursor installed** - Download from [cursor.com](https://www.cursor.com)
2. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent to get infrastructure level visibility. Your AI Client (running on your desktop or laptop) needs to have direct network access to the Netdata IP and port (usually 19999).
   - **v2.6.0 - v2.7.1**: Only WebSocket transport available, requires `nd-mcp` bridge
   - **v2.7.2+**: Can use `npx mcp-remote` bridge for HTTP/SSE support
3. **Bridge required: Choose one:**
   - `nd-mcp` bridge - The stdio-to-websocket bridge for all Netdata versions. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
   - `npx mcp-remote@latest` - Official MCP remote client supporting HTTP/SSE (requires Netdata v2.7.2+)
4. **Netdata MCP API key loaded into the environment** (recommended) - export it before launching Cursor:
   ```bash
   export ND_MCP_BEARER_TOKEN="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"
   ```
   Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Configuration Methods

Cursor uses a `.cursor/mcp.json` configuration file for MCP servers. You can place this file in two locations:

- **Project-specific**: `.cursor/mcp.json` in your project root directory
- **Global**: In your user configuration directory

### Method 1: Using nd-mcp Bridge

For all Netdata versions (v2.6.0+):

Create or edit `.cursor/mcp.json` in your project directory:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "ws://YOUR_NETDATA_IP:19999/mcp"
      ]
    }
  }
}
```

### Method 2: Using npx remote-mcp (Recommended for v2.7.2+)

For Netdata v2.7.2+ with HTTP/SSE support. For detailed options and troubleshooting, see [Using MCP Remote Client](/docs/learn/mcp.md#using-mcp-remote-client).

**HTTP transport:**

```json
{
  "mcpServers": {
    "netdata": {
      "command": "npx",
      "args": [
        "mcp-remote@latest",
        "--http",
        "http://YOUR_NETDATA_IP:19999/mcp",
        "--allow-http",
        "--header",
        "Authorization: Bearer NETDATA_MCP_API_KEY"
      ]
    }
  }
}
```

**SSE transport:**

```json
{
  "mcpServers": {
    "netdata": {
      "command": "npx",
      "args": [
        "mcp-remote@latest",
        "--sse",
        "http://YOUR_NETDATA_IP:19999/mcp",
        "--allow-http",
        "--header",
        "Authorization: Bearer NETDATA_MCP_API_KEY"
      ]
    }
  }
}
```

After creating the configuration file, restart Cursor for changes to take effect.

Replace in all examples:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (nd-mcp method only)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `ND_MCP_BEARER_TOKEN` - Export with your API key before launching Cursor (nd-mcp method only)

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
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://prod-parent:19999/mcp"]
    },
    "netdata-dev": {
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
