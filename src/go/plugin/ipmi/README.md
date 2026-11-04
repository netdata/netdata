# IPMI plugin

This Go plugin collects local Linux OpenIPMI sensors and remote BMCs over LAN/LAN+. Linux source and static builds
include it by default. For DEB/RPM installations, install the optional `netdata-plugin-ipmi` package separately.
`freeipmi.plugin` remains available for rollback and capabilities listed under Compatibility below.

## Selection and rollback

Netdata starts at most one of `ipmi.plugin` and `freeipmi.plugin`. It prefers Go when both are installed and enabled.
The choice is made at Agent startup across all configured plugin directories and stays fixed until restart, including
when the chosen plugin exits or fails collection. There is no automatic failover after a collection failure.

Existing configuration intent is preserved when `[plugins] ipmi` is unset:

- `freeipmi = no` keeps IPMI collection disabled.
- Non-blank `[plugin:freeipmi] command options` keep the C implementation when FreeIPMI is enabled, preserving remote
  targets and legacy options.

To migrate either case, configure the Go jobs, set `[plugins] ipmi = yes`, and restart Netdata. The Go plugin does not
translate C command options. To switch back, install the FreeIPMI plugin if necessary and use:

```ini
[plugins]
    ipmi = no
    freeipmi = yes
```

Restart Netdata after changing the selection or installing an IPMI plugin. `enable running new plugins = no` disables
automatic selection unless one plugin is explicitly enabled. In the Netdata container environment, IPMI requires explicit
enablement and access to the device for local collection.

The Function is now named `ipmi-sensors`; callers using the experimental `ipmi-go-sensors` name must update.

## Build and configure

Linux source builds enable the plugin by default. `--disable-plugin-ipmi` disables it; CMake users can set
`-DENABLE_PLUGIN_IPMI=OFF`. A standalone binary needs no C compiler:

```sh
cd src/go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/ipmi.plugin ./cmd/ipmiplugin
```

Set `GOARCH` for the target system; use `GOARCH=arm GOARM=6` or `GOARM=7` for ARM32. Cross-compilation verifies a
binary can be built; physical device access still needs kernel OpenIPMI support and suitable hardware.

The stock `ipmi/ipmi.conf` attempts local collection using this job:

```yaml
jobs:
  - name: local
    driver: open
    device: 0
```

Hosts without an accessible OpenIPMI device do not collect local IPMI metrics. Replace this job to collect remote BMCs,
or set `jobs: []` to disable the local job. Existing jobs named `ipmi` keep their previous chart IDs; renaming a job creates
new chart IDs.

The installers give the dedicated `ipmi.plugin` binary root ownership and setuid permission, restricted to the Netdata
group, so it can open the local device. A manually built binary needs read/write access to `/dev/ipmi0` (or the selected
device); use a trusted configuration and explicit `sudo` invocation for local debugging. Containers must also expose
the device, for example with `--device /dev/ipmi0`.

Remote collection uses an ordinary UDP connection to the BMC and needs no local device permissions. Prefer LAN+
(IPMI 2.0); use `driver: lan` for BMCs that require IPMI 1.5. For example:

```yaml
jobs:
  - name: server-bmc
    driver: lanplus
    hostname: bmc.example
    username: monitor
    password: '${env:IPMI_PASSWORD}'
```

The default UDP port is 623 and the requested session privilege is `user`, which permits standard sensor and SEL reads.
Set `privilege_level: operator` or `administrator` only if required by the BMC policy. IPMI usernames and LAN passwords
are limited to 16 bytes. The BMC must have IPMI over LAN enabled and permit the configured account to connect.

Metrics belong to the Agent host by default. To attach a remote job to a different host, define a named virtual node in
`vnodes.conf` and add `vnode: server-node` to the job. Give each BMC a distinct job name.

The command and session setup timeout defaults to five seconds. Cancellation interrupts local response waits and remote
UDP exchanges. Remote session cleanup gets a separate two-second budget, including after a canceled collection.

## Sensor thresholds

The `ipmi-sensors` Function shows each threshold sensor's limits next to its reading, in the reading's units: LNR, LCR
and LNC (lower non-recoverable, critical and non-critical) and UNC, UCR and UNR (upper). The values come from Get Sensor
Thresholds, the thresholds the BMC uses, not from the SDR: a BMC can keep earlier values in its SDR after its thresholds
change. They are read with the SDR inventory, after a repository change, about every five minutes and after a
reconnect, so a changed threshold appears within about five minutes. A blank cell is a threshold the sensor does not have, or one the
BMC did not return or that cannot be converted. Sensors whose SDR declares fixed thresholds show the SDR values, which
the IPMI specification defines as their values. Non-linear sensors and sensors without a supported unit show none. A
failed threshold read never fails a collection or changes a sensor state: states come from the threshold comparison the
BMC reports with each reading. By default only critical and non-recoverable crossings make a state critical.

## System Event Log

With `collect_sel: true` (the default), each job charts the SEL entry count and new records, and serves the
`ipmi-sel` Function: every record the BMC retains, newest first, with sensor, event, direction and the default FreeIPMI
state. The dashboard time range does not filter it, because SEL records are often weeks apart. The Function reads
records the collector already holds and sends no BMC requests. `collect_sel: false` stops every SEL command, so neither
SEL chart exists, and removes the job from `ipmi-sel`.

The collector keeps the records it read and, when the log changes, reads the records the BMC added plus a few boundary
records that show where the log starts and ends; it rereads the log after the BMC clears or rewrites it. The first read
of a large log is spread over several collections, so the Function shows the oldest records until it completes. Records
are kept in memory, so an Agent restart reads the log again. The plugin never clears or writes the SEL.

`ipmi.sel_new_records` counts records newly observed since the previous value, by default FreeIPMI state, in two
chart instances: `event_type: threshold` for threshold events, which also change their sensor's state, and
`event_type: discrete` for every other record. Records already in the log when collection starts are history, so the
chart appears once the first read completes; records logged while the Agent was stopped are not counted. After a clear
or rewrite, rereading the same record does not count it again. After that initial baseline, observed events are
reported during rescans even when rollover prevents the whole log from being read. A failed SEL read leaves a gap;
records observed during it count in the next successful value.

The `ipmi_sel_critical_events` alert raises CRITICAL when records newly observed in the last 10 minutes include a
critical discrete record, for example a power supply failure, an uncorrectable memory error or a processor IERR. A SEL record
reports an event, not whether its cause is resolved, so the alert clears 10 minutes after the last such record without
a clear notification; the `ipmi-sel` Function shows the records. Threshold
events are left to `ipmi_sensor_state`, which alerts while a sensor reads critical at collection time; a critical
crossing that ends between collections shows only in the `threshold` chart and in `ipmi-sel`. Warning records never
alert: FreeIPMI marks routine boot records as warning. A critical discrete record from a sensor that the BMC also
reports, such as a power supply status sensor, can raise both alerts: this one at once, and `ipmi_sensor_state` if the
sensor still reads critical after that alert's delay.

Times are converted to UTC with the BMC's SEL UTC offset. Records logged before a daylight saving change can be off by
an hour, because the BMC reports only its current offset. When the BMC reports no offset, times are the BMC clock shown
as UTC, and the Function shows how far the BMC clock is from the Agent clock.

## Compatibility

- Numeric contexts use `ipmi.*`; chart IDs, units and nominal/warning/critical/unknown states retain the C shape. The
  state interpretation uses FreeIPMI defaults, including nominal non-critical threshold assertions; custom
  interpretation files are not loaded.
- Unavailable numeric data produces gaps. `ipmi-sensors` reads an immutable snapshot and reports null readings. Its
  summary charts show sensor counts by Component and State. Its threshold columns are Go only.
- The stock `local` job uses the `ipmi_local` chart type. Job names receive the standard framework job prefix;
  an existing job named `ipmi` keeps the `ipmi` chart type.
  Sensor keys remain stable when a reading becomes unavailable; the C library can change its reading-type key then.
- `ipmi.sel` is the raw repository entry count, not FreeIPMI's interpreted/filtered count. The `ipmi_events` alert
  matches `ipmi.sel`; the `ipmi_sensor_state` alert matches `ipmi.sensor_state`. Both apply to every Go job and to C.
  `ipmi.sel_new_records` and `ipmi_sel_critical_events` are Go only.
- `ipmi-sel` states use FreeIPMI's default SEL interpretation. OEM records, records with a reserved record type, OEM
  event types and unlisted events are unknown. The C plugin has no SEL Function.
- Bridged sensors, device-only SDR repositories, legacy direct KCS/SSIF/SUNBMC/INTELDCMI drivers, FreeBSD local access,
  custom interpretation files and OEM health policy remain covered by FreeIPMI. Kernel-backed local KCS/SSIF is accessed
  through OpenIPMI.

The hardware baseline covers one local Linux BMC with an earlier SDK revision. Automated loopback BMC tests exercise
LAN/LAN+ authentication, collection, cancellation and session cleanup with the current pin. Physical remote BMC
compatibility and full FreeIPMI parity remain unverified.
