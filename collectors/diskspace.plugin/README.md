# diskspace.plugin

This plugin monitors the disk space usage of mounted disks, under Linux.

Two charts are available for every mount:
 - Disk Space Usage
 - Disk Files (inodes) Usage

## configuration

Simple patterns can be used to exclude mounts from showed statistics based on path or filesystem. By default read-only mounts are not displayed. To display them `yes` should be set for a chart instead of `auto`.

```
[plugin:proc:diskspace]
    # remove charts of unmounted disks = yes
    # update every = 1
    # check for new mount points every = 15
    # exclude space metrics on paths = /proc/* /sys/* /var/run/user/* /run/user/* /snap/* /var/lib/docker/*
    # exclude space metrics on filesystems = *gvfs *gluster* *s3fs *ipfs *davfs2 *httpfs *sshfs *gdfs *moosefs fusectl
    # space usage for all disks = auto
    # inodes usage for all disks = auto
```

Charts can be enabled/disabled for every mount separately:

```
[plugin:proc:diskspace:/]
    # space usage = auto
    # inodes usage = auto
```

> for disks performance monitoring, see the `proc` plugin, [here](../proc.plugin/#monitoring-disks)


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fdiskspace.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
