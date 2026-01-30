# Reporting

Extract infrastructure insights for stakeholders, leadership, and external tools. Netdata provides four ways to create reports—from asking a simple question to exporting raw metrics into your existing business intelligence stack.

## Methods overview

| Method | Effort | Best for |
|--------|--------|----------|
| **[AI Insights](#ai-insights)** | Easiest | Executive summaries, recurring reports, natural language queries |
| **[AI Assistants (MCP)](#ai-assistants-mcp)** | Easy | Ad-hoc analysis, deep investigation, developer workflows |
| **[Grafana](#grafana-integration)** | Medium | Custom dashboards, teams already using Grafana |
| **[Export to BI](#export-to-business-intelligence-tools)** | Advanced | Power BI, Tableau, Looker, custom analytics pipelines |

## AI Insights

Ask Netdata anything about your infrastructure in plain language and receive an executive-ready report. No configuration required—just describe what you need.

### When to use it

- Monday morning recap of weekend incidents
- Post-incident executive summary for leadership
- Weekly health checks and situational awareness
- Cost optimization analysis
- SLO conformance reporting

### How to generate a report

1. Open Netdata Cloud and go to `Insights`
2. Select a pre-built report type (Infrastructure Summary, Performance Optimization, Capacity Planning, Anomaly Analysis) or click `New Investigation` for a custom prompt
3. Choose the time range and scope (all nodes or a specific room/space)
4. Click `Generate`

Reports complete in 2–3 minutes and are saved in Insights. You receive an email when the report is ready.

### Scheduling recurring reports

Automate your reporting workflow with scheduled reports:

1. Configure your report as above
2. Click `Schedule` (next to `Generate`)
3. Choose cadence: daily, weekly, or monthly
4. Set the delivery time

Scheduled reports run automatically and deliver results to your email and the Insights tab.

### Example prompts

**Weekly infrastructure health:**
```
Generate a weekly infrastructure summary for services A, B, and C.
Include major incidents, anomalies, capacity risks, and recommended follow-ups.
```

**Cost optimization:**
```
Identify underutilized nodes for cost savings. Monthly compute is ~$12K
with mixed workloads. Goal: save $2–3K/month without reliability impact.
```

**SLO conformance:**
```
Generate an SLO conformance report for 'user-auth' (99.9% uptime,
p95 latency <200ms) for the last 7 days. Include breaches, contributing
factors, and remediation recommendations.
```

### Availability

- Available to Business and Free Trial plans
- Each report consumes 1 AI credit (10 free per month on eligible plans)
- Data privacy: metrics are summarized into structured context; your data is not used to train foundation models

## AI Assistants (MCP)

Connect your AI assistant directly to Netdata using the Model Context Protocol (MCP). Ask questions in natural language and receive answers based on live infrastructure data.

Every Netdata Agent and Parent (v2.6.0+) includes an MCP server. AI assistants can query metrics, alerts, logs, and live system information across your entire infrastructure.

### Supported AI clients

| Client | Description |
|--------|-------------|
| Claude Desktop | Anthropic's desktop AI assistant |
| Claude Code | Anthropic's CLI for development workflows |
| Cursor | AI-powered code editor |
| VS Code | Visual Studio Code with MCP support |
| JetBrains IDEs | IntelliJ, PyCharm, WebStorm, and others |
| Gemini CLI | Google's Gemini CLI |
| OpenAI Codex CLI | OpenAI's development tools |

### How to connect

```bash
# Export your MCP key
export NETDATA_MCP_API_KEY="$(cat /var/lib/netdata/mcp_dev_preview_api_key)"

# Connect using mcp-remote
npx mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp \
  --allow-http \
  --header "Authorization: Bearer $NETDATA_MCP_API_KEY"
```

### Example queries

Once connected, ask natural language questions:

- "Show me CPU usage for all nodes in the last 24 hours"
- "What are the top 10 processes by memory consumption?"
- "Find any anomalies in network traffic this week"
- "Generate a summary of all active alerts"
- "Which nodes have the highest disk utilization?"

### Availability

- Available on all plans (v2.6.0+)
- Unlimited queries with no per-query charges
- Requires API key for access to sensitive data

See [Netdata MCP](/docs/netdata-ai/mcp/README.md) for detailed setup instructions.

## Grafana integration

Connect Grafana to Netdata Cloud for infrastructure-wide dashboards. Use Grafana's visualization capabilities with Netdata's real-time metrics.

### When to use it

- Teams already using Grafana for other data sources
- Custom dashboard requirements beyond Netdata's built-in charts
- Combining Netdata metrics with data from other systems

### How to connect

1. Install the Netdata data source plugin in Grafana
2. Configure connection to Netdata Cloud using an API token
3. Create dashboards using Grafana's query builder

:::tip

Generate API tokens from Netdata Cloud under **User Settings** → **API Tokens**. See [API Tokens](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md) for details.

:::

### Availability

- Requires Netdata Cloud account
- Grafana Cloud or self-hosted Grafana
- API tokens available on all plans

## Export to business intelligence tools

Export metrics from Netdata to external databases and business intelligence platforms. You can query data from individual Agents or use Netdata Cloud to aggregate metrics from your entire infrastructure.

### Supported BI platforms

Netdata integrates with popular business intelligence tools through several pathways:

| BI Platform | Integration Options |
|-------------|---------------------|
| **Power BI** | Netdata Cloud API, Prometheus endpoint, or database export |
| **Tableau** | Netdata Cloud API, PostgreSQL, or Prometheus |
| **Looker / Looker Studio** | Netdata Cloud API, BigQuery, or Prometheus |
| **Qlik** | Netdata Cloud API, PostgreSQL, or InfluxDB |
| **SAP Analytics Cloud** | Netdata Cloud API or PostgreSQL |
| **Metabase** | Netdata Cloud API, PostgreSQL, or TimescaleDB |
| **Apache Superset** | Netdata Cloud API, PostgreSQL, or Prometheus |
| **Domo** | Netdata Cloud API or database connectors |
| **ThoughtSpot** | Netdata Cloud API or PostgreSQL |

### Query options

#### Netdata Cloud API (recommended)

The Netdata Cloud API lets you query metrics from all your nodes through a single endpoint. This is the simplest approach for multi-node infrastructure.

1. Generate an API token from **User Settings** → **API Tokens**
2. Use the token to authenticate requests to `https://app.netdata.cloud/api/v2/data`

```bash
# Query CPU metrics from all nodes
curl -H 'Accept: application/json' \
     -H "Authorization: Bearer YOUR_API_TOKEN" \
     'https://app.netdata.cloud/api/v2/data?contexts=system.cpu&after=-3600'

# Get list of all nodes in your space
curl -H 'Accept: application/json' \
     -H "Authorization: Bearer YOUR_API_TOKEN" \
     'https://app.netdata.cloud/api/v2/nodes'
```

The Cloud API returns aggregated data from all nodes in your infrastructure, making it ideal for BI tools that need a unified view.

#### Prometheus endpoint (single-node)

For single-node deployments or Prometheus-based workflows, query the Agent or Parent directly:

```
http://NODE_IP:19999/api/v3/allmetrics?format=prometheus
```

Replace `NODE_IP` with your Netdata Agent or Parent IP address:
- **Agent IP**: Returns metrics from that single node
- **Parent IP**: Returns aggregated metrics from all child agents connected to that Parent

This endpoint is useful when you need metrics from a specific node or when your BI tool already integrates with Prometheus.

#### REST API with JSON

Query specific metrics from an Agent or Parent in JSON format. This is useful for BI tools that need to combine Netdata metrics with other business data.

**Common BI use cases:**

```bash
# Daily averages for last 30 days, grouped by node
curl 'http://NODE_IP:19999/api/v3/data?contexts=system.cpu&after=-2592000&points=30&time_group=avg&group_by=node'

# Weekly max values for capacity planning
curl 'http://NODE_IP:19999/api/v3/data?contexts=system.ram&after=-604800&points=4&time_group=max&group_by=node'

# Hourly sum for cost analysis
curl 'http://NODE_IP:19999/api/v3/data?contexts=system.cpu&after=-86400&points=24&time_group=sum'
```

**Key parameters for BI workflows:**

| Parameter | Description | Example |
|-----------|-------------|---------|
| `contexts` | Metric context to query | `system.cpu`, `system.ram`, `disk.io` |
| `after` / `before` | Timeframe (seconds or Unix timestamp) | `-2592000` = last 30 days |
| `points` | Number of output points | `30` = daily for monthly view |
| `time_group` | Aggregation function | `avg`, `sum`, `min`, `max` |
| `group_by` | How to group results | `node`, `context`, `label:LABEL_NAME` |

Power BI, Tableau, and similar tools can consume this JSON through their data transformation features (Power Query, etc.).

#### Database export connectors

For persistent storage and historical analysis, export metrics to a database:

| Database | Connector |
|----------|-----------|
| PostgreSQL | Prometheus remote write adapter |
| TimescaleDB | Prometheus remote write or netdata-timescale-relay |
| InfluxDB | Graphite or Prometheus remote write |
| Elasticsearch | Graphite or Prometheus remote write |
| Google BigQuery | Prometheus remote write |
| AWS services | AWS Kinesis Data Streams |
| Azure services | Prometheus remote write |

See [Export Metrics to External Time-Series Databases](/docs/exporting-metrics/README.md) for full connector documentation.

### When to use this approach

- Existing BI infrastructure with established workflows
- Requirement to combine Netdata data with other business data sources
- Custom visualization needs not covered by built-in dashboards
- Long-term archival beyond Netdata's retention
- Compliance requirements for data export

## Choosing the right method

### Start with AI Insights if you:

- Need reports quickly without setup
- Want executive-ready summaries
- Prefer natural language over configuration
- Need recurring automated reports

### Use AI Assistants (MCP) if you:

- Want real-time answers to ad-hoc questions
- Already use Claude, Cursor, or similar AI tools
- Need deep investigation capabilities
- Prefer conversational interaction with your data

### Use Grafana if you:

- Already have Grafana deployed
- Need highly customized dashboards
- Want to combine Netdata with other data sources in one view
- Have team expertise in Grafana

### Export to BI tools if you:

- Have established Power BI, Tableau, or Looker workflows
- Need to combine infrastructure metrics with business data
- Require custom analytics beyond monitoring
- Have compliance requirements for data in specific systems

## Related documentation

- [AI Insights: Infrastructure Summary](/docs/netdata-ai/insights/infrastructure-summary.md)
- [AI Insights: Scheduled Reports](/docs/netdata-ai/insights/scheduled-reports.md)
- [Investigations](/docs/netdata-ai/investigations/index.md)
- [Netdata MCP](/docs/netdata-ai/mcp/README.md)
- [API Tokens](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md)
- [Export Metrics to External Time-Series Databases](/docs/exporting-metrics/README.md)
- [REST API](/src/web/api/README.md)
