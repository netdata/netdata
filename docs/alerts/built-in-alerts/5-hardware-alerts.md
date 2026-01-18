# 11.5 Hardware and Sensor Alerts

Hardware monitoring provides visibility into infrastructure that is often neglected until failure occurs. These alerts protect physical infrastructure health.

:::note

Hardware monitoring requires collector support for your specific hardware. Check that IPMI, SMART, and sensor collectors are enabled for your platform.

:::

## 11.5.1 RAID Monitoring

### adaptec_raid_logical_device_status

Fires when an array has lost redundancy, which could mean a second disk failure would cause data loss.

**Context:** `adaptecraid.logical_device_status`
**Thresholds:** CRIT > 0 (degraded)

### adaptec_raid_physical_device_state

Tracks individual disk failures within RAID arrays.

**Context:** `adaptecraid.physical_device_state`
**Thresholds:** CRIT > 0 (failed)

## 11.5.2 Power Monitoring

### apcupsd_ups_battery_charge

Monitors remaining battery capacity on UPS-equipped systems.

**Context:** `apcupsd.ups_battery_charge`
**Thresholds:** WARN < 25%, CRIT < 10%

### apcupsd_ups_status_onbatt

Fires immediately when mains power fails and system switches to battery.

**Context:** `apcupsd.ups_status`
**Thresholds:** CRIT > 0 (on battery)

### apcupsd_ups_load_capacity

Tracks UPS load percentage to prevent overloading.

**Context:** `apcupsd.ups_load_capacity_utilization`
**Thresholds:** WARN > 80%, CRIT > 90%

### upsd_ups_battery_charge

NUT UPS battery charge monitoring.

**Context:** `upsd.ups_battery_charge`
**Thresholds:** WARN < 25%, CRIT < 10%

### upsd_10min_ups_load

NUT UPS load percentage.

**Context:** `upsd.ups_load`
**Thresholds:** WARN > 80%, CRIT > 90%

## 11.5.3 BMC/IPMI Monitoring

### ipmi_sensor_state

Monitors Baseboard Management Controller sensor states for servers with IPMI.

**Context:** `ipmi.sensor_state`
**Thresholds:** WARN/CRIT based on sensor type

### ipmi_events

Tracks IPMI event log entries for critical hardware events.

**Context:** `ipmi.events`
**Thresholds:** Any events present

## Related Sections

- [11.1 System Resource Alerts](./1-system-resource-alerts.md) - CPU, memory, disk, and load alerts
- [11.2 Container Alerts](./2-container-alerts.md) - Docker and Kubernetes monitoring
- [11.3 Application Alerts](./3-application-alerts.md) - Database, web server, cache, and message queue alerts
- [11.4 Network Alerts](./4-network-alerts.md) - Network interface and protocol monitoring