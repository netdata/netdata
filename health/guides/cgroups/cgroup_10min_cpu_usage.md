# cgroup_10min_cpu_usage

**Cgroups | CPU**

_Control groups, usually referred to as Cgroups, are a Linux kernel feature which allow processes to
be organized into hierarchical groups whose usage of various types of resources can then be limited
and monitored. The Kernel's cgroup interface is provided through a pseudo-filesystem called
cgroupfs (`/sys/fs/cgroup`). Grouping is implemented in the core Cgroup Kernel code, while resource
tracking and limits are implemented in a set of per-resource-type subsystems (memory, CPU, and so
on).<sup>[1](https://man7.org/linux/man-pages/man7/cgroups.7.html) </sup>_

The Netdata Agent calculates the average CPU utilization over the last 10 minutes. This alert
indicates that one of the group of you processes is in high CPU utilization. The system will
throttle the group CPU usage when the usage is over the limit.

This alert is raised in warning state when the average CPU utilization is between 75-80% of the
limits and in critical when it is between 85-95%.

<details>
<summary>More about Cgroups</summary>

Since Linux kernel 4.5 (March 2016) there are two implementations of Cgroups, v1 and v2.

#### Cgroup controllers (in v1)

In the vanilla kernel you will find the following controllers.

- cpu: `CONFIG_CGROUP_SCHED`, Cgroups can be guaranteed a minimum number of "CPU shares" when a
  system is busy.
- cpuacct: `CONFIG_CGROUP_CPUACCT`, This controller provides accounting for CPU usage by groups of
  processes.
- cpuset: `CONFIG_CPUSETS`, This Cgroup can be used to bind the processes in a Cgroup to a specified
  set of CPUs and NUMA nodes.
- memory: `CONFIG_MEMCG`, The memory controller supports reporting and limiting of process memory,
  kernel memory, and swap used by Cgroups.
- devices: `CONFIG_CGROUP_DEVICE`, This supports controlling which processes may create (mknod)
  devices as well as open them for reading or writing. The policies may be specified as allow-lists
  and deny-lists. Hierarchy is enforced, so new rules must not violate existing rules for the target
  or ancestor Cgroups.
- freezer: `CONFIG_CGROUP_FREEZER`, The freezer Cgroup can suspend and restore (resume) all
  processes in a Cgroup. Freezing a Cgroup /A also causes its children, for example, processes in
  /A/B, to be frozen.
- net_cls: `CONFIG_CGROUP_NET_CLASSID`, This places a classid, specified for the Cgroup, on network
  packets created by a Cgroup. These classids can then be used in firewall rules, as well as used to
  shape traffic using `tc`. This applies only to packets leaving the Cgroup, not to traffic arriving
  at the Cgroup.
- blkio: `CONFIG_BLK_CGROUP`, The blkio Cgroup controls and limits access to specified block devices
  by applying IO control in the form of throttling and upper limits against leaf nodes and
  intermediate nodes in the storage hierarchy. Two policies are available. The first is a
  proportional- weight time-based division of disk implemented with CFQ. This is in effect for leaf
  nodes using CFQ. The second is a throttling policy which specifies upper I/O rate limits on a
  device.
- perf_event: `CONFIG_CGROUP_PERF`, This controller allows perf monitoring of the set of processes
  grouped in a Cgroup.
- net_prio: `CONFIG_CGROUP_NET_PRIO`, This allows priorities to be specified, per network interface,
  for Cgroups.
- hugetlb: `CONFIG_CGROUP_HUGETLB`, This supports limiting the use of huge pages by Cgroups.
- pids: `CONFIG_CGROUP_PIDS` , This controller permits limiting the number of process that may be
  created in a Cgroup (
  and its descendants).
- rdma: `CONFIG_CGROUP_RDMA`, The RDMA controller permits limiting the use of RDMA/IB- specific
  resources per Cgroup. _since Linux 4.11_

Different variations of the Linux kernel can have more or less Cgroup controllers OR/AND enabled not
all of them.

#### Cgroups v1

Under Cgroups v1, each controller may be mounted against a separate Cgroup filesystem that provides
its own hierarchical organization of the processes on the system. It is also possible to co-mount
multiple (or even all) Cgroups v1 controllers against the same Cgroup filesystem. That means that
the mounted controllers manage the same hierarchical organization of processes.

For each mounted hierarchy, the directory tree mirrors the control group hierarchy. Each control
group is represented by a directory, with each of its child control Cgroups represented as a child
directory. For instance, `/user/joe/1.session` represents control group 1.session, which is a child
of Cgroup joe, which is a child of /user. Under each Cgroup directory is a set of files which can be
read or written to, reflecting resource limits and a few general Cgroup properties.

##### Hierarchy in Cgroups v1

The Cgroups v1 is organized in a tree way:
`/sys/fs/cgroup/{controller: cpu, cpuacct, cpupids, memory}/{cgroup_process: {process A, process B docker}/{rules: } `

#### Cgroups v2

In Cgroups v2, all mounted controllers reside in a single unified hierarchy. While (different)
controllers may be simultaneously mounted under the v1 and v2 hierarchies, it is not possible to
mount the same controller simultaneously under both the v1 and the v2 hierarchies.

The new behaviors in Cgroups v2 are summarized here, and in some cases elaborated in the following
subsections.

1. Cgroups v2 provides a unified hierarchy against which all controllers are mounted.

2. "Internal" processes are not permitted. Except of the root Cgroup, processes may reside only in
   leaf nodes
   (Cgroups that do not themselves contain child Cgroups). The details are somewhat more subtle than
   this, and are described below.

3. Active Cgroups must be specified via the files cgroup.controllers and cgroup.subtree_control.

4. The tasks file has been removed. In addition, the cgroup.clone_children file that is employed by
   the cpuset controller has been removed.

5. An improved mechanism for notification of empty Cgroups is provided by the cgroup.events file.

   For more changes, see the Documentation/admin-guide/cgroup-v2.rst file in the kernel source (or
   Documentation/cgroup-v2.txt in Linux 4.17 and earlier).

##### Hierarchy in Cgroups v2

The Cgroups v2 is organized in a tree way:
`/sys/fs/cgroup/{cgroup1: {/cgroup2, /cgroup3}, cgroup5: {/cgroup6:  {/cgroup8} } } . . .`

So every Cgroup contains over Cgroups, cgroup1 contains cgroup2 and cgroup3, cgroup5 contains
cgroup6 which contains cgroup8 and so on so forth. So all the parent cgroups/processes shares their
resource with their childs. Active Cgroups must be specified via the `cgroup.controllers` and
`cgroup.subtree_control` files. Consult
the [cgroup man pages](https://man7.org/linux/man-pages/man7/cgroups.7.html) for more information.

Cgroups v2 implements only a subset of the controllers available in Cgroups v1. **The two systems
are implemented so that both v1 controllers and v2 controllers can be mounted on the same system**.
Thus, for example, it is possible to use those controllers that are supported under version 2, while
also using version 1 controllers where version 2 does not yet support those controllers. The only
restriction here is that a controller can't be simultaneously employed in both a Cgroups v1
hierarchy and in the Cgroups v2 hierarchy.

#### Major between Cgroups v1 & v2:

You might already have figure out some differences from the explanations above but let's note some
of them.

The biggest difference is that in Cgroups v2 a process can't join different groups for different
controllers. For example in v1 a process could use the /sys/fs/cgroup/cpu/cgroupA and the
/sys/fs/cgroup/memory/cgroupB controllers at the same time. In v2 a process joins only the
/sys/fs/cgroups/cgroupC and is subject to all the controller of this Cgroup.

Another difference is that, in Cgroups v1 when events like page cache writebacks or network packets
reception occur, the resources allocated was not charged to the responsible Cgroup. For example,
when your network interface receives packets, and the kernel is not aware of the destination Cgroup
, the packets need to be processed first and re-routed to the corresponding subsystem or userland
application. This operation was not included/calculated in the applications Cgroup.

### More

You can experiment with Cgroups, (safely, in a testing environment) create a new Cgroup and set limits
in your system with tools like `cgcreate` and `cgset`from the `libcgroup-tools` package and then
make your services run with those constrains. In the following link you can find a
blog [in the linuxhint blog](https://linuxhint.com/limit_cpu_usage_process_linux/) which can guide
you through it.

</details>


<details>
<summary>References and sources:</summary>

1. [cgroups(7) man pages](https://man7.org/linux/man-pages/man7/cgroups.7.html)
2. [linuxhint blog](https://linuxhint.com/limit_cpu_usage_process_linux/)
3. [cgroups on Arch linux](https://wiki.archlinux.org/title/cgroups)

</details>

### Troubleshooting section

When your service/app/container reaches it's Cgroup (cpu) hard limits (or on pressure the soft
limits), it would be halted or/and start thrashing. You can imagine the case when a service halts in
a lock, nightmare! If needed, you can extend those limits:

<details>
<summary>Native linux applications</summary>

Control groups can be accessed with various tools, please consult this
guide ([cgroups on Arch linux](https://wiki.archlinux.org/title/cgroups)) to do that

For example if you would like to set a CPU share limit in your apache server:

    ```
    root@netdata # systemctl set-property --runtime httpd.service CPUShares=500
    ```

**Note**: To change it permanently, omit the `--runtime` flag

</details>

<details>
<summary>Docker containers</summary>

Follow
the [Configure the default CFS scheduler](https://docs.docker.com/config/containers/resource_constraints/#configure-the-default-cfs-scheduler)
section in the official docs to do that.

</details>

<details>
<summary>Kubernetes </summary>

You can apply Cgroups restrictions in any manifest that creates Pods. For example in a Deployment,
in the PodSpecs configuration you can apply limits under `..spec.resources.limits.cpu: <value>`
Consult
this [resource management for pods and container](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)
guide in the official docs.

You can also apply limits per namespaces or other policy, which is the best practice for handling
Kubernetes resources. Consult
this [resource quotas](https://kubernetes.io/docs/concepts/policy/resource-quotas/) guide in the
official docs.


</details>

