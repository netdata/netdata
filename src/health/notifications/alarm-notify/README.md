# Experimental Go notifier

This standalone Go module routes a JSON notification to named webhook destinations. It has no imports from
the existing `src/go` module. It is for local development and is not installed, packaged, or invoked by the Agent.
The active notifier remains `../alarm-notify.sh.in` and its shell configuration.

The current increments provide explicit delivery and role-based routing. [CAPABILITIES.md](CAPABILITIES.md) tracks
the remaining Bash functionality. Configuration and code may change substantially before production adoption;
final redesign follows the working functional baseline.

## Build and run

From this directory, using the Go version in `go.mod`:

```sh
go build -o /tmp/alarm-notify .
/tmp/alarm-notify validate --config examples/notify.yaml
/tmp/alarm-notify send --config examples/notify.yaml --destination local < examples/event.json
/tmp/alarm-notify send --config examples/notify.yaml --role sysadmin --role dba < examples/event.json
```

The send command needs a receiver. For a local demonstration, run this in a separate terminal and stop it with Ctrl-C:

```sh
python3 - <<'PY'
from http.server import BaseHTTPRequestHandler, HTTPServer

class Receiver(BaseHTTPRequestHandler):
    def do_POST(self):
        print(self.rfile.read(int(self.headers["Content-Length"])).decode(), flush=True)
        self.send_response(204)
        self.end_headers()

HTTPServer(("127.0.0.1", 18080), Receiver).serve_forever()
PY
```

The root CMake build also provides an opt-in target. Starting from the repository root, add the option to a normal
developer configuration, then build only that target:

```sh
cmake -S . -B build -DENABLE_ALARM_NOTIFY_GO=ON
cmake --build build --target alarm-notify-go
```

The result is `build/alarm-notify` (`alarm-notify.exe` on Windows). `ENABLE_ALARM_NOTIFY_GO` defaults to `OFF`,
independently of `DEFAULT_FEATURE_STATE`; even with it enabled there is no notifier install rule. CMake requires the
normal Agent configuration dependencies. Direct Go builds require only this module and its dependencies.

## Command and configuration contract

- `validate --config FILE` checks one YAML document. Unknown fields, duplicate keys, unsupported provider types, and
  invalid literal settings fail. It does not resolve secrets, read event input, or make network requests.
- `send --config FILE --destination NAME` reads exactly one JSON event from stdin and delivers it to that named
  destination. Unknown destinations fail. Other configured destinations are validated but not resolved or contacted.
- `send --config FILE --role ROLE [--role ROLE ...]` uses YAML routing to select destinations. Choose either an explicit
  destination or roles; mixing the selectors fails. Roles are exact, case-sensitive names, without comma splitting,
  wildcards, or implicit role selection. They do not become fields in the webhook payload.
- Both commands accept a positive `--timeout` (default `10s`) covering input/configuration reads and delivery.
  Deadline expiration, Ctrl-C, or SIGTERM stops the invocation. Run this developer tool as an ordinary user.
- Exit status is `0` when at least one delivery succeeds, no destinations are selected, validation succeeds, or help
  is requested. Invalid options/configuration/input, all selected deliveries failing, or command cancellation/timeout
  return `1`. Successful validation writes a confirmation to stdout. Sending leaves stdout empty and logs to stderr.

Configuration has `version: 1` and a `destinations` mapping. Each destination requires `type: webhook` and `url`;
`bearer_token` is optional. Destination names are nonsecret identifiers. The URL must be an absolute HTTP or HTTPS
URL with a host and without embedded user/password information or a fragment. HTTP allows deliberate local or
self-hosted delivery; HTTPS verifies certificates. Proxy selection follows Go's HTTP_PROXY/HTTPS_PROXY/NO_PROXY rules.

The URL and bearer token accept literal strings or a whole `${env:VARIABLE}` or `${file:/absolute/path}` reference.
On Windows the file operand must be an absolute Windows path. File reads use native Go I/O under the invoking user's
identity. Resolved values have surrounding whitespace trimmed and must be nonempty. Interpolation, command execution,
and secret-store references are unsupported. Examples prefer environment references so credentials stay outside YAML.
`validate` checks reference syntax only; `send` resolves and validates the selected destination before sending.

## Routing and delivery results

The optional `routing` section maps roles to destination names:

```yaml
routing:
  default: [local]
  roles:
    sysadmin: [local, audit]
    dba: [audit]
    muted: []
```

Each name must exist in `destinations`. A missing role uses `default`; an explicit empty list suppresses delivery for
that role. Null role entries are rejected so an accidental missing value does not suppress notifications. The
reserved roles `silent` and `disabled` select nothing and cannot have configured routes. They do not suppress other
roles in the same invocation. With neither a matching role nor defaults, sending is a successful no-op.

The selected destinations are the union across roles, preserving input-role and configured-list order. Each
destination name is sent once. Different names remain distinct destinations even if they use the same URL. In the
example, `--role sysadmin --role dba` sends to `local` and `audit` once each; an unknown role selects `local`.

All configuration and event structure is validated before delivery. Selected destinations are then attempted
sequentially; a secret-resolution, HTTP, or transport error does not stop the next destination. The total timeout
still covers the whole invocation, so a slow destination can exhaust the remaining time. Cancellation stops the
invocation even after earlier successful deliveries; no further destinations are started once it is observed.

Each completed delivery reports its quoted destination name and outcome to stderr, followed by a success/failure
count. Partial failure returns `0` when another delivery succeeded, matching Bash's any-success behavior; inspect
the individual results to see failures. On interruption, counts cover results reported before cancellation and do
not claim an outcome for interrupted or unstarted deliveries.

Each delivery sends one POST with `Content-Type: application/json` and, when configured, `Authorization: Bearer ...`.
HTTP 200–299 acknowledges delivery. Redirects and other status codes fail; there are no application retries.
Response bodies are closed without being buffered, logged, or interpreted. Errors do not echo config/input values,
secret contents, or endpoint URLs.

## Event document

The webhook receives the typed event as JSON. `version` must be `1`. Required fields are `incident_id`, `timestamp`
(RFC 3339), `node`, `alert`, `status`, and `summary`. `incident_id` is an opaque stable incident identifier supplied by
the caller. Current statuses are `WARNING`, `CRITICAL`, and `CLEAR`.

Optional fields are `chart`, `context`, `previous_status`, `info`, `value`, `previous_value`, and `units`.
`previous_status` accepts the three current statuses plus `UNINITIALIZED`, `UNDEFINED`, and `REMOVED`.
Values are finite JSON numbers or null; missing values are sent as null and zero remains zero. Unknown fields and
trailing documents are rejected. Strings are encoded as JSON, including quotes, newlines, and Unicode.

This increment does not infer initial-CLEAR eligibility or apply severity filters or critical-history policy. Those
capabilities remain pending in the inventory. Add future internal-only event facts separately from this public
webhook document.

## Validation

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

Tests use tables keyed by case name and local HTTP receivers; no provider account or credentials are needed.
CI runs this module's tests on Linux. To check Windows compilation locally, use:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/alarm-notify.exe .
```

A Windows cross-build checks compilation, not native Windows runtime behavior.
