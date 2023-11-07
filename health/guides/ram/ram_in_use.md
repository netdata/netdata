# ram_in_use

## OS: Linux, FreeBSD

This alert shows the percentage of used RAM. If you receive this alert, there is high RAM utilization on the node. Running
low on RAM memory, means that the performance of running applications might be affected.

If there is no `swap` space available, the OOM Killer can start killing processes.

> When a system runs out of RAM, it can store it's inactive content in persistent storage (e.g. your
> main drive). The borrowed space is called `swap` or "swap space".

> The OOM Killer (Out of Memory Killer) is a process that the Linux Kernel uses when the system is critically low on
> RAM. As the name suggests, it has the duty to review all running processes and kill one or more of them in order
> to free up RAM memory and keep the system running.

### Troubleshooting section:

<details>
<summary>Check per-process RAM usage to find the top consumers</summary>

<details>
<summary>Linux</summary>

Use `top`:

```
root@netdata~ # top -b -o +%MEM | head -n 22
```

Here, you can see which processes are the main RAM consumers on the `%MEM` column (it is calculated in percentage).

It would be helpful to close any of the main consumer processes, but Netdata strongly suggests knowing exactly what
processes your are closing and being certain that they are not necessary.
</details>

<details>
<summary>FreeBSD</summary>

Use `top`:

```
root@netdata~ # top -b -o res | head -n 22
```

Here, you can see which processes are the main RAM consumers on the `RES` column (it is calculated in percentage).

It would be helpful to close any of the main consumer processes, but Netdata strongly suggests knowing exactly what
processes your are closing and being certain that they are not necessary.
</details>
</details>
