# Cursor

Configure Cursor IDE to access your Netdata infrastructure through MCP.

## Prerequisites

1. **Cursor installed** - Download from [cursor.com](https://www.cursor.com)
2. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
3. **`nd-mcp` program available on your desktop or laptop** - This is the bridge that translates `stdio` to `websocket`, connecting your AI Client to your Netdata Agent or Parent. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
4. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Configuration

1. Open Cursor
2. Navigate to Settings:
   - **Windows/Linux**: File → Preferences → Settings (or `Ctrl+,`)
   - **macOS**: Cursor → Preferences → Settings (or `Cmd+,`)
3. Search for "MCP" in settings
4. Add your Netdata configuration to MCP Servers

The configuration format:

```json
{
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
      "args": ["ws://prod-parent:19999/mcp?api_key=PROD_KEY"]
    },
    "netdata-dev": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://dev-parent:19999/mcp?api_key=DEV_KEY"]
    }
  }
}
```

Use the toggle in settings to enable only the environment you need.

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
