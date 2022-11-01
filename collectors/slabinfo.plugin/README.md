<!--
title: "slabinfo.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/slabinfo.plugin/README.md
-->

# slabinfo.plugin

SLAB is a cache mechanism used by the Kernel to avoid fragmentation.

Each internal structure (process, file descriptor, inode...) is stored within a SLAB.

## configuring Netdata for slabinfo

The plugin is disabled by default because it collects and displays a huge amount of metrics.
To enable it set `slabinfo = yes` in the `plugins` section of the `netdata.conf` configuration file.

There is currently no configuration needed for the plugin itself.

As `/proc/slabinfo` is only readable by root, this plugin is setuid root.

## For what use

This slabinfo details allows to have clues on actions done on your system.
In the following screenshot, you can clearly see a `find` done on a ext4 filesystem (the number of `ext4_inode_cache` & `dentry` are rising fast), and a few seconds later, an admin issued a `echo 3 > /proc/sys/vm/drop_cached` as their count dropped.

![netdata_slabinfo](https://user-images.githubusercontent.com/9157986/64433811-7f06e500-d0bf-11e9-8e1e-087497e61033.png)



