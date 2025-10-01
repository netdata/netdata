# OpenAI Codex CLI

Configure OpenAI's Codex CLI to access your Netdata infrastructure through MCP for AI-powered DevOps operations.

## Transport Support

Codex CLI currently has limited MCP transport support:

| Transport | Support | Use Case |
|-----------|---------|----------|
| **stdio** (via nd-mcp bridge) | ✅ Supported | Local bridge to WebSocket |
| **stdio** (via npx remote-mcp) | ✅ Supported | Alternative bridge with HTTP/SSE support |
| **Streamable HTTP** | ❌ Not Supported | Use npx remote-mcp bridge |
| **SSE** (Server-Sent Events) | ❌ Not Supported | Use npx remote-mcp bridge |
| **WebSocket** | ❌ Not Supported | Use nd-mcp bridge |

> **Note:** Codex CLI currently only supports stdio-based MCP servers. For HTTP/SSE connections to Netdata, you must use a bridge like nd-mcp or npx remote-mcp.

## Prerequisites

1. **OpenAI Codex CLI installed** - Available via npm, Homebrew, or direct download from [GitHub](https://github.com/openai/codex)
2. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
3. **Bridge required: Choose one:**
   - `nd-mcp` bridge - The stdio-to-websocket bridge. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
   - `npx mcp-remote@latest` - Official MCP remote client supporting HTTP/SSE
4. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Installation

Install Codex CLI using one of these methods:

```bash
# Using npm (recommended)
npm install -g @openai/codex

# Using Homebrew (macOS)
brew install codex

# Or download directly from GitHub releases
# https://github.com/openai/codex/releases
```

## Configuration Methods

Codex CLI uses a TOML configuration file at `~/.codex/config.toml` for MCP server settings.

### Method 1: Using npx remote-mcp (Recommended for HTTP/SSE)

This method allows Codex CLI to connect to Netdata's HTTP/SSE endpoints through the official MCP remote client:

```toml
# ~/.codex/config.toml

[mcp_servers.netdata]
command = "npx"
args = [
  "mcp-remote@latest",
  "--http",
  "--allow-http",
  "http://YOUR_NETDATA_IP:19999/mcp",
  "--header",
  "Authorization: Bearer NETDATA_MCP_API_KEY"
]
startup_timeout_sec = 20  # Optional: increase for remote connections
tool_timeout_sec = 120     # Optional: increase for complex queries
```

For SSE transport instead of HTTP:

```toml
[mcp_servers.netdata]
command = "npx"
args = [
  "mcp-remote@latest",
  "--sse",
  "http://YOUR_NETDATA_IP:19999/mcp",
  "--allow-http",
  "--header",
  "Authorization: Bearer NETDATA_MCP_API_KEY",
]
```

### Method 2: Using nd-mcp Bridge

For environments where nd-mcp is available and preferred:

```toml
# ~/.codex/config.toml

[mcp_servers.netdata]
command = "/usr/sbin/nd-mcp"
args = ["ws://YOUR_NETDATA_IP:19999/mcp"]
env = { "ND_MCP_BEARER_TOKEN" = "YOUR_API_KEY_HERE" }
startup_timeout_sec = 15
tool_timeout_sec = 60

[mcp_servers.netdata_prod]
command = "/usr/sbin/nd-mcp"
args = ["ws://prod-parent:19999/mcp"]
env = { "ND_MCP_BEARER_TOKEN" = "${NETDATA_PROD_API_KEY}" }
```

Export `ND_MCP_BEARER_TOKEN` before starting Codex CLI (or define it in your shell profile) so the bridge authenticates without exposing the key in command-line arguments.

When Codex CLI starts the bridge it will inject the environment variable, so `nd-mcp` authenticates without exposing the token in the connection arguments.

## CLI Management (Experimental)

Codex CLI provides experimental commands for managing MCP servers:

```bash
# Add a new MCP server
codex mcp add netdata -- npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer NETDATA_MCP_API_KEY"

# List configured MCP servers
codex mcp list

# Remove an MCP server
codex mcp remove netdata
```

## Verify Configuration

After configuring, verify that Netdata MCP is available:

1. Start Codex CLI:
   ```bash
   codex
   ```

2. Check available tools (if MCP is properly configured, Netdata tools should be available)

Replace in all examples:
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (nd-mcp method only)

## How to Use

Once configured, Codex CLI can leverage Netdata's observability data for infrastructure analysis:

```
# Start Codex CLI
codex

# Ask infrastructure questions
What's the current CPU usage across all servers?
Show me any performance anomalies in the last hour
Which services are consuming the most resources?
```

## Example Workflows

**Performance Investigation:**
```
Investigate why our application response times increased this afternoon
```

**Resource Optimization:**
```
Analyze memory usage patterns and suggest optimization strategies
```

**Alert Analysis:**
```
Explain the current active alerts and their potential impact
```

> **💡 Advanced Usage:** Codex CLI can combine observability data with code generation capabilities for powerful DevOps workflows. Learn about the opportunities and security considerations in [AI DevOps Copilot](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md).

## Troubleshooting

### MCP Server Not Starting

- Check the command path exists and is executable
- Increase `startup_timeout_sec` for slow-starting servers
- Verify network connectivity to Netdata

### Connection Timeouts

- Ensure Netdata is accessible: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Increase timeout values in configuration
- Check firewall rules between Codex CLI and Netdata

### Limited Data Access

- Verify the Authorization header is set to `Bearer <your key>`
- Ensure the Netdata agent is properly configured for MCP
- Check that MCP is enabled in your Netdata build

### Windows Issues

- MCP servers may have issues on Windows
- Consider using WSL (Windows Subsystem for Linux)
- Check GitHub issues for Windows-specific workarounds

## Advanced Configuration

### Multiple Environments

Configure different Netdata instances for different purposes:

```toml
# Production environment
[mcp_servers.netdata_prod]
command = "/usr/sbin/nd-mcp"
args = ["ws://prod-parent.company.com:19999/mcp"]
env = { "ND_MCP_BEARER_TOKEN" = "${PROD_API_KEY}" }
startup_timeout_sec = 30
tool_timeout_sec = 120

[mcp_servers.netdata_staging]
command = "/usr/sbin/nd-mcp"
args = ["ws://staging-parent.company.com:19999/mcp"]
env = { "ND_MCP_BEARER_TOKEN" = "${STAGING_API_KEY}" }

[mcp_servers.netdata_local]
command = "/usr/sbin/nd-mcp"
args = ["ws://localhost:19999/mcp"]
env = { "ND_MCP_BEARER_TOKEN" = "${LOCAL_API_KEY}" }
```

### Timeout Configuration

Adjust timeouts based on your network and query complexity:

```toml
[mcp_servers.netdata]
command = "npx"
args = [
  "mcp-remote@latest",
  "--http",
  "http://remote-netdata:19999/mcp",
  "--allow-http",
  "--header",
  "Authorization: Bearer NETDATA_MCP_API_KEY"
]
startup_timeout_sec = 30  # Time to wait for MCP server to start
tool_timeout_sec = 180     # Time limit for individual tool calls
```

## Documentation Links

- [OpenAI Codex CLI GitHub Repository](https://github.com/openai/codex)
- [Codex CLI Configuration Documentation](https://github.com/openai/codex/blob/main/docs/config.md)
- [Codex CLI Installation Guide](https://github.com/openai/codex#installation)
- [Netdata MCP Setup](/docs/learn/mcp.md)
- [AI DevOps Best Practices](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md)
