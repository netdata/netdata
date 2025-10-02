# Crush

Configure Crush by Charmbracelet to access your Netdata infrastructure through MCP for glamorous terminal-based AI operations.

## Transport Support

Crush has comprehensive MCP transport support, making it highly flexible for connecting to Netdata:

| Transport | Support | Netdata Version | Use Case |
|-----------|---------|-----------------|----------|
| **stdio** (via nd-mcp bridge) | ✅ Fully Supported | v2.6.0+ | Local bridge to WebSocket |
| **Streamable HTTP** | ✅ Fully Supported | v2.7.2+ | Direct connection to Netdata's HTTP endpoint (recommended) |
| **SSE** (Server-Sent Events) | ✅ Fully Supported | v2.7.2+ | Direct connection to Netdata's SSE endpoint |
| **WebSocket** | ❌ Not Supported | - | Use nd-mcp bridge or HTTP/SSE instead |

## Prerequisites

1. **Crush installed** - Available via npm, Homebrew, or direct download from [GitHub](https://github.com/charmbracelet/crush)
2. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent to get infrastructure level visibility. Your AI Client (running on your desktop or laptop) needs to have direct network access to the Netdata IP and port (usually 19999).
   - **v2.6.0 - v2.7.1**: Only WebSocket transport available, requires `nd-mcp` bridge
   - **v2.7.2+**: Direct HTTP/SSE support available (recommended)
3. **For WebSocket or stdio connections: `nd-mcp` bridge** - The stdio-to-websocket bridge. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge). Not needed for direct HTTP/SSE connections on v2.7.2+.
4. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

> Export `ND_MCP_BEARER_TOKEN` with your MCP key before launching Crush so credentials never appear in command-line arguments or config files:
> ```bash
> export ND_MCP_BEARER_TOKEN="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"
> ```

## Installation

Install Crush using one of these methods:

```bash
# Homebrew (recommended for macOS)
brew install charmbracelet/tap/crush

# NPM
npm install -g @charmland/crush

# Arch Linux
yay -S crush-bin

# Windows (Winget)
winget install charmbracelet.crush

# Windows (Scoop)
scoop bucket add charm https://github.com/charmbracelet/scoop-bucket.git
scoop install crush

# Or install with Go
go install github.com/charmbracelet/crush@latest
```

## Configuration Methods

Crush uses JSON configuration files with the following priority:
1. `.crush.json` (project-specific)
2. `crush.json` (project-specific)
3. `~/.config/crush/crush.json` (global)

### Method 1: Direct HTTP Connection (Recommended for v2.7.2+)

Connect directly to Netdata's HTTP endpoint without needing the nd-mcp bridge:

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata": {
      "type": "http",
      "url": "http://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      },
      "timeout": 120,
      "disabled": false
    }
  }
}
```

For HTTPS connections:

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata": {
      "type": "http",
      "url": "https://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      },
      "timeout": 120
    }
  }
}
```

### Method 2: Direct SSE Connection (v2.7.2+)

Connect directly to Netdata's SSE endpoint for real-time streaming:

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata": {
      "type": "sse",
      "url": "http://YOUR_NETDATA_IP:19999/mcp?transport=sse",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      },
      "timeout": 120,
      "disabled": false
    }
  }
}
```

### Method 3: Using nd-mcp Bridge (stdio)

For environments where you prefer or need to use the bridge:

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata": {
      "type": "stdio",
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://YOUR_NETDATA_IP:19999/mcp"],
      "timeout": 120,
      "disabled": false
    }
  }
}
```

### Method 4: Using npx mcp-remote (Alternative Bridge for v2.7.2+)

If nd-mcp is not available, use the official MCP remote client (requires Netdata v2.7.2+). For detailed options and troubleshooting, see [Using MCP Remote Client](/docs/learn/mcp.md#using-mcp-remote-client).

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata": {
      "type": "stdio",
      "command": "npx",
      "args": [
        "mcp-remote@latest",
        "--http",
        "http://YOUR_NETDATA_IP:19999/mcp",
        "--allow-http",
        "--header",
        "Authorization: Bearer NETDATA_MCP_API_KEY"
      ],
      "timeout": 120
    }
  }
}
```

## Environment Variables

Crush supports environment variable expansion using `$(echo $VAR)` syntax:

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata": {
      "type": "http",
      "url": "http://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer $(echo $NETDATA_API_KEY)"
      },
      "timeout": 120
    }
  }
}
```

## Project-Based Configuration

Create project-specific configurations by placing `.crush.json` or `crush.json` in your project root:

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata-prod": {
      "type": "http",
      "url": "https://prod-parent.company.com:19999/mcp",
      "headers": {
        "Authorization": "Bearer $(echo $PROD_API_KEY)"
      },
      "timeout": 120
    },
    "netdata-staging": {
      "type": "sse",
      "url": "https://staging-parent.company.com:19999/mcp?transport=sse",
      "headers": {
        "Authorization": "Bearer $(echo $STAGING_API_KEY)"
      },
      "timeout": 120
    }
  }
}
```

Replace in all examples:
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (stdio method only)

## How to Use

Once configured, start Crush and it will automatically connect to your Netdata MCP servers:

```bash
# Start Crush
crush

# Ask infrastructure questions
What's the current CPU usage across all servers?
Show me any performance anomalies in the last hour
Which services are consuming the most resources?
```

## Tool Permissions

Crush asks for permission before running tools by default. You can pre-approve certain Netdata tools:

```json
{
  "$schema": "https://charm.land/crush.json",
  "permissions": {
    "allowed_tools": [
      "mcp_netdata_list_metrics",
      "mcp_netdata_query_metrics",
      "mcp_netdata_list_nodes",
      "mcp_netdata_list_alerts"
    ]
  }
}
```

> **⚠️ Warning:** Use the `--yolo` flag to bypass all permission prompts, but be extremely careful with this feature.

## Example Workflows

**Performance Investigation:**
```
Investigate why our application response times increased this afternoon using Netdata metrics
```

**Resource Optimization:**
```
Check memory usage patterns across all nodes and suggest optimization strategies
```

**Alert Analysis:**
```
Explain the current active alerts from Netdata and their potential impact
```

**Anomaly Detection:**
```
Find any anomalous metrics in the last 2 hours and explain what might be causing them
```

> **💡 Advanced Usage:** Crush can combine observability data with its terminal-based interface for powerful DevOps workflows. Learn about the opportunities and security considerations in [AI DevOps Copilot](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md).

## Troubleshooting

### MCP Server Not Connecting

- Verify Netdata is accessible: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Check the JSON syntax in your configuration file
- Ensure the MCP server is not disabled (`"disabled": false`)

### Connection Timeouts

- Increase the `timeout` value in your configuration (default is 120 seconds)
- Check network connectivity between Crush and Netdata
- Verify firewall rules allow access to port 19999

### Limited Data Access

- Verify API key is included in the connection URL or headers
- Check that the Netdata agent is properly configured for MCP
- Ensure MCP is enabled in your Netdata build

### Environment Variable Issues

- Crush uses `$(echo $VAR)` syntax, not `$VAR` or `${VAR}`
- Ensure environment variables are exported before starting Crush
- Test with `echo $NETDATA_API_KEY` to verify the variable is set

## Advanced Configuration

### Multiple Environments with Different Transports

Configure different Netdata instances using different transport methods:

```json
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "netdata-local": {
      "type": "stdio",
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://localhost:19999/mcp"],
      "timeout": 60
    },
    "netdata-parent": {
      "type": "http",
      "url": "https://parent.company.com:19999/mcp",
      "headers": {
        "Authorization": "Bearer ${PARENT_API_KEY}"
      },
      "timeout": 180
    },
    "netdata-streaming": {
      "type": "sse",
      "url": "https://stream-parent.company.com:19999/mcp?transport=sse",
      "headers": {
        "Authorization": "Bearer ${STREAM_API_KEY}"
      },
      "timeout": 300
    }
  }
}
```

> ℹ️ Before switching between environments, export `ND_MCP_BEARER_TOKEN` with the matching key so the bridge authenticates without exposing credentials in the JSON file.

### Debugging MCP Connections

Enable debug logging to troubleshoot MCP issues:

```json
{
  "$schema": "https://charm.land/crush.json",
  "options": {
    "debug": true
  }
}
```

View logs:
```bash
# View recent logs
crush logs

# Follow logs in real-time
crush logs --follow
```

## Documentation Links

- [Crush GitHub Repository](https://github.com/charmbracelet/crush)
- [Crush Configuration Schema](https://charm.land/crush.json)
- [Charmbracelet Documentation](https://charm.sh)
- [Netdata MCP Setup](/docs/learn/mcp.md)
- [AI DevOps Best Practices](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md)
