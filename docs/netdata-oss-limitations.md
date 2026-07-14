# Access Control and Feature Availability

Netdata uses several independent controls to decide whether a request is allowed. Authentication, the data sensitivity declared by an endpoint, network configuration, user permissions, and commercial entitlements are separate concerns.

This page explains the durable access model. For current plan limits and feature entitlements, see the [pricing page](https://www.netdata.cloud/pricing/).

## Access Layers

| Layer                         | What it controls                                                                              |
|:------------------------------|:----------------------------------------------------------------------------------------------|
| Network reachability and ACLs | Which clients can connect to an Agent or Parent endpoint.                                     |
| Authentication                | Which identity is making the request.                                                         |
| Endpoint permissions          | Whether that identity can access anonymous, sensitive, configuration, or administrative data. |
| Space membership and role     | What an authenticated Cloud user can do in a particular Space.                                |
| Product entitlement           | Whether a Cloud or commercial feature is enabled for the account or deployment.               |

Passing one layer does not bypass the others. For example, a plan may include a feature while the current user still lacks the permission required to use it.

## Metrics and Sensitive Data

Ordinary metrics and chart metadata use the anonymous-data access class in the Agent API. They may be reachable without a signed-in identity when the Agent is exposed directly and bearer protection is disabled.

More detailed operational data can be sensitive:

- Process command lines may contain credentials or tokens.
- Database queries and errors may contain literal or business data.
- Logs and event records may contain personal or security information.
- Network connections can reveal internal services and topology.
- Agent configuration can reveal infrastructure details and secrets.

Endpoints and Functions declare the access level they require. Sensitive operations can require a signed identity, same-Space membership, sensitive-data access, or a specific configuration permission.

## Direct Agent and Parent Access

A default direct deployment commonly relies on network isolation and Agent ACLs. Before exposing an Agent or Parent beyond a trusted network, configure an authentication boundary.

Supported controls include:

- [Bearer token protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md) integrated with Netdata Cloud identities.
- An authenticating reverse proxy.
- TLS for the Agent web server.
- Endpoint-specific and global network ACLs.

Bearer protection changes how otherwise anonymous-data APIs are handled. Review the linked configuration page before enabling it, especially for direct API and MCP clients.

## Netdata Cloud Access

A Cloud user must be authenticated, belong to the relevant Space, and have the permissions required by the requested endpoint. Sensitive Function output is retrieved from the selected Agent or Parent only after those checks succeed.

Roles and available permissions evolve independently of this architecture. Use the role descriptions shown in Netdata Cloud and the current product documentation instead of relying on a copied entitlement matrix.

## Functions

[Netdata Functions](/docs/top-monitoring-netdata-functions.md) declare their own access requirements. Some return ordinary system information; others expose processes, query text, logs, network connections, or configuration and therefore require stronger access.

The set available to a user depends on:

- Functions registered by the selected node.
- Node and streaming-path availability.
- Agent network and bearer-protection settings.
- The Function's declared access requirements.
- The current user's identity and permissions.

## Configuration and Management

Reading or changing configuration is distinct from reading metrics. Dynamic Configuration, notification settings, alert silencing, and Agent configuration use separate permissions. Do not infer configuration access from dashboard access.

Commercial availability for configuration interfaces can change. Check the [pricing page](https://www.netdata.cloud/pricing/) and the documentation for the specific configuration feature.

## MCP Access

Netdata supports MCP through Agents or Parents and through Netdata Cloud.

- Direct Agent and Parent MCP follows the local MCP API-key, bearer-protection, network ACL, and endpoint permission model.
- Cloud MCP follows Cloud identity, Space, permission, and entitlement checks.
- Tools that execute Functions inherit the access requirements of those Functions.

See the [Netdata MCP documentation](/docs/netdata-ai/mcp/README.md) for current setup and authentication behavior.

## Deployment Checklist

1. Decide which interfaces must be reachable and from which networks.
2. Enable TLS and an authentication boundary for untrusted networks.
3. Grant users only the roles and endpoint permissions they need.
4. Treat Functions, logs, queries, and configuration as potentially sensitive.
5. Re-check current plan entitlements before depending on a commercial feature.
6. Test access with both an intended user and an identity that should be denied.
