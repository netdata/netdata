### Understand the alert

If the system needs more memory resources than your available RAM, inactive pages in memory can be moved into the swap space (or swap file). The swap space (or swap file) is located on hard drives,
which have a slower access time than physical memory.

The Netdata Agent calculates the percentage of the system RAM swapped in the last 30 minutes. This alert is triggered in warning state if the percentage of the system RAM swapped in is more than 20%.

### Troubleshoot the alert 

You can find the most resource greedy processes in your system, but if you receive this alert many times you must consider upgrading your system's RAM.

- Find the processes that consume the most RAM 

Linux:
```
top -b -o +%MEM | head -n 22
```

FreeBSD:
```
top -b -o res | head -n 22
```

Here, you can see which processes are the main RAM consumers. Consider killing any of the main consumer processes that you do not need to avoid thrashing.

Netdata strongly suggests knowing exactly what processes you are closing and being certain that they are not necessary.
