# cockroachdb_open_file_descriptors_limit

## Database | CockroachDB

This alert presents the percentage of used file descriptors for CockroachDB.  
If you  receive this, it means that there is high file descriptor utilization against the 
soft-limit.

This alert is raised in a warning state when the metric exceeds 80%.

> In Unix and Unix-like computer operating systems, a file descriptor (FD, less frequently 
> fildes) is a unique identifier (handle) for a file or other input/output resource, such as a 
> pipe or network socket.
>
> File descriptors typically have non-negative integer values, with negative values being 
> reserved to indicate "no value" or error conditions.<sup>[1](
> https://en.wikipedia.org/wiki/File_descriptor) </sup>

<details><summary>References and Sources</summary>

1. [CockroachDB documentation](
   https://www.cockroachlabs.com/docs/v21.2/recommended-production-settings#file-descriptors-limit)

</details>

### Troubleshooting Section

<details><summary>Adjust the file descriptors limit for the process or system-wide</summary>

Check out the [CockroachDB  documentation](
https://www.cockroachlabs.com/docs/v21.2/recommended-production-settings#file-descriptors-limit) for troubleshooting advice.

</details>
