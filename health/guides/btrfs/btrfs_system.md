# btrfs_system

## OS: Any

*Btrfs is a modern copy on write (CoW) filesystem for Linux aimed at implementing advanced features while also focusing
on fault tolerance, repair and easy administration. Btrfs is intended to address the lack of pooling, snapshots,
checksums, and integral multi-device spanning in Linux file systems.* [[1]](https://wiki.archlinux.org/title/btrfs)

Unlike most filesystems, Btrfs allocates disk space in two distinct stages. The first stage allocates chunks of physical
disk space for usage by a particular type of filesystem blocks, either data blocks (which store actual file data),
metadata blocks (which store inodes and other file metadata), and system blocks (which store metadata about the
filesystem itself). The second stage then allocates actual blocks within those chunks for usage by the filesystem. This
metric tracks space usage in the first allocation stage.
The Netdata Agent monitors the percentage of used Btrfs system space.

<details>
<summary>Subvolumes in Btrfs </summary>

> A Btrfs subvolume is an independently mountable POSIX filetree and not a block device (and cannot be treated as one).
Most other POSIX filesystems have a single mountable root. Btrfs has an independent mountable root for the volume (top
level subvolume) and for each subvolume. A Btrfs volume can contain more than a single filetree; it can contain a forest
of filetrees. A Btrfs subvolume can be thought of as a POSIX file namespace.
>
> A subvolume in Btrfs is not the same as a LVM logical volume or a ZFS subvolume. With LVM, a logical volume is a block
device in its own right, which could, for example, contain any other filesystem or container like dm-crypt, MD RAID,
etc. This is not the case with Btrfs.
>
> A Btrfs subvolume root directory differs from a directory in that each subvolume defines a distinct inode number space
(distinct inodes in different subvolumes can have the same inumber) and each inode under a subvolume has a distinct
device number (as reported by stat(2)). Each subvolume root can be accessed as implicitly mounted via the volume (top
level subvolume) root, if that is mounted, or it can be mounted in its own right. [[2]](https://btrfs.wiki.kernel.org)


</details>

<details>
<summary>Snapshots in Btrfs</summary>

> A snapshot is a subvolume that shares its data (and metadata) with some other subvolume, using Btrfs's COW 
> capabilities.
>
> Once a [writable] snapshot is made, there is no difference in status between the original subvolume, and the new
snapshot subvolume. To roll back to a snapshot, unmount the modified original subvolume, use mv to rename the old
subvolume to a temporary location, and then rename the snapshot to the original name. You can then remount the
subvolume.
>
> At this point, the original subvolume may be deleted, if desired. Since a snapshot is a subvolume, snapshots of
snapshots are also possible.
>
> Caution: Care must be taken when snapshots are created that are then visible to any user (e.g. when they're created
> in a nested layout) as this may have security implications. Of course, the snapshot will have the same permissions
> as the subvolume from which it was created at the time it was, but these permissions may be tightened later on, while
> those of the snapshot wouldn't change, possibly allowing access to files that shouldn't be accessible anymore.
> Similarly, especially on the system's "main" filesystem, the snapshot would contain any files (for example, setuid
> programs) of the state when it was created. In the meantime however, security updates may have been rolled out on
> the original subvolume, but when the snapshot is accessible (and for example the vulnerable setuid has been accessible
> before) a user could still invoke it. [[2]](https://btrfs.wiki.kernel.org)

</details>

<details>
<summary>Useful commands for btrfs tool</summary>

You can see some commands from the man pages of Btrfs in [Ubuntu man pages (Bionic)](https://manpages.ubuntu.com/manpages/bionic/man8)

- `btrfs subvolume` is used to create/delete/list/show btrfs subvolumes and snapshots. For example,
  with `btrfs subvolume list <path>`, you can list the subvolumes present in the filesystem `<path>`.


- `btrfs filesystem` is used to perform several whole filesystem level tasks, including all the regular filesystem
  operations like resizing, space stats, label setting/getting, and defragmentation. For example
  with `btrfs filesystem show  [<path>|<uuid>|<device>|<label>]` you can see the Btrfs filesystem with some additional
  info about devices and space allocation.


- `btrfs balance` can balance (restripe) the allocated extents across all of the existing devices. The primary purpose
  of the balance feature is to spread block groups across all devices so they match constraints defined by the
  respective profiles. The balance operation is cancellable by the user. The on-disk state of the filesystem is always
  consistent so an unexpected interruption (eg. system crash, reboot) does not corrupt the filesystem. The progress of
  the balance operation is temporarily stored as an internal state and will be resumed upon mount, unless the mount
  option skip_balance is specified. **Running balance without filters will take a lot of time as it basically rewrites
  the entire filesystem and needs to update all block pointers**.


- `btrfs device` command group is used to manage devices of the btrfs filesystems. For example, with `btrfs device add`
  command, you can add new devices to a mounted filesystem.


- `btrfs rescue` is used to try to recover a damaged btrfs filesystem.


- `btrfs scrub <subcommand>` scrub command attempts to report and repair bad blocks on Btrfs file systems.
  **Scrubbing is performed in the background by default**.

</details>


<details>
<summary>References and sources:</summary>

1. [The Btrfs filesystem on Arch linux website](https://wiki.archlinux.org/title/btrfs)
1. [The Btrfs filesystem on kernel.org website](https://btrfs.wiki.kernel.org)


</details>

### Troubleshooting section:

Data is priceless. Before you perform any action, make sure that you have taken any necessary backup steps. Netdata is
not liable for any loss or corruption of any data, database, or software.

<details>
<summary>Add more physical space</summary>

Adding a new disk always depends on your infrastructure, disk RAID configuration, encryption, etc. An easy way to add a
new disk to a filesystem is:

1. Determine which disk you want to add and in which path
   ```
   root@netdata~ # btrfs device add -f /dev/<new_disk> <path>
   ```

1. If you get an error that the drive is already mounted, you might have to unmount
   ```
   root@netdata~ # btrfs device add -f /dev/<new_disk> <path>
   ```
1. See the newly added disk
   ```
   root@netdata~ # btrfs filesystem show
   Label: none  uuid: d6b9d7bc-5978-2677-ac2e-0e68204b2c7b
	Total devices 2 FS bytes used 192.00KiB
	devid    1 size 10.01GiB used 536.00MiB path /dev/sda1
	devid    2 size 10.01GiB used 0.00B path /dev/sdb
   ```

1. Balance the system to make use of the new drive.
   ```
   root@netdata~ # btrfs filesystem balance <path>
   ```

</details>

<details>
<summary>Delete snapshots</summary>

You can identify and delete snapshots that you no longer need.

1. Find the snapshots for a specific path.
   ```
   root@netdata~ # sudo btrfs subvolume list -s <path>
   ```

1. Delete a snapshot that you do not need any more.
   ```
   root@netdata~ # btrfs subvolume delete <path>/@some_dir-snapshot-test
   ```

</details>

<details>
<summary>Enable a compression mechanism</summary>

> The `compress=alg` mount option into the `fstab` configuration file (or in the `mount` procedure) enables 
automatically considering every file for compression, where `alg` is either `zlib`, `lzo`, `zstd`, or `no` (for no 
compression). Using this option, Btrfs will check if compressing the first portion of the data shrinks it. If it does,
the entire write to
that file will be compressed. If it does not, none of it is compressed. With this option, if the first portion of the
write does not shrink, no compression will be applied to the write even if the rest of the data would shrink
tremendously. This is done to prevent making the disk wait to start writing until all of the data to be written is fully
-given to Btrfs and compressed. [[1]](https://wiki.archlinux.org/title/btrfs)

1. Apply compression to existing files. This command will re-compress the  `mount/point` path, with the `zstd`
   compression algorithm.

    ```
    root@netdata # btrfs filesystem defragment -r -v -czstd /mount/point
    ```

</details>

<details>
<summary>Enable a deduplication mechanism</summary>

Using copy-on-write, Btrfs is able to copy files or whole subvolumes without actually copying the data. However, when a
file is altered, a new proper copy is created. Deduplication takes this a step further, by actively identifying blocks
of data which share common sequences and combining them into an extent with the same copy-on-write semantics.

Tools dedicated to deduplicate a Btrfs formatted partition include duperemove, bees, and dduper. These projects are 3rd
party, and it is strongly suggested that you check their status before you decide to use them.

- [duperemove](https://github.com/markfasheh/duperemove)
- [bees](https://github.com/Zygo/bees)
- [dduper](https://github.com/lakshmipathi/dduper)

</details>

<details>
<summary>Perform a balance</summary>

Especially in a Btrfs with multiple disks, data/metadata might be unevenly allocated into the disks.

```
root@netdata # btrfs balance start -musage=10 -dusage=10 -susage=50 /mount/point
```

> This command will attempt to relocate data/metdata/system-data in empty or near-empty chunks (at most X% used, in this
> example), allowing the space to be reclaimed and reassigned between data and metadata. If the balance command ends
> with "Done, had to relocate 0 out of XX chunks", then you need to increase the "dusage/musage" percentage parameter
> until at least some chunks are relocated.

</details>
