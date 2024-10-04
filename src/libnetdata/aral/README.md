# Array Allocator

Come on! Array allocators are embedded in libc! Why do we need such a thing in Netdata?

Well, we have a couple of problems to solve:

1. **Fragmentation** - It is important for Netdata to keeps its overall memory footprint as low as possible. libc does an amazing job when the same thread allocates and frees some memory. But it simply cannot do better without knowing the specifics of the application when memory is allocated and freed randomly between threads.
2. **Speed** - Especially when allocations and de-allocations happen across threads, the speed penalty is tremendous.

In Netdata we have a few moments that are very tough. Imagine collecting 1 million metrics per second. You have a buffer for each metric and put append new points there. This works beautifully, of course! But then, when the buffers get full, imagine the situation. You suddenly need 1 million buffers, at once!

To solve this problem we first spread out the buffers. So, the first time each metric asks for a buffer, it gets a smaller one. We added logic there to spread them as evenly as possible across time. Solved? Not exactly!

We have 3 tiers for each metric. For the metrics of tier 0 (per second resolution) we have a max buffer for 1024 points and every new metrics gets a random size between 3 points and 1024. So they are distributed across time. For 1 million metrics, we have about 1000 buffers beings created every second.

But at some point, the end of the minute will come, and suddenly all the metrics will need a new buffer for tier 1 (per minute). Oops! We will spread tier 1 buffers across time too, but the first minute is a tough one. We really need 1 million buffers instantly.

And if that minute happens to also be the beginning of an hour... tier 2 (per hour) kicks in. For that instant we are going to need 2 million buffers instantly.

The problem becomes even bigger when we collect 2, or even 10 million metrics...

So solve it, Netdata uses a special implementation of an array allocator that is tightly integrated with the structures we need.

## Features

1. Malloc, or MMAP modes. File based MMAP is also supported to put the data in file backed up shared memory.
2. Fully asynchronous operations. There are just a couple of points where spin-locks protect a few counters and pointers.
3. Optional defragmenter, that once enabled it will make free operation slower while trying to maintain a sorted list of fragments to offer first during allocations. The defragmenter can be enabled / disabled at run time. The defragmenter can hurt performance on application with intense turn-around of allocation, like Netdata dbengine caches. So, it is disabled by default.
4. Without the defragmenter enabled, ARAL still tries to keep pages full, but the depth of the search is limited to 3 pages (so, a page with a free slot will either become 1st, 2nd, or 3rd). At the same time, during allocations, ARAL will evaluate the first 2 pages to find the one that is more full than the other, to use it for the new allocation.

## How it works

Allocations are organized in pages. Pages have a minimum size (a system page, usually 4KB) and a maximum defined by for each different kind of object.

Initially every page is free. When an allocation request is made, the free space is split, and the first element is reserved. Free space is now considered there rest.

This continuous until the page gets full, where a new page is allocated and the process is repeated.

Each allocation returned has a pointer appended to it. The pointer points to the page the allocation belongs to.

When a pointer is freed, the page it belongs is identified, its space is marked free, and it is prepended in a single linked list that resides in the page itself. So, each page has its own list of free slots to use.

Pages are then on another linked list. This is a double linked list and at its beginning has the pages with free space and at the end the pages that are full. 

When the defragmenter is enabled the pages double linked list is also sorted, like this: the fewer the free slots on a page, the earlier in the linked list the page will be, except if it does not have any free slot, in which case it will be at the end. So, the defragmenter tries to have pages full.

When a page is entirerly free, it is given back to the system immediately. There is no caching of free pages.


Parallelism is achieved like this:

When some threads are waiting for a page to be allocated, free operations are allowed. If a free operation happens before a new page is allocated, any waiting thread will get the slot that is freed on another page.

Free operations happen in parallel, even for the same page. There is a spin-lock on each page to protect the base pointer of the page's free slots single linked list. But, this is instant. All preparative work happens lockless, then to add the free slot to the page, the page spinlock is acquired, the free slot is prepended to the linked list on the page, the spinlock is released. Such free operations on different pages are totally parallel.

Once the free operation on a page has finished, the pages double linked list spinlock is acquired to put the page first on that linked list. If the defragmenter is enabled, the spinlock is retained for a little longer, to find the exact position of the page in the linked list.

During allocations, the reverse order is used. First get the pages double linked list spinlock, get the first page and decrement its free slots counter, then release the spinlock. If the first page does not have any free slots, a page allocation is spawn, without any locks acquired. All threads are spinning waiting for a page with free slots, either from the newly allocated one or from a free operation that may happen in parallel.

Once a page is acquired, each thread locks its own page to get the first free slot and releases the lock immediately. This is guaranteed to succeed, because when the page was given to that thread its free slots counter was decremented. So, there is a free slot for every thread that got that page. All preparative work to return a pointer to the caller is done lock free. Allocations on different pages are done in parallel, without any intervention between them.


## What to expect

Systems not designed for parallelism achieve their top performance single threaded. The single threaded speed is the baseline. Adding more threads makes them slower.

The baseline for ARAL is the following, the included stress test when running single threaded:

```
Running stress test of 1 threads, with 10000 elements each, for 5 seconds...
2023-01-29 17:04:50: netdata INFO  : TH[0] : set name of thread 1314983 to TH[0]
ARAL executes 12.27 M malloc and 12.26 M free operations/s
ARAL executes 12.29 M malloc and 12.29 M free operations/s
ARAL executes 12.30 M malloc and 12.30 M free operations/s
ARAL executes 12.30 M malloc and 12.29 M free operations/s
ARAL executes 12.29 M malloc and 12.29 M free operations/s
Waiting the threads to finish...
2023-01-29 17:04:55: netdata INFO  : MAIN : ARAL: did 61487356 malloc, 61487356 free, using 1 threads, in 5003808 usecs
```

The same test with 2 threads, both threads on the same ARAL of course. As you see performance improved:

```
Running stress test of 2 threads, with 10000 elements each, for 5 seconds...
2023-01-29 17:05:25: netdata INFO  : TH[0] : set name of thread 1315537 to TH[0]
2023-01-29 17:05:25: netdata INFO  : TH[1] : set name of thread 1315538 to TH[1]
ARAL executes 17.75 M malloc and 17.73 M free operations/s
ARAL executes 17.93 M malloc and 17.93 M free operations/s
ARAL executes 18.17 M malloc and 18.18 M free operations/s
ARAL executes 18.33 M malloc and 18.32 M free operations/s
ARAL executes 18.36 M malloc and 18.36 M free operations/s
Waiting the threads to finish...
2023-01-29 17:05:30: netdata INFO  : MAIN : ARAL: did 90976190 malloc, 90976190 free, using 2 threads, in 5029462 usecs
```

The same test with 4 threads:

```
Running stress test of 4 threads, with 10000 elements each, for 5 seconds...
2023-01-29 17:10:12: netdata INFO  : TH[0] : set name of thread 1319552 to TH[0]
2023-01-29 17:10:12: netdata INFO  : TH[1] : set name of thread 1319553 to TH[1]
2023-01-29 17:10:12: netdata INFO  : TH[2] : set name of thread 1319554 to TH[2]
2023-01-29 17:10:12: netdata INFO  : TH[3] : set name of thread 1319555 to TH[3]
ARAL executes 19.95 M malloc and 19.91 M free operations/s
ARAL executes 20.08 M malloc and 20.08 M free operations/s
ARAL executes 20.85 M malloc and 20.85 M free operations/s
ARAL executes 20.84 M malloc and 20.84 M free operations/s
ARAL executes 21.37 M malloc and 21.37 M free operations/s
Waiting the threads to finish...
2023-01-29 17:10:17: netdata INFO  : MAIN : ARAL: did 103549747 malloc, 103549747 free, using 4 threads, in 5023325 usecs
```

The same with 8 threads:

```
Running stress test of 8 threads, with 10000 elements each, for 5 seconds...
2023-01-29 17:07:06: netdata INFO  : TH[0] : set name of thread 1317608 to TH[0]
2023-01-29 17:07:06: netdata INFO  : TH[1] : set name of thread 1317609 to TH[1]
2023-01-29 17:07:06: netdata INFO  : TH[2] : set name of thread 1317610 to TH[2]
2023-01-29 17:07:06: netdata INFO  : TH[3] : set name of thread 1317611 to TH[3]
2023-01-29 17:07:06: netdata INFO  : TH[4] : set name of thread 1317612 to TH[4]
2023-01-29 17:07:06: netdata INFO  : TH[5] : set name of thread 1317613 to TH[5]
2023-01-29 17:07:06: netdata INFO  : TH[6] : set name of thread 1317614 to TH[6]
2023-01-29 17:07:06: netdata INFO  : TH[7] : set name of thread 1317615 to TH[7]
ARAL executes 15.73 M malloc and 15.66 M free operations/s
ARAL executes 13.95 M malloc and 13.94 M free operations/s
ARAL executes 15.59 M malloc and 15.58 M free operations/s
ARAL executes 15.49 M malloc and 15.49 M free operations/s
ARAL executes 16.16 M malloc and 16.16 M free operations/s
Waiting the threads to finish...
2023-01-29 17:07:11: netdata INFO  : MAIN : ARAL: did 78427750 malloc, 78427750 free, using 8 threads, in 5088591 usecs
```

The same with 16 threads:

```
Running stress test of 16 threads, with 10000 elements each, for 5 seconds...
2023-01-29 17:08:04: netdata INFO  : TH[0] : set name of thread 1318663 to TH[0]
2023-01-29 17:08:04: netdata INFO  : TH[1] : set name of thread 1318664 to TH[1]
2023-01-29 17:08:04: netdata INFO  : TH[2] : set name of thread 1318665 to TH[2]
2023-01-29 17:08:04: netdata INFO  : TH[3] : set name of thread 1318666 to TH[3]
2023-01-29 17:08:04: netdata INFO  : TH[4] : set name of thread 1318667 to TH[4]
2023-01-29 17:08:04: netdata INFO  : TH[5] : set name of thread 1318668 to TH[5]
2023-01-29 17:08:04: netdata INFO  : TH[6] : set name of thread 1318669 to TH[6]
2023-01-29 17:08:04: netdata INFO  : TH[7] : set name of thread 1318670 to TH[7]
2023-01-29 17:08:04: netdata INFO  : TH[8] : set name of thread 1318671 to TH[8]
2023-01-29 17:08:04: netdata INFO  : TH[9] : set name of thread 1318672 to TH[9]
2023-01-29 17:08:04: netdata INFO  : TH[10] : set name of thread 1318673 to TH[10]
2023-01-29 17:08:04: netdata INFO  : TH[11] : set name of thread 1318674 to TH[11]
2023-01-29 17:08:04: netdata INFO  : TH[12] : set name of thread 1318675 to TH[12]
2023-01-29 17:08:04: netdata INFO  : TH[13] : set name of thread 1318676 to TH[13]
2023-01-29 17:08:04: netdata INFO  : TH[14] : set name of thread 1318677 to TH[14]
2023-01-29 17:08:04: netdata INFO  : TH[15] : set name of thread 1318678 to TH[15]
ARAL executes 11.77 M malloc and 11.62 M free operations/s
ARAL executes 12.80 M malloc and 12.81 M free operations/s
ARAL executes 13.26 M malloc and 13.25 M free operations/s
ARAL executes 13.30 M malloc and 13.29 M free operations/s
ARAL executes 13.23 M malloc and 13.25 M free operations/s
Waiting the threads to finish...
2023-01-29 17:08:09: netdata INFO  : MAIN : ARAL: did 65302122 malloc, 65302122 free, using 16 threads, in 5066009 usecs
```

As you can see, the top performance is with 4 threads, almost double the single thread speed.
16 threads performance is still better than single threaded, despite the intense concurrency.
