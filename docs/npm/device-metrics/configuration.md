<!-- markdownlint-disable-file -->

# Configuration

SNMP devices are configured in `go.d/snmp.conf`. Each device is one job. This page covers the options you actually set — credentials, security, multiple devices, and the polling knobs. For the complete, generated option reference and more examples, see the [SNMP collector page](/src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md); for profiles, see [SNMP Profile Format](/src/go/plugin/go.d/collector/snmp/profile-format.md).

Edit the file with:

```bash
cd /etc/netdata        # or your Netdata config directory
sudo ./edit-config go.d/snmp.conf
```

## The smallest working job

For an SNMPv2c device, a hostname and a community string are enough — Netdata auto-detects the model and applies the matching profiles:

```yaml
jobs:
  - name: core-switch-1
    hostname: 10.0.0.1
    community: public
```

`name` is how the device appears in Netdata, so use something recognizable. Repeat the block per device.

## SNMPv3

SNMPv3 replaces the community string with a user and security level. Set the level to match how the device is provisioned:

```yaml
jobs:
  - name: edge-firewall-1
    hostname: 10.0.2.1
    options:
      version: 3
    user:
      name: netdata-monitor
      level: authPriv          # noAuthNoPriv | authNoPriv | authPriv
      auth_proto: sha256       # md5 | sha | sha224 | sha256 | sha384 | sha512
      auth_key: "[AUTH_PASSPHRASE]"
      priv_proto: aes256       # des | aes | aes192 | aes256 | aes192c | aes256c
      priv_key: "[PRIV_PASSPHRASE]"
      # context_name: ""       # only for multi-context agents
```

Never commit real passphrases; treat `auth_key`/`priv_key` as secrets. If a device uses `authNoPriv`, omit the `priv_*` fields; for `noAuthNoPriv`, omit all of them.

## Multiple devices

Each device is its own job, and you can set different options per device — for example, a longer interval for heavy core routers than for access switches:

```yaml
jobs:
  - name: access-sw-1
    hostname: 10.0.10.11
    community: public
    # default update_every: 10s is fine for simple switches

  - name: core-router-1
    hostname: 10.0.0.254
    community: public
    update_every: 30            # poll heavy devices less often
    options:
      max_repetitions: 10       # smaller bulk walks for a device that struggles
```

## Discover devices automatically

Instead of listing devices one by one, point Netdata at your network ranges and let it find them. Edit `go.d/sd/snmp.conf`, set `disabled: no`, and give it the credentials to try and the subnets to scan:

```yaml
disabled: no
discoverer:
  snmp:
    credentials:
      - name: ro-v2c
        version: "2"
        community: public
    networks:
      - subnet: "10.0.10.0/24"
        credential: ro-v2c       # which credential above to try on this range
```

Netdata scans each subnet, identifies the SNMP devices, and **creates a polling job for each one automatically** — recognizing the model and starting collection with no further configuration. It rescans on an interval to pick up new devices. Each subnet is scanned up to /23 (512 IPs); add more `subnet` entries for larger networks. This is the fastest way to onboard a whole network: set the credentials and ranges once, and every device on them comes up monitored.

The defaults suit most networks. For large or sensitive ones, tune the scan under `discoverer.snmp`:

| Option | Default | What it does |
|---|---|---|
| `rescan_interval` | `30m` | How often to rescan the networks for new devices. A negative value scans once and stops. |
| `device_cache_ttl` | `12h` | How long a discovered device is trusted before it is re-probed. |
| `parallel_scans_per_network` | `32` | How many IPs are probed at once within each subnet. Lower it to be gentler on the network. |
| `timeout` | `1s` | How long to wait for a probe reply before moving on. |

## The options that matter

| Option | Default | What it does |
|---|---|---|
| `hostname` | (required) | Device IP or DNS name. |
| `community` | `public` | SNMPv1/v2c community string. |
| `options.version` | `2c` | SNMP version: `1`, `2c`, or `3`. |
| `options.port` | `161` | SNMP UDP port. |
| `options.timeout` | `5` | Seconds to wait for a response. |
| `options.retries` | `1` | Retries before a request is counted failed. |
| `options.max_repetitions` | `25` | Rows per `GETBULK`. Lower it for devices that choke on large responses. |
| `options.max_request_size` | `20` | OIDs per request. |
| `update_every` | `10` | Polling interval (seconds). Raise it for heavy or slow devices. |
| `manual_profiles` | (none) | Profiles to apply to a device that reports no `sysObjectID` (rare). Devices that report one are matched automatically — normally leave this empty. |
| `create_vnode` | `true` | Expose the device as its own virtual node in Netdata. |
| `vnode_device_down_threshold` | `3` | Consecutive failed polls before the device is marked down. |

### ICMP alongside SNMP

By default Netdata also pings each device, giving you reachability and latency next to the SNMP metrics. SNMP and ping run concurrently — ping never delays polling.

```yaml
jobs:
  - name: core-router-1
    hostname: 10.0.0.254
    community: public
    ping:
      enabled: true            # default
      privileged: true         # default; uses raw ICMP
      packets: 3
      interval: 100ms
    # ping_only: true          # skip SNMP, collect ICMP only (e.g. a non-SNMP device)
```

## Profiles: auto-detect first, force only when needed

You normally configure nothing for profiles — the collector reads `sysObjectID`/`sysDescr` and applies every stock profile whose selector matches the device. To add coverage for a model or collect extra OIDs, drop a custom profile with a matching `selector` into `/etc/netdata/go.d/snmp.profiles/`; it loads alongside the stock profiles and applies automatically to every device it matches. A file there with the same name as a stock profile overrides that stock profile — use this to tighten a selector or change what a model collects. The format and the `extends` composition model are documented in [SNMP Profile Format](/src/go/plugin/go.d/collector/snmp/profile-format.md). `manual_profiles` is only for the rare device that reports no `sysObjectID`: with nothing to auto-match on, you name the profiles to apply — otherwise leave it empty.

## What's next

- [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md) — how `update_every` and `max_repetitions` set a hub's capacity, and when to add hubs.
- [Validation](/docs/npm/device-metrics/validation.md) — confirm the device matched a profile and is polling cleanly.
- [Troubleshooting](/docs/npm/device-metrics/troubleshooting.md) — timeouts, missing metrics, and SNMPv3 auth failures.
