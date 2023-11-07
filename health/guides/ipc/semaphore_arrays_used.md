# semaphores_arrays_used

## OS: Linux

This alert presents the percentage of allocated `System V IPC semaphore arrays (sets)`.  \
If you receive this alert, it means that your system is experiencing high `IPC semaphore arrays` utilization and a lack
of available semaphore arrays can affect application performance.

<details>
<summary>What is an "IPC Semaphore array (or set)"</summary>

"IPC" stands for "Interprocess Communication". IPC messages are a counterpart to UNIX pipes for IPC operations. \
The fastest way to communicate through processes is with shared memory. semaphores, help synchronise shared memory
access across processes.<sup> [1](https://docs.oracle.com/cd/E19455-01/806-4750/6jdqdfltn/index.html) </sup>

System V semaphores are allocated in groups called sets.  \
A "semaphore set" consists of a control structure and an array of individual semaphores.

As illustrated by E. W. Dijkstra, semaphores can be better understood using his railroad model <sup> [2](
https://users.cs.cf.ac.uk/Dave.Marshall/C/node26.html) </sup>.

- Imagine a railroad, where only a single train at a time is allowed to pass. Responsible for the traffic is a
  **semaphore**. Each train that wants to enter the single track must wait for the **semaphore** to be in a state that
  allows access to the railroad. When a train enters the track, the **semaphore** changes the state to prevent all other
  traffic in the track. When the train leaves the railroad, it must change the state of the **semaphore** to allow another
  train to enter.


- In the computer world, a **semaphore** is an integer and the train is a process (or a thread). For a process to proceed,
  it has to wait for the semaphore's value to become 0. If it proceeds, it increments this value by 1. Upon finishing
  the task, the process decrements the same value by 1.

> **semaphores** let processes query or alter status information. They are often used to monitor and control the
> availability of system resources such as shared memory segments.
> <sup> [2](https://users.cs.cf.ac.uk/Dave.Marshall/C/node26.html) </sup>



</details>

<br>

<details>
<summary>References and Sources</summary>

[[1] Interprocess Communication](https://docs.oracle.com/cd/E19455-01/806-4750/6jdqdfltn/index.html)  \
[[2] IPC:Semaphores](https://users.cs.cf.ac.uk/Dave.Marshall/C/node26.html)

</details>

### Troubleshooting Section

<details>
    <summary>Adjust the semaphore limit on your system</summary>

You can check current `semaphore arrays` limit on your machine, by running:

```
root@netdata~ # ipcs -ls
```

The output will be similar to this:

```
------ Semaphore Limits --------
max number of arrays = 32000
max semaphores per array = 32000
max semaphores system wide = 1024000000
max ops per semop call = 500
semaphore max value = 32767
```

To adjust the limit of the max semaphores,you can go to the `/proc/sys/kernel/sem` file and adjust the fourth field
accordingly.

</details>
