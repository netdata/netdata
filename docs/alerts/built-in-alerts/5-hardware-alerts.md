# 11.5 Hardware and Sensor Alerts

Hardware monitoring provides visibility into infrastructure that is often neglected until failure occurs. These alerts protect physical infrastructure health.

:::note

Hardware monitoring requires collector support for your specific hardware. Check that IPMI, SMART, and sensor collectors are enabled for your platform.

:::

## 11.5.1 RAID Monitoring

### raid_degraded

Fires when an array has lost redundancy, which could mean a second disk failure would cause data loss.

**Context:** `adaptecraid.logical_device_status`
**Thresholds:** CRIT > 0

### raid_disk_failed

Tracks individual disk failures within RAID arrays.

**Context:** `adaptecraid.physical_device_state`
**Thresholds:** CRIT > 0

## 11.5.2 Power Monitoring

### ups_battery_charge

Monitors remaining battery capacity on UPS-equipped systems.

**Context:** `apcupsd.ups_battery_charge`
**Thresholds:** WARN < 25%, CRIT < 10%

### ups_on_battery

Fires immediately when mains power fails and system switches to battery.

**Context:** `apcupsd.ups_status`
**Thresholds:** CRIT > 0

### ups_load_percentage

Tracks UPS load percentage to prevent overloading.

**Context:** `apcupsd.ups_load_capacity_utilization`
**Thresholds:** WARN > 80%, CRIT > 90%

## 11.5.3 BMC/IPMI Monitoring

### bmc_temp

Monitors Baseboard Management Controller temperature for servers with IPMI.

**Context:** `ipmi.sensor_state`
**Thresholds:** WARN > 80C, CRIT > 90C

### bmc_fan_speed

Monitors BMC-controlled fan speeds.

**Context:** `ipmi.sensor_state`
**Thresholds:** WARN < 1000 RPM

### bmc_power_consumption

Tracks power consumption against baseline for anomaly detection.

**Context:** `ipmi.sensor_state`
**Thresholds:** WARN > baseline * 1.2

## Related Sections

- [11.1 Application Alerts](3-application-alerts.md) - Database, web server, cache, and message queue alerts
- [11.2 Container Alerts](2-container-alerts.md) - Docker and Kubernetes monitoring
- [11.3 Network Alerts](4-network-alerts.md) - Network interface and protocol monitoring
- [11.4 System Resource Alerts](1-system-resource-alerts.md) - CPU, memory, disk, and load alerts