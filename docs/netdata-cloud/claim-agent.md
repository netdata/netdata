# netdata-claim.sh Reference

The `netdata-claim.sh` script connects a Netdata Agent to Netdata Cloud via the [Agent-Cloud Link (ACLK)](docs/netdata-cloud/README.md). This page provides a complete reference for the script's CLI options, environment variables, and troubleshooting.

> **Note:** The `netdata-claim.sh` script is **deprecated** and will be officially unsupported in the future. For new installations, use the kickstart script with `--claim-*` options instead. For existing installations, write the claiming configuration directly to `claim.conf`.

## Prerequisites

Before claiming an agent, ensure you have:

1. **A valid claiming token** - Generated from Netdata Cloud (Space Settings > Nodes > Add Node)
2. **Netdata Agent installed** - The agent must be running
3. **Root access** - The script requires root privileges or write access to the configuration directory

## Quick Start

Claim your agent using the kickstart script (recommended):

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --claim-token YOUR_TOKEN --claim-rooms ROOM1,ROOM2
```

Or claim an existing installation directly:

```bash
sudo netdata-claim.sh --claim-token YOUR_TOKEN --claim-rooms ROOM1,ROOM2
```

## CLI Options

The `netdata-claim.sh` script accepts the following command-line options:

| Option | Description |
|--------|-------------|
| `--claim-token <token>` | The claiming token for your Netdata Cloud Space (required) |
| `-token=<token>` | Shorthand for `--claim-token` |
| `--claim-rooms <rooms>` | Comma-separated list of Room IDs to add the node to |
| `-rooms=<rooms>` | Shorthand for `--claim-rooms` |
| `--claim-url <url>` | The Netdata Cloud base URL (default: `https://app.netdata.cloud/`) |
| `-url=<url>` | Shorthand for `--claim-url` |
| `--claim-proxy <proxy>` | Proxy URL to use for the connection |
| `-proxy=<proxy>` | Shorthand for `--claim-proxy` |
| `-noproxy` | Disable proxy configuration |
| `--noproxy` | Shorthand for `-noproxy` |
| `-noreload` | Skip reloading the claiming state after configuration |
| `--noreload` | Shorthand for `-noreload` |
| `-insecure` | Disable SSL/TLS host verification |
| `--insecure` | Shorthand for `-insecure` |

### Unsupported Options

The following options are no longer supported and will display a warning if used:

| Option | Alternative |
|--------|-------------|
| `-id=<id>` | Remove `/var/lib/netdata/registry/netdata.public.unique.id` and restart the agent |
| `-hostname=<name>` | Update the main Netdata configuration manually |
| `-user=<user>` | Run the script with appropriate permissions |

## Environment Variables

You can also configure claiming using environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `NETDATA_CLAIM_TOKEN` | The claiming token for your Netdata Cloud Space | - |
| `NETDATA_CLAIM_ROOMS` | Comma-separated list of Room IDs | - |
| `NETDATA_CLAIM_URL` | The Netdata Cloud base URL | `https://app.netdata.cloud/` |
| `NETDATA_CLAIM_PROXY` | Proxy URL to use for the connection | - |
| `NETDATA_CLAIM_INSECURE` | Set to `yes` to disable SSL/TLS verification | - |
| `NETDATA_CLAIM_CONFIG_DIR` | Directory for the claim configuration | `/etc/netdata` |
| `NETDATA_CLAIM_NETDATACLI_PATH` | Path to the `netdatacli` binary | Auto-detected |
| `NETDATA_CLAIM_CONFIG_GROUP` | Netdata group for file permissions | `netdata` |

## Configuration File

The script writes configuration to `${NETDATA_CLAIM_CONFIG_DIR:-/etc/netdata}/claim.conf`. For most standard installations, this path is `/etc/netdata/claim.conf`.

```bash
[global]
    url = https://app.netdata.cloud
    token = YOUR_TOKEN
    rooms = ROOM1,ROOM2
    proxy = http://user:pass@proxy:8080
    insecure = no
```

### Configuration Options

| Option | Description | Required |
|--------|-------------|----------|
| `url` | The Netdata Cloud base URL | No (default: `https://app.netdata.cloud/`) |
| `token` | The claiming token for your Netdata Cloud Space | Yes |
| `rooms` | A comma-separated list of Room IDs | No |
| `proxy` | Proxy configuration (see below) | No |
| `insecure` | Set to `yes` to disable host verification | No |

## Proxy Configuration

You can configure proxy settings for the connection:

| Value | Description |
|-------|-------------|
| empty | No proxy configuration |
| `none` | Disable proxy configuration |
| `env` | Use the environment variable `http_proxy` (default) |
| `http://[user:pass@]host:port` | HTTP proxy |
| `socks5[h]://[user:pass@]host:port` | SOCKS5 proxy |

### Proxy Security Considerations

> **Note:** Data between Netdata Agents and Netdata Cloud remains **end-to-end encrypted** when using a proxy. The agent establishes a TLS/SSL connection through the proxy tunnel directly with Netdata Cloud.

The proxy only sees encrypted TLS traffic flowing through the tunnel; it never sees the decrypted content. This is standard HTTP CONNECT tunneling.

## Claim Status Messages

The kickstart script (when using `--claim-only`) displays messages for these claim statuses:

| Status | Message | Action |
|--------|---------|--------|
| Success | `Successfully claimed node` | None needed |
| Invalid options | `Unable to claim node due to invalid claiming options` | Check token and room IDs |
| Directory issues | `Unable to claim node due to issues creating the claiming directory` | Check permissions and disk space |
| Missing dependencies | `Unable to claim node due to missing dependencies` | Reinstall with Cloud support |
| Connection failure | `Failed to claim node due to inability to connect` | Check network and URL |
| Restart needed | `Successfully claimed node, but was not able to notify the Netdata Agent` | Restart the Netdata Agent |
| Invalid agent ID | `Failed to claim node due to an invalid agent ID` | Remove `/var/lib/netdata/registry/netdata.public.unique.id` and restart |
| Invalid hostname | `Failed to claim node due to an invalid node name` | Specify a valid hostname |
| Invalid room ID | `Failed to claim node due to an invalid room ID` | Check room IDs in Netdata Cloud |
| RSA key issues | `Failed to claim node due to an issue with the generated RSA key pair` | Remove `/var/lib/netdata/cloud.d` and retry |
| Invalid token | `Failed to claim node due to an invalid or expired claiming token` | Generate a new token from Cloud |
| Already claimed | `Failed to claim node because the Cloud thinks it is already claimed` | Use a new UUID or clear claim files |
| Already being claimed | `Failed to claim node because the node is already in the process of being claimed` | Wait and retry |
| Server error | `Failed to claim node due to an internal server error in the Cloud` | Retry later |
| No unique ID | `Unable to claim node because this Netdata installation does not have a unique ID yet` | Ensure the agent is running |

## Docker and Kubernetes

### Docker

For Docker containers, pass environment variables to the container:

```bash
docker run -d --name netdata \
  -e NETDATA_CLAIM_TOKEN=YOUR_TOKEN \
  -e NETDATA_CLAIM_ROOMS=ROOM1,ROOM2 \
  -e NETDATA_CLAIM_URL=https://app.netdata.cloud \
  netdata/netdata:latest
```

Or use a custom `claim.conf`:

```bash
docker run -d --name netdata \
  -v $(pwd)/claim.conf:/etc/netdata/claim.conf:ro \
  netdata/netdata:latest
```

### Kubernetes

For Kubernetes, use environment variables in your pod specification or add the configuration to a ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netdata-claim-config
data:
  claim.conf: |
    [global]
      url = https://app.netdata.cloud
      token = YOUR_TOKEN
      rooms = ROOM1,ROOM2
```

## Verification

After claiming, verify the connection status:

```bash
sudo netdatacli aclk-state
```

Expected output shows the ACLK status including:

```bash
ACLK Available: Yes
ACLK Version: 2
Protocols Supported: Protobuf
Protocol Used: Protobuf
MQTT Version: 5
Claimed: Yes
Claimed Id: <uuid>
Cloud URL: https://app.netdata.cloud
Online: Yes
```

You can also check the agent's API:

```bash
curl http://localhost:19999/api/v3/info | jq '.cloud'
```

## Troubleshooting

### Check Connection Status

**Web Interface:** Visit `http://NODE:19999/api/v3/info` and check the `cloud` section.

**Command Line:** Run `sudo netdatacli aclk-state` for diagnostic information.

### Common Issues

#### Invalid claiming options

**Error:** `Unable to claim node due to invalid claiming options`

**Solution:** Ensure you provide a valid `--claim-token` option.

#### Connection failure

**Error:** `Failed to claim node due to inability to connect`

**Solution:** Check network connectivity and proxy settings.

#### Already claimed

**Error:** `Failed to claim node because the Cloud thinks it is already claimed`

**Solution:** If this node was cloned from a template, remove `/var/lib/netdata/registry/netdata.public.unique.id` and restart the agent.

#### Agent restart needed

**Error:** `Successfully claimed node, but was not able to notify the Netdata Agent`

**Solution:** Restart the Netdata Agent to complete the claiming process:

```bash
sudo systemctl restart netdata
```

## Related Documentation

- [Netdata Cloud Overview](docs/netdata-cloud/README.md) - Main Netdata Cloud documentation
- [Connect Agent to Cloud](docs/netdata-cloud/README.md#connect-agent-to-cloud) - Connect agents to Netdata Cloud
- [Agent-Cloud Link (ACLK)](docs/netdata-cloud/README.md#agent-cloud-link-aclk) - Technical details about the ACLK