# Experimental Go notifier

This standalone Go module sends a JSON notification to one explicitly selected webhook. It has no imports from
the existing `src/go` module. It is for local development and is not installed, packaged, or invoked by the Agent.
The active notifier remains `../alarm-notify.sh.in` and its shell configuration.

The first increment establishes a working delivery path. [CAPABILITIES.md](CAPABILITIES.md) tracks the remaining Bash
functionality. Configuration and code may change substantially before production adoption; final redesign follows
the working functional baseline.

## Build and run

From this directory, using the Go version in `go.mod`:

```sh
go build -o /tmp/alarm-notify .
/tmp/alarm-notify validate --config examples/notify.yaml
/tmp/alarm-notify send --config examples/notify.yaml --destination local < examples/event.json
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
- Both commands accept a positive `--timeout` (default `10s`) covering input/configuration reads and delivery.
  Deadline expiration, Ctrl-C, or SIGTERM stops the invocation. Run this developer tool as an ordinary user.
- Exit status is `0` for success/help and `1` for invalid options, configuration, input, cancellation, or delivery
  failure. Successful validation writes a confirmation to stdout. Sending leaves stdout empty and logs to stderr.

Configuration has `version: 1` and a `destinations` mapping. Each destination requires `type: webhook` and `url`;
`bearer_token` is optional. Destination names are nonsecret identifiers. The URL must be an absolute HTTP or HTTPS
URL with a host and without embedded user/password information or a fragment. HTTP allows deliberate local or
self-hosted delivery; HTTPS verifies certificates. Proxy selection follows Go's HTTP_PROXY/HTTPS_PROXY/NO_PROXY rules.

The URL and bearer token accept literal strings or a whole `${env:VARIABLE}` or `${file:/absolute/path}` reference.
On Windows the file operand must be an absolute Windows path. File reads use native Go I/O under the invoking user's
identity. Resolved values have surrounding whitespace trimmed and must be nonempty. Interpolation, command execution,
and secret-store references are unsupported. Examples prefer environment references so credentials stay outside YAML.
`validate` checks reference syntax only; `send` resolves and validates the selected destination before sending.

Delivery sends one POST with `Content-Type: application/json` and, when configured, `Authorization: Bearer ...`.
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

This increment delivers explicitly selected events. It does not infer initial-CLEAR eligibility, role matches, or
critical-history policy. Those capabilities remain pending in the inventory. Add future internal-only event facts
separately from this public webhook document.

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
