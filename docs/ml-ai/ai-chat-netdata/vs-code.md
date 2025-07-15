# VS Code

Configure Visual Studio Code extensions to access your Netdata infrastructure through MCP.

## Available Extensions

### Cline (Recommended)

The most popular autonomous coding agent with full MCP support. Cline can analyze performance issues, create monitoring scripts, and debug based on your infrastructure metrics.

### Continue

Open-source AI code assistant with MCP support for interactive infrastructure queries.

## Prerequisites

Before you begin, ensure you have:

1. **VS Code installed** - [Download VS Code](https://code.visualstudio.com)
2. **An MCP-compatible extension** - Install Cline or Continue from VS Code Marketplace
3. **A running Netdata Agent** - You need the IP and port (usually 19999). Use a Netdata Parent for infrastructure-wide visibility. Netdata v2.6.0 or later includes MCP support.
4. **The `nd-mcp` bridge program** - This translates `stdio` to `websocket` on your local machine. [Find nd-mcp on your system](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
5. **Your Netdata MCP API key (optional)** - This unlocks full access to logs and sensitive functions. [Find your API key](/docs/learn/mcp.md#finding-your-api-key)

## Extension Setup

<details>
<summary><strong>Cline Setup</strong></summary>
<br/>

### Install Cline

1. Open VS Code Extensions (Ctrl+Shift+X)
2. Search for "Cline"
3. Click Install
4. Reload VS Code when prompted

### Configure Cline for Netdata

1. Open VS Code Settings (Ctrl+,)
2. Search for "Cline MCP"
3. Add your Netdata configuration:

```json
{
  "cline.mcpServers": [
    {
      "name": "netdata",
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"
      ]
    }
  ]
}
```

Replace:
- `/usr/sbin/nd-mcp` - Your actual nd-mcp path
- `YOUR_NETDATA_IP` - Your Netdata server's IP or hostname
- `NETDATA_MCP_API_KEY` - Your API key (or remove `?api_key=...` for limited access)

### Use Cline with Netdata

1. Open Cline chat (Ctrl+Shift+P → "Cline: Open Chat")
2. Ask Cline to interact with your infrastructure:

```
Create a Python script that monitors CPU usage from Netdata and alerts when it exceeds 80%
```

```
Analyze the performance metrics for my database servers over the last hour
```

```
Debug why my web server is responding slowly based on Netdata metrics
```

Cline will autonomously connect to Netdata, analyze metrics, and create solutions for you.

</details>

<details>
<summary><strong>Continue Setup</strong></summary>
<br/>

### Install Continue

1. Open VS Code
2. Go to Extensions (Ctrl+Shift+X)
3. Search for "Continue"
4. Click Install

:::important

5. Reload VS Code (Ctrl+R or Cmd+R on macOS)

:::

### Verify Your Setup First

Before configuring Continue, test that everything works:

#### 1. Test nd-mcp Bridge
```bash
# Run this in your terminal
/path/to/nd-mcp ws://YOUR_NETDATA_IP:19999/mcp

# You should see:
# nd-mcp: Connecting to ws://YOUR_NETDATA_IP:19999/mcp...
# nd-mcp: Connected
# Press Ctrl+C to stop
```

If this fails:
- Verify the nd-mcp path is correct
- Make the file executable: `chmod +x /path/to/nd-mcp`
- Check that Netdata is running

#### 2. Test Network Access
```bash
# Verify you can reach Netdata
curl http://YOUR_NETDATA_IP:19999/api/v3/info
```

If this fails:
- Check firewall rules for port 19999
- Verify Netdata status: `sudo systemctl status netdata`
- Ensure your machine can reach the Netdata server

#### 3. Test Your API Key (Optional)
```bash
# If using an API key, test it works
/path/to/nd-mcp ws://YOUR_NETDATA_IP:19999/mcp?api_key=YOUR_API_KEY
```

### Configure Continue for Netdata

1. Open Command Palette (Ctrl+Shift+P or Cmd+Shift+P on macOS)
2. Type "Continue: Open config.json"
3. Add your Netdata configuration:

```json
{
  "models": [
    {
      "title": "Claude 3.5 Sonnet",
      "provider": "anthropic",
      "model": "claude-3-5-sonnet-20241022",
      "apiKey": "YOUR_ANTHROPIC_KEY"
    }
  ],
  "mcpServers": {
    "netdata": {
      "type": "stdio",
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"
      ]
    }
  }
}
```

Replace:
- `/usr/sbin/nd-mcp` - Your verified nd-mcp path from testing
- `YOUR_NETDATA_IP` - Your Netdata server's IP or hostname
- `NETDATA_MCP_API_KEY` - Your API key (or omit `?api_key=...` for limited access)
- `YOUR_ANTHROPIC_KEY` - Your Anthropic API key

:::info

The model name shown above is an example. Check [Continue's documentation](https://docs.continue.dev/setup/select-model) for current model names and providers.

:::

:::note

The `"type": "stdio"` field is required for local MCP servers.

:::

4. Save the file (Ctrl+S or Cmd+S)
5. Restart VS Code to load MCP servers

### Verify Your Connection

After restarting VS Code:

1. Open Continue chat (Ctrl+L or Cmd+L)
2. Open VS Code Output panel (View → Output)
3. Select "Continue" from the dropdown
4. Look for success messages:
   ```
   [MCP] Starting server: netdata
   [MCP] Server started successfully: netdata
   ```

### Common Issues and Solutions

:::warning

**"MCP server failed to start"**

:::

:::tip

- Verify nd-mcp path is absolute and correct
- Make the file executable
- Check that `"type": "stdio"` is in your config
- Review VS Code Output → Continue for details

:::

:::warning

**"@netdata not recognized"**

:::

:::tip

- Restart VS Code after saving config
- Fix any JSON syntax errors (watch for trailing commas)
- Ensure "mcpServers" is at the config root level
- Remember: MCP only works in Continue's agent mode

:::

:::warning

**"Connection refused" errors**

:::

:::tip

- Run the network test again
- Verify Netdata is running
- Check port 19999 is open
- Try connecting without API key first

:::

:::warning

**"Authentication failed"**

:::

:::tip

- Double-check your API key (no extra spaces)
- Verify you're using the correct instance's key
- Test basic connectivity without API key

:::

### Use Continue with Netdata

Press Ctrl+L (or Cmd+L) to open Continue chat and query your infrastructure:

```
@netdata what's the current CPU usage across all servers?
```

```
@netdata show memory usage trends for the database server over the last 4 hours
```

```
@netdata are there any active alerts or anomalies?
```

```
@netdata which services are consuming the most resources right now?
```

<details>
<summary><strong>Still Having Issues?</strong></summary>
<br/>

1. **Enable debug logging** in Continue settings:
   ```json
   "continueOptions": {
     "logLevel": "debug"
   }
   ```

2. **Check all available logs**:
   - VS Code Output → Continue
   - VS Code Output → Extension Host
   - Developer Tools (Help → Toggle Developer Tools) → Console

3. **Try a minimal setup**:
   - Remove the API key temporarily
   - Use IP address instead of hostname
   - Disable any proxy settings
</details>

</details>

## Multiple Environments

### Set Up Project-Specific Connections

Create different Netdata connections for each project by adding `.vscode/settings.json`:

```json
{
  "continue.mcpServers": {
    "netdata-prod": {
      "type": "stdio",
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://prod-parent:19999/mcp?api_key=PROD_KEY"]
    }
  }
}
```

:::note

This lets you:
- Connect frontend projects to frontend servers
- Connect backend projects to backend infrastructure
- Give each team their own monitoring scope

:::

<details>
<summary><strong>Advanced Usage</strong></summary>
<br/>

### Create Custom VS Code Commands

Add Netdata queries to your command palette:

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

### Automate with Tasks

Add monitoring checks to `tasks.json`:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Check Production Metrics",
      "type": "shell",
      "command": "continue",
      "args": ["--ask", "@netdata show current system status"]
    }
  ]
}
```

### Create Monitoring Snippets

Speed up common queries with snippets:

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
</details>

</details>

<details>
<summary><strong>Troubleshooting</strong></summary>
<br/>

:::warning

**Extension can't find MCP**

:::

:::tip

- Restart VS Code after any configuration change
- Check extension logs (Output → Continue/Cline)
- Validate your JSON syntax

:::

:::warning

**Can't connect to Netdata**

:::

:::tip

- Test connection: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Verify nd-mcp is executable
- Check network access from your machine

:::

:::warning

**@netdata not working**

:::

:::tip

- Type `@netdata` exactly (case-sensitive)
- Verify MCP server configuration is saved
- Reload VS Code window (Ctrl+R)

:::

:::warning

**Slow performance**

:::

:::tip

- Connect to a local Netdata Parent for faster queries
- Monitor VS Code's memory usage
- Disable unnecessary extensions

:::
</details>

</details>

## Best Practices

### Optimize Your Workflow

1. Start every debugging session with infrastructure context
2. Check metrics before making performance changes
3. Validate fixes against real production data
4. Monitor deployment impact immediately

### Share with Your Team

Make monitoring accessible to everyone:

- Commit `.vscode/settings.json` with project-specific configs
- Document which Netdata Parent to connect to
- Create team snippets for common investigations