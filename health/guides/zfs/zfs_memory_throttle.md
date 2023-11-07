# zfs_memory_throttle

## OS: Linux | FreeBSD

This alert presents the number of times ZFS had to limit the Adaptive Replacement Cache (ARC) growth in the last 10 minutes.

The alert is raised to a warning state when the metric starts counting (when it is greater than 0).

<details>
<summary>Linux: What is the ARC?</summary>

The ARC stores the most recently used, and most frequently used data within RAM.  
Having a large ARC can take up a lot of RAM, but it will decrease as other
applications need it. ARC can be set to customized optimal settings for your system.

ARC uses varying portions of the most recently used and the most often used data by
allocating more space to one or the other whenever a cold hit occurs. A cold hit occurs when
some data is requested that was previously cached, but has already been pushed out to allow
the ARC to store new data. ZFS keeps track of what data was stored in the cache after it is
removed in order to enable the recognition of cold hits. As new data comes in, data that
hasn't been used in a while, or that has not been used as much as the new data, will be pushed
out.

The more RAM your system has the better, as it will just give you enhanced read performance. There
will be physical and cost limitations to adding more ARC due to motherboard RAM slots and budget
constraints.<sup>[1](https://www.45drives.com/community/articles/zfs-caching/) </sup>

</details>

<br>

<details>
<summary>FreeBSD: What is the ARC?</summary>  

The ARC functions by storing the most recently used, and most frequently used data within RAM.  
Having a large ARC can take up a lot of RAM, but it will give it up as other applications need
it and can be set to whatever you think is optimal for your system.

**Single Copy ARC**
OpenZFS caches disk blocks in-memory in the adaptive replacement cache (ARC). Originally when
the same disk block was accessed from different clones it was cached multiple times (one for
each clone accessing the block) in case a clone planned to modify the block. OpenZFS caches
at most one copy of every block unless a clone is actually modifying the block.

**ARC Shouldn't Cache Freed Blocks**
Originally cached blocks in the ARC remained cached until they were evicted due to memory
pressure, even if the underlying disk block was freed. In some workloads these freed blocks
were so frequently accessed before they were freed that the ARC continued to cache them while
evicting blocks which had not been freed yet. Since freed blocks could never be accessed
again continuing to cache them was unnecessary. In OpenZFS ARC blocks are evicted immediately
when their underlying data blocks are freed.<sup>[2](https://openzfs.org/w/index.php?title=Features&mobileaction=toggle_view_mobile#Single_Copy_ARC)  
</sup>

</details>

<br>

<details>
<summary>References and Sources</summary>

1.  [Linux: ZFS Caching](https://www.45drives.com/community/articles/zfs-caching/)
2.  [FreeBSD: OpenZFS documentation](https://openzfs.org/w/index.php?title=Features&mobileaction=toggle_view_mobile#Single_Copy_ARC)
    </details>

### Troubleshooting Section

<details>
<summary>Linux | FreeBSD: Increase your RAM capacity and effectively increase ARC size</summary>

ZFS will throttle the ARC growth as the system needs more RAM for other tasks. 
If you are experiencing a lot of throttling, then you should consider increasing your RAM capacity.  
If the ARC size needs to be limited, then the read performance of the system will drop and cold hits
are more likely to happen.

</details>
