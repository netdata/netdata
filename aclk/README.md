<!--
---
title: "Agent-cloud link (ACLK)"
description: "The agent-cloud link (ACLK) is the mechanism responsible for connecting a Netdata agent to Netdata Cloud. 
The ACLK uses MQTT over secure websockets to create, persist, and encrpyt the connect from end-to-end, and enable the 
features found in Netdata Cloud."
date: 2020-03-16
custom_edit_url: https://github.com/netdata/netdata/edit/master/aclk/README.md
---
-->

# Agent-cloud link (ACLK)

The agent-cloud link (ACLK) is the mechanism responsible for connecting a Netdata agent to Netdata Cloud. The ACLK uses
[MQTT](https://en.wikipedia.org/wiki/MQTT) over secure websockets to create, persist, and encrpyt the connect from
end-to-end, and enable the features found in Netdata Cloud.

The ACLK is a vital component of the entirely redesigned and re-architected Netdata Cloud.

## What data is sent through the ACLK?

_No data is exchanged with Netdata Cloud until you claim a node._ By claiming a node, you opt-in to sending data from
your agent to Netdata Cloud via the ACLK.

Three types of data are sent across the ACLK:

-   Configuration: Details available via an agent's [API's `info`
    payload](https://registry.my-netdata.io/swagger/#/default/get_info) at `/api/v1/info`.
-   Alarms: States and trigger events.
-   Metrics: Information from the agent's API are redirected to a user's browser via the ACLK. However, this information
    is only sent via the ACLK after a user explictly requests this information via the Netdata Cloud UI.

## Claim nodes with the ACLK

In order to use the ACLK to add a node to Netdata Cloud, you must **claim** the node.

Claiming nodes is a security feature. Through the process of claiming, you demonstate in a few ways that you have
administrative access to that node and the configuration settings for its Netdata agent. By logging into the node, you
prove you have access, and by running the `netdata-cloud.sh` script, you prove you have write access and thus
administrative privileges.

The claiming process ensures the ACLK will never be used by a third party to add your node, and thus view your metrics,
via their Netdata Cloud account.

When you first sign in to the new Netdata Cloud, it will help onboard you to the new platform by asking you to **claim**
any nodes you want to add to Netdata Cloud. The claiming process involves running a command, provided to you by the
onboarding process, on each node to establish the token and use the ACLK to create a connection between the node and
Netdata Cloud.

```bash
netdata-cloud.sh --token 5AJRdC9H.XytDT8yWZB5psoy.BzCQz5IRm3uqo.nHNl9HQG3nEiTbPBxu_qYW2xdeFsEqFboD2.sf6zVPuveRtK3Xc1ouuDOS9hiCCLJRx1IBvktsvD16zH57tfCYh3kWxjEQH4 --rooms general,web
```

The `netdata-cloud.sh` command takes two arguments:

-   `--token`: The unique token to your Netdata Cloud account to establish the ACLK.
-   `--rooms`: A comma-separated list of the War Rooms you want to add this node to.

> It may take up to 60 seconds for your node to connect to Netdata Cloud after executing the `netdata-cloud.sh` command.

When a user of a Space visits an agent claimed by that Space, they automatically grant that agent access to their Cloud
data via the ACLK.

## Enable and configure the ACLK

The ACLK is enabled by default if its prerequisites installed correctly. You do not need to explicitly configure the
ACLK to connect to Netdata Cloud. There are configuration options in `netdata.conf`, but these do not need to be changed
by users in ordinary circumstances.

> 🎆 Needed: Any other configuration settings?

```conf
[cloud]
    cloud base url = https://up.netdata.cloud
```

### Proxy users

If your Netdata agent uses a proxy to reach the outside internet, you must set up a SOCKS5 proxy.

> 🎆 Needed: Configuration options for proxy.

## Disable the ACLK

If you would prefer to disable the ACLK and not use Netdata Cloud, you have two options.

The first option is to pass `--disable-cloud` to `netdata-installer.sh` during installation. When you pass this
parameter, the installer does not download or compile any extra libraries, and the agent behaves as though the ACLK, and
thus Netdata Cloud, does not exist.

The second option is to change a runtime setting in your `netdata.conf` file. This setting will only stop the agent from
attempting the connection, but will not prevent the installer from downloading and compiling the ACLK's dependencies.

> 🎆 Needed: Configuration flag for disabling ACLK.

```conf
[global]
    ???
```

## Troubleshooting

The ACLK is a new and complex feature, but we're commited to maintaining compatibility across all of Netdata's
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
one of the following two commands to search for ACLK-related errrors.

```bash
less /var/log/netdata/error.log
grep ACLK /var/log/netdata/error.log
```

To ACLK troubleshooting help from our engineers, [create an issue on
GitHub](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage%2C+ACLK&template=bug_report.md&title=The+installer+failed+to+prepare+the+required+dependencies+for+Netdata+Cloud+functionality).
Include any error messages you might have seen during the installation process, or in `/var/log/netdata/error.log`. We
will update this troubleshooting section with specific workarounds for common issues.
