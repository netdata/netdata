# Netdata Cloud Security and Privacy Design

Netdata Cloud provides identity, authorization, coordination, and a centralized interface for connected Agents and Parents. It does not replace the time-series database running on those nodes.

## Data Flow and Storage Boundary

Agents and Parents retain metrics in their local databases according to their storage configuration. Netdata Cloud stores the account, organization, node, chart, alert, and integration metadata needed to operate the service.

When a user requests a chart, log view, or Function result:

1. Cloud authorizes the user and identifies an eligible connected node.
2. The request travels over the Agent-Cloud Link.
3. The Agent or Parent queries its local data source or executes the Function.
4. The response returns through encrypted Cloud services to the user's client.

Metric, log, and Function responses therefore transit Cloud infrastructure. They are not ingested into Cloud-hosted persistent time-series storage merely because they are viewed through Cloud.

Configured integrations can create additional storage or delivery paths. For example, notification providers receive alert content selected for that integration. Review each configured destination separately.

## Identity and Authorization

Netdata Cloud authenticates users and authorizes actions within a Space. Authorization considers:

- Space membership.
- The user's assigned role and permissions.
- The access level required by the requested API or Function.
- Product entitlements that apply to the deployment.

A user who can view charts does not automatically have access to sensitive Functions or configuration. Grant the minimum permissions required and remove access when a user no longer needs it.

Current role definitions and plan entitlements can change. Consult the Cloud UI, product documentation, and [pricing page](https://www.netdata.cloud/pricing/) rather than relying on a copied feature matrix.

## Transport Security

Browser-to-Cloud and Agent-to-Cloud communication uses TLS. The Agent-Cloud Link is initiated by the Agent, so connecting a node to Cloud does not require exposing a new inbound Agent port to the internet.

Domain-based firewall rules are preferred because service addresses can change. See [Configure Netdata for cybersecurity platforms](/docs/netdata-agent/configure-netdata-for-cybersecurity-platforms.md) for the current endpoint and port requirements.

## Sensitive Operational Data

Cloud can relay data whose sensitivity is higher than ordinary metrics, including:

- Process details and command lines.
- Database statements and errors.
- System and application logs.
- Network connections and topology.
- Agent configuration.

The endpoint or Function declares the access required for that data. Users should still review output before sharing it because authorization does not redact secrets or personal information returned by the underlying system.

## Responsibility Boundaries

Netdata is responsible for the security of the Cloud service and its published controls. Deployment operators remain responsible for:

- Securing monitored hosts, Agents, Parents, and their credentials.
- Selecting appropriate Cloud roles and permissions.
- Configuring streaming and direct Agent access securely.
- Reviewing collectors, exporters, notifications, and external integrations.
- Defining retention and deletion requirements for their organization.
- Handling exported or copied Function and log results appropriately.

## Privacy, Retention, and Assurance

Privacy terms, data-subject rights, subprocessors, international transfers, and retention commitments are policy and contractual matters. Use the [Netdata Privacy Policy](https://www.netdata.cloud/privacy/) as the current source.

Current security attestations and control evidence are available through the [Netdata Trust Center](https://trust.netdata.cloud/). These sources supersede copied certification, vendor, or retention claims in product documentation.

For account and personal-data deletion procedures, follow the current Cloud UI and Privacy Policy. Do not assume a fixed retention period unless it appears in the applicable current policy or contract.

## Vulnerability Reporting

Report suspected vulnerabilities through the [Netdata GitHub Security Policy](https://github.com/netdata/netdata/security/policy).
