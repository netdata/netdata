# mdstat

> THIS MODULE IS OBSOLETE.
> USE THE [PROC PLUGIN](../../proc.plugin) - IT IS MORE EFFICIENT

---

Module monitor /proc/mdstat

It produces:

1. **Health** Number of failed disks in every array (aggregate chart).

2. **Disks stats**
 * total (number of devices array ideally would have)
 * inuse (number of devices currently are in use)

3. **Current status**
 * resync in percent
 * recovery in percent
 * reshape in percent
 * check in percent

4. **Operation status** (if resync/recovery/reshape/check is active)
 * finish in minutes
 * speed in megabytes/s

### configuration
No configuration is needed.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fmdstat%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
