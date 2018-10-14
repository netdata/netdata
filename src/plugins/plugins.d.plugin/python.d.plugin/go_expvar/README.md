# go_expvar

The `go_expvar` module can monitor any Go application that exposes its metrics with the use of `expvar` package from the Go standard library.

`go_expvar` produces charts for Go runtime memory statistics and optionally any number of custom charts. Please see the [wiki page](https://github.com/netdata/netdata/wiki/Monitoring-Go-Applications) for more info.

For the memory statistics, it produces the following charts:

1. **Heap allocations** in kB
 * alloc: size of objects allocated on the heap
 * inuse: size of allocated heap spans

2. **Stack allocations** in kB
 * inuse: size of allocated stack spans

3. **MSpan allocations** in kB
 * inuse: size of allocated mspan structures

4. **MCache allocations** in kB
 * inuse: size of allocated mcache structures

5. **Virtual memory** in kB
 * sys: size of reserved virtual address space

6. **Live objects**
 * live: number of live objects in memory

7. **GC pauses average** in ns
 * avg: average duration of all GC stop-the-world pauses

### configuration

Please see the [wiki page](https://github.com/netdata/netdata/wiki/Monitoring-Go-Applications#using-netdata-go_expvar-module) for detailed info about module configuration.

---
