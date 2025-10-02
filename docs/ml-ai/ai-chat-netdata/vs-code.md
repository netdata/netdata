# VS Code

Configure Visual Studio Code extensions to access your Netdata infrastructure through MCP.

## Available Extensions

### Continue (Recommended)

The most popular open-source AI code assistant with MCP support.

### Cline

Autonomous coding agent that can use MCP tools.

## Transport Support

VS Code extensions typically support stdio-based MCP servers:

| Transport | Support | Netdata Version | Use Case |
|-----------|---------|-----------------|----------|
| **stdio** (via nd-mcp bridge) | ✅ Fully Supported | v2.6.0+ | Local bridge to WebSocket |
| **stdio** (via npx remote-mcp) | ✅ Fully Supported | v2.7.2+ | Alternative bridge with HTTP/SSE support |
| **Streamable HTTP** | ⚠️ Varies by Extension | v2.7.2+ | Check extension documentation |
| **SSE** (Server-Sent Events) | ⚠️ Varies by Extension | v2.7.2+ | Check extension documentation |
| **WebSocket** | ❌ Not Supported | - | Use nd-mcp bridge |

> **Note:** Most VS Code extensions support stdio-based MCP servers. For HTTP/SSE connections to Netdata v2.7.2+, you can use npx remote-mcp bridge. For older Netdata versions (v2.6.0 - v2.7.1), use the nd-mcp bridge with WebSocket.

## Prerequisites

1. **VS Code installed** - [Download VS Code](https://code.visualstudio.com)
2. **MCP-compatible extension** - Install from VS Code Marketplace
3. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent to get infrastructure level visibility. Your AI Client (running on your desktop or laptop) needs to have direct network access to the Netdata IP and port (usually 19999).
   - **v2.6.0 - v2.7.1**: Only WebSocket transport available, requires `nd-mcp` bridge
   - **v2.7.2+**: Can use `npx mcp-remote` bridge for HTTP/SSE support
4. **Bridge required: Choose one:**
   - `nd-mcp` bridge - The stdio-to-websocket bridge for all Netdata versions. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
   - `npx mcp-remote@latest` - Official MCP remote client supporting HTTP/SSE (requires Netdata v2.7.2+)
5. **Netdata MCP API key exported before launching VS Code** - keep secrets out of config files by setting:
   ```bash
   export ND_MCP_BEARER_TOKEN="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"
   ```
   Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Continue Extension Setup

### Installation

1. Open VS Code
2. Go to Extensions (Ctrl+Shift+X)
3. Search for "Continue"
4. Install the Continue extension
5. Reload VS Code

### Configuration

#### Step 1: Add Claude Model

1. Click "**Select model**" dropdown at the bottom (next to Chat dropdown)
2. Click "**+ Add Chat model**"
3. In the configuration screen:
    - **Provider**: Change to "Anthropic"
    - **Model**: Select `Claude-3.5-Sonnet`
    - **API key**: Enter your Anthropic API key
    - Click "**Connect**"

#### Step 2: Add Netdata MCP Server

**Method 1: Using nd-mcp Bridge** (All Netdata versions v2.6.0+)

1. Click "**MCP**" in the top toolbar
2. Click "**+ Add MCP Servers**"
3. It creates the file in your current project's `.continue/mcpServers/` directory as `new-mcp-server.yaml`. You might want to rename the file to something more descriptive like `netdata.yaml` after editing.
4. Replace the content with:
    ```yaml
    name: Netdata MCP
    version: 0.0.1
    schema: v1
    mcpServers:
       - name: netdata
         command: /usr/sbin/nd-mcp
         args:
            - ws://YOUR_NETDATA_IP:19999/mcp
         env: {}
    ```
5. Replace:
    - `/usr/sbin/nd-mcp` with your actual nd-mcp path
    - `YOUR_NETDATA_IP` with your Netdata instance IP/hostname
    - `ND_MCP_BEARER_TOKEN` exported with your Netdata MCP API key before launching VS Code
6. Save the file

**Method 2: Using npx remote-mcp** (Recommended for Netdata v2.7.2+)

For detailed options and troubleshooting, see [Using MCP Remote Client](/docs/learn/mcp.md#using-mcp-remote-client).

```yaml
name: Netdata MCP
version: 0.0.1
schema: v1
mcpServers:
   - name: netdata
     command: npx
     args:
        - mcp-remote@latest
        - --http
        - http://YOUR_NETDATA_IP:19999/mcp
        - --allow-http
        - --header
        - "Authorization: Bearer NETDATA_MCP_API_KEY"
     env: {}
```

For SSE transport:

```yaml
name: Netdata MCP
version: 0.0.1
schema: v1
mcpServers:
   - name: netdata
     command: npx
     args:
        - mcp-remote@latest
        - --sse
        - http://YOUR_NETDATA_IP:19999/mcp
        - --allow-http
        - --header
        - "Authorization: Bearer NETDATA_MCP_API_KEY"
     env: {}
```

### Usage

Press `Ctrl+L` to open Continue chat, then:

```
@netdata what's the current CPU usage?
@netdata show me memory trends for the last hour
@netdata are there any anomalies in the database servers?
```

## Cline Extension Setup

### Installation

1. Search for "Cline" in Extensions
2. Install and reload VS Code

### Configuration

Cline supports two configuration methods: UI-based (recommended) and JSON-based (advanced).

#### Method 1: UI-Based Configuration (Recommended)

1. Click the "**MCP Servers**" icon in Cline's navigation bar
2. Click "**Configure**" button
3. The UI shows two tabs:
   - **Installed**: View and manage configured servers
   - **Marketplace**: Browse available MCP servers
4. Add Netdata configuration through the UI:
   - **Name**: `netdata`
   - **Command**: `/usr/sbin/nd-mcp` (or `npx` for mcp-remote)
   - **Arguments**: `ws://YOUR_NETDATA_IP:19999/mcp` (or mcp-remote args)

> **Note:** The UI method is recommended as it handles configuration validation and provides a better user experience.

#### Method 2: JSON Configuration (Advanced)

For advanced users who prefer editing configuration files directly, you can edit `cline_mcp_settings.json`:

**Using nd-mcp Bridge** (All Netdata versions v2.6.0+):

```json
{
  "cline.mcpServers": [
    {
      "name": "netdata",
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "ws://YOUR_NETDATA_IP:19999/mcp"
      ]
    }
  ]
}
```

**Using npx remote-mcp** (Recommended for Netdata v2.7.2+):

For detailed options and troubleshooting, see [Using MCP Remote Client](/docs/learn/mcp.md#using-mcp-remote-client).

```json
{
  "cline.mcpServers": [
    {
      "name": "netdata",
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
  ]
}
```

### Usage

1. Open Cline (Ctrl+Shift+P → "Cline: Open Chat")
2. Cline can autonomously:
    - Analyze performance issues
    - Create monitoring scripts
    - Debug based on metrics

Example:

```
Create a Python script that checks Netdata for high CPU usage and sends an alert
```

## Multiple Environments

### Workspace-Specific Configuration

Create a YAML file in your project's `.continue/mcpServers/` directory (e.g., `netdata-prod.yaml`):

```yaml
name: Netdata Production
version: 0.0.1
schema: v1
mcpServers:
   - name: netdata-prod
     command: /usr/sbin/nd-mcp
     args:
        - ws://prod-parent:19999/mcp
     env: {}
```

### Environment Switching

Different projects can have different Netdata connections:

- `~/projects/frontend/.continue/mcpServers/netdata.yaml` → Frontend servers
- `~/projects/backend/.continue/mcpServers/netdata.yaml` → Backend servers
- `~/projects/infrastructure/.continue/mcpServers/netdata.yaml` → All servers

> ℹ️ Export `ND_MCP_BEARER_TOKEN` with the appropriate key before opening VS Code so the bridge picks up credentials without storing them in the YAML files.

## Advanced Usage

### Custom Commands

Create custom VS Code commands that query Netdata:

```json
{
  "commands": [
    {
      "command": "netdata.checkHealth",
      "title": "Netdata: Check System Health"
    }
  ]
}
```

### Task Integration

Add Netdata checks to tasks.json:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Check Production Metrics",
      "type": "shell",
      "command": "continue",
      "args": [
        "--ask",
        "@netdata show current system status"
      ]
    }
  ]
}
```

### Snippets with Metrics

Create snippets that include metric checks:

```json
{
  "Check Performance": {
    "prefix": "perf",
    "body": [
      "// @netdata: Current ${1:CPU} usage?",
      "$0"
    ]
  }
}
```

## Extension Comparison

| Feature            | Continue | Cline  | Codeium | Copilot Chat |
|--------------------|----------|--------|---------|--------------|
| MCP Support        | ✅ Full   | ✅ Full | ❓ Check | ❓ Future     |
| Autonomous Actions | ❌        | ✅      | ❌       | ❌            |
| Multiple Models    | ✅        | ✅      | ❌       | ❌            |
| Free Tier          | ❌        | ❌      | ✅       | ❌            |
| Open Source        | ✅        | ✅      | ❌       | ❌            |

## Troubleshooting

### Extension Not Finding MCP

- Restart VS Code after configuration
- Check extension logs (Output → Continue/Cline)
- Verify JSON syntax in settings

### Connection Issues

- Test Netdata: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Check bridge is executable
- Verify network access from VS Code

### No Netdata Option

- Ensure `@netdata` is typed correctly
- Check MCP server is configured
- Try reloading the window (Ctrl+R)

### Performance Problems

- Use local Netdata Parent for faster response
- Check extension memory usage
- Disable unused extensions

## Best Practices

### Development Workflow

1. Start coding with infrastructure context
2. Check metrics before optimization
3. Validate changes against production data
4. Monitor impact of deployments

### Team Collaboration

Share Netdata configurations:

- Commit `.vscode/settings.json` for project-specific configs
- Document which Netdata Parent to use
- Create team snippets for common queries
