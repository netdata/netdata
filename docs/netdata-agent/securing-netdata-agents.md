# Securing Netdata Agents

Upon installation, the Agent serves the **local dashboard** at port `19999`. If the node's IP is publicly accessible, anyone can load the dashboard and metrics at `http://NODE:19999`.

Below are some best practices that can further secure your Agents.

- [Disable the local dashboard](#disable-the-local-dashboard)

  **Simplest and recommended method** for those who have added nodes to Netdata Cloud and view dashboards and metrics there.

- [Use Netdata Parents as Web Application Firewalls](#use-netdata-parents-as-web-application-firewalls)

- [Expose the local dashboard only in a private LAN](#expose-the-local-dashboard-only-in-a-private-lan)

  To retain access to your node's local dashboard, via a LAN connection.

- [Fine-grained access control](#fine-grained-access-control)

  Allow local dashboard access only from specific IP addresses, such as a trusted static IP or connections from behind a management LAN.

- [Use a reverse proxy (authenticating web server in proxy mode)](#use-an-authenticating-web-server-in-proxy-mode)

  Password-protect a local dashboard and enable TLS to secure it.

## Disable the local dashboard

This is the recommended method for those who have connected their nodes to Netdata Cloud.

You can disable the local dashboard (and API) but retain the encrypted Agent-Cloud link ([ACLK](/src/aclk/README.md)). That allows you to view metrics on demand from your nodes via the Cloud UI. This change mitigates all concerns about revealing metrics and system design to the internet at large, while keeping all the functionality you need to view metrics and troubleshoot issues using Netdata Cloud.

Open `netdata.conf` using the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) script. Scroll down to the `[web]` section, and find the `mode = static-threaded` setting, and change it to `none`.

```text
[web]
    mode = none
```

Save and close the editor, then [restart your Agent](/docs/netdata-agent/start-stop-restart.md). If you try to visit the local dashboard to `http://NODE:19999` again, the connection will fail as that node no longer serves it.

> **Note**
>
> If you’re using Netdata with Docker, make sure to set the `NETDATA_HEALTHCHECK_TARGET` environment variable to `cli`.

## Use Netdata Parents as Web Application Firewalls

The Agents you install on your production systems don’t need direct access to the Internet. Even when you use Netdata Cloud, you can appoint one or more Parents to act as border gateways or application firewalls, isolating your production systems from the rest of the world. They receive metric data from their Children or other Parents on one side, and serve most queries using their own copy of the data to satisfy dashboard requests on the other side.

For more information, see our documentation about [Observability Centralization Points](/docs/observability-centralization-points/README.md).

## Expose the local dashboard only in a private LAN

If your organization has a private administration and management LAN, you can bind the Agent on this network interface on all your servers.

Open `netdata.conf` using the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) script. Scroll down to the `[web]` section, and set:

```text
[web]
    bind to = 10.1.1.1:19999 localhost:19999
```

You can bind the Agent to multiple IPs and ports. If you use hostnames, it will resolve them and use all the IPs (in the above example `localhost` usually resolves to both `127.0.0.1` and `::1`).

<details><summary>More info for cloud-based installations</summary>

For cloud-based installations, if your cloud provider doesn’t provide such a private LAN (or if you use multiple providers), you can create a virtual management and administration LAN with tools like `tincd` or `gvpe`. These tools create a mesh VPN allowing all servers to communicate securely and privately. Your administration stations join this mesh VPN to get access to management and administration tasks on all your cloud servers.

For `gvpe` we have developed a [simple provisioning tool](https://github.com/netdata/netdata-demo-site/tree/master/gvpe) you may find handy (it includes statically compiled `gvpe` binaries for Linux and FreeBSD, and also a script to compile `gvpe` on your macOS system). We use this to create a management and administration LAN for all Netdata demo sites (spread all over the internet using multiple hosting providers).

</details>

## Fine-grained access control

If you want to keep using the local dashboard, but don't want it exposed to the internet, you can restrict access with [access lists](/src/web/server/README.md#access-lists). This method also fully retains the ability to view metrics on-demand through Netdata Cloud.

The `allow connections from` setting helps you allow only certain IP addresses or FQDN/hostnames, such as a trusted static IP, only `localhost`, or connections from behind a management LAN.

By default, this setting is `localhost *`. This setting allows connections from `localhost` in addition to _all_ connections, using the `*` wildcard. You can change this setting using Netdata's [simple patterns](/src/libnetdata/simple_pattern/README.md).

```text
[web]
    # Allow only localhost connections
    allow connections from = localhost

    # Allow only from management LAN running on `10.X.X.X`
    allow connections from = 10.*

    # Allow connections only from a specific FQDN/hostname
    allow connections from = example*
```

The `allow connections from` setting is global and restricts access to the dashboard, badges, streaming, API, and `netdata.conf`, but you can also set each of those access lists in more detail if you want:

```text
[web]
    allow connections from = localhost *
    allow dashboard from = localhost *
    allow badges from = *
    allow streaming from = *
    allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*
    allow management from = localhost
```

Check the [Web Server](/src/web/server/README.md#access-lists) reference for additional details about access lists.

You can take access lists one step further by [enabling SSL](/src/web/server/README.md#enable-httpstls-support) to encrypt data from the local dashboard in transit. The connection to Netdata Cloud is always secured with TLS.

## Use an authenticating web server in proxy mode

You can use one web server to provide authentication in front of **all your Agents**. You will be accessing them with URLs like `http://{HOST}/netdata/{NETDATA_HOSTNAME}/` and authentication will be shared among all of them (you will sign in once for all your servers).
Instructions are provided on how to set the proxy configuration to have Netdata run behind [nginx](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-nginx.md), [HAproxy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-haproxy.md), [Apache](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-apache.md), [lighthttpd](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-lighttpd.md), [caddy](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-caddy.md), and [H2O](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/Running-behind-h2o.md).
