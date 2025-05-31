# Netdata Model Context Protocol (MCP) Integration

All Netdata Agents provide a Model Context Protocol (MCP) server that enable AI assistants like Claude to interact with your infrastructure monitoring data. This integration allows AI assistants to access metrics, logs, alerts, and system information, acting as a capable DevOps/SRE/SysAdmin assistant.

## Overview

When connected to a Netdata parent node, the MCP integration provides complete visibility across all child nodes connected to that parent. This makes it an ideal solution for infrastructure-wide observability through AI assistants.

## Installation

The MCP server is built into Netdata and requires no additional installation. Simply ensure you have a recent version of Netdata installed.

## Configuration

### 1. Enable MCP in Netdata

MCP is enabled by default in Netdata. The server communicates via WebSocket on the same port as the Netdata web interface.

### 2. Generate API Key (Dev Preview)

During the dev preview phase, Netdata automatically generates a random API key stored at:
```
/var/lib/netdata/mcp_dev_preview_api_key
```

To view your API key:
```bash
sudo cat /var/lib/netdata/mcp_dev_preview_api_key
```

**Important**: 
- In future releases, this feature will be available exclusively for Netdata Cloud users
- For full access to sensitive functions (logs and live functions), your Netdata agent must be claimed to Netdata Cloud
- Without an API key or unclaimed agent, all features work except functions and logs

### 3. Install a Bridge

Claude Desktop and Claude Code communicate via stdio, while Netdata's MCP server uses WebSocket. You need a bridge to convert between these protocols. Netdata provides bridges in multiple languages:

- **Node.js bridge**: `src/web/mcp/bridges/stdio-nodejs/`
- **Python bridge**: `src/web/mcp/bridges/stdio-python/`
- **Go bridge**: `src/web/mcp/bridges/stdio-golang/`

Each of these directories includes `build.sh` script to install dependencies and prepare the bridge.
The Go bridge provides also a `build.bat` script for Windows users.

Choose the bridge that matches your environment.

#### Installing the Node.js Bridge

```bash
bash /path/to/netdata.git/src/web/mcp/bridges/stdio-nodejs/build.sh
```

### 4. Configure Claude Desktop

To add Netdata MCP to Claude Desktop:

1. Open Claude Desktop
2. Navigate to the Developer settings:
   - **Windows/Linux**: File → Settings → Developer (or use Ctrl+,)
   - **macOS**: Claude → Settings → Developer (or use Cmd+,)
3. Click the "Edit Config" button (below the server list)
4. This will open or show the exact configuration file location
5. Add the following configuration to the file:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/path/to/netdata.git/src/web/mcp/bridges/stdio-nodejs/nd-mcp.js",
      "args": [
        "ws://IP_OF_YOUR_NETDATA:19999/mcp?api_key=YOUR_API_KEY"
      ]
    }
  }
}
```

**Linux Users**: If using the AppImage version (https://github.com/fsoft72/claude-desktop-to-appimage), it works best with https://github.com/TheAssassin/AppImageLauncher

Replace:
- `/path/to/netdata/` with the actual path to your Netdata source directory
- `ws://localhost:19999/api/v2/mcp` with your Netdata WebSocket URL if different
- `your_api_key_here` with the actual API key from `/var/lib/netdata/mcp_dev_preview_api_key`

### 5. Configure Claude Code

For Claude Code (claude.ai/code), add to your project's `.mcp.json` file:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/path/to/netdata.git/src/web/mcp/bridges/stdio-nodejs/nd-mcp.js",
      "args": [
        "ws://IP_OF_YOUR_NETDATA:19999/mcp?api_key=YOUR_API_KEY"
      ]
    }
  }
}
```

### 6. Verify the Connection

Once configured correctly, you should see "netdata" appear in Claude Desktop:
- Click the "Search and tools" button (just below the prompt)
- You should see "netdata" listed among the available tools
- If you don't see it, check your configuration and ensure the bridge is accessible

## Using Alternative Bridges

### Python Bridge

If you prefer Python:

```bash
bash /path/to/netdata.git/src/web/mcp/bridges/stdio-python/build.sh
```

Then use `python` as the command and `bridge.py` as the script:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/path/to/netdata.git/src/web/mcp/bridges/stdio-python/nd-mcp.py",
      "args": [
        "ws://IP_OF_YOUR_NETDATA:19999/mcp?api_key=YOUR_API_KEY"
      ]
    }
  }
}
```

### Go Bridge

For Go users:

```bash
bash /path/to/netdata.git/src/web/mcp/bridges/stdio-golang/build.sh
```

Then use the compiled binary:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/path/to/netdata.git/src/web/mcp/bridges/stdio-golang/nd-mcp",
      "args": [
        "ws://IP_OF_YOUR_NETDATA:19999/mcp?api_key=YOUR_API_KEY"
      ]
    }
  }
}
```

## Capabilities

The MCP integration provides access to:

### Infrastructure Discovery
- **Nodes information**: Complete visibility across all connected nodes in your infrastructure
  - Hardware specifications, OS details, virtualization info
  - Streaming configuration and parent-child relationships
  - Connection status and data collection capabilities
- **Metrics discovery**: All metrics collected by your Netdata installation
  - System metrics: CPU, memory, disks, network interfaces
  - Application metrics: databases, web servers, containers
  - Hardware metrics: IPMI sensors, GPU, temperature sensors
  - Custom metrics: StatsD, logs-based metrics

### Metrics and Analytics
- **Time-series queries**: Powerful data aggregation and analysis
  - Multiple grouping options: by dimension, instance, node, or label
  - Aggregation methods: sum, average, min, max, percentages
  - Time aggregations: average, min, max, median, percentile, etc.
- **Anomaly detection**: ML-powered anomaly detection across all metrics
  - Real-time anomaly rates (0-100% of time anomalous)
  - Per-metric and per-dimension anomaly tracking
- **Correlation analysis**: Find metrics that changed during incidents
  - Compare problem periods with baseline periods
  - Statistical and volume-based correlation methods
- **Variability analysis**: Identify unstable or fluctuating metrics

### Live System Information (requires API key and claimed agent)
- **Processes**: Detailed process information including:
  - CPU usage, memory consumption, I/O statistics
  - File descriptors, page faults, parent-child relationships
  - Container-aware process tracking
- **Network connections**: Active connections with:
  - Protocol details, states, addresses, ports
  - Performance metrics per connection
- **Systemd services & units**: Service health and resource usage
- **Mount points**: Filesystem usage, capacity, and inode statistics
- **Block devices**: I/O performance, latency, and utilization
- **Containers & VMs**: Resource usage across containerized workloads
- **Network interfaces**: Traffic rates, packets, drops, link status
- **Streaming status**: Real-time replication and ML synchronization

### Logs Access (requires API key and claimed agent)
- **systemd-journal**: Comprehensive log access including:
  - Local system logs, user logs, and namespaces
  - Remote system logs from connected nodes
  - Advanced filtering and search capabilities
  - Historical log data based on retention
- **Windows events**: Query Windows event logs (on Windows systems)

### Alerts and Monitoring
- **Active alerts**: Currently raised warnings and critical alerts
  - Detailed alert information including values, timestamps, and context
  - Alert classification by type, component, and severity
- **Alert history**: Complete alert state tracking
  - All states: critical, warning, clear, undefined, uninitialized
  - Alert transitions with timestamps and values
- **Alert metadata**: Recipients, configurations, and thresholds

### Available Metric Categories
The integration provides access to all metrics categories collected by Netdata including:
- Core system: CPU, memory, disks, network, processes
- Containers: Docker, cgroups, systemd services
- Databases: MySQL, PostgreSQL, Redis, MongoDB
- Web servers: Apache, Nginx, LiteSpeed
- Hardware: IPMI, GPUs, temperature sensors, SMART
- Network services: DNS, DHCP, VPN, firewalls
- Applications: Custom StatsD metrics, logs-based metrics
- And any other metrics collected by your Netdata installation

## Security Considerations

- The MCP integration currently provides **read-only** access to Netdata
- Dynamic configuration is not exposed - AI assistants cannot read or modify Netdata settings
- API key is required for accessing sensitive functions (logs and live data)
- For production use, ensure your Netdata agent is claimed to Netdata Cloud

## Usage Examples

Once configured, you can ask Claude questions like:

### Infrastructure Overview
- "Show me all connected nodes and their status"
- "What metrics are available for my database servers?"
- "Provide an observability coverage report for my infrastructure"
- "Which nodes are offline or having connection issues?"

### Performance Analysis
- "What are the top CPU-consuming processes across all my servers?"
- "Show me network interface utilization across all nodes"
- "What's the memory usage trend for my database servers?"
- "List all block devices and their I/O performance"
- "Which of my nodes have disk backlog issues?"
- "Show me container resource usage statistics"

### Anomaly Detection and Troubleshooting
- "Which metrics are showing anomalous behavior in the last hour?"
- "Find metrics that changed significantly during the outage at 2 PM"
- "What are the most unstable metrics in my infrastructure?"
- "Analyze the correlation between disk I/O and application response time"

### Alerts and Monitoring
- "Are there any critical alerts currently active?"
- "Show me all alert transitions in the last 24 hours"
- "Which systems have disk space warnings?"
- "What alerts fired and cleared during the night?"

### System Logs and Events
- "Show me systemd journal logs for failed services"
- "Search for authentication failures in the last hour"
- "Display kernel errors from all nodes"
- "Find all logs related to out-of-memory conditions"

### Live System State
- "List all systemd services and their status"
- "Show me active network connections on the web servers"
- "What's the current streaming replication status?"
- "Display mount points with low available space"

## Troubleshooting

1. **Connection refused**: Ensure Netdata is running and accessible at the specified URL
2. **Bridge not found**: Verify the bridge path is correct and dependencies are installed
3. **Authentication errors**: Verify the API key is correct and the agent is claimed
4. **Missing data**: Check that the Netdata agent has the required collectors enabled
5. **Limited access**: Without API key or unclaimed agent, functions and logs won't be available

## Future Enhancements

- Integration with Netdata Cloud for enhanced authentication
- Support for dynamic configuration management
- Extended function capabilities
- Custom alert rule creation through MCP

For more information about Netdata, visit [netdata.cloud](https://netdata.cloud)
