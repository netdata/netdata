<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/configuration.md"
sidebar_label: "Configuration"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'configuration', 'snmpv3', 'usm', 'allowlist', 'dedup', 'otlp', 'journal']
endmeta-->

<!-- markdownlint-disable-file -->

# Configuration

The SNMP trap listener reads jobs from `go.d/snmp_traps.conf`. A production job should answer three questions clearly:

- Which local address and port should receive traps?
- Which sources, SNMP versions, communities, and SNMPv3 users are allowed?
- Which output backends should receive the decoded trap events?

Use this page to harden a listener after the first trap has been received. If you have not tested receipt yet, start with [Quick Start](/docs/snmp-traps/quick-start.md).

## Where the file lives

| Path | Purpose |
|---|---|
| `/etc/netdata/go.d/snmp_traps.conf` | Your SNMP trap listener jobs. Edits here survive package upgrades. |
| `/usr/lib/netdata/conf.d/go.d/snmp_traps.conf` | The stock commented template shipped with Netdata. Reference only. |
| `/etc/netdata/go.d/snmp.trap-profiles/` | Optional user trap profiles and profile metric rules. |
| `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/` | Stock trap profiles shipped with Netdata. Reference only. |

Depending on installation type, paths may be prefixed with `/opt/netdata`.

Edit the job file with `edit-config` from the Netdata configuration directory:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config go.d/snmp_traps.conf
```

After file edits, restart Netdata so the listener job is recreated:

```bash
sudo systemctl restart netdata
```

## Three things to know before you edit

1. **SNMP trap collection is explicit.** The stock file is a commented template. Create or uncomment a `jobs:` entry before expecting traps.
2. **The default listener is broad.** The default endpoint listens on `0.0.0.0:162`, accepts SNMPv1 and SNMPv2c, accepts all communities when `communities` is empty, and accepts all source IPs unless you set `allowlist.source_cidrs`. On exposed networks, restrict traffic with network firewalls or ACLs too; Netdata's allowlist is evaluated after the packet reaches the host.
3. **At least one output backend must stay enabled.** Direct journal storage is enabled by default. OTLP export is disabled by default. If you disable journal output, enable OTLP or the job fails validation.

## Job layout

Every listener is a job under `jobs:`:

```yaml
jobs:
  - name: campus-core
    listen:
      endpoints:
        - protocol: udp
          address: 10.0.20.10
          port: 162
    versions:
      - v2c
    communities:
      - "${file:/run/secrets/snmp-trap-community}"
    allowlist:
      source_cidrs:
        - 10.0.0.0/8
        - 192.0.2.0/24
    journal:
      enabled: true
```

Job names must be 64 characters or shorter, start with a letter or digit, and may contain letters, digits, underscores, and hyphens. Keep names stable because the name is used in log source selection, per-job metrics, and per-job storage paths.

## Option map

| Area | Option | Default | What it controls |
|---|---|---|---|
| Base | `update_every` | `1` | Framework collection interval for self-metrics. Trap packet reception is event-driven and does not depend on this interval. |
| Base | `vnode` | unset | Associates the listener job with a Virtual Node. |
| Listener | `listen.receive_buffer` | `4194304` | UDP socket receive buffer requested per bound endpoint, in bytes. Set `0` to keep the operating system default. |
| Listener | `listen.endpoints` | UDP `0.0.0.0:162` | Local UDP endpoints to bind. At least one endpoint is required. |
| SNMP | `versions` | `[v1, v2c]` | Accepted SNMP versions. Allowed values: `v1`, `v2c`, `v3`. At least one is required. |
| SNMPv1/v2c | `communities` | `[]` | Community allowlist. Empty accepts all communities. |
| SNMPv3 | `usm_users` | `[]` | SNMPv3 USM users, auth protocols, and privacy protocols. |
| SNMPv3 | `engine_id_whitelist` | `[]` | Static sender engine IDs accepted for SNMPv3 Trap PDUs. Required for static v3 jobs. |
| SNMPv3 | `local_engine_id` | generated | Receiver-local engine ID for SNMPv3 INFORM authentication. |
| SNMPv3 | `dynamic_engine_id_discovery` | `false` | Opt-in runtime discovery of SNMPv3 sender engine IDs. |
| SNMPv3 | `dynamic_engine_id_max_pairs` | `4096` | Maximum in-memory dynamic `(engineID, username)` pairs per job. |
| Source controls | `allowlist.source_cidrs` | `["0.0.0.0/0", "::/0"]` | Pre-decode source-IP CIDR allowlist. |
| Source controls | `source.trusted_relays` | `[]` | Trap relays allowed to supply original source identity through `snmpTrapAddress.0`. |
| Enrichment | `reverse_dns.enabled` | `false` | Adds best-effort PTR annotation as `TRAP_REVERSE_DNS`. Never used as authoritative identity. |
| Storm controls | `rate_limit` | disabled | Optional per-source token-bucket rate limiting. |
| Storm controls | `dedup` | disabled | Optional suppression of repeated identical traps inside a window. |
| Outputs | `journal.enabled` | `true` | Writes decoded traps to local journal-compatible files and exposes local trap jobs through the `snmp:traps` Function. Linux only. |
| Outputs | `otlp` | disabled | Optional OTLP/gRPC Logs export. |
| Storage | `retention` | `max_size: 10GB` | Per-job direct journal retention and rotation. Ignored when `journal.enabled` is `false`. |
| Meaning | `overrides` | `[]` | Per-OID category, severity, and label overrides on top of profile defaults. |
| Metrics | `profile_metrics` | disabled | Enables selected trap-to-metric rules defined by loaded trap profiles. |

## Listener endpoints

`listen.endpoints` controls where the job binds UDP sockets.

```yaml
listen:
  receive_buffer: 4194304
  endpoints:
    - protocol: udp
      address: 10.0.20.10
      port: 162
```

| Key | Default | Notes |
|---|---|---|
| `receive_buffer` | `4194304` | Requested UDP receive buffer in bytes. Maximum: `268435456`. Set `0` to keep the operating system default. |
| `endpoints[].protocol` | `udp` | Only `udp` is supported. |
| `endpoints[].address` | `0.0.0.0` | Local address to bind. Use a specific interface address when the receiver should not listen on every interface. |
| `endpoints[].port` | `162` | UDP port. Port `162` requires `CAP_NET_BIND_SERVICE` or root. Netdata packages grant this capability to `go.d.plugin`. |

Use multiple endpoints when the same job should listen on more than one local address:

```yaml
listen:
  endpoints:
    - protocol: udp
      address: 10.0.20.10
      port: 162
    - protocol: udp
      address: 192.0.2.10
      port: 162
```

If you only need a non-privileged test listener, use a port above `1024`:

```yaml
listen:
  endpoints:
    - protocol: udp
      address: 127.0.0.1
      port: 1062
```

## SNMP versions and communities

The default accepts SNMPv1 and SNMPv2c. For production, enable only the versions your devices send.

```yaml
versions:
  - v2c
```

For SNMPv1 and SNMPv2c, `communities` is an allowlist:

```yaml
communities:
  - "${file:/run/secrets/snmp-trap-community}"
```

| Setting | Behavior |
|---|---|
| `communities: []` | Accepts all SNMPv1/v2c communities. Useful only for short validation or tightly isolated lab networks. |
| One or more values | Accepts only traps whose community matches one of the configured values. |

Community strings are credentials. Use [Secrets Management](/src/collectors/SECRETS.md) so the real value is not stored inline in `snmp_traps.conf`.

## SNMPv3 users

SNMPv3 jobs use USM users. A static v3 job needs:

- `versions` containing `v3`
- at least one `usm_users` entry
- `engine_id_whitelist` containing the accepted sender engine IDs

```yaml
jobs:
  - name: core-router-v3
    listen:
      endpoints:
        - protocol: udp
          address: 10.0.20.10
          port: 162
    versions:
      - v3
    usm_users:
      - username: trapmon
        engine_id: 80001f8880e5a5c0d6c7b8a9
        auth_proto: sha256
        auth_key: "${file:/run/secrets/snmp-v3-auth-key}"
        priv_proto: aes
        priv_key: "${file:/run/secrets/snmp-v3-priv-key}"
    engine_id_whitelist:
      - 80001f8880e5a5c0d6c7b8a9
```

| Key | Required | Values |
|---|---:|---|
| `username` | yes | SNMPv3 USM security name. |
| `engine_id` | yes for static v3 | Sender authoritative engine ID as hex, 5-32 bytes. May be omitted when dynamic engine ID discovery is enabled. |
| `auth_proto` | no | `none`, `md5`, `sha`, `sha224`, `sha256`, `sha384`, `sha512`. Empty behaves as `none`. |
| `auth_key` | when auth is enabled | Authentication passphrase. Minimum 8 characters after secret resolution. |
| `priv_proto` | no | `none`, `des`, `aes`, `aes192`, `aes256`, `aes192c`, `aes256c`. Empty behaves as `none`. Privacy requires authentication; `priv_proto` cannot be enabled while `auth_proto` is `none`. |
| `priv_key` | when privacy is enabled | Privacy passphrase. Minimum 8 characters after secret resolution. |

Use [Secrets Management](/src/collectors/SECRETS.md) for `auth_key` and `priv_key`. The example uses file resolvers, but environment, command, and secretstore resolvers are also supported there.

### SNMPv3 INFORM receiver identity

SNMPv3 INFORM authentication uses a receiver-local engine ID.

```yaml
local_engine_id: 80001f888001020304050607
```

If `local_engine_id` is empty or omitted, Netdata generates and persists a stable per-job value under the Netdata lib directory. Set it explicitly only when your operational process requires a known receiver engine ID.

### Dynamic engine ID discovery

Use dynamic discovery when sender engine IDs are not known in advance.

```yaml
jobs:
  - name: dynamic-v3
    listen:
      endpoints:
        - protocol: udp
          address: 10.0.20.10
          port: 162
    versions:
      - v3
    usm_users:
      - username: trapmon
        auth_proto: sha256
        auth_key: "${file:/run/secrets/snmp-v3-auth-key}"
        priv_proto: aes
        priv_key: "${file:/run/secrets/snmp-v3-priv-key}"
    engine_id_whitelist: []
    dynamic_engine_id_discovery: true
    dynamic_engine_id_max_pairs: 4096
```

Important behavior:

- `engine_id_whitelist` must be empty when `dynamic_engine_id_discovery` is `true`.
- `usm_users[].engine_id` may be omitted in dynamic mode. You may still set it for known senders to pre-seed those `(engineID, username)` pairs.
- New `(engineID, username)` pairs are registered in memory only and are not persisted across restarts.
- The cap is controlled by `dynamic_engine_id_max_pairs`; `0` uses the default `4096`.
- Discovery applies to SNMPv3 Trap PDUs. It is skipped for reportable SNMPv3 messages, including INFORM traffic.
- A first-time dynamic registration increments the `unknown_engine_id` error counter and logs the accepted pair once. Treat that first increment as an audit signal when the sender is expected.

For the most controlled deployment, use static engine IDs. Use dynamic discovery when inventory data is incomplete and you can restrict senders with `allowlist.source_cidrs`.

## Source controls

Source controls decide which packets are allowed before decode and which identity is trusted after decode.

### Restrict source CIDRs

`allowlist.source_cidrs` is checked before BER decode. Use it to keep unexpected senders away from the decoder and from authentication paths.

```yaml
allowlist:
  source_cidrs:
    - 10.0.0.0/8
    - 192.0.2.0/24
```

| Setting | Behavior |
|---|---|
| Omitted or `source_cidrs: []` | Accepts all source IPs through the default open IPv4 and IPv6 allowlist. |
| CIDR list | Accepts only packets whose UDP peer IP is inside one of the CIDRs. |

For production, avoid broad ranges unless the network path is already tightly controlled by firewalls or ACLs.

### Trust relays carefully

By default, the UDP peer is the authoritative source. This is the safest setting for direct device-to-Netdata delivery.

Use `source.trusted_relays` only when traps are forwarded by a known trap relay that preserves the original device address in `snmpTrapAddress.0`:

```yaml
source:
  trusted_relays:
    - 10.0.30.5/32
```

Only peers in `trusted_relays` may override source identity with `snmpTrapAddress.0`.

Do not configure catch-all trusted relays such as `0.0.0.0/0` unless every sender on that path is trusted to report original source identity correctly. A broad trusted-relay list lets untrusted senders influence source attribution.

### Reverse DNS is annotation only

Reverse DNS is disabled by default:

```yaml
reverse_dns:
  enabled: false
```

When enabled, Netdata can emit `TRAP_REVERSE_DNS`. It does not replace source IP identity and is never used as the authoritative source.

## Storm controls

Rate limiting and deduplication protect the receiver from noisy senders and repeated events. They are disabled by default so first validation shows every accepted trap.

### Rate limiting

```yaml
rate_limit:
  enabled: true
  per_source_pps: 1000
  mode: drop
```

| Key | Default | Notes |
|---|---|---|
| `enabled` | `false` | Enables per-source token-bucket rate limiting. |
| `per_source_pps` | `1000` | Maximum traps per second from one source IP. `0` uses the default. |
| `mode` | `drop` | `drop` discards over-limit traps. `sample` allows them but counts rate-limited events. |

Use `drop` when the receiver must protect downstream storage or forwarding. Use `sample` when you need to observe storm volume while testing limits.

When rate limiting is enabled, Netdata tracks up to 10000 active source buckets per job. This cap is fixed and not user-configurable. Idle buckets expire after 10 minutes, and high source churn evicts the oldest bucket before rejecting a new source. The token bucket starts full, so an idle source can send up to `per_source_pps` traps before limiting starts.

### Deduplication

```yaml
dedup:
  enabled: true
  window_sec: 5
  cache_max_entries: 100000
```

| Key | Default | Notes |
|---|---|---|
| `enabled` | `false` | Enables per-job deduplication. |
| `window_sec` | `5` | Repeated matching traps inside the window are summarized. `0` uses the default. |
| `cache_max_entries` | `100000` | Maximum fingerprints kept in memory. `0` uses the default. |
| `key_varbinds` | `[]` | Optional additional varbind names to include in the fingerprint. |

By default, the dedup fingerprint uses the source device and trap OID. Add `key_varbinds` only when one trap OID carries several operationally distinct events that should not suppress each other.

```yaml
dedup:
  enabled: true
  window_sec: 10
  key_varbinds:
    - ifIndex
```

Dedup-suppressed traps do not update profile-defined metrics. They are summarized instead of being stored one by one.

## Output backends

The listener can write to the local direct journal, export to OTLP/gRPC, or do both.

| Backend | Enabled by default | Local querying | Notes |
|---|---:|---:|---|
| Direct journal | yes | yes | Stores journal-compatible files under the Netdata log directory and exposes the job through the `snmp:traps` Function. Linux only. |
| OTLP/gRPC Logs | no | no | Exports traps as OTLP LogRecords to an external collector. |

When both backends are enabled, traps go to both outputs and journal write success is the authoritative local commit. When journal is disabled and OTLP is enabled, no local journal files are created and the job does not appear as a local log source.

Trap rows can contain sensitive operational payloads, not only credentials. Review `MESSAGE`, `TRAP_VAR_*`, `TRAP_JSON`, and OTLP attributes before sharing exports, forwarding to downstream systems, or granting broad journal access.

### Direct journal

Direct journal storage is enabled by default for explicit jobs and requires Linux:

```yaml
journal:
  enabled: true
```

Trap entries are written as journal-compatible files under the configured Netdata log directory. The per-job root is, by default:

```text
/var/log/netdata/traps/<job>/
```

or at runtime:

```text
${NETDATA_LOG_DIR}/traps/<job>/
```

The journal writer stores files in a machine-id child under that job root. The child directory uses the canonical 32-character lowercase hexadecimal machine ID; `$(tr -d '-' < /etc/machine-id)` normalizes dashed IDs if needed. For `journalctl --directory`, use the effective child directory, such as `/var/log/netdata/traps/<job>/$(tr -d '-' < /etc/machine-id)`. The host also needs the `journalctl` command installed.

These files back the embedded `snmp:traps` logs Function. Individual journal entries use `ND_LOG_SOURCE=snmp-trap`, and useful local filters include `TRAP_JOB=<job>`, `TRAP_OID=<oid>`, and `TRAP_REPORT_TYPE=trap`. For the complete field list, see [Field Reference](/docs/snmp-traps/field-reference.md). These files are not the host systemd-journald service's normal journal.

### OTLP/gRPC export

OTLP export is disabled by default:

```yaml
otlp:
  enabled: true
  endpoint: "https://otel-collector.example.net:4317"
  headers:
    authorization: "${file:/run/secrets/snmp-trap-otlp-authorization}"
  request_timeout: 5s
  flush_interval: 200ms
  batch_size: 512
  queue_capacity: 10000
```

| Key | Default | Notes |
|---|---|---|
| `enabled` | `false` | Enables OTLP/gRPC Logs export. |
| `endpoint` | `http://127.0.0.1:4317` | Bare `host:port` and `http://host:port` use plaintext gRPC. `https://host:port` uses TLS with system trust roots. Paths, query strings, and fragments are not supported. |
| `headers` | `null` | Optional OTLP metadata headers. Header values may use secret references. Header keys with the `grpc-` prefix are reserved and rejected. |
| `request_timeout` | `5s` | Timeout for connection preflight and export calls. A failed preflight prevents a static job from starting and rejects a Dynamic Configuration apply. |
| `flush_interval` | `200ms` | Maximum time to buffer records before an export attempt. |
| `batch_size` | `512` | Maximum OTLP LogRecords per export request. `0` uses the default. |
| `queue_capacity` | `10000` | Maximum OTLP records buffered per job. `0` uses the default. |

Use `https://` for remote collectors when trap contents should be protected in transit. Plaintext `http://127.0.0.1:4317` is intended for loopback collectors. Netdata logs a startup warning for remote plaintext OTLP targets.

### OTLP-only jobs

To send traps only to OTLP, explicitly disable journal output:

```yaml
jobs:
  - name: otlp-only
    listen:
      endpoints:
        - protocol: udp
          address: 10.0.20.10
          port: 162
    versions:
      - v2c
    communities:
      - "${file:/run/secrets/snmp-trap-community}"
    allowlist:
      source_cidrs:
        - 10.0.0.0/8
    journal:
      enabled: false
    otlp:
      enabled: true
      endpoint: "https://otel-collector.example.net:4317"
```

OTLP-only jobs do not create local journal files and do not appear as local log sources in the `snmp:traps` Function. Use this mode only when the external OTLP receiver is the intended system of record.

## Direct journal retention

`retention` applies only when `journal.enabled` is `true`.

```yaml
retention:
  max_size: 10GB
  max_duration: null
  rotation_size: null
  rotation_duration: null
```

| Key | Default | Notes |
|---|---|---|
| `max_size` | `10GB` | Maximum total bytes across journal files for this job. Set `null` to disable size-based eviction. |
| `max_duration` | `null` | Maximum age of the oldest journal file before deletion. Set `null` to disable age-based eviction. |
| `rotation_size` | `null` | Maximum size of a single journal file before rotation. `null` means automatic: `max_size / 20`, clamped between `5MB` and `200MB`. `0` is invalid. |
| `rotation_duration` | `null` | Maximum age of a single journal file before rotation. `null` or `0` disables time-based rotation. |

Example with both size and age budgets:

```yaml
retention:
  max_size: 50GB
  max_duration: 14d
  rotation_size: null
  rotation_duration: 24h
```

Choose retention based on expected trap volume and how long operators need local forensic access. If OTLP is the system of record and local querying is not needed, use an OTLP-only job instead of keeping a large local journal.

## Per-OID overrides

Use `overrides` when one trap OID should use site-specific category, severity, or labels without editing the stock profile.

```yaml
overrides:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1
    category: config_change
    severity: notice
    labels:
      change_window: business_hours
```

| Key | Required | Notes |
|---|---:|---|
| `oid` | yes | Numeric trap OID in dotted-decimal format, such as `1.3.6.1.4.1.9.9.43.2.0.1`. MIB names and short OID names are not accepted. |
| `category` | no | One of `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown`. |
| `severity` | no | One of `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`. |
| `labels` | no | Additional labels for this OID. Label keys must use lowercase letters, digits, and underscores, starting with a letter. |

Use overrides for small local policy changes. For broader trap definitions, use trap profile files under `/etc/netdata/go.d/snmp.trap-profiles/`.

## Profile metrics

Profile metrics are disabled by default. Enable them only after you know which trap-to-metric rules you want. The current stock trap profile pack provides decoding coverage but does not ship metric rules; profile-derived charts require loaded operator profiles that define `metrics:` and `charts:` rules.

```yaml
profile_metrics:
  enabled: true
  mode: exact
  include:
    - cisco.config.changed  # example only: replace with a loaded operator-profile rule
  identity:
    device: source
    unresolved_source: source_label
    source_id_privacy: hash
  limits:
    max_rules: 500
    max_sources: 2000
    max_resources_per_source: 512
    max_instances_per_job: 50000
    overflow: drop_and_count
```

| Key | Default | Notes |
|---|---|---|
| `enabled` | `false` | When `true`, evaluates selected profile metric rules for successfully committed traps. |
| `mode` | `none` | `none`, `auto`, `exact`, or `combined`. `none` disables rule evaluation even if `enabled` is `true`. |
| `include` | `[]` | Rule names to enable with `exact` or `combined`. Setting `include` with `none` or `auto` causes job validation failure when `enabled` is `true`; when `enabled` is `false`, `profile_metrics` is disabled and `include` is ignored. |
| `identity.device` | `source` | `source`, `source_label`, or `listener`. |
| `identity.unresolved_source` | `source_label` | `source_label` or `drop_metric_instance`. |
| `identity.source_id_privacy` | `hash` | `hash` emits a stable local hash. `raw` emits the selected source value and can expose source IPs or labels in metric labels. |
| `limits.max_rules` | `500` | Maximum enabled metric rules evaluated by this job. |
| `limits.max_sources` | `2000` | Maximum non-listener source identities tracked by this job. |
| `limits.max_resources_per_source` | `512` | Default resource cap per source and resource class. |
| `limits.max_instances_per_job` | `50000` | Maximum profile-derived metric instances for this job. |
| `limits.overflow` | `drop_and_count` | Only `drop_and_count` is supported. Over-cap metric instances are skipped, accepted traps are still committed, and diagnostics increment. |

Selection modes:

- `none`: no profile metric rules are evaluated.
- `auto`: enables rules marked `auto_safe: true`.
- `exact`: enables only rule names listed in `include`.
- `combined`: enables `auto_safe: true` rules plus rule names listed in `include`.

If an `include` name does not match a loaded rule, the listener job fails validation with `profile_metrics.include rule "<name>" not found` and does not start. If the rule exists but is disabled in the profile, validation fails with `profile_metrics.include rule "<name>" is disabled by profile`.

Profile metrics are updated only after the authoritative output backend accepts the trap. Dedup-suppressed traps and write-failed traps do not update profile metrics. OTLP export failures do not roll back profile metrics that were already updated, even when OTLP reports a delivery error after the journal write succeeds.

Do not enable `auto` blindly on unreviewed custom rules. Profile metrics create time-series, so rule identity and labels must be bounded. For rule syntax and cardinality rules, see [SNMP Trap Profile Format](/src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md).

## Common configurations

### Hardened SNMPv2c listener

```yaml
jobs:
  - name: campus-v2c
    listen:
      endpoints:
        - protocol: udp
          address: 10.0.20.10
          port: 162
    versions:
      - v2c
    communities:
      - "${file:/run/secrets/snmp-trap-community}"
    allowlist:
      source_cidrs:
        - 10.0.0.0/8
        - 192.0.2.0/24
    source:
      trusted_relays: []
    rate_limit:
      enabled: true
      per_source_pps: 1000
      mode: drop
    dedup:
      enabled: true
      window_sec: 5
    journal:
      enabled: true
```

### Static SNMPv3 with journal and OTLP

```yaml
jobs:
  - name: core-v3
    listen:
      endpoints:
        - protocol: udp
          address: 10.0.20.10
          port: 162
    versions:
      - v3
    usm_users:
      - username: trapmon
        engine_id: 80001f8880e5a5c0d6c7b8a9
        auth_proto: sha256
        auth_key: "${file:/run/secrets/snmp-v3-auth-key}"
        priv_proto: aes
        priv_key: "${file:/run/secrets/snmp-v3-priv-key}"
    engine_id_whitelist:
      - 80001f8880e5a5c0d6c7b8a9
    allowlist:
      source_cidrs:
        - 10.0.0.0/8
    journal:
      enabled: true
    otlp:
      enabled: true
      endpoint: "https://otel-collector.example.net:4317"
      headers:
        authorization: "${file:/run/secrets/snmp-trap-otlp-authorization}"
```

### Trap relay with original source attribution

```yaml
jobs:
  - name: relay-input
    listen:
      endpoints:
        - protocol: udp
          address: 10.0.20.10
          port: 162
    versions:
      - v2c
    communities:
      - "${file:/run/secrets/snmp-trap-community}"
    allowlist:
      source_cidrs:
        - 10.0.30.5/32
    source:
      trusted_relays:
        - 10.0.30.5/32
```

In this pattern, the relay IP is the only allowed UDP peer and the only trusted relay. Do not add device subnets to `trusted_relays`; only the relay itself should be allowed to override source identity.

## Things that go wrong

- **The job starts but accepts too much traffic.** Restrict `listen.endpoints`, set `allowlist.source_cidrs`, avoid `communities: []`, and keep `source.trusted_relays` empty unless a known relay is in use.
- **The job fails validation after disabling journal.** Enable `otlp.enabled: true`. At least one backend must be enabled.
- **The job is OTLP-only and does not appear in local Logs.** That is expected. OTLP-only jobs do not create local journal files, so they do not appear as local sources in the `snmp:traps` Function.
- **SNMPv3 traps fail with unknown engine ID.** In static mode, add the sender engine ID to `engine_id_whitelist`. In dynamic mode, keep the whitelist empty and confirm the dynamic pair cap has not been reached.
- **SNMPv3 authentication fails.** Check username, engine ID, auth protocol, privacy protocol, and resolved secret values. Do not paste real keys into the config while debugging.
- **Rate limiting hides events you expected to see.** Temporarily use `mode: sample` or disable the limiter while validating sender volume.
- **Dedup hides repeated traps.** Disable dedup during validation, or add bounded `key_varbinds` when one OID represents multiple distinct resources.
- **Profile metrics create too many series.** Use `mode: exact`, review the selected rules, keep `source_id_privacy: hash`, and lower the `limits` values if needed.

## What's next

After saving the job and restarting Netdata, validate with [Quick Start](/docs/snmp-traps/quick-start.md). For local storage and export workflows, continue to [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) or [Forwarding to SIEM](/docs/snmp-traps/forwarding-to-siem.md). For profile behavior and rule syntax, see [Trap Profiles](/docs/snmp-traps/trap-profiles.md). If the receiver is not installed or cannot bind the port, check [Installation](/docs/snmp-traps/installation.md).
