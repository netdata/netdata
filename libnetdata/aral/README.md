<!--
title: "Array Allocator"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/aral/README.md
-->

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
3. Optional defragmenter, that once enabled it will make free operation a little slower while trying to maintain a sorted list of fragments to offer first during allocations. The defragmenter can be enabled / disabled at run time.

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



