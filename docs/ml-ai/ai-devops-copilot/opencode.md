# OpenCode

Configure SST's OpenCode to access your Netdata infrastructure through MCP for terminal-based AI-powered DevOps operations.

## Transport Support

OpenCode supports both local and remote MCP servers:

| Transport | Support | Netdata Version | Use Case |
|-----------|---------|-----------------|----------|
| **stdio** (via nd-mcp bridge) | ✅ Fully Supported | v2.6.0+ | Local bridge to WebSocket |
| **Streamable HTTP** (remote) | ✅ Fully Supported | v2.7.2+ | Direct connection to Netdata's HTTP endpoint (recommended) |
| **SSE** (Server-Sent Events) | ⚠️ Limited Support | v2.7.2+ | Known issues with SSE servers |
| **WebSocket** | ❌ Not Supported | - | Use nd-mcp bridge or HTTP instead |

> **Note:** OpenCode has reported issues with SSE-based MCP servers ([GitHub Issue #834](https://github.com/sst/opencode/issues/834)). Use HTTP streamable transport for best compatibility.

## Prerequisites

1. **OpenCode installed** - Available via npm, brew, or direct download from [GitHub](https://github.com/sst/opencode)
2. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent to get infrastructure level visibility. Your AI Client (running on your desktop or laptop) needs to have direct network access to the Netdata IP and port (usually 19999).
   - **v2.6.0 - v2.7.1**: Only WebSocket transport available, requires `nd-mcp` bridge
   - **v2.7.2+**: Direct HTTP/SSE support available (recommended)
3. **For WebSocket or stdio connections: `nd-mcp` bridge** - The stdio-to-websocket bridge. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge). Not needed for direct HTTP connections on v2.7.2+.
4. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

> Export `ND_MCP_BEARER_TOKEN` with your MCP key before launching OpenCode to keep secrets out of configuration files:
> ```bash
> export ND_MCP_BEARER_TOKEN="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"
> ```

## Installation

Install OpenCode using one of these methods:

```bash
# Using npm (recommended)
npm i -g opencode-ai@latest

# Using Homebrew
brew install sst/tap/opencode

# Using curl installation script
curl -fsSL https://opencode.ai/install.sh | bash
```

## Configuration Methods

OpenCode uses an `opencode.json` configuration file with MCP servers defined under the `mcp` key.

### Method 1: Direct HTTP Connection (Recommended for v2.7.2+)

Connect directly to Netdata's HTTP endpoint without needing the nd-mcp bridge:

```json
{
  "mcp": {
    "netdata": {
      "type": "remote",
      "url": "http://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      },
      "enabled": true
    }
  }
}
```

For HTTPS connections:

```json
{
  "mcp": {
    "netdata": {
      "type": "remote",
      "url": "https://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      },
      "enabled": true
    }
  }
}
```

### Method 2: Using nd-mcp Bridge (Local)

For environments where you prefer or need to use the bridge:

```json
{
  "mcp": {
    "netdata": {
      "type": "local",
      "command": ["/usr/sbin/nd-mcp", "ws://YOUR_NETDATA_IP:19999/mcp"],
      "enabled": true
    }
  }
}
```

### Method 3: Using npx remote-mcp (Alternative Bridge for v2.7.2+)

If nd-mcp is not available, use the official MCP remote client (requires Netdata v2.7.2+). For detailed options and troubleshooting, see [Using MCP Remote Client](/docs/learn/mcp.md#using-mcp-remote-client).

```json
{
  "mcp": {
    "netdata": {
      "type": "local",
      "command": [
        "npx",
        "mcp-remote@latest",
        "--http",
        "http://YOUR_NETDATA_IP:19999/mcp",
        "--allow-http",
        "--header",
        "Authorization: Bearer NETDATA_MCP_API_KEY"
      ],
      "enabled": true
    }
  }
}
```

## Environment Variables

OpenCode supports environment variables in local server configurations:

```json
{
  "mcp": {
    "netdata": {
      "type": "local",
      "command": ["/usr/sbin/nd-mcp", "ws://YOUR_NETDATA_IP:19999/mcp"],
      "enabled": true,
      "environment": {
        "ND_MCP_BEARER_TOKEN": "your-api-key-here"
      }
    }
  }
}
```

For remote servers with environment variables:

```json
{
  "mcp": {
    "netdata": {
      "type": "remote",
      "url": "https://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer ${NETDATA_API_KEY}"
      },
      "enabled": true
    }
  }
}
```

Replace in all examples:
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `ND_MCP_BEARER_TOKEN` - Export with your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key) before launching OpenCode
- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (local method only)

## How to Use

Once configured, OpenCode can leverage Netdata's observability data through its terminal interface:

```bash
# Start OpenCode
opencode

# The AI assistant will have access to Netdata tools
# Ask infrastructure questions naturally:
What's the current CPU usage across all servers?
Show me any performance anomalies in the last hour
Which services are consuming the most resources?
```

## Selective Tool Enabling

OpenCode allows fine-grained control over MCP tool availability per agent:

```json
{
  "mcp": {
    "netdata": {
      "type": "remote",
      "url": "http://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      },
      "enabled": true
    }
  },
  "tools": {
    "netdata*": false
  },
  "agent": {
    "infrastructure-analyst": {
      "tools": {
        "netdata*": true
      }
    }
  }
}
```

This configuration:
- Disables Netdata tools globally
- Enables them only for the "infrastructure-analyst" agent

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

> **💡 Advanced Usage:** OpenCode's terminal-based interface combined with Netdata observability creates powerful DevOps workflows. Learn about the opportunities and security considerations in [AI DevOps Copilot](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md).

## Troubleshooting

### MCP Server Not Connecting

- Verify Netdata is accessible: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Check the JSON syntax in your `opencode.json` file
- Ensure the MCP server is enabled (`"enabled": true`)

### SSE Transport Issues

OpenCode has known issues with SSE-based MCP servers. If you encounter "UnknownError Server error" messages:
- Switch to HTTP streamable transport (remove `?transport=sse` from URL)
- Use the local nd-mcp bridge instead
- Check [GitHub Issue #834](https://github.com/sst/opencode/issues/834) for updates

### Limited Data Access

- Verify API key is included in the connection URL or headers
- Check that the Netdata agent is properly configured for MCP
- Ensure MCP is enabled in your Netdata build

### Command Format Issues

- Local servers require command as an array: `["command", "arg1", "arg2"]`
- Remote servers use a URL string: `"url": "http://..."`
- Don't mix local and remote configuration options

## Advanced Configuration

### Multiple Environments

Configure different Netdata instances for different purposes:

```json
{
  "mcp": {
    "netdata-prod": {
      "type": "remote",
      "url": "https://prod-parent.company.com:19999/mcp",
      "headers": {
        "Authorization": "Bearer ${PROD_API_KEY}"
      },
      "enabled": true
    },
    "netdata-staging": {
      "type": "remote",
      "url": "https://staging-parent.company.com:19999/mcp",
      "headers": {
        "Authorization": "Bearer ${STAGING_API_KEY}"
      },
      "enabled": false
    },
    "netdata-local": {
      "type": "local",
      "command": ["/usr/sbin/nd-mcp", "ws://localhost:19999/mcp"],
      "environment": {
        "ND_MCP_BEARER_TOKEN": "${LOCAL_API_KEY}"
      },
      "enabled": true
    }
  }
}
```

### Debugging MCP Connections

Enable verbose logging to troubleshoot MCP issues:

```json
{
  "mcp": {
    "netdata": {
      "type": "remote",
      "url": "http://YOUR_NETDATA_IP:19999/mcp",
      "headers": {
        "Authorization": "Bearer NETDATA_MCP_API_KEY"
      },
      "enabled": true,
      "debug": true
    }
  }
}
```

## Documentation Links

- [OpenCode GitHub Repository](https://github.com/sst/opencode)
- [OpenCode Documentation](https://opencode.ai/docs)
- [OpenCode MCP Servers Guide](https://opencode.ai/docs/mcp-servers/)
- [SST Discord Community](https://discord.gg/sst)
- [Netdata MCP Setup](/docs/learn/mcp.md)
- [AI DevOps Best Practices](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md)
