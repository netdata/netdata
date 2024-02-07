### Understand the alert

This alert shows the percentage of used RAM. If you receive this alert, there is high RAM utilization on the node. Running low on RAM memory, means that the performance of running applications might be affected.

If there is no `swap` space available, the OOM Killer can start killing processes.

When a system runs out of RAM, it can store it's inactive content in persistent storage (e.g. your main drive). The borrowed space is called `swap` or "swap space".

The OOM Killer (Out of Memory Killer) is a process that the Linux Kernel uses when the system is critically low on RAM. As the name suggests, it has the duty to review all running processes and kill one or more of them in order
to free up RAM memory and keep the system running.

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
