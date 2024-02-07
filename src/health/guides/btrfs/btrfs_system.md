### Understand the alert

The `btrfs_system` alert monitors the percentage of used Btrfs system space. If you receive this alert, it means that your Btrfs system space usage has reached a critical level and could potentially cause issues on your system.

### Troubleshoot the alert

**Important**: Data is priceless. Before you perform any action, make sure that you have taken any necessary backup steps. Netdata is not liable for any loss or corruption of any data, database, or software.

1. Add more physical space

   Adding a new disk always depends on your infrastructure, disk RAID configuration, encryption, etc. To add a new disk to a filesystem:

   - Determine which disk you want to add and in which path:
     ```
     root@netdata~ # btrfs device add -f /dev/<new_disk> <path>
     ```
   - If you get an error that the drive is already mounted, you might have to unmount:
     ```
     root@netdata~ # btrfs device add -f /dev/<new_disk> <path>
     ```
   - See the newly added disk:
     ```
     root@netdata~ # btrfs filesystem show
     Label: none  uuid: d6b9d7bc-5978-2677-ac2e-0e68204b2c7b
       Total devices 2 FS bytes used 192.00KiB
       devid    1 size 10.01GiB used 536.00MiB path /dev/sda1
       devid    2 size 10.01GiB used 0.00B path /dev/sdb
     ```
   - Balance the system to make use of the new drive:
     ```
     root@netdata~ # btrfs filesystem balance <path>
     ```

2. Delete snapshots

   You can identify and delete snapshots that you no longer need.

   - Find the snapshots for a specific path:
     ```
     root@netdata~ # sudo btrfs subvolume list -s <path>
     ```
   - Delete a snapshot that you do not need any more:
     ```
     root@netdata~ # btrfs subvolume delete <path>/@some_dir-snapshot-test
     ```

3. Enable a compression mechanism

   - Apply compression to existing files. This command will re-compress the `mount/point` path, with the `zstd` compression algorithm:
     ```
     root@netdata # btrfs filesystem defragment -r -v -czstd /mount/point
     ```

4. Enable a deduplication mechanism

   Tools dedicated to deduplicate a Btrfs formatted partition include duperemove, bees, and dduper. These projects are 3rd party, and it is strongly suggested that you check their status before you decide to use them.

   - [duperemove](https://github.com/markfasheh/duperemove)
   - [bees](https://github.com/Zygo/bees)
   - [dduper](https://github.com/lakshmipathi/dduper)

5. Perform a balance

   Especially in a Btrfs with multiple disks, data/metadata might be unevenly allocated into the disks.

   ```
   root@netdata # btrfs balance start -musage=10 -dusage=10 -susage=50 /mount/point
   ```

   > This command will attempt to relocate data/metdata/system-data in empty or near-empty chunks (at most X% used, in this example), allowing the space to be reclaimed and reassigned between data and metadata. If the balance command ends with "Done, had to relocate 0 out of XX chunks", then you need to increase the "dusage/musage" percentage parameter until at least some chunks are relocated.

### Useful resources

1. [The Btrfs filesystem on Arch Linux website](https://wiki.archlinux.org/title/btrfs)
2. [The Btrfs filesystem on kernel.org website](https://btrfs.wiki.kernel.org)