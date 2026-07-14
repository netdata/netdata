# Netdata Agent Security and Privacy Design

The Netdata Agent collects, stores, and serves observability data close to the systems it monitors. Operators control where that data is stored, streamed, exported, and exposed.

## Data Collection and Storage

Collectors read operating-system and application sources and submit metrics to the Netdata daemon. Most collectors run without elevated privileges; collectors that need additional access should receive only the capabilities or helper permissions required for their task.

The Agent database stores time-series metrics and related metadata locally. When streaming is configured, a Child can also send metrics to one or more Parents. When exporting is configured, the Agent can send selected metrics to an external backend.

Collectors and plugins can also provide on-demand Functions. Function results may include logs, processes, database queries, network connections, or other non-metric data. These results are produced for a request and are not equivalent to the time-series data stored in the Agent database.

## Data Paths

| Path                        | Data behavior                                                                      | Protection                                                 |
|:----------------------------|:-----------------------------------------------------------------------------------|:-----------------------------------------------------------|
| Collector to daemon         | Metrics and metadata pass through local plugin protocols or in-process interfaces. | Local process permissions and plugin isolation.            |
| Agent database              | Metrics and metadata are retained according to local configuration.                | Host filesystem and operating-system controls.             |
| Child to Parent streaming   | Configured metrics and metadata are copied to the Parent.                          | API-key authentication; TLS when configured.               |
| Direct web API              | The Agent returns requested metrics, metadata, or Function results.                | Network ACLs, optional TLS, and configured authentication. |
| Agent-Cloud Link            | Metadata and authorized query or Function responses transit to Cloud users.        | Agent-initiated TLS connection and Cloud authorization.    |
| Exporting and notifications | Configured data is sent to operator-selected destinations.                         | Destination-specific configuration and transport security. |

The statement that data “stays local” is true only when streaming, exporting, Cloud access, notifications, and other outbound integrations are not configured. Review the complete deployment rather than treating the Agent database location as the only data boundary.

## Privilege Boundaries

The main Agent and most collectors run as the Netdata service account. Platform-specific helpers or collectors may need capabilities, group membership, device access, or elevated helper binaries.

- Grant only the permissions documented for the collector.
- Protect Netdata configuration and credential files with filesystem permissions.
- Review Function output separately from chart data.
- Keep the Agent and its dependencies updated.

## Network Access and Authentication

Direct Agent access can be intentionally open inside a trusted network. It should not be exposed to an untrusted network without additional controls.

Available controls include:

- Global and endpoint-specific network ACLs.
- TLS for the web server.
- An authenticating reverse proxy.
- [Bearer token protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md) tied to Netdata Cloud identities and permissions.

Streaming receivers authenticate senders with an API key. Enable TLS when streaming crosses an untrusted network, and protect streaming API keys as secrets.

See [Secure your Netdata Agents](/docs/netdata-agent/securing-netdata-agents.md) for operational configuration.

<a id="outbound-network-communication"></a>
## Netdata-Managed Outbound Connections

A stock installation can use these Netdata-managed outbound paths:

| Path                     | When it is used                                                                   | Operator control                                 |
|:-------------------------|:----------------------------------------------------------------------------------|:-------------------------------------------------|
| Usage telemetry          | Agent lifecycle events and dashboard usage unless opted out.                      | Follow the documented telemetry opt-out methods. |
| Agent-Cloud Link         | After the node is connected to a Netdata Cloud Space.                             | Disconnect or do not connect the node.           |
| Installation and updates | During installation and when the separate updater checks configured repositories. | Use offline installation and update controls.    |

[Usage telemetry](/docs/netdata-agent/configuration/anonymous-telemetry-events.md) masks selected identifying fields, but uses a stable installation or machine identifier for event association. Treat it as pseudonymous usage telemetry, not as proof that no identifying data is processed. The canonical telemetry page lists the fields and opt-out methods.

The Agent-Cloud Link is outbound and uses TLS. It can carry node metadata, alerts, and responses to authorized metric, log, or Function requests. Those responses may contain operational data even though Netdata Cloud does not act as the Agent's persistent time-series database.

Installer and updater traffic is separate from the running daemon. For disconnected deployments, see [Install Netdata on offline systems](/packaging/installer/methods/offline.md).

User-configured collectors, exporters, streaming destinations, notification methods, service discovery, and proxies can add other network connections. They are not covered by the stock-path table.

## Air-Gapped Operation

For a deployment with no Netdata-managed internet traffic:

1. Do not connect the Agent to Netdata Cloud.
2. Disable usage telemetry.
3. Disable online updates and use the offline installation workflow.
4. Audit configured collectors, streaming, exporters, notifications, and service discovery for external destinations.
5. Restrict direct web access to approved networks.

The Agent can continue collecting, storing, and serving data locally under these conditions.

## Vulnerability Reporting

Report suspected vulnerabilities through the [Netdata GitHub Security Policy](https://github.com/netdata/netdata/security/policy). Do not disclose a suspected vulnerability in a public issue before following that process.
