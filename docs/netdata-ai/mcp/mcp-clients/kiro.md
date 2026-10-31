# Kiro

[Kiro](https://kiro.dev) is an AI coding assistant with spec-driven development, available across the IDE, CLI, Web, and more. It brings AI agents into your workflow to plan, write, and refactor code. Kiro supports the Model Context Protocol (MCP), so it can connect to external servers like Netdata for real infrastructure context.

Configure Kiro to access your Netdata infrastructure through MCP. For the full configuration reference, see the [Kiro MCP documentation](https://kiro.dev/docs/mcp/) and [MCP configuration guide](https://kiro.dev/docs/mcp/configuration/).

## How Kiro configures MCP servers

Kiro reads MCP definitions from a `mcpServers` object in a JSON file. A server is **local** when it has a `command` (and optional `args`), and **remote** when it has a `url` (and optional `headers`). Kiro has no separate transport `type` field: the presence of `command` versus `url` is what selects the transport. See [configuration file structure](https://kiro.dev/docs/mcp/configuration/#configuration-file-structure).

| Connection | Netdata version | How to configure |
|------------|-----------------|------------------|
| **Local (`command`)** | v2.6.0+ | Launch an Agent or Parent through the `nd-mcp` bridge |
| **Remote (`url`)** | v2.7.2+ | Point Kiro at an Agent or Parent HTTP endpoint - Kiro requires HTTPS unless the endpoint is on `localhost` |
| **Remote (`url`)** | Any | Point Kiro at Netdata Cloud - no Agent version requirement |

## Prerequisites

1. **Kiro installed** - see [Installation](https://kiro.dev/docs/getting-started/installation/).
2. **MCP enabled** - in the Kiro IDE, enable MCP support in Settings (see [enabling MCP support](https://kiro.dev/docs/mcp/configuration/#enabling-mcp-support)).
3. **Something to connect to** - either a self-hosted Agent or Parent, or Netdata Cloud:
   - **Self-hosted Agent or Parent**: **Netdata v2.6.0 or later** with MCP support. Prefer a Netdata Parent for infrastructure-level visibility. The machine running Kiro needs network access to the Netdata IP and port (usually 19999). On v2.6.0 - v2.7.1, connect through the `nd-mcp` bridge (local server). From v2.7.2 you can also connect to the HTTP endpoint directly (remote server) when that endpoint is on `localhost` or behind HTTPS, or keep using `nd-mcp` for everything else.
   - **Netdata Cloud**: no Agent version requirement. See the Netdata Cloud prerequisites below.
4. **Netdata MCP API key**. Each Netdata Agent or Parent has its own key - [find your Netdata MCP API key](/docs/netdata-ai/mcp/README.md#finding-your-api-key). Keep the key in an environment variable rather than hardcoding it, and reference it as `${VAR}` from the `env` or `headers` object - those are the fields where Kiro documents expansion (see [environment variables](https://kiro.dev/docs/mcp/configuration/#environment-variables)). Kiro does not document `${VAR}` expansion in `args` or `url`, so never place a `${VAR}` reference in an argument list. Kiro only expands variables you have approved: when you add a config with an unapproved `${VAR}`, Kiro shows a security warning listing the variables - approve them in Settings under "Mcp Approved Env Vars".

## Configuration file locations

Kiro reads MCP config from two levels ([configuration locations](https://kiro.dev/docs/mcp/configuration/#configuration-locations)):

- **Workspace (this project only)**: `.kiro/settings/mcp.json` in the workspace root.
- **User (all workspaces)**: `~/.kiro/settings/mcp.json`.

Open either from the command palette: **Kiro: Open workspace MCP config (JSON)** or **Kiro: Open user MCP config (JSON)**. Changes apply automatically on save - Kiro [hot-reloads](https://kiro.dev/docs/mcp/configuration/#hot-reload) only the servers that changed, without restarting your session.

## Configuration methods

### Netdata Cloud (remote)

Connect to your entire Netdata Cloud infrastructure through a single endpoint - no local bridge or firewall changes needed.

**Prerequisites:**

- Netdata Cloud account with a Paid plan
- Nodes claimed to Netdata Cloud
- API token with `scope:mcp` ([create one](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md))

```json
{
  "mcpServers": {
    "netdata-cloud": {
      "url": "https://app.netdata.cloud/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer ${NETDATA_CLOUD_API_TOKEN}"
      }
    }
  }
}
```

Export `NETDATA_CLOUD_API_TOKEN` with a token that has `scope:mcp`, then approve the variable under "Mcp Approved Env Vars". For more details, see [Netdata Cloud MCP](/docs/netdata-ai/mcp/README.md#netdata-cloud-mcp).

### Local Agent or Parent - stdio bridge (all Netdata versions)

Launch a Netdata Agent or Parent on your network through the `nd-mcp` bridge. This is a **local** server (`command`), so it works on every Netdata version with MCP support, and it is also how you reach an Agent that Kiro's remote `url` scheme rules exclude (see the remote endpoint section below).

The bridge reads the MCP API key from the `ND_MCP_BEARER_TOKEN` environment variable, so pass it through Kiro's `env` object, where `${VAR}` expansion is supported:

```json
{
  "mcpServers": {
    "netdata": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://YOUR_NETDATA_IP:19999/mcp"],
      "env": {
        "ND_MCP_BEARER_TOKEN": "${NETDATA_MCP_API_KEY}"
      }
    }
  }
}
```

Export `NETDATA_MCP_API_KEY` in the environment Kiro starts from, then approve it in Settings under "Mcp Approved Env Vars". All three bridge implementations (Go, Node.js, Python) accept `ND_MCP_BEARER_TOKEN`.

The bridge also accepts the key as a `--bearer TOKEN` argument, but Kiro does not expand `${VAR}` inside `args`, so use `--bearer` only with a literal key.

`nd-mcp` sends the key as an `Authorization` header over the WebSocket connection, and `ws://` is unencrypted - the key and everything you query travel in cleartext. Use `ws://` only on loopback or a network you explicitly trust. To cross an untrusted network, put Netdata behind a TLS-terminating reverse proxy and point the bridge at the proxy with a `wss://` URL instead.

Replace `YOUR_NETDATA_IP` with your Netdata Agent or Parent address. On Linux the bridge is usually at `/usr/sbin/nd-mcp`; see [finding the nd-mcp bridge](/docs/netdata-ai/mcp/README.md#finding-the-nd-mcp-bridge) for other paths. You can read the key with:

```bash
sudo cat /var/lib/netdata/mcp_dev_preview_api_key
```

### Local Agent or Parent - remote endpoint (Netdata v2.7.2+)

From v2.7.2, a Netdata Agent or Parent exposes an HTTP streamable MCP endpoint you can connect to directly as a **remote** server (`url`), no bridge required.

Kiro's `url` field accepts an HTTPS endpoint, or a plain HTTP endpoint only when it is on `localhost` ([configuration properties](https://kiro.dev/docs/mcp/configuration/#configuration-properties)). A Netdata Agent serves plain HTTP, so this method fits two cases:

- **Netdata on the same machine as Kiro** - use `http://localhost:19999/mcp`.
- **Netdata behind a TLS-terminating reverse proxy** - use the proxy's `https://` URL.

For a remote Agent or Parent reachable only over plain HTTP across your LAN, use the stdio bridge above instead - `nd-mcp` does not enforce this scheme restriction, but the connection is then unencrypted, so treat it as trusted-network only.

```json
{
  "mcpServers": {
    "netdata": {
      "url": "http://localhost:19999/mcp",
      "headers": {
        "Authorization": "Bearer ${NETDATA_MCP_API_KEY}"
      }
    }
  }
}
```

Save the file and Kiro connects the new server automatically.

## Using Netdata in Kiro

Once connected, reference your infrastructure directly in chat:

```
what's the current CPU usage on my infrastructure?
show me database query performance
are there any anomalies in the web servers?
```

Or pull infrastructure context in while coding:

```python
# what's the typical memory usage of this service?
def process_large_dataset():
    ...
```

## Multiple environments

Define one server per environment and disable the ones you are not using with `disabled` ([disabling servers](https://kiro.dev/docs/mcp/configuration/#disabling-servers-and-tools)):

```json
{
  "mcpServers": {
    "netdata-prod": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://prod-parent:19999/mcp"],
      "env": {
        "ND_MCP_BEARER_TOKEN": "${PROD_MCP_API_KEY}"
      }
    },
    "netdata-dev": {
      "command": "/usr/sbin/nd-mcp",
      "args": ["ws://dev-parent:19999/mcp"],
      "env": {
        "ND_MCP_BEARER_TOKEN": "${DEV_MCP_API_KEY}"
      },
      "disabled": true
    }
  }
}
```

## Best practices

- Keep API keys in environment variables and reference them as `${VAR}` from `env` or `headers`; never commit config files with credentials. See [MCP security best practices](https://kiro.dev/docs/mcp/security/).
- Encrypt the connection whenever it leaves the machine or a trusted network. Plain `ws://` and `http://` send the API key and your query results in cleartext, so terminate TLS in front of Netdata and use `wss://` (bridge) or `https://` (remote `url`).
- Prefer connecting to a Netdata Parent so a single server covers your whole infrastructure.
- Name servers clearly (`netdata-prod`, `netdata-dev`) and disable the ones you are not querying, so Kiro does not send a question to the wrong environment.

## Troubleshooting

Start with Kiro's own MCP troubleshooting guidance: [checking MCP logs](https://kiro.dev/docs/mcp/#checking-mcp-logs) (Kiro panel, Output tab, "Kiro - MCP Logs") for the specific error, the [common issues table](https://kiro.dev/docs/mcp/#common-issues-and-solutions), and [troubleshooting configuration](https://kiro.dev/docs/mcp/configuration/#troubleshooting-configuration). The Netdata-specific checks below cover the rest.

**Server not connecting**
- Save the config file (Kiro reconnects on save) and check the server status in the Kiro MCP panel; validate the JSON syntax.
- If the key is an unapproved environment variable, approve it in Settings under "Mcp Approved Env Vars" (see [environment variables](https://kiro.dev/docs/mcp/configuration/#environment-variables)). Check that the `${VAR}` reference sits in `env` or `headers` - Kiro does not expand it inside `args`, so the bridge would receive the literal text as its token and authentication would fail.
- For a remote (`url`) server, confirm the scheme: Kiro allows plain HTTP only on `localhost`. Use the stdio bridge for a plain-HTTP Netdata elsewhere on your network.
- Test that Netdata is reachable: `curl http://YOUR_NETDATA_IP:19999/api/v3/info`, and confirm the firewall allows the Netdata port. For the local bridge, verify the `nd-mcp` path is correct and executable.

**Limited results**
- Ensure the API key is present, and that the Agent is claimed with the required collectors enabled.
