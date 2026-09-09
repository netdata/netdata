<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/licensing/metrics.md"
sidebar_label: "Metrics and Functions"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Licensing Monitoring"
keywords: ['license', 'licensing', 'metrics', 'charts', 'snmp:licenses', 'function', 'alerts']
endmeta-->

<!-- markdownlint-disable-file -->

# Metrics and the Licenses Function

Netdata reads license telemetry from devices that expose it over SNMP and normalizes it into charts and an interactive table.

## Charts

- **`snmp.license.remaining_time`** — the earliest expiry across all of a device's licenses.
- **`snmp.license.authorization_remaining_time`**, **`snmp.license.certificate_remaining_time`**, **`snmp.license.grace_remaining_time`** — the specific authorization, certificate, and grace-period timers.
- **`snmp.license.usage_percent`** — the highest pool pressure: how much of a licensed capacity is consumed.
- **`snmp.license.state`** — counts of licenses by state (healthy, informational, degraded, broken, ignored).

## The `snmp:licenses` function

Open **`snmp:licenses`** on a device for a per-license table — served from already-collected data, with no extra requests to the device. Each row shows the **license**, its raw **state** and normalized **state bucket**, the **component** and **type**, the **remaining time** and **expiry timestamp**, **usage** against **capacity** and the **usage %**, and a plain-language **impact** note. Every column sorts and filters, so you can sort by remaining time to see what expires first, or filter to the licenses in a degraded or broken state.

## Alerts

Netdata ships seven stock licensing alerts that raise on their own — route them to your channels as you would any Netdata alert:

- **Expiring** — a license, authorization, or certificate timer is within **30 days** (warning) or **7 days** (critical) of expiry; a **grace period** warns at **7 days** and goes critical on expiry. Four timers in all, giving you lead time to renew.
- **State degraded** and **state broken** — a license has moved into a degraded or broken state, independent of any timer.
- **Usage high** — a licensed pool is filling up (warning at 80%, critical at 95%), so you can add capacity before it runs out.

When one fires, open the `snmp:licenses` function on the device, find the affected license by its state or remaining time, and renew or expand it with your vendor before it lapses.

## Supported devices

Licensing telemetry comes up automatically for the vendors that ship dedicated licensing telemetry over SNMP — Check Point, Fortinet, Cisco (including Smart Licensing), Sophos, Blue Coat ProxySG, and MikroTik.

## What's next

- [Overview](/docs/npm/licensing/README.md) — why licensing monitoring matters.
