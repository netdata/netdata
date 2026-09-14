<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/licensing/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Licensing Monitoring"
keywords: ['license', 'licensing', 'entitlement', 'expiry', 'snmp', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Licensing Monitoring

Netdata tracks the license state of your network devices — what's entitled, how much is used, and when it expires — so a license doesn't quietly lapse and disable a feature you depend on.

## What Netdata tracks

For devices that expose licensing over SNMP:

- **Time to expiry** — the earliest license, authorization, certificate, and grace-period timers.
- **Usage** — how much of a licensed pool is consumed.
- **State** — healthy, informational, degraded, broken, or ignored, per license.

These appear as live charts and in the **`snmp:licenses`** function — a per-device table of every license with its state and remaining time.

## Which vendors

Licensing telemetry comes up automatically with the device's profile, for the vendors that ship dedicated licensing telemetry over SNMP — Check Point, Fortinet, Cisco (including Smart Licensing), Sophos, Blue Coat ProxySG, and MikroTik.

## Alerts

Stock alerts on licensing health — an approaching expiry, a license moving into a degraded or broken state, and a licensed pool filling up.

## Where to start

- Licensing comes up automatically when you [monitor a device](/docs/npm/device-metrics/README.md) that reports it — open its charts or the `snmp:licenses` function.
