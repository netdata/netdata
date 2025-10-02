# Claude Desktop

Configure Claude Desktop to access your Netdata infrastructure through MCP.

## Prerequisites

1. **Claude Desktop installed** - Download from [claude.ai/download](https://claude.ai/download)
2. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
3. **`nd-mcp` program available on your desktop or laptop** - This is the bridge that translates `stdio` to `websocket`, connecting your AI Client to your Netdata Agent or Parent. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
4. **Netdata MCP API key loaded into the environment** (recommended) - export it before launching Claude Code to avoid exposing it in config files:
   ```bash
   export ND_MCP_BEARER_TOKEN="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"
   ```
   Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Platform-Specific Installation

### Windows & macOS

Download directly from [claude.ai/download](https://claude.ai/download)

### Linux

Use the community AppImage project:

1. Download from [github.com/fsoft72/claude-desktop-to-appimage](https://github.com/fsoft72/claude-desktop-to-appimage)
2. For best experience, install [AppImageLauncher](https://github.com/TheAssassin/AppImageLauncher)

## Configuration

1. Open Claude Desktop
2. Navigate to Settings:
   - **Windows/Linux**: File → Settings → Developer (or `Ctrl+,`)
   - **macOS**: Claude → Settings → Developer (or `Cmd+,`)
3. Click "Edit Config" button
4. Add the Netdata configuration:

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

Replace:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `ND_MCP_BEARER_TOKEN` - Export this environment variable with your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key) before launching Claude Desktop

5. Save the configuration
6. **Restart Claude Desktop** (required for changes to take effect)

## Verify Connection

1. Click the "Search and tools" button (below the prompt)
2. You should see "netdata" listed among available tools
3. If not visible, check your configuration and restart

## Usage Examples

Simply ask Claude about your infrastructure:

```
What's the current CPU usage across all my servers?
Show me any anomalies in the last 4 hours
Which processes are consuming the most memory?
Are there any critical alerts active?
Search the logs for authentication failures
```

## Multiple Environments

Claude Desktop has limitations with multiple MCP servers. Options:

### Option 1: Toggle Servers

Add multiple configurations and enable/disable as needed:

```json
{
  "mcpServers": {
    "netdata-production": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://prod-parent:19999/mcp"]
    },
    "netdata-staging": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://stage-parent:19999/mcp"]
    }
  }
}
```

Use the toggle switch in settings to enable only one at a time.

> ℹ️ Set `ND_MCP_BEARER_TOKEN` to the appropriate key before switching between environments to avoid storing secrets in the configuration file.

### Option 2: Single Parent

Connect to your main Netdata Parent that has visibility across all environments.

## Troubleshooting

### Netdata Not Appearing in Tools

- Ensure configuration file is valid JSON
- Restart Claude Desktop after configuration changes
- Check the bridge path exists and is executable

### Connection Errors

- Verify Netdata is accessible from your machine
- Test: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Check firewall rules allow connection to port 19999

### "Bridge Not Found" Error

- Verify the nd-mcp path is correct
- Windows users: Include the `.exe` extension
- Ensure Netdata is installed on your local machine (for the bridge)

### Limited Access to Data

- Verify API key is included in the connection string
- Ensure the API key file exists on the Netdata server
- Check that functions and logs collectors are enabled
