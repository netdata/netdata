<!-- markdownlint-disable-file -->

# Troubleshooting

The problems below are the ones that actually come up when polling network devices. Each lists the symptom, the likely cause, and the fix.

## The device doesn't appear, or has almost no metrics

**Symptom.** You added the job but the device shows no node, or only a bare interface count with no vendor-specific metrics.

**Likely causes and fixes.**

- **SNMP isn't reachable.** Confirm the device answers from the hub: `snmpget -v2c -c <community> <device> .1.3.6.1.2.1.1.3.0` (sysUpTime). No answer means a firewall, ACL, wrong community/credentials, or wrong port — not a Netdata problem. SNMP is UDP/161 by default.
- **No profile matched the model.** The collector matches on `sysObjectID`/`sysDescr`; an unknown model gets only what generic profiles cover. Check the device's `sysObjectID` and `sysDescr`, then add a custom profile with a `selector` matching them under `/etc/netdata/go.d/snmp.profiles/` — it applies automatically once the selector matches. If the device reports no `sysObjectID` at all (rare), name the profiles to apply with `manual_profiles`. See [SNMP Profile Format](/src/go/plugin/go.d/collector/snmp/profile-format.md).
- **Wrong profile auto-matched.** If a stock selector is too broad, an inappropriate profile's metrics can appear. Override that stock profile by placing a file of the same name under `/etc/netdata/go.d/snmp.profiles/` with a tighter `selector`, or report the over-broad selector so it can be fixed upstream.

## Timeouts and gaps in the charts

**Symptom.** Metrics arrive intermittently; the device's SNMP timeout/retry rate is climbing.

**Likely causes and fixes.**

- **The device's management CPU is saturated by the walk.** Large tables (full MAC/FDB, big interface lists, full BGP tables) are expensive for a device's control plane. Reduce `max_repetitions` so each bulk walk asks for fewer rows, and/or raise `update_every` for that device.
- **The poll cycle can't finish inside the interval.** If one hub polls too many objects for its `update_every`, every device suffers. Raise `update_every`, stagger devices, or split them across more hubs — see [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).
- **A large walk exceeds the path MTU.** Over a WAN or VPN, big SNMP responses can exceed the path MTU and be dropped when Don't-Fragment is set — it looks exactly like a timeout but is an MTU problem. Poll locally on the device LAN (the hub model), or lower `max_repetitions` so responses stay small.

## SNMPv3 authentication failures

**Symptom.** The device answers nothing under SNMPv3, or the very first poll is slow.

**Likely causes and fixes.**

- **Mismatched security parameters.** The `level`, `auth_proto`, `priv_proto`, and the two keys must match the device's user exactly. `authPriv` needs both auth and priv set; `authNoPriv` needs auth only; `noAuthNoPriv` needs neither. A single mismatch yields silence.
- **First SNMPv3 exchange is slow.** Engine-ID discovery and key localization add latency to the first request to a device. A slow first poll that then settles is normal — don't tune for it.

## A device shows traffic spikes to impossible rates

**Symptom.** An interface briefly shows terabit-per-second utilization with no real event.

**Cause.** A 32-bit octet counter wrapped (it rolls over in roughly 3.4 seconds at 10G line rate) and naive differencing turned the rollover into a fake spike. Netdata reads the 64-bit high-capacity counters (`ifHCInOctets`) where the device exposes them — these don't wrap at line rate, so the artifact never appears. A persistent fake spike usually means the device is only exposing 32-bit counters for that interface, or the matched profile isn't collecting the HC variant. Confirm the device supports HC counters and that the profile collects them.

## A device is marked down but it's actually up

**Symptom.** Netdata flags the device down; it pings fine and serves traffic.

**Likely causes and fixes.**

- **The poller fell behind.** If the hub is overloaded, polls queue and time out, and devices can appear down when the problem is the hub, not the network. Check whether many devices went "down" together (hub-side) or just one (device-side), and relieve the hub per [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).
- **A transient miss.** A device is marked down only after `vnode_device_down_threshold` (3) consecutive failures, but a genuinely flaky path can still trip it. Compare against the ICMP probe: SNMP down while ICMP is up points at the SNMP agent or the walk, not the device being offline.

## First poll after a device reboot is slow or empty

**Symptom.** Right after a device reload, SNMP is slow or returns little.

**Cause.** The SNMP agent may take 30–60 seconds to be ready, and the MIB cache is cold. This is expected; it clears within a cycle or two. Counter resets at reboot are handled by Netdata's counter logic — a counter that jumps backward is treated as a reset, not a negative or impossibly large rate, so you won't see a phantom spike.

## Missing a specific vendor metric

**Symptom.** Interfaces and system metrics are there, but a vendor-specific value you expect (a sensor, a license field, a vendor counter) is not.

**Cause and fix.** The matched profile doesn't declare that OID for your model. Extend the relevant base profile or add a custom one under `/etc/netdata/go.d/snmp.profiles/` that collects the OID, following [SNMP Profile Format](/src/go/plugin/go.d/collector/snmp/profile-format.md).

## What's next

- [Configuration](/docs/npm/device-metrics/configuration.md) — the options referenced above.
- [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md) — fixing hub-side overload for good.
- [Anti-patterns](/docs/npm/device-metrics/anti-patterns.md) — the mistakes that cause these problems in the first place.
