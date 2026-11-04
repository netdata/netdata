# PAN-OS IPsec fixture evidence

The IPsec flow, gateway, and IKE SA fixtures were supplied in
[PR #24386](https://github.com/netdata/netdata/pull/24386). The author reports that they were captured through the
XML operational API on PAN-OS 11.1.13-h3 and sanitized. The partial fixtures represent a firewall where only some
configured tunnels and gateways have SAs.

| Fixtures | Operational command |
| --- | --- |
| `ipsec_flow.xml`, `ipsec_flow_partial.xml` | `show vpn flow` |
| `ipsec_gateway.xml` | `show vpn gateway` |
| `ipsec_ike_sa.xml`, `ipsec_ike_sa_partial.xml` | `show vpn ike-sa` |
| `ipsec_ike_sa_ikev1_failed.xml` | Unfiltered `show vpn ike-sa` during a failed IKEv1 negotiation |
| `ipsec_ike_sa_empty.xml` | Gateway-filtered `show vpn ike-sa` for a gateway without an SA |

The collector uses the **unfiltered** IKE SA command. The empty fixture confirms the empty-result shape of the
filtered command; it is not a capture of an empty unfiltered response.

The [additional source evidence](https://github.com/netdata/netdata/pull/24386#issuecomment-6613267600) uses the exact
unfiltered XML API operation on PAN-OS 11.1.13-h3. The failed-IKEv1 fixture preserves that comment's full XML response:
gateway `gw-test` (ID 5) is absent while the CLI lists a half-open negotiation with blank establishment/expiration
columns. The three other gateways remain present, including two established IKEv2 SAs. The test's matching gateway
inventory is synthetic; the comment does not include its raw inventory response.

The same report observes IKEv1 phase 1 established with tunnel flow `init`, and a cleared parent IKE SA with a
surviving child SA and flow `active`. For IKEv2 it supplies an established entry with populated times and reports
absence after parent deletion or authentication failure. The failing IKEv2 negotiation completed within one second
and was **not captured while in progress**. This establishes observed XML behavior on this version, not a guarantee
for every PAN-OS release or transient state.

Every reported established IKEv1/IKEv2 entry contains nonempty `created` and `expires`. The parser requires both
before accepting the SA list. A missing or blank time invalidates gateway observations for that cycle; it is not
treated as a down gateway. This conservative check covers an unsupported/incomplete response without inventing XML
status codes. The yearless device-local times are not parsed or compared with the Agent's clock.

The collector retains only the identities from each last complete validated flow/gateway inventory. Failed or
malformed inventory cannot establish removals. Valid inventory still updates membership when SA telemetry is
unavailable, so a confirmed removal does not depend on successful SA collection.

The two state families use snapshot gauge fields to distinguish present-but-unavailable telemetry from absence.
Unavailable fields emit empty chart values; known identities keep their charts through repeated failures without
replaying old state. Complete inventory removal stops refreshing an identity and retires its chart in that cycle.
The public chart IDs, dimensions, labels and numeric state meanings are preserved. Persistent-planner tests cover
failure, recovery, complete/invalid inventory, removals and retained-state bounds; an HTTP fixture also exercises the
shipped SDK's per-operation API-error path. These lifecycle tests are synthetic, not additional firewall captures.

The inline payloads in `collect_ipsec_state_test.go` and the gateway benchmark are synthetic. They test invalid
payload handling, identity matching, incomplete-time gaps, independent gateway/tunnel states, transitions, and
scaling; they are not evidence of device output. The incomplete-time cases are synthetic contract checks, not a
reproduction of an observed device false-up defect.

Relevant vendor evidence:

- [Failed IKEv1 negotiation](https://knowledgebase.paloaltonetworks.com/KCSArticleDetail?id=kA10g000000PP5sCAG&lang=en_US)
  shows an unestablished row in unfiltered CLI `show vpn ike-sa`. The source capture above demonstrates that CLI
  inclusion does not imply XML inclusion on the tested version.
- [IKE SA field definitions](https://knowledgebase.paloaltonetworks.com/KCSArticleDetail?id=kA10g000000PLQhCAO&lang=en_US)
  define Created as the establishment time and Expires as the SA expiration time. Combined with the captures, these
  support the populated-time requirement; they do not establish a numeric XML state mapping.
- [IPsec negotiation failure](https://knowledgebase.paloaltonetworks.com/KCSArticleDetail?id=kA14u000000kF5JCAU&lang=en_US)
  shows the flow state `init`; it is a known provider state, distinct from an unrecognized value.
