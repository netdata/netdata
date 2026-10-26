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
| `ipsec_ike_sa_empty.xml` | Gateway-filtered `show vpn ike-sa` for a gateway without an SA |

The collector uses the **unfiltered** IKE SA command. The empty fixture confirms the empty-result shape of the
filtered command; it is not a capture of an empty unfiltered response. The fixtures do not include a failed or
negotiating IKE SA. [Additional captures have been requested](https://github.com/netdata/netdata/pull/24386#issuecomment-6419008676)
to establish that XML contract.

The inline payloads in `collect_ipsec_state_test.go` and the gateway benchmark are synthetic. They test invalid
payload handling, identity matching, state transitions, and scaling; they are not evidence of device output.

Relevant vendor evidence:

- [Failed IKEv1 negotiation](https://knowledgebase.paloaltonetworks.com/KCSArticleDetail?id=kA10g000000PP5sCAG&lang=en_US)
  shows an unestablished row in unfiltered CLI `show vpn ike-sa`. A matching row alone therefore needs XML-specific
  validation before it can establish a healthy gateway.
- [IKE SA field definitions](https://knowledgebase.paloaltonetworks.com/KCSArticleDetail?id=kA10g000000PLQhCAO&lang=en_US)
  define Created as the establishment time. This does not establish which fields the XML API emits for failed SAs.
- [IPsec negotiation failure](https://knowledgebase.paloaltonetworks.com/KCSArticleDetail?id=kA14u000000kF5JCAU&lang=en_US)
  shows the flow state `init`; it is a known provider state, distinct from an unrecognized value.
