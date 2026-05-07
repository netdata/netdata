<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/enrichment/static-metadata.md"
sidebar_label: "Static Metadata"
learn_status: "Published"
learn_rel_path: "Network Flows/Enrichment"
keywords: ['static metadata', 'enrichment', 'networks', 'exporters', 'interface']
endmeta-->

# Static metadata

Static metadata is the foundational enrichment for any multi-exporter deployment. It lets you give your routers, your switches, your interfaces, and your own networks the names and labels you want to see on the dashboard — instead of raw IP addresses and SNMP indexes.

There are two independent configuration blocks. They populate different fields and use different lookup keys, but you typically configure both:

| Block | Lookup key | What it labels |
|---|---|---|
| `enrichment.metadata_static.exporters` | exporter IP / CIDR + ifIndex | The exporter device and its individual interfaces |
| `enrichment.networks` | source / destination IP | Your own networks (CIDRs you operate) |

## What it populates

### From `metadata_static.exporters`

Per-exporter (matched by source IP of the UDP datagram):

- `EXPORTER_NAME`, `EXPORTER_GROUP`, `EXPORTER_ROLE`, `EXPORTER_SITE`, `EXPORTER_REGION`, `EXPORTER_TENANT`

Per-interface (matched by ifIndex from the flow record):

- `IN_IF_NAME` / `OUT_IF_NAME`
- `IN_IF_DESCRIPTION` / `OUT_IF_DESCRIPTION`
- `IN_IF_SPEED` / `OUT_IF_SPEED` (in **bits per second**)
- `IN_IF_PROVIDER` / `OUT_IF_PROVIDER`
- `IN_IF_CONNECTIVITY` / `OUT_IF_CONNECTIVITY`
- `IN_IF_BOUNDARY` / `OUT_IF_BOUNDARY` (`1` = external, `2` = internal)

### From `networks`

Per source/destination IP (matched by CIDR):

- `SRC_NET_NAME` / `DST_NET_NAME`
- `SRC_NET_ROLE` / `DST_NET_ROLE`
- `SRC_NET_SITE` / `DST_NET_SITE`
- `SRC_NET_REGION` / `DST_NET_REGION`
- `SRC_NET_TENANT` / `DST_NET_TENANT`
- `SRC_COUNTRY` / `DST_COUNTRY`, `SRC_GEO_STATE` / `DST_GEO_STATE`, `SRC_GEO_CITY` / `DST_GEO_CITY` — overrides for the GeoIP-derived fields
- `SRC_GEO_LATITUDE` / `DST_GEO_LATITUDE`, `SRC_GEO_LONGITUDE` / `DST_GEO_LONGITUDE` — overrides for the coordinate fields
- `SRC_AS_NAME` / `DST_AS_NAME` — only when the configured `asn` causes the chain to render `AS{n}` and the MMDB has a matching name

The `networks` block can also override the AS **number** via the `asn` field, for matching prefixes. AS **names** still come from the ASN database — see [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md).

## Configuration

### Naming exporters and interfaces

```yaml
enrichment:
  metadata_static:
    exporters:
      192.0.2.10:                          # single IP (treated as /32)
        name: edge-router-1
        site: par1
        region: eu-west
        role: edge
        tenant: tenant-a
        default:                           # template applied to interfaces not in if_indexes
          description: unclassified port
        if_indexes:
          1:
            name: Gi0/0/1
            description: uplink to ISP-A
            speed: 10000000000             # 10 Gbps in bits per second
            provider: isp-a
            connectivity: transit
            boundary: external
          2:
            name: Gi0/0/2
            description: LAN core
            speed: 1000000000
            connectivity: lan
            boundary: internal
```

The `if_indexes` map keys by the integer ifIndex the router sends in flow records. If a flow arrives with an ifIndex not present in the map, the `default` interface block is used. The `skip_missing_interfaces: true` option overrides this — when set, missing entries get no interface labels at all.

### Matching multiple exporters with one block

CIDR prefixes work too. Longest-prefix match wins.

```yaml
enrichment:
  metadata_static:
    exporters:
      198.51.100.0/24:                     # all routers in this subnet
        site: dc-fra1
        region: eu-central
        role: spine
        default:
          connectivity: lan
          boundary: internal
      198.51.100.10:                       # specific override for one IP
        name: spine-fra1-a
        if_indexes:
          1:
            name: 100Ge-0/0/1
            description: leaf-uplink
            speed: 100000000000
            connectivity: transit
            boundary: external
```

### Tagging your own networks

```yaml
enrichment:
  networks:
    10.0.0.0/8:
      name: corp-internal
      role: internal
      tenant: tenant-a
    172.16.0.0/12:
      name: corp-internal
      role: internal
      tenant: tenant-a
    192.168.0.0/16:
      name: corp-internal
      role: internal
      tenant: tenant-a
    198.51.100.0/24:                       # a public block you operate
      name: customer-acme
      role: customer
      site: par1
      country: FR
      city: Paris
      latitude: 48.8566
      longitude: 2.3522
      asn: 64500                           # forces SRC_AS / DST_AS for traffic in this prefix
    203.0.113.0/24: transit-a              # shorthand: name only
```

Two things to know:

- The `networks` map merges all containing CIDRs in ascending prefix-length order — least-specific first, with more-specific overrides. A `/24` entry inherits any non-empty fields from a containing `/16` entry, and adds or overwrites its own fields.
- The shorthand form (`203.0.113.0/24: transit-a`) sets only the `name`. All other fields are empty.

## Lookup priority and pipeline order

Within an exporter:

1. **`metadata_static.exporters` longest-prefix match** wins for exporter labels and interface labels.
2. **`if_indexes` lookup** runs against the ifIndex from the flow record, falling back to `default` (or returning empty when `skip_missing_interfaces: true`).

Within a flow's source/destination IP:

1. **GeoIP** runs first as the base layer.
2. **`network_sources` (remote feeds)** merge on top.
3. **`networks` (static config)** merges last and wins on any non-empty field.

The two paths run independently. An exporter IP that also matches a `networks` entry will get **both** treatments — exporter labels for the device, network labels for any traffic to or from that IP.

## Things to know

### `IN_IF_BOUNDARY` / `OUT_IF_BOUNDARY` semantics

These label **the interface itself**, not the direction of traffic:

- `1` = external — the port faces the outside world (Internet, peer, transit)
- `2` = internal — the port faces your own infrastructure
- `0` (or omitted) = undefined — the field is removed from the output

Filtering for `IN_IF_BOUNDARY=1` cleanly gives you "traffic that arrived from outside". The encoding is intentional even if `1` for "external" looks counter-intuitive.

The values `external` and `internal` are also accepted as strings (case-insensitive) in the YAML.

### `speed` is in bits per second

A 1 Gbps interface is `1000000000`, not `1000`. Operators thinking in megabits or gigabits will get the speed wrong by a factor of 1000 to 1 000 000. The plugin treats `speed: 0` as "not set" and removes the field from the output.

### CIDR prefixes accept single IPs

`192.0.2.10` and `192.0.2.10/32` are equivalent. Use whichever is clearer.

### `networks.<cidr>.asn` overrides only the number

Setting `asn: 64500` overrides whatever the [ASN resolution chain](/docs/network-flows/enrichment/asn-resolution.md) computed. The AS *name* still comes from the ASN database — there is no `asn_name` config field.

### Coordinates are silently dropped if invalid

`latitude: 91.5` (out of range) sets the field to an empty string with no error. Same for non-finite values. Validate manually if your data is important.

### Renamed interfaces don't auto-track

`if_indexes` keys by the numeric ifIndex. If a router renumbers its interfaces (line-card reseat, stack rebuild), the old ifIndex no longer matches and the per-interface block silently no longer applies. Audit after hardware changes.

### Static metadata blocks classifiers

If `metadata_static.exporters` set **any** classification field (group / role / site / region / tenant) for an exporter, the [classifiers](/docs/network-flows/enrichment/classifiers.md) do not run for that exporter at all. The same applies to interfaces: if static metadata set any of provider / connectivity / boundary, the interface classifiers don't run for that interface. Plan accordingly.

## Sampling rate overrides

Sampling rates can also be configured per exporter prefix here, in case your exporter doesn't carry the rate or you want to override it:

```yaml
enrichment:
  default_sampling_rate: 1                  # global fallback
  override_sampling_rate:
    10.1.0.0/16: 1024                       # override for this network of exporters
```

`default_sampling_rate` applies when the flow record doesn't carry a rate and no override matches. `override_sampling_rate` always wins when its prefix matches the exporter IP. Both accept either an integer (uniform rate) or a CIDR-keyed map.

## What can go wrong

- **Wrong CIDR matches.** Overlapping ranges merge ascending — a more-specific entry that leaves a field blank will inherit the supernet's value. To clear a field on a more-specific entry, you must set it explicitly to a sentinel value, not leave it blank.
- **Forgotten internal range.** Until you declare your RFC 1918 / RFC 6598 / link-local ranges as `networks` entries, GeoIP can return spurious data for them.
- **Renamed interface no longer matches.** ifIndex keys are numeric; renames or hardware changes break the mapping silently.
- **Stale exporter prefix.** A new device with a different management IP doesn't match an old block. Audit when you replace gear.
- **`speed: 1000` means 1 kbps.** Use bits per second.
- **`boundary: 0` is indistinguishable from "not set"**, both result in field removal. If you want explicit "undefined" use the string `"undefined"`.
- **Lat / lng silent drop.** Invalid values become empty strings. The map quietly stops drawing the marker.

## What's next

- [GeoIP](/docs/network-flows/enrichment/ip-intelligence.md) — How country / city / coordinates and AS names get resolved.
- [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md) — The provider chain that picks AS numbers.
- [Classifiers](/docs/network-flows/enrichment/classifiers.md) — Rule-based labelling that runs only when static metadata didn't already classify the exporter or interface.
- [Network sources](/docs/network-flows/enrichment/network-identity.md) — Fetching `networks`-style data from remote endpoints.
