# Gorilla compression and decompression

This provides an alternative way of representing values stored in database
pages. Instead of allocating and using a page of fixed size, ie. 4096 bytes,
the Gorilla implementation adds support for dynamically sized pages that
contain a variable number of Gorilla buffers.

Each buffer takes 512 bytes and compresses incoming data using the Gorilla
compression:

- The very first value is stored as it is.
- For each new value, Gorilla compression doesn't store the value itself. Instead,
it computes the difference (XOR) between the new value and the previous value.
- If the XOR result is zero (meaning the new value is identical to the previous
value), we store just a single bit set to `1`.
- If the XOR result is not zero (meaning the new value differs from the previous):
  - We store a `0` bit to indicate the change.
  - We compute the leading-zero count (LZC) of the XOR result, and compare it
    with the previous LZC. If the two LZCs are equal we store a `1` bit.
  - If the LZCs are different we use 5 bits to store the new LZC, and we store
    the rest of the value (ie. without its LZC) in the buffer.

A Gorilla page can have multiple Gorilla buffers. If the values of a metric
are highly compressible, just one Gorilla buffer is able to store all the values
that otherwise would require a regular 4096 byte page, ie. we can use just 512
bytes instead. In the worst case scenario (for metrics whose values are not
compressible at all), a Gorilla page might end up having `9` Gorilla buffers,
consuming 4608 bytes. In practice, this is pretty rare and does not negate
the effect of compression for the metrics.

When a gorilla page is full, ie. it contains 1024 slots/values, we serialize
the linked-list of gorilla buffers directly to disk. During deserialization,
eg. when performing a DBEngine query, the Gorilla page is loaded from the disk and
its linked-list entries are patched to point to the new memory allocated for
serving the query results.

Overall, on a real-agent the Gorilla compression scheme reduces memory
consumption approximately by ~30%, which can be several GiB of RAM for parents
having hundreds, or even thousands of children streaming to them.
