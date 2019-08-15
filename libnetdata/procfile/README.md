# PROCFILE

procfile is a library for reading text data files (i.e `/proc` files) in the fastest possible way.

## How it works

The library automatically adapts (through the iterations) its memory so that each file
is read with single `read()` call.

Then the library splits the file into words, using the supplied separators.
The library also supported quoted words (i.e. strings within of which the separators are ignored).

### Initialization

Initially the caller: 

-   calls `procfile_open()` to open the file and allocate the structures needed.

### Iterations

For each iteration, the caller:

-   calls `procfile_readall()` to read updated contents.
     This call also rewinds (`lseek()` to 0) before reading it.

     For every file, a [BUFFER](../buffer/) is used that is automatically adjusted to fit
     the entire file contents of the file. So the file is read with a single `read()` call
     (providing atomicity / consistency when the data are read from the kernel).

     Once the data are read, 2 arrays of pointers are updated:

    -   a `words` array, pointing to each word in the data read
    -   a `lines` array, pointing to the first word for each line

     This is highly optimized. Both arrays are automatically adjusted to
     fit all contents and are updated in a single pass on the data.

     The library provides a number of macros:

    -   `procfile_lines()` returns the # of lines read
    -   `procfile_linewords()` returns the # of words in the given line
    -   `procfile_word()` returns a pointer the given word #
    -   `procfile_line()` returns a pointer to the first word of the given line #
    -   `procfile_lineword()` returns a pointer to the given word # of the given line #

### Cleanup

When the caller exits:

-   calls `procfile_free()` to close the file and free all memory used.

### Performance

-   a **raspberry Pi 1** (the oldest single core one) can process 5.000+ `/proc` files per second.
-   a **J1900 Celeron** processor can process 23.000+ `/proc` files per second per core.

To achieve this kind of performance, the library tries to work in batches so that the code
and the data are inside the processor's caches.

This library is extensively used in Netdata and its plugins.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Flibnetdata%2Fprocfile%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
