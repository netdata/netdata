### Understand the alert

The `btrfs_metadata` alert calculates the percentage of used Btrfs metadata space for a Btrfs filesystem. If you receive this alert, it indicates that your Btrfs filesystem's metadata space is being utilized at a high rate.

### Troubleshoot the alert

**Warning: Data is valuable. Before performing any actions, make sure to take necessary backup steps. Netdata is not responsible for any loss or corruption of data, database, or software.**

1. **Add more physical space**

   - Determine which disk you want to add and in which path:
     ```
     root@netdata~ # btrfs device add -f /dev/<new_disk> <path>
     ```

   - If you get an error that the drive is already mounted, you might have to unmount:
     ```
     root@netdata~ # btrfs device add -f /dev/<new_disk> <path>
     ```

   - Check the newly added disk:
     ```
     root@netdata~ # btrfs filesystem show
     ```

   - Balance the system to make use of the new drive:
     ```
     root@netdata~ # btrfs filesystem balance <path>
     ```

2. **Delete snapshots**

   - List the snapshots for a specific path:
     ```
     root@netdata~ # sudo btrfs subvolume list -s <path>
     ```

   - Delete an unnecessary snapshot:
     ```
     root@netdata~ # btrfs subvolume delete <path>/@some_dir-snapshot-test
     ```

3. **Enable a compression mechanism**

   Apply compression to existing files by modifying the `fstab` configuration file (or during the `mount` procedure) with the `compress=alg` option. Replace `alg` with `zlib`, `lzo`, `zstd`, or `no` (for no compression). For example, to re-compress the `/mount/point` path with `zstd` compression:
   
   ```
   root@netdata # btrfs filesystem defragment -r -v -czstd /mount/point
   ```

4. **Enable a deduplication mechanism**

   Deduplication tools like duperemove, bees, and dduper can help identify blocks of data sharing common sequences and combine extents via copy-on-write semantics. Ensure you check the status of these 3rd party tools before using them.

   - [duperemove](https://github.com/markfasheh/duperemove)
   - [bees](https://github.com/Zygo/bees)
   - [dduper](https://github.com/lakshmipathi/dduper)

5. **Perform a balance**

   Balance data/metadata/system-data in empty or near-empty chunks for Btrfs filesystems with multiple disks, allowing space to be reassigned:

   ```
   root@netdata # btrfs balance start -musage=50 -dusage=10 -susage=5 /mount/point
   ```

### Useful resources

1. [The Btrfs filesystem on Arch Linux website](https://wiki.archlinux.org/title/btrfs)
2. [The Btrfs filesystem on kernel.org website](https://btrfs.wiki.kernel.org)