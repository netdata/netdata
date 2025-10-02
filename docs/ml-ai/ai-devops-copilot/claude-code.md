# Claude Code

Configure Claude Code to access your Netdata infrastructure through MCP.

## Transport Support

Claude Code supports multiple MCP transport types, giving you flexibility in how you connect to Netdata:

| Transport | Support | Netdata Version | Use Case |
|-----------|---------|-----------------|----------|
| **stdio** (via nd-mcp bridge) | ✅ Fully Supported | v2.6.0+ | Local bridge to WebSocket |
| **Streamable HTTP** | ✅ Fully Supported | v2.7.2+ | Direct connection to Netdata's HTTP endpoint (recommended) |
| **SSE** (Server-Sent Events) | ⚠️ Limited Support | v2.7.2+ | Legacy, being deprecated |
| **WebSocket** | ❌ Not Supported | - | Use nd-mcp bridge or HTTP instead |

## Prerequisites

1. **Claude Code installed** - Available at [anthropic.com/claude-code](https://www.anthropic.com/claude-code)
2. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent to get infrastructure level visibility. Your AI Client (running on your desktop or laptop) needs to have direct network access to the Netdata IP and port (usually 19999).
   - **v2.6.0 - v2.7.1**: Only WebSocket transport available, requires `nd-mcp` bridge
   - **v2.7.2+**: Direct HTTP/SSE support available (recommended)
3. **For WebSocket or stdio connections: `nd-mcp` bridge** - The stdio-to-websocket bridge. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge). Not needed for direct HTTP connections on v2.7.2+.
4. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Configuration Methods

Claude Code has comprehensive MCP server management capabilities. For detailed documentation on all configuration options and commands, see the [official Claude Code MCP documentation](https://docs.anthropic.com/en/docs/claude-code/mcp).

### Method 1: Direct HTTP Connection (Recommended for v2.7.2+)

Connect directly to Netdata's HTTP endpoint without needing the nd-mcp bridge:

```bash
# Add Netdata via direct HTTP connection (project-scoped for team sharing)
claude mcp add --transport http --scope project netdata \
  http://YOUR_NETDATA_IP:19999/mcp \
  --header "Authorization: Bearer NETDATA_MCP_API_KEY"

# Or add locally for personal use only
claude mcp add --transport http netdata \
  http://YOUR_NETDATA_IP:19999/mcp \
  --header "Authorization: Bearer NETDATA_MCP_API_KEY"

# For HTTPS connections
claude mcp add --transport http --scope project netdata \
  https://YOUR_NETDATA_IP:19999/mcp \
  --header "Authorization: Bearer NETDATA_MCP_API_KEY"
```

### Method 2: Using nd-mcp Bridge (stdio)

For environments where you prefer or need to use the bridge:

```bash
# Add Netdata via nd-mcp bridge (project-scoped)
claude mcp add --scope project netdata /usr/sbin/nd-mcp \
  --bearer NETDATA_MCP_API_KEY \
  ws://YOUR_NETDATA_IP:19999/mcp

# Or add locally for personal use only
claude mcp add netdata /usr/sbin/nd-mcp \
  --bearer NETDATA_MCP_API_KEY \
  ws://YOUR_NETDATA_IP:19999/mcp
```

### Method 3: Using npx remote-mcp (Alternative Bridge for v2.7.2+)

If nd-mcp is not available, you can use the official MCP remote client (requires Netdata v2.7.2+). For detailed options and troubleshooting, see [Using MCP Remote Client](/docs/learn/mcp.md#using-mcp-remote-client).

```bash
# Using SSE transport
claude mcp add --scope project netdata npx mcp-remote@latest \
  --sse http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer NETDATA_MCP_API_KEY"

# Using HTTP transport
claude mcp add --scope project netdata npx mcp-remote@latest \
  --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer NETDATA_MCP_API_KEY"
```

### Verify Configuration

```bash
# List configured servers
claude mcp list

# Get server details
claude mcp get netdata
```

Replace in all examples:
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (stdio method only)

**Project-scoped configuration** creates a `.mcp.json` file that can be shared with your team via version control.

## How to Use

Claude Code can automatically use Netdata MCP when you ask infrastructure-related questions. If Netdata is your only observability solution configured via MCP, simply ask your question naturally:

```
What's the current CPU usage across all servers?
Show me any anomalies in the last hour
Which processes are consuming the most memory?
```

### Explicit MCP Server Selection

Claude Code also allows you to explicitly specify which MCP server to use with the `/mcp` command:

1. Open Claude Code in the directory containing `.mcp.json`
2. Type `/mcp` to verify Netdata is available
3. Use `/mcp netdata` followed by your query:

```
/mcp netdata describe my infrastructure
/mcp netdata what alerts are currently active?
/mcp netdata show me database performance metrics
```

This is particularly useful when you have multiple MCP servers configured and want to ensure Claude uses the correct one.

> **💡 Advanced Usage:** Claude Code can combine observability data with system automation for powerful DevOps workflows. Learn about the opportunities and security considerations in [AI DevOps Copilot](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md).

## Project-Based Configuration

Claude Code's strength is project-specific configurations. You can have different project directories with different MCP servers, allowing you to control the MCP servers based on the directory from which you started Claude Code.

### Configuration File Format (`.mcp.json`)

#### Direct HTTP Connection (Recommended)

Create `~/projects/production/.mcp.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "type": "http",
      "url": "http://prod-parent.company.com:19999/mcp",
      "headers": [
        "Authorization: Bearer ${NETDATA_API_KEY}"
      ]
    }
  }
}
```

#### Using nd-mcp Bridge

Create `~/projects/production/.mcp.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "--bearer",
        "${NETDATA_API_KEY}",
        "ws://prod-parent.company.com:19999/mcp"
      ]
    }
  }
}
```

#### Using npx remote-mcp

Create `~/projects/production/.mcp.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "npx",
      "args": [
        "mcp-remote@latest",
    "--sse",
    "http://prod-parent.company.com:19999/mcp",
    "--allow-http",
    "--header",
    "Authorization: Bearer ${NETDATA_API_KEY}",
      ]
    }
  }
}
```

### Environment Variables

Claude Code supports environment variable expansion in `.mcp.json`:
- `${VAR}` - Expands to the value of environment variable `VAR`
- `${VAR:-default}` - Uses `VAR` if set, otherwise uses `default`

This allows you to keep sensitive API keys out of version control.

## Claude Instructions

Create a `CLAUDE.md` file in your project root with default instructions:

```markdown
# Claude Instructions

You have access to Netdata monitoring for our production infrastructure.

When I ask about performance or issues:
1. Always check current metrics first
2. Look for anomalies in the relevant time period
3. Check logs if investigating errors
4. Provide specific metric values and timestamps

Our key services to monitor:
- Web servers (nginx)
- Databases (PostgreSQL, Redis)
- Message queues (RabbitMQ)
```

## Troubleshooting

### MCP Not Available

- Ensure `.mcp.json` is in the current directory
- Restart Claude Code after creating the configuration
- Verify the JSON syntax is correct

### Connection Failed

- Check Netdata is accessible: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Verify the bridge path exists and is executable
- Ensure API key is correct

### Limited Data Access

- Verify API key is included in the connection string
- Check that the Netdata agent is claimed

## Documentation Links

- [Official Claude Code Documentation](https://docs.claude.com/en/docs/claude-code)
- [Claude Code MCP Configuration Guide](https://docs.claude.com/en/docs/claude-code/mcp)
- [Claude Code Getting Started](https://docs.claude.com/en/docs/claude-code/getting-started)
- [Claude Code Commands Reference](https://docs.claude.com/en/docs/claude-code/commands)
- [Netdata MCP Setup](/docs/learn/mcp.md)
- [AI DevOps Best Practices](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md)
