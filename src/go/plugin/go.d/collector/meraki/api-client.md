# Meraki discovery and availability client

`merakiapi` is a collector-local Go client for four read-only Dashboard API v1 operations. This initial delivery
does not register a collector or publish metrics, charts, configuration forms or Functions.

## Supported operations

| Method | GET path | Initial page size | Server page-size range |
|---|---|---|---|
| `ListOrganizations` | `/organizations` | 9000 | 3–9000 |
| `ListNetworks` | `/organizations/{organizationId}/networks` | 1000 | 3–100000 |
| `ListDevices` | `/organizations/{organizationId}/devices` | 1000 | 3–5000 |
| `ListDeviceAvailabilities` | `/organizations/{organizationId}/devices/availabilities` | 1000 | 3–1000 |

All four operations return bare arrays. There are no initial entity filters or generic request/query escape hatches.
`ListDevices` reads devices assigned to networks, rather than unused organization inventory.

The response types are minimal projections of identity, names, tags and the selected device metadata. Physical
addresses, coordinates, notes, IMEI, enrollment strings and management/customer details are discarded.

IDs remain opaque strings. Missing string fields remain empty; missing/null lists remain nil. The client does not
interpret a missing ID, unknown product type or unknown availability state. Consumers MUST validate the identity
and meaning needed for their own use; the provider does not declare these response properties required.

## Ownership and authentication

Construct a standard `*http.Client` using `web.NewHTTPClient` and pass it to `merakiapi.New`. The caller owns TLS,
proxy, request timeout, transport lifetime and idle-connection cleanup. Client construction performs no network I/O.
It copies the HTTP client and installs the Meraki redirect policy without mutating the supplied client.

Construction accepts an HTTPS base URL ending in `/api/v1`, and exactly one inline API key or bearer-token file.
The default is `https://api.meraki.com/api/v1`; the documented regional bases are:

- `https://api.meraki.ca/api/v1`
- `https://api.meraki.cn/api/v1`
- `https://api.meraki.in/api/v1`
- `https://api.gov-meraki.com/api/v1`

Every page and accepted redirect builds a new authenticated request through `web.NewHTTPRequest`, using the current
operation context. Bearer files are re-read through the shared credential authority boundary on every construction.
There is no credential cache or fallback to unauthenticated requests.

Methods are safe for concurrent calls, subject to the supplied transport's normal concurrency contract. The client
does not coordinate organization budgets between calls, jobs, processes or other applications.

## Complete traversal and decoding

Methods return a complete list or nil plus an error. A failed later page MUST NOT become an apparently complete
inventory. A successful traversal is not an atomic snapshot: the provider may change data between pages.

- Follow `rel=next` until absent, including empty intermediate pages.
- Read all physical `Link` headers and respect commas inside targets and quoted parameters.
- Follow the first `rel` parameter when duplicated, as RFC5988 requires; ignore extension parameters that do not
  affect traversal. A next link with an unsupported `anchor` context fails rather than implying complete inventory.
- Resolve relative references against the final response URL, including a validated redirect.
- Preserve server-owned cursor values. Do not generate a cursor from the final record or infer completion from size.
- Accept only the same operation/organization path and documented pagination parameters.
- Reject malformed or conflicting links, invalid page sizes and repeated canonical page targets.
- Require exactly one non-null JSON array containing non-null objects, followed by whitespace and EOF.
- Preserve the supplied HTTP client's response limit, including transports that advertise it without enforcing it.
  Standard HTTP gzip decompression is supported; custom transports must decompress before returning a response.
  Other unprocessed content encodings fail explicitly.

No fixed page/record ceiling is imposed. The caller SHOULD provide a whole-operation deadline appropriate to its
fleet, in addition to request timeouts. Cancellation reaches in-flight HTTP requests and credential construction.

## Authenticated redirect policy

Cisco documents GET redirects and requires authentication on the destination. Go's default redirect behavior can
strip Authorization between the API and sibling shard hosts. The client validates each destination before building
fresh authentication and retains Go's ten-hop limit.

Targets MUST preserve the exact operation path and query meaning and use HTTPS without user information or a fragment.
The configured origin is trusted. For an official regional API base, additional destinations are HTTPS/443 API
hosts and numeric shard forms `n<digits>.<regional-root>` and `n<digits>.dashboard.<regional-root>` in that same region.
Cross-region, lookalike and other hosts are rejected. An explicitly configured nonstandard origin trusts only itself.
Pagination uses the same destination trust boundary. Separate operations always start at the configured base.

The numeric shard grammar is a conservative inference from Cisco's published examples, not an exhaustive vendor
catalogue or live regional verification. Unknown forms produce an error and require evidence before extending trust.

## Errors and rate limits

`HTTPError` exposes the operation, HTTP status and parsed `Retry-After` information. Both delta-seconds and HTTP dates
are supported; absent, invalid or overflowing delays remain unknown. There are no automatic retries, cooldowns or
background loops. The consumer owns retry timing and total request budget.

The documented budgets are shared: 10 requests/second per organization and 100 requests/second per source IP.
Pagination and redirects also issue requests. A client-local policy cannot guarantee those shared budgets.

Errors MUST NOT include raw response bodies, complete URLs, Link/Location contents, credentials or unsafe nested
transport/parser/credential errors. Known safe cancellation, deadline and `web.ErrResponseTooLarge` causes remain
available through `errors.Is`. HTTP statuses are not interpreted as removal or capability detection: Cisco documents
authentication-related 404 behavior, and successful organization listing does not prove operational organization
access because that operation is exempt from organization IP restrictions.

## Availability semantics

The provider refreshes availability every **five minutes**. The response has no observation timestamp; request
completion time MUST NOT be presented as the physical sample time. Documented states are `online`, `alerting`,
`offline` and `dormant`. Unknown/missing strings remain distinguishable from those states and are not normalized.

## Evidence and validation boundary

Source contract: `meraki/openapi @ 23cd4b4ea6d06803903d41795acc717047695b4e`, `openapi/spec3.json`, API v1.75.1,
dated 2026-10-16. Fixtures under `merakiapi/testdata/` are reduced synthetic examples shaped by those operation
schemas; they contain no real organization or device captures.

Tests exercise real client construction, TLS HTTP requests, pagination, standard gzip, caller-advertised body limits,
trusted host routing, cancellation, status/retry metadata, and safe diagnostics. Credential-file tests use the shipped
web/credentialfile path with a Go subprocess protocol peer; the shared credentialfile suite owns actual C privilege
transition tests. No live Meraki credentials, fleet-scale runtime or regional device coverage is claimed.

Primary sources:

- [Stable OpenAPI specification](https://github.com/meraki/openapi/blob/23cd4b4ea6d06803903d41795acc717047695b4e/openapi/spec3.json)
- [Getting started and regional URLs](https://developer.cisco.com/meraki/api-v1/getting-started/)
- [Authentication](https://developer.cisco.com/meraki/api-v1/authorization/)
- [Pagination](https://developer.cisco.com/meraki/api-v1/pagination/)
- [Web Linking framing and context](https://www.rfc-editor.org/rfc/rfc5988#section-5)
- [Rate limits](https://developer.cisco.com/meraki/api-v1/rate-limit/)
- [Availability](https://developer.cisco.com/meraki/api-v1/get-organization-devices-availabilities/)
- [Status, redirects and access restrictions](https://documentation.meraki.com/Platform_Management/Dashboard_Administration/Operate_and_Maintain/How-Tos/How_to_Use_the_Cisco_Meraki_Dashboard_API)
- [API base and shard URL guidance](https://documentation.meraki.com/Platform_Management/Dashboard_Administration/Troubleshooting_and_Support/Troubleshooting/Meraki_Dashboard_URL_Update_-_PCI_Compliance)
