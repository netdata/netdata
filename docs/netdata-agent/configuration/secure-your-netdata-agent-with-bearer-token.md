# Secure Your Netdata Agent with Bearer Token Protection

Netdata provides native bearer token protection that integrates with Netdata Cloud Single Sign-On (SSO). With a single configuration setting, you can secure direct access to your Netdata agents and parents while inheriting the same permissions and roles your users have in Netdata Cloud.

## Who Can Use This

Bearer token protection is available to all Netdata Cloud users:

- **Community plan** (free)
- **Business plan** (paid)

Your agent must be [claimed to Netdata Cloud](/docs/netdata-cloud/connect-agent-to-cloud.md) to use this feature.

## How It Works

When bearer token protection is enabled:

1. Users visit your agent's dashboard directly (e.g., `http://your-server:19999`)
2. The agent redirects them to Netdata Cloud for authentication
3. After successful Cloud SSO login, users receive a time-limited bearer token
4. The token grants access based on their Netdata Cloud role (Admin, Manager, Troubleshooter, etc.)
5. Tokens expire after 24 hours and are automatically renewed through Cloud

This means:

- **Single Sign-On**: Users authenticate once via Netdata Cloud
- **Role-Based Access**: Cloud roles and permissions apply to direct agent access
- **Centralized Control**: Manage access through Netdata Cloud, not per-agent configurations
- **No Password Files**: No htpasswd files or reverse proxy auth configuration needed

## Enable Bearer Token Protection

Edit your `netdata.conf` using the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) script:

```bash
cd /etc/netdata
sudo ./edit-config netdata.conf
```

Add or modify the `[web]` section:

```ini
[web]
    bearer token protection = yes
```

Restart Netdata to apply:

```bash
sudo systemctl restart netdata
```

## What Gets Protected

When enabled, bearer token protection secures **all data APIs**, including:

- Metrics and charts (`/api/v3/data`, `/api/v3/allmetrics`)
- Alerts (`/api/v3/alerts`, `/api/v3/alert_transitions`)
- Contexts and nodes (`/api/v3/contexts`, `/api/v3/nodes`)
- Functions (`/api/v3/function`, `/api/v3/functions`)
- Dynamic configuration (`/api/v3/config`)

## What Remains Public

**Static web files** (HTML, CSS, JavaScript) in Netdata's web directory are **not protected**. This means:

- Users can still download and view the dashboard UI
- The dashboard will load but **won't display any data**
- All API calls from the dashboard will fail until the user authenticates

This is by design - it allows the dashboard to redirect users to Netdata Cloud for authentication.

A small set of APIs also remain publicly accessible for operational reasons:

| API | What it exposes |
|-----|-----------------|
| `/api/v3/info` | Agent version, OS, build info, capabilities |
| `/api/v3/me` | Current user authentication status |
| `/api/v3/claim` | Agent claiming endpoint (protected by separate security key) |
| `/api/v3/stream_info` | Streaming connection statistics |
| `/api/v2/claim` | Agent claiming endpoint (v2, protected by security key) |
| `/api/v1/registry?action=hello` | Node list, machine GUIDs, cloud connection status |
| `/api/v1/manage/health` | Alert silencing (protected by separate X-Auth-Token) |

These APIs are required for the authentication flow and dashboard initialization. The registry `hello` action returns node identifiers and cloud connection status, which the dashboard needs to initiate the authentication redirect.

**Note:** Other v1 and v2 APIs (like `/api/v2/info`, `/api/v3/versions`, `/api/v3/progress`) **are protected** by bearer token - only the specific endpoints listed above bypass protection.

## Requirements

- Agent must be claimed to Netdata Cloud
- ACLK connection must be active (agent connected to Cloud)
- Users must have a Netdata Cloud account with access to the space containing the agent

## Comparison with Other Methods

| Method | Setup Complexity | SSO | Centralized Management | Works Offline |
|--------|-----------------|-----|----------------------|---------------|
| **Bearer Token Protection** | Single setting | Yes | Yes | No |
| Reverse Proxy + Basic Auth | High (proxy + htpasswd) | No | No | Yes |
| IP-Based Restrictions | Medium | No | No | Yes |
| Disable Dashboard | Single setting | N/A | N/A | N/A |

Choose bearer token protection when you want the simplest setup with Cloud SSO integration. Choose reverse proxy if you need custom authentication, don't use Netdata Cloud, or require offline access.

## Combining with Other Security Measures

Bearer token protection can be combined with:

- **TLS/SSL encryption**: Configure [TLS in Netdata](/src/web/server/README.md#examples) for encrypted connections
- **IP restrictions**: Add `allow connections from` to limit which IPs can even attempt to connect
- **Firewall rules**: Block port 19999 from untrusted networks

Example combining bearer token with IP restrictions:

```ini
[web]
    bearer token protection = yes
    allow connections from = 10.* 192.168.* localhost
```

## Troubleshooting

**Users can't authenticate:**

- Verify the agent is claimed: Check `http://your-server:19999/api/v3/info` for `cloud-available: true`
- Verify ACLK is connected: Look for "ACLK" status in the agent logs
- Ensure users have access to the same Cloud space as the agent

**Token expired errors:**

- Tokens automatically renew when users have an active Cloud session
- If tokens expire, users simply re-authenticate through Cloud

**Want to disable temporarily:**

```ini
[web]
    bearer token protection = no
```

Or via API (requires Admin/Manager role via Cloud):

```
POST /api/v3/bearer_protection
```
