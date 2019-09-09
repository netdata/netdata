# slabinfo.plugin

SLAB is a cache mechanism used by the Kernel to avoid fragmentation.

Each internal structure (process, file descriptor, inode...) is stored within a SLAB.


## configuring Netdata for slabinfo

There is currently no configuration needed, aside to have the `slabinfo.plugin` plugin being able to read 
`/proc/slabinfo`. As this procfile is only readable by root, (mode `0400` in the kernel source), we need to grant the 
slabinfo.plugin this access, either by:
-  with Setuid root: `chown root: slabinfo.plugin && chmod u+s slabinfo.plugin`
-  with capability dac_override: `setcap cap_dac_override+ep slabinfo.plugin`


## For what use

This slabinfo details allows to have clues on actions done on your system.
In the following screenshot, you can clearly see a `find` done on a ext4 filesystem (the number of `ext4_inode_cache` & `dentry` are rising fast), and a few seconds later, an admin issued a `echo 3 > /proc/sys/vm/drop_cached` as their count dropped.

![netdata_slabinfo](https://user-images.githubusercontent.com/9157986/64433811-7f06e500-d0bf-11e9-8e1e-087497e61033.png)


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fslabinfo.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
