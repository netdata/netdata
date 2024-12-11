# Worker Utilization

This library is to be used when there are 1 or more worker threads accepting requests
of some kind and servicing them. The goal is to provide a very simple way to monitor
worker threads utilization, as a percentage of the time they are busy and the amount
of requests served.

## Design goals

1. Minimal, if any, impact on the performance of the workers
2. Easy to be integrated into any kind of worker
3. No state of any kind at the worker side

## How to use

When a working thread starts, call:

```c
void worker_register(const char *name);
```

This will create the necessary structures for the library to work.
No need to keep a pointer to them. They are allocated as `__thread` variables.

Then job types need to be defined. Job types are anything a worker does that can be
counted and their execution time needs to be reported. The library is fast enough to
be integrated even on workers that perform hundreds of thousands of actions per second.

Job types are defined like this:

```c
void worker_register_job_type(size_t id, const char *name);
```

`id` is a number starting from zero. The library is compiled with a fixed size of 50
ids (0 to 49). More can be allocated by setting `WORKER_UTILIZATION_MAX_JOB_TYPES` in
`worker_utilization.h`. `name` can be any string up to 22 characters. This can be
changed by setting `WORKER_UTILIZATION_MAX_JOB_NAME_LENGTH` in `worker_utilization.h`.

Each thread that calls `worker_register(name)` will allocate about 3kB for maintaining
the information required.

When the thread stops, call:

```c
void worker_unregister(void);
```

Again, no parameters, or return values.

> IMPORTANT: cancellable threads need to add a call to `worker_unregister()` to the
> `pop` function that cleans up the thread. Failure to do so, will result in about
> 3kB of memory leak for every thread that is stopped.

When you are about to do some work in the working thread, call:

```c
void worker_is_busy(size_t id);
```

When you finish doing the job, call:

```c
void worker_is_idle(void);
```

Calls to `worker_is_busy(id)` can be made one after another (without calling
`worker_is_idle()` between them) to switch jobs without losing any time between
them and eliminating one of the 2 clock calls involved.

## Implementation details

Totally lockless, extremely fast, it should not introduce any kind of problems to the
workers. Every time `worker_is_busy(id)` or `worker_is_idle()` are called, a call to
`now_realtime_usec()` is done and a couple of variables are updated. That's it!

The worker does not need to update the variables regularly. Based on the last status
of the worker, the statistics collector of netdata will calculate if the thread is
busy or idle all the time or part of the time. Works well for both thousands of jobs
per second and unlimited working time (being totally busy with a single request for
ages).

The statistics collector is called by the telemetry thread of netdata. So,
even if the workers are extremely busy with their jobs, netdata will be able to know
how busy they are.
