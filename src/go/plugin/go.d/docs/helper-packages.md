# Go Helper Packages For go.d Collectors

Use existing helper packages before adding collector-local plumbing. A helper is not better because it is shared; it is
better when it gives users the same configuration shape, the same safety behavior, or the same testable parsing path as
other collectors.

This guide covers helper surfaces used by go.d collectors across:

- `src/go/pkg/*` for shared Go packages used beyond go.d;
- `src/go/plugin/go.d/pkg/*` for go.d-specific helpers;
- `src/go/logger` for the logger embedded through `collectorapi.Base`.

It is not an exhaustive API reference. Before adding a local helper, search these roots for an existing package that
already owns the behavior.

## Helper Roots

| Need | Start with |
|---|---|
| V2 metrics, metric stores, host scopes | `src/go/pkg/metrix` |
| Duration and tri-state config option types | `src/go/pkg/confopt` |
| HTTP request/client config | `src/go/pkg/web` |
| TLS config outside HTTP | `src/go/pkg/tlscfg` |
| Prometheus exposition parsing | `src/go/pkg/prometheus` |
| User selector/matcher grammar | `src/go/pkg/matcher` |
| Collector logging and log limiting | `src/go/logger` |
| Function request/response helpers | `src/go/pkg/funcapi` |
| Topology payloads | `src/go/pkg/topology/v1` |
| Agent API / chart emission payloads | `src/go/pkg/netdataapi` |
| TCP/UDP/Unix line-protocol clients | `src/go/plugin/go.d/pkg/socket` |
| Command execution | `src/go/plugin/go.d/pkg/ndexec` |
| Log-file readers/parsers | `src/go/plugin/go.d/pkg/logs` |
| IP range parsing | `src/go/plugin/go.d/pkg/iprange` |
| SQL query/scan helpers | `src/go/plugin/go.d/pkg/sqlquery` |
| Cloud auth config/credentials | `src/go/plugin/go.d/pkg/cloudauth` |
| Profile-catalog loading (YAML profiles, stock/user dirs) | `src/go/plugin/go.d/pkg/profilecatalog` |
| Ping probing | `src/go/plugin/go.d/pkg/pinger` |
| SNMP utilities | `src/go/plugin/go.d/pkg/snmputils` |
| Kubernetes client helpers | `src/go/plugin/go.d/pkg/k8sclient` |
| Docker host helpers | `src/go/plugin/go.d/pkg/dockerhost` |
| Test helpers for collectors | `src/go/plugin/go.d/pkg/collecttest` |
| Legacy V1 metric helpers | `src/go/pkg/stm`, `src/go/plugin/go.d/pkg/oldmetrix` |

## Config Option Types

Use `src/go/pkg/confopt` for common configuration value types.

When:

- users configure durations that should accept strings such as `5s`, `30m`, or numeric seconds;
- users need explicit `auto` / `enabled` / `disabled` behavior instead of a plain boolean;
- a migration needs to preserve legacy pointer-boolean semantics without keeping pointer plumbing in new code.

Why:

- `confopt.Duration` and `confopt.LongDuration` centralize YAML/JSON duration parsing and formatting;
- `confopt.AutoBool` makes tri-state behavior explicit and schema-friendly;
- collectors avoid ad hoc parsers and inconsistent boolean defaults.

## HTTP Collectors

Use `src/go/pkg/web` for HTTP-based collectors.

When:

- the collector talks to an HTTP or HTTPS endpoint;
- users need the normal Netdata HTTP options: `url`, timeout, redirects, proxy, basic auth, bearer token file, headers,
  body, method, and TLS fields;
- the collector builds repeated requests against the same endpoint.

Why:

- `web.HTTPConfig` embeds `web.RequestConfig` and `web.ClientConfig` so HTTP collectors expose the same option surface;
- `web.NewHTTPClient(c.ClientConfig)` applies timeout, TLS, proxy, redirect, and HTTP/2 behavior consistently;
- `web.NewHTTPRequest(c.RequestConfig)` and `web.NewHTTPRequestWithPath(c.RequestConfig, path)` apply user agent,
  authentication, headers, body, and safe path joining.

Pattern:

```go
type Config struct {
    web.HTTPConfig `yaml:",inline" json:""`
}
```

Use `src/go/pkg/tlscfg` directly only when the collector is not HTTP-based but still needs TLS, such as Redis or
x509-style checks. HTTP collectors should get TLS behavior through `web.HTTPConfig`.

## Prometheus Endpoints

Use `src/go/pkg/prometheus` when the upstream endpoint exposes Prometheus text format.

When:

- the collector scrapes `/metrics` or another Prometheus exposition endpoint;
- the collector needs to parse metric families or sorted series;
- the collector needs a bounded selector for metric names.

Why:

- it reuses `web.RequestConfig` and `*http.Client`;
- it handles Prometheus text parsing and gzip responses;
- selectors avoid parsing or processing metric families the collector will not use.

Do not hand-roll text exposition parsing in a collector.

## Selectors And Matchers

Use `src/go/pkg/matcher` for user-facing include/exclude or selector fields.

When:

- users select entities by name, ID, interface, queue, topic, or similar labels;
- the selector syntax can be glob, regexp, string, or simple patterns;
- negative matches such as `!*test* *` are sufficient.

Why:

- users get one matcher grammar across collectors;
- tests can cover selector behavior without custom parser logic;
- existing logical matchers can combine conditions when needed.

Do not invent a selector language unless the upstream API requires one. Prefer a single simple-pattern field for simple
cases; add separate include/exclude fields only when the user problem needs that shape.

Do not use `src/go/pkg/selectorcore` for user-facing collector selectors. It is the lower-level selector metadata/parser
surface used by template and selector engines, not the normal collector selector helper.

## Limited Logging

Collectors embed `collectorapi.Base`, which embeds `*logger.Logger`. Use the logger's built-in limiting before adding
collector-local rate-limit state.

When:

- an error can repeat every collection cycle;
- a partial failure is useful to report but would spam logs;
- a one-time notice or warning is enough.

Why:

- in go.d jobs, `c.Once(key).Warningf(...)` is cycle-local because the runtime resets `Once` state each `runOnce`; it is
  useful for suppressing duplicate messages inside one cycle only;
- `c.Limit(key, n, window).Warningf(...)` logs at most `n` messages per key per window and is the right default for
  cross-cycle spam control;
- the limiter is shared through the collector logger and already used by modern collectors such as Cato Networks,
  PAN-OS, and vSphere.

Pattern:

```go
c.Limit("mycollector:operation:error", 1, time.Hour).
    Warningf("operation failed: %v", err)
```

Use stable keys. Include the operation and bounded error class when needed, but do not put unbounded IDs, URLs, query
strings, customer names, or raw provider messages in the key.

Custom warning gates are justified only when the built-in count-per-window semantics are not the right behavior, for
example when logging only on state transitions. Document that reason in the PR description or design note so reviewers
can see why the built-in limiter was not enough.

## Socket Clients

Use `src/go/plugin/go.d/pkg/socket` for simple TCP, UDP, or Unix-socket line-protocol collectors.

When:

- the collector connects to a local or remote socket and sends text commands;
- the response is processed line by line;
- the collector needs shared timeout, TLS, and max-read-line behavior.

Why:

- socket address parsing is shared across collectors;
- connect, command, read, disconnect, deadline, and line-limit behavior stay consistent;
- tests can use the helper's fake TCP/UDP/Unix servers instead of custom socket harnesses.

Do not hand-roll socket dial/read loops for common line-oriented protocols.

## External Commands

Use `src/go/plugin/go.d/pkg/ndexec` for collectors that execute binaries.

When:

- the collector needs a local command output;
- the command should run through Netdata's helper wrappers;
- the command may need privilege through `ndsudo`;
- tests need to stub helper paths.

Why:

- arguments are passed without a shell;
- timeouts and context cancellation are handled;
- stderr snippets are bounded;
- helpers integrate with Netdata's execution model.

Use:

- `RunUnprivileged` / `RunUnprivilegedWithOptions...` for unprivileged commands;
- `RunNDSudo` for commands exposed through `ndsudo`;
- `RunDirect` only when direct execution is intentionally required;
- `FindBinary` for PATH/default-path discovery.

Do not call `exec.Command` directly unless the helper cannot support the case and the reason is documented.

## Log File Collectors

Use `src/go/plugin/go.d/pkg/logs` for collectors that parse application log files.

When:

- the collector tails files that can rotate;
- the log format is CSV, LTSV, regexp, or JSON;
- parser errors should be distinguishable from I/O errors.

Why:

- `logs.Reader` is log-rotation aware;
- `logs.NewParser` centralizes supported parser types;
- `logs.IsParseError` lets collection logic treat malformed rows differently from source failures.

Do not open and seek log files manually unless the collector's source is not a normal file-tail workflow.

## IP Ranges

Use `src/go/plugin/go.d/pkg/iprange` when users configure address ranges.

When:

- the collector filters IPs, networks, peers, or hosts by ranges;
- the config accepts CIDR, range, or other supported range syntax.

Why:

- range parsing and membership checks are shared;
- invalid syntax handling is consistent;
- collectors avoid slightly different IP matching semantics.

## SQL Helpers

Use `src/go/plugin/go.d/pkg/sqlquery` for repeated SQL row-scanning patterns.

When:

- the collector or Function scans rows into strings, integers, floats, or discard columns;
- the collector needs table-column discovery with `?` or `$1` placeholders;
- the row-to-value assignment is generic across queries.

Why:

- scan holders and null handling are centralized;
- query duration measurement and row iteration behavior stay testable;
- Function code can avoid custom one-off scanners.

## Cloud Auth Helpers

Use `src/go/plugin/go.d/pkg/cloudauth` when a cloud collector needs supported cloud-provider credentials.

When:

- the collector supports `cloud_auth` configuration;
- Azure AD credential construction is needed.

Why:

- provider names normalize consistently;
- validation is centralized;
- unsupported providers fail with consistent errors.

## Ping Helpers

Use `src/go/plugin/go.d/pkg/pinger` for ping/latency probing.

When:

- a collector needs ICMP-style probing;
- it needs shared latency/jitter derivation.

Why:

- probe config validation and derived metrics are shared;
- collectors avoid reimplementing packet sampling and jitter math.

## Profile Catalog Helpers

Use `src/go/plugin/go.d/pkg/profilecatalog` when a collector ships curated per-target "profiles" as YAML files (a
profile's identity is its file basename) and loads them from stock plus user directories. Used by the `prometheus`,
`azure_monitor`, and `cloudwatch` collectors.

When:

- the collector reads profiles from `config/go.d/<name>.profiles/` (stock) and the user config dirs;
- it needs stock/user override precedence (user overrides stock by basename), the stock-fatal / user-skip error policy,
  and a process-wide cached catalog.

Why:

- one shared `Load[P]` + `Catalog[P]` + `Cached[T]` replaces per-collector copies of the directory walk, override
  precedence, and singleton caching;
- it is generic over the collector's profile type `P` and oblivious to matching (matching stays in the collector);
- decode depth is the collector's choice via `Options.Decode`: parse everything eagerly, or parse a lightweight header
  now and hydrate the heavy part later (as `prometheus` does for its chart templates).

Do NOT put matching logic in this package; it is a catalog + loader, not a matcher. Keep the profile schema, its
decode/validate, the `defaultDirSpecs` directory resolution (location-specific), and specialized queries in the
collector's own `*profiles` package, wrapping `profilecatalog.Catalog[P]` by struct embedding.

## Legacy V1 Helpers

`src/go/pkg/stm` converts structs into `map[string]int64`. `src/go/plugin/go.d/pkg/oldmetrix` provides V1 metric vector
helper types such as counters, summaries, histograms, and boolean conversions used by existing V1 collectors. Both
helpers are V1-shaped. New V2 collectors MUST NOT use them as their metric path.

Acceptable uses:

- maintaining an existing V1 collector;
- temporary parity tests during V1-to-V2 migration, provided the helper is not reachable from the final runtime path.
