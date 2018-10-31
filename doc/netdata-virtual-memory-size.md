# Netdata virtual memory size

You may notice that netdata's virtual memory size, as reported by `ps` or `/proc/pid/status`  (or even netdata's applications virtual memory chart)  is unrealistically high.

For example, it may be reported to be 150+MB, even if the resident memory size is just 25MB. Similar values may be reported for netdata plugins too.

Check this for example: A netdata installation with default settings on Ubuntu 16.04LTS. The top chart is **real memory used**, while the bottom one is **virtual memory**:

![image](https://cloud.githubusercontent.com/assets/2662304/19013772/5eb7173e-87e3-11e6-8f2b-a2ccfeb06faf.png)


## why this happens?

The system memory allocator allocates virtual memory arenas, per thread running. To my observations, this defaults to 16MB per thread on 64 bit machines. So, if you get the difference between real and virtual memory and divide it by 16MB you will roughly get the number of threads running.

The system does this for speed. Having a separate memory arena for each thread, allows the threads to run in parallel in multi-core systems, without any locks between them.

This behaviour is system specific. For example, the chart above when running netdata on alpine linux (that uses **musl** instead of **glibc**) is this:

![image](https://cloud.githubusercontent.com/assets/2662304/19013807/7cf5878e-87e4-11e6-9651-082e68701eab.png)

## can we do anything to lower it?

Since netdata already uses minimal memory allocations while it runs (i.e. it adapts its memory on start, so that while repeatedly collects data it does not do memory allocations), it already instructs the system memory allocator to minimize the memory arenas for each thread. We have also added 2 configuration options to allow you tweak these settings: https://github.com/netdata/netdata/blob/5645b1ee35248d94e6931b64a8688f7f0d865ec6/src/main.c#L410-L418

However, even if we instructed the memory allocator to use just one arena, it seems it allocates an arena per thread.

netdata also supports `jemalloc` and `tcmalloc`, however both behave exactly the same to the glibc memory allocator in this aspect.

## Is this a problem?

No, it is not.

Linux reserves real memory (physical RAM) in pages (on x86 machines pages are 4KB each). So even if the system memory allocator is allocating huge amounts of virtual memory, only the 4KB pages that are actually used are reserving physical RAM. The **real memory** chart on netdata application section, shows the amount of physical memory these pages occupy(it accounts the whole pages, even if parts of them are actually used).
