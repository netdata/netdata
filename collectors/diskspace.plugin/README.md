# Monitor disk (diskspace.plugin)

This plugin monitors the disk space usage of mounted disks, under Linux. The plugin requires Netdata to have execute/search permissions on the mount point itself, as well as each component of the absolute path to the mount point.

Two charts are available for every mount:

- Disk Space Usage
- Disk Files (inodes) Usage

## configuration

Simple patterns can be used to exclude mounts from showed statistics based on path or filesystem. By default read-only mounts are not displayed. To display them `yes` should be set for a chart instead of `auto`.

By default, Netdata will enable monitoring metrics only when they are not zero. If they are constantly zero they are ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though).

Netdata will try to detect mounts that are duplicates (i.e. from the same device), or binds, and will not display charts for them, as the device is usually already monitored.

To configure this plugin, you need to edit the configuration file `netdata.conf`. You can do so by using the `edit config` script.  

> ### Info
>
> To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> It is recommended to use this way for configuring Netdata.
>
> Please also note that after most configuration changes you will need to [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for the changes to take effect.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config netdata.conf
```

You can enable the effect of each line by uncommenting it.

You can set `yes` for a chart instead of `auto` to enable it permanently. You can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics for all internal Netdata plugins.

```conf
[plugin:proc:diskspace]
    # remove charts of unmounted disks = yes
    # update every = 1
    # check for new mount points every = 15
    # exclude space metrics on paths = /proc/* /sys/* /var/run/user/* /run/user/* /snap/* /var/lib/docker/*
    # exclude space metrics on filesystems = *gvfs *gluster* *s3fs *ipfs *davfs2 *httpfs *sshfs *gdfs *moosefs fusectl autofs
    # space usage for all disks = auto
    # inodes usage for all disks = auto
```

Charts can be enabled/disabled for every mount separately, just look for the name of the mount after `[plugin:proc:diskspace:`.

```conf
[plugin:proc:diskspace:/]
    # space usage = auto
    # inodes usage = auto
```

> for disks performance monitoring, see the `proc` plugin, [here](https://github.com/netdata/netdata/blob/master/collectors/proc.plugin/README.md#monitoring-disks)
