<!--
title: "Agent-Cloud link (ACLK)"
description: "The Agent-Cloud link (ACLK) is the mechanism responsible for connecting a Netdata agent to Netdata Cloud."
date: 2020-05-11
custom_edit_url: https://github.com/netdata/netdata/edit/master/aclk/README.md
-->

# Agent-cloud link (ACLK)

The Agent-Cloud link (ACLK) is the mechanism responsible for securely connecting a Netdata Agent to your web browser
through Netdata Cloud. The ACLK establishes an outgoing secure WebSocket (WSS) connection to Netdata Cloud on port
`443`. The ACLK is encrypted, safe, and _is only established if you claim your node_.

The Cloud App lives at app.netdata.cloud which currently resolves to 35.196.244.138. However, this IP or range of 
IPs can change without notice. Watch this page for updates.

Netdata is a distributed monitoring system. Very few data are streamed to the cloud, such as data about:
 - All configured alarms and their current status
 - Metrics that are requested by the cloud user
 - A list of the active collectors

For a guide to claiming a node using the ACLK, plus additional troubleshooting and reference information, read our [get
started with Cloud](https://learn.netdata.cloud/docs/cloud/get-started) guide or the full [claiming
documentation](/claim/README.md).

## Enable and configure the ACLK

The ACLK is enabled by default, with its settings automatically configured and stored in the Agent's memory. No file is
created at `/var/lib/netdata/cloud.d/cloud.conf` until you either claim a node or create it yourself. The default
configuration uses two settings:

```conf
[global]
  enabled = yes
  cloud base url = https://app.netdata.cloud
```

If your Agent needs to use a proxy to access the internet, you must [set up a proxy for
claiming](/claim/README.md#claim-through-a-proxy).

You can configure following keys in the `netdata.conf` section `[cloud]`:
```
[cloud]
    statistics = yes
    query thread count = 2
```

- `statistics` enables/disables ACLK related statistics and their charts. You can disable this to save some space in the database and slightly reduce memory usage of Netdata Agent.
- `query thread count` specifies the number of threads to process cloud queries. Increasing this setting is useful for nodes with many children (streaming), which can expect to handle more queries (and/or more complicated queries).

## Disable the ACLK

You have two options if you prefer to disable the ACLK and not use Netdata Cloud.

### Disable at installation

You can pass the `--disable-cloud` parameter to the Agent installation when using a kickstart script
([kickstart.sh](/packaging/installer/methods/kickstart.md) or
[kickstart-static64.sh](/packaging/installer/methods/kickstart-64.md)), or a [manual installation from
Git](/packaging/installer/methods/manual.md).

When you pass this parameter, the installer does not download or compile any extra libraries. Once running, the Agent
kills the thread responsible for the ACLK and claiming behavior, and behaves as though the ACLK, and thus Netdata Cloud,
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
reinstall Netdata with Cloud enabled or change the runtime setting in your `cloud.conf` file.

If you passed `--disable-cloud` to `netdata-installer.sh` during installation, you must reinstall your Agent. Use the
same method as before, but pass `--require-cloud` to the installer. When installation finishes you can [claim your
node](/claim/README.md#how-to-claim-a-node).

If you changed the runtime setting in your `var/lib/netdata/cloud.d/cloud.conf` file, edit the file again and change
`enabled` to `yes`:

```conf
[global]
    enabled = yes
```

Restart your Agent and [claim your node](/claim/README.md#how-to-claim-a-node).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Faclk%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
