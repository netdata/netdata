<!-- markdownlint-disable-file -->

# Quick Start

Get one device polling in a few minutes, confirm Netdata auto-detected it, and read its interfaces. Do this on the Agent nearest the device — that Agent becomes your site's [SNMP hub](/docs/npm/device-metrics/README.md).

## Before you start

You need:

- A running Netdata Agent on the device's LAN.
- SNMP enabled on the device, with a community string (SNMPv2c) or an SNMPv3 user.
- The device reachable from the Agent on UDP/161.

Confirm reachability from the hub before touching Netdata:

```bash
snmpget -v2c -c <community> <device-ip> .1.3.6.1.2.1.1.3.0
```

A `sysUpTime` value back means SNMP works. No answer means a firewall, ACL, or credential problem to fix first — not a Netdata issue.

## 1. Add the device

Open the SNMP configuration:

```bash
cd /etc/netdata
sudo ./edit-config go.d/snmp.conf
```

Add one job (SNMPv2c shown; for SNMPv3 see [Configuration](/docs/npm/device-metrics/configuration.md)):

```yaml
jobs:
  - name: core-switch-1
    hostname: 10.0.0.1
    community: public
```

Save and reload Netdata (for example `sudo systemctl reload netdata`, or restart per your install).

## 2. Confirm auto-detection

Open the Netdata dashboard. Within a poll cycle or two the device appears as **its own node** named after the job. You did not list any OIDs — the collector read the device's `sysObjectID`/`sysDescr` and applied every matching profile.

Check two things:

- The node carries **vendor/model labels** and **vendor-specific charts**, not just a bare interface count. That confirms a real profile matched, not only generic collection. If it's bare, see [Validation](/docs/npm/device-metrics/validation.md) and [Troubleshooting](/docs/npm/device-metrics/troubleshooting.md).
- **Interface charts** show traffic, errors, discards, and operational status per port.

## 3. Read the interfaces

Run the **`snmp:interfaces`** Function (Functions tab, pick the device) for a live, sortable table of every interface — traffic, status, errors, and discards — served from data already collected, with no extra requests to the device. If the device runs BGP or exposes licenses, `snmp:bgp-peers` and `snmp:licenses` are there too.

## Next steps

- **Add credentials and more devices** — [Configuration](/docs/npm/device-metrics/configuration.md) (SNMPv3, per-device intervals, multiple jobs).
- **Make sure the data is trustworthy** — [Validation](/docs/npm/device-metrics/validation.md).
- **Plan for many devices or sites** — [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).
- **Add the other legs** — [SNMP Traps](/docs/npm/snmp-traps/README.md), [Network Flows](/docs/npm/network-flows/README.md), and topology, all on the same hub.
