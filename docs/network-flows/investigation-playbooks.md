<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/investigation-playbooks.md"
sidebar_label: "Investigation Playbooks"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['playbooks', 'investigation', 'workflows', 'troubleshooting traffic']
endmeta-->

<!-- markdownlint-disable-file -->

# Investigation playbooks

Step-by-step recipes for common questions, all using the Netdata Network Flows view (open the **Live** tab and select **Network Flows**). Each playbook fits in a 5-15 minute investigation window.

## Playbook 1 — "The link is saturated, who's responsible?"

**The situation.** SNMP shows your Internet link at 95% utilisation. Users complain about slowness.

**The goal.** Identify the talker(s) consuming the bandwidth.

**Steps.**

1. **Open Network Flows** with the default view (Sankey + Table). Set the time range to **the last 15 minutes** — recent enough to be live, wide enough to smooth bursts.

2. **Filter to the saturated interface.** In the filter ribbon, set:
   - `Exporter Name` = the router with the saturated link
   - `Egress Interface Name` = the interface name (or `Ingress Interface Name` if you want incoming traffic)

   This eliminates the doubling effect and shows only one direction.

3. **Change the aggregation to "who's responsible".** Click the group-by selector and change the fields to:
   - `Source AS Name` → `Destination AS Name` (for an Internet-edge link)

   Or for an internal link:
   - `Source IP` → `Destination IP`

4. **Read the Sankey.** The widest band is your top talker pair. Click on the wide band to drill in.

5. **If a single ASN/IP dominates** — that's your answer. Click the value to add it as a filter and look at the table for the specific 5-tuple details.

6. **If traffic is evenly distributed** across many sources — the problem is aggregate demand, not a single offender. The link genuinely needs more capacity, or you need traffic shaping. Move to Playbook 3.

**What to record.**

- Timestamp range of the investigation
- Top 3 talker pairs and their byte volumes
- Whether this is a one-time spike or sustained
- The URL of the dashboard view (preserves all filters and aggregation)

**Common findings.**

- Backup software running during business hours.
- A SaaS sync or cloud upload.
- Misconfigured automation (e.g., logs being shipped to the wrong place).
- New user / new application doing something unexpected.

## Playbook 2 — "Investigating a specific IP"

**The situation.** A security alert references an IP address. You need to know what it talked to, when, and how much.

**The goal.** Construct a timeline and traffic profile for that IP.

**Steps.**

1. **Open Network Flows** with **the last 24 hours** as the time range.

2. **Filter by the IP.** In the filter ribbon:
   - For inbound investigation: `Destination IP` = the IP
   - For outbound investigation: `Source IP` = the IP
   - For both directions: filter both, separately, in two browser tabs

   IP filtering forces raw tier (raw retention) — the time depth is bounded by your raw-tier retention. If you need to look further back than that, the data isn't there.

3. **Switch to Time-Series view.** This shows when the IP was active. Look for:
   - When did activity start? End?
   - Is it constant, periodic, or bursty?
   - Does it correlate with a known event (deployment, business hours, maintenance window)?

4. **Switch back to Sankey + Table.** Change the group-by to surface the relevant context:
   - `Destination IP` → `Destination Port` → `Destination Country` (if the suspect is a source)
   - `Source IP` → `Source ASN` (if the suspect is a destination)

5. **Read the table.** The top rows show the IP's most-talked-to peers, ranked by bytes. Look for:
   - Unknown external IPs in unexpected geographies.
   - Connections on unusual ports (anything not in your normal protocol mix).
   - Sustained outbound transfers (potential exfiltration) vs short bursts (likely normal).

6. **For each suspicious peer, drill in.** Add the peer IP to the filter ribbon, switch back to Time-Series. Confirm the timeline aligns with the original alert.

**What to record.**

- Time range of all activity by the IP
- Top destinations and their byte/packet counts
- Whether the activity is consistent with a legitimate use (backup, SaaS sync) or anomalous
- The URL of each dashboard view used in the investigation

**Caveats.**

- IP filter forces raw tier; older data may not be available.
- If the IP is internal and you haven't declared it under `enrichment.networks`, GeoIP may misrepresent its country.
- If the IP is a NAT public address, multiple internal hosts may be hidden behind it. Cross-check with NAT translation logs.

## Playbook 3 — "Justifying a link upgrade"

**The situation.** A WAN circuit is at 80% utilisation during peak hours. Finance wants justification before approving an upgrade.

**The goal.** Produce a defensible trend showing growth and projecting the date of saturation.

**Steps.**

1. **Open Network Flows** with **the last 30 days** as the time range. (Adjust based on tier-1/5/60 retention. If your retention is shorter, use whatever you have.)

2. **Filter to the WAN interface.** Set `Exporter Name` and `Egress Interface Name` (or `Ingress Interface Name` — pick one direction). This removes the doubling effect.

3. **Switch to Time-Series view.** The chart now shows ~30 days of bandwidth on the link. The bucket size auto-adjusts to roughly 1 hour at this range.

4. **Identify the trend.** Look at the daily peaks (one curve cycle = one day). The peak should be growing month-over-month. Eyeball the slope.

5. **Identify the growth driver.** Switch back to Sankey + Table, group by `Destination ASN` or `Destination Port` (service). Compare top consumers from the start of the period to the end. New entries that weren't there 30 days ago are growth drivers.

6. **Compute the upgrade need.** Take the current peak (e.g., 80% of 100 Mbps = 80 Mbps), project forward at the observed monthly growth rate (e.g., 10%/month = ~30%/quarter), and find when it crosses 100% (or 70% if you want headroom).

   Example: if peak grows from 70 Mbps to 80 Mbps over 30 days, that's roughly 14% monthly growth. At that rate it crosses 100 Mbps in ~2 months and 200 Mbps would buy you ~1 year.

**What to record.**

- Trend chart (screenshot or shareable URL)
- Growth driver: the specific applications / services consuming the new bandwidth
- Projected saturation date and recommended upgrade timeline
**Caveats.**

- A large spike one day shouldn't drive the projection. Use weekly peaks (averaged across same-day-of-week) for stability.
- If your retention is shorter than 30 days, use what you have but caveat the projection.

## Playbook 4 — "Scoping a security alert"

**The situation.** Your IDS / EDR / SIEM fired an alert: an internal host communicated with a known-malicious external IP. You have the internal IP, the external IP, and a rough time window.

**The goal.** Determine the scope and timeline. Did other internal hosts talk to the same external IP? When did it start? How much data was exchanged?

**Steps.**

1. **Open Network Flows** with the time range covering 24 hours before the alert through now.

2. **Filter by the external IP.** In the filter ribbon: `Destination IP` = the external IP.

   This forces raw tier. Time depth is your raw-tier retention.

3. **Switch to Time-Series view.** When did communication start? Is it ongoing? Did it correlate with the alert time?

4. **Switch to Sankey + Table.** Group by `Source IP`. The result is "every internal IP that talked to this external IP, ranked by bytes".

   - If only one internal host appears, scope is contained.
   - If multiple appear, you have a broader scope. Investigate each.

5. **For the alerted internal host**, swap the filter: `Source IP` = the internal host (remove the external filter). Group by `Destination IP` → `Destination Country` → `Destination ASN`. Look for other suspicious peers.

6. **Reverse-direction check.** Switch the filter to the external IP as `Source IP` (now you're looking at incoming traffic from it). Internal hosts that received connections from the external IP show up — useful for inbound C2 / probe analysis.

7. **Geographic check.** Switch to Country Map. The location of the external IP gives a quick "where" — useful to compare against what your threat intelligence said.

**What to record.**

- All internal IPs that communicated with the external IP, time ranges, byte counts
- Other suspicious destinations the alerted host talked to in the same window
- Whether traffic is ongoing or stopped
- The dashboard URL of each view (for the incident report)

**Caveats.**

- Sampled flows can miss small connections. Beaconing at low rates may not be visible at 1-in-1000 sampling. If you sample, your security investigation has a floor.
- An external IP behind a CDN may be one of many destinations served by that infrastructure. ASN-level analysis (`Destination ASN`) is often more informative than IP.
- The malicious IP being public doesn't mean the internal host was compromised — false positives in threat intel are common. Cross-check with the host's logs.

## A note on the dashboard

All the playbooks above use the same controls: time range, filters, group-by fields, view switcher. Once you're comfortable with these four, every investigation becomes a permutation. Mastering the tool means knowing which permutation fits the question.

The URL preserves all your selections — copy it and paste into your incident-management ticket so anyone reviewing has the exact same view you saw.

## What's next

- [Sankey and Table](/docs/network-flows/visualization/summary-sankey.md) — Full reference for the default view.
- [Filters and Facets](/docs/network-flows/visualization/filters-facets.md) — How to narrow effectively.
- [Time-Series](/docs/network-flows/visualization/time-series.md) — Trends over the time range.
- [Anti-patterns](/docs/network-flows/anti-patterns.md) — What "wrong" looks like and why.
- [Validation and Data Quality](/docs/network-flows/validation.md) — Confirming your numbers before acting on them.
