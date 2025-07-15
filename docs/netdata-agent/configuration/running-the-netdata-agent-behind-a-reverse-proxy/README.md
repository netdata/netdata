# Running Netdata Behind a Reverse Proxy

You can improve security and capabilities by running your Netdata Agent behind another web server in production environments. This approach lets you secure access to the dashboard with SSL, user authentication, and firewall rules while providing more robustness and capabilities than the Agent's [internal web server](/src/web/README.md).

## Supported Reverse Proxy Solutions

We have documented configuration guides for these web servers:

- [nginx](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-nginx.md)
- [Apache](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-apache.md)
- [HAProxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-haproxy.md)
- [Lighttpd](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-lighttpd.md)
- [Caddy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-caddy.md)
- [H2O](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-h2o.md)

:::tip

If you prefer a different web server, we suggest you follow the nginx documentation and tell us how you did it by adding your own "Running behind webserverX" document.

:::

## Secure Direct Access to Netdata

After setting up your reverse proxy, you should firewall protect all your Netdata servers so that only the web server IP can directly access Netdata.

### Method 1: Using Firewall Rules

You can use iptables to block direct access. Run this on each of your servers (or use your firewall manager):

```bash
PROXY_IP="1.2.3.4"
iptables -t filter -I INPUT -p tcp --dport 19999 \! -s ${PROXY_IP} -m conntrack --ctstate NEW -j DROP
```

This prevents anyone except your web server from accessing a Netdata dashboard running on the host.

### Method 2: Using Netdata Configuration

You can also configure access control in `netdata.conf`:

```text
[web]
    allow connections from = localhost 1.2.3.4
```

You can add more IPs as needed to this setting.
