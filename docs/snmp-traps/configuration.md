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

## On this page

- [Listener endpoints](#listener-endpoints)
- [SNMP versions and communities](#security-versions-and-communities)
- [SNMPv3 users](#snmpv3-users)
- [Source controls](#source-controls)
- [Storm controls](#storm-controls)
- [Output backends](#output-backends)
- [Direct journal retention](#direct-journal-retention)
- [Profile metrics](#profile-metrics)

## Where the file lives

| Path | Purpose |
|---|---|
| `/etc/netdata/go.d/snmp_traps.conf` | Your SNMP trap listener jobs. Edits here survive package upgrades. |
| `/usr/lib/netdata/conf.d/go.d/snmp_traps.conf` | The stock commented template shipped with Netdata. Reference only. |
| `/etc/netdata/go.d/snmp.trap-profiles/` | Optional user trap profiles and profile metric rules. |
| `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/` | Stock trap profiles shipped with Netdata. Reference only. |

Depending on installation type, paths may be prefixed with `/opt/netdata`.

## How to apply configuration

This page documents the options in YAML form because that is the most compact way to show them, and because the same keys appear in both configuration methods. You can apply them two ways.

**Recommended — Dynamic Configuration (no restart).** Add, edit, test, and deploy a listener job from the Netdata UI under **Integrations → SNMP Trap Listener → Configure**, with no file editing and no `netdata` restart. This is the primary method; see [enable a job via Dynamic Configuration](/docs/snmp-traps/installation.md#enable-via-dynamic-configuration) for the step-by-step UI flow and the paid-Cloud-connection requirement.

**Fallback — edit the file and restart.** Use this for headless, automated, or free-tier deployments. Edit the job file with `edit-config` from the Netdata configuration directory:

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config go.d/snmp_traps.conf
```

After file edits, restart Netdata so the listener job is recreated:

```bash
sudo systemctl restart netdata
```

## Three things to know before you edit

1. **Collection is explicit.** The stock file is a commented template. Create or uncomment a `jobs:` entry before expecting traps.
2. **The default listener is broad.** It binds `0.0.0.0:162`, accepts SNMPv1 and SNMPv2c, accepts all communities when `communities` is empty, and accepts all source IPs unless you set `allowlist.source_cidrs`. On exposed networks, restrict traffic with firewalls or ACLs too — the allowlist is evaluated only after the packet reaches the host.
3. **At least one output backend must stay enabled.** Journal is on by default, OTLP is off. If you disable journal, enable OTLP or the job fails validation.

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
| Source controls | `allowlist.source_cidrs` | `["0.0.0.0/0", "::/0"]` | Source-IP CIDR allowlist, checked before the packet is parsed. |
| Source controls | `source.trusted_relays` | `[]` | Trap relays allowed to supply original source identity through `snmpTrapAddress.0`. |
| Enrichment | `reverse_dns.enabled` | `false` | Adds best-effort PTR annotation as `TRAP_REVERSE_DNS`. Never used as authoritative identity. |
| Storm controls | `rate_limit` | disabled | Optional per-source token-bucket rate limiting. |
| Storm controls | `dedup` | disabled | Optional suppression of repeated identical traps inside a window. |
| Outputs | `journal.enabled` | `true` | Writes decoded traps to local journal-compatible files and exposes listener jobs through the `snmp:traps` Function. Linux only. |
| Outputs | `otlp` | disabled | Optional OTLP/gRPC Logs export. |
| Storage | `retention` | `max_size: 10GB` | Per-job direct journal retention and rotation. Ignored when `journal.enabled` is `false`. |
| Meaning | `overrides` | `[]` | Per-OID category, severity, and label overrides on top of profile defaults. |
| Metrics | `profile_metrics` | disabled | Enables selected trap-to-metric rules defined by loaded trap profiles. |

## Listener endpoints

`listen.endpoints` controls where the job binds UDP sockets. List more than one entry when the same job should bind multiple local addresses.

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
| `receive_buffer` | `4194304` | Requested UDP receive buffer in bytes. Maximum: `268435456`. Set `0` to keep the OS default. |
| `endpoints[].protocol` | `udp` | Only `udp` is supported. |
| `endpoints[].address` | `0.0.0.0` | Local address to bind. Use a specific interface address to avoid listening on every interface. |
| `endpoints[].port` | `162` | UDP port. Port `162` requires `CAP_NET_BIND_SERVICE` or root; Netdata packages grant this to `go.d.plugin`. For a non-privileged test listener use a port above `1024`, such as `9162`. |

## SNMP versions and communities {#security-versions-and-communities}

The default accepts SNMPv1 and SNMPv2c. For production, enable only the versions your devices send.

**Security: SNMPv1/v2c community strings travel in cleartext.** They authenticate but provide no confidentiality, so anyone on the path can read the community and the trap payload. On any segment you do not fully trust, prefer SNMPv3 with `authPriv` (authentication and privacy). When you use v3, prefer SHA-256 authentication and AES privacy; treat MD5, SHA-1, and DES as deprecated. See [SNMPv3 users](#snmpv3-users).


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

For a complete static v3 job, see [Static SNMPv3 with journal and OTLP](#static-snmpv3-with-journal-and-otlp).

| Key | Required | Values |
|---|---:|---|
| `username` | yes | SNMPv3 USM security name. |
| `engine_id` | yes for static v3 | Sender authoritative engine ID as hex, 5-32 bytes. May be omitted when dynamic engine ID discovery is enabled. |
| `auth_proto` | no | `none`, `md5`, `sha`, `sha224`, `sha256`, `sha384`, `sha512`. Empty behaves as `none`. |
| `auth_key` | when auth is enabled | Authentication passphrase. Minimum 8 characters after secret resolution. |
| `priv_proto` | no | `none`, `des`, `aes`, `aes192`, `aes256`, `aes192c`, `aes256c`. Empty behaves as `none`. Privacy requires authentication; `priv_proto` cannot be enabled while `auth_proto` is `none`. |
| `priv_key` | when privacy is enabled | Privacy passphrase. Minimum 8 characters after secret resolution. |

For new deployments, prefer `auth_proto: sha256` and `priv_proto: aes`, and enable privacy (`authPriv`) so trap payloads are encrypted in transit. `md5`, `sha` (SHA-1), and `des` are accepted for legacy senders but are deprecated; migrate away from them where the device firmware allows.

Use [Secrets Management](/src/collectors/SECRETS.md) for `auth_key` and `priv_key`. The example uses file resolvers, but environment, command, and secretstore resolvers are also supported there.

### SNMPv3 INFORM receiver identity

SNMPv3 INFORM authentication uses a receiver-local engine ID.

```yaml
local_engine_id: 80001f888001020304050607
```

If `local_engine_id` is empty or omitted, Netdata generates and persists a stable per-job value under the Netdata lib directory. Set it explicitly only when your operational process requires a known receiver engine ID.

### Dynamic engine ID discovery

Use dynamic discovery when sender engine IDs are not known in advance. Start from the static v3 job and swap the engine-ID keys:

```yaml
    engine_id_whitelist: []
    dynamic_engine_id_discovery: true
    dynamic_engine_id_max_pairs: 4096
```

Key behavior:

- `engine_id_whitelist` must be empty when `dynamic_engine_id_discovery` is `true`, and `usm_users[].engine_id` may then be omitted (set it only to pre-seed known senders).
- New `(engineID, username)` pairs are tracked in memory only and not persisted across restarts. The cap is `dynamic_engine_id_max_pairs`; `0` uses the default `4096`.
- Operator-observable symptom: pairs over the cap are rejected and counted in the `unknown_engine_id` error counter, and the first registration of a new pair also increments that counter once as an audit signal. Discovery is skipped for INFORM traffic.

For the most controlled deployment, use static engine IDs. Use dynamic discovery when inventory data is incomplete and you can restrict senders with `allowlist.source_cidrs`.

## Source controls

Source controls decide which packets are allowed before parsing, and which identity is trusted afterward.

### Restrict source CIDRs

`allowlist.source_cidrs` is checked against the UDP peer before the packet is parsed, keeping unexpected senders away from the decoder and authentication paths. Omitted or empty accepts all source IPs (the default open IPv4/IPv6 allowlist); a CIDR list accepts only peers inside it. For production, avoid broad ranges unless firewalls or ACLs already control the path.

```yaml
allowlist:
  source_cidrs:
    - 10.0.0.0/8
    - 192.0.2.0/24
```

### Trust relays carefully

Set `source.trusted_relays` only when a known relay forwards traps and preserves the original device address in `snmpTrapAddress.0`. Keep the list narrow and never use catch-all ranges such as `0.0.0.0/0`.

```yaml
source:
  trusted_relays:
    - 10.0.30.5/32
```

For why the UDP peer is authoritative by default and how a trusted relay overrides source identity, see the [source-identity model in Enrichment](/docs/snmp-traps/enrichment.md#direct-senders-vs-trusted-relays).

### Reverse DNS is annotation only

Reverse DNS is disabled by default. When enabled, Netdata emits `TRAP_REVERSE_DNS`, which is never authoritative identity (see [Enrichment](/docs/snmp-traps/enrichment.md#reverse-dns)).

```yaml
reverse_dns:
  enabled: false
```

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

Rate limiting is tracked per source IP, up to a fixed cap of 10,000 active sources per job (not user-configurable). A source that has been idle is allowed an initial burst of up to `per_source_pps` traps before limiting takes effect.

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

Dedup-suppressed traps do not update profile-defined metrics. Instead of one journal entry per repeat, the listener records a periodic summary entry with the suppressed count.

## Output backends

The listener can write to the direct journal, export to OTLP/gRPC, or do both.

| Backend | Enabled by default | Local querying | Notes |
|---|---:|---:|---|
| Direct journal | yes | yes | Stores journal-compatible files under the Netdata log directory and exposes the job through the `snmp:traps` Function. Linux only. |
| OTLP/gRPC Logs | no | no | Exports traps as OTLP LogRecords to an external collector. |

When both backends are enabled, traps go to both outputs and the local journal is authoritative: an OTLP export failure does not affect what was already written to the journal. When journal is disabled and OTLP is enabled, no local journal files are created and the job does not appear as a local SNMP trap log source.

Trap rows can contain sensitive operational payloads, not only credentials. Review what Netdata emits and redacts before sharing exports, forwarding, or granting broad journal access; see [Field Reference sensitive-data cautions](/docs/snmp-traps/field-reference.md#sensitive-data-cautions).

### Direct journal

Direct journal storage is enabled by default and requires Linux:

```yaml
journal:
  enabled: true
```

Trap entries are written as journal-compatible files under the configured Netdata log directory, with a per-job root of `/var/log/netdata/traps/<job>/` (or `${NETDATA_LOG_DIR}/traps/<job>/` at runtime). They back the embedded `snmp:traps` Function, use `ND_LOG_SOURCE=snmp-trap`, and are not the host systemd-journald journal. For how the `journalctl --directory` path is built, see [Journal and Querying](/docs/snmp-traps/journal-and-querying.md); for the field list, see [Field Reference](/docs/snmp-traps/field-reference.md).

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

To send traps only to OTLP, explicitly disable journal output on an otherwise normal job:

```yaml
    journal:
      enabled: false
    otlp:
      enabled: true
      endpoint: "https://otel-collector.example.net:4317"
```

OTLP-only jobs do not create local journal files and do not appear as local SNMP trap log sources in the `snmp:traps` Function. Use this mode only when the external OTLP receiver is the intended system of record.

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

For what each category and severity value means, see [Metrics](/docs/snmp-traps/metrics.md#categories-and-severities); for where they appear as fields, see [Field Reference](/docs/snmp-traps/field-reference.md#trap-meaning-fields).

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

Only committed traps update profile metrics. For the exact update rule and the diagnostics chart, see [Metrics](/docs/snmp-traps/metrics.md#profile-defined-metrics).

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

For a relayed setup, set both `allowlist.source_cidrs` and `source.trusted_relays` to the relay's `/32` only (the YAML is the same as the [Trust relays carefully](#trust-relays-carefully) block). The relay is then the only allowed UDP peer and the only peer that may override source identity. Never add device subnets to `trusted_relays`.

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
