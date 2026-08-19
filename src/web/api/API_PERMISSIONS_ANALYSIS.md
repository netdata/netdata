# Netdata API Permissions and Access Control Analysis

**Verified:** 2026-07-16
**Purpose:** Document the route ACL and `HTTP_ACCESS` requirements for the 68
API route registrations in a normal production DBEngine build.

The count covers 27 v3, 17 v2, and 24 v1 route-table entries. The v1
`dbengine_stats` entry is compiled only with `ENABLE_DBENGINE`; without it the
inventory is 67. Root routes and the `/host/<id>/...` and `/node/<id>/...`
aliases are dispatch paths, not additional API registrations.

## Permission System Overview

### HTTP_ACL (Access Control List)

Controls the transport/source and feature category accepted by a route:

- `HTTP_ACL_NOCHECK` - Skip only the route-table ACL comparison; independent
  connection, coarse-dispatch, `HTTP_ACCESS`, bearer-derived access, and
  callback-local checks still apply where present
- `HTTP_ACL_API` - Via HTTP/HTTPS web server (TCP port 19999)
- `HTTP_ACL_ACLK` - **Via ACLK only** (Netdata Cloud connection)
- `HTTP_ACL_WEBRTC` - Via WebRTC connection
- `HTTP_ACL_METRICS` - Metrics data access category
- `HTTP_ACL_FUNCTIONS` - Functions execution category
- `HTTP_ACL_NODES` - Node information category
- `HTTP_ACL_ALERTS` - Alerts access category
- `HTTP_ACL_DYNCFG` - Dynamic configuration category
- `HTTP_ACL_REGISTRY` - Registry access category
- `HTTP_ACL_BADGES` - Badges generation category
- `HTTP_ACL_MANAGEMENT` - Management operations category
- `HTTP_ACL_STREAMING` - Streaming category
- `HTTP_ACL_NETDATACONF` - Netdata configuration category

### HTTP_ACCESS (Permission Flags)

Controls **WHAT** the authenticated user can do (capabilities):

- `HTTP_ACCESS_NONE` - No route-table access bit is required; this does not
  bypass the other authorization layers
- `HTTP_ACCESS_SIGNED_ID` - User must be authenticated
- `HTTP_ACCESS_SAME_SPACE` - User and agent must be in same Netdata Cloud space
- `HTTP_ACCESS_COMMERCIAL_SPACE` - Requires commercial plan
- `HTTP_ACCESS_ANONYMOUS_DATA` - Can view basic metrics/data
- `HTTP_ACCESS_SENSITIVE_DATA` - Can view sensitive information
- `HTTP_ACCESS_VIEW_AGENT_CONFIG` - Can read agent configuration
- `HTTP_ACCESS_EDIT_AGENT_CONFIG` - Can modify agent configuration
- `HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG` - Can read notifications config
- `HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG` - Can modify notifications config
- `HTTP_ACCESS_VIEW_ALERTS_SILENCING` - Can read silencing rules
- `HTTP_ACCESS_EDIT_ALERTS_SILENCING` - Can modify silencing rules

### HTTP_USER_ROLE

Roles are authentication metadata carried with the access mask. The generic
route dispatcher does not compare role ordinals: it authorizes against the
required `HTTP_ACCESS` bits. Do not infer endpoint authorization from role names
alone.

The enum values are `HTTP_USER_ROLE_NONE`, `HTTP_USER_ROLE_ADMIN`,
`HTTP_USER_ROLE_MANAGER`, `HTTP_USER_ROLE_TROUBLESHOOTER`,
`HTTP_USER_ROLE_OBSERVER`, `HTTP_USER_ROLE_MEMBER`, `HTTP_USER_ROLE_BILLING`,
and `HTTP_USER_ROLE_ANY`.

---

## V3 APIs (27 total) - CURRENT/LATEST

### Anonymous-data APIs

These require `HTTP_ACCESS_ANONYMOUS_DATA`. Unauthenticated requests receive
that bit only while bearer protection is disabled. Their route ACL and all
earlier transport/connection gates also apply.

| API | ACL | Access | Description |
|-----|-----|--------|-------------|
| `/api/v3/data` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Time-series data query |
| `/api/v3/badge.svg` | `HTTP_ACL_BADGES` | `ANONYMOUS_DATA` | Badge generation |
| `/api/v3/weights` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Scoring engine |
| `/api/v3/allmetrics` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Metrics export |
| `/api/v3/context` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Context metadata |
| `/api/v3/contexts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Multi-node contexts |
| `/api/v3/q` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` | Full-text search |
| `/api/v3/alerts` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Multi-node alerts |
| `/api/v3/alert_transitions` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Alert history |
| `/api/v3/alert_config` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Alert configuration |
| `/api/v3/variable` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` | Chart variables |
| `/api/v3/nodes` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` | Nodes listing |
| `/api/v3/node_instances` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` | Node instances |
| `/api/v3/stream_path` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` | Streaming topology |
| `/api/v3/function` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` | Execute function (permissions checked per-function) |
| `/api/v3/functions` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` | List functions |
| `/api/v3/config` | `HTTP_ACL_DYNCFG` | `ANONYMOUS_DATA` | Dynamic configuration (read/write permissions checked per-action) |
| `/api/v3/settings` | `HTTP_ACL_DASHBOARD` | `ANONYMOUS_DATA` | User settings (GET/PUT) |

### Per-route ACL bypass entries

These use `HTTP_ACL_NOCHECK`. Their independent access requirements remain in
force: `versions` and `progress` are bearer-protected because they require
`HTTP_ACCESS_ANONYMOUS_DATA`; the other four require no route-table access bit.

| API | ACL | Access | Description |
|-----|-----|--------|-------------|
| `/api/v3/info` | `HTTP_ACL_NOCHECK` | `NONE` | Agent information |
| `/api/v3/versions` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` | Version information |
| `/api/v3/progress` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` | Function progress tracking |
| `/api/v3/stream_info` | `HTTP_ACL_NOCHECK` | `NONE` | Streaming statistics |
| `/api/v3/claim` | `HTTP_ACL_NOCHECK` | `NONE` | Claim status; mutation requires current session key |
| `/api/v3/me` | `HTTP_ACL_NOCHECK` | `NONE` | Current user info |

### ACLK-Only APIs (Netdata Cloud Access Required)

These require `HTTP_ACL_ACLK` - **ONLY accessible via Netdata Cloud (ACLK)**:

| API | ACL | Access | Requirements | Description |
|-----|-----|--------|--------------|-------------|
| `/api/v3/rtc_offer` | `HTTP_ACL_ACLK \| ACL_DEV_OPEN_ACCESS` | `SIGNED_ID` + `SAME_SPACE` | Exact access mask shown | WebRTC connection establishment |
| `/api/v3/bearer_protection` | `HTTP_ACL_ACLK \| ACL_DEV_OPEN_ACCESS` | `SIGNED_ID` + `SAME_SPACE` + `VIEW_AGENT_CONFIG` + `EDIT_AGENT_CONFIG` | Exact access mask shown | Enable/disable bearer protection |
| `/api/v3/bearer_get_token` | `HTTP_ACL_ACLK \| ACL_DEV_OPEN_ACCESS` | `SIGNED_ID` + `SAME_SPACE` | Exact access mask shown | Generate bearer token |

**Note:** `ACL_DEV_OPEN_ACCESS` expands to `HTTP_ACL_NOCHECK` only in
`NETDATA_DEV_MODE`; it is zero in production builds.

---

## V2 APIs (17 total) - DEPRECATED

### V2 anonymous-data APIs

| API | ACL | Access |
|-----|-----|--------|
| `/api/v2/data` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/weights` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/contexts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/q` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v2/alerts` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v2/alert_transitions` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v2/alert_config` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v2/nodes` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v2/node_instances` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v2/versions` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v2/functions` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` |

### V2 per-route ACL bypass entries

| API | ACL | Access |
|-----|-----|--------|
| `/api/v2/info` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` |
| `/api/v2/progress` | `HTTP_ACL_NOCHECK` | `ANONYMOUS_DATA` |
| `/api/v2/claim` | `HTTP_ACL_NOCHECK` | `NONE` |

### ACLK-Only APIs

| API | ACL | Access |
|-----|-----|--------|
| `/api/v2/rtc_offer` | `HTTP_ACL_ACLK \| ACL_DEV_OPEN_ACCESS` | `SIGNED_ID` + `SAME_SPACE` |
| `/api/v2/bearer_protection` | `HTTP_ACL_ACLK \| ACL_DEV_OPEN_ACCESS` | `SIGNED_ID` + `SAME_SPACE` + `VIEW_AGENT_CONFIG` + `EDIT_AGENT_CONFIG` |
| `/api/v2/bearer_get_token` | `HTTP_ACL_ACLK \| ACL_DEV_OPEN_ACCESS` | `SIGNED_ID` + `SAME_SPACE` |

---

## V1 APIs (24 total) - DEPRECATED

### V1 anonymous-data APIs

| API | ACL | Access |
|-----|-----|--------|
| `/api/v1/data` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/weights` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/metric_correlations` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/badge.svg` | `HTTP_ACL_BADGES` | `ANONYMOUS_DATA` |
| `/api/v1/allmetrics` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/chart` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/charts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/context` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/contexts` | `HTTP_ACL_METRICS` | `ANONYMOUS_DATA` |
| `/api/v1/function` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` |
| `/api/v1/functions` | `HTTP_ACL_FUNCTIONS` | `ANONYMOUS_DATA` |
| `/api/v1/config` | `HTTP_ACL_DYNCFG` | `ANONYMOUS_DATA` |

### Alert APIs

| API | ACL | Access |
|-----|-----|--------|
| `/api/v1/alarms` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarms_values` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarm_log` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarm_variables` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/variable` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |
| `/api/v1/alarm_count` | `HTTP_ACL_ALERTS` | `ANONYMOUS_DATA` |

### Node Info APIs

| API | ACL | Access |
|-----|-----|--------|
| `/api/v1/info` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v1/aclk` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v1/dbengine_stats` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |
| `/api/v1/ml_info` | `HTTP_ACL_NODES` | `ANONYMOUS_DATA` |

### Special APIs

| API | ACL | Access | Notes |
|-----|-----|--------|-------|
| `/api/v1/registry` | `HTTP_ACL_NONE` | `NONE` | Callback requires dashboard ACL for `hello`, registry ACL for every other action |
| `/api/v1/manage` | `HTTP_ACL_MANAGEMENT` | `NONE` | Only `manage/health` is accepted; its callback requires the management API key in `X-Auth-Token` |

---

## Independent Accounting

ACL and access counts are independent dimensions; `NOCHECK` overlaps the
access columns. The ACL count is for a production build, where
`ACL_DEV_OPEN_ACCESS` is zero.

| Version | Routes | `NOCHECK` ACL | `ANONYMOUS_DATA` access | `NONE` access | Signed ACLK access |
|---------|-------:|--------------:|------------------------:|--------------:|-------------------:|
| v3 | 27 | 6 | 20 | 4 | 3 |
| v2 | 17 | 3 | 13 | 1 | 3 |
| v1 | 24 | 0 | 22 | 2 | 0 |
| **Total** | **68** | **9** | **55** | **7** | **6** |

The nine `NOCHECK` routes split as follows:

| Route | Access | Bearer effect | Additional local check |
|-------|--------|---------------|------------------------|
| `/api/v3/info` | `NONE` | None from the generic access gate | None |
| `/api/v3/versions` | `ANONYMOUS_DATA` | Unauthenticated request denied when enabled | None |
| `/api/v3/progress` | `ANONYMOUS_DATA` | Unauthenticated request denied when enabled | None |
| `/api/v3/stream_info` | `NONE` | None from the generic access gate | None |
| `/api/v3/claim` | `NONE` | None from the generic access gate | Rotating session key before a claim mutation |
| `/api/v3/me` | `NONE` | None from the generic access gate | None |
| `/api/v2/info` | `ANONYMOUS_DATA` | Unauthenticated request denied when enabled | None |
| `/api/v2/progress` | `ANONYMOUS_DATA` | Unauthenticated request denied when enabled | None |
| `/api/v2/claim` | `NONE` | None from the generic access gate | Rotating session key before a claim mutation |

`NOCHECK` is therefore not an "always public" classification. Five entries
require no generic access bit, while four are protected by bearer mode.

## Permission Checking Flow

### Entry paths

- Direct HTTP enters through `web_client_process_url()`. The global
  `allow connections from` check, socket/port ACL construction, and the coarse
  web-feature admission check occur before the API version dispatcher.
- ACLK requests enter through `http_api_v2()` and
  `web_client_api_request_with_node_selection()` with ACLK-derived ACL and
  Cloud authentication headers.
- WebRTC requests enter through `webrtc_execute_api_request()` and the same
  node-selection dispatcher with WebRTC ACLs. WebRTC is an optional build
  feature and is disabled by default.
- `/host/<id>/api/...` and `/node/<id>/api/...` select a host and re-enter the
  same API dispatcher. They do not add route registrations or skip route
  authorization.

### Route dispatch order

For v1, v2, and v3, `web_client_api_request_vX()` applies this order:

1. `web_client_ensure_proper_authorization()` initializes an unauthenticated
   client's access to `ANONYMOUS_DATA` when bearer protection is off, or to
   `NONE` when bearer protection is on. Valid Cloud/bearer permissions have
   already been attached to the request.
2. The route ACL is checked. `HTTP_ACL_NOCHECK` makes only this predicate pass.
3. The request access mask must contain every bit required by the route. This
   is a bitmask containment check, not a role comparison.
4. The callback runs and may impose further checks.

The callback-local exceptions relevant to this inventory are:

- v1 `registry`: `hello` requires dashboard ACL; all other actions require
  registry ACL.
- v1 `manage`: only `manage/health` is routed. It requires management ACL at
  the table and the management API key in `X-Auth-Token` in the health
  callback.
- v2/v3 `claim`: the information response needs no local key, but a claim
  mutation requires the current rotating session key and rotates it on use or
  failure.
- v1/v3 `function` and `config`: the caller's access mask is passed to the
  function framework, where registered functions can require additional
  access bits.

## Configuration Impact

For direct HTTP, the relevant source configuration is:

- `[web] allow connections from`: global connection admission.
- `[web] allow dashboard from`: grants the dashboard feature group
  (`METRICS`, `FUNCTIONS`, `ALERTS`, `NODES`, and `DYNCFG`).
- `[web] allow badges from`: grants `BADGES`.
- `[web] allow management from`: grants `MANAGEMENT`.
- `[registry] allow from`: grants `REGISTRY`.
- Port ACLs further intersect the feature ACLs granted to the client.

The direct HTTP coarse admission check requires at least one recognized web
feature before API dispatch. A `NOCHECK` route skips its own route-table feature
comparison after that point; it does not bypass global connection admission,
port ACLs, or coarse admission. ACLK and WebRTC construct their ACLs separately
and do not use the direct-client IP allowlist path.

### Bearer protection

Bearer protection affects all 55 routes requiring
`HTTP_ACCESS_ANONYMOUS_DATA`, including four of the nine `NOCHECK` routes. An
unauthenticated direct or WebRTC request has no access bits while protection is
enabled and fails those routes. A valid bearer token supplies its stored role
and access mask; authorization still depends on the mask.

The seven `HTTP_ACCESS_NONE` routes do not acquire a generic bearer requirement.
The six signed ACLK registrations require their exact signed/same-space/config
access masks and, in production builds, the ACLK route ACL.

## Guidance for Endpoint Documentation

Each endpoint description should state independent facts rather than assign one
security label:

1. Route ACL, including the production/dev meaning of `ACL_DEV_OPEN_ACCESS`.
2. Required `HTTP_ACCESS` bits and the resulting bearer-mode effect.
3. Entry-path restrictions and direct HTTP allowlist mapping, where applicable.
4. Callback-local checks for the selected action or function.

For `HTTP_ACL_NOCHECK`, use wording equivalent to:

> This route skips the per-route ACL comparison. Connection/transport/coarse
> admission, the listed HTTP access requirement, bearer-derived access, and
> callback-local checks still apply.

Do not describe `NOCHECK` routes as bypassing all security, immune to bearer
protection, outside all IP admission, or impossible to restrict.

**Last verified:** 2026-07-16
