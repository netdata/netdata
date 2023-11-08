### Understand the alert

This alarm presents the percentage of used `inodes` storage of a particular disk.

The number of `inodes` indicates the number of files and folders you have. An `inode` is a data structure, containing metadata about a file. All filenames are internally mapped to respective `inode` numbers, so if you have a
lot of files, it means there are a lot of `inodes`.

If the alarm is raised, it means that your storage device is running out of `inode` space. Each disk has a particular **limitation on the amount of `inodes` it can store**, determined by its size.

Many modern filesystems use dynamically allocated `inodes` instead of a static table. These should not be presented on the charts associated with this alarm, and should not ever trigger it. If such a filesystem **does** trigger this alarm, and it's constantly reporting max `inode` usage, it's probably a bug in the filesystem driver. Some such filesystems incorrectly report having max `inode` count when they should not because they have no max limit, and in turn they trigger a false positive alarm.

### Troubleshoot the alert

Clear cache files or delete unnecessary files and folders

- To reduce the amount of how many `inodes` you store currently, you can clear your cache, trash any unnecessary files and folders in your system.

We strongly suggest that you practice a high degree of caution when cleaning up drives, and removing files, make sure that you are certain that you delete only unnecessary files.

### Useful resources

[Linux Inodes](https://www.javatpoint.com/linux-inodes)  
[Understanding UNIX / Linux filesystem Inodes](https://www.cyberciti.biz/tips/understanding-unixlinux-filesystem-inodes.html)