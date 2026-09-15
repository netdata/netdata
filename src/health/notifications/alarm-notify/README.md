# Experimental Go notifier

This standalone Go module routes JSON notifications to webhook, Slack, Discord, Telegram, Pushover, Pushbullet,
Twilio and MessageBird.
It has no imports from the existing `src/go` module. It is for local development and is not installed, packaged, or
invoked by the Agent.
The active notifier remains `../alarm-notify.sh.in` and its shell configuration.

The current increments provide explicit delivery, role-based routing, modern Slack and native Discord webhooks,
Telegram bot messages, Pushover/Pushbullet notifications and Twilio/MessageBird text messages.
[CAPABILITIES.md](CAPABILITIES.md) tracks the remaining Bash functionality. Configuration and code may change substantially
before production adoption; final redesign follows the working functional baseline.

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
        self.send_response(201 if self.path.endswith(("/Messages.json", "/messages")) else 200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(
            b'{"ok":true,"status":1,"iden":"test-push","sid":"test-message",'
            b'"id":"test-message","result":{"message_id":1}}'
        )

    def log_message(self, *args):
        pass  # Avoid logging credential-bearing request paths.

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

Configuration has `version: 1` and a `destinations` mapping. Webhook, Slack and Discord destinations require `type`
and `url`; `bearer_token` is optional for generic webhooks and rejected for Slack/Discord. Telegram requires
`type: telegram`, `bot_token` and `chat_id`, with the additional settings described below. Provider-specific settings
are rejected on other provider types. Pushover requires `type: pushover`, `app_token` and `user_key`.
Pushbullet requires `type: pushbullet`, `access_token`, and one `email` or `channel_tag`.
Twilio requires `type: twilio`, `account_sid`, `auth_token`, `from` and `to`.
MessageBird requires `type: messagebird`, `access_key`, `originator` and `recipient`.
Destination names are nonsecret identifiers.
The URL must be an absolute HTTP or HTTPS URL with a host and without embedded user/password information
or a fragment. HTTP allows deliberate local or self-hosted delivery; HTTPS verifies certificates. Proxy selection
follows Go's HTTP_PROXY/HTTPS_PROXY/NO_PROXY rules.

URLs, bearer tokens, Telegram bot tokens, Pushover app/user keys, Pushbullet access tokens, Twilio account credentials
and MessageBird access keys accept literal strings or a whole `${env:VARIABLE}` or `${file:/absolute/path}` reference.
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

Deliveries use POST. Twilio and MessageBird use `application/x-www-form-urlencoded`; all other providers use
`application/json`.
Generic webhooks may add `Authorization: Bearer ...`.
Generic webhooks accept HTTP 200–299; Slack and Discord accept HTTP 200. These three providers make one attempt and
close response bodies without buffering or interpreting them. Telegram, Pushover, Pushbullet, Twilio and MessageBird
check bounded JSON acknowledgments; Telegram can retry rate limits as described below. Redirects are never followed.
Errors do not echo config/input values, secret contents, response text, or endpoint URLs.

## Slack app webhooks

Create a [Slack app incoming webhook](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/)
for each channel, then configure one named destination per webhook URL:

```yaml
destinations:
  slack:
    type: slack
    url: ${env:NOTIFY_SLACK_URL}
routing:
  roles:
    chatops: [slack]
```

Use this with the top-level `version: 1`, as shown in `examples/notify.yaml`. Slack's webhook URL is a secret; supply
it through an environment variable or file reference. Slack manages the channel, username and icon in the app's
configuration. Runtime channel/user/username/icon overrides from Bash's legacy Slack integration remain pending and
are not accepted by this provider. This does not change the active Bash integration.

Messages show a plain-text Block Kit summary above details in a status-colored attachment: warning/yellow,
critical/red, clear/green. They include the status transition, node, alert, summary, chart/context when present,
known current/previous values with units, timestamp, and details. Unknown values are omitted; zero remains visible.
An optional event `url` adds a **View alert** link without requiring an interaction server. The plain-text fallback
includes the same content and link.
Alert text cannot introduce mentions or markdown formatting; automatic link parsing and unfurls are disabled.

Slack enforces [section/field limits](https://docs.slack.dev/reference/block-kit/blocks/section-block/) of 3000/2000
characters. Counts include labels, escaped Slack control characters, and the navigation link markup. Oversized content
fails that destination with a safe error rather than being truncated; other selected destinations are still attempted.

For a local demonstration using the receiver above, set the URL to that receiver for this one invocation:

```sh
NOTIFY_SLACK_URL=http://127.0.0.1:18080/slack /tmp/alarm-notify send \
  --config examples/notify.yaml --role chatops < examples/event.json
```

Tests inspect complete Slack payloads and HTTP behavior using local receivers. They do not require or post to a live
Slack workspace, so native Slack rendering is not part of the automated validation.

## Discord webhooks

Create a [Discord incoming webhook](https://docs.discord.com/developers/resources/webhook#execute-webhook) for each
channel and configure its full native URL as a named destination:

```yaml
version: 1
destinations:
  discord_ops:
    type: discord
    url: ${env:NOTIFY_DISCORD_OPS_URL}
  discord_database:
    type: discord
    url: ${env:NOTIFY_DISCORD_DATABASE_URL}
routing:
  roles:
    sysadmin: [discord_ops]
    dba: [discord_ops, discord_database]
```

`--role sysadmin --role dba` sends once to each destination. Use the native webhook URL without a `/slack` suffix.
The URL contains credentials, so an environment or file reference is recommended. The channel belongs to the webhook.
Bash's Discord sender loops over channel names, but Discord's Slack-compatible endpoint does not support its `channel`
field. The Go implementation intentionally omits that ineffective field and loop; role routing selects actual webhook
destinations. The active Bash notifier is unchanged.

The notifier sets `wait=true` on the resolved URL to request server confirmation, preserving other query parameters
such as `thread_id`. It replaces an existing `wait` value and rejects malformed query strings instead of silently
losing parameters. HTTP 200 acknowledges delivery; HTTP 204 is unconfirmed and fails. There are no automatic retries.

Messages contain a status-colored native embed with summary, details, node/alert, status transition, chart/context,
known values with units, timestamp, and an optional title link from event `url`. Markdown control characters are
escaped and `allowed_mentions` disables mentions. Missing or whitespace-only optional details are omitted; zero values
remain visible. Sender names follow Bash's `netdata on NODE` pattern, shortened to 32 characters; the full node remains
in the embed. Discord's configured webhook avatar is used. Additional artwork and presentation options remain pending.

The [embed limits](https://docs.discord.com/developers/resources/message#embed-limits) are 256 characters for the title,
4096 for details, 1024 per field value, and 6000 across the embed's displayed text. Counts include escape characters and
field labels, after trimming surrounding whitespace. Oversized content fails that destination with a safe error;
other selected destinations are still attempted. Alert content is not truncated.

To inspect a Discord payload with the local receiver shown above:

```sh
NOTIFY_DISCORD_URL=http://127.0.0.1:18080/discord /tmp/alarm-notify send \
  --config examples/notify.yaml --role discord_ops < examples/event.json
```

Tests use local receivers and complete JSON comparisons; they do not send to a Discord server or verify native
Discord rendering. This experimental implementation does not complete the production Bash migration requested in
[issue #23531](https://github.com/netdata/netdata/issues/23531).

## Telegram bot messages

Configure one chat and optional topic per named destination. Route to multiple names for multiple recipients;
different names may share a bot token. Create the bot and obtain its token through Telegram's BotFather, and give
the bot access to each target chat before using a real API endpoint.

```yaml
version: 1
destinations:
  telegram_ops:
    type: telegram
    bot_token: ${env:NOTIFY_TELEGRAM_TOKEN}
    chat_id: '-100123'
    message_thread_id: 7
    retries_on_limit: 0
routing:
  roles:
    sysadmin: [telegram_ops]
```

`chat_id` is a nonzero signed numeric ID or `@username`; quote it in YAML. `message_thread_id` is an optional positive
integer for a topic. Bash's `CHAT_ID:TOPIC_ID` syntax becomes these two fields. `retries_on_limit` is an optional
nonnegative integer counting additional attempts; it defaults to zero. Fractional values are rejected.
Telegram destinations reject `url` and `bearer_token`.

The optional `api_url` defaults to `https://api.telegram.org`. For a local Bot API server, supply an HTTP(S) base URL,
optionally with a path prefix. It must not contain credentials, a query, or a fragment. The official Telegram API
requires HTTPS. The notifier appends `/bot<TOKEN>/sendMessage`; do not include the token or method in `api_url`.
Use an environment or file reference for `bot_token`. Selected secrets are resolved before delivery; unused bots
need no local credentials when validating or sending to a different destination.

Messages use escaped HTML with status emoji, summary, node/alert, status transition, chart/context, known values
with units, timestamp, details and an optional event navigation link. Unknown values are omitted and zero remains
visible. CLEAR messages are silent and link previews are disabled, preserving Bash's delivery settings.
The [text limit](https://core.telegram.org/bots/api#sendmessage) is 4,096 Unicode characters after formatting;
generated tags and HTML escapes do not consume visible characters. Oversized content fails the destination without
truncating the alert. Richer cross-provider presentation remains pending in the capability inventory.

Delivery succeeds only on HTTP 200 with a JSON `ok: true` acknowledgment. Responses are limited to 256 KiB and are
never logged. Invalid, oversized, interrupted or negative acknowledgments fail the destination safely.
Optional retries apply only to HTTP 429 with a valid negative acknowledgment. Transport failures, redirects and
other HTTP errors are not retried.

When retries are enabled, the notifier honors Telegram's
[`retry_after`](https://core.telegram.org/bots/api#responseparameters), falling back to one second when absent.
This intentionally corrects Bash's fixed one-second delay. A delay that cannot fit the remaining invocation timeout
fails the destination immediately, allowing later targets to proceed while time remains. Retry waits are cancelable;
all attempts and waits share the existing total deadline. The final destination result includes its retries in the
same success/failure outcome.

To inspect Telegram requests using the local receiver above, use a synthetic token and a local API base:

```yaml
version: 1
destinations:
  telegram_local:
    type: telegram
    api_url: http://127.0.0.1:18080
    bot_token: '123:synthetic-token'
    chat_id: '1'
```

Save as `telegram-local.yaml`, then run:

```sh
/tmp/alarm-notify send --config telegram-local.yaml --destination telegram_local < examples/event.json
```

Local tests verify requests, acknowledgments, retry timing and cancellation. They do not send to Telegram or verify
native Telegram rendering. The production Bash notifier and its configuration are unchanged.

## Pushover notifications

Register a [Pushover application](https://pushover.net/api) and configure one user or group key per named destination.
Routes can select several destinations, each with its own app token and recipient key:

```yaml
version: 1
destinations:
  pushover_ops:
    type: pushover
    app_token: ${env:NOTIFY_PUSHOVER_APP_TOKEN}
    user_key: ${env:NOTIFY_PUSHOVER_USER_KEY}
routing:
  roles:
    sysadmin: [pushover_ops]
```

Both keys contain 30 ASCII letters or digits. Environment and file references keep them out of configuration files;
unused destinations need no local credentials. Other providers' credential, recipient and retry fields are rejected.
An optional `api_url` accepts a literal or secret reference and defaults to `https://api.pushover.net`. A custom HTTP(S)
base may include a path prefix but no credentials, query or fragment. The official service requires HTTPS.
The notifier appends `/1/messages.json`.

Messages include an escaped HTML summary and details, node/alert, status transition, chart/context, known values
with units, the event timestamp and an optional navigation link. Priorities match Bash: CLEAR uses -1 (quiet),
WARNING uses 0 (normal) and CRITICAL uses 1 (high). Unknown values are omitted; zero remains visible.

Following Bash, titles longer than 250 characters and encoded messages longer than 1,024 characters are shortened
with `...`. Shortening preserves Unicode, complete HTML entities and balanced generated tags. HTML markup and entities
count toward the message budget. Supplementary URLs longer than 512 characters are omitted; the notification still
sends. Richer shared presentation remains pending in the inventory.

Delivery requires HTTP 200 and JSON `status: 1`, confirming acceptance into Pushover's queue. Acknowledgments are
limited to 256 KiB; errors omit response text and credentials. There are no automatic retries, including on quota
exhaustion or server errors. A failed destination allows later destinations to proceed within the invocation deadline.

For a local demonstration with the receiver above, save this as `pushover-local.yaml`:

```yaml
version: 1
destinations:
  pushover_local:
    type: pushover
    api_url: http://127.0.0.1:18080
    app_token: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
    user_key: UUUUUUUUUUUUUUUUUUUUUUUUUUUUUU
```

```sh
/tmp/alarm-notify send --config pushover-local.yaml --destination pushover_local < examples/event.json
```

Tests use synthetic keys and local receivers. They verify complete requests, acknowledgments, shortening and
cancellation, without sending to Pushover or verifying native client rendering.

## Pushbullet notifications

Configure one email recipient or channel tag per named destination. Both modes preserve the Bash sender's
capabilities; use multiple named destinations to reach several recipients. Obtain an access token through your
Pushbullet account, and use an environment or file reference to keep it outside YAML:

```yaml
version: 1
destinations:
  pushbullet_ops:
    type: pushbullet
    access_token: ${env:NOTIFY_PUSHBULLET_TOKEN}
    email: ops@example.com
    source_device_id: test-source-device
  pushbullet_channel:
    type: pushbullet
    access_token: ${env:NOTIFY_PUSHBULLET_TOKEN}
    channel_tag: test-alerts
routing:
  roles:
    sysadmin: [pushbullet_ops, pushbullet_channel]
```

Set exactly one recipient field. `email` is a single address without a display name. `channel_tag` is a tag without
Bash's leading `#` or whitespace. The optional `source_device_id` identifies the sending device; omit the synthetic
example value unless you replace it with your device's identifier. These three settings are literal strings.
The token must be nonempty printable ASCII without whitespace. Other providers' settings are rejected.

Pushbullet's [target rules](https://docs.pushbullet.com/#create-push) require ownership of a target channel.
Email targets can receive an email from Pushbullet when they have no account or registered devices, matching the
service behavior used by Bash. The notifier does not look up accounts or devices.

Messages contain the summary/details, node/alert, status transition, chart/context, known values with units and
timestamp. Text is preserved through JSON encoding without adding HTML or shortening it. An event `url` produces a
native link push, as in Bash; without a URL, the notifier sends a note with the same content. Zero values remain visible.
Richer shared presentation stays pending in the inventory.

The optional `api_url` defaults to `https://api.pushbullet.com` and accepts a literal or secret reference. Custom
HTTP(S) bases may have a path prefix but no credentials, query or fragment; the official service requires HTTPS.
The notifier appends `/v2/pushes` and authenticates with the `Access-Token` header.

Delivery requires HTTP 200 and a JSON acknowledgment containing a nonempty created-push `iden` without an error.
Responses are limited to 256 KiB and never logged. There are no automatic retries, including rate-limit and server
errors. Individual failures allow later destinations to proceed within the invocation deadline.

To inspect a Pushbullet request using the receiver above, save this as `pushbullet-local.yaml`:

```yaml
version: 1
destinations:
  pushbullet_local:
    type: pushbullet
    api_url: http://127.0.0.1:18080
    access_token: synthetic-token
    channel_tag: test-alerts
```

```sh
/tmp/alarm-notify send --config pushbullet-local.yaml --destination pushbullet_local < examples/event.json
```

Tests use synthetic tokens and local receivers to verify complete requests, recipient modes, acknowledgments,
secret handling, failure isolation and cancellation. They do not send to Pushbullet or verify native client rendering.

## Twilio text messages

Configure an account SID, Auth Token, sender and one recipient per named destination. Routes select several names
for multiple recipients, preserving Bash's account-based text-message delivery:

```yaml
version: 1
destinations:
  twilio_ops:
    type: twilio
    account_sid: ${env:NOTIFY_TWILIO_ACCOUNT_SID}
    auth_token: ${env:NOTIFY_TWILIO_AUTH_TOKEN}
    from: '+15005550006'
    to: '+15005550009'
routing:
  roles:
    sysadmin: [twilio_ops]
```

Replace the synthetic numbers with your Twilio sender and recipient before using the real service. `from` and `to`
are literal strings, not recipient lists. The sender may be a phone number, alphanumeric sender ID, short code or
channel address; the recipient is a phone number or channel address supported by Twilio. The notifier preserves
these values and leaves service-specific validity checks to Twilio. Quote numbers in YAML to preserve `+` and zeros.

`account_sid` is `AC` followed by 32 hexadecimal characters. `auth_token` is nonempty printable ASCII without
whitespace. Both accept environment/file references; validation checks their syntax without resolving unused
credentials. Other providers' fields are rejected. This increment uses Bash's account SID/Auth Token
[Basic authentication](https://www.twilio.com/docs/usage/requests-to-twilio#using-your-account-sid-and-auth-token).

The optional `api_url` defaults to `https://api.twilio.com` and accepts a literal or secret reference. A custom
HTTP(S) base may include a path prefix but no credentials, query or fragment; the official API requires HTTPS.
The notifier appends `/2010-04-01/Accounts/<ACCOUNT_SID>/Messages.json` and sends form-encoded `From`, `To` and `Body`.

The plain-text body includes summary/details, node/alert, status transition, available chart/context and values,
timestamp and optional event URL. Zero values remain visible. Form encoding preserves Unicode, plus signs and
other special characters. Like Bash, the notifier does not shorten, split or retry messages. Twilio's
[Message API](https://www.twilio.com/docs/messaging/api/message-resource) limits bodies to 1,600 characters and can
segment SMS into multiple charged messages; rejected requests fail the destination. Richer shared content remains
pending in the inventory.

Success requires HTTP 201 and a nonempty created-message `sid` in a JSON response of at most 256 KiB. This confirms
API creation, not delivery to the recipient; the notifier does not poll delivery status. Redirects are refused and
errors omit account credentials, phone numbers, response text and endpoint URLs. A failure allows later destinations
to proceed within the invocation deadline.

For a local demonstration with the receiver above, save this as `twilio-local.yaml`:

```yaml
version: 1
destinations:
  twilio_local:
    type: twilio
    api_url: http://127.0.0.1:18080
    account_sid: AC00000000000000000000000000000000
    auth_token: synthetic-token
    from: '+15005550006'
    to: '+15005550009'
```

```sh
/tmp/alarm-notify send --config twilio-local.yaml --destination twilio_local < examples/event.json
```

Tests use synthetic credentials and loopback receivers to verify complete forms, authentication, routing,
acknowledgments and cancellation. They do not send live messages or verify handset delivery.

## MessageBird SMS

Configure an access key, originator and one recipient per named destination. Select several destinations through
routing to send to multiple recipients:

```yaml
version: 1
destinations:
  messagebird_ops:
    type: messagebird
    access_key: ${env:NOTIFY_MESSAGEBIRD_ACCESS_KEY}
    originator: Netdata
    recipient: '+15005550009'
routing:
  roles:
    sysadmin: [messagebird_ops]
```

Replace the example recipient with your phone number before using the real service. `recipient` is a literal
MSISDN string: digits with an optional leading `+`. Quote it in YAML to preserve its spelling. Recipient lists
and secret references are not accepted in this field; configure one named destination per recipient.

`originator` is a literal sender number or sender ID. MessageBird also supports `inbox` for its Sticky VMN sender
selection where available. The notifier preserves the originator and leaves service-specific validity checks to
MessageBird; alphanumeric sender IDs have an API limit of 11 characters. The `access_key` accepts a literal or
environment/file reference and must resolve to nonempty printable ASCII without whitespace. Other providers'
settings are rejected.

The optional `api_url` defaults to `https://rest.messagebird.com` and accepts a literal or secret reference. A custom
HTTP(S) base may include a path prefix but no credentials, query or fragment; the official API requires HTTPS.
The notifier appends `/messages`, sends form-encoded `originator`, `recipients`, `body` and `datacoding=auto`, and uses
[AccessKey authentication](https://developers.messagebird.com/api/#authentication) with `Accept: application/json`.

The plain-text body includes the summary/details, node/alert, status transition, available chart/context and values,
timestamp and optional event URL. Zero values remain visible. Following Bash, `datacoding=auto` lets the
[SMS API](https://developers.messagebird.com/api/sms-messaging/#send-outbound-sms) select GSM or Unicode encoding.
Long SMS messages may be segmented and billed separately by the service. The notifier does not shorten, split,
retry or poll messages; remote rejections fail the destination. Richer shared presentation stays pending.

Success requires HTTP 201 and a nonempty created-message `id` in a JSON acknowledgment of at most 256 KiB. This
confirms API creation, not handset delivery. Failures omit credentials, recipients, response text and URLs, and
allow later destinations to proceed within the invocation deadline.

For a local demonstration using the receiver above, save this as `messagebird-local.yaml`:

```yaml
version: 1
destinations:
  messagebird_local:
    type: messagebird
    api_url: http://127.0.0.1:18080
    access_key: synthetic-key
    originator: Netdata
    recipient: '+15005550009'
```

```sh
/tmp/alarm-notify send --config messagebird-local.yaml --destination messagebird_local < examples/event.json
```

Tests use synthetic keys and loopback receivers to verify complete forms, `datacoding=auto`, routing,
acknowledgments and cancellation. They do not send live SMS or verify handset delivery.

## Event document

The webhook receives the typed event as JSON. `version` must be `1`. Required fields are `incident_id`, `timestamp`
(RFC 3339), `node`, `alert`, `status`, and `summary`. `incident_id` is an opaque stable incident identifier supplied by
the caller. Current statuses are `WARNING`, `CRITICAL`, and `CLEAR`.

Optional fields are `chart`, `context`, `previous_status`, `info`, `value`, `previous_value`, `units`, and `url`.
`previous_status` accepts the three current statuses plus `UNINITIALIZED`, `UNDEFINED`, and `REMOVED`.
`url` is a caller-supplied alert navigation link: absolute HTTP(S), without embedded user/password information;
fragments are allowed. Use a URL suitable for disclosure in notifications. The notifier does not fetch this link.
Generic webhooks include `url` when it is supplied and omit it otherwise.
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
