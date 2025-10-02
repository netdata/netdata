# Netdata Model Context Protocol (MCP) Integration

Netdata Agents (and soon Netdata Cloud) provide a Model Context Protocol (MCP) server that enables AI assistants like Claude or Cursos to interact with your infrastructure monitoring data. This integration allows AI assistants to access metrics, logs, alerts, and live system information (processes, services, containers, VMs, network connections, etc), acting as a capable DevOps/SRE/SysAdmin assistant.

## Overview

The AI assistants have different visibility on your infrastructure, depending on where in a Netdata hierarchy they are connected:

 - **Netdata Cloud**: (not yet available) AI assistants connected to Netdata Cloud will have full visibility across all nodes in your infrastructure.
 - **Netdata Parent Node**: AI assistants connected to a Netdata parent node will have visibility across all child nodes connected to that parent.
 - **Netdata Child Node**: AI assistants connected to a Netdata child node will only have visibility into that specific node.
 - **Netdata Standalone Node**: AI assistants connected to a standalone Netdata node will only have visibility into that specific node.

## Supported AI Assistants

You can use Netdata with the following AI assistants:

- [Claude Desktop](https://claude.ai/download): supports flat-fee usage for unlimited access
- [Claude Code](https://claude.ai/code): supports flat-fee usage for unlimited access
- [Cursor](https://www.cursor.com/): supports flat-fee usage for unlimited access. Enables Netdata use with multiple AI assistants, including Claude, ChatGPT, and Gemini.

Probably more: Check the [MCP documentation](https://modelcontextprotocol.io/clients) for a full list of supported AI assistants.

All these AI assistants need local access to the MCP servers. When the client supports **HTTP streamable** or **Server-Sent Events (SSE)** transports (for example, `npx @modelcontextprotocol/remote-mcp`), it can now connect directly to Netdata's `/mcp` (HTTP) or `/sse` endpoints—no custom bridge required.

Many desktop assistants, however, still talk to MCP servers over `stdio`. For them you still need a bridge that converts `stdio` to a network transport. Netdata keeps shipping the `nd-mcp` bridge (plus the polyglot bridges in `bridges/`) for this purpose.

Once MCP is integrated into Netdata Cloud, Web-based AI assistants will also be supported. For Web-based AI assistants, the backend of the assistant connects to a publicly accessible MCP server (i.e. Netdata Cloud) to access infrastructure observability data, without needing a bridge.

## Installation

The MCP server is built into Netdata and requires no additional installation. Just ensure you have a recent version of Netdata installed.

To use the MCP integration of Netdata with AI clients, you need to configure them and bridge them to the Netdata MCP server.

## Configuration of AI Assistants

The configuration of most AI assistants is done via a configuration file, which is almost identical for all of them.

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/bin/nd-mcp",
      "args": [
        "--bearer",
        "YOUR_API_KEY",
        "ws://IP_OF_YOUR_NETDATA:19999/mcp"
      ]
    }
  }
}
```

The program `nd-mcp` is still the universal bridge that converts `stdio` communication to network transports. This program is part of all Netdata installations, so by installing Netdata on your personal computer (Linux, macOS, Windows) you will have it available.

There may be different paths for it, depending on how you installed Netdata:

- `/usr/bin/nd-mcp` or `/usr/sbin/nd-mcp`: Linux native packages (together with the `netdata` and `netdatacli` commands)
- `/opt/netdata/usr/bin/nd-mcp`: Linux static Netdata installations
- `/usr/local/netdata/usr/bin/nd-mcp`: MacOS installations from source
- `C:\\Program Files\\Netdata\\usr\\bin\\nd-mcp.exe`: Windows installations

### Native HTTP/SSE connection (remote-mcp)

If your client supports HTTP or SSE, you can skip the bridge entirely. The Netdata agent exposes two MCP HTTP endpoints on the same port as the dashboard:

| Endpoint | Transport | Notes |
| --- | --- | --- |
| `http://IP_OF_YOUR_NETDATA:19999/mcp` | Streamable HTTP (chunked JSON) | Default response; add `Accept: application/json` |
| `http://IP_OF_YOUR_NETDATA:19999/mcp?transport=sse` | Server-Sent Events | Equivalent to sending `Accept: text/event-stream` |

To test quickly with the official MCP CLI:

```bash
npx @modelcontextprotocol/remote-mcp \
  --sse http://IP_OF_YOUR_NETDATA:19999/mcp \
  --header "Authorization: Bearer YOUR_API_KEY"
```

Or, to prefer streamable HTTP:

```bash
npx @modelcontextprotocol/remote-mcp \
  --http http://IP_OF_YOUR_NETDATA:19999/mcp \
  --header "Authorization: Bearer YOUR_API_KEY"
```

These commands let you browse the Netdata MCP tools without installing `nd-mcp`. You can still keep `nd-mcp` in your assistant configuration as a fallback for clients that only speak `stdio`.

You will also need:

`IP_OF_YOUR_NETDATA`, is the IP address or hostname of the Netdata instance you want to connect to. This will eventually be replaced by the Netdata Cloud URL. For this dev preview, use any Netdata, preferably one of your parent nodes. Remember that the AI assistant will "see" only the nodes that are connected to that Netdata instance.

`YOUR_API_KEY` is the API key that allows the AI assistant to access sensitive functions like logs and live system information. Just start Netdata and it will automatically generate a random UUID for you. You can find it at:

```
/var/lib/netdata/mcp_dev_preview_api_key
```

or, if you installed a static Netdata package, it may be located at:

```
/opt/netdata/var/lib/netdata/mcp_dev_preview_api_key
```


To view your API key:
```bash
sudo cat /var/lib/netdata/mcp_dev_preview_api_key
```

or

```bash
sudo cat /opt/netdata/var/lib/netdata/mcp_dev_preview_api_key
```

### Claude Desktop

To add Netdata MCP to Claude Desktop:

1. Open Claude Desktop
2. Navigate to the Developer settings:
  - **Windows/Linux**: File → Settings → Developer (or use Ctrl+,)
  - **macOS**: Claude → Settings → Developer (or use Cmd+,)
3. Click the "Edit Config" button (below the server list)
4. This will open or show the exact configuration file location
5. Add the configuration mentioned above to that file.

**Linux Users**: Claude Desktop is available via a community project (https://github.com/fsoft72/claude-desktop-to-appimage). It works best with https://github.com/TheAssassin/AppImageLauncher.

Once configured correctly, you will need to restart Claude Desktop.
Once restarted, you should see "netdata" appear in Claude Desktop:
- Click the "Search and tools" button (just below the prompt)
- You should see "netdata" listed among the available tools
- If you don't see it, check your configuration and ensure the bridge is accessible

### Claude Code

For [Claude Code](https://claude.ai/code), add to your project's root, the file `.mcp.json`, with the contents given above. This file will be automatically detected by Claude Code the next time it starts in that directory.

Alternatively, you can add it using a Claude CLI command like this:

```bash
claude mcp add netdata /usr/bin/nd-mcp --bearer YOUR_API_KEY ws://IP_OF_YOUR_NETDATA:19999/mcp
```

Once configured correctly, run `claude mcp list` or you can issue the command `/mcp` to your Claude Code. It should show you the available MCP servers, including "netdata".

### Cursor

For [Cursor](https://www.cursor.com/), add the configuration to the MCP settings.

## Alternative `stdio` to `websocket` Bridges
These bridges remain useful for AI assistants that only support `stdio`. If your tooling can use Netdata's native HTTP/SSE endpoints you can skip this section.

We provide 3 different bridges for you to choose the one that best fits your environment:

1. **Go bridge**: Located at `src/web/mcp/bridges/stdio-golang/`
2. **Node.js bridge**: Located at `src/web/mcp/bridges/stdio-nodejs/`
3. **Python bridge**: Located at `src/web/mcp/bridges/stdio-python/`

All these bridges should provide exactly the same functionality, so you can choose the one that best fits your environment.

Each of these directories includes `build.sh` script to install dependencies and prepare the bridge.
The Go bridge provides also a `build.bat` script for Windows users.

## Capabilities

The MCP integration provides AI assistants with access to:

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

Once configured, you can ask questions like:

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

## FAQ

- **Q: Can I use MCP with other AI assistants?**
  - A: Yes, MCP supports multiple AI assistants. Check the [MCP documentation](https://modelcontextprotocol.io/clients) for a full list.

- **Q: Do I need to run a bridge on my local machine?**
- A: Only if your client speaks `stdio` (Claude Desktop, Cursor, etc). Modern MCP clients such as `npx @modelcontextprotocol/remote-mcp` can talk HTTP/SSE directly to Netdata's `/mcp` endpoints, so no bridge is required in that case. Keep `nd-mcp` as a fallback for assistants that still require `stdio`.

- **Q: How do I find my API key?**
  - A: The API key is automatically generated by Netdata and stored in `/var/lib/netdata/mcp_dev_preview_api_key` or `/opt/netdata/var/lib/netdata/mcp_dev_preview_api_key` on the Netdata Agent you will connect to. Use `sudo cat` to view it.

- **Q: Can I use MCP with Netdata Cloud?**
  - A: Yes, once MCP is integrated into Netdata Cloud, you will be able to use it with web-based AI assistants without needing a bridge.

- **Q: What data can I access with MCP?**
  - A: You can access metrics, logs, alerts, live system information (processes, services, containers, network connections), and more.

- **Q: Can I use MCP with my existing Netdata installation?**
  - A: Yes, as long as you have a recent version of Netdata installed, you can use the MCP integration without any additional installation.

- **Q: Is MCP secure?**
  - A: Yes, MCP currently provides read-only access. Sensitive functions like logs and live system information require an API key, and the agent should be claimed to Netdata Cloud for production use.

- **Q: Will my observability data be exposed to AI companies?**
  - A: Yes, but it depends on the AI assistant you use and the subscription you have. For example, Claude promises that your data will not be used to train their models for certain subscriptions, and Cursor allows you to use multiple AI assistants. Always check the privacy policies of the AI assistant you choose.

- **Q: Are the responses of AI assistants accurate?**
  - A: AI assistants like Claude are designed to provide accurate and relevant responses based on the data they have access to. However, they may not always be perfect, or they may have not checked all the aspects before giving answers. It's important to verify critical information.

## Best Practices

### AI Assistants sampling data

Sometimes, when you ask generic questions about your infrastructure, AI assistants do a simple sampling on a few nodes of the infrastructure, instead of querying all nodes. In Netdata we have provided the tools to properly do that, but AI assistants may not use them.

Examples:

Q: "which are the top processes/containers/VMs/services running on my servers?"

The AI assistant may respond with a list of processes/containers/VMs/services from a few nodes, instead of querying all nodes.

The proper way in Netdata is to query:

 - `app.*` charts/contexts for `processes`, which will return the processes running on all nodes grouped by category.
 - `systemd.*` to get the services running on all nodes.
 - `cgroup.*` to get the all the containers and VMs on all nodes.

For all such queries, Netdata responses return cardinality information (much like the NIDL charts on your Netdata dashboard), so the AI assistant could get a much better picture instead of sampling data. When you notice that, you could ask the AI assistant to find the answer using more generic queries.

### AI Assistants missing newer Netdata features

Sometimes you ask AI assistants about features that have been recently added to Netdata (eg logs, or windows capabilities), and the AI assistant instead of checking what is available via their MCP connection, they say that Netdata does not support that feature. Answering "check your MCP tools, features, functions" is usually enough for the AI assistant to check the available features and start using them.

### AI Assistants not using MCP at all

Sometimes you need instruct them to use their MCP connection. So instead of saying "check the performance of my production db", you can say "use netdata to check the performance of my production db". This way, the AI assistant will use its MCP connection to query the Netdata instance and provide you with the relevant information.

### Use AI Assistants to do your DevOps/SRE/SysAdmin "laundry"

Our advice is to use AI assistants to do "your laundry": Give them specific tasks, check the queries they did to get that information, and when possible ask them to cross-check their answers using a different tool/source. AI assistants usually rush to make conclusions, so **challenge them** and they will go deeper and correct themselves. Remember that you always need to verify their answers, especially for critical tasks.

### Multiple Netdata MCP servers for a single AI assistant

If you need to configure multiple MCP servers, you can add them under the `mcpServers` section with different names. Example:

```json
{
  "mcpServers": {
    "netdata-production": {
        "command": "/usr/bin/nd-mcp",
        "args": [
          "--bearer",
          "YOUR_API_KEY",
          "ws://IP_OF_YOUR_NETDATA:19999/mcp"
        ]
    },
    "netdata-testing": {
        "command": "/usr/bin/nd-mcp",
        "args": [
          "--bearer",
          "YOUR_API_KEY",
          "ws://IP_OF_YOUR_NETDATA:19999/mcp"
        ]
    }
  }
}
```

However, when multiple netdata MCP servers are configured, all AI assistants have difficulties to determine which one to use:

- **Claude Desktop**: it seems there is no way to instruct it to use the right one. There is an enable/disable toggle for each MCP server, which however does not work properly. So, it is best to configure only one MCP server at a time.
- **Cursor**: Similarly, it is impossible to instruct it to use the right one. However, there is a toggle and it works properly, but you still need to ensure that the Netdata server you want to use is the only one enabled.
- **Claude Code**: this project has a different philosophy: you can have a different `.mcp.json` file in each project directory (the current directory from which you run it), so you can have different configurations for each project/directory. Since **Claude Code** also supports a `Claude.md` file with default instructions to the AI assistant, you can have different directories with different instructions and configurations, so you can use multiple Netdata MCP servers by spawning multiple Claude Code instances in different directories.

For more information about Netdata, visit [netdata.cloud](https://netdata.cloud)
