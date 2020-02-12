# ceph

This module monitors the ceph cluster usage and consumption data of a server.

It produces:

-   Cluster statistics (usage, available, latency, objects, read/write rate)
-   OSD usage
-   OSD latency
-   Pool usage
-   Pool read/write operations
-   Pool read/write rate
-   number of objects per pool

**Requirements:**

-   `rados` python module
-   Granting read permissions to ceph group from keyring file

```shell
# chmod 640 /etc/ceph/ceph.client.admin.keyring
```

## Configuration

Edit the `python.d/ceph.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/ceph.conf
```

Sample:

```yaml
local:
  config_file: '/etc/ceph/ceph.conf'
  keyring_file: '/etc/ceph/ceph.client.admin.keyring'
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fceph%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
