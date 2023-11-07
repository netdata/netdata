# zfs_pool_state_crit

## OS: Any

ZFS is a local file system and logical volume manager created by Sun Microsystems Inc. to direct and
control the placement, storage, and retrieval of data in enterprise-class computing systems. ZFS is 
scalable, suitable for high storage capacities, and includes extensive protection against data corruption.

The Netdata Agent monitors the state of the ZFS pool. Receiving this alert means that the ZFS pool
is faulted or unavailable.

<details>
<summary>ZFS pool health status</summary>

The ZFS pool health status as described in the Oracle's
website <sup>[1](https://docs.oracle.com/cd/E19253-01/819-5461/gamno/index.html) </sup>

ZFS provides an integrated method of examining pool and device health. The health of a pool is
determined from the state of all its devices. This state information is displayed by using the `zpool
status` command. In addition, potential pool and device failures are reported by fmd, displayed on
the system console, and logged in the /var/adm/messages file.

Each device can fall into one of the following states:

- ONLINE, the device or virtual device is in normal working order. Although some transient errors
  might still occur, the device is otherwise in working order.

- DEGRADED, the virtual device has experienced a failure but can still function. This state is most
  common when a mirror or RAID-Z device has lost one or more constituent devices. The fault
  tolerance of the pool might be compromised, as a subsequent fault in another device might be
  unrecoverable.

- FAULTED, the device or virtual device is completely inaccessible. This status typically indicates
  total failure of the device, such that ZFS is incapable of sending data to it or receiving data
  from it. If a top-level virtual device is in this state, then the pool is completely inaccessible.

- OFFLINE, the device has been explicitly taken offline by the administrator.

- UNAVAIL, the device or virtual device cannot be opened. In some cases, pools with UNAVAIL devices
  appear in DEGRADED mode. If a top-level virtual device is UNAVAIL, then nothing in the pool can be
  accessed.

- REMOVED, the device was physically removed while the system was running. Device removal detection
  is hardware-dependent and might not be supported on all platforms.

The health of a pool is determined from the health of all its top-level virtual devices. If all
virtual devices are ONLINE, then the pool is also ONLINE. If any one of the virtual devices is
DEGRADED or UNAVAIL, then the pool is also DEGRADED. If a top-level virtual device is FAULTED or
OFFLINE, then the pool is also FAULTED. A pool in the FAULTED state is completely inaccessible. No
data can be recovered until the necessary devices are attached or repaired. A pool in the DEGRADED
state continues to run, but you might not achieve the same level of data redundancy or data
throughput than if the pool were online. <sup>[1](https://docs.oracle.com/cd/E19253-01/819-5461/gamno/index.html) </sup>

</details>

<details>
<summary>References and source</summary>

1. [Determining the Health Status of ZFS Storage Pools](https://docs.oracle.com/cd/E19253-01/819-5461/gamno/index.html)
1. [Chapter 11, Oracle Solaris ZFS Troubleshooting and Pool Recovery](https://docs.oracle.com/cd/E53394_01/html/E54801/gavwg.html)
1. [ZFS on FreeBSD documentation](https://docs.freebsd.org/en/books/handbook/zfs/)
1. [OpenZFS documentation](https://openzfs.github.io/openzfs-docs/)

</details>

### Troubleshooting section

<details>
<summary>Migrate ZFS Storage Pools</summary>

If the state of the ZFS pool is UNAVAIL, then you should consider migrating your ZFS pool. To do so follow the
workflow at the [official troubleshooting section in Oracle's website](https://docs.oracle.com/cd/E53394_01/html/E54801/gbchy.html#scrolltoc).

In this workflow, there are no notable changes in the commands for FreeBSD.
<sup>[3](https://docs.freebsd.org/en/books/handbook/zfs/) </sup> or ZFS on
linux <sup>[4](https://openzfs.github.io/openzfs-docs/) </sup>, for completeness you can refer to
individual guides.

</details>
