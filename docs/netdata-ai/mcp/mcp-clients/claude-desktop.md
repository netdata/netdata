# Claude Desktop

Configure Claude Desktop to access your Netdata infrastructure through MCP.

## Transport Support

Claude Desktop launches MCP servers as child processes over `stdio` (the only transport the client supports today). Remote servers must be proxied through a launcher that exposes a stdio interface, such as `nd-mcp` or `npx mcp-remote`, before Claude Desktop can connect.

| Transport delivered to Claude Desktop | Support | Netdata Version | Notes |
|--------------------------------------|---------|-----------------|-------|
| **stdio** (via nd-mcp bridge) | ✅ Fully Supported | v2.6.0+ | Native Claude transport |
| **stdio** (via `npx mcp-remote`) | ✅ Fully Supported | v2.7.2+ | Wraps Netdata HTTP/SSE in stdio |
| **Direct HTTP / SSE** | ⚠️ Use bridge | - | Requires a stdio bridge (Claude cannot speak HTTP/SSE directly) |

> **Reference:** Claude Desktop’s official quickstart configures MPC servers by editing `claude_desktop_config.json` and launching stdio bridges (https://modelcontextprotocol.io/docs/develop/connect-local-servers).

## Prerequisites

1. **Claude Desktop installed** - Download from [claude.ai/download](https://claude.ai/download)
2. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent to get infrastructure level visibility. Your AI Client (running on your desktop or laptop) needs to have direct network access to the Netdata IP and port (usually 19999).
   - **v2.6.0 - v2.7.1**: Only WebSocket transport available, requires `nd-mcp` bridge
   - **v2.7.2+**: Can use `npx mcp-remote` bridge for HTTP/SSE support
3. **Bridge required: Choose one:**
   - `nd-mcp` bridge - The stdio-to-websocket bridge for all Netdata versions. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
   - `npx mcp-remote@latest` - Official MCP remote client supporting HTTP/SSE (requires Netdata v2.7.2+)
4. **Netdata MCP API key loaded into the environment** (recommended) - export it before launching Claude Desktop to avoid exposing it in config files:
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

## Configuration Methods

Claude Desktop supports MCP servers through two methods: Custom Connectors for remote servers (recommended), and traditional JSON configuration (manual).

### Method 1: Claude Desktop Custom Connectors (Anthropic-hosted beta)

Anthropic’s custom connectors beta lets Team/Enterprise owners add remote servers through Claude’s UI. The connector flow relies on the server’s OAuth or custom auth and does **not** expose arbitrary HTTP headers. Follow the server developer’s instructions to complete the OAuth hand-off; the UI handles credential storage (https://support.claude.com/en/articles/11175166-getting-started-with-custom-connectors-using-remote-mcp).

Because Netdata currently authenticates via bearer tokens, you’ll need the stdio launcher methods below unless you front your Netdata MCP endpoint with an OAuth-capable bridge.

### Method 2: Traditional JSON Configuration with nd-mcp Bridge

For all Netdata versions (v2.6.0+), you can manually configure MCP servers:

1. Open Claude Desktop
2. Navigate to Settings:
   - **Windows/Linux**: File → Settings → Developer (or `Ctrl+,`)
   - **macOS**: Claude → Settings → Developer (or `Cmd+,`)
3. Click "Edit Config" button
4. This opens `claude_desktop_config.json` in your system’s config folder:
   - **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
   - **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
   - **Linux** (preview builds): `~/.config/claude/claude_desktop_config.json`

Add the Netdata configuration:

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

5. Save the configuration file
6. **Restart Claude Desktop** (required for changes to take effect)

### Method 3: Traditional JSON Configuration with `npx mcp-remote` (v2.7.2+)

For Netdata v2.7.2+ with HTTP/SSE support. `mcp-remote` wraps remote transports in a stdio session Claude can launch (https://modelcontextprotocol.io/docs/develop/connect-local-servers). Edit `claude_desktop_config.json` as above.

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

For SSE transport instead of HTTP:

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

Replace in all examples:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (nd-mcp method only)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `ND_MCP_BEARER_TOKEN` - Export this environment variable with your API key before launching Claude Desktop (nd-mcp method only)

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

Claude Desktop supports multiple environments:

### Option 1: Multiple Custom Connectors (Recommended)

Add multiple connectors for different environments via **Settings → Connectors**:

- Add `Netdata Production` pointing to `http://prod-parent:19999/mcp`
- Add `Netdata Staging` pointing to `http://stage-parent:19999/mcp`
- Enable/disable connectors as needed

### Option 2: Toggle JSON Configuration

For local bridges, add multiple configurations in `claude_desktop_config.json` and enable/disable as needed:

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

### Option 3: Single Parent

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
