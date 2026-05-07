<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/enrichment/decapsulation.md"
sidebar_label: "Decapsulation"
learn_status: "Published"
learn_rel_path: "Network Flows/Enrichment"
keywords: ['decapsulation', 'srv6', 'vxlan', 'tunnel', 'overlay']
endmeta-->

# Decapsulation

Decapsulation extracts the inner packet from tunnelled traffic so the dashboard reflects the actual endpoints, not the tunnel endpoints. Two modes are supported: **SRv6** and **VXLAN**.

This is useful when your routers are observing overlay traffic — VXLAN-encapsulated VM traffic between hypervisors, SRv6-encapsulated data-centre fabric, etc. Without decap, you see the same "10.0.0.x → 10.0.0.y" flow for every VM-to-VM conversation, which tells you nothing.

## Modes

```yaml
protocols:
  decapsulation_mode: vxlan      # one of: none, srv6, vxlan
```

| Mode | What it strips | What it surfaces |
|---|---|---|
| `none` (default) | nothing | the outer-header view |
| `srv6` | IPv6 outer + extension headers + Routing Header type 4 (SRH) | the inner IPv4 (next-header 4) or IPv6 (next-header 41) packet |
| `vxlan` | outer Ethernet/IP + UDP (port 4789) + 8-byte VXLAN header | the inner Ethernet frame, then the inner L3/L4 |

GRE, IP-in-IP, GENEVE, and other tunnel types are **not** supported. Only SRv6 and VXLAN.

## How the modes interact with each protocol

Decap relies on the exporter shipping inner-packet bytes. That happens in three different ways depending on the source protocol:

| Source | Inner-packet bytes carried as | Required exporter capability |
|---|---|---|
| **NetFlow v9** | Information Element 104 (`Layer2packetSectionData`, RFC 7270) | Exporter must include IE 104 in the template; it carries the captured frame bytes |
| **IPFIX** | Information Element 315 (`dataLinkFrameSection`, RFC 7133) | Same idea, IPFIX-standard IE |
| **sFlow** | Always — `SampledHeader` records carry the truncated raw packet | sFlow agents send `SampledHeader` by default for header-sampling mode |

For **NetFlow v9 / IPFIX without IE 104 / 315 in the template, decapsulation does not run** — even with `decapsulation_mode: vxlan` set. Standard flow records pass through unchanged. So enabling decap on the plugin is half the work; you also have to configure your exporter to ship the frame bytes.

For **sFlow, decap always runs** when the mode is set, because every flow sample carries a `SampledHeader`.

## What gets surfaced

When decap succeeds, the inner 5-tuple replaces the outer one in the flow record:

- `SRC_ADDR` / `DST_ADDR` — inner source/destination IPs
- `SRC_PORT` / `DST_PORT` — inner ports
- `PROTOCOL` — inner L4 protocol
- `ETYPE` — inner EtherType
- `IPTOS`, `IPTTL`, `IPV6_FLOW_LABEL`, `TCP_FLAGS` — inner IP/TCP fields
- `IP_FRAGMENT_ID`, `IP_FRAGMENT_OFFSET` — inner fragmentation
- `ICMPV4_TYPE` / `ICMPV4_CODE` / `ICMPV6_TYPE` / `ICMPV6_CODE` — inner ICMP
- `MPLS_LABELS` — inner MPLS label stack (if present)
- `BYTES` — inner L3 length (so byte counts represent inner traffic, not outer overhead)

For VXLAN, the inner Ethernet frame is parsed, so `SRC_MAC` / `DST_MAC` / `SRC_VLAN` / `DST_VLAN` come from the inner frame. **The outer MACs and VLANs are lost.**

For SRv6, the outer is an IPv6 packet (no L2 to lose).

The **VXLAN VNI is dropped**. Netdata does not surface it. If you need to distinguish overlay segments, you need a different mechanism — VLAN-tagged inner frames work, but pure VNI-based segmentation isn't visible.

## Decapsulation is destructive on non-tunnel traffic

When `decapsulation_mode` is set and the exporter ships records via the special L2-section path (NetFlow v9 IE 104 / IPFIX IE 315 / sFlow `SampledHeader`), but the inner packet doesn't match the configured tunnel:

- For VXLAN mode: a non-VXLAN packet (different UDP port, malformed VXLAN header, or not UDP at all) is **dropped**. The flow does NOT fall back to outer-header view.
- For SRv6 mode: an IPv6 packet without the right extension-header chain leading to next-header 4 or 41 is **dropped**.
- For sFlow with decap on, only `SampledHeader` records are processed. `SampledIPv4`, `SampledIPv6`, `SampledEthernet`, `ExtendedSwitch`, `ExtendedRouter`, `ExtendedGateway` records are all skipped.

Plain NetFlow / IPFIX flow records that don't go through the special L2-section path are **unaffected** — they pass through normally regardless of the decap setting. So enabling `decapsulation_mode: vxlan` doesn't break your normal flow stream; it only filters the L2-section path.

This means decapsulation is safe to enable when:

- All your tunnel-bearing exporters use the same encapsulation, AND
- The L2-section / `SampledHeader` data they ship is exclusively (or near-exclusively) tunnel traffic.

If you mix VXLAN and SRv6 traffic on the same exporter, you cannot decap both — the plugin has one global setting.

## Configuring exporters to ship inner-packet bytes

For decap to work, your exporter must include the inner-packet bytes in its export. This is platform-specific. The CLI snippets below are starting points — verify against the vendor's reference manual before deploying.

### Cisco IOS-XE / IOS-XR (NetFlow v9 with `datalink mac`)

```
flow record FNF-WITH-MAC
 match ipv4 source address
 match ipv4 destination address
 match transport source-port
 match transport destination-port
 match ipv4 protocol
 match datalink mac source address input
 match datalink mac destination address input
 collect counter bytes
 collect counter packets
 collect timestamp absolute first
 collect timestamp absolute last
 collect datalink frame-section section header size 128
```

The `collect datalink frame-section` directive is what causes the exporter to include IE 104. Adjust the section size based on your maximum tunnel header size; 128 bytes covers VXLAN over Ethernet over IPv4. SRv6 inner extraction needs more — 256 or higher.

### Juniper JunOS (IPFIX with frame export)

JunOS' IPFIX support varies by platform. On platforms that support frame-section export, configure the template to include `dataLinkFrameSection` (IE 315). Refer to your platform's documentation.

### sFlow (built-in)

sFlow agents send `SampledHeader` by default. No special configuration needed beyond enabling sFlow.

## Failure modes

- **Exporter doesn't ship IE 104 / IE 315.** The plugin can't decap. Records pass through with outer-header view.
- **Inner packet isn't VXLAN/SRv6.** With decap on, the flow is dropped. There is no "fall back to outer view" — this is intentional, but be aware.
- **Truncated frame section.** The inner Ethernet/IP/L4 parsing fails and the flow is dropped.
- **VXLAN on a non-standard port.** The plugin only matches UDP destination port 4789 (RFC 7348). VXLAN-GPE on 4790 and vendor-custom ports are not detected.
- **VNI not visible.** Bytes 4-6 of the VXLAN header are skipped. If you need VNI-based segmentation, see if your exporter can place the VNI in a separate field; otherwise this isn't surfaceable today.

## What's next

- [Configuration](/docs/network-flows/configuration.md) — `protocols.decapsulation_mode` setting reference.
- [Sources / NetFlow](/src/crates/netflow-plugin/integrations/netflow.md) — IE 104 export configuration.
- [Sources / IPFIX](/src/crates/netflow-plugin/integrations/ipfix.md) — IE 315 export configuration.
- [Sources / sFlow](/src/crates/netflow-plugin/integrations/sflow.md) — `SampledHeader` semantics.
