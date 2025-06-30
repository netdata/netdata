# Claude Code

Configure Claude Code to access your Netdata infrastructure through MCP.

## Prerequisites

1. **Claude Code installed** - Available at [anthropic.com/claude-code](https://www.anthropic.com/claude-code)
2. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
3. **`nd-mcp` program available on your desktop or laptop** - This is the bridge that translates `stdio` to `websocket`, connecting your AI Client to your Netdata Agent or Parent. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
4. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Configuration

Claude Code has comprehensive MCP server management capabilities. For detailed documentation on all configuration options and commands, see the [official Claude Code MCP documentation](https://docs.anthropic.com/en/docs/claude-code/mcp).

### Adding Netdata MCP Server

Use Claude Code's built-in MCP commands to add your Netdata server:

```bash
# Add Netdata MCP server (project-scoped for team sharing)
claude mcp add --scope project netdata /usr/sbin/nd-mcp ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY

# Or add locally for personal use only
claude mcp add netdata /usr/sbin/nd-mcp ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY

# List configured servers to verify
claude mcp list

# Get server details
claude mcp get netdata
```

Replace:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

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

Claude Code's strength is project-specific configurations. So you can have different project directories with different MCP servers on each of them, allowing you to control the MCP servers that will be used, based on the directory from which you started it.

### Production Environment

Create `~/projects/production/.mcp.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://prod-parent.company.com:19999/mcp?api_key=PROD_KEY"]
    }
  }
}
```

### Development Environment

Create `~/projects/development/.mcp.json`:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://dev-parent.company.com:19999/mcp?api_key=DEV_KEY"]
    }
  }
}
```

## Claude Instructions

Create a `Claude.md` file in your project root with default instructions:

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
