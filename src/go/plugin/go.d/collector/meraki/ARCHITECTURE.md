# Meraki discovery and availability

The `meraki` CollectorV2 monitors one configured Dashboard organization. It uses the collector-local
[`merakiapi` client](api-client.md) for read-only cloud API requests. It does not connect to individual devices.

## Collection contract

`Init` validates configuration and constructs the owned HTTP transport. `Check` reads one small assigned-device
page for the configured organization. Empty results are valid; Check does not traverse inventory or write metrics.
The organization-list endpoint alone cannot prove operational access because it is exempt from organization IP
restrictions.

Each collection completely reads organizations, organization networks, assigned devices and device availability,
then normalizes the result before observing metrics. A required request, decoding or core-inventory error fails the
whole cycle. The framework owns begin, commit, abort and chart publication. Failed cycles publish no fresh device
values and do not establish removal. A valid empty organization succeeds and emits an assigned-device count of zero.

There is no discovery cache, background scheduler, automatic retry loop or durable collector state. `timeout` bounds
the complete Init, Check or Collect operation, including credential construction, pagination and pacing. HTTP client
requests also use that timeout. Default polling is 300 seconds; default timeout is 60 seconds.

## Membership and availability

Assigned-device inventory owns membership. Join availability by exact serial within the configured organization.
Availability-only rows do not create devices. Both nonblank network IDs must agree before status is accepted; absent
network metadata does not remove a device.

| Evidence | Observation |
|---|---|
| Assigned serial present | `device_assigned = 1` on its device node |
| Known compatible status | One active enum state: `online`, `alerting`, `offline` or `dormant` |
| Missing, unknown, conflicting or network-mismatched availability | Omit status; keep inventory membership |
| Identical normalized inventory duplicate | Deduplicate |
| Missing serial or conflicting normalized assigned-device records | Fail the whole cycle |
| Conflicting availability records | Omit only that device's status |
| Assigned serial absent after complete successful discovery | Stop observations; framework retires its charts |

Meraki refreshes availability every five minutes and supplies no sample timestamp. Request completion is not the
measurement time. The collector neither invents offline observations nor replays prior status as fresh. Inventory
membership describes assignment, not health. Optional tags, firmware, addresses and other unused inventory data do
not become metric identity.

## Identity and publication

Each assigned device gets a deterministic SHA1 UUID in the DNS namespace, using `meraki:` followed by the byte length
of the organization ID, `:`, the organization ID and the serial. Length framing prevents opaque-component collisions.
That UUID is both HostScope key and GUID. Names and network membership are display metadata, so renames and network
moves preserve identity. Device names become hostnames when wire-safe; otherwise a UUID-based hostname is used.

Host labels include `_vnode_type=meraki_device`. Organization counts use the default job scope; `vnode` only changes
that summary's placement. Configured-host authority, publication ownership, chart expiry and scope retirement remain
framework responsibilities. Retirement does not promise immediate deletion of historical Netdata nodes.

The assigned-inventory chart anchors nodes whose status is missing: V2 discovers live scopes from fresh metric
series and publishes hosts only when there are chart actions. Instruments have fixed names and per-cycle scoped
bindings; no permanent Vec cache retains renamed label tuples. Cardinality is one node per assigned serial, with
one inventory chart and, when known, one availability chart.

## Request budgets and credentials

Actual HTTP starts are spaced by at least 100 ms per job, covering pages and redirects, following Meraki's 10 requests
per second steady-state organization budget. Organization and source-IP quotas are shared with other jobs and apps;
local pacing does not reserve quota. A 429 fails the cycle. A valid `Retry-After` sets a transient not-before time;
Check and Collect issue no requests before it expires. There is no sleep until that time and no background retry.
The error carries that absolute deadline through `collectorapi.WithRetryNotBefore`, so Job Manager also preserves
the minimum when a failed startup instance is disposed and reconstructed. Repeated locally suppressed checks keep
the original deadline. The annotation grants no retry permission and does not schedule ordinary collections;
existing explicit manual acceptance and fresh enablement retain the framework's reset semantics.

API keys can use framework secret references. Bearer-token files are reread through the shared reduced-authority
credential helper for every page/redirect. TLS files use the same shared authority boundary at initialization. The
collector exposes only effective HTTP/TLS/proxy options; redirect and authentication policy remain client-owned.
Cleanup closes owned idle connections and is safe before Init or after prior cleanup.
