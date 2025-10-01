# Netdata MCP

All Netdata Agents and Parents are Model Context Protocol (MCP) servers, enabling AI assistants to interact with your infrastructure monitoring data.

Every Netdata Agent and Parent includes an MCP server that:

- Implements the protocol with multiple transport options: WebSocket, HTTP streamable, and SSE (Server-Sent Events)
- Provides read-only access to metrics, logs, alerts, and live system information
- Requires no additional installation - it's part of Netdata

## Transport Options

Netdata MCP supports three transport mechanisms:

| Transport | Endpoint | Use Case |
|-----------|----------|----------|
| **WebSocket** | `ws://YOUR_IP:19999/mcp` | Original transport, requires nd-mcp bridge for stdio clients |
| **HTTP Streamable** | `http://YOUR_IP:19999/mcp` | Direct connection from AI clients supporting HTTP |
| **SSE** | `http://YOUR_IP:19999/mcp?transport=sse` | Server-Sent Events for real-time streaming |

### Direct Connection vs Bridge

With the new HTTP and SSE transports, many AI clients can now connect directly to Netdata without needing the nd-mcp bridge:

- **Direct Connection**: AI clients that support HTTP or SSE transports can connect directly to Netdata
- **Bridge Required**: AI clients that only support stdio (like some desktop apps) still need the nd-mcp bridge or the official MCP remote client

### Official MCP Remote Client

If your AI client doesn't support HTTP/SSE directly and you don't want to use nd-mcp, you can use the official MCP remote client:

```bash
# Export your MCP key once per shell
export NETDATA_MCP_API_KEY="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"

# For SSE transport
npx mcp-remote@latest --sse http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer $NETDATA_MCP_API_KEY"

# For HTTP transport
npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer $NETDATA_MCP_API_KEY"
```

## Visibility Scope

Netdata provides comprehensive access to all available observability data through MCP, including complete metadata:

- **Node Discovery** - Hardware specifications, operating system details, version information, streaming topology, and associated metadata
- **Metrics Discovery** - Full-text search capabilities across contexts, instances, dimensions, and labels
- **Function Discovery** - Access to system functions including `processes`, `network-connections`, `streaming`, `systemd-journal`, `windows-events`, etc.
- **Alert Discovery** - Real-time visibility into active and raised alerts
- **Metrics Queries** - Complex aggregations and groupings with ML-powered anomaly detection
- **Metrics Scoring** - Root cause analysis leveraging anomaly detection and metric correlations
- **Alert History** - Complete alert transition logs and state changes
- **Function Execution** - Execute Netdata functions on any connected node (requires Netdata Parent)
- **Log Exploration** - Access logs from any connected node (requires Netdata Parent)

For sensitive features currently protected by Netdata Cloud SSO, a temporary MCP API key is generated on each Netdata instance. When presented via the `Authorization: Bearer` header, this key unlocks access to sensitive data and protected functions (like `systemd-journal`, `windows-events` and `processes`). This temporary API key mechanism will eventually be replaced with a new authentication system integrated with Netdata Cloud.

AI assistants have different visibility depending on where they connect:

- **Netdata Cloud**: (coming soon) Full visibility across all nodes in your infrastructure
- **Netdata Parent Node**: Visibility across all child nodes connected to that parent
- **Netdata Child/Standalone Node**: Visibility only into that specific node

## Finding the nd-mcp Bridge

> **Note**: With the new HTTP and SSE transports, many AI clients can now connect directly to Netdata without nd-mcp. Check your AI client's documentation to see if it supports direct HTTP or SSE connections.

The nd-mcp bridge is only needed for AI clients that:
- Only support `stdio` communication (like some desktop applications)
- Cannot use HTTP or SSE transports directly
- Cannot use `npx mcp-remote@latest`

AI clients like Claude Desktop run locally on your computer and use `stdio` communication. Since your Netdata runs remotely on a server, you need a bridge to convert `stdio` to WebSocket communication.

The `nd-mcp` bridge needs to be available on your desktop or laptop where your AI client runs. Since most users run Netdata on remote servers rather than their local machines, you have two options:

1. **If you have Netdata installed locally** - Use the existing nd-mcp
2. **If Netdata is only on remote servers** - Build nd-mcp on your desktop/laptop

### Option 1: Using Existing nd-mcp

If you have Netdata installed on your desktop/laptop, find the existing bridge:

#### Linux

```bash
# Try these locations in order:
which nd-mcp
ls -la /usr/sbin/nd-mcp
ls -la /usr/bin/nd-mcp
ls -la /opt/netdata/usr/bin/nd-mcp
ls -la /usr/local/bin/nd-mcp
ls -la /usr/local/netdata/usr/bin/nd-mcp

# Or search for it:
find / -name "nd-mcp" 2>/dev/null
```

Common locations:

- **Native packages (apt, yum, etc.)**: `/usr/sbin/nd-mcp` or `/usr/bin/nd-mcp`
- **Static installations**: `/opt/netdata/usr/bin/nd-mcp`
- **Built from source**: `/usr/local/netdata/usr/bin/nd-mcp`

#### macOS

```bash
# Try these locations:
which nd-mcp
ls -la /usr/local/bin/nd-mcp
ls -la /usr/local/netdata/usr/bin/nd-mcp
ls -la /opt/homebrew/bin/nd-mcp

# Or search for it:
find / -name "nd-mcp" 2>/dev/null
```

#### Windows

```powershell
# Check common locations:
dir "C:\Program Files\Netdata\usr\bin\nd-mcp.exe"
dir "C:\Netdata\usr\bin\nd-mcp.exe"
# Or search for it:
where nd-mcp.exe
```

### Option 2: Building nd-mcp for Your Desktop

If you don't have Netdata installed loca you can build just the nd-mcp bridge. Netdata provides three implementations - choose the one that best fits your environment:

1. **Go bridge** (recommended) - [Go bridge source code](https://github.com/netdata/netdata/tree/master/src/web/mcp/bridges/stdio-golang)
   - Produces a single binary with no dependencies
   - Creates executable named `nd-mcp` (`nd-mcp.exe` on windows)
   - Includes both `build.sh` and `build.bat` (for Windows)

2. **Node.js bridge** - [Node.js bridge source code](https://github.com/netdata/netdata/tree/master/src/web/mcp/bridges/stdio-nodejs)
   - Good if you already have Node.js installed
   - Creates script named `nd-mcp.js`
   - Includes `build.sh`

3. **Python bridge** - [Python bridge source code](https://github.com/netdata/netdata/tree/master/src/web/mcp/bridges/stdio-python)
   - Good if you already have Python installed
   - Creates script named `nd-mcp.py`
   - Includes `build.sh`

To build:

```bash
# Clone the Netdata repository
git clone https://github.com/netdata/netdata.git
cd netdata

# Choose your preferred implementation
cd src/web/mcp/bridges/stdio-golang/  # or stdio-nodejs/ or stdio-python/

# Build the bridge
./build.sh  # On Windows with the Go version, use build.bat

# The executable will be created with different names:
# - Go: nd-mcp
# - Node.js: nd-mcp.js
# - Python: nd-mcp.py

# Test the bridge with your Netdata instance (replace localhost with your Netdata IP)
./nd-mcp ws://localhost:19999/mcp      # Go bridge
./nd-mcp.js ws://localhost:19999/mcp   # Node.js bridge  
./nd-mcp.py ws://localhost:19999/mcp   # Python bridge

# You should see:
# nd-mcp: Connecting to ws://localhost:19999/mcp...
# nd-mcp: Connected
# Press Ctrl+C to stop the test

# Get the absolute path for your AI client configuration
pwd  # Shows current directory
# Example output: /home/user/netdata/src/web/mcp/bridges/stdio-golang
# Your nd-mcp path would be: /home/user/netdata/src/web/mcp/bridges/stdio-golang/nd-mcp
```

**Important**: When configuring your AI client, use the full absolute path to the executable:

- Go bridge: `/path/to/bridges/stdio-golang/nd-mcp`
- Node.js bridge: `/path/to/bridges/stdio-nodejs/nd-mcp.js`
- Python bridge: `/path/to/bridges/stdio-python/nd-mcp.py`

### Verify the Bridge Works

Once you have nd-mcp (either from existing installation or built), test it:

```bash
# Test connection to your Netdata instance (replace YOUR_NETDATA_IP with actual IP)
/path/to/nd-mcp ws://YOUR_NETDATA_IP:19999/mcp

# You should see:
# nd-mcp: Connecting to ws://YOUR_NETDATA_IP:19999/mcp...
# nd-mcp: Connected
# Press Ctrl+C to stop the test
```

## Finding Your API Key

To access sensitive functions like logs and live system information, you need an API key. Netdata automatically generates an API key on startup. The key is stored in a file on the Netdata server you want to connect to.

You need the API key of the Netdata you will connect to (usually a Netdata Parent).

**Note**: This temporary API key mechanism will eventually be replaced by integration with Netdata Cloud.

### Find the API Key File

```bash
# Try the default location first:
sudo cat /var/lib/netdata/mcp_dev_preview_api_key

# For static installations:
sudo cat /opt/netdata/var/lib/netdata/mcp_dev_preview_api_key

# If not found, search for it:
sudo find / -name "mcp_dev_preview_api_key" 2>/dev/null
```

### Copy the API Key

The file contains a UUID that looks like:

```
a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

Copy this entire string - you'll need it for your AI client configuration.

### No API Key File?

If the file doesn't exist:

1. Ensure you have a recent version of Netdata
2. Restart Netdata: `sudo systemctl restart netdata`
3. Check the file again after restart

## AI Client Configuration

AI clients can connect to Netdata MCP in different ways depending on their transport support:

### Direct Connection (HTTP/SSE)

For AI clients that support HTTP or SSE transports:

```json
{
  "mcpServers": {
    "netdata": {
      "type": "http",
      "url": "http://IP_OF_YOUR_NETDATA:19999/mcp",
      "headers": [
        "Authorization: Bearer YOUR_API_KEY"
      ]
    }
  }
}
```

Or for SSE:

```json
{
  "mcpServers": {
    "netdata": {
      "type": "sse",
      "url": "http://IP_OF_YOUR_NETDATA:19999/mcp?transport=sse",
      "headers": [
        "Authorization: Bearer YOUR_API_KEY"
      ]
    }
  }
}
```

### Using nd-mcp Bridge (stdio)

For AI clients that only support stdio:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": [
        "--bearer",
        "YOUR_API_KEY",
        "ws://IP_OF_YOUR_NETDATA:19999/mcp"
      ]
    }
  }
}
```

### Using Official MCP Remote Client

```json
{
  "mcpServers": {
    "netdata": {
      "command": "npx",
      "args": [
        "mcp-remote@latest",
        "--http",
        "http://IP_OF_YOUR_NETDATA:19999/mcp",
        "--header",
        "Authorization: Bearer YOUR_API_KEY"
      ]
    }
  }
}
```

Replace:

- `IP_OF_YOUR_NETDATA`: Your Netdata instance IP/hostname
- `YOUR_API_KEY`: The API key from the file mentioned above
- `/usr/sbin/nd-mcp`: With your actual nd-mcp path (if using the bridge)

### Multiple MCP Servers

You can configure multiple Netdata instances:

```json
{
  "mcpServers": {
    "netdata-production": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["--bearer", "PROD_KEY", "ws://prod-parent:19999/mcp"]
    },
    "netdata-testing": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["--bearer", "TEST_KEY", "ws://test-parent:19999/mcp"]
    }
  }
}
```

Note: Most AI clients have difficulty choosing between multiple MCP servers. You may need to enable/disable them manually.

### Legacy Query String Support

For compatibility with older tooling, Netdata still accepts the `?api_key=YOUR_API_KEY` query parameter on the `/mcp` endpoints. New integrations should prefer the `Authorization: Bearer YOUR_API_KEY` header, but the query-string form remains available if you are migrating gradually.

## AI Client Specific Documentation

For detailed configuration instructions for specific AI clients, see:

- [Claude Code](/docs/ml-ai/ai-devops-copilot/claude-code.md) - Anthropic's CLI for Claude
- [Gemini CLI](/docs/ml-ai/ai-devops-copilot/gemini-cli.md) - Google's Gemini CLI
- [OpenAI Codex CLI](/docs/ml-ai/ai-devops-copilot/codex-cli.md) - OpenAI's Codex CLI
- [Crush](/docs/ml-ai/ai-devops-copilot/crush.md) - Charmbracelet's glamorous terminal AI
- [OpenCode](/docs/ml-ai/ai-devops-copilot/opencode.md) - SST's terminal-based AI assistant

Each guide includes specific transport support matrices and configuration examples optimized for that client.
