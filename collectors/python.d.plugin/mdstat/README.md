# mdstat

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
