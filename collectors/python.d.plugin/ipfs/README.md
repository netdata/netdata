<!--
title: "IPFS monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/ipfs/README.md"
sidebar_label: "IPFS"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Storage"
-->

# IPFS collector

Collects [`IPFS`](https://ipfs.io) basic information like file system bandwidth, peers and repo metrics.

## Charts

It produces the following charts:

-   Bandwidth in `kilobits/s`
-   Peers in `peers`
-   Repo Size in `GiB`
-   Repo Objects in `objects`

## Configuration

Edit the `python.d/ipfs.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/ipfs.conf
```



Calls to the following endpoints are disabled due to `IPFS` bugs:

-   `/api/v0/stats/repo` (https://github.com/ipfs/go-ipfs/issues/3874)
-   `/api/v0/pin/ls` (https://github.com/ipfs/go-ipfs/issues/7528)

Can be enabled in the collector configuration file.

The configuration needs only `url` to `IPFS` server, here is an example for 2 `IPFS` instances:

```yaml
localhost:
  url: 'http://localhost:5001'

remote:
  url: 'http://203.0.113.10::5001'
```




### Troubleshooting

To troubleshoot issues with the `ipfs` module, run the `python.d.plugin` with the debug option enabled. The 
output will give you the output of the data collection job or error messages on why the collector isn't working.

First, navigate to your plugins directory, usually they are located under `/usr/libexec/netdata/plugins.d/`. If that's 
not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the 
plugin's directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

Now you can manually run the `ipfs` module in debug mode:

```bash
./python.d.plugin ipfs debug trace
```

