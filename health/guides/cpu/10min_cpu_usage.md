### Understand the alert

This alarm calculates an average on CPU utilization over a period of 10 minutes, **excluding** `iowait`, `nice` and `steal` values.

*Note that on FreeBSD, the alert excludes only `nice`.

`iowait` is the percentage of time the CPU waits on a disk for an I/O; it happens when the former is getting bottlenecked by the latter. At this point the CPU is being idle, waiting only on the I/O.

`nice` value of a processor is the time it has spent on running low priority processes. Low priority processes are those with a 'nice' value greater than 0 (on UNIX-like systems, a higher ‘nice’ value indicates a lower priority).

`steal`, in a virtual machine, is the percentage of time that particular virtual CPU has to wait for an available host CPU to run on. If this metric goes up, it means that your VM is not getting the processing power it needs.

### Troubleshooting Section

- Processes slowing down your CPU

There are two primary cases in which this alarm is raised, and determining which applies to you requires understanding your own scenario.

1. High CPU utilization with high `nice` value means that the system is running through all the low priority processes, and if some high priority process needs CPU time, it can get it at any time.
2. High CPU utilization with low `nice` value means that the CPU is used on high priority processes and new ones will not be able to take CPU time, and they will have to wait.

The latter scenario is worth investigating if there is a process slowing down your CPU. We suggest you go to your node on Netdata Cloud and click the `nice` dimension under the `Total CPU Utilization` chart to see the value. You can then check per process CPU usage using `top`:

If you're using Linux:
```
root@netdata~ # top -o +%CPU -i
```

And for FreeBSD:
```
root@netdata~ # top -o cpu -I
```

Here, you can see which processes are the main cpu consumers on the `CPU` column.
  
It would be helpful to close any of the main consumer processes, but Netdata strongly suggests knowing exactly what processes you are closing and being certain that they are not necessary.

