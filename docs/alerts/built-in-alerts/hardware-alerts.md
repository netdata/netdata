# 11.5 Hardware and Sensor Alerts

Hardware monitoring provides visibility into infrastructure that is often neglected until failure occurs. These alerts protect physical infrastructure health.

:::note
Hardware alerts require appropriate collectors to be enabled (SMART, IPMI, RAID monitoring tools).
:::

## 11.5.1 RAID Monitoring

RAID alerts detect array degradation before complete failure.

### raid_degraded

Fires when an array has lost redundancy, which could mean a second disk failure would cause data loss.

| Context | Thresholds |
|---------|------------|
| `raid.status` | CRIT > 0 |

### raid_disk_failed

Tracks individual disk failures within RAID arrays.

| Context | Thresholds |
|---------|------------|
| `raid.disk` | CRIT > 0 |

## 11.5.2 SMART Monitoring

SMART alerts provide early warning of disk failure before complete breakdown.

### smart_self_test

Monitors the results of regular SMART self-tests. Self-test failures indicate imminent disk failure.

| Context | Thresholds |
|---------|------------|
| `smart.test` | WARN > 0, CRIT > 0 |

### smart_reallocated_sectors

Tracks bad sector remapping which indicates the disk is beginning to fail.

| Context | Thresholds |
|---------|------------|
| `smart.sectors` | WARN > 0 |

### smart_pending_sectors

Monitors pending sector remaps that indicate imminent failure.

| Context | Thresholds |
|---------|------------|
| `smart.pending` | WARN > 0 |

### smart_wear_level

For SSDs, tracks remaining write endurance.

| Context | Thresholds |
|---------|------------|
| `smart.wear` | WARN < 10% remaining |

## 11.5.3 Temperature Monitoring

Temperature alerts prevent thermal damage to hardware components.

### sensor_temperature

Monitors hardware temperatures with thresholds that vary by device specifications.

| Context | Thresholds |
|---------|------------|
| `sensors.temperature` | WARN > 80C, CRIT > 90C |

### fan_speed_low

Detects when fans are spinning below expected RPM, indicating potential cooling failure.

| Context | Thresholds |
|---------|------------|
| `sensors.fan` | WARN < 90% of expected |

### fan_speed_zero

Critical alert for completely stopped fans.

| Context | Thresholds |
|---------|------------|
| `sensors.fan` | CRIT == 0 |

## 11.5.4 Power Monitoring

UPS and power supply monitoring for infrastructure protection.

### ups_battery_charge

Monitors remaining battery capacity on UPS-equipped systems.

| Context | Thresholds |
|---------|------------|
| `ups.battery` | WARN < 25%, CRIT < 10% |

### ups_on_battery

Fires immediately when mains power fails and system switches to battery.

| Context | Thresholds |
|---------|------------|
| `ups.status` | CRIT > 0 |

### ups_input_voltage

Monitors input voltage for instability or power quality issues.

| Context | Thresholds |
|---------|------------|
| `ups.input` | WARN > 10% deviation |

### ups_load_percentage

Tracks UPS load percentage to prevent overloading.

| Context | Thresholds |
|---------|------------|
| `ups.output` | WARN > 80%, CRIT > 90% |

## 11.5.5 BMC/IPMI Monitoring

Baseboard Management Controller alerts for remote server management.

### bmc_temp

Monitors Baseboard Management Controller temperature for servers with IPMI.

| Context | Thresholds |
|---------|------------|
| `ipmi.temperature` | WARN > 80C, CRIT > 90C |

### bmc_fan_speed

Monitors BMC-controlled fan speeds.

| Context | Thresholds |
|---------|------------|
| `ipmi.fans` | WARN < 1000 RPM |

### bmc_power_consumption

Tracks power consumption against baseline for anomaly detection.

| Context | Thresholds |
|---------|------------|
| `ipmi.power` | WARN > baseline * 1.2 |

## Related Sections

- [11.1 System Resource Alerts](system-resource-alerts.md) - CPU, memory, disk, network
- [11.2 Network Alerts](network-alerts.md) - Network monitoring
- [11.3 Application Alerts](application-alerts.md) - Application-specific alerts
- [11.4 Container Alerts](container-alerts.md) - Container metrics