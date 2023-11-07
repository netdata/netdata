# 30min_ram_swapped_out

If the system needs more memory resources than your available RAM, inactive pages in memory can be
moved into the swap space (or swap file). The swap space (or swap file) is located on hard drives,
which have a slower access time than physical memory.

The Netdata Agent calculates the percentage of the system RAM swapped in the last 30 minutes.

This alert is triggered in warning state if the percentage of the system RAM swapped in is more than
20%.


## OS: Linux

### Troubleshooting section:

You can find the most resource greedy processes in your system, but if you receive this alert many 
times you must consider upgrade your system's RAM.

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

You can find the most resource greedy processes in your system, but if you receive this alert many 
times you must consider upgrade your system's RAM.

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





