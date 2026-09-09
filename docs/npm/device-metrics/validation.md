<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/device-metrics/validation.md"
sidebar_label: "Validation"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Device Metrics"
keywords: ['snmp', 'validation', 'data quality', 'profile match', 'counters', 'trust']
endmeta-->

<!-- markdownlint-disable-file -->

# Validation and Data Quality

Polling that "works" can still be wrong — a device matched only generic profiles, a link reports 32-bit counters, or the hub is quietly falling behind. Run these checks once after you add devices, and again after profile or scale changes, so you trust what the charts say.

## 1. Confirm a real profile matched

Auto-detection is all-or-nothing per profile: a device either matched the right profiles or fell back to generic collection.

- On the device's node, check for **vendor and model labels** and **vendor-specific charts** (sensors, vendor counters, BGP, licensing where applicable) — not just interface counts.
- If you only see generic interface and system metrics, the model isn't covered. Read the device's `sysObjectID`/`sysDescr`, then add a custom profile with a matching `selector` under `/etc/netdata/go.d/snmp.profiles/` — it applies automatically once it matches. See [SNMP Profile Format](/src/go/plugin/go.d/collector/snmp/profile-format.md).

## 2. Confirm utilization uses 64-bit counters

Interface utilization is only trustworthy on 64-bit high-capacity counters at 1G and above.

- Watch a busy interface. **Sane, continuous utilization** is correct. **Occasional impossible spikes** (terabits/sec) mean a 32-bit counter wrapped and the device isn't exposing — or the profile isn't collecting — the HC variant for that interface.
- Cross-check one interface's utilization against the **device's own counters** (`show interface`, the device UI). They should be close. A large gap points at a counter or profile problem.

## 3. Confirm polling is clean

A hub that is overloaded reports stale or flapping data without saying so.

- **SNMP timeout / retry rate** per device should sit near zero. A device with a persistently high rate is slow to answer (reduce `max_repetitions` or raise its `update_every`); many devices climbing together means the hub is overloaded — see [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).
- **No unexpected "device down" flaps.** A device that toggles down while it pings fine is usually the poller falling behind, not the device.
- If ICMP is enabled, **SNMP state and ICMP state should agree**. SNMP down while ICMP is up isolates the problem to the SNMP agent or the walk, not the device being offline.

## 4. Confirm coverage matches intent

- Every device you meant to monitor has a node.
- Every interface you care about is present (some profiles filter by interface type — check the `snmp:interfaces` Function for the full list).
- Heavy devices are on an interval that finishes inside the poll cycle (the poll cycle stays comfortably under `update_every`).

## A quick acceptance check

Before you rely on a new hub:

1. Every device matched a vendor profile (Step 1) or you've consciously accepted generic collection for it.
2. Utilization on your fastest links is sane and matches the device's own counters (Step 2).
3. Timeout/retry rates are near zero and no device is flapping down (Step 3).
4. Coverage is complete (Step 4).

If any check fails, fix it before you build alerts or capacity decisions on the data.

## What's next

- [Troubleshooting](/docs/npm/device-metrics/troubleshooting.md) — fixes for each failure above.
- [Anti-patterns](/docs/npm/device-metrics/anti-patterns.md) — the habits that cause bad data.
- [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md) — keeping the hub inside its limits.
