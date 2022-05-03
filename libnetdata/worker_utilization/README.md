<!--
title: "Worker Utilization"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/onewayallocator/README.md
-->

# Worker Utilization

This library is to be used when there are 1 or more worker threads accepting requests of some kind and servicing them.
The goal is to provide a very simple way to monitor worker threads utilization, as a percentage of the time they are busy and the amount of requests served.

## How to use

When a working thread starts, call:

```c
void worker_register(const char *name);
```

This will create the necessary structures for the library to work.
No need to keep a pointer to them. They are allocated as `__thread` variables.

When the thread stops, call:

```c
void worker_unregister(void)
```

Again, no parameters, or return values.

When you are about to do some work in the working thread, call:

```c
void worker_is_busy(void)
```

When you finish doing the job, call:

```c
void worker_is_idle(void)
```

## Implementation details

Totally lockless, extremely fast, it should not introduce any kind of problems to the workers.
Every time `worker_is_busy()` or `worker_is_idle()` are called, a call to `now_realtime_usec()`
is done and a couple of variables are updated. That's it!

The worker does not need to update the variables regularly. Based on the last status of the worker,
the statistics collector of netdata will calculate if the thread is busy or idle all the time or
part of the time. Works well for both thousands of jobs per second and unlimited working time
(being totally busy with a single request for ages).

The statistics collector is called by the global statistics thread of netdata. So, even if the workers
are extremely busy with their jobs, netdata will be able to know how busy they are.
