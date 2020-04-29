<!--
---
title: "Agent-Cloud link (ACLK)"
description: "The Agent-Cloud link (ACLK) is the mechanism responsible for connecting a Netdata agent to Netdata Cloud."
date: 2020-04-29
custom_edit_url: https://github.com/netdata/netdata/edit/master/aclk/README.md
---
-->

# Agent-cloud link (ACLK)

The Agent-Cloud link (ACLK) is the mechanism responsible for connecting a Netdata Agent to Netdata Cloud. The ACLK uses
[MQTT](https://en.wikipedia.org/wiki/MQTT) over secure websockets to first create, persist, encrypt the connection, and
then enable the features found in Netdata Cloud. _No data is exchanged with Netdata Cloud until you claim a node._

For a guide for claiming a node using the ACLK, plus additional troubleshooting and reference information, read our
[claiming documentation](/claim/README.md) or the [get started with
Cloud](https://learn.netdata.cloud/docs/cloud/get-started) guide.

## Enable and configure the ACLK

The ACLK is enabled by default and automatically configured if the prerequisites installed correctly. You can see this
in the `[global]` section of `var/lib/netdata/cloud.d/cloud.conf`.

```conf
[global]
  enabled = yes
  cloud base url = https://app.netdata.cloud
```

If your Agent needs to use a proxy to access the internet, you must [set up a proxy for
claiming](/claim/README.md#claim-through-a-proxy).

## Disable the ACLK

You have two options if you prefer to disable the ACLK and not use Netdata Cloud.

### Disable at installation

You can pass the `--disable-cloud` parameter to the Agent installation when using a kickstart script
([kickstart.sh](/packaging/installer/methods/kickstart.md) or
[kickstart-static64.sh](/packaging/installer/methods/kickstart-64.md)), or a [manual installation from
Git](/packaging/installer/methods/manual.md).

When you pass this parameter, the installer does not download or compile any extra libraries, and the Agent behaves as
though the ACLK, and thus Netdata Cloud, does not exist. ACLK functionality is available in the Agent but remains fully
inactive.

### Disable at runtime

You can change a runtime setting in your `var/lib/netdata/cloud.d/cloud.conf` file to disable the ACLK. This setting
only stops the Agent from attempting any connection via the ACLK, but does not prevent the installer from downloading
and compiling the ACLK's dependencies.

Change the `enabled` setting to `no`:

```conf
[global]
    enabled = no
```

If the file at `var/lib/netdata/cloud.d/cloud.conf` doesn't exist, you need to create it.

```bash
cd /var/lib/netdata/cloud.d
cat > cloud.conf << EOF
```

Copy the following text into the `cat` prompt and then hit `Enter`:

```conf
[global]
    enabled = no
    cloud base url = https://app.netdata.cloud
EOF
```

You also need to change the file's permissions. Use `grep "run as user" /etc/netdata/netdata.conf` to figure out which
user your Agent runs as, and replace `netdata:netdata` as shown below if necessary:

```bash
chmod 0770 cloud.conf
chown netdata:netdata cloud.conf
```

Restart your Agent to disable the ACLK.

### Re-enable the ACLK

If you first disable the ACLK and any Cloud functionality, and then decide you would like to use Cloud, you must either
reinstall Netdata with Cloud enabled or change the runtime setting in your `cloud.conf` file.

If you passed `--disable-cloud` to `netdata-installer.sh` during installation, you must reinstall your Agent. Use the
same method as before, but pass `--require-cloud` to the installer. When installation finishes you can [claim your
node](/claim/README.md#claim-a-node).

If you changed the runtime setting in your `var/lib/netdata/cloud.d/cloud.conf` file, edit the file again and change
`enabled` to `yes`:

```conf
[global]
    enabled = yes
    cloud base url = https://app.netdata.cloud
```

Restart your Agent and [claim your node](/claim/README.md#claim-a-node).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Faclk%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
