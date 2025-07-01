# Gemini CLI

Configure Google's Gemini CLI to access your Netdata infrastructure through MCP for powerful AI-driven operations.

## Prerequisites

1. **Gemini CLI installed** - Available from [GitHub](https://github.com/google-gemini/gemini-cli)
2. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
3. **`nd-mcp` program available on your desktop or laptop** - This is the bridge that translates `stdio` to `websocket`, connecting your AI Client to your Netdata Agent or Parent. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
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

## Configuration

Gemini CLI has built-in MCP server support. For detailed MCP configuration, see the [official MCP documentation](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md).

### Adding Netdata MCP Server

Configure your Gemini settings to include the Netdata MCP server:

```bash
# Edit Gemini settings file
~/.gemini/settings.json
```

Add your Netdata MCP server configuration:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"]
    }
  }
}
```

### Verify MCP Configuration

Use the `/mcp` command to verify your setup:

```bash
# List configured MCP servers
/mcp

# Show detailed descriptions of MCP servers and tools
/mcp desc

# Show MCP server schema details
/mcp schema
```

Replace:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

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

## Documentation Links

- [Gemini CLI GitHub Repository](https://github.com/google-gemini/gemini-cli)
- [Gemini CLI Official Documentation](https://developers.google.com/gemini-code-assist/docs/gemini-cli)
- [Netdata MCP Setup](/docs/learn/mcp.md)
- [AI DevOps Best Practices](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md)
