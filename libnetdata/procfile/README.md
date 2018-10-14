
# PROCFILE

procfile is a library for reading kernel files from `/proc` in the fastest possible way.

The idea is:

- every file is opened once with `procfile_open()`.

- to read updated contents, we rewind it (`lseek()` to 0) and read it again
using `procfile_readall()`.

- for every file, we use a [BUFFER](../buffer/) that is automatically adjusted to fit
the entire file contents in memory, allowing us to read it with a single read() call.
(this provides atomicity / consistency on the data read from the kernel).

- once the data are read, we update two arrays of pointers:

  - a `words` array, pointing to each word in the data read
  - a `lines` array, pointing to the first word for each line

This is highly optimized. Both arrays are automatically adjusted to
fit all contents and are updated in a single pass on the data.

The result is:

- a **raspberry Pi 1** (yes, the oldest single core one) can process 5.000+ `/proc` files per second.
- a **J1900 Celeron** processor can process 23.000+ `/proc` files per second per core.

To achieve this kind of performance, the library tries to work in batches so that the code
and the data read are at the processor's caches.

This library is extensively used in netdata and its plugins.
