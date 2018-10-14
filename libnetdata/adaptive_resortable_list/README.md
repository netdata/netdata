
# Adaptive Re-sortable List (ARL)

This structure allows netdata to read a file of `name - value` pairs
in the **fastest possible way**.

ARLs are used all over netdata, as they are the most
CPU utilization efficient way to parse `/proc` files. They are used to
parse both vertical and horizontal `name - value` pairs.

## How ARL works

It maintains a linked list of all `NAME` (keywords), sorted in the
same order as found in the source data file. The linked list is kept
sorted at all times - the source file may change at any time, the
linked list will adapt at the next iteration.

During initialization (just once), the caller:

- calls `arl_create()` to create the ARL

- calls `arl_expect()` multiple times to register the expected keywords

Then, for each iteration through the data source:

- calls `arl_begin()` to initiate a data collection iteration.
   This is to be called just ONCE every time the source is re-scanned.

- calls `arl_check()` for each entry read from the file.

When it exits:

- calls `arl_free()` to destroy this and free all memory.

The library will call the `processor()` function, given to
`arl_create()`, for each expected keyword found.
The default `processor()` expects `dst` to be an unsigned long long *.

Each `name` keyword may have a different `processor()`.

Thus, ARL maintains a list of expectations (`name` keywords expected).
When all expectations are satisfied (even in the middle of an iteration),
the call to `arl_check()` will return 1, so signal the caller to stop the loop,
saving valuable CPU resources. 

## Limitations
Do not use this if the a name/keyword may appear more than once in the
source data set.
