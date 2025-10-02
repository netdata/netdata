# JetBrains IDEs

Configure JetBrains IDEs to access your Netdata infrastructure through MCP.

## Supported IDEs

- IntelliJ IDEA
- PyCharm
- WebStorm
- PhpStorm
- GoLand
- DataGrip
- Rider
- CLion
- RubyMine

## Transport Support

JetBrains AI Assistant currently communicates with MCP servers over `stdio` only (https://www.jetbrains.com/help/ai-assistant/mcp.html).

| Transport | Support | Netdata Version | Notes |
|-----------|---------|-----------------|-------|
| **stdio** (via `nd-mcp`) | ✅ Fully Supported | v2.6.0+ | Launches bridge as subprocess |
| **stdio** (via `npx mcp-remote`) | ✅ Fully Supported | v2.7.2+ | Wrap remote HTTP/SSE in stdio |
| **Streamable HTTP / SSE** | ❌ Not Supported | - | Use a stdio launcher |
| **WebSocket** | ❌ Not Supported | - | Accessible only through `nd-mcp` |

> JetBrains documents a “workaround for remote servers” that relies on launching a stdio wrapper. Native HTTP/SSE support is not available yet.

## Prerequisites

1. **JetBrains IDE installed** - Any IDE from the list above
2. **AI Assistant plugin** - Install from IDE marketplace
3. **Netdata v2.6.0 or later** with MCP support - Prefer a Netdata Parent for full infrastructure visibility.
4. **Stdio launcher**:
   - `nd-mcp` bridge - Required for Netdata versions that only expose WebSocket (v2.6.0–v2.7.1)
   - `npx mcp-remote@latest` - Optional wrapper that exposes Netdata HTTP/SSE as stdio (useful for v2.7.2+)
5. **Netdata MCP API key exported before launching the IDE**:
   ```bash
   export ND_MCP_BEARER_TOKEN="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"
   ```
   Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

## Installing AI Assistant

1. Open your JetBrains IDE
2. Go to Settings/Preferences:
   - **Windows/Linux**: File → Settings → Plugins
   - **macOS**: IntelliJ IDEA → Preferences → Plugins
3. Search for "AI Assistant" in Marketplace
4. Install and restart IDE

## MCP Configuration

:::note
MCP support in JetBrains IDEs may require additional plugins or configuration. Check the plugin documentation for the latest setup instructions.
:::

### Method 1: Using nd-mcp Bridge (All Netdata versions v2.6.0+)

**AI Assistant Settings:**

1. Go to Settings → Tools → AI Assistant
2. Look for MCP or External Tools configuration
3. Add Netdata MCP server:

```json
{
  "name": "netdata",
  "command": "/usr/sbin/nd-mcp",
  "args": [
    "ws://YOUR_NETDATA_IP:19999/mcp"
  ]
}
```

**External Tools (if AI Assistant doesn't support MCP directly):**

1. Go to Settings → Tools → External Tools
2. Click "+" to add new tool
3. Configure:
   - **Name**: Netdata MCP
   - **Program**: `/usr/sbin/nd-mcp`
   - **Arguments**: `ws://YOUR_NETDATA_IP:19999/mcp`

### Method 2: Using npx mcp-remote (Netdata v2.7.2+)

For detailed options and troubleshooting, see [Using MCP Remote Client](/docs/learn/mcp.md#using-mcp-remote-client). JetBrains still launches this command over stdio; `mcp-remote` converts the remote HTTP/SSE session into the format AI Assistant understands.

**AI Assistant Settings:**

```json
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
```

**External Tools:**

- **Name**: Netdata MCP
- **Program**: `npx`
- **Arguments**: `mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp --allow-http --header "Authorization: Bearer NETDATA_MCP_API_KEY"`

Replace in all examples:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge) (nd-mcp method only)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)
- `ND_MCP_BEARER_TOKEN` - Export with your API key before launching the IDE (nd-mcp method only)

## Usage in Different IDEs

### IntelliJ IDEA (Java/Kotlin)

Monitor JVM applications:

```
// Ask AI Assistant about production performance
"What's the memory usage of our Java services?"
"Show me GC patterns in the last hour"
"Are there any thread pool issues?"
```

### PyCharm (Python)

Debug Python applications:

```python
# Ask: What's the CPU usage when this function runs in production?
def process_data():
    pass

# Ask: Show me memory patterns for the Python workers
```

### WebStorm (JavaScript/TypeScript)

Monitor Node.js applications:

```javascript
// Ask: What's the event loop latency?
// Ask: Show me API endpoint response times
// Ask: Any memory leaks in the Node processes?
```

### DataGrip (Databases)

Analyze database performance:

```sql
-- Ask: Show me database query latency
-- Ask: What's the connection pool usage?
-- Ask: Any slow queries in the last hour?
```

## IDE-Specific Features

### Code Annotations

Add infrastructure context to your code:

```java
@NetdataMonitor("cpu.usage > 80%")
public void resourceIntensiveMethod() {
    // AI Assistant can show real-time metrics
}
```

### Debugging with Metrics

While debugging:

1. Set breakpoint
2. Ask AI Assistant: "What were the system metrics when this code last ran in production?"
3. Get historical context for better debugging

### Performance Profiling

Combine IDE profiler with Netdata metrics:

- Run profiler in IDE
- Ask: "Show me system metrics during the profiling period"
- Correlate application and system performance

## Best Practices

### Development Workflow

1. Before deploying: "What's the current production load?"
2. During testing: "Compare metrics between dev and prod"
3. After deployment: "Show me metrics changes after deployment"

### Troubleshooting Production Issues

```
"Show me what happened at 14:32 when the error occurred"
"What were the system resources during the last OutOfMemory error?"
"Find correlated metrics during the last service degradation"
```

### Capacity Planning

```
"What's the resource usage trend for this service?"
"Project memory needs based on current growth"
"When will we need to scale based on current patterns?"
```

## Plugin Alternatives

If official MCP support is limited, consider:

### MCP Bridge Plugin

Search marketplace for:

- "MCP Client"
- "Model Context Protocol"
- "External AI Tools"

### Custom Plugin Development

Create a simple plugin that bridges JetBrains with Netdata:

1. Use IDE Plugin SDK
2. Implement MCP client
3. Add tool window for Netdata metrics

## Troubleshooting

### AI Assistant Not Connecting

- Check MCP configuration in settings
- Restart IDE after configuration changes

### No Netdata Option

- Ensure latest AI Assistant version
- Check for additional MCP plugins
- Try External Tools approach

### Connection Errors

- Test Netdata access: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Verify bridge path and permissions
- Check IDE logs for detailed errors

### Limited Functionality

- Some IDEs may have restricted AI Assistant features
- Try different JetBrains IDEs for better support
- Consider using Cursor or VS Code for full MCP support
