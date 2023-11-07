# oom_kill

## OS: Linux

The OOM Killer (Out of Memory Killer) is a process that the Linux kernel uses when the system is
critically low on memory or a process reached its memory limits. As the name suggests, it has the
duty to review all running processes and kill one or more of them in order to free up memory and
keep the system running.<sup>[1](https://neo4j.com/developer/kb/linux-out-of-memory-killer/) </sup>

Linux Kernel 4.19 introduced cgroup awareness of OOM killer implementation which adds an ability to
kill a cgroup as a single unit and to guarantee the integrity of the workload. In a nutshell,
cgroups allow the limitation of memory, disk I/O, and network usage for a group of processes.
Furthermore, cgroups may set usage quotas, and prioritize a process group to receive more CPU time
or memory than other groups. You can see more about cgroups in
the [cgroup man pages](https://man7.org/linux/man-pages/man7/cgroups.7.html)

The Netdata Agent monitors the number of Out Of Memory (OOM) kills in the last 30 minutes. Receiving
this alert indicates that some processes got killed by OOM Killer.

<details>
<summary>References and Sources</summary>

1. [Linux Out of Memory Killer](https://neo4j.com/developer/kb/linux-out-of-memory-killer/)
2. [Memory Resource Controller in linux kernel](https://docs.kernel.org/admin-guide/cgroup-v1/memory.html?highlight=oom)
3. [OOM killer blogspot](https://www.psce.com/en/blog/2012/05/31/mysql-oom-killer-and-everything-related/)

</details>

### Troubleshooting Section

<details>
<summary>Troubleshoot issues in the OOM killer</summary>

The OOM Killer uses a heuristic system to choose a processes for termination. It is based on a score
associated with each running application, which is calculated by `oom_badness()` call inside Linux
kernel <sup>[3](https://www.psce.com/en/blog/2012/05/31/mysql-oom-killer-and-everything-related/) </sup>
  
1. To identify which process/apps was killed from the OOM killer, inspect the logs:

```
root@netdata~ # dmesg -T | egrep -i 'killed process'
```
The system response looks similar to this: 
```
Jan 7 07:12:33 mysql-server-01 kernel: Out of Memory: Killed process 3154 (mysqld).
```

2. To see the current `oom_score` (the priority in which OOM killer will act upon your processes) run the following script.
The script prints all running processes (by pid and name) with likelihood to be killed by the OOM killer (second column). 
The greater the `oom_score` (second column) the more propably to be killed by OOM killer.

```
root@netdata~ #  while read -r pid comm; do  
  printf '%d\t%d\t%s\n' "$pid" "$(cat /proc/$pid/oom_score)" "$comm"; 
done < <(ps -e -o pid= -o comm=) | sort -k 2n
```

3. Adjust the `oom_score` to protect processes using the `choom` util from
the `util-linux` [package v2.33-rc1+](https://github.com/util-linux/util-linux/commit/8fa223daba1963c34cc828075ce6773ff01fafe3)

```
root@netdata~ #  choom -p PID -n number
```

> Note: Setting an adjust score value of +500, for example, is roughly equivalent to allowing
> the remainder of tasks sharing the same system, cpuset, mempolicy, or memory controller resources to
> use at least 50% more memory. A value of -500, on the other hand, would be roughly equivalent to
> discounting 50% of the task’s allowed memory from being considered as scoring against the task.

4. Once the settings work to your case, make the change permanent. In the unit file of your service, under the [Service] section, add the following value: `OOMScoreAdjust=<PREFFERRED_VALUE>`

</details>

<details>
<summary>Check the per-process RAM usage to find the top consumers</summary>

1. To see which processes are the main RAM consumers, use `top utility`. The `%MEM` column displays RAM consumption in percent.

```
root@netdata~ # top -b -o +%MEM | head -n 22
```

2. Close any of the main consumer processes. Netdata strongly suggests knowing exactly what processes you are closing and being certain that they are not necessary.
</details>


<details>
<summary>Add a temporary swap file</summary>

Keep in mind this requires creating a swap file in one of the disks. Performance of your system may
be affected.

1. Decide where your swapfile will live. It is strongly advised to allocate the swap file under in
   the root directory. A swap file is like an extension of your RAM and it should be protected, far
   from normal user accessible directories. Run the following command:

   ```
   root@netdata # dd if=/dev/zero of=<path_in_root> bs=1024 count=<size_in_bytes>
   ```

2. Grant root only access to the swap file:

   ```
   root@netdata # chmod 600 <path_to_the_swap_file_you_created>
   ```

3. Make it a Linux swap area:

   ```
   root@netdata # mkswap <path_to_the_swap_file_you_created>
   ```

4. Enable the swap with the following command:

   ```
   root@netdata # swapon <path_to_the_swap_file_you_created>
   ```

5. If you plan to use it a regular basis, you should update the `/etc/fstab` config. The entry you
   will add would look like:

   ```
   /swap_file            swap    sw              0       0
   ```

   For more information see the fstab manpage: `man fstab`.

</details>
