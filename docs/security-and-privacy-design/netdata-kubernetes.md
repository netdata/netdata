# Netdata Container Security Considerations

This document outlines the necessary permissions and security considerations when deploying Netdata in containerized environments like Kubernetes. To effectively monitor both the host system and other containers, the Netdata agent container requires specific privileges. Understanding these permissions is essential for balancing robust monitoring capabilities with a strong security posture.

## Recommended Permissions for Comprehensive Monitoring

For full monitoring capabilities (host-level metrics, process information, and container resource usage), Netdata child pods (agents running on each node) need access to specific host resources and namespaces, as well as certain Linux capabilities.

### Privilege Separation Security Model

Netdata uses a privilege separation model to isolate risks associated with the permissions required for comprehensive observability:

1. **Unprivileged Core**: The Netdata daemon and internal plugins (`proc.plugin`, `cgroups.plugin`, etc.) run as a normal user without access to sensitive system information or capabilities.
2. **Isolated Privileged Components**: When elevated permissions are needed, Netdata uses either dedicated external plugins or helper processes to collect the sensitive information. Only these specific components receive elevated privileges, not the core daemon.

Netdata communicates with its plugins using a text-based protocol, which prevents any binary data exchange between the main daemon and its plugins, minimizing the potential attack surface.

### Netdata Network Exposure

- **Netdata Children**: Limited to `localhost` connectivity, cannot accept external network connections, and only connect outbound to their Netdata Parent for data streaming. They don’t maintain a database on disk.
- **Netdata Parents**: Run in unprivileged containers without requiring host mounts, namespaces, or capabilities. They ingest data from Netdata Children and expose information via APIs.
- **Cloud Connectivity**: Only Netdata Parents need to connect to Netdata Cloud; Children do not require this connection.

### Container Mounts

The following table lists the main host-mounted devices required for monitoring:

| Mount                          | Type     | Node                          | Component & Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
|--------------------------------|----------|-------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `/`                            | hostPath | child                         | • `diskspace.plugin`: Monitor host mount points (Docker only, not Kubernetes)                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `/etc/os-release`              | hostPath | child<br/>parent<br/>k8sState | • `netdata`: Collect OS info                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `/etc/passwd`<br/>`/etc/group` | hostPath | child                         | • `apps.plugin`: Resolve numeric users and groups to names<br/>• `network-viewer.plugin`: Resolve numeric users and groups to names                                                                                                                                                                                                                                                                                                                                                                         |
| `/proc`                        | hostPath | child                         | • `proc.plugin`: Monitor host system resources (CPU, Memory, Network, uptime)<br/>• `apps.plugin`: Monitor all running processes<br/>• `cgroups.plugin`: Detect memory limits and pause containers (improves discovery performance in k8s) <br/>• `cgroup-network`: Discover container virtual network interfaces. Map virtual interfaces in the system namespace to interfaces inside containers <br/>• `network-viewer.plugin`: Monitor TCP/UDP sockets<br/>• `netdata`: Collect system and hardware info |
| `/sys`                         | hostPath | child                         | • `cgroups.plugin`: Monitor containers<br/>• `netdata`: Detect container limits and host hardware<br/>• `proc.plugin`: Detect network interfaces type, RAID devices, ZRAM, GPUs, Numa Nodes, Infiniband, BTRFS, PCI AEC, EDAC MC, KSM, BCACHE, CPU thermal throttling.<br/>• `debugfs.plugin`: Monitor hardware sensors, ZSWAP, NUMA memory fragmentation, PowerCap                                                                                                                                         |
| `/var/log`                     | hostPath | child                         | • `systemd-journal.plugin`: Process and query system logs                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `/var/lib/netdata`*            | hostPath | child                         | • `netdata`: Persist Netdata's identity data                                                                                                                                                                                                                                                                                                                                                                                                                                                                |

**Notes**:

- **Persistence**: The `/var/lib/netdata` mount is critical for maintaining node identity across pod recreations (updates, reinstalls, rescheduling). This directory stores only metadata, not the time-series database, and requires minimal storage space. Without it, each pod recreation registers as a new node.
- **Read-Only Access**: All mounts except `/var/lib/netdata` are mounted read-only.

### Host Namespaces

Access to host namespaces allows Netdata to observe network activity and processes from the host's perspective:

| Namespace    | Node  | Component & Purpose                                                                                                                                                                                                                              |
|--------------|-------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Host Network | child | • `proc.plugin`: Monitor host's networking stack<br/>• `cgroup-network`: Detect containers' network interfaces<br/>• `network-viewer.plugin`: Discover host's network connections<br/>• `go.d.plugin`: Discover applications running at the host |
| Host PID     | child | • `cgroup-network`: Monitor containers' network interfaces by switching Network Namespaces using process PIDs                                                                                                                                    |

### Container Capabilities

Specific Linux capabilities grant elevated privileges for certain monitoring functions:

| Capability | Component & Purpose                                                                                                                                                                                                                                                                                                                                                                                                             | 
|------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| SYS_ADMIN  | • `cgroup-network`: Links host virtual interfaces to their containers. Without it, interfaces are monitored but not associated with containers.<br/>• `network-viewer.plugin`: Discovers and monitors network connections from containers. Without it, network connections originating from containers will not be monitored at all.                                                                                            |
| SYS_PTRACE | • `apps.plugin`: Collect I/O per running process at the host<br/>• `network-viewer.plugin`: Associates host network connections with specific processes. Without it, connections are monitored but not linked to their source processes.<br/>• `go.d.plugin`: Discovers applications running on the host system and automatically creates monitoring jobs for them. Without it, automatic application discovery won't function. |

**IMPORTANT**: All plugins or helpers using these capabilities are isolated from the rest of Netdata. The main daemon and other plugins can’t use these capabilities, even when granted to the whole container.

## Impact of Security Restrictions on Monitoring Scope

### Operating with Minimal Privileges

When Netdata is deployed without the recommended mounts, namespaces, and capabilities, its monitoring scope becomes significantly restricted:

**Limited To:**

- Kubernetes state metrics and application discovery (with appropriate RBAC permissions)
- Metrics of the Netdata container itself (CPU, memory, internal processes), not the host system

**Missing Capabilities:**

- No host-level metrics (system-wide CPU, memory, network, disk I/O)
- No visibility into processes running outside the Netdata container
- No direct container resource usage monitoring via cgroups
- No access to host system logs

**Identity Issue:**

- Without the persistent volume mount (`/var/lib/netdata`), Netdata will appear as a new node after each pod recreation (during updates, reinstalls, or rescheduling)

### Balanced approach

For environments with stricter security requirements, a balanced approach would be to maintain host mounts and namespaces while excluding certain capabilities. If you must disable some Netdata monitoring features for security reasons, starting with capabilities creates the least monitoring impact while providing the most security benefit.

#### Exclude `SYS_ADMIN` and `SYS_PTRACE` Capabilities

These capabilities grant the broadest system access. If you need to restrict permissions, these can be excluded while preserving many essential monitoring functions.

**Trade-offs when removing these capabilities**:

- Container interfaces are monitored but not associated with containers
- Container network traffic remains invisible
- No per-process disk I/O metrics
- Network connections are monitored but not linked to specific processes
- Automatic application discovery won't function

## Conclusion

Deploying Netdata for comprehensive system monitoring requires granting access to host resources. While these permissions enable deep visibility, they represent a security trade-off. Administrators must carefully evaluate their organization's security policies and monitoring requirements to determine the appropriate level of privilege for Netdata agents. Running with restricted permissions enhances security isolation but severely limits Netdata's ability to monitor the underlying host and other containerized workloads effectively.
