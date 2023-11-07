# zfs_pool_state_warn

## OS: Any

ZFS is a local file system and logical volume manager created by Sun Microsystems Inc. to direct and
control the placement, storage, and retrieval of data in enterprise-class computing systems. ZFS is 
scalable, suitable for high storage capacities, and includes extensive protection against data corruption.

The Netdata Agent monitors the state of the ZFS pool. Receiving this alert means that the ZFS pool
is degraded.

<details>
<summary>See more on ZFS pool health status</summary>

The ZFS pool health status as described in the Oracle's
website <sup>[1](https://docs.oracle.com/cd/E19253-01/819-5461/gamno/index.html) </sup>

ZFS provides an integrated method of examining pool and device health. The health of a pool is
determined from the state of all its devices. This state information is displayed by using the zpool
status command. In addition, potential pool and device failures are reported by fmd, displayed on
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
1. [AlchemyCS blogspot](https://alchemycs.com/2019/05/how-to-force-zfs-to-replace-a-failed-drive-in-place/)

</details>

### Troubleshooting section

<details>
<summary>Replace a failed drive</summary>

Data is priceless. Before you perform any action, make sure that you have taken any necessary backup
steps. Netdata is not liable for any loss or corruption of any data, database, or software.

Based on a nice guide from the Alchemycs blogspot.

1. Check your zpool status

```
root@netdata # zpool status my_pool
  pool: my_pool
 state: DEGRADED
status: One or more devices could not be used because the label is missing or
        invalid.  Sufficient replicas exist for the pool to continue
        functioning in a degraded state.
action: Replace the device using 'zpool replace'.
   see: http://zfsonlinux.org/msg/ZFS-8000-4J
  scan: scrub repaired 0B in 0h0m with 0 errors on Sun May 12 00:24:51 2019
config:

        NAME                      STATE     READ WRITE CKSUM
        my_pool                   DEGRADED     0     0     0
          mirror-0                DEGRADED     0     0     0
            52009894889112747750  UNAVAIL      0     0     0  was /dev/sdm5
            sdb5                  ONLINE       0     0     0errors: No known data errors
```

1. Find the UUIDs (for GPT) of the faulty dev (the UNAVAIL) and the new disk you want to add.
   Use `blkid` for Linux or  `geom` utility for FreeBSD


1. Offline the UNAVAIL drive
   
```
root@netdata # zpool offline my_pool /dev/disk/by-uuid/{UUID_BAD_DRIVE}
```

1. Replace it in place

```
root@netdata # zpool replace -f my_pool /dev/disk/by-uuid/{UUID_OLD_DRIVE} /dev/disk/by-uuid/{UUID_NEW_DRIVE}
```

</details>
