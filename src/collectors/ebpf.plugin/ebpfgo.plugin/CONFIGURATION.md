# eBPF Plugin Configuration

The eBPF plugin uses the Netdata agent orchestration framework for configuration management, supporting both traditional YAML files and dynamic configuration (DynCfg) via functions.

## Configuration Sources

Configuration is loaded in this priority order:

1. **DynCfg (Dynamic Configuration)** - Runtime configuration via agent functions (highest priority)
2. **YAML Config Files** - `/etc/netdata/ebpf.d/*.yaml` or `.conf`
3. **Legacy Config Files** - `/etc/netdata/ebpf.d/*.conf` (backward compatible)

## Collectors

The eBPF plugin provides five collectors:

### cachestat
**Description**: Page cache hit ratio and dirty page statistics

**Config Options**:
- `enabled` (bool, default: true) - Enable/disable collector
- `update_every` (int, default: 1) - Collection interval in seconds
- `apps` (bool, default: false) - Per-application metrics (higher CPU usage)
- `cgroups` (bool, default: false) - Per-cgroup metrics (higher CPU usage)
- `per_core_stats` (bool, default: false) - Per-CPU core statistics
- `kernel_debugfs` (string, default: "/sys/kernel/debug") - Debugfs path

### dcstat
**Description**: Directory cache statistics

**Config Options**:
- `enabled` (bool, default: true) - Enable/disable collector
- `update_every` (int, default: 1) - Collection interval in seconds
- `apps` (bool, default: false) - Per-application metrics
- `cgroups` (bool, default: false) - Per-cgroup metrics

### fd
**Description**: File descriptor statistics (open/close calls)

**Config Options**:
- `enabled` (bool, default: true) - Enable/disable collector
- `update_every` (int, default: 1) - Collection interval in seconds
- `apps` (bool, default: false) - Per-application metrics
- `cgroups` (bool, default: false) - Per-cgroup metrics

### socket
**Description**: Network socket statistics (send/receive)

**Config Options**:
- `enabled` (bool, default: true) - Enable/disable collector
- `update_every` (int, default: 1) - Collection interval in seconds

### dns
**Description**: DNS query statistics

**Config Options**:
- `enabled` (bool, default: true) - Enable/disable collector
- `update_every` (int, default: 1) - Collection interval in seconds

## Configuration Examples

### YAML Configuration

```yaml
# /etc/netdata/ebpf.d/cachestat.yaml
jobs:
  cachestat:
    enabled: yes
    update_every: 1
    apps: yes
    cgroups: yes
```

### Legacy Configuration (Backward Compatible)

```conf
# /etc/netdata/ebpf.d/cachestat.conf
[cachestat]
  enabled = yes
  update_every = 1
  apps = yes
  cgroups = yes
```

## Performance Considerations

- **Per-app metrics** (`apps: yes`): ~5-10% CPU overhead per collector
- **Per-cgroup metrics** (`cgroups: yes`): ~5-10% CPU overhead per collector
- **Per-core stats** (`per_core_stats: yes`): ~2-5% CPU overhead per collector

Enable these only if you need the granular data.

## Dynamic Configuration (DynCfg)

The Netdata UI can dynamically configure eBPF collectors at runtime via the agent's function interface. Changes take effect on the next collection cycle without restarting the plugin.

## Integration with Agent Orchestration

The eBPF plugin uses the Netdata agent orchestration framework, which provides:

- **Automatic configuration loading** from YAML files
- **Configuration validation** against JSON schemas
- **Dynamic configuration** via agent functions
- **Automatic collector lifecycle management** (Init → Check → Collect → Cleanup)
- **Graceful shutdown** handling

Each collector is implemented as a CollectorV2 that:
1. Loads configuration from the agent
2. Initializes eBPF programs in `Init()`
3. Validates readiness in `Check()`
4. Collects metrics in `Collect()`
5. Cleans up resources in `Cleanup()`

## Troubleshooting

### Collector won't start
1. Check `/var/log/netdata/agent.log` for errors
2. Verify configuration is valid YAML
3. Ensure eBPF programs can be loaded (requires Linux kernel support)
4. Check kernel version (requires 4.15+)

### High CPU usage
- Disable `apps` and `cgroups` if not needed
- Increase `update_every` to reduce collection frequency
- Check for kernel memory pressure

### No metrics appearing
1. Verify collector is enabled in config
2. Check that eBPF probes attached successfully
3. Ensure target system calls are being executed
4. Review eBPF program logs in agent debug output
