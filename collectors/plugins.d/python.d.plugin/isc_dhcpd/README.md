# isc_dhcpd

Module monitor leases database to show all active leases for given pools.

**Requirements:**
 * dhcpd leases file MUST BE readable by netdata
 * pools MUST BE in CIDR format

It produces:

1. **Pools utilization** Aggregate chart for all pools.
 * utilization in percent

2. **Total leases**
 * leases (overall number of leases for all pools)

3. **Active leases** for every pools
  * leases (number of active leases in pool)


### configuration

Sample:

```yaml
local:
  leases_path       : '/var/lib/dhcp/dhcpd.leases'
  pools       : '192.168.3.0/24 192.168.4.0/24 192.168.5.0/24'
```

In case of python2 you need to  install `py2-ipaddress` to make plugin work.
The module will not work If no configuration is given.

---
