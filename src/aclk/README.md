# Agent-Cloud link (ACLK)

The Agent-Cloud Link (ACLK) provides secure communication between your Netdata Agents and Cloud. This connection:

- Uses outgoing secure WebSocket (WSS) on port `443`
- Activates only after you [connect a node](/src/claim/README.md) to your Space
- Ensures encrypted, safe data transmission

For ACLK to function properly, your Agents need outbound access to Netdata Cloud services.

| Allowlisting Method | Required Access                                                            |
|---------------------|----------------------------------------------------------------------------|
| Domain              | • `app.netdata.cloud`<br/>• `api.netdata.cloud`<br/>• `mqtt.netdata.cloud` |

> **Important**
>
> IP addresses can change without notice! Always **prefer domain allowlisting**. If you must use IP addresses, be aware that they vary based on your geographic location due to CDN-edge servers. You'll need to regularly verify the IP addresses specific to your region.

### Allowlisting for Netdata Cloud On-Prem

The domains and ports above (`app.netdata.cloud`, `api.netdata.cloud`, `mqtt.netdata.cloud`) apply only to the Netdata SaaS Cloud Service. When you connect Agents to a self-hosted [Netdata Cloud On-Prem](/docs/netdata-cloud/versions.md) deployment, the required domains and ports are those of **your own On-Prem Cloud installation** — its ingress/load balancer and the MQTT-over-WebSocket (WSS, port `443`) service it exposes — not the SaaS domains.

Because the Agent [claims](/src/claim/README.md#connecting-to-netdata-cloud-on-prem-self-hosted) against your On-Prem Cloud and discovers its connection endpoints from it, you only need to allowlist the endpoints your own installation serves. See the [On-Prem deployment documentation](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/README.md) for the exact domains and ports your installation uses.

## Data privacy

Your monitoring data belongs to you. Here's how we ensure this:

- **Zero Metric Storage**: We do not store any metrics or logs in Netdata Cloud.
- **Local Data Control**: All your monitoring data stays within your infrastructure.
- **Minimal Metadata**: We store only essential metadata needed for coordination and access control.

For complete transparency:

- Read our detailed [Privacy Policy](https://netdata.cloud/privacy/)
- Learn more about [stored metadata](/docs/netdata-cloud/README.md#stored-metadata)

## Enable and configure the ACLK

The Agent-Cloud Link is enabled automatically—no configuration needed.
If your Agent requires a proxy to access the internet, you'll need to [configure proxy settings](/src/claim/README.md#proxy-configuration).
