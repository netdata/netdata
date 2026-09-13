# Devin

Configure Cognition's Devin to access your Netdata infrastructure through MCP for AI-powered, autonomous DevOps and troubleshooting workflows.

Devin is a cloud-based autonomous AI software engineer. Because it runs in the cloud, it connects to Netdata as a remote MCP client — no local bridges or firewall changes are required when you use the Netdata Cloud MCP endpoint.

## Transport Support

Devin supports three MCP transport types for custom servers: STDIO, SSE, and HTTP (Streamable HTTP).

| Transport | Support | Netdata Version | Use Case |
|-----------|---------|-----------------|----------|
| **Streamable HTTP** | ✅ Fully Supported | v2.7.2+ (Agent/Parent); Cloud always | Direct connection to Netdata Cloud, or to an internet-reachable Agent/Parent (recommended) |
| **SSE** (Server-Sent Events) | ✅ Supported | v2.7.2+ | Legacy streaming transport for an internet-reachable Agent/Parent |
| **stdio** (via `npx mcp-remote`) | ✅ Supported | v2.7.2+ | Devin's STDIO transport bridged to Netdata's HTTP endpoint |
| **WebSocket** | ❌ Not Supported | - | Devin does not expose a WebSocket transport; use HTTP or the `mcp-remote` bridge |

> **Note:** HTTP (Streamable HTTP) is recommended for new integrations; SSE is supported for legacy servers. Devin does not support a WebSocket transport, so the Netdata `nd-mcp` stdio-to-WebSocket bridge is not usable from Devin — use the HTTP endpoint or the `npx mcp-remote` bridge instead.

## Prerequisites

1. **Devin account** with an organization admin role — only organization admins can add custom MCP servers. Sign up at [devin.ai](https://devin.ai).
2. **For the Netdata Cloud MCP path (recommended):**
   - A Netdata Cloud account with a **Paid plan**
   - Nodes claimed to Netdata Cloud
   - An API token with `scope:mcp` ([create one](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md))
3. **For a local Agent/Parent connection:**
   - Netdata **v2.7.2 or later** (HTTP/SSE transport)
   - The Netdata node must be reachable from the public internet, or from Devin's network over a VPN — Devin runs in the cloud and cannot reach a node on your private LAN without exposure
   - A local MCP API key, required when bearer protection is enabled — [Find your Netdata MCP API key](/docs/netdata-ai/mcp/README.md#finding-your-api-key)

> Prefer the [Netdata Cloud MCP](#netdata-cloud-mcp) path unless you specifically need a single local node. Exposing an Agent/Parent's port 19999 to the internet has security implications that the Cloud path avoids entirely.

## Configuration Methods

Custom MCP servers are configured through Devin's UI, not a local file. You add one under **Settings → Connections → MCP servers → Add a custom MCP**, choose a transport, and fill in the server details. After saving, click **Test listing tools** so Devin verifies it can discover the server's tools from its cloud environment.

Only the fields you need to fill in are listed below. For the full set of options, see the [Devin MCP Marketplace documentation](https://docs.devin.ai/work-with-devin/mcp).

### Netdata Cloud MCP

Connect Devin to your entire Netdata Cloud
infrastructure through a single endpoint —
no local setup, bridges, or firewall changes needed.

**Prerequisites:**

- Netdata Cloud account with a Paid plan
- Nodes claimed to Netdata Cloud
- API token with `scope:mcp`
  ([create one](/docs/netdata-cloud/authentication-and-authorization/api-tokens.md))

1. In Devin, go to **Settings → Connections → MCP servers** and click **Add a custom MCP**.
2. Fill in the server details:
   - **Server Name:** `netdata-cloud`
   - **Short Description:** e.g. `Netdata infrastructure observability`
3. Select **HTTP** as the transport type.
4. Set **Server URL** to:
   ```
   https://app.netdata.cloud/api/v1/mcp
   ```
5. Set **Authentication method** to **Auth Header**:
   - **Header key:** `Authorization` (the default)
   - **Header value:** `Bearer YOUR_NETDATA_CLOUD_API_TOKEN`
6. **Save**, then click **Test listing tools** to verify the connection.

Replace `YOUR_NETDATA_CLOUD_API_TOKEN` with your Netdata Cloud API token (must have `scope:mcp`). For more details, see [Netdata Cloud MCP](/docs/netdata-ai/mcp/README.md#netdata-cloud-mcp).

> Store the token as a [Devin Secret](https://docs.devin.ai/work-with-devin/mcp) and reference it from the header value rather than pasting the raw token, so the credential stays out of plaintext configuration.

### Local Agent or Parent

The following methods connect Devin directly to a Netdata Agent or Parent on your network. Because Devin is a cloud agent, the Netdata node **must be reachable from the internet** (or from Devin's network over a VPN). If the node is not exposed, use the [Netdata Cloud MCP](#netdata-cloud-mcp) path instead.

#### Method 1: Direct HTTP Connection (Recommended for v2.7.2+)

Connect directly to Netdata's HTTP endpoint without any bridge.

1. In Devin, go to **Settings → Connections → MCP servers** and click **Add a custom MCP**.
2. Fill in the server details:
   - **Server Name:** `netdata`
   - **Short Description:** e.g. `Local Netdata Agent`
3. Select **HTTP** as the transport type.
4. Set **Server URL** to your exposed Netdata endpoint:
   ```
   http://YOUR_NETDATA_IP:19999/mcp
   ```
   For an HTTPS endpoint, use `https://YOUR_NETDATA_IP:19999/mcp`.
5. Set **Authentication method** to **Auth Header**:
   - **Header key:** `Authorization`
   - **Header value:** `Bearer NETDATA_MCP_API_KEY`
6. **Save**, then click **Test listing tools** to verify the connection.

For an SSE-only setup, choose **SSE** as the transport and use the same Server URL and Auth Header. HTTP is recommended over SSE for new integrations.

#### Method 2: Using `npx mcp-remote` (stdio bridge for v2.7.2+)

If you prefer Devin's STDIO transport, use the official MCP remote client to bridge from stdio to Netdata's HTTP endpoint (requires Netdata v2.7.2+). For detailed `mcp-remote` options and troubleshooting, see [Using MCP Remote Client](/docs/netdata-ai/mcp/README.md#using-mcp-remote-client).

1. In Devin, go to **Settings → Connections → MCP servers** and click **Add a custom MCP**.
2. Select **STDIO** as the transport type.
3. Set the command and arguments to launch `mcp-remote` against your Netdata HTTP endpoint, passing the bearer token:
   - **Command:** `npx`
   - **Arguments:** `mcp-remote@latest --http http://YOUR_NETDATA_IP:19999/mcp --allow-http --header "Authorization: Bearer NETDATA_MCP_API_KEY"`
   - **Environment Variables:** (optional) store the key in a [Devin Secret](https://docs.devin.ai/work-with-devin/mcp) and reference it instead of inlining the token
4. **Save**, then click **Test listing tools** to verify the connection.

> The `--allow-http` flag is required for non-HTTPS endpoints. Only use it on trusted networks, since traffic will not be encrypted. The Netdata `nd-mcp` stdio-to-WebSocket bridge is not usable from Devin because Devin has no WebSocket transport and `nd-mcp` is not an npm package available in Devin's environment.

Replace in all examples:

- `YOUR_NETDATA_IP` - IP address or hostname of your exposed Netdata Agent/Parent
- `YOUR_NETDATA_CLOUD_API_TOKEN` - Your Netdata Cloud API token (must have `scope:mcp`)
- `NETDATA_MCP_API_KEY` - Your local [Netdata MCP API key](/docs/netdata-ai/mcp/README.md#finding-your-api-key)

## How to Use

Once the server passes **Test listing tools**, Devin can use Netdata's observability data during a session. Ask infrastructure questions naturally:

```
What's the current CPU usage across all servers?
Show me any performance anomalies in the last hour.
Which services are consuming the most memory on the database node?
Explain the current active Netdata alerts and their potential impact.
```

**Performance Investigation:**
```
Investigate why application response times increased this afternoon using Netdata metrics.
```

**Resource Optimization:**
```
Check memory usage patterns across all nodes and suggest optimization strategies.
```

> **💡 Advanced Usage:** Devin combines observability data with autonomous action for powerful DevOps workflows. Learn about the opportunities and security considerations in [AI DevOps Copilot](/docs/netdata-ai/mcp/mcp-clients/ai-devops-copilot.md).

## Troubleshooting

### "Test listing tools" fails

Devin runs the connection test from its cloud environment, so the failure reasons map directly to network reachability and credentials:

- **"Verify server URL and network connectivity"** — For a local Agent/Parent, the node is not reachable from the internet. Expose port 19999 securely, connect over a VPN to Devin's network, or switch to the [Netdata Cloud MCP](#netdata-cloud-mcp) path. For the Cloud endpoint, confirm `https://app.netdata.cloud/api/v1/mcp` resolves and is not blocked by a corporate egress filter.
- **"Check authentication credentials and permissions"** — For the Cloud path, verify your API token has `scope:mcp`. For a local Agent/Parent, verify the MCP API key is correct and that bearer protection is satisfied.
- **"Server took too long to respond"** — Ensure the Netdata node is running and responsive, and that no firewall is dropping the connection.

### No nodes visible (Cloud path)

- Confirm your nodes are claimed to Netdata Cloud and appear in the web dashboard.
- Check that agents are online and streaming to Cloud.
- Verify the token was created with `scope:mcp` and that your space is on a Paid plan.

### Local node unreachable

Devin cannot reach a Netdata node on your private LAN. Either expose the node to the internet (with appropriate ACLs — see `allow mcp from` in `netdata.conf`), connect over a VPN to Devin's network, or use the [Netdata Cloud MCP](#netdata-cloud-mcp) path, which needs no inbound exposure.

### Limited data access

- Verify the API key is included in the `Authorization: Bearer` header.
- For a local Agent/Parent with bearer protection enabled, anonymous access is rejected on all MCP transports — the MCP API key is mandatory.
- When bearer protection is disabled, the key is still required to unlock sensitive operations (logs, protected functions).

## Documentation Links

- [Devin](https://devin.ai)
- [Devin MCP Marketplace documentation](https://docs.devin.ai/work-with-devin/mcp)
- [Devin MCP server documentation](https://docs.devin.ai/work-with-devin/devin-mcp)
- [Netdata MCP Setup](/docs/netdata-ai/mcp/README.md)
- [AI DevOps Best Practices](/docs/netdata-ai/mcp/mcp-clients/ai-devops-copilot.md)
