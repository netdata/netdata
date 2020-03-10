<!--
---
title: "ISC DHCP monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/isc_dhcpd/README.md
---
-->

# ISC DHCP monitoring with Netdata

Monitors the leases database to show all active leases for given pools.

## Requirements

-   dhcpd leases file MUST BE readable by Netdata
-   pools MUST BE in CIDR format

It produces:

1.  **Pools utilization** Aggregate chart for all pools.

    -   utilization in percent

2.  **Total leases**

    -   leases (overall number of leases for all pools)

3.  **Active leases** for every pools

    -   leases (number of active leases in pool)

## Configuration

Edit the `python.d/isc_dhcpd.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/isc_dhcpd.conf
```

Sample:

```yaml
local:
  leases_path       : '/var/lib/dhcp/dhcpd.leases'
  pools       : '192.168.3.0/24 192.168.4.0/24 192.168.5.0/24'
```

In case of python2 you need to  install `py2-ipaddress` to make plugin work.
The module will not work If no configuration is given.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fisc_dhcpd%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
