# Netdata Cloud Outbound IP Addresses

Some Netdata Cloud features require Netdata Cloud to open a connection **to your infrastructure** — for example delivering
alert notifications to a webhook you host, or reading context from an MCP server you run.

If those endpoints sit behind a firewall, a WAF, or an IP allowlist, you need to know which source addresses Netdata Cloud
connects from. Netdata publishes that list at a public endpoint so you can allowlist it and refresh it automatically.

## The endpoint

```bash
curl https://app.netdata.cloud/ips-v4
```

- **Authentication:** none required.
- **Response:** `text/plain`, one IPv4 CIDR block per line, newline-separated.
- **Scope:** IPv4 only. There is no IPv6 equivalent today, so an allowlist built from this endpoint must not assume
  Netdata Cloud will reach you over IPv6.

Example response:

```text
3.224.66.12/32
52.21.70.201/32
35.169.216.169/32
```

:::important

The values above are an illustration of the response format, not a reference list. **Always read the live endpoint.**
Anything you copy from this page will eventually be wrong.

:::

## Which traffic these addresses cover

These are the source addresses for connections Netdata Cloud initiates **outbound to your systems**, including:

- [Webhook notifications](/integrations/cloud-notifications/integrations/webhook.md) — alert and reachability payloads
  POSTed to your webhook URL.
- [MCP Connections](/docs/netdata-ai/mcp/mcp-connections.md) — requests to a Custom MCP Server or a self-hosted MCP
  endpoint.

This is the **opposite direction** from your Agents connecting to Netdata Cloud. For the outbound access your Agents need
(ACLK, telemetry, updates), see
[Required endpoints and ports](/docs/netdata-agent/configure-netdata-for-cybersecurity-platforms.md#required-endpoints-and-ports).

## Using the list in a firewall rule

Fetch the list and rebuild your rule rather than pasting addresses into a config once:

```bash
curl -fsS https://app.netdata.cloud/ips-v4
```

Guidance:

- **Refresh on a schedule.** Re-fetch periodically and reconcile your allowlist, so a change to the published list does
  not silently break notification delivery.
- **Fail closed on a bad fetch.** If the request fails or returns an empty body, keep the previous list. Do not replace a
  working allowlist with an empty one.
- **Parse defensively.** Skip blank lines and trim whitespace. Treat each entry as a CIDR block, not a bare address.
- **Combine with authentication.** An IP allowlist is a network control, not an identity control. Keep the
  [webhook authentication mechanism](/integrations/cloud-notifications/integrations/webhook.md#authentication-mechanisms)
  — mutual TLS, Basic, or Bearer — in place as well.
