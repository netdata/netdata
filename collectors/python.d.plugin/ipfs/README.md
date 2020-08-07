<!--
title: "IPFS monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/ipfs/README.md
sidebar_label: "IPFS"
-->

# IPFS monitoring with Netdata

Collects [`IPFS`](https://ipfs.io) basic information like file system bandwidth, peers and repo metrics.

## Charts

It produces the following charts:

-   Bandwidth in `kilobits/s`
-   Peers in `peers`
-   Repo Size in `GiB`
-   Repo Objects in `objects`

## Configuration

Edit the `python.d/ipfs.conf` configuration file using `edit-config` from the your agent's [config
directory](/docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/ipfs.conf
```

---

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

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fipfs%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
