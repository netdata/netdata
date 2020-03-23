<!--
---
title: "Agent-cloud link (ACLK)"
description: "The agent-cloud link (ACLK) is the mechanism responsible for connecting a Netdata agent to Netdata Cloud. 
The ACLK uses MQTT over secure websockets to create, persist, and encrypt the connection from end-to-end, and enable 
the features found in Netdata Cloud."
date: 2020-03-23
custom_edit_url: https://github.com/netdata/netdata/edit/master/aclk/README.md
---
-->

# Agent-cloud link (ACLK)

The agent-cloud link (ACLK) is the mechanism responsible for connecting a Netdata agent to Netdata Cloud. The ACLK uses
[MQTT](https://en.wikipedia.org/wiki/MQTT) over secure websockets to create, persist, and encrypt the connection from
end-to-end, and enable the features found in Netdata Cloud.

The ACLK is a vital component of the entirely redesigned and re-architected Netdata Cloud.

_No data is exchanged with Netdata Cloud until you claim a node._ By claiming a node, you opt-in to sending data from
your agent to Netdata Cloud via the ACLK. Data used to render charts and alarms in Netdata Cloud is accessed directly
from agents through the ACLK. Though the data does flow through Netdata servers on its way from agents to the browser,
it is never logged or stored.

Read our [claiming documentation](../claim/README.md) for details on node claiming.

## Enable and configure the ACLK

The ACLK is enabled by default if its prerequisites installed correctly, and the correct configuration is already set in
your agent's `netdata.conf` file.

```conf
[cloud]
    cloud base url = https://up.netdata.cloud
```

### Proxy configuration

If your Netdata agent uses a proxy to reach the outside internet, you must configure a SOCKS5 proxy in the
`[agent_cloud_link]` section of your `netdata.conf` file. By default, the section looks like the following:

```conf
[agent_cloud_link]
    proxy = env
```

The `proxy` setting takes a few different values:

-   `env`: Netdata reads the environment variables `http_proxy` and `socks_proxy` to discover the correct
    proxy settings.
-   `none`: Do not use any proxy, even if system is configured otherwise.
-   `socks5[h]://[user:pass@]host:ip`: Netdata uses the specified SOCKS proxy.
-   `http://[user:pass@]host:ip`: Netdata uses the specified HTTP proxy.

## Disable the ACLK

If you would prefer to disable the ACLK and not use Netdata Cloud, you have two options:

1.  Pass `--disable-cloud` to `netdata-installer.sh` during installation. When you pass this parameter, the installer
    does not download or compile any extra libraries, and the agent behaves as though the ACLK, and thus Netdata Cloud,
    does not exist.

2.  Change a runtime setting in your `netdata.conf` file. This setting only stops the agent from attempting any
    connection via the ACLK, but does not prevent the installer from downloading and compiling the ACLK's dependencies.

> 🎆 Needed: Configuration flag for disabling ACLK.

```conf
[global]
    ???
```

## Troubleshooting

The ACLK is a new and complex feature, but we're committed to maintaining compatibility across all of Netdata's
installation base. The ACLK should work on every Netdata agent, but there may be edge cases of incompatibility our team
is not yet aware of.

If the ACLK fails to build during manual installation, you may see one of the following error messages:

-   Failed to build libmosquitto. The install process will continue, but you will not be able to connect this node to
    Netdata Cloud.
-   Unable to fetch sources for libmosquitto. The install process will continue, but you will not be able to connect
    this node to Netdata Cloud.
-   Failed to build libwebsockets. The install process will continue, but you may not be able to connect this node to
    Netdata Cloud.
-   Unable to fetch sources for libwebsockets. The install process will continue, but you may not be able to connect
    this node to Netdata Cloud.

If your Netdata agent updates automatically, you can also look for error messages in `/var/log/netdata/error.log`. Try
one of the following two commands to search for ACLK-related errors.

```bash
less /var/log/netdata/error.log
grep ACLK /var/log/netdata/error.log
```

To get ACLK troubleshooting help from our engineers, [create an issue on
GitHub](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage%2C+ACLK&template=bug_report.md&title=The+installer+failed+to+prepare+the+required+dependencies+for+Netdata+Cloud+functionality).
Include any error messages you might have seen during the installation process, or in `/var/log/netdata/error.log`. We
will update this troubleshooting section with specific workarounds for common issues.

## What's next?

If the ACLK is working on your node, you can move on to the [claiming process](../claim/README.md).
