# NGINX Monitoring with Netdata

Monitor NGINX performance metrics including connections, requests, and status information using Netdata's built-in collectors.

## Prerequisites

- NGINX with `ngx_http_stub_status_module` compiled (standard in most distributions)
- Netdata agent installed and running
- Access to modify NGINX configuration

## Path Variations

:::note

File paths may vary by distribution:
- NGINX config: `/etc/nginx/conf.d/` (RHEL/CentOS) or `/etc/nginx/sites-available/` (Debian/Ubuntu)
- Netdata config: `/etc/netdata/` or `/opt/netdata/etc/netdata/`
- Plugin location: `/usr/libexec/netdata/` or `/usr/lib/netdata/`

To find your paths:
```bash
# Find NGINX config
nginx -t 2>&1 | grep configuration

# Find Netdata config
find /etc /opt -name netdata.conf 2>/dev/null

# Find Netdata plugins
find /usr -name go.d.plugin 2>/dev/null
```
:::

## Quick Setup (5 minutes)

### Step 1: Enable NGINX stub_status

Create a new configuration file for the status endpoint:

```nginx
# /etc/nginx/conf.d/netdata_status.conf
server {
    listen 127.0.0.1:80;
    server_name localhost;
    
    location /stub_status {
        stub_status;
        allow 127.0.0.1;
        allow ::1;
        deny all;
    }
}
```

### Step 2: Reload NGINX

```bash
# Test configuration
sudo nginx -t

# Reload if test passes
sudo systemctl reload nginx
```

### Step 3: Verify the endpoint

```bash
curl http://127.0.0.1/stub_status
```

Expected output:
```
Active connections: 1
server accepts handled requests
 10 10 10
Reading: 0 Writing: 1 Waiting: 0
```

### Step 4: Configure Netdata (optional)

Netdata auto-detects common stub_status URLs. If using a custom path:

```bash
cd /etc/netdata
sudo ./edit-config go.d/nginx.conf
```

```yaml
jobs:
  - name: local
    url: http://127.0.0.1/stub_status
```

### Step 5: Restart Netdata

```bash
sudo systemctl restart netdata
```

## Viewing Metrics

Access your Netdata dashboard at `http://your-server:19999`

Navigate to **nginx** section in the menu. You'll see:

| Chart | Description |
|-------|-------------|
| nginx.connections | Current active connections |
| nginx.connections_status | Reading, Writing, Waiting states |
| nginx.connections_accepted_handled | Accepted and handled connections per second |
| nginx.requests | HTTP requests per second |

## Multi-Node Setup

For multiple NGINX servers (reverse proxy farm):

### On each NGINX node:

1. Configure stub_status (Step 1 above)
2. Install Netdata agent
3. Configure local monitoring:

```yaml
# /etc/netdata/go.d/nginx.conf on each node
jobs:
  - name: local
    url: http://127.0.0.1/stub_status
```

### Centralized viewing:

Use Netdata Cloud or configure streaming to a parent node:

```ini
# /etc/netdata/stream.conf on child nodes
[stream]
    enabled = yes
    destination = parent-node-ip:19999
    api key = YOUR-API-KEY
```

## Common Issues

### Issue: No NGINX metrics appearing

**Check stub_status module:**
```bash
nginx -V 2>&1 | grep -o with-http_stub_status_module
```

If missing, you need NGINX compiled with this module.

**Verify endpoint accessibility:**
```bash
curl -v http://127.0.0.1/stub_status
```

**Check Netdata can reach it:**
```bash
sudo -u netdata curl http://127.0.0.1/stub_status
```

### Issue: "stub_status" vs "status" confusion

- `stub_status` - Built-in NGINX module (free, open source)
- `/nginx_status` or `/status` - Common path names, but still uses `stub_status` directive
- NGINX Plus uses `/api` for extended metrics (different module)

### Issue: 403 Forbidden

Ensure `127.0.0.1` is allowed in your configuration:
```nginx
allow 127.0.0.1;  # IPv4 localhost
allow ::1;        # IPv6 localhost
```

### Issue: Connection refused

Check NGINX is listening on 127.0.0.1:
```bash
ss -tlnp | grep :80
```

## Advanced Configuration

### Secure behind existing reverse proxy

```nginx
upstream netdata {
    server 127.0.0.1:19999;
    keepalive 64;
}

server {
    listen 80;
    server_name netdata.example.com;
    
    location / {
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://netdata;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
    }
}
```

### Custom URL path

If your stub_status is at a different location:

```yaml
# /etc/netdata/go.d/nginx.conf
jobs:
  - name: local
    url: http://127.0.0.1:8080/server-status  # Custom port and path
```

### Authentication

If stub_status requires basic auth:

```yaml
jobs:
  - name: local
    url: http://127.0.0.1/stub_status
    username: monitoring
    password: secretpass
```

### Multiple NGINX instances on same host

```yaml
jobs:
  - name: nginx_8080
    url: http://127.0.0.1:8080/stub_status
    
  - name: nginx_8081
    url: http://127.0.0.1:8081/stub_status
```

## Performance Impact

- stub_status endpoint: Negligible overhead
- Netdata collection: <1% CPU, minimal memory
- Default collection frequency: Every 1 second
- Network traffic: ~200 bytes per collection

## Security Considerations

1. **Bind to localhost only** - Never expose stub_status publicly
2. **Use allow/deny directives** - Restrict access explicitly
3. **Consider TLS** - For remote monitoring, use HTTPS
4. **Firewall rules** - Block external access to Netdata port 19999

## Next Steps

- [Configure alerts](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/agent-alert-notifications) for NGINX metrics
- [Set up Netdata Cloud](https://learn.netdata.cloud/docs/netdata-cloud) for multi-node dashboards
- [Export metrics](https://learn.netdata.cloud/docs/exporting-metrics) to external databases

## Troubleshooting Commands

```bash
# Check NGINX syntax
nginx -t

# Verify stub_status response
curl http://127.0.0.1/stub_status

# Check Netdata is running
systemctl status netdata

# View Netdata logs
journalctl -u netdata -f

# Test Netdata collection manually
/usr/libexec/netdata/plugins.d/go.d.plugin -d -m nginx

# List all available NGINX metrics
curl http://localhost:19999/api/v1/charts | jq '.charts | keys[] | select(contains("nginx"))'
```

## References

- [NGINX ngx_http_stub_status_module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html)
- [Netdata NGINX collector](https://learn.netdata.cloud/docs/collecting-metrics/popular-collectors/nginx)
- [Netdata streaming configuration](https://learn.netdata.cloud/docs/streaming)