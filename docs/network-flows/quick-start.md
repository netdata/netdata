<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/quick-start.md"
sidebar_label: "Quick Start"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['quick start', 'netflow', 'sflow', 'ipfix', 'getting started', 'setup']
endmeta-->

# Quick Start

Get flow monitoring running in 15 minutes. The path: install the plugin, configure your first router, open the dashboard, and read it correctly.

## Before you start

- The Netdata Agent is running on the host that will collect flow data.
- The [netflow plugin is installed](/docs/network-flows/installation.md) on that host.
- You can configure flow export on at least one router or switch.
- The router can reach the agent's IP on UDP port 2055.

If the plugin isn't installed yet, follow the [Installation page](/docs/network-flows/installation.md) first.

## Step 1 — Configure your router

Pick the closest match to your platform. The configurations below set sensible defaults: 60-second active timeout (industry best practice), 60-second template refresh (so a collector restart recovers in under a minute), and monitoring on both directions of an interface.

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
set forwarding-options sampling instance NETDATA family inet output flow-server 10.0.0.10 port 2055
set forwarding-options sampling instance NETDATA family inet output flow-server 10.0.0.10 version9 template ipv4-template
set services flow-monitoring version9 template ipv4-template flow-active-timeout 60
set services flow-monitoring version9 template ipv4-template flow-inactive-timeout 15
set services flow-monitoring version9 template ipv4-template template-refresh-rate seconds 60
set interfaces ge-0/0/1 unit 0 family inet sampling input
set interfaces ge-0/0/1 unit 0 family inet sampling output
```

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

For more vendors and details, see [Sources / NetFlow](/src/crates/netflow-plugin/integrations/netflow.md), [IPFIX](/src/crates/netflow-plugin/integrations/ipfix.md), and [sFlow](/src/crates/netflow-plugin/integrations/sflow.md).

## Step 2 — Open the dashboard

In your browser, open the Netdata UI and click the **Network Flows** tab.

By default you'll see:

- A Sankey diagram on top, with a sortable table beneath
- The default time range — last 15 minutes (Netdata's global picker)
- Top-25 flows by bytes
- Aggregated as **Source ASN → Protocol → Destination ASN**

Within 60-90 seconds of the router being configured, flow records should start appearing.

## Step 3 — Read the dashboard correctly

Before drawing any conclusion, read this. It's the single biggest source of confusion when people first look at flow data.

### Traffic looks doubled

Routers normally export both ingress and egress flow records on every monitored interface. A packet that enters interface A and leaves interface B produces **two** records — one ingress on A, one egress on B.

If you look at total bandwidth without filtering, you see roughly **2× the real traffic**. Add a second router on the same path and you see 4×.

**To see real bandwidth on a specific link**, filter to one exporter and one direction:

1. In the filter ribbon: `Exporter Name = <your router>`.
2. Add: `Input Interface Name = <the interface>` (for incoming) **or** `Output Interface Name = <the interface>` (for outgoing). Pick one. Not both.

That's the actual traffic on that link in that direction.

### Conversations look mirrored

Each bidirectional conversation produces two flow records — one for the request direction, one for the response. The Sankey, country map, and time-series all show both. When you see traffic between Country X and Country Y *and* traffic between Country Y and Country X of similar volume, that's the same conversation, not two.

This is correct behaviour. To see only one direction of a conversation, filter by `Source ASN` (your network) for outbound or `Destination ASN` for inbound.

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
   sudo journalctl -u netdata --since "5 minutes ago" | grep -i netflow
   ```

## What's next

You now have flow data flowing in. The natural next steps:

- [Configuration](/docs/network-flows/configuration.md) — Tune retention so older data is preserved (the default 7-day shared retention is rarely enough).
- [Static metadata](/docs/network-flows/enrichment/static-metadata.md) — Give your routers and your internal networks friendly names and labels. Without this, dashboards show raw IPs.
- [Investigation Playbooks](/docs/network-flows/investigation-playbooks.md) — Concrete recipes for the questions flow data is good at answering.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — Mistakes to avoid as you develop confidence with the data.
- [Validation and Data Quality](/docs/network-flows/validation.md) — How to confirm your numbers are correct.

For more sources or vendors:

- [NetFlow](/src/crates/netflow-plugin/integrations/netflow.md) — More vendor configurations, sampling caveats.
- [IPFIX](/src/crates/netflow-plugin/integrations/ipfix.md) — When and why to prefer IPFIX over NetFlow v9.
- [sFlow](/src/crates/netflow-plugin/integrations/sflow.md) — Different protocol, different semantics.
