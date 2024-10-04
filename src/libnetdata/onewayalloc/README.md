# One Way Allocator

This is a very fast single-threaded-only memory allocator, that minimized system calls
when a lot of memory allocations needs to be made to perform a task, which all of them
can be freed together when the task finishes.

It has been designed to be used for netdata context queries.

For netdata to perform a context query, it builds a virtual chart, a chart that contains
all the dimensions of the charts having the same context. This process requires allocating
several structures for each of the dimensions to attach them to the virtual chart. All
these data can be freed immediately after the query finishes.

## How it works

1. The caller calls `ONEWAYALLOC *owa = onewayalloc_create(sizehint)` to create an OWA.
   Internally this allocates the first memory buffer with size >= `sizehint`.
   If `sizehint` is zero, it will allocate 1 hardware page (usually 4kb).
   No need to check for success or failure. As with `mallocz()` in netdata, a `fatal()`
   will be called if the allocation fails - although this will never fail, since Linux
   does not really check if there is memory available for `mmap()` calls.
   
2. The caller can then perform any number of the following calls to acquire memory:
   - `onewayalloc_mallocz(owa, size)`, similar to `mallocz()`
   - `onewayalloc_callocz(owa, nmemb, size)`, similar to `callocz()`
   - `onewayalloc_strdupz(owa, string)`, similar to `strdupz()`
   - `onewayalloc_memdupz(owa, ptr, size)`, similar to `mallocz()` and then `memcpy()`
   
3. Once the caller has done all the work with the allocated buffers, all memory allocated 
   can be freed with `onewayalloc_destroy(owa)`.

## How faster it is?

On modern hardware, for any single query the performance improvement is marginal and not
noticeable at all.

We performed the following tests using the same huge context query (1000 charts,
100 dimensions each = 100k dimensions)

1. using `mallocz()`, 1 caller, 256 queries (sequential)
2. using `mallocz()`, 256 callers, 1 query each (parallel)
3. using `OWA`, 1 caller, 256 queries (sequential)
4. using `OWA`, 256 callers, 1 query each (parallel)

Netdata was configured to use 24 web threads on the 24 core server we used.

The results are as follows:

### sequential test

branch|transactions|time to complete|transaction rate|average response time|min response time|max response time
:---:|:---:|:---:|:---:|:---:|:---:|:---:|
`malloc()`|256|322.35s|0.79/sec|1.26s|1.01s|1.87s
`OWA`|256|310.19s|0.83/sec|1.21s|1.04s|1.63s

For a single query, the improvement is just marginal and not noticeable at all.

### parallel test

branch|transactions|time to complete|transaction rate|average response time|min response time|max response time
:---:|:---:|:---:|:---:|:---:|:---:|:---:|
`malloc()`|256|84.72s|3.02/sec|68.43s|50.20s|84.71s
`OWA`|256|39.35s|6.51/sec|34.48s|20.55s|39.34s

For parallel workload, like the one executed by netdata.cloud, `OWA` provides a 54% overall speed improvement (more than double the overall
user-experienced speed, including the data query itself).
