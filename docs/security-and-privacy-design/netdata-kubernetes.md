# Netdata Container Security Considerations

This document outlines the necessary permissions and security considerations when deploying Netdata within containerized environments, such as Kubernetes. Achieving comprehensive monitoring of the host system and other containers necessitates granting specific privileges to the Netdata agent container. Understanding the implications of these permissions is crucial for balancing observability requirements with security posture.

## Recommended Permissions for Comprehensive Host and Container Monitoring

To enable full monitoring capabilities, including host-level metrics, process information, and container resource usage, the Netdata child pods (agents running on each node) require access to specific host resources and namespaces, as well as certain Linux capabilities.

### Privilege Separation Security Model

Netdata uses a privilege separation model for its processes in order to isolate the risk associated with permissions and capabilities required to achieve comprehensive observability:

1. The Netdata daemon and it internal plugins run unprivileged as a normal user, without any access to sensitive system information or capabilities. This includes `proc.plugin`, `cgroups.plugin` and the core Netdata daemon.
2. When elevated permissions are needed to access sensitive or protected information, either a dedicated/isolated external plugin is used, or an external helper process is utilized to collect the needed information. When Netdata runs in a container, only some of the Netdata external plugins and helpers have elevated privileges, while the core of Netdata with its internal plugins do not.

Netdata talks to its plugins using a text based protocol, which eliminates the possibility of any binary data being exchanged, especially from the Netdata daemon to its plugins.

Using this design, Netdata minimizes the potential attack surface. The main daemon does not require anything special and external plugins perform a hard-coded task for reading some specific sensitive information and returning it back to the main daemon.

### Netdata Network Exposure

Netdata helm charts limit Netdata children connectivity to `localhost`. Netdata children are not allowed to accept any Network connections from outside the local node, and the only outbound network connection they need is towards the Netdata Parent to stream their data. The Netdata children also do not maintain a database on disk. All information they collect is streamed in real-time to their Netdata Parent.

Netdata Parents on the other hand, do not require any mounts, host namespaces, or capabilities. They run in an unprivileged container, ingesting data in real-time from Netdata children and exposing this information via their APIs.

Netdata Children do not need to connect to Netdata Cloud. Netdata Parents only need to connect to Netdata Cloud. 

### Container Mounts

Mounting specific host directories into the Netdata container provides essential data access for various collection plugins.

| Mount | Type | Role | Component | Why |
|:---:|:---:|:---:|:---:|:---|
| `/`| hostPath | child | `diskspace.plugin` | Detect host mount points (only in Docker deployments, not in Kubernetes deployments). |
| `/etc/os-release` | hostPath | child | `netdata` | Collect host labels. |
| `/etc/passwd`<br/>`/etc/group` | hostPath | child | `apps.plugin` | Resolve numeric users and groups to names. |
| `/etc/passwd`<br/>`/etc/group` | hostPath | child | `network-viewer.plugin` | Resolve numeric users and groups to names. |
| `/proc` | hostPath | child | `proc.plugin` | Monitor host system resources (CPU, Memory, Network, uptime, etc). |
| `/proc` | hostPath | child | `apps.plugin` | Monitor all running processes. |
| `/proc` | hostPath | child | `cgroups.plugin` | Detect available memory to calculate container memory limits. Detect paused containers in k8s to improve discovery performance. |
| `/proc` | hostPath | child | `cgroup-network` | Discover container virtual network interfaces and associates them with running containers. |
| `/proc` | hostPath | child | `network-viewer.plugin` | Monitor all TCP/UDP sockets of running processes. |
| `/proc` | hostPath | child<br/>parent<br/>k8sState | `netdata` | Collect system information and detect various system characteristics like number of CPU cores, total and available memory protection, and more. |
| `/sys` | hostPath | child | `cgroups.plugin` | Monitor containers. |
| `/sys` | hostPath | child<br/>parent<br/>k8sState | `netdata` | Detect `netdata` container limits. Detect host hardware (part of system info). |
| `/sys` | hostPath | child | `proc.plugin` | Detect network interface types. Monitor software RAID block devices. Detect ZRAM, GPUs, Numa Nodes, Infiniband, BTRFS, PCI AEC, EDAC MC, KSM, BCACHE, CPU thermal throttling. |
| `/sys` | hostPath | child | `debugfs.plugin` | Monitor hardware sensors, ZSWAP, Numa Memory Fragmentation, PowerCap. |
| `/var/log` | hostPath | child | `systemd-journal.plugin` | Enable the Logs pipeline of Netdata to process and query system logs. |
| `/var/lib/netdata`* | hostPath | child | `netdata` | Persist of Netdata's private data. |

Notes:

-   **Persistence Note:** The `/var/lib/netdata` host path mount (configurable via `{{ .Values.child.persistence.hostPath }}/var/lib/netdata` in Helm charts) is critical for maintaining node identity across restarts. This directory stores metadata, _not_ the time-series database, and is relatively small. Without this persistence, each pod restart registers as a new node.
- **Read-Only Mounts**: All the mounts described above, except `/var/lib/netdata`, are mounted read-only.

### Host Namespaces

Utilizing host namespaces allows Netdata to observe network activity and processes as they appear on the host, rather than being confined to the container's isolated view.

| Namespace | Role | Component | Why |
|:---:|:---:|:---:|:---|
| Host Network Namespace | child | `proc.plugin` | Monitor host's networking stack. |
| Host Network Namespace | child | `cgroup-network` | Detect containers' network interfaces. |
| Host Network Namespace | child | `network-viewer.plugin` | Discover host's network connections. |
| Host Network Namespace | child | `go.d.plugin` | Discover applications running at the host. |
| Host PID Namespace | child | `cgroup-network` | Monitor containers' network interfaces (it does so by switching Network Namespaces, using the PID of processes associated with containers). |

### Container Capabilities

Specific Linux capabilities grant elevated privileges necessary for certain monitoring functions, particularly those involving process inspection and namespace manipulation.

| Capability | Role | Component | Why |
|:---:|:---:|:---:|:---|
| SYS_ADMIN | child | `cgroup-network` | Associate containers' network interfaces with the containers (it does so by switching Network Namespaces). Without it, `veth` network interfaces will not be associated to their respective containers, so they will be monitored as host network interfaces. |
| SYS_ADMIN | child | `network-viewer.plugin` | Discover containers' network connections (it does so by switching Network Namespaces). Without it, network connections of other containers will not be monitored, limiting the scope of network connections to the host system. |
| SYS_PTRACE | child | `apps.plugin` | Collect the I/O per running process at the host (including the ones in containers). Without it, processes will be monitored excluding their physical or logical disk I/O. |
| SYS_PTRACE | child | `network-viewer.plugin` | Discover host's network connections per application. Without it all host network connections will still be monitored, but Netdata will not be able to associate them with processes. |
| SYS_PTRACE | child | `go.d.plugin` | Discover listening applications running at the host. Without it, localhost service discovery (not the kubernetes one), `go.d.plugin` will judge about the applications running based only on port number, without the process name. |

IMPORTANT: All the plugins or helpers that utilize these capabilities are isolated from the rest of Netdata. This means that all the other plugins of Netdata and the main Netdata daemon cannot utilize these capabilities, even when the capabilities have been given to the whole container Netdata is installed.

## Impact of Security Restrictions on Monitoring Scope

### Operating with Minimal Privileges (Restricted Permissions)

If Netdata child pods are deployed without the aforementioned host path mounts, host namespace access, and container capabilities, the monitoring scope will be significantly reduced.

-   **Functionality:** Netdata will start and operate, but it will primarily monitor **only the resources consumed by the Netdata container itself**. This includes its own CPU/memory usage, internal processes, and network activity within its isolated namespace.
-   **Limitations:** Host-level metrics (overall CPU, memory, network stack, disk I/O), processes running outside the Netdata container, direct container resource usage monitoring (via cgroups), and host system logs will **not** be available.
-   **Kubernetes Integration:** If appropriate RBAC permissions are granted to query the Kubernetes API, Netdata can still provide Kubernetes state metrics (e.g., pod counts, node status). Kubernetes-based application discovery may also function, allowing Netdata to collect metrics from _other_ containers if they expose compatible endpoints accessible via the K8s API/network, but without the deep host-level process correlation.
-   **Persistence Issue:** Without the persistent `/var/lib/netdata` volume mount, the Netdata agent will lose its unique identity upon restart. Each time the pod is rescheduled or restarted, it will appear as a completely new node in the Netdata UI or Cloud dashboard.

### Balanced approach

This approach aims to provide substantial host and container observability while significantly reducing the security risks compared to the "full monitoring" configuration. It operates on the principle of least privilege, excluding the most dangerous capabilities and mounts by default, while retaining access needed for core monitoring functions. This involves accepting certain trade-offs in the depth or context of collected data.

#### Capabilities: Exclude `SYS_ADMIN` and `SYS_PTRACE`

These capabilities grant excessive privileges with high potential for misuse or exploitation. `SYS_ADMIN` offers broad administrative control, while `SYS_PTRACE` allows invasive inspection of any process. Excluding them dramatically reduces the potential impact of a container compromise.

**Impact:**
* Container network interfaces (`veth`) may appear as host interfaces, lacking direct container attribution within Netdata's network interface metrics.
* Direct discovery of _other_ containers' network connections via namespace switching will be disabled.
* Per-process physical/logical disk I/O metrics via `apps.plugin` will be unavailable.
* Network connections and listening ports will not be directly associated with specific process names by `network-viewer.plugin` or `go.d.plugin`.

We suggest to keep the rest (mounts and host namespaces) enabled, so that Netdata can still provide its full monitoring features.

## Conclusion

Deploying Netdata for comprehensive system monitoring requires granting access to host resources. While these permissions enable deep visibility, they represent a security trade-off. Administrators must carefully evaluate their organization's security policies and monitoring requirements to determine the appropriate level of privilege for Netdata agents. Running with restricted permissions enhances security isolation but severely limits Netdata's ability to monitor the underlying host and other containerized workloads effectively.
