
# Adaptive Re-sortable List (ARL)

This library allows netdata to read a series of `name - value` pairs
in the **fastest possible way**.

ARLs are used all over netdata, as they are the most
CPU utilization efficient way to process `/proc` files. They are used to
process both vertical (csv like) and horizontal (one pair per line) `name - value` pairs.

## How ARL works

It maintains a linked list of all `NAME` (keywords), sorted in the
order found in the data source. The linked list is kept
sorted at all times - the data source may change at any time, the
linked list will adapt at the next iteration.

### Initialization

During initialization (just once), the caller:

- calls `arl_create()` to create the ARL

- calls `arl_expect()` multiple times to register the expected keywords

The library will call the `processor()` function (given to
`arl_create()`), for each expected keyword found.
The default `processor()` expects `dst` to be an `unsigned long long *`.

Each `name` keyword may have a different `processor()` (by calling
`arl_expect_custom()` instead of `arl_expect()`).

### Data collection iterations

For each iteration through the data source, the caller:

- calls `arl_begin()` to initiate a data collection iteration.
   This is to be called just ONCE every time the source is re-evaluated.

- calls `arl_check()` for each entry read from the file.

### Cleanup

When the caller exits:

- calls `arl_free()` to destroy this and free all memory.

### Performance

ARL maintains a list of `name` keywords found in the data source (even the ones
that are not useful for data collection).

If the data source maintains the same order on the `name-value` pairs, for each
each call to `arl_check()` only an `strcmp()` is executed to verify the
expected order has not changed, a counter is incremented and a pointer is changed.
So, if the data source has 100 `name-value` pairs, and their order remains constant
over time, 100 successful `strcmp()` are executed.

In the unlikely event that an iteration sees the data source with a different order,
for each out-of-order keyword, a full search of the remaining keywords is made. But
this search uses 32bit hashes, not string comparisons, so it should also be fast. 

When all expectations are satisfied (even in the middle of an iteration),
the call to `arl_check()` will return 1, to signal the caller to stop the loop,
saving valuable CPU resources for the rest of the data source. 

In the following test we used alternative methods to process, **1M times**,
a data source like `/proc/meminfo`, already tokenized, in memory,
to extract the same number of expected metrics:

test|code|string comparison|number parsing|duration
:---:|:---:|:---:|:---:|:---:|
1|if-else-if-else-if|`strcmp()`|`strtoull()`|4698657 usecs
2|if-else-if-else-if|inline `simple_hash()` and `strcmp()`|`strtoull()`| 872005 usecs
3|if-else-if-else-if|statement expression `simple_hash()` and `strcmp()`|`strtoull()`|861626 usecs
4|if-continue|inline `simple_hash()` and `strcmp()`|`strtoull()`|871887 usecs
5|if-else-if-else-if|inline `simple_hash()` and `strcmp()`|`str2ull()`|606541 usecs
6|ARL|ARL|`strtoull()`|424149 usecs
7|ARL|ARL|`str2ull()`|199324 usecs

So, compared to unoptimized code (test No 1: 4.7sec), before ARL netdata was using test
No **5** with hashing and a custom `str2ull()` to achieve 607ms.
The current ARL implementation is test No **7** that needs only 199ms
(23 times faster vs unoptimized code, 3 times faster vs optimized code).

## Limitations

Do not use ARL if the a name/keyword may appear more than once in the
source data. 
