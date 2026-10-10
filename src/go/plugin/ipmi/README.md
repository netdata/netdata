# Experimental Go IPMI plugin

This plugin is an opt-in replacement candidate for `freeipmi.plugin`. It is excluded from production builds. The initial
executable supports Linux amd64 and arm64. FreeIPMI remains available for production use and capabilities not yet
supported by the experiment.

## Build and configure

From the repository root:

```sh
sudo ./netdata-installer.sh --enable-plugin-ipmi
```

`--disable-plugin-ipmi` disables it explicitly. CMake users can set `-DENABLE_PLUGIN_IPMI=ON`. A standalone test binary
needs no C compiler:

```sh
cd src/go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/ipmi.plugin ./cmd/ipmiplugin
```

No jobs start until configured. In the Netdata configuration directory, add `ipmi/ipmi.conf`:

```yaml
jobs:
  - name: ipmi
    driver: open
    device: 0
```

Before activating this job, disable `freeipmi` in the `[plugins]` section of `netdata.conf`. The canonical `ipmi` job
retains chart IDs used by the C plugin, so run only one for a given monitored host with this job name. The Go collector
uses separate `ipmi_go.*` contexts and the `ipmi-go-sensors` Function.

Installation grants no additional privileges. Local collection needs read/write access to `/dev/ipmi0` (or the selected
device number). For a controlled test on a root-only device, invoke the binary with `sudo` and a trusted configuration
directory. Normal Agent execution requires device access arranged by the operator. Do not grant broader privileges to
`go.d.plugin`.

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

## Compatibility

- Numeric contexts use `ipmi_go.*`; chart IDs, units and nominal/warning/critical/unknown states retain the C shape. The
  state interpretation uses FreeIPMI defaults, including nominal non-critical threshold assertions; custom
  interpretation files are not loaded.
- Unavailable numeric data produces gaps. `ipmi-go-sensors` reads an immutable snapshot and reports null readings. Its
  summary charts show sensor counts by Component and State.
- Canonical local job `ipmi` uses the `ipmi` chart type. Other job names receive the standard framework job prefix.
  Sensor keys remain stable when a reading becomes unavailable; the C library can change its reading-type key then.
- SEL is the raw repository entry count, not FreeIPMI's interpreted/filtered count. The existing SEL alarm matches the
  canonical local chart `ipmi.events`; the `ipmi_go_sensor_state` alert matches `ipmi_go.sensor_state` for every Go job.
- Bridged sensors, device-only SDR repositories, legacy direct KCS/SSIF/SUNBMC/INTELDCMI drivers, FreeBSD local access,
  custom interpretation files and OEM health policy remain covered by FreeIPMI. Kernel-backed local KCS/SSIF is accessed
  through OpenIPMI.

The hardware baseline covers one local Linux BMC with an earlier SDK revision. Automated loopback BMC tests exercise
LAN/LAN+ authentication, collection, cancellation and session cleanup with the current pin. Physical remote BMC
compatibility and full FreeIPMI parity remain unverified.
