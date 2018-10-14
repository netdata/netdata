# cpufreq

This module shows the current CPU frequency as set by the cpufreq kernel
module.

**Requirement:**
You need to have `CONFIG_CPU_FREQ` and (optionally) `CONFIG_CPU_FREQ_STAT`
enabled in your kernel.

This module tries to read from one of two possible locations. On
initialization, it tries to read the `time_in_state` files provided by
cpufreq\_stats. If this file does not exist, or doesn't contain valid data, it
falls back to using the more inaccurate `scaling_cur_freq` file (which only
represents the **current** CPU frequency, and doesn't account for any state
changes which happen between updates).

It produces one chart with multiple lines (one line per core).

### configuration

Sample:

```yaml
sys_dir: "/sys/devices"
```

If no configuration is given, module will search for cpufreq files in `/sys/devices` directory.
Directory is also prefixed with `NETDATA_HOST_PREFIX` if specified.

---
