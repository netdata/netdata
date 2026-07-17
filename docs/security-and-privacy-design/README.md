# Security and Privacy Design

Netdata separates data collection and storage from the services used to access and visualize that data. Understanding that boundary is the starting point for securing a deployment.

## Architecture at a Glance

- **Netdata Agents** collect metrics and operational data from monitored systems.
- **Netdata Parents** can receive and store metrics streamed by Child Agents.
- **Netdata Cloud** coordinates access and relays authorized queries to connected Agents and Parents.
- **Browsers and API clients** receive the results requested by an authorized user.
- **Exporters and external notification systems** receive data only when an operator configures those paths.

By default, time-series metric storage remains on Agents and Parents. When a Cloud user requests charts, logs, or a Function, the relevant Agent or Parent returns the requested data through an encrypted connection. These responses transit Cloud services but are not turned into Cloud-hosted time-series storage.

Configured streaming, exporting, notifications, collectors, and integrations can copy or send data to other systems. Review those paths as part of your own deployment design.

## Security Principles

### Minimize Privilege

The Agent normally runs as an unprivileged service account. Components that need additional operating-system access are isolated and should receive only the permissions required for their collection task.

### Protect Sensitive Operational Data

Metrics are often less sensitive than logs, process command lines, database queries, network connections, and configuration. Functions and log sources can expose this higher-risk data, so their access requirements are stricter than ordinary chart access.

### Authenticate at the Appropriate Boundary

Direct Agent access, Agent-to-Agent streaming, and Cloud access use different security controls:

- Agent web APIs support network ACLs, TLS, reverse-proxy authentication, and [bearer token protection](/docs/netdata-agent/configuration/secure-your-netdata-agent-with-bearer-token.md).
- Streaming connections use an API key and can use TLS.
- Netdata Cloud authenticates users and applies Space roles and permissions to connected infrastructure.

### Encrypt Network Traffic

Cloud connections use TLS. Operators are responsible for enabling and validating TLS where Agents stream to one another or expose their web APIs over untrusted networks.

### Keep Security Information Current

Product architecture belongs in these documentation pages. Current attestations and security-control evidence belong in the [Netdata Trust Center](https://trust.netdata.cloud/). Legal and privacy terms belong in the [Netdata Privacy Policy](https://www.netdata.cloud/privacy/), and current commercial entitlements belong on the [pricing page](https://www.netdata.cloud/pricing/).

## Documentation by Boundary

- [Access Control and Feature Availability](/docs/netdata-oss-limitations.md): how public data, sensitive data, authentication, and product entitlements interact.
- [Netdata Agent Security and Privacy Design](/docs/security-and-privacy-design/netdata-agent-security.md): local storage, collection privileges, communication paths, and Agent access controls.
- [Netdata Cloud Security and Privacy Design](/docs/security-and-privacy-design/netdata-cloud-security.md): Cloud data flow, identity, authorization, and responsibility boundaries.
- [Secure your Netdata Agents](/docs/netdata-agent/securing-netdata-agents.md): operational hardening guidance.
- [GitHub Security Policy](https://github.com/netdata/netdata/security/policy): report a vulnerability privately and review the supported-version policy.
