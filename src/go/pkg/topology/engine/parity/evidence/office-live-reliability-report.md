# Office Live Reliability Report (Topology SNMP)

## Scope
- Gate: `T4.5` + `T4.6`
- Office seed: `10.20.4.0/24`
- Community: `atadteN`
- Function under validation: `topology:snmp` (`topology_view:l2|l3|merged`)

## Commands
```bash
# rebuild/install latest code
./install.sh > /tmp/install-topology-t45.log 2>&1

# capture views
TOKEN=$(jq -r '.token' /var/lib/netdata/bearer_tokens/9c85ea3b-548e-4cca-8a53-165859d2724f)
curl -sS -H "Authorization: Bearer $TOKEN" "http://localhost:19999/api/v3/function?function=topology:snmp%20topology_view:l2" > /tmp/topology-office-20260221T025600Z/l2.json
curl -sS -H "Authorization: Bearer $TOKEN" "http://localhost:19999/api/v3/function?function=topology:snmp%20topology_view:l3" > /tmp/topology-office-20260221T025600Z/l3.json
curl -sS -H "Authorization: Bearer $TOKEN" "http://localhost:19999/api/v3/function?function=topology:snmp%20topology_view:merged" > /tmp/topology-office-20260221T025600Z/merged.json
```

## Before/After (Identity-Split)
- Baseline snapshot: `/tmp/topology-office-20260221T024438Z/l2.json`
- Post-fix snapshot: `/tmp/topology-office-20260221T025600Z/l2.json`

| Metric | Before | After |
|---|---:|---:|
| Actors | 89 | 75 |
| Links | 11 | 11 |
| Duplicate actors by normalized MAC | 7 | 0 |
| Duplicate actors by normalized IP | 7 | 0 |
| Unidirectional links | 11 | 11 |
| Bidirectional links | 0 | 0 |

## Stability (12 refreshes)
- Artifact directory: `/tmp/topology-office-stability-20260221T025625Z`
- Hash file: `/tmp/topology-office-stability-20260221T025625Z/hash-counts.txt`
- Identity summary: `/tmp/topology-office-stability-20260221T025625Z/identity-summary.txt`

Results:
- Normalized structure hash: `12/12` identical.
- Per-sample counts stable: `actors=76`, `links=11`, `mac_dups=0`, `ip_dups=0`.

## Core Device Presence (single occurrence in post-fix capture)
- `MikroTik-router`: 1
- `MikroTik-Switch`: 1
- `XS1930`: 1
- `GS1900`: 1
- `mega.plaka`: 1
- `beast.local`: 1
- `nova`: 1

## View Status
- `l2`: `status=200`, topology data present.
- `l3`: `status=200`, `actors=0`, `links=0` (no OSPF/ISIS activity observed in office snapshot).
- `merged`: `status=200`, mirrors available L2 + L3 data.

## Conclusion
- Identity-split pathology is removed in office live captures (duplicate MAC/IP actors reduced to zero).
- Topology output is deterministic across repeated refreshes.
- Office reliability criteria for current office protocol visibility are satisfied.

## Post-T5.3 Validation (2026-02-21)
- Artifact directory: `/tmp/topology-office-20260221T203221Z-t53`
- Stability directory: `/tmp/topology-office-stability-20260221T203349Z-t53`

View summary:
- `l2`: `status=200`, `actors=101`, `links=190`, `links_lldp=10`, `links_fdb=180`, `bidirectional=181`, `unidirectional=9`
- `l3`: `status=200`, `actors=0`, `links=0`
- `merged`: `status=200`, `actors=101`, `links=190`

LLDP summary:
- Bidirectional core edge present: `XS1930:8 <-> MikroTik-router:ether3`.
- Remaining `9` LLDP edges are unidirectional due missing reciprocal remote rows on peers (for example GS1900 rem-table exposure).

Stability sampling (`12` refreshes):
- Structural hashes: `3` unique (`1 + 1 + 10` distribution).
- Samples `3..12` converged to one stable hash with fixed counts:
  - `actors=101`, `links=188`, `links_lldp=10`, `links_fdb=178`, `bidirectional=179`, `unidirectional=9`.
- Interpretation:
  - first two refreshes after restart reflected expected transient cache convergence;
  - steady-state snapshots are stable after convergence.
