# bcache_cache_dirty

## OS: Any

This alarm presents the percentage of `bcache` cache space used for `dirty` data and `metadata`. If this alarm is
raised, it means that your SSD cache is too small, and overpopulated with said data.

You can view `bcache_cache_dirty` as the `bcache` analogous metric to `dirty memory`. `dirty memory` is memory that has
been changed but has not yet been written out to disk. For example, you make a change to a file but do not save it. These
temporary changes are stored in memory, waiting to be written to disk.
So `dirty` data on `bcache` is data that is stored on the cache disk and waits to be written to the backing device (
Normally your HDD).

> `bcache` is a cache in the block layer of the Linux Kernel. **It allows fast storage devices**, as SSDs
> (Solid State Drives), **to act as a cache for slower storage devices**, such as HDDs (Hard Disk Drives). As a result,
> **hybrid volumes are made with performance improvements**. Generally, a cache device is divided up into `buckets`,
> matching the physical disk's erase blocks.

> `dirty` data is data in the cache that has not been written to the backing device (normally your HDD). So when the
> system shuts down, the cache device and the backing device are not safe to be separated.  
> `metadata` in general, is data that provides information about other data.

Links:  
[kernel.org](https://www.kernel.org/doc/html/latest/admin-guide/bcache.html#)  
[Wikipedia](https://en.wikipedia.org/wiki/Bcache)  
[Bcache](https://wiki.archlinux.org/title/bcache)  
[Bcache: Caching beyond just RAM](https://lwn.net/Articles/394672/)

### Troubleshooting section

<details>
<summary>Upgrade your cache's capacity</summary>

The alarm is raised when there is more than 70% *(for warning status)* of your cache populated by `dirty` data and
`metadata`, it means that your current cache device doesn't have the capacity to support your workflow. Using a bigger
capacity device as cache can solve the problem.

</details>