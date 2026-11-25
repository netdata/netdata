# ChatGPT

Configure the ChatGPT desktop app to access your Netdata observability data through MCP.

## Transport Support

The ChatGPT desktop app adds custom MCP servers from its settings, with these transports:

| Transport | Support | Netdata Version | Notes |
|-----------|---------|-----------------|-------|
| **Streamable HTTP** | ✅ Fully Supported | v2.7.2+ | Netdata Cloud MCP, or a Netdata Agent or Parent directly |
| **stdio** | ✅ Fully Supported | v2.6.0+ | Launch Netdata through `nd-mcp` |

## Prerequisites

1. **ChatGPT desktop app installed**
2. **One of:**
   - **Netdata Cloud** account with a Paid plan, nodes claimed to Netdata Cloud, and an API token with `scope:mcp`
     ([create one](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md))
   - **Netdata Agent or Parent** v2.6.0 or later, reachable from the computer running ChatGPT, and its MCP API key
     ([find your Netdata MCP API key](/docs/netdata-ai/mcp/README.md#finding-your-api-key)). Prefer a Netdata Parent
     for visibility across all its nodes.

## Add Netdata to ChatGPT

Open **Settings → Plugins → MCPs**, then **Add** and choose to connect a custom MCP. The **Connect to a custom MCP**
form opens.

### Netdata Cloud MCP

Connect to all your Netdata Cloud nodes through a single endpoint, with no local setup, bridges or firewall
changes.

| Field | Value |
|-------|-------|
| **Name** | `netdata-cloud` |
| **Type** | Streamable HTTP |
| **URL** | `https://app.netdata.cloud/api/v1/mcp` |
| **Bearer token env var** | the name of an environment variable holding your Netdata Cloud API token, for example `NETDATA_CLOUD_TOKEN` |

Instead of an environment variable, you can add a header under **Headers**: key `Authorization`, value
`Bearer YOUR_NETDATA_CLOUD_API_TOKEN`. The environment variable keeps the token out of the app's settings.

Select **Save**. For more details, see [Netdata Cloud MCP](/docs/netdata-ai/mcp/README.md#netdata-cloud-mcp).

### Local Agent or Parent: Streamable HTTP (Netdata v2.7.2+)

| Field | Value |
|-------|-------|
| **Name** | `netdata` |
| **Type** | Streamable HTTP |
| **URL** | `http://YOUR_NETDATA_IP:19999/mcp` |
| **Bearer token env var** | the name of an environment variable holding your Netdata MCP API key, for example `ND_MCP_BEARER_TOKEN` |

Use `https://` when the Agent or Parent is served over TLS.

### Local Agent or Parent: stdio bridge (all Netdata versions)

| Field | Value |
|-------|-------|
| **Name** | `netdata` |
| **Type** | STDIO |
| **Command** | `/usr/sbin/nd-mcp` |
| **Arguments** | `ws://YOUR_NETDATA_IP:19999/mcp` |
| **Environment variables** | `ND_MCP_BEARER_TOKEN` = your Netdata MCP API key |

The bridge reads the API key from the `ND_MCP_BEARER_TOKEN` environment variable.

## Environment Variables

The app reads environment variables when it starts. Set the variable in the environment the ChatGPT desktop app is
launched from, then restart the app. If the app cannot see the variable, use the **Headers** option instead.

## Using Netdata in ChatGPT

Ask about your systems, applications and network in plain language:

```text
Which nodes have the highest CPU utilization right now?
Show me anomalies across my database servers in the last hour.
What changed on the web servers before the latency spike at 14:00?
```

## Troubleshooting

### Netdata does not appear in ChatGPT

- Check that the server is listed and enabled under **Settings → Plugins → MCPs**
- Restart the ChatGPT desktop app after changing environment variables

### Authentication errors

- Netdata Cloud: the API token must have `scope:mcp`
- Agent or Parent: each Agent and Parent has its own MCP API key; use the key of the node in the URL
- Agent or Parent: verify the Netdata Agent is claimed to Netdata Cloud
- Check that the environment variable named in **Bearer token env var** is set for the app

### Connection errors

- Test that Netdata is reachable: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`
- Check that firewalls allow the connection to port 19999
