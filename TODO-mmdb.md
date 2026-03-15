# TODO: Enable GeoIP/ASN enrichment in netflow plugin

## TL;DR

The netflow plugin already supports GeoIP country/city/state and ASN number enrichment, but it is disabled by default because MMDB paths are not auto-detected. The remaining work is to enable ASN/GEO MMDBs by default when they exist, add ASN name extraction and canonical fields, and make country plus ASN name part of the default function response.

## Purpose

Fit the netflow plugin for production-default IP intelligence enrichment:
- use local MMDB databases automatically when present
- populate ASN number and ASN name consistently
- populate Geo country/city/state consistently
- expose country plus ASN number and ASN name by default in function responses for both source and destination
- verify the behavior on the running Netdata agent after `./install.sh`, not only in crate tests

## Decisions

### User decisions made
- 2026-03-15: The ASN DB must include ASN name and the plugin must extract it.
- 2026-03-15: The ASN DB must be used by default when it exists.
  - Fill ASN number from MMDB whenever MMDB provides one.
  - Always fill ASN name from the ASN DB when available.
- 2026-03-15: The GEO DB must be used by default when it exists.
- 2026-03-15: The function response must return GEO, ASN number, and ASN name for both `src` and `dst` by default.
  - Refined on 2026-03-15: only `COUNTRY` is wanted from GEO in the default function response.
- 2026-03-15: Task completion requires all relevant tests to pass, installation with `./install.sh`, and live verification against the running agent API/function output.

### Pending user decisions
- None currently open.
- 2026-03-15 resolved decisions:
  - only `COUNTRY` is wanted from GEO in the default function response
  - MMDB ASN is authoritative when available, so `AS` and `AS_NAME` remain aligned

## Current State

### What exists and works
- **MMDB databases**: `/var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb` (10M) and `topology-ip-country.mmdb` (5.7M)
- **Go downloader tool**: `src/go/tools/topology-ip-intel-downloader/` — downloads from iptoasn.com, generates MMDB files
- **MMDB contains**: `autonomous_system_number`, `autonomous_system_organization` (ASN name), country ISO codes, city, state
- **Rust enrichment code**: `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs` — full GeoIP resolver with 30s auto-reload, loads MaxMind MMDB format
- **Output fields implemented**: `SRC_AS`, `DST_AS`, `SRC_COUNTRY`, `DST_COUNTRY`, `SRC_GEO_CITY`, `DST_GEO_CITY`, `SRC_GEO_STATE`, `DST_GEO_STATE`
- **Tier rollup**: All GEO fields survive tier aggregation (defined in `tiering.rs`)
- **ASN precedence behavior**: current code already backfills ASN from network/MMDB data only when the current ASN is `0`
- **Config reality**: a minimal `netflow.yaml` containing only `enrichment.geoip` still fails today with `missing field 'listener'`

### What doesn't work
- **GeoIP is NOT enabled by default** — `GeoIpConfig` defaults to empty database lists → `GeoIpResolver::from_config()` returns `None`
- **No auto-detection of MMDB files** — the plugin doesn't probe well-known paths
- **ASN names not extracted** — `AsnLookupRecord` struct in `enrichment.rs` only reads `autonomous_system_number`, ignores `autonomous_system_organization`
- **No `SRC_AS_NAME`/`DST_AS_NAME` canonical fields** — need to be added to decoder, tiering, and query
- **Country fields not in default aggregation** — `GROUP_BY_DEFAULT_AGGREGATED` in `query.rs` lacks `SRC_COUNTRY`/`DST_COUNTRY`
- **ASN name fields not in default response** — no canonical fields, no tier persistence, no default grouping
- **Critical live bug found during verification** — enabling MMDB-only enrichment activated `FlowEnricher`, but `enrich_record()` still hard-dropped flows when static metadata or sampling data was missing. This made auto-detected MMDBs incompatible with the stock config and prevented live ingestion after restart.

## Environment Challenge

The netflow plugin runs with an **empty environment** — `NETDATA_CACHE_DIR` and other env vars are not available (verified via `/proc/<pid>/environ`). The `NetdataEnv::from_environment()` returns all `None`. Despite this, the plugin works because it resolves paths through config files and CLI arguments.

The auto-detection of MMDB files cannot rely on `NETDATA_CACHE_DIR`. It must either:
1. Derive the cache directory from the resolved `journal_dir` (which is typically `{cache}/flows` — parent = cache dir)
2. Use a hardcoded default fallback (`/var/cache/netdata`)
3. Pass the cache directory through the plugins.d protocol (if supported)

The `running_under_netdata()` check returns `false` because no env vars are set, so `PluginConfig::new()` calls `Self::parse()` (CLI args) instead of `Self::load_from_netdata_config()`.

## Plan

### 1. Auto-detect GeoIP databases (plugin_config.rs)

After config loading in `PluginConfig::new()`, if `enrichment.geoip` has empty database lists:
- Derive cache dir from resolved `journal.journal_dir` parent, or fallback to `/var/cache/netdata`
- Probe `{cache}/topology-ip-intel/topology-ip-asn.mmdb` and `topology-ip-country.mmdb`
- If found, populate `cfg.enrichment.geoip` with paths and set `optional: true`

**Key risk**: The environment is empty. Need to figure out the cache directory without `NETDATA_CACHE_DIR`. The `journal_dir` after `resolve_relative_path()` may still be relative ("flows") when `cache_dir` is None.

**Suggested approach**: Use the journal base directory. Check `cfg.journal.base_dir()` — it likely resolves to the absolute path. If not, fallback to `/var/cache/netdata`.

### 2. Add ASN name fields

Files to modify:
- **`decoder.rs`**: Add `SRC_AS_NAME`/`DST_AS_NAME` to canonical fields list (~line 89), `FlowRecord` struct (~line 278), `to_fields()`, `from_fields()`, `to_compact_fields()`, field match arms (~line 4117)
- **`enrichment.rs`**: Add `asn_name: String` to `NetworkAttributes` struct (~line 2804), add `autonomous_system_organization` to `AsnLookupRecord` (~line 2910), extract it in `lookup()` (~line 3037), write it in `write_network_attributes_record_src/dst` (~line 3493)
- **`tiering.rs`**: Add `src_as_name`/`dst_as_name` to `RollupDimensions` struct (~line 115), `from FlowRecord` construction (~line 189), `to_fields()` serialization (~line 314)
- **`network_sources.rs`**: Add `asn_name` to `NetworkAttributes` construction (~line 247)
- **`query.rs`**: Add `SRC_AS_NAME`/`DST_AS_NAME` to `GROUP_BY_DEFAULT_AGGREGATED`

Behavior required:
- ASN number: use MMDB ASN whenever MMDB provides one
- ASN name: fill from MMDB when available

**Critical risk discovered and resolved during live verification**:
- Root cause was not journal serialization. It was the enrichment gate.
- `FlowEnricher` became active as soon as GeoIP MMDBs were auto-detected.
- The existing enrichment logic still enforced old parity rules:
  - drop flows when both interfaces were zero
  - drop flows when exporter metadata was missing
  - drop flows when sampling rate stayed zero after overrides/defaults
- With the stock config, that meant decoded IPFIX packets were counted but live flows were not written.
- Fix applied:
  - enrichment is now additive for MMDB-only/default operation
  - exporter name falls back to the exporter IP/name already present on the record
  - missing sampling overrides no longer drop the flow
  - MMDB-only or GeoIP-only activation no longer blocks ingestion

### 3. Add country fields to default aggregation (query.rs)

Add `SRC_COUNTRY` and `DST_COUNTRY` to `GROUP_BY_DEFAULT_AGGREGATED` (~line 32). This is safe to do independently — the fields already exist in the canonical field list, they're just not included in the default group-by.

User decision:
- only `COUNTRY` is wanted by default from GEO
- `CITY` and `STATE` remain available in the record format, but are not part of the default function response

### 4. Config file approach (alternative)

Creating `/etc/netdata/netflow.yaml` with ONLY the geoip section crashed the plugin — current parsing fails with `missing field 'listener'`. A partial config doesn't work. Options:
- Copy the stock config and add geoip section (fragile — stock config may change)
- Make auto-detection work properly (preferred)
- Make the config merging more flexible (overlay user config on top of stock defaults)

## Files Reference

| File | Purpose |
|------|---------|
| `src/crates/netdata-netflow/netflow-plugin/src/plugin_config.rs` | Config loading, GeoIpConfig struct (line 661), PluginConfig::new() (line 926) |
| `src/crates/netdata-netflow/netflow-plugin/src/enrichment.rs` | GeoIpResolver (line 2954), NetworkAttributes (line 2795), AsnLookupRecord (line 2908), write_network_attributes_record_src/dst (line 3493) |
| `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs` | Canonical fields (line 63), FlowRecord struct (line 243), serialization |
| `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs` | RollupDimensions struct (line 109), rollup hash, tier serialization |
| `src/crates/netdata-netflow/netflow-plugin/src/query.rs` | GROUP_BY_DEFAULT_AGGREGATED (line 32), RAW_ONLY_FIELDS, FACET_EXCLUDED_FIELDS |
| `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs` | NetworkAttributes construction (line 247) |
| `src/crates/netdata-netflow/netflow-plugin/configs/netflow.yaml` | Stock config (no geoip section) |
| `src/go/tools/topology-ip-intel-downloader/` | Go tool that generates MMDB files |
| `src/crates/netdata-plugin/rt/src/netdata_env.rs` | NetdataEnv struct — reads env vars (all None for netflow plugin) |

## Testing

- Bearer token auth required: `X-Netdata-Auth: Bearer {token}` from `/var/lib/netdata/bearer_tokens/`
- Tokens regenerate on restart — check for new token after each rebuild
- Query: `curl -s -H "X-Netdata-Auth: Bearer ${TOKEN}" 'http://localhost:19999/api/v1/function?function=flows:netflow&timeout=60'`
- Costa's router (MikroTik at 10.20.4.1) exports IPFIX flows to costa-desktop
- Most traffic is LAN-to-LAN (private IPs) — no GeoIP data for private ranges. Need public IP traffic to verify enrichment.
- Required before declaring completion:
  - pass the relevant Rust test suite(s)
  - install the updated agent with `./install.sh`
  - verify the running agent exposes `SRC_COUNTRY`, `DST_COUNTRY`, `SRC_AS`, `DST_AS`, `SRC_AS_NAME`, and `DST_AS_NAME`
  - verify the live path on real flow data, not only synthetic unit tests

## Verification Notes

- 2026-03-15: `cargo test -p netflow-plugin -- --nocapture` passed with `188 passed; 0 failed; 1 ignored`
- 2026-03-15: `./install.sh` completed successfully twice after code changes
  - installer still reports a non-fatal warning: `git fetch -t` failed due local GitHub SSH auth on this workstation
- 2026-03-15 live verification against the running agent:
  - `GET /api/v1/functions` shows `flows:netflow` registered
  - after the enrichment-gate fix, new raw journals were created again under `/var/cache/netdata/flows/raw/`
  - a new 1-minute tier journal was created under `/var/cache/netdata/flows/1m/`
  - default `GET /api/v1/function?function=flows:netflow` returned live aggregated rows
  - verified `key` includes:
    - `SRC_AS`
    - `SRC_AS_NAME`
    - `SRC_COUNTRY`
    - `DST_AS`
    - `DST_AS_NAME`
    - `DST_COUNTRY`
  - verified with live examples such as:
    - `SRC_AS=13335`, `SRC_AS_NAME=CLOUDFLARENET`, `SRC_COUNTRY=US`
    - `DST_AS=6799`, `DST_AS_NAME=OTENET-GR Athens - Greece`, `DST_COUNTRY=GR`
    - `DST_AS=399358`, `DST_AS_NAME=ANTHROPIC`, `DST_COUNTRY=US`
