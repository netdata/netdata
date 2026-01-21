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

## ACLK State API

The `/api/v1/aclk` endpoint provides detailed diagnostics for the Agent-Cloud Link connection.

**Endpoint:** `GET /api/v1/aclk`

**Returns:** Complete ACLK state including connection status, protocol versions, and per-node information.

### Response Schema

```json
{
  "aclk-available": true,
  "aclk-version": 2,
  "protocols-supported": ["Protobuf"],
  "agent-claimed": true,
  "claimed-id": "uuid-string",
  "cloud-url": "https://app.netdata.cloud",
  "aclk_proxy": null,
  "publish_latency_us": 12345,
  "online": true,
  "used-cloud-protocol": "Protobuf",
  "mqtt-version": 5,
  "received-app-layer-msgs": 100,
  "received-mqtt-pubacks": 50,
  "pending-mqtt-pubacks": 0,
  "reconnect-count": 2,
  "last-connect-time-utc": "2024-01-01T00:00:00Z",
  "last-connect-time-puback-utc": "2024-01-01T00:00:01Z",
  "last-disconnect-time-utc": "2024-01-01T00:00:00Z",
  "next-connection-attempt-utc": "2024-01-01T00:00:10Z",
  "last-backoff-value": 10.5,
  "banned-by-cloud": false,
  "node-instances": [
    {
      "hostname": "my-host",
      "mguid": "a1b2c3d4-...",
      "claimed_id": "uuid-string",
      "node-id": "uuid-string",
      "streaming-hops": 1,
      "relationship": "self",
      "streaming-online": true,
      "alert-sync-status": {}
    }
  ]
}
```

### Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `aclk-available` | boolean | Whether ACLK is compiled into this binary |
| `aclk-version` | integer | ACLK protocol version (2) |
| `protocols-supported` | array | Supported wire protocols (["Protobuf"]) |
| `agent-claimed` | boolean | Whether Agent holds valid Cloud credentials |
| `claimed-id` | string | Claimed ID UUID (only if claimed) |
| `cloud-url` | string | Target Cloud URL |
| `aclk_proxy` | string | Configured proxy URL (null if none) |
| `online` | boolean | Current connection status |
| `used-cloud-protocol` | string | Wire protocol in use ("Protobuf") |
| `mqtt-version` | integer | MQTT protocol version (5) |
| `reconnect-count` | integer | Number of successful reconnects |
| `pending-mqtt-pubacks` | integer | Messages awaiting acknowledgment |
| `banned-by-cloud` | boolean | Whether Cloud has disabled this Agent |
| `node-instances` | array | Per-node status for Parents and self |

### Interpreting Connection Issues

:::note

**Not Claimed (`agent-claimed: false`)**

The Agent lacks valid claiming credentials. If you expect this Agent to be claimed:

1. Check `/var/lib/netdata/cloud.d/` exists
2. Verify `private.pem` and `cloud.conf` are present
3. Attempt re-claiming via [Claiming Guide](/src/claim/README.md)

:::

:::note

**Offline (`online: false`)**

Possible causes:

- Network firewall blocking port 443 to Cloud domains
- Proxy misconfiguration
- Cloud-side ban (check `banned-by-cloud`)
- Temporary Cloud outage

Diagnose with `journalctl -u netdata | grep ACLK`

:::

:::note

**Banned by Cloud (`banned-by-cloud: true`)**

Contact Netdata Support. This indicates Cloud has revoked the Agent's access credentials.

:::
