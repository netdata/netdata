# bcache_cache_errors

## OS: Any

This alarm presents the number of `bcache` read races in the last minute. The `bucket` was reused and invalidated while
reading from the cache. When this occurs, the data is reread from the backing device.

> `bcache` is a cache in the block layer of the Linux Kernel. **It allows fast storage devices**, as SSDs
> (Solid State Drives), **to act as a cache for slower storage devices**, such as HDDs (Hard Disk Drives). As a result,
> **hybrid volumes are made with performance improvements**. Generally, a cache device is divided up into `buckets`,
> matching the physical disk's erase blocks.

There is a mechanism where `bcache` can keep the cache disk full (typically your SSD), and when it needs to write more
data, it selects a `bucket`, invalidates it, and removes all pointers from it. **The alarm got triggered, because while
there was a reading operation from the cache** *(meaning the data is stored inside a bucket)* **that bucket got
invalidated so the read operation couldn't be completed.** Following up, the data is reread from the backing device
(normally your HDD).

Links:  
[kernel.org](https://www.kernel.org/doc/html/latest/admin-guide/bcache.html#)  
[Wikipedia](https://en.wikipedia.org/wiki/Bcache)  
[Bcache](https://wiki.archlinux.org/title/bcache)  
[Bcache: Caching beyond just RAM](https://lwn.net/Articles/394672/)