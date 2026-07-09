# MCP Connections

An alert tells you *what* changed. It rarely tells you *why*. That answer usually lives somewhere else — the pull request that shipped minutes earlier, the incident already open in PagerDuty, the runbook sitting in Confluence.

**MCP Connections** let Netdata AI reach those systems directly. Through the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/), Netdata Cloud acts as an **MCP client** and connects to the tools your team already runs, reading from them while it investigates. It correlates the metrics and anomalies Netdata detects on every node with the context in your stack — so it can tie a latency spike to the deploy that caused it, link an anomaly to the incident already tracking it, and surface the relevant runbook without you going to look for it.

This is the reverse of connecting an AI client *to* Netdata. Here, **Netdata reaches out to your MCP servers**. To instead connect an AI assistant (Claude, Cursor, a CLI) to Netdata's own MCP server, see [Supported AI Clients](/docs/netdata-ai/mcp/mcp-clients/ai-devops-copilot.md).

![MCP Connections settings](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-settings.png)

## Prerequisites

- A **Netdata Cloud** account on a **Paid plan**.
- **Space admin** access — MCP Connections are configured per Space, under **Settings → AI → MCP Connections**.
- A reachable **MCP server** to connect to. Netdata ships built-in integrations for popular providers (GitHub, PagerDuty, Atlassian Cloud for Jira/Confluence/Bitbucket) and a **Custom MCP Server** option for any HTTPS MCP endpoint.

## Configure a new integration

1. Go to **Settings → AI → MCP Connections**.
2. Select an integration, such as **GitHub**, or choose **Custom MCP Server** to point at your own HTTPS MCP endpoint.
3. Choose an authentication method (see [Authentication methods](#authentication-methods) below). The available options depend on the integration.

   ![Choose an authentication method](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-auth.png)

4. Provide the required configuration parameters — such as the connection name or account region — then click **Connect & discover tools**.

   ![Connection parameters](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-connection.png)

   For **OAuth** integrations, you'll be redirected to the provider to authorize Netdata. Once you approve, you're sent back and the connection is established automatically.

   ![Authorize Netdata](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-authorize.png)

5. On success, Netdata retrieves the tools the remote MCP server exposes. Select the tools you want to make available for this connection.

   **Netdata only enables read-only tools.** The server may advertise tools that create, modify, or delete data (for example "Create an incident" or "Delete a team") — these appear in the discovered list but **cannot be enabled** from the Netdata UI. Netdata AI reads context; it does not act on your systems.

   ![Select read-only tools](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-tools.png)

6. Click **Save Changes** to complete the configuration. Enabled tools are not active until you save.

## Authentication methods

| Method | Who authenticates | Scope | When to use |
|--------|-------------------|-------|-------------|
| **Bearer token** | One shared token, entered once | All users in the Space share this single token | Providers that authenticate with an access token; simplest setup |
| **OAuth** | Each user authorizes individually with their own credentials | Per user | The standard method for most integrations; keeps per-user access boundaries intact |

For OAuth integrations, enabling a tool does **not** widen access: each user is still restricted by their own permissions on the underlying resource. A user only sees what they're already allowed to see in the connected system.

## Using MCP servers

Once a connection is saved, Netdata AI can use it during investigations. You control which servers are used, per conversation and per report.

### In a conversation

During a conversation you can see all MCP servers enabled for your Space and toggle them on or off for that conversation. The **Connected tools** row shows which servers Netdata AI will draw on as it answers.

![Toggle MCP servers in a conversation](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-conversation.png)

See [Conversations](/docs/netdata-ai/conversations.md) for more on live, interactive troubleshooting.

### In reports and investigations

When generating a report, you can select which MCP servers to include before the report runs — so a scheduled Insight or a Custom Investigation can pull in code changes, incidents, or on-call context alongside the metrics.

![Select MCP servers for a report](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-reports.png)

See [Investigations](/docs/netdata-ai/investigations/index.md) and [Scheduled Reports](/docs/netdata-ai/insights/scheduled-reports.md).

## Security and access

- **Read-only by design.** Only read-only tools can be enabled; mutating actions are never available through Netdata.
- **OAuth respects your permissions.** With OAuth, each user authenticates individually and is limited to what they can already access in the connected system.
- **Bearer tokens are Space-wide.** A bearer token is shared by everyone in the Space, so use it for providers where a shared, scoped access token is appropriate.
