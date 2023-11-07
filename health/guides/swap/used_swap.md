# used_swap

If the system needs more memory resources than your available RAM, inactive pages in memory can be
moved into the swap space (or swap file). The Swap space (or swap file) is located on hard drives,
which have a slower access time than physical memory.

The Netdata Agent calculates the percentage of the used swap. This alert indicates high swap memory
utilization. It may be a sign that the system has experienced memory pressure, which can affect the
performance of your system. If there is no RAM and swap available, OOM Killer can start killing
processes. 

This alert is triggered in warning state when the percentage of used swap is between 80-90% and in
critical state when it is between 90-98%.

## OS: Linux

### Troubleshooting section:

<details>
<summary>Find the processes that consume the most RAM </summary>

1. Use `top` to see the top RAM consumers
    ```
    root@netdata~ # top -b -o +%MEM | head -n 22
    ```

Here, you can see which processes are the main RAM consumers on the `%MEM` column (it is calculated
in percentage). It would be wise to close/kill any of the main consumer processes that you do not
need to avoid thrashing.

Netdata strongly suggests knowing exactly what processes you are closing and being certain that they
are not necessary.
</details>

## OS: FreeBSD

### Troubleshooting section:

<details>
<summary>Find the processes that consume the most RAM </summary>

1. Use `top` to see the top RAM consumers
   ```
   root@netdata~ # top -b -o res | head -n 22
   ```

Here, you can see which processes are the main RAM consumers on the `RES` column (calculated in
percentage). It would be wise to close/kill any of the main consumer processes that you do not need
to avoid thrashing, though Netdata strongly suggests knowing exactly what processes you are closing 
and being certain that they are not necessary.
</details>
