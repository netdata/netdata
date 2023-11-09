### Understand the alert

Btrfs is a modern copy on write (CoW) filesystem for Linux aimed at implementing advanced features while also focusing on fault tolerance, repair and easy administration. Btrfs is intended to address the lack of pooling, snapshots, checksums, and integral multi-device spanning in Linux file systems.

Unlike most filesystems, Btrfs allocates disk space in two distinct stages. The first stage allocates chunks of physical disk space for usage by a particular type of filesystem blocks, either data blocks (which store actual file data), metadata blocks (which store inodes and other file metadata), and system blocks (which store metadata about the filesystem itself). The second stage then allocates actual blocks within those chunks for usage by the filesystem. This metric tracks space usage in the first allocation stage. 

The Netdata Agent monitors the percentage of allocated Btrfs physical disk space.

### Troubleshoot the alert

- Add more physical space

Adding a new disk always depends on your infrastructure, disk RAID configuration, encryption, etc. An easy way to add a new disk to a filesystem is:

1. Determine which disk you want to add and in which path
   ```
   btrfs device add -f /dev/<new_disk> <path>
   ```

2. If you get an error that the drive is already mounted, you might have to unmount
   ```
   btrfs device add -f /dev/<new_disk> <path>
   ```
3. See the newly added disk
   ```
   btrfs filesystem show
   ```
4. Balance the system to make use of the new drive.
   ```
   btrfs filesystem balance <path>
   ```

- Delete snapshots

You can identify and delete snapshots that you no longer need.

1. Find the snapshots for a specific path.
   ```
   sudo btrfs subvolume list -s <path>
   ```

2. Delete a snapshot that you do not need any more.
   ```
   btrfs subvolume delete <path>/@some_dir-snapshot-test
   ```

- Enable a compression mechanism

1. Apply compression to existing files. This command will re-compress the  `mount/point` path, with the `zstd` compression algorithm.

    ```
    btrfs filesystem defragment -r -v -czstd /mount/point
    ```

- Enable a deduplication mechanism

Using copy-on-write, Btrfs is able to copy files or whole subvolumes without actually copying the data. However, when a file is altered, a new proper copy is created. Deduplication takes this a step further, by actively identifying blocks of data which share common sequences and combining them into an extent with the same copy-on-write semantics.

Tools dedicated to deduplicate a Btrfs formatted partition include duperemove, bees, and dduper. These projects are 3rd party, and it is strongly suggested that you check their status before you decide to use them.

- Perform a balance

Especially in a Btrfs with multiple disks, there might be unevenly allocated data/metadata into the disks.
```
btrfs balance start -musage=10 -dusage=10 -susage=5 /mount/point
```
This command will attempt to relocate data/metdata/system-data in empty or near-empty chunks (at most X% used, in this example), allowing the space to be reclaimed and reassigned between data and metadata. If the balance command ends with "Done, had to relocate 0 out of XX chunks", then you need to increase the "dusage/musage" percentage parameter until at least some chunks are relocated.

### Useful resources

1. [The Btrfs filesystem on Arch linux website](https://wiki.archlinux.org/title/btrfs)
2. [The Btrfs filesystem on kernel.org website](https://btrfs.wiki.kernel.org)
3. [duperemove](https://github.com/markfasheh/duperemove)
4. [bees](https://github.com/Zygo/bees)
5. [dduper](https://github.com/lakshmipathi/dduper)
