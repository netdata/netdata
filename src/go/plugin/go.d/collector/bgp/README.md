<!-- markdownlint-disable MD043 -->

# BGP collector

## Overview

This collector monitors BGP daemon health and peer activity.

The current implementation supports:

- FRR through the local `bgpd.vty` and `zebra.vty` Unix sockets
- BIRD through the local control socket, usually `bird.ctl`
- GoBGP through its gRPC API, usually on `127.0.0.1:50051`
- OpenBGPD through a read-only HTTP JSON endpoint such as `bgplgd` or an
  equivalent state-server

It focuses on:

- collector scrape health and backend reachability
- bounded family and peer chart scope
- peer state
- family and peer activity counters
- neighbor churn counters
- received prefix counts
- accepted, filtered, and advertised prefix counts when the backend
  exposes them
- peer uptime
- family-level summaries per VRF or routing-table scope and AFI/SAFI
- backend-specific session diagnostics when the daemon exposes them
  cheaply

Backend-specific notes:

- FRR:
  - session-level neighbor churn comes from FRR updates, notifications,
    and route-refresh counters
  - neighbor message-type breakdowns, transition counters, and
    last-reset details come from `show bgp vrf all neighbors json`
  - bounded EVPN family summaries and VNI charts are supported through
    `zebra.vty`
  - optional deeper FRR route queries fill accepted, filtered, and
    advertised prefix counters only when cheap summary and neighbors
    JSON counters are missing
- BIRD:
  - the collector uses one direct `show protocols all` query in the
    normal loop
  - BIRD routing table names are mapped into Netdata's current `vrf`
    selector slot and also exposed as an explicit `table` label
  - family and peer activity charts use BIRD route-change statistics as
    the nearest daemon-native signal, not full BGP message totals
  - neighbor churn charts use BIRD import and export update and withdraw
    counters
  - BIRD does not currently provide Netdata with FRR-style transition
    counters, full message-type counters, last-reset details, EVPN VNI
    charts, or deep per-peer route queries through this path
- OpenBGPD:
  - the collector uses a read-only HTTP JSON endpoint and auto-detects
    the common bgplgd or state-server path layout once from the
    configured base URL
  - family and peer state come from the cheap `neighbors` path
  - session message and churn counters stay on neighbor charts; family
    and peer message totals are only attached when the session exposes
    exactly one negotiated AFI/SAFI, so Netdata does not double-count
    multi-family sessions
  - optional cached `/rib` summaries add family route-table counts and
    aggregated correctness counts without querying the full RIB on every
    scrape
- GoBGP:
  - the collector uses the gRPC `GoBgpService` API directly
  - the cheap healthy path is `GetBgp` + `ListPeer` plus per-family
    `GetTable` counts
  - VRF-aware families use the peer `vrf` value and switch table lookups
    to `TABLE_TYPE_VRF`
  - session message and churn counters stay on neighbor charts; family
    and peer message totals are only attached when the session exposes
    exactly one enabled AFI/SAFI, so Netdata does not double-count
    multi-family sessions
  - optional cached validation summaries use `ListPath` only when
    `collect_rib_summaries: yes`

## Collected metrics

Collector-level metrics:

- `bgp.collector_status`
- `bgp.collector_scrape_duration`
- `bgp.collector_failures`
- `bgp.collector_deep_queries`

Family-level metrics:

- `bgp.family_peer_states`
- `bgp.family_peer_inventory`
- `bgp.family_messages`
- `bgp.family_prefixes_received`
- `bgp.family_rib_routes`
- `bgp.family_correctness` (GoBGP and OpenBGPD only, and only when
  `collect_rib_summaries: yes`)

Peer-level metrics:

- `bgp.peer_messages`
- `bgp.peer_prefixes_received`
- `bgp.peer_prefixes_policy`
- `bgp.peer_prefixes_advertised`
- `bgp.peer_uptime`
- `bgp.peer_state`

Neighbor-level metrics:

- `bgp.neighbor_churn`
- `bgp.neighbor_transitions` (FRR only)
- `bgp.neighbor_message_types` (FRR only)
- `bgp.neighbor_last_reset_state` (FRR only)
- `bgp.neighbor_last_reset_age` (FRR only)
- `bgp.neighbor_last_error_codes` (FRR only)

EVPN VNI metrics:

- `bgp.evpn_vni_entries` (FRR only)
- `bgp.evpn_vni_remote_vteps` (FRR only)

## Stock alerts

- `bgp_collector_backend_unreachable`
  - critical when Netdata cannot reach the configured BGP backend for
    the full 2-minute evaluation window
- `bgp_peer_down`
  - critical when a peer remains fully down for the full 2-minute
    evaluation window
- `bgp_family_no_established_peers`
  - critical when a family has peers configured, none are established,
    and at least one is down
- `bgp_peer_prefixes_received_anomaly`
  - ML-based alert for unusual received-prefix behavior on a peer
- `bgp_family_messages_anomaly`
  - ML-based alert for unusual family-level BGP message churn
- `bgp_neighbor_churn_anomaly`
  - ML-based alert for unusual neighbor-scoped churn traffic without
    keepalives
- `bgp_peer_transitions_anomaly`
  - ML-based alert for unusual neighbor connection drops that usually
    indicate flap behavior where the backend exposes transition counters
- `bgp_family_correctness_invalid`
  - critical when the backend-native aggregated correctness view reports
    one or more invalid routes for a charted family

## Configuration

Basic FRR example:

``` yaml
jobs:
  - name: local
    backend: frr
    socket_path: /var/run/frr/bgpd.vty
    zebra_socket_path: /var/run/frr/zebra.vty
```

Basic BIRD example:

``` yaml
jobs:
  - name: local-bird
    backend: bird
    socket_path: /var/run/bird.ctl
```

Basic OpenBGPD example:

``` yaml
jobs:
  - name: local-openbgpd
    backend: openbgpd
    api_url: http://127.0.0.1:8080
```

Basic GoBGP example:

``` yaml
jobs:
  - name: local-gobgp
    backend: gobgp
    address: 127.0.0.1:50051
```

Bound peer charts when many peers exist:

``` yaml
jobs:
  - name: edge
    backend: frr
    socket_path: /var/run/frr/bgpd.vty
    zebra_socket_path: /var/run/frr/zebra.vty
    max_families: 10
    select_families:
      includes:
        - "=default/ipv4/unicast"
    max_peers: 100
    select_peers:
      includes:
        - "=192.0.2.1"
        - "=2001:db8::1"
    max_vnis: 20
    select_vnis:
      includes:
        - "=default/172192"
```

Stock alerts only evaluate charted families and charted peers. If the
environment is larger than `max_families` or `max_peers`, use
`select_families` and `select_peers` deliberately so the routers, VRFs,
and peers you care about stay inside alert coverage. If no selector is
configured, Netdata keeps a deterministic bounded fallback instead of
dropping chart selection to zero.

Enable deeper FRR prefix policy counters for a bounded peer set:

``` yaml
jobs:
  - name: edge-deep
    backend: frr
    socket_path: /var/run/frr/bgpd.vty
    zebra_socket_path: /var/run/frr/zebra.vty
    max_families: 10
    select_families:
      includes:
        - "=default/ipv4/unicast"
    max_peers: 10
    deep_peer_prefix_metrics: yes
    max_deep_queries_per_scrape: 20
    select_peers:
      includes:
        - "=192.0.2.1"
        - "=2001:db8::1"
```

Enable cached OpenBGPD RIB summaries and correctness counts:

``` yaml
jobs:
  - name: local-openbgpd-rib
    backend: openbgpd
    api_url: http://127.0.0.1:8080
    collect_rib_summaries: yes
    rib_summary_every: 60
```

Enable cached GoBGP validation summaries:

``` yaml
jobs:
  - name: local-gobgp-rpki
    backend: gobgp
    address: 127.0.0.1:50051
    collect_rib_summaries: yes
    rib_summary_every: 60
```

## Requirements

- FRR:
  - FRR must expose the `bgpd.vty` Unix socket.
  - FRR must expose the `zebra.vty` Unix socket for EVPN VNI charts.
  - The `netdata` user must be able to open those sockets.
  - Treat access to `bgpd.vty` and `zebra.vty` as privileged FRR CLI
    access, not as harmless read-only sockets.
  - FRR must support JSON output for
    `show bgp vrf all ipv4 summary json` and
    `show bgp vrf all ipv6 summary json`.
  - EVPN family summaries require FRR support for
    `show bgp vrf all l2vpn evpn summary json`.
  - EVPN VNI charts require FRR support for `show evpn vni json`.
  - For peer descriptions, peer groups, cheap accepted-prefix counters,
    neighbor message-type counters, neighbor transition counters, and
    last-reset details, FRR should also support
    `show bgp vrf all neighbors json`.
  - For `deep_peer_prefix_metrics`, FRR should also support peer route
    JSON commands such as `show bgp ... neighbors ... routes json` and
    `show bgp ... neighbors ... advertised-routes json`. Netdata only
    uses them as fallback when summary and neighbors JSON do not already
    expose the needed prefix counters.
- BIRD:
  - BIRD must expose a control socket, usually `/var/run/bird.ctl`.
  - The `netdata` user must be able to open that socket.
  - Prefer a restricted read-only socket when the deployment allows it.
  - Prefer BIRD 3 `cli { v2 attributes; }` when you need current BIRD 3
    daemons to keep `show protocols all` output compatible with this
    collector path.
  - The collector currently depends on `show protocols all` output with
    BGP protocol details, channel sections, route summaries, and
    route-change statistics.
- OpenBGPD:
  - OpenBGPD must expose a read-only HTTP JSON endpoint such as `bgplgd`
    or an equivalent state-server.
  - `api_url` must point to the base URL for that endpoint.
  - If `collect_rib_summaries` is enabled, the same endpoint must expose
    a JSON RIB path.
  - Treat direct `bgpd` control sockets as a different trust boundary;
    this collector path intentionally uses the read-only HTTP interface
    instead.
- GoBGP:
  - GoBGP must expose the `GoBgpService` gRPC API.
  - `address` must point to that endpoint in `host:port` form.
  - Use TLS only when the GoBGP deployment actually exposes TLS on the
    gRPC listener; otherwise leave TLS options unset and Netdata uses
    insecure localhost gRPC.
  - If `collect_rib_summaries` is enabled, GoBGP must allow `ListPath`
    on the relevant global or VRF table families.
- ML-based alerts require Netdata's ML anomaly detection to be enabled
  and trained for the relevant charts.

## Troubleshooting

- If collection fails with a permissions error, verify socket ownership
  and group membership for `netdata` on the configured backend socket,
  verify access control on the configured OpenBGPD HTTP endpoint, or
  verify that the configured GoBGP gRPC listener is reachable from the
  Netdata host.
- If the `bgp.collector_status` chart shows `permission_error`,
  `timeout`, or `query_error`, treat that as a backend access problem
  first, not a routing problem.
- If collection fails with a parse error:
  - for FRR, run the FRR JSON commands manually and compare the output
    with the fixtures in `testdata/frr/`
  - for BIRD, run `show protocols all` manually and compare the output
    with the fixtures in `testdata/bird/`
  - for OpenBGPD, query the HTTP JSON endpoint manually and compare the
    output with the fixtures in `testdata/openbgpd/`
- If the GoBGP backend fails, verify the gRPC endpoint first:
  - confirm the configured `address` answers
  - confirm TLS settings match the listener, if TLS is enabled
  - confirm the daemon exposes `GetBgp`, `ListPeer`, `GetTable`, and,
    when enabled, `ListPath`
- If peer descriptions, peer groups, neighbor transition counters, or
  cheap accepted-prefix counters are missing, verify that
  `show bgp vrf all neighbors json` works on the same router. The
  collector treats this enrichment as best-effort and still collects
  summary metrics without it.
- If `show bgp vrf all neighbors json` fails transiently after
  previously working, Netdata reuses the last good neighbor snapshot so
  the neighbor transition and neighbor message-type charts do not fake a
  counter reset.
- If last-reset charts stay empty, verify that
  `show bgp vrf all neighbors json` includes FRR reset fields such as
  `lastReset`, `lastErrorCodeSubcode`, or `downLastResetTimeSecs`.
- If you monitor many VRFs or AFI/SAFI families, use `max_families` and
  `select_families` to keep family chart creation bounded.
- If you monitor many EVPN VNIs, use `max_vnis` and `select_vnis` to
  keep VNI chart creation bounded.
- If `deep_peer_prefix_metrics` is enabled, keep the charted peer set
  small. FRR exporters warn that per-peer route queries can be slow
  because FRR executes them serially even when the caller parallelizes
  requests.
- `max_deep_queries_per_scrape` is one budget for the whole scrape, not
  one budget per peer. If the budget is too small, later charted peers
  will keep summary metrics but may miss fallback accepted or advertised
  prefix counters.
- Use `bgp.collector_scrape_duration`, `bgp.collector_failures`, and
  `bgp.collector_deep_queries` to see whether deep mode is making the
  collector slow, error-prone, or budget-limited on a specific router.
- If accepted/filtered prefix charts stay empty, first verify that
  `show bgp vrf all neighbors json` exposes `acceptedPrefixCounter` for
  the relevant address family. Only then fall back to checking the
  per-peer route JSON commands and `deep_peer_prefix_metrics`.
- `deep_peer_prefix_metrics` is a rare safety net, not a normal code
  path. On a healthy modern FRR (8.0 or later) with a warm neighbor
  cache the fallback never fires because the cheap summary `pfxSnt` and
  neighbor `acceptedPrefixCounter` fields already set the required
  flags. The fallback only activates for Established, selected peers
  in three realistic situations: FRR < 7.0 (where neighbor
  `acceptedPrefixCounter` and `sentPrefixCounter` did not exist yet),
  FRR 7.0 through 7.5.x for peers without an update-subgroup (where
  summary `pfxSnt` and neighbor `sentPrefixCounter` are still
  conditional on the subgroup attachment), and a cold-scrape
  degraded state on any FRR version including current 10.6 when the
  `show bgp vrf all neighbors json` query fails or parses badly and
  the collector's neighbor cache is still empty. Leave
  `deep_peer_prefix_metrics` disabled unless you have a specific
  reason to enable it; per-peer route queries are expensive on
  large routers.
- If `zebra.vty` is unavailable, Netdata still collects family, peer,
  and neighbor charts. EVPN VNI collection stays best-effort, and the
  failed VNI query appears under `bgp.collector_failures`.
- If EVPN VNI charts stay empty, first verify that
  `show bgp vrf all l2vpn evpn summary json` returns at least one EVPN
  family and `show evpn vni json` returns VNI objects on the same
  router.
- Family charts are AFI/SAFI-scoped. Neighbor transition, neighbor
  churn, neighbor message-type, and last-reset charts are VRF plus peer
  scoped so a multi-family FRR session is not counted once per family.
- For OpenBGPD, session counters remain on neighbor charts when one
  session negotiates multiple AFI/SAFI families. Netdata does not
  duplicate those counters into multiple family charts.
- For GoBGP, session counters remain on neighbor charts when one session
  enables multiple AFI/SAFI families. Netdata does not duplicate those
  counters into multiple family or peer charts.
- `collect_rib_summaries` is intentionally optional for OpenBGPD. It
  adds family route-table counts and correctness summaries, but it still
  depends on a full RIB JSON response from the daemon-side service.
- `collect_rib_summaries` is intentionally optional for GoBGP too. It
  adds cached validation summaries, but the healthy path still keeps
  route-table counts on the cheaper `GetTable` API.
- For BIRD, the current `vrf` selector slot is the BIRD routing table
  name. Netdata also exposes that raw table name as a `table` label on
  family, peer, and neighbor charts.
- For FRR, the `bgp.neighbor_churn` chart leaves `withdraws_received`
  and `withdraws_sent` at zero because FRR's current JSON API does not
  expose per-peer normal-withdraw counters. The only `withdrawn` field
  FRR emits under `show bgp neighbor json` is a treat-as-withdraw
  counter (RFC 7606 malformed-attribute error recovery and the
  configured `neighbor ... path-attribute treat-as-withdraw` policy),
  not normal withdraw NLRIs. The normal withdraw path in FRR `bgpd`
  does not increment any per-peer counter on the success path, so
  Netdata leaves these dimensions at zero rather than synthesizing a
  misleading proxy from other counters. Use `updates_received` and
  `updates_sent` on the same chart for FRR churn visibility instead,
  or use the BIRD backend for true per-direction withdraw counters
  (BIRD exposes them from `show protocols all` and Netdata fills both
  dimensions directly). This is a current FRR JSON limitation, not a
  permanent one.
- EVPN VNI charts are tenant-VRF and VNI scoped. They stay separate from
  peer charts so Netdata keeps one clean instance type per context.
- If ML-based alerts stay quiet on a new installation, confirm that ML
  is enabled and give Netdata enough time to train on the BGP charts.

## Testing

- Fast collector tests:
  - `go test ./plugin/go.d/collector/bgp`
- The fast suite now also includes a real in-process GoBGP gRPC replay
  server test built on the pinned upstream `v4.4.0` protobuf service.
- BIRD socket protocol and parser tests are part of that fast suite and
  use Netdata-owned fixtures under `testdata/bird/`.
- OpenBGPD parser and HTTP path-detection tests are part of that fast
  suite and use Netdata-owned fixtures under `testdata/openbgpd/`.
- Opt-in live GoBGP integration test:
  - command:

    ```sh
    BGP_GOBGP_ENABLE_INTEGRATION=1 \
      go test -tags=integration \
      -run TestIntegration_GoBGPLiveCollection \
      -v ./plugin/go.d/collector/bgp
    ```

  - default runtime image:
    - `BGP_GOBGP_DOCKER_IMAGE=debian:bookworm-slim`
  - optional source override:
    - `BGP_GOBGP_SOURCE_PATH=/opt/baddisk/monitoring/bgp/osrg__gobgp`
- Opt-in live BIRD integration test:
  - command:

    ```sh
    BGP_BIRD_ENABLE_INTEGRATION=1 \
      go test -tags=integration \
      -run TestIntegration_BIRDLiveCollection \
      -v ./plugin/go.d/collector/bgp
    ```

  - default images:
    - `BGP_BIRD2_DOCKER_IMAGE=debian:bookworm-slim`
    - `BGP_BIRD3_DOCKER_IMAGE=debian:trixie-slim`
- Opt-in live FRR integration test:
  - command:

    ```sh
    BGP_FRR_ENABLE_INTEGRATION=1 \
      go test -tags=integration \
      -run TestIntegration_FRRLiveCollection \
      -v ./plugin/go.d/collector/bgp
    ```

- Optional image override for integration testing:
  - `BGP_FRR_DOCKER_IMAGE=quay.io/frrouting/frr:10.6.0`
- The live GoBGP test builds real `gobgp` and `gobgpd` binaries from the
  mirrored upstream source tree, starts a real `gobgpd` container, seeds
  one default peer plus one VRF peer and local routes, and validates
  Netdata against the live gRPC API.
- The live FRR test uses the official FRR container and its documented
  privileged startup model, mounts a host runtime directory, and
  validates collection through the real `bgpd.vty` and `zebra.vty`
  socket paths.
