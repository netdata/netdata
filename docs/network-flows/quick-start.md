<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/quick-start.md"
sidebar_label: "Quick Start"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['quick start', 'netflow', 'sflow', 'ipfix', 'getting started', 'setup']
endmeta-->

<!-- markdownlint-disable-file -->

# Quick Start

Get flow monitoring running in 15 minutes. The path: install the plugin, configure your first router, open the dashboard, and read it correctly.

## Before you start

- The Netdata Agent is running on the host that will collect flow data.
- The [netflow plugin is installed](/docs/network-flows/installation.md) on that host.
- You can configure flow export on at least one router or switch.
- The router can reach the agent's IP on UDP port 2055.

If the plugin isn't installed yet, follow the [Installation page](/docs/network-flows/installation.md) first.

## Step 1 — Configure your router

Pick the closest match to your platform. The configurations below set sensible defaults: 60-second active timeout (industry best practice), a quick template refresh where the platform supports tuning it (so a collector restart recovers in under a minute), and monitoring on the appropriate direction(s) of an interface. softflowd and Arista sFlow do not expose a template-refresh knob; they ship reasonable internal defaults.

### Cisco IOS / IOS-XE (Flexible NetFlow, v9)

```
flow exporter NETDATA
 destination 10.0.0.10                       ! Netdata agent IP
 source GigabitEthernet0/0/0                 ! source interface
 transport udp 2055
 export-protocol netflow-v9
 template data timeout 60
!
flow record NETDATA-RECORD
 match ipv4 source address
 match ipv4 destination address
 match transport source-port
 match transport destination-port
 match ipv4 protocol
 match interface input
 collect interface output
 collect counter bytes
 collect counter packets
 collect timestamp sys-uptime first
 collect timestamp sys-uptime last
!
flow monitor NETDATA-MONITOR
 record NETDATA-RECORD
 exporter NETDATA
 cache timeout active 60
 cache timeout inactive 15
!
interface GigabitEthernet0/0/1
 ip flow monitor NETDATA-MONITOR input
 ip flow monitor NETDATA-MONITOR output
```

### Juniper JunOS (J-Flow v9)

```
set chassis fpc 0 sampling-instance NETDATA
set forwarding-options sampling instance NETDATA input rate 1000
set forwarding-options sampling instance NETDATA family inet output flow-server 10.0.0.10 port 2055
set forwarding-options sampling instance NETDATA family inet output flow-server 10.0.0.10 version9 template ipv4-template
set services flow-monitoring version9 template ipv4-template flow-active-timeout 60
set services flow-monitoring version9 template ipv4-template flow-inactive-timeout 15
set services flow-monitoring version9 template ipv4-template template-refresh-rate seconds 60
set interfaces ge-0/0/1 unit 0 family inet sampling input
set interfaces ge-0/0/1 unit 0 family inet sampling output
```

Notes:

- The `set chassis fpc <slot> sampling-instance NETDATA` line is mandatory; without it the sampling instance is defined but never bound to a forwarding card and no flows are produced.
- `input rate 1000` sets a 1-in-1000 sampling rate. Adjust to match your traffic; the netflow plugin handles per-flow sampling-rate multiplication automatically.
- Replace `fpc 0`, `ge-0/0/1`, and `1000` with the FPC slot, interface, and sampling rate that match your platform.

### Arista EOS (sFlow)

```
sflow run
sflow source-interface Loopback0
sflow destination 10.0.0.10 2055
sflow polling-interval 30
sflow sample dangerous 2000
!
interface Ethernet1
   sflow enable
```

EOS treats sample rates below 16 384 as "aggressive" — the `dangerous` keyword is required to opt in. For higher-rate interfaces, drop the `dangerous` keyword and use 16 384 or above.

### Linux host (`softflowd`, NetFlow v9)

For Linux servers, hypervisors, or any host that doesn't natively speak NetFlow:

```bash
sudo softflowd -i eth0 -n 10.0.0.10:2055 -v 9 -t maxlife=60 -t expint=15
```

`maxlife` caps a flow's wall-clock lifetime at 60 seconds; `expint` controls how often softflowd scans the flow table for expired entries (it is not a template-refresh knob — softflowd's NetFlow v9 template interval is a compile-time default of 16 packets and is not exposed on the command line).

For more vendors and details, see [Flow Protocols / NetFlow](/src/crates/netflow-plugin/integrations/netflow.md), [IPFIX](/src/crates/netflow-plugin/integrations/ipfix.md), and [sFlow](/src/crates/netflow-plugin/integrations/sflow.md).

## Step 2 — Open the dashboard

In your browser, open the Netdata UI, click the **Live** tab in the top navigation, and select **Network Flows** from the Functions list.

By default you'll see:

- A Sankey diagram on top, with a sortable table beneath
- The default time range — last 15 minutes (Netdata's global picker)
- Top-25 flows by bytes
- Aggregated as **Source AS Name → Protocol → Destination AS Name**

Within 60-90 seconds of the router being configured, flow records should start appearing.

## Step 3 — Read the dashboard correctly

Before drawing any conclusion, read this. It's the single biggest source of confusion when people first look at flow data.

### Traffic looks doubled

When a router is configured to export both ingress and egress flow records on every monitored interface — a common configuration — a packet that enters interface A and leaves interface B produces **two** records: one ingress on A, one egress on B. Vendor best practice is to export ingress-only to avoid this; if you can't change the exporter, the dashboard view has to compensate.

If you look at total bandwidth without filtering, you see roughly **2× the real traffic**. Add a second router on the same path and you see 4×.

**To see real bandwidth on a specific link**, filter to one exporter and one interface:

1. In the filter ribbon: `Exporter Name = <your router>`.
2. Add: `Ingress Interface Name = <the interface>` **or** `Egress Interface Name = <the interface>` — pick one, not both. Each packet then appears in exactly one record on that interface.

That's the actual traffic on that link.

### Bidirectional traffic shows both directions

Every conversation has packets going both ways: requests / uploads in one direction, responses / downloads in the other. These are real, separate packets and produce separate flow records. The Sankey, country map, and time-series all show both directions when you don't filter by direction.

Volumes in the two directions are usually asymmetric — for example, a video download produces large B→A flows and small A→B ACKs. A "Country X to Country Y" entry and a "Country Y to Country X" entry refer to the same conversations but typically have very different byte counts. That's correct per-direction accounting, not duplication.

To see only one direction, filter by `Source AS Name` (your network) for outbound or `Destination AS Name` (your network) for inbound.

## Step 4 — Verify it's working

If the Sankey is empty after 60-90 seconds, work through this:

1. **Datagrams arriving at the host.**

   ```bash
   sudo tcpdump -i any -nn -c 20 'udp port 2055'
   ```

   If you see packets, the network path is fine. If not, check the router's exporter status, the firewall, and the source IP the router uses.

2. **Listener bound on the host.**

   ```bash
   sudo ss -unlp | grep 2055
   ```

   Should show `netflow-plugin` listening. If not, see [Troubleshooting](/docs/network-flows/troubleshooting.md).

3. **Plugin actually decoding.**

   Open the standard Netdata charts page and find `netflow.input_packets`. If `udp_received` is rising but `parsed_packets` isn't, datagrams are arriving but failing to decode. Check `parse_errors` and `template_errors` to narrow down. See [Plugin Health Charts](/docs/network-flows/visualization/dashboard-cards.md).

4. **Plugin log lines.**

   ```bash
   sudo journalctl --namespace netdata --since "5 minutes ago" | grep -i netflow
   ```

## What's next

You now have flow data flowing in. The natural next steps:

- [Configuration](/docs/network-flows/configuration.md) — Tune retention after first validation; production retention should be sized from observed flow rate.
- [Static Metadata integration card](/src/crates/netflow-plugin/integrations/static_metadata.md) — Give your routers and your internal networks friendly names and labels. Without this, dashboards show raw IPs.
- [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md) — Concrete recipes for the questions flow data is good at answering.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Mistakes to avoid as you develop confidence with the data.
- [Validation and Data Quality](/docs/network-flows/validation.md) — How to confirm your numbers are correct.

For more sources or vendors:

- [NetFlow](/src/crates/netflow-plugin/integrations/netflow.md) — More vendor configurations, sampling caveats.
- [IPFIX](/src/crates/netflow-plugin/integrations/ipfix.md) — When and why to prefer IPFIX over NetFlow v9.
- [sFlow](/src/crates/netflow-plugin/integrations/sflow.md) — Different protocol, different semantics.
