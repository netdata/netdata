# Agent-Cloud link (ACLK)

The Agent-Cloud link (ACLK) is the mechanism responsible for securely connecting a Netdata Agent to your web browser
through Netdata Cloud. The ACLK establishes an outgoing secure WebSocket (WSS) connection to Netdata Cloud on port
`443`. The ACLK is encrypted, safe, and _is only established if you connect your node_.

The Cloud App lives at app.netdata.cloud which currently resolves to the following list of IPs: 

- 54.198.178.11
- 44.207.131.212
- 44.196.50.41 

> ### Caution
>
>This list of IPs can change without notice, we strongly advise you to whitelist following domains `api.netdata.cloud`, `mqtt.netdata.cloud`, if this is not an option in your case always verify the current domain resolution (e.g via the `host` command).

For a guide to connecting a node using the ACLK, plus additional troubleshooting and reference information, read our [connect to Cloud
documentation](https://github.com/netdata/netdata/blob/master/claim/README.md).

## Data privacy

[Data privacy](https://netdata.cloud/privacy/) is very important to us. We firmly believe that your data belongs to
you. This is why **we don't store any metric data in Netdata Cloud**.

All the data that you see in the web browser when using Netdata Cloud, is actually streamed directly from the Netdata Agent to the Netdata Cloud dashboard. The data passes through our systems, but it isn't stored.

However, to be able to offer the stunning visualizations and advanced functionality of Netdata Cloud, it does store a limited number of _metadata_. Read more about our [security and privacy design](https://github.com/netdata/netdata/blob/master/docs/netdata-security.md).

## Enable and configure the ACLK

The ACLK is enabled by default, with its settings automatically configured and stored in the Agent's memory. No file is
created at `/var/lib/netdata/cloud.d/cloud.conf` until you either connect a node or create it yourself. The default
configuration uses two settings:

```conf
[global]
  enabled = yes
  cloud base url = https://api.netdata.cloud
```

If your Agent needs to use a proxy to access the internet, you must [set up a proxy for
connecting to cloud](https://github.com/netdata/netdata/blob/master/claim/README.md#connect-through-a-proxy).

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
([kickstart.sh](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md), or a [manual installation from
Git](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/manual.md).

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
    cloud base url = https://api.netdata.cloud
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
[reinstall Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/REINSTALL.md) with Cloud enabled or change the runtime setting in your
`cloud.conf` file.

If you passed `--disable-cloud` to `netdata-installer.sh` during installation, you must
[reinstall](https://github.com/netdata/netdata/blob/master/packaging/installer/REINSTALL.md) your Agent. Use the same method as before, but pass `--require-cloud` to
the installer. When installation finishes you can [connect your node](https://github.com/netdata/netdata/blob/master/claim/README.md#how-to-connect-a-node).

If you changed the runtime setting in your `var/lib/netdata/cloud.d/cloud.conf` file, edit the file again and change
`enabled` to `yes`:

```conf
[global]
    enabled = yes
```

Restart your Agent and [connect your node](https://github.com/netdata/netdata/blob/master/claim/README.md#how-to-connect-a-node).


