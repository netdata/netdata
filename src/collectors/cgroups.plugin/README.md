# Monitor Cgroups (cgroups.plugin)

You can monitor containers and virtual machines using **cgroups**.

cgroups (or control groups), are a Linux kernel feature that provides accounting and resource usage limiting for
processes. When cgroups are bundled with namespaces (i.e. isolation), they form what we usually call **containers**.

cgroups are hierarchical, meaning that cgroups can contain child cgroups, which can contain more cgroups, etc. All
accounting is reported (and resource usage limits are applied) also in a hierarchical way.

To visualize cgroup metrics Netdata provides configuration for cherry-picking the cgroups of interest. By default,
Netdata should pick **systemd services**, all kinds of **containers** (lxc, docker, etc.) and **virtual machines** spawn
by managers that register them with cgroups (qemu, libvirt, etc.).

The collector also exposes cachestat charts for both regular cgroups and systemd services, mirroring the legacy
`ebpf.plugin` contexts.

## Supported Technologies

cgroups.plugin monitors **any process** that creates Linux cgroups. For the following technologies, Netdata also
resolves friendly names (e.g., showing "my-web-app" instead of a raw cgroup path like
`/sys/fs/cgroup/system.slice/docker-abc123.scope`).

The **Naming** column below indicates whether Netdata can resolve the cgroup path to a human-readable name
(e.g., the container name, VM name, or service name):

| Technology                 | Naming  |
|:---------------------------|:-------:|
| Docker                     |   Yes   |
| Podman                     |   Yes   |
| Kubernetes pods/containers |   Yes   |
| Nomad (via Docker)         |   Yes   |
| AWS ECS (via Docker)       |   Yes   |
| containerd (via Docker)    |   Yes   |
| Proxmox QEMU/KVM VMs       |   Yes   |
| Proxmox LXC containers     |   Yes   |
| libvirt QEMU/KVM VMs       |   Yes   |
| libvirt LXC containers     |   Yes   |
| LXC 4.0+ (including Incus) |   Yes   |
| systemd-nspawn             |   Yes   |
| Systemd services           |   Yes   |
| KubeVirt VMs (in K8s)      | Partial |

Technologies that use the above as their underlying infrastructure are also covered. For example:

- **OpenShift** and other Kubernetes distributions (k3s, RKE, RKE2, MicroK8s, EKS, GKE, AKS) — via Kubernetes cgroup paths
- **OpenStack Nova** — via libvirt/QEMU cgroup paths
- **oVirt / RHV** — via libvirt/QEMU cgroup paths

Any other technology creating Linux cgroups is **monitored** (CPU, memory, disk I/O, network), but will show its
raw cgroup path as the name.

## Configuring Netdata for cgroups

In general, no additional settings are required. Netdata discovers all available cgroups on the host system and
collects their metrics.

### How Netdata finds the available cgroups

Linux exposes resource usage reporting and provides dynamic configuration for cgroups, using virtual files (usually)
under `/sys/fs/cgroup`. Netdata reads `/proc/self/mountinfo` to detect the exact mount point of cgroups.

Netdata rescans directories inside `/sys/fs/cgroup` for added or removed cgroups every `check for new cgroups every`
seconds (default: 10 seconds).

### Hierarchical search for cgroups

Since cgroups are hierarchical, for each of the directories shown above, Netdata walks through the subdirectories
recursively searching for cgroups (each subdirectory is another cgroup).

To provide a sensible default for this setting, Netdata uses the following pattern list (patterns starting with `!` give a
negative match and their order is important: the first matching a path will be used):

```text
[plugin:cgroups]
 search for cgroups in subpaths matching =  !*/init.scope  !*-qemu  !*.libvirt-qemu  !/init.scope  !/system  !/systemd  !/user  !/lxc/*/*  !/lxc.monitor  !/lxc.payload/*/*  !/lxc.payload.*  *
```

So, we disable checking for **child cgroups** in systemd internal
cgroups ([systemd services are monitored by Netdata](#monitoring-systemd-services)), user cgroups (normally used for
desktop and remote user sessions), qemu virtual machines (child cgroups of virtual machines) and `init.scope`. All
others are enabled.

### Enabled cgroups

To provide a sensible default, Netdata uses the
following [pattern list](/src/libnetdata/simple_pattern/README.md):

- Checks the pattern against the path of the cgroup

  ```text
  [plugin:cgroups]
   enable by default cgroups matching =  !*/init.scope  !/system.slice/run-*.scope  *user.slice/docker-*  !*user.slice*  *.scope  !/machine.slice/*/.control  !/machine.slice/*/payload*  !/machine.slice/*/supervisor  /machine.slice/*.service  */kubepods/pod*/*  */kubepods/*/pod*/*  */*-kubepods-pod*/*  */*-kubepods-*-pod*/*  !*kubepods*  !*kubelet*  !*/vcpu*  !*/emulator  !*.mount  !*.partition  !*.service  !*.service/udev  !*.socket  !*.slice  !*.swap  !*.user  !/  !/docker  !*/libvirt  !/lxc  !/lxc/*/*  !/lxc.monitor*  !/lxc.pivot  !/lxc.payload  !*lxcfs.service/.control  !/machine  !/qemu  !/system  !/systemd  !/user  *
  ```

- Checks the pattern against the name of the cgroup (as you see it on the dashboard)

  ```text
  [plugin:cgroups]
   enable by default cgroups names matching = *
  ```

Renaming is configured with the following options:

```text
[plugin:cgroups]
 run script to rename cgroups matching =  !/  !*.mount  !*.socket  !*.partition  /machine.slice/*.service  !*.service  !*.slice  !*.swap  !*.user  !init.scope  !*.scope/vcpu*  !*.scope/emulator  *.scope  *docker*  *lxc*  *qemu*  */kubepods/pod*/*  */kubepods/*/pod*/*  */*-kubepods-pod*/*  */*-kubepods-*-pod*/*  !*kubepods*  !*kubelet*  *.libvirt-qemu  *
 cgroup-name timeout = 120
```

The additional pattern list serves to limit the number of times the resolver will be called. Without it, the resolver
might be called thousands of times, depending on the number of cgroups available in the system.

The `cgroup-name timeout` option (default `120` seconds; `0` disables the
timeout) bounds how long Netdata waits for the helper to resolve a single
cgroup. If the helper exceeds it, Netdata stops waiting for that cgroup; if a
helper call simply cannot resolve a name yet (for example, while the Kubernetes
API is still populating a new pod's metadata), it is retried on a later
discovery cycle.

The above pattern list is matched against the path of the cgroup. For matched
cgroups, Netdata calls the `cgroup-name` helper to get its name. This helper
queries `docker`, `kubectl`, `podman`, or applies heuristics to find a name for
the cgroup.

#### Note on Podman container names

Podman's security model is a lot more restrictive than Docker's, so Netdata will not be able to detect container names
out of the box unless they were started by the same user as Netdata itself.

If Podman is used in "rootful" mode, it's also possible to use `podman system service` to grant Netdata access to
container names. To do this, ensure `podman system service` is running and Netdata has access
to `/run/podman/podman.sock` (the default permissions as specified by upstream are `0600`, with owner `root`, so you
will have to adjust the configuration).

[Docker Socket Proxy (HAProxy)](https://github.com/Tecnativa/docker-socket-proxy)
or [CetusGuard](https://github.com/hectorm/cetusguard)
can also be used to give Netdata restricted access to the socket. Note that `PODMAN_HOST` in Netdata's environment
should be set to the proxy's URL in this case.

#### Note on Kubernetes API access

Inside a Kubernetes cluster, the `cgroup-name` helper resolves pod and container
names by querying the kubelet and the Kubernetes API server, using the pod's
mounted service-account token and CA certificate.

By default the helper verifies the API server's TLS certificate against the
mounted cluster CA (`/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`),
which is correct for standard in-cluster deployments. If your API endpoint
presents a certificate that does not chain to that CA — for example a custom
PKI, an intercepting proxy, or an external load balancer with its own
certificate — the lookups fail and the affected pods keep a generic `k8s_<id>`
name. In that case set the environment variable `K8S_TLS_INSECURE=true` for the
Netdata service to skip API-server certificate verification (matching the
behavior of the previous shell helper). The kubelet connection is always made
without certificate verification, because kubelet serving certificates are
commonly self-signed.

### Customizing cgroup names

When Netdata resolves a cgroup to a friendly name (a Docker container name, a Kubernetes pod name, and so on), you can override the resolved name with your own value by setting a label or annotation with the key `netdata.cloud/cgroup.name`. The value becomes the cgroup's display name, and Netdata derives the chart identifier from it.

This is useful when the auto-resolved name is a raw container ID, a long auto-generated string, or simply not the name you want to see on the dashboard — for example, to turn a chart name like `cgroup_twxae02wzkyy2d19gz4dsj6a-storefront-green-1.cpu` into `cgroup_storefront-green.cpu`.

The override changes only the cgroup-name segment of a chart name such as `cgroup_<name>.cpu`. It does not affect the chart-type prefix (`cgroup_`), the metric suffix (e.g. `.cpu`), or the `@<node>` identifier shown when metrics come from a different node.

#### Docker and Podman containers

Add a container label named `netdata.cloud/cgroup.name`:

```sh
docker run --label netdata.cloud/cgroup.name=my-web-app ...
```

With Docker Compose, set it under the service `labels`:

```yaml
services:
  web:
    image: nginx
    labels:
      - "netdata.cloud/cgroup.name=my-web-app"
```

Netdata reads the label from the same `docker inspect` / `podman inspect` output it already uses for name resolution and applies the override. Podman works the same way (see the [Podman note](#note-on-podman-container-names) for the socket access requirement).

#### Kubernetes pods

For controller-managed pods (Deployments, StatefulSets, and similar), add the annotation in the pod template so it persists across pod restarts:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: storefront
spec:
  template:
    metadata:
      annotations:
        netdata.cloud/cgroup.name: "storefront-green"
```

To annotate a running pod directly (does not persist when the pod is replaced by a controller):

```sh
kubectl annotate pods <pod-name> netdata.cloud/cgroup.name=<desired-name>
```

Netdata extracts `netdata.cloud/*` annotations from pod metadata and uses `netdata.cloud/cgroup.name` to override the resolved name.

#### Verifying the result and troubleshooting

You can check the chart names Netdata created through the Agent's `/api/v3/contexts` endpoint and filter for the `cgroup_` prefix to confirm the override took effect. Use the chart name shown by that endpoint when referencing the chart in API queries.

:::note
If you see a long alphanumeric string such as `twxae02wzkyy2d19gz4dsj6a-` at the start of a cgroup's chart name, that string is part of the resolved name itself (typically a container ID or Kubernetes UID from the cgroup path), not a prefix added by Netdata. Setting `netdata.cloud/cgroup.name` replaces it with your own value.
:::

If no override is set and Netdata cannot resolve a friendly name via Docker, Kubernetes, or Podman, the raw cgroup path is used as the display name.

### Alerts

CPU and memory limits are watched and used to raise alerts. Memory usage for every cgroup is checked against `ram`
and `ram+swap` limits. CPU usage for every cgroup is checked against `cpuset.cpus`
and `cpu.cfs_period_us` + `cpu.cfs_quota_us` pair assigned for the cgroup. Configuration for the alerts is available
in `health.d/cgroups.conf` file.

## Monitoring systemd services

Netdata monitors **systemd services**.

### Monitored systemd service metrics

- CPU utilization
- Memory
- Writeback Memory
- Memory Paging I/O
- Memory Page Faults
- Memory Limit Failures
- Used Memory
- Disk Read/Write Bandwidth
- Disk Read/Write Operations
- Throttle Disk Read/Write Bandwidth
- Throttle Disk Read/Write Operations
- Queued Disk Read/Write Operations
- Merged Disk Read/Write Operations
- Number of Processes

### How to enable cgroup accounting on systemd systems that is by default disabled

:::note
On systems using cgroup v2 (the default on Ubuntu 22.04+, Fedora 31+, RHEL 9+, and other modern distributions), memory accounting is always enabled — no kernel boot parameters or systemd changes are needed. To check which cgroup version is active: `stat -fc %T /sys/fs/cgroup` returns `cgroup2fs` for cgroup v2 or `tmpfs` for cgroup v1.
:::

You can verify there is no accounting enabled, by running `systemd-cgtop`. The program will show only resources for
cgroup `/`, but all services will show nothing.

To enable cgroup accounting, execute this:

```sh
sed -e 's|^#Default\(.*\)Accounting=.*$|Default\1Accounting=yes|g' /etc/systemd/system.conf >/tmp/system.conf
```

To see the changes it made, run this:

```sh
# diff /etc/systemd/system.conf /tmp/system.conf
40,44c40,44
< #DefaultCPUAccounting=no
< #DefaultIOAccounting=no
< #DefaultBlockIOAccounting=no
< #DefaultMemoryAccounting=no
< #DefaultTasksAccounting=yes
---
> DefaultCPUAccounting=yes
> DefaultIOAccounting=yes
> DefaultBlockIOAccounting=yes
> DefaultMemoryAccounting=yes
> DefaultTasksAccounting=yes
```

If you are happy with the changes, run:

```sh
# copy the file to the right location
sudo cp /tmp/system.conf /etc/systemd/system.conf

# restart systemd to take it into account
sudo systemctl daemon-reexec
```

Note: `systemctl daemon-reload` does not reload systemd's own configuration — use `systemctl daemon-reexec` instead.

Now, when you run `systemd-cgtop`, services will start reporting usage (if it does not, restart any service to wake it
up). Refresh your Netdata dashboard, and you will have the charts too.

In case memory accounting is missing (cgroup v1 systems), you will need to enable it at your kernel, by appending the following kernel boot
options and rebooting:

```sh
cgroup_enable=memory swapaccount=1
```

You can add the above, directly at the `linux` line in your `/boot/grub/grub.cfg` or appending them to
the `GRUB_CMDLINE_LINUX` in `/etc/default/grub` (in which case you will have to run `update-grub` before rebooting). On
DigitalOcean debian images you may have to set it at `/etc/default/grub.d/50-cloudimg-settings.cfg`.

Which systemd services are monitored by Netdata is determined by the following pattern list:

```text
[plugin:cgroups]
 cgroups to match as systemd services =  !/system.slice/*.service/*.service  /system.slice/*.service
```

## Monitoring ephemeral containers

Netdata monitors containers automatically when it is installed at the host, or when it is installed in a container that
has access to the `/proc` and `/sys` filesystems of the host.

Network interfaces and cgroups (containers) are self-cleaned. When a network interface or container stops, Netdata might
log a few errors in error.log complaining about files it cannot find, but immediately:

1. It will detect this is a removed container or network interface
2. It will freeze/pause all alerts for them
3. It will mark their charts as obsolete
4. Obsolete charts are not be offered on new dashboard sessions (so hit F5 and the charts are gone)
5. Existing dashboard sessions will continue to see them, but of course they will not refresh
6. Obsolete charts will be removed from memory, 1 hour after the last user viewed them (configurable)
   with `[db].cleanup obsolete charts after = 3600` (at `netdata.conf`).

### Monitored container metrics

- CPU Usage
- CPU Usage within the limits
- CPU Throttled Runnable Periods
- CPU Throttled Time Duration
- CPU Time Relative Share
- CPU Usage Per Core
- Memory Usage
- Writeback Memory
- Memory Activity
- Memory Page Faults
- Used RAM within the limits
- Memory Utilization
- Memory Limit Failures
- Used Memory
- I/O Bandwidth (all disks)
- Serviced I/O Operations (all disks)
- Throttle I/O Bandwidth (all disks)
- Throttle Serviced I/O Operations (all disks)
- Queued I/O Operations (all disks)
- Merged I/O Operations (all disks)
- CPU some pressure
- CPU some pressure stall time
- CPU full pressure
- CPU full pressure stall time
- Memory some pressure
- Memory some pressure stall time
- Memory full pressure
- Memory full pressure stall time
- IRQ some pressure
- IRQ some pressure stall time
- IRQ full pressure
- IRQ full pressure stall time
- I/O some pressure
- I/O some pressure stall time
- I/O full pressure
- I/O full pressure stall time
- Number of processes

Network interfaces inside containers are discovered and monitored by the cgroups plugin using
[`cgroup-network-helper.sh`](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/cgroup-network-helper.sh).
