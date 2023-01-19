<!--
title: "Agent-cloud link"
sidebar_label: "Agent-cloud link"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md"
sidebar_position: "1400"
learn_status: "Unpublished"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata agent"
learn_docs_purpose: "Explain what the ACLK is"
-->

### Claim process

Claim is the process where you initialize a request from an Agent to start monitoring it from your Netdata Cloud 
environment. To achieve this, the Agent needs to be aware of some information (specific) for your space such as the 
`NETDATA_CLAIM_TOKEN` of your space, the domain under the Netdata Cloud lives `NETDATA_CLAIM_URL` and optional, the 
rooms in the particular space you want the node to be included `NETDATA_CLAIM_ROOMS` and/or any proxy endpoint
`NETDATA_CLAIM_PROXY` to connect through. The setup process is completed by running the claiming process via the 
Netdata Agent's command line, or the netdata-claim script or directly from the kickstart.sh script. On success; all the 
necessary information to securely connect your Netdata Agent to the cloud end to end, are stored under your 
`<NETDATA_PREFIX>/var/lib/netdata/cloud.d` directory  (default: `/var/lib/netdata/cloud.d`)

### ACLK

The Agent-Cloud link (ACLK) is the mechanism responsible for secure communication between the Netdata Agent and the
Netdata Cloud. ACLK is active by default but idle, after a successful [claim process](#claim-process) you expect to see
a configuration under your `<NETDATA_PREFIX>/var/lib/netdata/cloud.d/cloud.conf` like the following


```conf
[global]
  enabled = yes
  cloud base url = https://app.netdata.cloud
```

You can disable it at any given moment this component if you need to either on installation process or during runtime. 

The Cloud App lives at app.netdata.cloud which currently resolves to the following list of IPs:

- 54.198.178.11
- 44.207.131.212
- 44.196.50.41

As such the Agent needs to be able to access these IPs.

:::caution

This list of IPs can change without notice, we strongly advise you to whitelist following domains `api.netdata.cloud`, 
`mqtt.netdata.cloud`, if this is not an option in your case always verify the current domain resolution 
(e.g via the host command).

:::

The ACLK establishes an outgoing secure WebSocket (WSS) connection to Netdata Cloud on port `443`. The ACLK is encrypted,
 and is only established if you successfully [claim](#claim-process) your node. Through the ACLK the Agent push
information about:

1. Metadata, of what metrics the Agent monitors.
2. Alert status transitions.

Only when you are trying to see the actual charts/dashboards from your Netdata Cloud, the Cloud queries the Agent's data 
(again though the ACLK) so you can see the actual metrics in your browser for a timeframe you specified.


<!-- TODO: Make the following sections tasks

## Enable and configure the ACLK

The ACLK is enabled by default, with its settings automatically configured and stored in the Agent's memory. No file is
created at `/var/lib/netdata/cloud.d/cloud.conf` until you either connect a node or create it yourself. The default
configuration uses two settings:



If your Agent needs to use a proxy to access the internet, you
must [set up a proxy for connecting to cloud](/claim/README.md#connect-through-a-proxy).

You can configure following keys in the `netdata.conf` section `[cloud]`:

```
[cloud]
  statistics = yes
  query thread count = 2
  mqtt5 = yes
```

- `statistics` enables/disables ACLK related statistics and their charts. You can disable this to save some space in the
  database and slightly reduce memory usage of Netdata Agent.
- `query thread count` specifies the number of threads to process cloud queries. Increasing this setting is useful for
  nodes with many children (streaming), which can expect to handle more queries (and/or more complicated queries).
- `mqtt5` allows disabling the new MQTT5 implementation which is used now by default in case of issues. This option will
  be removed in future stable release.

## Disable the ACLK

You have two options if you prefer to disable the ACLK and not use Netdata Cloud. The following subsections provide
expalantion and instruction for these options.

### Disable at installation

You can pass the `--disable-cloud` parameter to the Agent installation when using a kickstart script
([kickstart.sh](/packaging/installer/methods/kickstart.md), or
a [manual installation from Git](/packaging/installer/methods/manual.md).

When you pass this parameter, the installer does not download or compile any extra libraries. Once running, the Agent
kills the thread responsible for the ACLK and connecting behavior, and behaves as though the ACLK, and thus Netdata
Cloud, does not exist.

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

Copy and paste in lines 3-6, and after the final `EOF`, hit **Enter**. The final line must contain only `EOF`. Hit **
Enter** again to return to your normal prompt with the newly-created file.

To get your normal prompt back, the final line must contain only `EOF`.

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

-->
