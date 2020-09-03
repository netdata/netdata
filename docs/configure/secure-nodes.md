<!--
title: "Secure your nodes"
description: "Your data and systems are safe with Netdata, but "
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/configure/secure-nodes.md
-->

# Secure your nodes

Upon installation, the Netdata Agent serves the local dashboard at port `19999`. If the node is accessible to the
internet at large, anyone can access the dashboard and your node's metrics at `http://NODE:19999`.

We made this decision so that the local dashboard was immediately accessible to users, and so that we don't dictate how
professionals set up and secure their infrastructures. In addition, Netdata is read-only, cannot do anything other than
present metrics, runs without special/`sudo` privileges, and only exposes chart metadata and metric values, not raw
data.

Simply put, your [data](/docs/security-design.md#your-data-are-safe-with-netdata) and your
[systems](/docs/security-design.md#your-systems-are-safe-with-netdata) are safe with Netdata. You can read more about
Netdata's security practices in the [security design](/docs/security-design.md) doc.

While Netdata is secure by design, the local dashboard can reveal sensitive information about your infrastructure. For
example, an attacker can view which applications you run (databases, webservers, and so on), or see the names of every
user associated with the node. The default local dashboard on `19999` may also not comply with your organization's
standards.

We believe you should [protect your nodes](/docs/security-design.md#why-netdata-should-be-protected), but leave the
method up to you. We have a few recommended solutions:

-   [Disable the local dashboard](#disable-the-local-dashboard): **Simplest and recommended method** for those who have
    added nodes to Netdata Cloud and view metrics there.
-   [Restrict access to the local dashboard](#restrict-access-to-the-local-dashboard): A quick change to allow only
    certain IP addresses, such as a trusted static IP or connections from behind a management LAN.
-   [Use a reverse proxy](#use-a-reverse-proxy): Put Nginx in front of the local dashboard to improve performance,
    enable password protection, or enable SSL.

## Disable the local dashboard

This is the _recommended method for those who have claimed their nodes to Netdata Cloud_ and prefer viewing real-time
metrics using the Nodes view and Cloud dashboards.

You can disable the local dashboard entirely, but retain the encrypted Agent-Cloud link ([ACLK](/aclk/README.md) that
allows you to stream metrics on demand from your nodes via the Netdata Cloud interface. This change mitigates all
concerns about revealing metrics and system design to the internet at large, while keeping all the functionality you
need to view metrics and troubleshoot issues.

Open `netdata.conf` with `./edit-config netdata.conf`. Scroll down to the `[web]` section, and find the `mode =
static-threaded` setting. To disable the local dashboard, change this setting to `none`.

```conf
[web]
    mode = none
```

Save and close the editor, then restart your Agent using `service netdata restart`. If you try to visit the local
dashboard to `http://NODE:19999` again, the connection will fail because the local dashboard is no longer served.

> See the [configuration basics doc](/docs/configure/nodes.md) for details on how to find `netdata.conf` and use
> `edit-config`.

## Restrict access to the local dashboard

If you want to keep using the local dashboard, but don't want it exposed to the internet, you can restrict access with
access lists and the `allow connections from` setting.

With `allow connections from`, you can allow only certain IP addresses or FQDN/hostnames, such as a trusted static IP,
only `localhost`, or connections from behind a management LAN. This method also fully retains the ability to see metrics
in Netdata Cloud.

By default, this setting is `localhost *`, which allows both `localhost` connections, and _all_ connections with the `*`
wildcard. You can change this to allow only `localhost` by removing the `*` wildcard.

```conf
[web]
    # Allow only localhost connections
    allow connections from = localhost

    # Allow only from management LAN running on `10.X.X.X`
    allow connections from = 10.*

    # Allow connections only from a specific FQDN/hostname
    allow connections from = example*
```

Use Netdata's [simple patterns](/libnetdata/simple_patterns/README.md) to customize the access lists.

The `allow connections from` setting is global and restricts acess to the dashboard, badges, streaming, API, and
`netdata.conf`, but you can also set each of those access lists more granularly if you choose:

```conf
[web]
    allow connections from = localhost *
    allow dashboard from = localhost *
    allow badges from = *
    allow streaming from = *
    allow netdata.conf from = localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*
    allow management from = localhost
```

After you finish editing access lists, you can further improve security by [enabling
SSL](/web/server/README.md#enabling-tls-support) to encrypt data in transit.

## Use a reverse proxy

You can also retain the local dashboard's functionality, but put it behind a reverse proxy for additional security and
peformance. You can password-protect the local dashboard and enable HTTPS to ensure the dashboard's data is encrypted
during transit to your browser.

We recommend Nginx, as it's what we use for our [demo server](https://london.my-netdata.io/), and we have a guide
dedicated to [running Netdata behind Nginx](/docs/Running-behind-nginx.md).

We also have guides for [Apache](/docs/Running-behind-apache.md), [Lighttpd](/docs/Running-behind-lighttpd.md),
[HAProxy](/docs/Running-behind-haproxy.md), and [Caddy](/docs/Running-behind-caddy.md).

## What's next?

If you haven't already, be sure to read about [Netdata's security design](/docs/security-design.md).

Next up, learn about [collectors](/docs/collect/how-collectors-work.md) to ensure you're gathering every essential
metric about your node, its applications, and your infrastructure at large.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfigure%2Fsecure-nodesa&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
