# Agent-Cloud link (ACLK)

The Agent-Cloud Link (ACLK) provides secure communication between your Netdata Agents and Cloud. This connection:
- Uses outgoing secure WebSocket (WSS) on port `443`
- Activates only after you [connect a node](/src/claim/README.md) to your Space
- Ensures encrypted, safe data transmission

For ACLK to function properly, your Agents need outbound access to Netdata Cloud services.

| Allowlisting Method | Required Access                                                            |
|---------------------|----------------------------------------------------------------------------|
| Domain              | • `app.netdata.cloud`<br/>• `api.netdata.cloud`<br/>• `mqtt.netdata.cloud` |

:::warning

IP addresses can change without notice! Always **prefer domain allowlisting**. If you must use IP addresses, be aware that they vary based on your geographic location due to CDN-edge servers. You'll need to regularly verify the IP addresses specific to your region.

:::

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

If your Agent requires a proxy to access the internet, you'll need to [configure proxy settings](/src/claim/README.md#automatically-via-a-provisioning-system-or-the-command-line).

## Troubleshooting

### Check ACLK connection status

Use these commands to verify your ACLK connection:

```bash
# Command-line status check
sudo netdatacli aclk-state

# JSON format for programmatic access
sudo netdatacli aclk-state json

# Web API endpoint
curl http://localhost:19999/api/v1/aclk
curl http://localhost:19999/api/v3/info
```

**Expected output:**
```
ACLK Available: Yes/No
Claimed: Yes/No
Claimed Id: <UUID>
Online: Yes/No
Cloud URL: https://app.netdata.cloud
```

### ACLK status codes

| Status Code | Meaning | Action Required |
|-------------|---------|-----------------|
| `ACLK_STATUS_CONNECTED` | Successfully connected | None |
| `ACLK_STATUS_OFFLINE` | Generic offline state | Check network/firewall |
| `ACLK_STATUS_DISABLED` | ACLK disabled in config | Enable in netdata.conf |
| `ACLK_STATUS_CANT_CONNECT_NO_CLOUD_URL` | Missing cloud URL | Configure `[cloud] url` |
| `ACLK_STATUS_CANT_CONNECT_INVALID_CLOUD_URL` | Malformed cloud URL | Fix URL format |
| `ACLK_STATUS_BLOCKED` | Agent temporarily blocked | Wait for backoff period |
| `ACLK_STATUS_NO_OLD_PROTOCOL` | Protocol mismatch | Update agent |
| `ACLK_STATUS_NO_PROTOCOL_CAPABILITY` | Missing proto capability | Update agent |
| `ACLK_STATUS_INVALID_ENV_AUTH_URL` | Auth endpoint parse error | Check cloud connectivity |
| `ACLK_STATUS_INVALID_ENV_TRANSPORT_IDX` | No usable transport | Check network/proxy |
| `ACLK_STATUS_INVALID_ENV_TRANSPORT_URL` | Transport URL invalid | Contact support |
| `ACLK_STATUS_NO_LWT_TOPIC` | Missing Last Will topic | Check claiming |
| `ACLK_STATUS_OFFLINE_CLOUD_REQUESTED_DISCONNECT` | Cloud initiated disconnect | Temporary, will reconnect |
| `ACLK_STATUS_OFFLINE_PING_TIMEOUT` | Ping timeout (60s) | Check network latency |
| `ACLK_STATUS_OFFLINE_RELOADING_CONFIG` | Reclaiming/config reload | Wait for completion |
| `ACLK_STATUS_OFFLINE_POLL_ERROR` | poll() system call failed | Check system resources |
| `ACLK_STATUS_OFFLINE_CLOSED_BY_REMOTE` | Remote closed connection | Check cloud status |
| `ACLK_STATUS_OFFLINE_SOCKET_ERROR` | Socket error | Check network |
| `ACLK_STATUS_OFFLINE_MQTT_PROTOCOL_ERROR` | MQTT protocol violation | Update agent |
| `ACLK_STATUS_OFFLINE_WS_PROTOCOL_ERROR` | WebSocket protocol error | Check proxy compatibility |
| `ACLK_STATUS_OFFLINE_MESSAGE_TOO_BIG` | Message exceeds limit | Contact support |

### Log file locations

ACLK writes logs to your Netdata log directory:

```bash
# Default log locations (Linux)
/var/log/netdata/error.log    # ACLK errors logged here
/var/log/netdata/access.log   # ACLK connection events
/var/log/netdata/debug.log    # Debug output (if enabled)

# Check current log directory
grep "log directory" /etc/netdata/netdata.conf
```

### Enable debug logging

To enable ACLK debug logging, edit `/etc/netdata/netdata.conf`:

```ini
[logs]
    debug flags = 0x0000000000000800  # D_ACLK flag
```

Then restart Netdata:
```bash
sudo systemctl restart netdata
```

### Common error messages

| Error Message | Cause | Solution |
|---------------|-------|----------|
| `Claimed agent cannot establish ACLK - unable to load private key` | Missing/corrupted `/var/lib/netdata/cloud.d/private.pem` | Re-claim the agent |
| `Agent is claimed but the URL in configuration key "url" is invalid` | Malformed cloud URL | Fix `[cloud] url` in netdata.conf |
| `Connection Error or Dropped` | Network connectivity issue | Check firewall, DNS, routing |
| `MQTT CONNACK returned error` | Authentication/authorization failure | Verify claim token, re-claim |
| `Error setting TLS SNI host` | SSL/TLS configuration issue | Check OpenSSL version |
| `Failed to start SSL connection` | SSL handshake failure | Check certificates, proxy settings |

### Test network connectivity

Verify connectivity to required Netdata Cloud endpoints:

```bash
# Test DNS resolution
nslookup mqtt.netdata.cloud
nslookup api.netdata.cloud
nslookup app.netdata.cloud

# Test HTTPS connectivity (port 443)
curl -v https://app.netdata.cloud
curl -v https://api.netdata.cloud

# Check if port 443 is reachable
nc -zv mqtt.netdata.cloud 443
```

### Verify claiming status

Check if your Agent is properly claimed:

```bash
# Check if agent is claimed
ls -la /var/lib/netdata/cloud.d/

# Expected files after successful claiming:
# - private.pem (RSA private key)
# - claimed_id (UUID of the claim)
# - token (claim token used)

# Verify private key format
openssl rsa -in /var/lib/netdata/cloud.d/private.pem -check -noout

# Check claimed ID
cat /var/lib/netdata/cloud.d/claimed_id
```

### Re-claim an Agent

If ACLK fails due to claiming issues:

```bash
# 1. Stop Netdata
sudo systemctl stop netdata

# 2. Remove claiming data
sudo rm -rf /var/lib/netdata/cloud.d/

# 3. Re-claim using new token
sudo netdata-claim.sh -token=YOUR_NEW_TOKEN -rooms=YOUR_ROOM -url=https://app.netdata.cloud

# 4. Start Netdata
sudo systemctl start netdata

# 5. Verify connection
sudo netdatacli aclk-state
```

### Firewall configuration

Ensure your firewall allows outbound HTTPS (TCP 443) to Netdata Cloud services:

```bash
# Example iptables rules
sudo iptables -A OUTPUT -p tcp -d mqtt.netdata.cloud --dport 443 -j ACCEPT
sudo iptables -A OUTPUT -p tcp -d api.netdata.cloud --dport 443 -j ACCEPT
sudo iptables -A OUTPUT -p tcp -d app.netdata.cloud --dport 443 -j ACCEPT
```

**Corporate firewall checklist:**
- [ ] Outbound TCP 443 allowed to `*.netdata.cloud`
- [ ] WebSocket protocol not blocked
- [ ] TLS 1.2+ supported
- [ ] No SSL inspection/MITM on Netdata traffic
- [ ] DNS resolution working for `*.netdata.cloud`

### Proxy troubleshooting

Test proxy connectivity:

```bash
# Test HTTP proxy
curl -v --proxy http://proxy.example.com:8080 https://app.netdata.cloud

# Test with authentication
curl -v --proxy http://user:pass@proxy.example.com:8080 https://app.netdata.cloud

# Check proxy environment variables
echo $http_proxy
echo $https_proxy
```

**Common proxy issues:**

| Issue | Symptom | Solution |
|-------|---------|----------|
| Proxy blocks WebSocket | `ACLK_STATUS_OFFLINE_WS_PROTOCOL_ERROR` | Configure proxy to allow WebSocket Upgrade |
| Proxy requires auth | Connection fails silently | Add username/password to proxy config |
| Proxy does SSL inspection | SSL handshake fails | Whitelist `*.netdata.cloud` from inspection |

:::note

SOCKS5 proxies are not supported by ACLK.

:::

### Connection lifecycle

ACLK uses Truncated Binary Exponential Backoff (TBEB) for reconnections:

1. **Initial connection attempt:** Immediate
2. **First failure:** Wait 1-2 seconds
3. **Subsequent failures:** Double wait time up to maximum
4. **Maximum backoff:** ~5 minutes
5. **Stable connection:** After 3 successful PUBACKs, backoff resets

ACLK expects a ping response within **60 seconds**. If no response is received, the connection is dropped and reconnection begins.

### Force immediate reconnection

```bash
# Method 1: Reload claiming state (triggers reconnect)
sudo netdatacli reload-claiming-state

# Method 2: Restart Netdata
sudo systemctl restart netdata
```

### Container-specific issues

When running Netdata in Docker, ensure persistent storage for claiming data:

```bash
docker run -d \
  -v /var/lib/netdata:/var/lib/netdata:rw \
  -e NETDATA_CLAIM_TOKEN=YOUR_TOKEN \
  -e NETDATA_CLAIM_ROOMS=YOUR_ROOM \
  netdata/netdata
```

For Kubernetes deployments, ensure persistent volume claims are configured:

```yaml
# In Helm values.yaml
claim:
  token: "YOUR_TOKEN"
  rooms: "YOUR_ROOM"

persistence:
  enabled: true
  size: 1Gi
```

## Frequently asked questions

<details>
<summary>How do I check if ACLK is working?</summary>

Run `sudo netdatacli aclk-state` to check the connection status. The output should show `Online: Yes` if ACLK is connected successfully.

</details>

<details>
<summary>Why does ACLK keep disconnecting?</summary>

Common causes include:
- Network instability or high latency
- Firewall rules blocking intermittent connections
- Proxy issues with WebSocket protocols
- SSL/TLS handshake failures

Check your logs at `/var/log/netdata/error.log` for specific error messages.

</details>




Then restart Netdata: `sudo systemctl restart netdata`

</details>

<details>
<summary>Can I use a custom Cloud URL?</summary>

Yes, for on-premise deployments. Edit `/etc/netdata/netdata.conf`:

```ini
[cloud]
    url = https://your-onprem-cloud.example.com
```

</details>

<details>
<summary>What OpenSSL version is required?</summary>

ACLK requires OpenSSL 1.0.2 or newer. Check your version with `openssl version`. If your version is too old, consider using the static build which includes a modern OpenSSL version:

```bash
bash <(curl -Ss https://get.netdata.cloud/kickstart.sh) --static-only
```

</details>

<details>
<summary>How do I troubleshoot "unable to load private key" errors?</summary>

This error indicates missing or corrupted claiming data. To fix it:

1. Stop Netdata: `sudo systemctl stop netdata`
2. Remove claiming data: `sudo rm -rf /var/lib/netdata/cloud.d/`
3. Re-claim your Agent using a new claim token
4. Start Netdata: `sudo systemctl start netdata`

</details>
