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

4. Provide the required configuration parameters — such as the connection name, or your account URL with the provider — then click **Connect & discover tools**.

   An **account URL** is the address you use to reach the provider, including your organization's subdomain. For PagerDuty that is `https://acme.pagerduty.com`, or `https://acme.eu.pagerduty.com` if your account is hosted in the EU. Netdata derives the correct regional endpoint from it, and refuses the connection if the account you authorize with is not the one configured here.

   ![Connection parameters](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-connection.png)

   For **OAuth** integrations, you'll be redirected to the provider to authorize Netdata. Once you approve, you're sent back and the connection is established automatically.

   For **GitHub with OAuth**, authorizing is not enough on its own — the **Netdata MCP** GitHub App also has to be installed on the organization that owns your repositories. See [GitHub: OAuth or a personal access token](#github-oauth-or-a-personal-access-token).

   ![Authorize Netdata](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-authorize.png)

5. On success, Netdata retrieves the tools the remote MCP server exposes. Select the tools you want to make available for this connection.

   **Netdata only enables read-only tools.** The server may advertise tools that create, modify, or delete data (for example "Create an incident" or "Delete a team") — these appear in the discovered list but **cannot be enabled** from the Netdata UI. Netdata AI reads context; it does not act on your systems.

   ![Select read-only tools](https://raw.githubusercontent.com/netdata/docs-images/refs/heads/master/netdata-cloud/netdata-ai/mcp-connections-tools.png)

6. Click **Save Changes** to complete the configuration. Enabled tools are not active until you save.

## GitHub: OAuth or a personal access token

GitHub offers both authentication methods — **OAuth 2.0** and **Personal Access Token** — and what limits Netdata AI is different in each.

### OAuth: install the Netdata MCP app

With OAuth, what Netdata AI can read is decided when the **Netdata MCP** GitHub App is installed, not when you connect. Authorizing the OAuth flow can succeed while the connection still returns nothing, because the app is not installed on the organization that owns the repositories.

1. An **organization owner** installs [Netdata MCP](https://github.com/apps/netdata-mcp) on the organization, from <https://github.com/apps/netdata-mcp/installations/new>.
2. During installation, choose which repositories the app can access — **All repositories**, or a specific list. **This selection bounds the non-public repositories** Netdata AI can read. Public repositories are readable either way: GitHub gives an app acting on behalf of a user implicit read access to public data.
3. If you are not an owner, GitHub turns your installation into a **request**. An owner has to approve it before the connection returns any data.
4. To reach repositories you own personally, install the app on your own GitHub account as well.
5. If your organization enforces **SAML single sign-on**, start an SSO session for it — `https://github.com/orgs/YOUR-ORG/sso` — before you install or authorize. Without an active session the authorization does not cover that organization.

Every permission the app asks for is **read-only**: repository metadata and contents, issues, pull requests, workflow runs and artifact metadata, deployments, discussions, commit statuses, security alerts, and Copilot agent task and variable metadata. GitHub shows the full list on the installation screen. The app cannot write to your repositories, and it subscribes to no webhook events.

Each user still connects individually through OAuth, so a user sees only the repositories they can already access, within what the installation allows. An organization owner uninstalling the app revokes access for everyone at once; a user revoking it under **GitHub → Settings → Applications** revokes only their own connection.

### Personal access token: the token is the boundary

The **Personal Access Token** method needs no app installation — Netdata authenticates to GitHub with the token you paste in, so the token's own permissions decide what is reachable. Three things to plan for:

- The token is **shared by the whole Space**. Everyone using Netdata AI reads GitHub as the token's owner, with that person's access. Prefer a fine-grained token limited to the repositories and read permissions you want exposed, rather than a classic token with broad scopes.
- If your organization **requires approval for fine-grained personal access tokens**, an owner has to approve the token before it can read the organization's non-public repositories.
- If your organization enforces **SAML single sign-on**, a **classic** token has to be authorized for that organization after you create it: click **Configure SSO** next to the token, then **Authorize**. The option only appears once you have signed in through your identity provider at least once. Fine-grained tokens are authorized when they are created.

Revoking is done on the token itself, in **GitHub → Settings → Developer settings → Personal access tokens** — which disconnects the integration for the entire Space at once.

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
