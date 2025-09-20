# Fleet Deployment and Configuration Management

As infrastructures grow from a handful of servers to thousands of nodes across mixed environments (Linux, Kubernetes, Windows, macOS, FreeBSD), managing observability agents becomes one of the most painful operational tasks.

**Without a coherent strategy, teams face:**
- Dozens of exporters or collectors, each with its own configs and update cycles
- Manual configuration files scattered across nodes
- Service discovery gaps that lead to blind spots
- Downtime during upgrades and redeployments
- Compliance challenges when configuration states drift

## How Netdata Solves It

**Netdata's Fleet Management Philosophy:**
- **Zero-Configuration**: We eliminate 99% of manual configuration through comprehensive auto-discovery on supported platforms
- **Single Agent**: The Netdata Agent replaces dozens of exporters, dramatically reducing configuration complexity
- **Automatic Everything**: Installation and updates are fully automated when possible, with zero-downtime updates and automatic rollback
- **Flexible Management**: Choose Infrastructure as Code (IaC) for compliance, Dynamic Configuration for agility, or combine both
- **Strong Backwards Compatibility**: Ensures upgrades don't break existing configurations and data - Netdata maintains compatibility across versions allowing seamless updates

## Platform Capabilities

| Capability                         | Linux                                                                                                                   | Kubernetes                                                                       | FreeBSD        | macOS          | Windows                                                                                |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | -------------- | -------------- | -------------------------------------------------------------------------------------- |
| **Auto-deploy**                    | ✅ [kickstart.sh](https://learn.netdata.cloud/docs/netdata-agent/installation/one-line-installer-for-all-linux-systems)¹ | ✅ [Helm](https://learn.netdata.cloud/docs/netdata-agent/installation/kubernetes) | ✅ kickstart.sh | ✅ kickstart.sh | ✅ [MSI](https://learn.netdata.cloud/docs/netdata-agent/installation/windows)² (silent) |
| **Auto-update**                    | ✅ Built-in                                                                                                              | ✅ Built-in                                                                       | ✅ Built-in     | ✅ Built-in     | ⚠️ Manual³                                                                              |
| **Auto-discover system metrics**   | ✅ Yes                                                                                                                   | ✅ Yes                                                                            | ✅ Yes          | ✅ Yes          | ✅ Yes                                                                                  |
| **Auto-discover all processes**    | ✅ Yes                                                                                                                   | ✅ Yes                                                                            | ✅ Yes          | ✅ Yes          | ✅ Yes                                                                                  |
| **Auto-discover containers & VMs** | ✅ Yes                                                                                                                   | ✅ Yes                                                                            | ❌ No           | ❌ No           | ✅ Hyper-V                                                                              |
| **Auto-discover Docker apps**      | ✅ Yes                                                                                                                   | ✅ Via k8s⁴                                                                       | ✅ If Docker    | ✅ If Docker    | ✅ If Docker                                                                            |
| **Auto-discover system services**  | ✅ systemd                                                                                                               | ✅ Yes                                                                            | ⚠️ Limited      | ⚠️ launchd      | ✅ Windows Services                                                                     |
| **Auto-discover enterprise apps**  | ✅ netlistensd⁵                                                                                                          | ✅ Via k8s                                                                        | ❌ Manual       | ❌ Manual       | ✅ perflib⁶                                                                             |
| **Infrastructure as Code**         | ✅ Yes                                                                                                                   | ✅ Yes                                                                            | ✅ Yes          | ✅ Yes          | ✅ Yes                                                                                  |
| **Dynamic Configuration**          | ✅ Yes                                                                                                                   | ✅ Yes                                                                            | ✅ Yes          | ✅ Yes          | ✅ Yes                                                                                  |

**Legend**: ✅ Full support | ⚠️ Partial support | ❌ Not available

**Footnotes**:
1. **kickstart.sh**: [Universal installer script](https://learn.netdata.cloud/docs/netdata-agent/installation/one-line-installer-for-all-linux-systems) that auto-detects the best installation method
2. **MSI**: Microsoft Software Installer package for Windows deployment
3. **Manual**: Auto-updates coming Q3 2025; currently requires PowerShell/SCCM/GPO automation
4. **Via k8s**: Uses Kubernetes API for service discovery instead of Docker API
5. **netlistensd**: Local network service discovery - scans for listening services on Linux
6. **perflib**: Windows Performance Library - provides metrics for Windows applications

## Configuration Management Paradigms

The observability industry uses two primary approaches for configuration management:

### Infrastructure as Code (IaC)

IaC treats configurations as code artifacts that can be versioned, reviewed, and deployed through automated processes.

**Common tools**:
- **Ansible**: Agentless automation using YAML playbooks
- **Terraform**: Declarative infrastructure provisioning
- **Puppet/Chef**: Agent-based configuration management
- **Salt**: Event-driven automation platform

**Typical workflow**:
1. Define configuration in code
2. Store in version control
3. Review through pull requests
4. Deploy via CI/CD pipeline
5. Validate deployment state

### Dynamic Configuration Management

Dynamic configuration uses a central control plane to manage configurations without requiring code deployments.

**Common implementations**:
- Web-based configuration interfaces
- API-driven configuration updates
- Real-time configuration synchronization
- Central configuration databases

**Typical workflow**:
1. Access central management interface
2. Modify configuration through UI/API
3. Changes propagate to agents
4. Validation occurs at edge nodes
5. Status reported back to central system

### What to Use

| Approach          | When to Use                           | How                                                                                                                                             |
| ----------------- | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| **IaC Only**      | Compliance requirements, audit trails | Ansible, Terraform, Puppet, Chef                                                                                                                |
| **Dynamic Only**  | Small teams, rapid iteration          | Netdata Cloud UI or REST API                                                                                                                    |
| **Hybrid (Best)** | Most organizations                    | Base config in Git, credentials/thresholds via [UI](https://learn.netdata.cloud/docs/netdata-agent/configuration/dynamic-configuration-manager) |

## How Netdata Solves Deployment Challenges at Scale

Netdata addresses fleet deployment through a comprehensive strategy that minimizes operational overhead while maximizing flexibility:

### Universal Single-Script Installation

Netdata provides a unified installation experience across all platforms (except Windows) through `kickstart.sh`, which implements an intelligent cascade:

**Installation priority order on Linux**:
1. **Native binary packages** - For supported distributions (RPM, DEB, SUSE packages)
2. **Static builds** - Pre-compiled binaries for x86_64, armv7l, armv6l, aarch64 architectures
3. **Source compilation** - Automatic build from source as final fallback

**Platform-specific behavior**:
- **Linux/FreeBSD/macOS**: Single command installation via `kickstart.sh`
- **FreeBSD/macOS**: Direct compilation from source (no native packages or static builds)
- **Windows**: Separate MSI installer (auto-updates not yet available)

### Automatic Updates

All Netdata installations (except Windows) auto-update to the latest version:
- Updates are scheduled via systemd timers, cron, or interval scripts
- Maintains the same release channel (stable/nightly) as initially installed
- Zero-downtime updates with automatic rollback on failure
- No manual intervention required for security patches and new features

**Strong backwards compatibility**:
Netdata ensures upgrades don't break existing configurations and data - Netdata maintains compatibility across versions allowing seamless updates (see [Netdata Infrastructure](https://learn.netdata.cloud/docs/welcome-to-netdata) for architectural details)

## How Netdata Solves Fleet Configuration Management at Scale

Netdata supports both IaC and Dynamic Configuration while minimizing the need for manual configuration through extensive auto-discovery capabilities.

### Auto-Discovery and Zero Configuration

Netdata's primary operational approach is to eliminate manual configuration through comprehensive auto-discovery:
- **On Linux**: Achieves true zero-configuration - 99% of services are automatically detected and monitored
- **On Kubernetes**: Uses Kubernetes API for comprehensive service discovery
- **On Windows**: Auto-discovers enterprise applications via perflib
- **On FreeBSD/macOS**: System metrics are automatic, but application monitoring requires manual configuration
- Discovers new services at runtime through periodic scanning (where supported)

### Infrastructure as Code (IaC) Support

For organizations with established DevOps practices, Netdata fully supports configuration management through traditional IaC tools like [Ansible](https://learn.netdata.cloud/docs/netdata-agent/installation/ansible), allowing version-controlled, auditable deployments.

### Dynamic Configuration via the dashboard

Through [Netdata Cloud and the Dynamic Configuration Manager](https://learn.netdata.cloud/docs/netdata-agent/configuration/dynamic-configuration-manager) and authenticated Netdata Agent and Parent dashboards (the user must sign-in to the dashboard), users can manage collector configurations and alert rules across their entire fleet without touching configuration files, restarting or redeploying agents.

### Configuration Priority Order

When multiple configuration sources exist for the same component, Netdata applies them in the following priority order (highest to lowest):

1. **[Dynamic Configuration (DynCfg)](https://learn.netdata.cloud/docs/netdata-agent/configuration/dynamic-configuration-manager)** - Runtime configurations via Netdata Cloud UI or API
2. **[User Configuration](https://learn.netdata.cloud/docs/netdata-agent/configuration/configuration)** - Files in `/etc/netdata` or `/opt/netdata/etc/netdata`
3. **Auto-discovered Configuration** - Settings from service discovery mechanisms
4. **Stock Configuration** - Default configuration files shipped with Netdata
5. **Internal Defaults** - Built-in defaults in the code

## Automatic Discovery

### Operating System Metrics

Netdata auto-detects operating system metrics (compute, memory, networking stack, storage, etc) on all platforms.

On Linux, Netdata autodetect all kernel modules and technologies which have been instrumented, including firewalls, DDoS protections systems, storage technologies and filesystems, etc. Usually all these technologies require zero configuration.

Similarly for Windows, Netdata will autodetect everything exposed via Perflib.

### [Process and Application Monitoring](https://learn.netdata.cloud/docs/collecting-metrics/processes-and-system-services/applications) (apps.plugin)
The apps.plugin provides intelligent process tree aggregation and monitoring on all platforms (Linux, FreeBSD, macOS, Windows):

**Intelligent Process Tree Aggregation**:
- Automatically traverses the entire process tree to understand process relationships
- Identifies process managers (spawn servers/orchestrators like systemd, docker, containerd, etc.)
- Creates a finite, manageable set of metrics by intelligently grouping the entire process tree

**Resource Monitoring** (per application group):
- CPU utilization (user/system, context switches)
- Memory usage (real/virtual, page faults)
- Disk I/O (physical/logical reads/writes)
- Network traffic (if eBPF is enabled on Linux)
- File descriptors and handles
- Process/thread counts
- Accumulated uptime

**Key Benefits**:
- Zero configuration required - starts monitoring immediately with intelligent defaults
- Captures both running and exited processes, ensuring short-lived processes are accounted for
- Provides instant visibility into resource usage for any application
- Particularly valuable for shell scripts that spawn numerous short-lived subprocesses
- On Windows, automatically monitors all processes and Windows services

This provides comprehensive application monitoring even for software without specific collectors, making it an essential first line of observability.

### [Applications](https://learn.netdata.cloud/docs/collecting-metrics/collectors-configuration) (go.d.plugin)
The go.d.plugin provides auto-discovery for 150+ applications through multiple mechanisms:

**Service discovery mechanisms**:
- **Local network service discovery (netlistensd)** - Linux-only, uses `local-listeners` binary to detect listening services
- **[Docker container discovery](https://learn.netdata.cloud/docs/collecting-metrics/container-services/docker) (dockersd)** - Discovers applications running in Docker containers
- **[Kubernetes service discovery](https://learn.netdata.cloud/docs/netdata-agent/installation/kubernetes) (k8ssd)** - Discovers services running in Kubernetes pods
- **[SNMP device discovery](https://learn.netdata.cloud/docs/collecting-metrics/network-devices/snmp) (snmpsd)** - Discovers and profiles SNMP-enabled network devices
- **Configuration file scanning** - Detects applications based on their configuration files

**Platform-specific behavior**:

**Linux systems (non-Kubernetes)**:
Full auto-discovery is available through the `local-listeners` utility which:
- Scans for TCP/UDP sockets in LISTEN state every 2 minutes
- Identifies services by their listening ports and process information
- Maintains a cache with 10-minute expiry for discovered services
- Automatically creates collector jobs for recognized services
- Docker container discovery (dockersd) is enabled

**Non-Linux platforms (FreeBSD, macOS, Windows)**:
- Auto-discovery via netlistensd is **NOT available**
- Docker container discovery (dockersd) works if Docker is available

**Discovery status management**:
When services are discovered but cannot be monitored, the Dynamic Configuration (DynCfg) system tracks their status:
- **failed** - Collector failed to connect or collect data
- **incomplete** - Configuration requires additional parameters (e.g., credentials)

Users can view discovered services in the Netdata Cloud UI and supply missing credentials or configuration parameters through the interface, allowing the collectors to retry connection without manual file editing and without restarting Netdata.

Supported applications include databases (MySQL, PostgreSQL, Redis, MongoDB), web servers (NGINX, Apache, HAProxy), message queues (RabbitMQ, Kafka), SNMP, and [many more](https://learn.netdata.cloud/docs/collecting-metrics/collectors-configuration).

### Microsoft Windows

**Installation**: Windows requires the MSI installer instead of kickstart.sh:
```powershell
# Silent installation for fleet deployment
msiexec /i netdata-installer.msi /qn /norestart `
        CLAIMING_TOKEN="YOUR_TOKEN" `
        CLAIMING_ROOMS="YOUR_ROOM_ID" `
        CLAIMING_URL="https://app.netdata.cloud"

# Via Group Policy or SCCM
# Deploy MSI with TRANSFORMS for site-specific configuration
```

**Update Management** (Manual - auto-updates coming):
- Use Windows Update Services (WSUS) or System Center Configuration Manager (SCCM)
- PowerShell DSC (Desired State Configuration) for version enforcement
- Scheduled task to check and download new versions

**[Windows-Specific Auto-Discovery](https://learn.netdata.cloud/docs/netdata-agent/installation/windows)**:
Windows monitoring is handled by the native **windows.plugin** which uses Windows Performance Counters (perflib) to automatically discover and monitor:

**Enterprise Applications** (auto-discovered via perflib):
- **IIS** (Internet Information Services) - Web server metrics, site traffic, requests, connections
- **MS SQL Server** - Database performance, transactions, locks, buffer cache
- **MS Exchange** - Mail server metrics, mailbox statistics, transport queues
- **Active Directory** - Domain controller metrics, LDAP operations, replication
- **Active Directory Certificate Services** - Certificate enrollment, requests, revocations
- **Active Directory Federation Services** - Authentication metrics, token issuance
- **Hyper-V** - Virtual machine metrics, CPU/memory allocation, network statistics
- **ASP.NET** - Application performance, request execution, session state
- **.NET Framework** - CLR performance, garbage collection, exceptions

**System Monitoring** (automatically enabled):
- Windows Services status and health
- Process and thread statistics
- Memory management and paging
- Network interfaces and protocols
- Physical and logical disk performance
- NUMA architecture metrics
- Power supply and battery status
- Thermal zones and sensors
- Semaphore and synchronization objects

**Process Monitoring**:
- apps.plugin provides detailed per-process and per-application monitoring on Windows
- Automatically groups Windows services and applications
- Monitors all processes for CPU, memory, handles, and I/O usage

### Kubernetes Environments
When Netdata runs inside a Kubernetes cluster, it provides comprehensive multi-level discovery:

**Cluster Monitoring**:
- **[Cluster state](https://learn.netdata.cloud/docs/netdata-agent/installation/kubernetes)** (k8s_state): Monitors nodes, pods, containers, deployments, services
- **[Kubelet metrics](https://learn.netdata.cloud/docs/netdata-agent/installation/kubernetes) (k8s_kubelet)**: Container and pod resource usage, volume statistics
- **[Control plane](https://learn.netdata.cloud/docs/netdata-agent/installation/kubernetes)** (kube-proxy, kube-scheduler, kube-controller-manager): When accessible

**[Service Discovery](https://learn.netdata.cloud/docs/collecting-metrics/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents)** (k8ssd):
- Automatically enabled, replacing traditional host-based discovery
- Uses Kubernetes API to monitor all pods and services
- Discovers applications by inspecting pod containers and their exposed ports
- Extracts environment variables, ConfigMaps, and Secrets for configuration
- Groups targets by pod, service, and namespace metadata
- Creates monitoring targets for each container:
  - One target per exposed port if ports are defined
  - One target using pod IP if no ports are exposed
- Tags all metrics with Kubernetes metadata (namespace, pod name, labels, annotations)

**Discovery Behavior in Kubernetes**:
- **k8ssd** becomes the primary discovery mechanism
- **dockersd** is automatically disabled to avoid conflicts
- **netlistensd** (local-listeners) still works if pod has host network access
- Stock configuration files are ignored (only user-provided configs are loaded)
- Application metrics from Prometheus endpoints are automatically collected

Most Kubernetes deployments require no configuration beyond the initial [Helm chart installation](https://learn.netdata.cloud/docs/netdata-agent/installation/kubernetes-helm-chart-reference).

## Comparison with Other Platforms

Understanding how different platforms handle configuration helps in planning migrations or hybrid deployments:

### Configuration Requirements by Platform

| Platform      | Components per Node          | Configuration Method          | Auto-Discovery | Updates & Restarts                   |
| ------------- | ---------------------------- | ----------------------------- | -------------- | ------------------------------------ |
| Netdata       | **Single agent**             | Files, Ansible, or Dynamic UI | **Extensive**  | **Zero-downtime for Dynamic Config** |
| Prometheus    | 5-20 exporters               | YAML files per exporter       | Limited        | Rolling restarts                     |
| Datadog       | Agent + integrations         | YAML files                    | Moderate       | Agent restart                        |
| OpenTelemetry | Collectors + instrumentation | YAML files per collector      | Limited        | Full restart                         |

## Related Documentation
- [Parent-Child Configuration Reference](https://learn.netdata.cloud/docs/netdata-parents/parent-child-configuration-reference)
- [Parent Configuration Best Practices](https://learn.netdata.cloud/docs/netdata-parents/parent-configuration-best-practices)
- [Streaming and Routing Reference](https://learn.netdata.cloud/docs/netdata-parents/streaming-routing-reference)
- [Dynamic Configuration Manager](https://learn.netdata.cloud/docs/netdata-agent/configuration/dynamic-configuration-manager)
