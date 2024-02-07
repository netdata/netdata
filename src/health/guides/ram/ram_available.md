### Understand the alert

This alarm shows the percentage of an estimated amount of RAM that is available for use in userspace processes without causing swapping. If this alarm gets raised it means that your system has low amount of available RAM memory, and it may affect the performance of running applications.

- If there is no `swap` space available, the OOM Killer can start killing processes.

- When a system runs out of RAM memory, it can store its inactive content in another storage's partition (e.g. your
main drive). The borrowed space is called `swap` or "swap space".

- The OOM Killer (Out of Memory Killer) is a process that the Linux Kernel uses when the system is critically low on
RAM. As the name suggests, it has the duty to review all running processes and kill one or more of them in order
to free up RAM memory and keep the system running.<sup>[1](https://neo4j.com/developer/kb/linux-out-of-memory-killer/)</sup>

### Troubleshoot the alert

- Check per-process RAM usage to find the top consumers

Linux:
```
top -b -o +%MEM | head -n 22
```
FreeBSD:
```
top -b -o res | head -n 22
```

It would be helpful to close any of the main consumer processes, but Netdata strongly suggests knowing exactly what processes you are closing and being certain that they are not necessary.

### Useful resources
[Linux Out of Memory Killer](https://neo4j.com/developer/kb/linux-out-of-memory-killer/)
