# Netdata Memory Requirements

Although `netdata` does all its calculations using `long double` (128 bit) arithmetics, it stores all values using a **custom-made 32-bit number**.

This custom-made number can store in 29 bits values from `-167772150000000.0` to  `167772150000000.0` with a precision of 0.00001 (yes, it is a floating point number, meaning that higher integer values have less decimal precision) and 3 bits for flags.

This provides an extremely optimized memory footprint with just 0.0001% max accuracy loss.

## Sizing memory

So, for each dimension of a chart, netdata will need: `4 bytes for the value * the entries of its history`. It will not store any other data for each value in the time series database. Since all its values are stored in a time series with fixed step, the time each value corresponds can be calculated at run time, using the position of a value in the round robin database.

The default history is 3.600 entries, thus it will need 14.4KB for each chart dimension. If you need 1.000 dimensions, they will occupy just 14.4MB.

Of course, 3.600 entries is a very short history, especially if data collection frequency is set to 1 second. You will have just one hour of data.

For a day of data and 1.000 dimensions, you will need: 86.400 seconds * 4 bytes * 1.000 dimensions = 345MB of RAM.

Currently the only option you have to lower this number is to use **[[Memory Deduplication - Kernel Same Page Merging - KSM]]**.

## The future

I investigate several alternatives to lower this number. The best so far is to split the in-memory round robin database in a small **realtime** database (e.g. an hour long) and a larger compressed **archive** database to store longer durations. So (for example) every hour netdata will compress the last hour of data using LZ4 (which is very fast: 350MB/s compression, 1850MB/s decompression) and append these compressed data to an **archive** round robin database. This **archive** database will be saved to disk and loaded back to memory on demand, when a chart is zoomed or panned to the compressed timeframe.

This is future though. For the moment, if you need a long history, you will need a lot of RAM.


## Running netdata in embedded devices

Embedded devices usually have very limited RAM resources available.

There are 2 settings for you to tweak:

1. `update every`, which controls the data collection frequency
2. `history`, which controls the size of the database in RAM

By default `update every = 1` and `history = 3600`. This gives you an hour of data with per second updates.

If you set `update every = 2` and `history = 1800`, you will still have an hour of data, but collected once every 2 seconds. This will **cut in half** both CPU and RAM resources consumed by netdata.

You can also disable plugins you don't need. Disabling the plugins will also free both CPU and RAM resources.
