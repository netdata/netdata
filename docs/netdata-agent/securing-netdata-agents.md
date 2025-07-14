# Securing Netdata Agents

By default, your Netdata Agent exposes its local dashboard on port `19999`. If your node has a public IP address, the dashboard and metrics are accessible to anyone at `http://NODE:19999`.

You can protect your Agents by implementing any of these security measures:

## Security Approaches

### Recommended Methods

**Disable the Local Dashboard**

*Best for:* Users who monitor their systems through Netdata Cloud dashboards

You can secure your nodes by disabling local dashboard access while maintaining Cloud monitoring capabilities. This eliminates public exposure of metrics and system information while maintaining secure metrics viewing through Netdata Cloud via [ACLK](/src/aclk/README.md).

Edit the `[web]` section in `netdata.conf` using the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) script:

```text
[web]
    mode = none
```

Restart your Agent to apply changes. After restart, the local dashboard (http://NODE:19999) will no longer be accessible, but all metrics remain available through Netdata Cloud.

:::tip

For Docker deployments, set `NETDATA_HEALTHCHECK_TARGET=cli` in your environment variables.

:::

**Use Netdata Parents as Web Application Firewalls**

*Best for:* Production systems requiring layered security and centralized access control

You can enhance security by deploying Parent nodes as border gateways, eliminating the need for direct internet access from production Agents.

Parent nodes provide security by:
- Acting as application firewalls
- Receiving metrics from Child Agents securely
- Serving dashboard requests using local data
- Maintaining Netdata Cloud connectivity through encrypted connection

:::info

This approach isolates production systems from direct internet exposure, even when using Netdata Cloud.

For more information, see [Observability Centralization Points](/docs/observability-centralization-points/README.md).

:::

### Alternative Methods

<details>
<summary><strong>Restrict Dashboard Access to Private Networks</strong></summary><br/>

**Best for:** Organizations with private management networks

You can enhance security by binding the Agent to your organization's private management network interface. This limits dashboard access to your administrative LAN only.

**Configuration:**

Edit the `[web]` section in `netdata.conf` using the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) script:

```text
[web]
    bind to = 10.1.1.1:19999 localhost:19999
```

The Agent supports binding to multiple IPs and ports. When using hostnames, all resolved IPs will be used (for example, `localhost` typically resolves to both `127.0.0.1` and `::1`).

**Cloud Environment Setup:**

For cloud environments without private LAN capabilities or multi-cloud deployments, you can create a virtual management network using mesh VPN tools like `tincd` or `gvpe`. These tools enable secure, private communication between servers while allowing administration stations to access management functions across your cloud infrastructure.

For `gvpe` specifically, we maintain a [deployment tool](https://github.com/netdata/netdata-demo-site/tree/master/gvpe) that includes pre-compiled binaries for Linux and FreeBSD, macOS compilation script, and configuration templates. We use this tool to manage our Netdata demo sites across multiple hosting providers.

</details>

<details>
<summary><strong>Configure Granular Access Control</strong></summary><br/>

**Best for:** Specific IP address or hostname-based access requirements

You can restrict access to your local dashboard while maintaining Netdata Cloud connectivity by using [access lists](/src/web/server/README.md#access-lists).

**Basic Access Control:**

Edit the `[web]` section in `netdata.conf` using the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) script.

Use the `allow connections from` setting to permit specific IP addresses or hostnames:

```text
[web]
    # Allow only localhost connections
    allow connections from = localhost

    # Allow only from management LAN running on `10.X.X.X`
    allow connections from = 10.*

    # Allow connections only from a specific FQDN/hostname
    allow connections from = example*
```

The default setting `localhost *` allows both localhost and all external connections. You can customize this using Netdata's [simple patterns](/src/libnetdata/simple_pattern/README.md).

**Advanced Feature-Specific Controls:**

While `allow connections from` globally controls access to all Netdata services, you can set specific permissions for individual features:

```text
[web]
    allow connections from = localhost *
    allow dashboard from = localhost *
    allow badges from = *
    allow streaming from = *
    allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*
    allow management from = localhost
```

**Additional Security Options:**
- Review detailed access list options in the [Web Server documentation](/src/web/server/README.md#access-lists)
- Consider [enabling SSL](/src/web/server/README.md#enable-httpstls-support) to encrypt local dashboard traffic (Netdata Cloud connections are always TLS-encrypted)

</details>

<details>
<summary><strong>Deploy a Reverse Proxy</strong></summary><br/>

**Best for:** Multi-agent environments requiring unified authentication and SSL termination

You can secure multiple Agents using a single authenticating web server as a reverse proxy. This provides:

- Unified access through URLs like `http://{HOST}/netdata/{NETDATA_HOSTNAME}/`
- Single sign-on across all Agents
- Optional TLS encryption

**Supported Web Servers:**

We provide detailed configuration guides for popular web servers:

- [nginx](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-nginx.md)
- [HAProxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-haproxy.md)
- [Apache](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-apache.md)
- [Lighttpd](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-lighttpd.md)
- [Caddy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-caddy.md)
- [H2O](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-h2o.md)

</details>