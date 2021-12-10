<!--
title: "Agent-Cloud link (ACLK)"
description: "The Agent-Cloud link (ACLK) is the mechanism responsible for connecting a Netdata agent to Netdata Cloud."
date: 2020-05-11
custom_edit_url: https://github.com/netdata/netdata/edit/master/aclk/README.md
-->

# Agent-cloud link (ACLK)

The Agent-Cloud link (ACLK) is the mechanism responsible for securely connecting a Netdata Agent to your web browser
through Netdata Cloud. The ACLK establishes an outgoing secure WebSocket (WSS) connection to Netdata Cloud on port
`443`. The ACLK is encrypted, safe, and _is only established if you connect your node_.

The Cloud App lives at app.netdata.cloud which currently resolves to 35.196.244.138. However, this IP or range of 
IPs can change without notice. Watch this page for updates.

For a guide to connecting a node using the ACLK, plus additional troubleshooting and reference information, read our [get
started with Cloud](https://learn.netdata.cloud/docs/cloud/get-started) guide or the full [connect to Cloud
documentation](/claim/README.md).

## Data privacy

[Data privacy](https://netdata.cloud/data-privacy/) is very important to us. We firmly believe that your data belongs to
you. This is why **we don't store any metric data in Netdata Cloud**. 

All the data that the user sees in the web browser when using Netdata Cloud, are actually streamed directly from the Netdata Agent to the Netdata Cloud dashboard. They pass through our systems, but they are not stored.

We do however store a limited number of *metadata* to be able to offer the stunning visualizations and advanced functionality of Netdata Cloud.

### Metadata

The information we store in Netdata Cloud is the following (using the publicly available demo server `frankfurt.my-netdata.io` as an example):
- The email address you used to sign up/or sign in
- For each node connected to your Spaces in Netdata Cloud:
 - Hostname (as it appears in Netdata Cloud)
 - Information shown in `/api/v1/info`. For example: [https://frankfurt.my-netdata.io/api/v1/info](https://frankfurt.my-netdata.io/api/v1/info).
 - The chart metadata shown in `/api/v1/charts`. For example: [https://frankfurt.my-netdata.io/api/v1/info](https://frankfurt.my-netdata.io/api/v1/info).
 - Alarm configurations shown in `/api/v1/alarms?all`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms?all](https://frankfurt.my-netdata.io/api/v1/alarms?all).
 - Active alarms shown in `/api/v1/alarms`. For example: [https://frankfurt.my-netdata.io/api/v1/alarms](https://frankfurt.my-netdata.io/api/v1/alarms).

How we use them:
- The data are stored in our production database on Google Cloud and some of it is also used in BigQuery, our data lake, for analytics purposes. These analytics are crucial for our product development process.
- Email is used to identify users in regards to product use and to enrich our tools with product use, such as our CRM.
- This data is only available to Netdata and never to a 3rd party.

## Enable and configure the ACLK

The ACLK is enabled by default, with its settings automatically configured and stored in the Agent's memory. No file is
created at `/var/lib/netdata/cloud.d/cloud.conf` until you either connect a node or create it yourself. The default
configuration uses two settings:

```conf
[global]
  enabled = yes
  cloud base url = https://app.netdata.cloud
```

If your Agent needs to use a proxy to access the internet, you must [set up a proxy for
connecting to cloud](/claim/README.md#connect-through-a-proxy).

You can configure following keys in the `netdata.conf` section `[cloud]`:
```
[cloud]
  statistics = yes
  query thread count = 2
  aclk implementation = ng
```

- `statistics` enables/disables ACLK related statistics and their charts. You can disable this to save some space in the database and slightly reduce memory usage of Netdata Agent.
- `query thread count` specifies the number of threads to process cloud queries. Increasing this setting is useful for nodes with many children (streaming), which can expect to handle more queries (and/or more complicated queries).
- `aclk implementation` - see [ACLK implementation](#aclk-implementation) section

## ACLK implementation

Currently we are in process of switching ACLK to brand new technical stack called ACLK-NG. To choose your implementation, change the `aclk implementation` setting in your `netdata.conf` (accepted values `ng` or `legacy`).

Before changing this value, check the desired implementation is available (determined at build time) by running `netdata -W buildinfo`. Following lines indicate which ACLK implementations are available:

```
Features:
    ACLK Next Generation:    YES
    ACLK Legacy:             YES
```

To verify which ACLK implementation Netdata uses, visit the `/api/v1/info` endpoint on your local dashboard and check the `aclk-implementation` key.

New Netdata Cloud features will be implemented on top of ACLK-NG only. ACLK Legacy is therefore kept as a fallback in case some users have issues.


> Note: ACLK Legacy will be removed in following releases! 
> Update your ACLK configuration to ACLK-NG to prevent any disruptions.


### Improvements of ACLK-NG over Legacy are:
- No dependency on custom patched `libmosquitto` (no bundling of libraries). Which should remove obstacles many GNU/Linux distribution package maintainers had trying to provide Netdata with Cloud support
- No dependency on libwebsockets
- Lower latency and higher throughput
- More up to date, new features for Netdata Cloud are currently developed on top of ACLK-NG first

## Disable the ACLK

You have two options if you prefer to disable the ACLK and not use Netdata Cloud.

### Disable at installation

You can pass the `--disable-cloud` parameter to the Agent installation when using a kickstart script
([kickstart.sh](/packaging/installer/methods/kickstart.md) or
[kickstart-static64.sh](/packaging/installer/methods/kickstart-64.md)), or a [manual installation from
Git](/packaging/installer/methods/manual.md).

When you pass this parameter, the installer does not download or compile any extra libraries. Once running, the Agent
kills the thread responsible for the ACLK and connecting behavior, and behaves as though the ACLK, and thus Netdata Cloud,
does not exist.

### Disable at runtime

You can change a runtime setting in your `cloud.conf` file to disable the ACLK. This setting only stops the Agent from
attempting any connection via the ACLK, but does not prevent the installer from downloading and compiling the ACLK's
dependencies.

The file typically exists at `/var/lib/netdata/cloud.d/cloud.conf`, but can change if you set a prefix during
installation. To disable the ACLK, open that file and change the `enabled` setting to `no`:

```conf
[global]
    enabled = no
```

If the file at `/var/lib/netdata/cloud.d/cloud.conf` doesn't exist, you need to create it. 

Copy and paste the first two lines from below, which will change your prompt to `cat`.

```bash
cd /var/lib/netdata/cloud.d
cat > cloud.conf << EOF
```

Copy and paste in lines 3-6, and after the final `EOF`, hit **Enter**. The final line must contain only `EOF`. Hit **Enter** again to return to your normal prompt with the newly-created file.

To get your normal prompt back, the final line
must contain only `EOF`.

```bash
[global]
    enabled = no
    cloud base url = https://app.netdata.cloud
EOF
```

You also need to change the file's permissions. Use `grep "run as user" /etc/netdata/netdata.conf` to figure out which
user your Agent runs as (typically `netdata`), and replace `netdata:netdata` as shown below if necessary:

```bash
sudo chmod 0770 cloud.conf
sudo chown netdata:netdata cloud.conf
```

Restart your Agent to disable the ACLK.

### Re-enable the ACLK

If you first disable the ACLK and any Cloud functionality and then decide you would like to use Cloud, you must either
[reinstall Netdata](/packaging/installer/REINSTALL.md) with Cloud enabled or change the runtime setting in your
`cloud.conf` file.

If you passed `--disable-cloud` to `netdata-installer.sh` during installation, you must
[reinstall](/packaging/installer/REINSTALL.md) your Agent. Use the same method as before, but pass `--require-cloud` to
the installer. When installation finishes you can [connect your node](/claim/README.md#how-to-connect-a-node).

If you changed the runtime setting in your `var/lib/netdata/cloud.d/cloud.conf` file, edit the file again and change
`enabled` to `yes`:

```conf
[global]
    enabled = yes
```

Restart your Agent and [connect your node](/claim/README.md#how-to-connect-a-node).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Faclk%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
