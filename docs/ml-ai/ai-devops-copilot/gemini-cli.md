# Gemini CLI

Configure Google's Gemini CLI to access your Netdata infrastructure through MCP for powerful AI-driven operations.

## Transport Support

Gemini CLI supports all major MCP transport types, giving you maximum flexibility:

| Transport | Support | Use Case |
|-----------|---------|----------|
| **stdio** (via nd-mcp bridge) | ✅ Fully Supported | Local bridge to WebSocket |
| **Streamable HTTP** | ✅ Fully Supported | Direct connection to Netdata's HTTP endpoint |
| **SSE** (Server-Sent Events) | ✅ Fully Supported | Direct connection to Netdata's SSE endpoint |
| **WebSocket** | ❌ Not Supported | Use nd-mcp bridge or HTTP/SSE instead |

## Prerequisites

1. **Gemini CLI installed** - Available from [GitHub](https://github.com/google-gemini/gemini-cli)
2. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
3. **For stdio connections only: `nd-mcp` bridge** - The stdio-to-websocket bridge. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge). Not needed for direct HTTP/SSE connections.
4. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Installation

```bash
# Run Gemini CLI directly from GitHub
npx https://github.com/google-gemini/gemini-cli

# Or clone and install locally
git clone https://github.com/google-gemini/gemini-cli.git
cd gemini-cli
npm install
npm run build
```

## Configuration Methods

Gemini CLI has built-in MCP server support. For detailed MCP configuration, see the [official MCP documentation](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md).

### Method 1: Direct HTTP Connection (Recommended)

Connect directly to Netdata's HTTP endpoint without needing any bridge:

```bash
# Using CLI command
gemini mcp add --transport http netdata http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY

# For HTTPS connections
gemini mcp add --transport http netdata https://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY
```

Or configure in `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "httpUrl": "http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY",
      "timeout": 30000
    }
  }
}
```

### Method 2: Direct SSE Connection

Connect directly to Netdata's SSE endpoint:

```bash
# Using CLI command
gemini mcp add --transport sse netdata http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY&transport=sse
```

Or configure in `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "url": "http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY&transport=sse",
      "timeout": 30000
    }
  }
}
```

### Method 3: Using nd-mcp Bridge (stdio)

For environments where you prefer or need to use the bridge:

```bash
# Using CLI command
gemini mcp add netdata /usr/sbin/nd-mcp ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY
```

Or configure in `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"],
      "timeout": 30000
    }
  }
}
```

### Method 4: Using npx remote-mcp (Alternative Bridge)

If nd-mcp is not available, use the official MCP remote client:

```bash
# Using CLI command with SSE
gemini mcp add netdata npx @modelcontextprotocol/remote-mcp \
  --sse http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY

# Using HTTP transport
gemini mcp add netdata npx @modelcontextprotocol/remote-mcp \
  --http http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY
```

Or configure in `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "npx",
      "args": [
        "@modelcontextprotocol/remote-mcp",
        "--sse",
        "http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"
      ]
    }
  }
}
```

## Environment Variables

Gemini CLI supports environment variable expansion in `settings.json`:
- `$VAR_NAME` or `${VAR_NAME}` - Expands to the value of environment variable

Example configuration with environment variables:

```json
{
  "mcpServers": {
    "netdata": {
      "httpUrl": "http://${NETDATA_HOST}:19999/mcp?api_key=${NETDATA_API_KEY}"
    }
  }
}
```

## Verify MCP Configuration

Use these commands to verify your setup:

```bash
# List all configured MCP servers
gemini mcp list

# Interactive MCP status (within Gemini session)
/mcp

# Show detailed descriptions of MCP servers and tools
/mcp desc

# Show MCP server schema details
/mcp schema
```

Replace in all examples:
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (stdio method only)

## How to Use

Gemini CLI can leverage Netdata's observability data for infrastructure analysis and automation:

```
What's the current system performance across all monitored servers?
Show me any performance anomalies in the last 2 hours
Which services are consuming the most resources right now?
Analyze the database performance trends over the past week
```

## Example Workflows

**Performance Investigation:**

```
Investigate why our application response times increased this afternoon
```

**Resource Optimization:**

```
Check memory usage patterns and suggest optimization strategies
```

**Alert Analysis:**

```
Explain the current active alerts and their potential impact
```

> **💡 Advanced Usage:** Gemini CLI can combine observability data with system automation for powerful DevOps workflows. Learn about the opportunities and security considerations in [AI DevOps Copilot](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md).

## Troubleshooting

### MCP Connection Issues

- Verify Netdata is accessible: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Check that the bridge path exists and is executable
- Ensure API key is correct and properly formatted

### Limited Data Access

- Verify API key is included in the connection string
- Check that the Netdata agent is properly configured for MCP
- Ensure network connectivity between Gemini CLI and Netdata

### Command Execution Problems

- Review command syntax for your specific Gemini CLI version
- Check MCP server configuration parameters
- Verify that MCP protocol is supported in your Gemini CLI installation

## Advanced Configuration

### Multiple Environments

Configure different Netdata instances for different purposes:

```json
{
  "mcpServers": {
    "netdata-prod": {
      "httpUrl": "https://prod-parent.company.com:19999/mcp?api_key=${PROD_API_KEY}"
    },
    "netdata-staging": {
      "httpUrl": "https://staging-parent.company.com:19999/mcp?api_key=${STAGING_API_KEY}"
    },
    "netdata-local": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://localhost:19999/mcp?api_key=${LOCAL_API_KEY}"]
    }
  }
}
```

### Tool Filtering

Control which Netdata tools are available:

```json
{
  "mcpServers": {
    "netdata": {
      "httpUrl": "http://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY",
      "includeTools": ["query_metrics", "list_alerts", "list_nodes"],
      "excludeTools": ["execute_function", "systemd_journal"]
    }
  }
}
```

## Documentation Links

- [Gemini CLI GitHub Repository](https://github.com/google-gemini/gemini-cli)
- [Gemini CLI MCP Documentation](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md)
- [Gemini CLI Configuration Guide](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/configuration.md)
- [Netdata MCP Setup](/docs/learn/mcp.md)
- [AI DevOps Best Practices](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md)
