# ceph

This module monitors the ceph cluster usage and consuption data of a server.

It produces:

* Cluster statistics (usage, available, latency, objects, read/write rate)
* OSD usage
* OSD latency
* Pool usage
* Pool read/write operations
* Pool read/write rate
* number of objects per pool

**Requirements:**

- `rados` python module
- Granting read permissions to ceph group from keyring file
```shell
# chmod 640 /etc/ceph/ceph.client.admin.keyring
```

### Configuration

Sample:
```yaml
local:
  config_file: '/etc/ceph/ceph.conf'
  keyring_file: '/etc/ceph/ceph.client.admin.keyring'
```

---
