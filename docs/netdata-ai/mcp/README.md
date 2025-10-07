# Netdata MCP

All Netdata Agents and Parents (v2.6.0+) are Model Context Protocol (MCP) servers, enabling AI assistants to interact with your infrastructure monitoring data.

Every Netdata Agent and Parent includes an MCP server, listening at same port the dashboard is listening at (default: `19999`).

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

## Transport Options

Netdata implements the MCP protocol with multiple transport options:

| Transport           | Endpoint                   | Use Case                                                     | Version Requirement  |
|---------------------|----------------------------|--------------------------------------------------------------|----------------------|
| **WebSocket**       | `ws://YOUR_IP:19999/mcp`   | Original transport, requires nd-mcp bridge for stdio clients | v2.6.0+              |
| **HTTP Streamable** | `http://YOUR_IP:19999/mcp` | Direct connection from AI clients supporting HTTP            | v2.7.2+              |
| **SSE**             | `http://YOUR_IP:19999/sse` | Server-Sent Events for real-time streaming                   | v2.7.2+              |

- **Direct Connection** (v2.7.2+): AI clients that support HTTP or SSE transports can connect directly to Netdata
- **Bridge Required**: AI clients that only support stdio need the `nd-mcp` (stdio-to-websocket) or `mcp-remote` (stdio-to-http or stdio-to-sse) bridge

### Official MCP Remote Client (mcp-remote)

If your AI client doesn't support HTTP/SSE directly and you don't want to use `nd-mcp`, you can use the official MCP remote client (requires Netdata v2.7.2+):

```bash
# Export your MCP key once per shell
export NETDATA_MCP_API_KEY="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"

# For HTTP transport
npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer $NETDATA_MCP_API_KEY"

# For SSE transport
npx mcp-remote@latest --sse http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer $NETDATA_MCP_API_KEY"
```

**Note:** The `--allow-http` flag is required for non-HTTPS connections. Only use this on trusted networks as traffic will not be encrypted.

## Finding the nd-mcp Bridge

> **Note**: With the new HTTP and SSE transports, many AI clients can now connect directly to Netdata without nd-mcp. Check your AI client's documentation to see if it supports direct HTTP or SSE connections.

The nd-mcp bridge is only needed for AI clients that:
- Only support `stdio` communication (like some desktop applications)
- Cannot use HTTP or SSE transports directly
- Cannot use `npx mcp-remote@latest`

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

## Using MCP Remote Client

The official MCP remote client (`mcp-remote`) is an alternative bridge that enables stdio-only AI clients to connect to Netdata's HTTP and SSE transports (requires Netdata v2.7.2+). Unlike nd-mcp which only supports WebSocket, mcp-remote provides broader transport compatibility.

### When to Use MCP Remote

Use `mcp-remote` when:
- Your AI client only supports stdio communication
- You want to use HTTP or SSE transports instead of WebSocket
- You're running Netdata v2.7.2 or later
- You don't want to build/install nd-mcp

### Installation

No installation required - `mcp-remote` runs via `npx`:

```bash
# Test the connection
npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer YOUR_API_KEY"
```

### Transport Options

`mcp-remote` supports multiple transport strategies:

```bash
# HTTP transport (recommended)
npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer YOUR_API_KEY"

# SSE transport
npx mcp-remote@latest --sse http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer YOUR_API_KEY"

# Auto-detect with fallback (tries SSE first, falls back to HTTP)
npx mcp-remote@latest --transport sse-first http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer YOUR_API_KEY"

# HTTPS (no --allow-http flag needed)
npx mcp-remote@latest --http https://YOUR_NETDATA_IP:19999/mcp \
  --header "Authorization: Bearer YOUR_API_KEY"
```

### Common Options

| Option         | Description                                          | Example                                                 |
|----------------|------------------------------------------------------|---------------------------------------------------------|
| `--http`       | Use HTTP transport                                   | `--http http://host:19999/mcp`                          |
| `--sse`        | Use SSE transport                                    | `--sse http://host:19999/mcp`                           |
| `--allow-http` | Allow non-HTTPS connections (required for HTTP URLs) | `--allow-http`                                          |
| `--header`     | Add custom headers (for authentication)              | `--header "Authorization: Bearer KEY"`                  |
| `--transport`  | Transport strategy                                   | `--transport sse-first` (tries SSE, falls back to HTTP) |
| `--debug`      | Enable debug logging                                 | `--debug`                                               |
| `--host`       | OAuth callback host (default: localhost)             | `--host 127.0.0.1`                                      |
| Port number    | OAuth callback port (optional)                       | `9696`                                                  |

### Authentication

For Netdata MCP, pass the API key via the Authorization header:

```bash
# Using environment variable (recommended)
export NETDATA_MCP_API_KEY="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"

npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer $NETDATA_MCP_API_KEY"
```

**Security Note:** The `--allow-http` flag is required for non-HTTPS connections. Only use this on trusted networks as traffic will not be encrypted.

### Troubleshooting

**Connection Issues:**
```bash
# Enable debug logging
npx mcp-remote@latest --debug --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer YOUR_API_KEY"

# Check debug logs (stored in ~/.mcp-auth/)
cat ~/.mcp-auth/*_debug.log
```

**Clear Authentication State:**
```bash
# Remove cached credentials
rm -rf ~/.mcp-auth
```

**Spaces in Arguments:**

Some AI clients (Cursor, Claude Desktop on Windows) have issues with spaces in arguments. Use environment variables as a workaround:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "npx",
      "args": [
        "mcp-remote@latest",
        "--http",
        "http://YOUR_IP:19999/mcp",
        "--allow-http",
        "--header",
        "Authorization: Bearer YOUR_API_KEY"
      ]
    }
  }
}
```

### Version Management

Always use the latest version:

```bash
# Force npx to check for latest version
npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp
```

Or in AI client configurations:
```json
{
  "args": ["mcp-remote@latest", "--http", "..."]
}
```

For more details, see the [official mcp-remote documentation](https://github.com/geelen/mcp-remote).

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

### Legacy Query String Support

For compatibility with older tooling, Netdata still accepts the `?api_key=YOUR_API_KEY` query parameter on the `/mcp` endpoints. New integrations should prefer the `Authorization: Bearer YOUR_API_KEY` header, but the query-string form remains available if you are migrating gradually.

## AI Client Specific Documentation

For detailed configuration instructions for specific AI clients, see:

**Chat Clients:**
- [Claude Desktop](/docs/netdata-ai/mcp/mcp-clients/claude-desktop.md) - Anthropic's desktop AI assistant
- [Cursor](/docs/netdata-ai/mcp/mcp-clients/cursor.md) - AI-powered code editor
- [Visual Studio Code](/docs/netdata-ai/mcp/mcp-clients/vs-code.md) - VS Code with MCP support
- [JetBrains IDEs](/docs/netdata-ai/mcp/mcp-clients/jetbrains-ides.md) - IntelliJ, PyCharm, WebStorm, etc.
- [Netdata Web Client](/docs/netdata-ai/mcp/mcp-clients/netdata-web-client.md) - Built-in web-based AI chat

**DevOps Copilots:**
- [Claude Code](/docs/netdata-ai/mcp/mcp-clients/claude-code.md) - Anthropic's CLI for Claude
- [Gemini CLI](/docs/netdata-ai/mcp/mcp-clients/gemini-cli.md) - Google's Gemini CLI
- [OpenAI Codex CLI](/docs/netdata-ai/mcp/mcp-clients/codex-cli.md) - OpenAI's Codex CLI
- [Crush](/docs/netdata-ai/mcp/mcp-clients/crush.md) - Charmbracelet's glamorous terminal AI
- [OpenCode](/docs/netdata-ai/mcp/mcp-clients/opencode.md) - SST's terminal-based AI assistant

Each guide includes specific transport support matrices and configuration examples optimized for that client.
