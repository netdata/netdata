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

## Prerequisites

1. **JetBrains IDE installed** - Any IDE from the list above
2. **AI Assistant plugin** - Install from IDE marketplace
3. **The IP and port (usually 19999) of a running Netdata Agent** - Prefer a Netdata Parent to get infrastructure level visibility. Currently the latest nightly version of Netdata has MCP support (not released to the stable channel yet). Your AI Client (running on your desktop or laptop) needs to have direct network access to this IP and port.
4. **`nd-mcp` program available on your desktop or laptop** - This is the bridge that translates `stdio` to `websocket`, connecting your AI Client to your Netdata Agent or Parent. [Find its absolute path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
5. **Optionally, the Netdata MCP API key** that unlocks full access to sensitive observability data (protected functions, full access to logs) on your Netdata. Each Netdata Agent or Parent has its own unique API key for MCP - [Find your Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

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

### Method 1: AI Assistant Settings

1. Go to Settings → Tools → AI Assistant
2. Look for MCP or External Tools configuration
3. Add Netdata MCP server:

```json
{
  "name": "netdata",
  "command": "/usr/sbin/nd-mcp",
  "args": [
    "ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY"
  ]
}
```

### Method 2: External Tools

If direct MCP support is not available, configure as an External Tool:

1. Go to Settings → Tools → External Tools
2. Click "+" to add new tool
3. Configure:
   - **Name**: Netdata MCP
   - **Program**: `/usr/sbin/nd-mcp`
   - **Arguments**: `ws://YOUR_NETDATA_IP:19999/mcp?api_key=NETDATA_MCP_API_KEY`

Replace:

- `/usr/sbin/nd-mcp` - With your [actual nd-mcp path](/docs/learn/mcp.md#finding-the-nd-mcp-bridge)
- `YOUR_NETDATA_IP` - IP address or hostname of your Netdata Agent/Parent
- `NETDATA_MCP_API_KEY` - Your [Netdata MCP API key](/docs/learn/mcp.md#finding-your-api-key)

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
