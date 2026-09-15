# Experimental Go notifier

This standalone Go module routes JSON notifications to webhook, Slack, Discord, Telegram, Pushover, Pushbullet,
Twilio, MessageBird, Gotify, ntfy, Rocket.Chat, Flock, Fleep, ilert, SIGNL4, Alerta, Dynatrace, Prowl, Kavenegar and SMSEagle.
It has no imports from the existing `src/go` module. It is for local development and is not installed, packaged, or
invoked by the Agent.
The active notifier remains `../alarm-notify.sh.in` and its shell configuration.

The current increments provide explicit delivery, role-based routing, modern Slack and native Discord webhooks,
Telegram bot messages, Pushover/Pushbullet/Gotify/ntfy notifications, Twilio/MessageBird text messages and
Rocket.Chat/Flock/Fleep webhooks, ilert/SIGNL4 incident events and recovery, Alerta/Dynatrace monitoring events,
Prowl push notifications, Kavenegar SMS and SMSEagle SMS/MMS and voice calls.
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
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

class Receiver(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers["Content-Length"])).decode()
        print(body, flush=True)
        status = 200
        if self.path.endswith("/events"):
            status = 202
        elif self.path.endswith(("/Messages.json", "/messages", "/signl4", "/alert", "/api/v2/events/ingest")):
            status = 201
        self.send_response(status)
        self.send_header("Content-Type", "application/xml" if self.path.endswith("/add") else "application/json")
        self.end_headers()
        if self.path.endswith(("/api/v2/messages/sms", "/api/v2/messages/mms",
                               "/api/v2/calls/ring", "/api/v2/calls/tts", "/api/v2/calls/tts_advanced")):
            recipients = json.loads(body)["to"]
            self.wfile.write(json.dumps([
                {"status": "queued", "message": "OK", "number": number, "id": index}
                for index, number in enumerate(recipients, 1)
            ]).encode())
        elif self.path.endswith("/add"):
            self.wfile.write(b'<prowl><success code="200" remaining="999" resetdate="1234567890"/></prowl>')
        elif self.path.endswith("/sms/send.json"):
            self.wfile.write(b'{"return":{"status":200},"entries":[{"messageid":1,"status":1}]}')
        elif self.path.endswith("/message"):
            self.wfile.write(b'{"id":1,"appid":1}')
        elif self.path.endswith("/alert"):
            self.wfile.write(b'{"status":"ok","id":"test-alert"}')
        elif self.path.endswith("/api/v2/events/ingest"):
            self.wfile.write(
                b'{"reportCount":1,"eventIngestResults":[{"status":"OK","correlationId":"test-event"}]}'
            )
        else:
            self.wfile.write(
                b'{"ok":true,"success":true,"status":1,"iden":"test-push","sid":"test-message",'
                b'"id":"test-message","event":"message","result":{"message_id":1}}'
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
Gotify requires `type: gotify`, `api_url` and `app_token`. ntfy requires `type: ntfy` and a full topic `url`;
optional authentication uses `access_token` or `username` with `password`.
Rocket.Chat, Flock and Fleep require their respective `type` and a complete webhook `url`. Rocket.Chat accepts an
optional `channel`; Fleep accepts an optional `sender`. Kavenegar also uses `sender` for its SMS number.
ilert requires `type: ilert` and `integration_key`, with an optional `api_url`. SIGNL4 requires `type: signl4` and
a complete webhook `url`. Alerta requires `type: alerta`, `api_url` and `environment`, with an optional `api_key`.
Dynatrace requires `type: dynatrace`, `api_url`, `api_token` and `entity_selector`; `event_type` and `source` are optional.
Prowl requires `type: prowl` and `api_key`; Kavenegar requires `type: kavenegar`, `api_key`, `sender` and `recipient`.
Both accept an optional `api_url`. SMSEagle requires `type: smseagle`, `api_url`, `access_token` and a `recipients`
array, with optional message/call settings described below. Destination names are nonsecret identifiers.
The URL must be an absolute HTTP or HTTPS URL with a host and without embedded user/password information
or a fragment. HTTP allows deliberate local or self-hosted delivery; HTTPS verifies certificates. Proxy selection
follows Go's HTTP_PROXY/HTTPS_PROXY/NO_PROXY rules.

URLs, bearer tokens, Telegram bot tokens, Pushover app/user keys, Pushbullet access tokens, Twilio account credentials,
MessageBird access keys, Gotify app tokens, ntfy credentials, ilert integration keys, Alerta/Prowl/Kavenegar API keys and Dynatrace
API tokens, plus SMSEagle access tokens, accept literal strings or a whole `${env:VARIABLE}` or `${file:/absolute/path}` reference.
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

Deliveries use POST. Twilio, MessageBird, Prowl and Kavenegar use `application/x-www-form-urlencoded`;
ntfy uses UTF-8 `text/plain`;
the other providers use `application/json`.
Generic webhooks may add `Authorization: Bearer ...`.
Generic webhooks accept HTTP 200–299; Slack, Discord, Flock and Fleep accept HTTP 200. ilert accepts HTTP 202;
SIGNL4 accepts HTTP 200, 201 or 202. These providers make one attempt and
close response bodies without buffering or interpreting them. Telegram, Pushover, Pushbullet, Twilio, MessageBird,
Gotify, ntfy, Rocket.Chat, Alerta, Dynatrace, Kavenegar and SMSEagle check bounded JSON acknowledgments;
Prowl checks a bounded XML acknowledgment. Telegram can retry rate limits as described below.
Redirects are never followed.
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

## Gotify and ntfy

These push services use the existing named destinations, role routing and secret references:

```yaml
version: 1
destinations:
  gotify_ops:
    type: gotify
    api_url: https://gotify.example.com
    app_token: ${env:NOTIFY_GOTIFY_APP_TOKEN}
  ntfy_ops:
    type: ntfy
    url: https://ntfy.example.com/alerts
    access_token: ${env:NOTIFY_NTFY_ACCESS_TOKEN}
routing:
  roles:
    sysadmin: [gotify_ops, ntfy_ops]
```

Gotify's `api_url` is required and may contain a reverse-proxy path prefix; it must not contain credentials,
a query or fragment. The notifier appends `/message` and sends JSON `title`, `message` and `priority` using
[application-token authentication](https://gotify.net/docs/pushmsg) in `X-Gotify-Key`.
The title contains the node, status and summary. The body contains the summary/details, node/alert, status
transition, available chart/context and values, timestamp and optional event URL.

ntfy's `url` is the complete topic URL, including any reverse-proxy path prefix. It accepts deliberate URL query
options; credentials and fragments are rejected. The body contains the same plain-text details, with navigation
provided by a **View node** action when the event has a URL. Following Bash, activating this action clears the
notification. Titles replace underscores in the alert name with spaces. The notifier uses ntfy's documented
[text publishing, header encoding and action format](https://docs.ntfy.sh/publish/) to preserve Unicode and URL
delimiters. It does not use the JSON publishing endpoint or shorten, split or retry messages; server limits and
server handling of large messages still apply.

| Event status | Gotify priority | ntfy priority | ntfy tag |
|---|---|---|---|
| WARNING | 4 | high | warning |
| CRITICAL | 10 | urgent | red_circle |
| CLEAR | 1 | default | white_check_mark |

ntfy supports anonymous publishing when authentication fields are omitted. For HTTP Basic authentication, replace
`access_token` with both `username` and `password`:

```yaml
version: 1
destinations:
  ntfy_basic:
    type: ntfy
    url: ${env:NOTIFY_NTFY_URL}
    username: ${env:NOTIFY_NTFY_USERNAME}
    password: ${env:NOTIFY_NTFY_PASSWORD}
```

Choose one authentication mode. Incomplete username/password pairs and mixing them with an access token fail
validation. Usernames cannot contain colons, and Basic credentials cannot contain control characters. Gotify app
tokens and ntfy access tokens must be printable ASCII without whitespace. All credentials and endpoints accept
whole environment/file references; only selected destinations resolve them. Other providers' fields are rejected.

Both providers require HTTP 200 and a JSON acknowledgment of at most 256 KiB. Gotify requires a positive numeric
message `id`; ntfy requires a nonempty message `id` and `event: message`. This confirms server acceptance, not
delivery to a phone. Errors omit credentials, endpoint URLs and response contents. Redirects are not followed,
and the invocation deadline covers delivery and response reads.

For a local demonstration with the receiver above, use:

```yaml
version: 1
destinations:
  gotify_local:
    type: gotify
    api_url: http://127.0.0.1:18080
    app_token: synthetic-token
  ntfy_local:
    type: ntfy
    url: http://127.0.0.1:18080/alerts
routing:
  roles:
    sysadmin: [gotify_local, ntfy_local]
```

Save as `push-local.yaml` and run:

```sh
/tmp/alarm-notify send --config push-local.yaml --role sysadmin < examples/event.json
```

Tests use synthetic credentials and loopback receivers to check full payloads/headers, authentication,
routing, safe failures and cancellation. They do not contact real push services or verify device notifications.

## Rocket.Chat, Flock and Fleep webhooks

These providers use complete incoming webhook URLs. Create the hook in the destination service, then configure
named destinations and reuse the existing role routing:

```yaml
version: 1
destinations:
  rocket_ops:
    type: rocketchat
    url: ${env:NOTIFY_ROCKETCHAT_URL}
    channel: '#alerts'
  flock_ops:
    type: flock
    url: ${env:NOTIFY_FLOCK_URL}
  fleep_ops:
    type: fleep
    url: ${env:NOTIFY_FLEEP_URL}
    sender: Netdata
routing:
  roles:
    sysadmin: [rocket_ops, flock_ops, fleep_ops]
```

Rocket.Chat's optional `channel` selects one `#channel` or `@user`. Enable **Allow to overwrite destination channel
in body parameters** in the [incoming integration](https://docs.rocket.chat/docs/integrations) when using it.
Omit `channel` to use the integration's configured destinations. For different overrides, define separate named
destinations sharing the same URL. Channel names are literal, with no comma lists, whitespace or secret references.
The notifier sends a host-derived alias, status/summary text and an attachment containing alert facts, timestamp,
optional info and a navigation link. URL previews are disabled.

Flock [binds each incoming webhook to its channel](https://support.flock.com/hc/en-us/articles/360006943354-Incoming-webhooks).
Define a separate destination URL for each channel. The Go sender omits Bash's ineffective channel-name loop by
explicit approval: it sends once per selected destination name. The payload uses a host-derived `sendAs` name,
status/summary text and an attachment with the full alert description and optional navigation URL. Its message
structure follows the [official Flock SDK](https://github.com/flockchat/pyflock).

Rocket.Chat and Flock use yellow (`#f0ad4e`) for WARNING, red (`#d9534f`) for CRITICAL and green (`#5cb85c`) for CLEAR.
Alert facts include node, alert name, status transition, available chart/context and current/previous values with
units. Zero values remain present. Extended artwork/presentation remains tracked in the migration inventory.

Fleep uses its [JSON webhook format](https://fleep.io/blog/integrations/webhooks/): `message` contains the alert
description, facts, timestamp and optional navigation URL. Optional YAML `sender` maps to the webhook's `user`
field; omit it to use the service's default sender. It is a literal name, permits spaces and Unicode, and rejects
control characters and secret references.

URLs accept literal, whole environment or whole absolute-file references and preserve query parameters.
Authentication is carried by the webhook URL; separate token fields are rejected. JSON encoding preserves quotes,
newlines and Unicode. Messages are sent whole, without automatic shortening or splitting; the service enforces its
own size limits and formatting rules.

All three require HTTP 200. Rocket.Chat also requires `success: true` in a JSON response of at most 256 KiB and
rejects reported top-level or individual-room errors. Flock and Fleep acknowledge through HTTP status; their response
bodies are closed without reading them. Errors omit endpoint URLs and response contents. There are no retries or
redirects, and invocation cancellation/deadline still stops remaining deliveries after an earlier success.

To use the local receiver above with all three providers:

```yaml
version: 1
destinations:
  rocket_local:
    type: rocketchat
    url: http://127.0.0.1:18080/rocket-hook
    channel: '#test-alerts'
  flock_local:
    type: flock
    url: http://127.0.0.1:18080/flock-hook
  fleep_local:
    type: fleep
    url: http://127.0.0.1:18080/fleep-hook
    sender: Netdata
routing:
  roles:
    sysadmin: [rocket_local, flock_local, fleep_local]
```

Save it as `/tmp/notify-chat.yaml` and send a synthetic event:

```sh
/tmp/alarm-notify send --config /tmp/notify-chat.yaml --role sysadmin < examples/event.json
```

## ilert and SIGNL4 incident events

These providers use `incident_id` to correlate WARNING/CRITICAL events with CLEAR recovery. Reuse the same ID for
updates and recovery of an incident; separate incidents, including those on different nodes, need distinct IDs.
No local history is required. WARNING and CRITICAL both create alert events; CLEAR sends a resolution event.

### ilert

Create an **API** alert source in ilert and use its integration key. This experimental provider uses the
[Event API](https://docs.ilert.com/developer-docs/rest-api/api-reference/events), which requires different source
setup from Bash's Netdata-specific webhook. A user API token or the existing Netdata-source webhook URL is not the
credential for this destination.

```yaml
version: 1
destinations:
  ilert:
    type: ilert
    integration_key: ${env:NOTIFY_ILERT_INTEGRATION_KEY}
routing:
  roles:
    oncall: [ilert]
```

`integration_key` accepts a literal or whole environment/file reference and must resolve to nonempty printable ASCII
without whitespace. `api_url` is optional, supports the same references, and defaults to `https://api.ilert.com/api`.
Custom HTTP(S) bases may include a proxy path prefix, but no query, fragment or embedded credentials. The official
API host requires HTTPS. The notifier appends `/events`; include the `/api` prefix when using the official base.
Other providers' fields are rejected. Only selected destinations resolve secrets.

The payload sends `ALERT` for WARNING/CRITICAL and `RESOLVE` for CLEAR. `alertKey` is lowercase SHA-256 hex of the
exact `incident_id`: ilert trims keys and compares them case-insensitively, so this encoding keeps IDs that differ
only in case or surrounding whitespace distinct. The original ID and complete event remain in `customDetails`.
The summary includes the node, current status and event summary; details include the current alert facts and status
transition. An optional navigation URL becomes a `View alert` link. Numeric severity and priority overrides are
unset; the API alert source controls those policies, and the Netdata status remains in the summary and event details.

Delivery succeeds on HTTP 202. The body is closed without reading it. No redirects or retries are performed,
including on rate limits or server errors.

### SIGNL4

Use a team webhook URL as described in SIGNL4's
[HTTP documentation](https://support.signl4.com/hc/en-us/articles/9006097919005-HTTP-details):

```yaml
version: 1
destinations:
  signl4:
    type: signl4
    url: ${env:NOTIFY_SIGNL4_URL}
routing:
  roles:
    oncall: [signl4]
```

The URL contains the team secret; it supports a literal or whole environment/file reference. Other providers'
fields are rejected. Configure separate named URLs for different teams and use normal role fan-out to select them.

`Title` contains the node, current status and summary. `Message` contains the summary, info, node, alert,
status transition, optional chart/context/values/units, timestamp and navigation URL. `Severity` carries the current
Netdata status, and `X-S4-SourceSystem` is `Netdata`. JSON encoding preserves quotes, newlines and Unicode.

Following SIGNL4's
[status mapping contract](https://support.signl4.com/hc/en-us/articles/9124452127773-Control-parameters-X-S4-parameters-Status-mapping-enrichment-filtering),
`X-S4-ExternalID` is the exact `incident_id`, and `X-S4-Status` is `new` for WARNING/CRITICAL or `resolved` for CLEAR.
This deliberately corrects Bash's per-event `unique_id`, which changes between an alert and its recovery.
HTTP 200, 201 and 202 are accepted, as in Bash; response bodies are closed without reading them. There are no retries
or redirects. Production Bash configuration and delivery remain unchanged.

### Local incident delivery

With the local receiver above, save this as `/tmp/notify-incidents.yaml`:

```yaml
version: 1
destinations:
  ilert_local:
    type: ilert
    integration_key: synthetic-key
    api_url: http://127.0.0.1:18080/api
  signl4_local:
    type: signl4
    url: http://127.0.0.1:18080/signl4
routing:
  roles:
    oncall: [ilert_local, signl4_local]
```

```sh
/tmp/alarm-notify send --config /tmp/notify-incidents.yaml --role oncall < examples/event.json
```

To exercise recovery, send another event with the same `incident_id`, `status: CLEAR` and the appropriate
`previous_status`. ilert will reuse `alertKey`; SIGNL4 will reuse `X-S4-ExternalID`.

## Alerta and Dynatrace monitoring events

Both API bases must be absolute HTTP(S) URLs without user information, queries or fragments, including empty `?`
or `#` suffixes. Other providers' configuration fields are rejected.

### Alerta

Use the [Alerta API](https://docs.alerta.io/api/reference.html) base URL and one environment per named destination:

```yaml
version: 1
destinations:
  alerta:
    type: alerta
    api_url: ${env:NOTIFY_ALERTA_API_URL}
    api_key: ${env:NOTIFY_ALERTA_API_KEY}
    environment: Production
routing:
  roles:
    monitoring: [alerta]
```

`api_url` is required and receives an appended `/alert`; include any API path prefix in the base. `api_key` is
optional for servers without authentication; when set it uses `Authorization: Key ...`. Both fields accept whole
environment/file references, resolved only for selected destinations. Keys must be printable ASCII without whitespace.
Alerta's documented public demo API hosts (`api.alerta.io`, `api.alerta.dev`, `alerta-api.fly.dev`) require HTTPS.
Custom and local servers may use HTTP. This check also applies after resolving an API URL reference.
`environment` is a required literal. Alerta commonly allows `Production` and `Development`; custom environments
must be permitted by the server's configuration. Use separate named destinations for multiple environments.

WARNING, CRITICAL and CLEAR map to `warning`, `critical` and `cleared`. Alerta correlates by environment, resource
and event. As in Bash, resource is the node and event is `chart.alert`; for charts starting with `httpcheck`, resource
is the chart and event is the alert name. With no chart, event is the alert name. Keep these fields stable through
recovery. Different nodes sharing an httpcheck chart therefore share that resource, preserving Bash behavior.

Messages use service `Netdata`, group `Performance`, origin `netdata/<node>` and type `netdataAlarm`. Text includes
summary, information, status/previous status, chart/context, values/units, timestamp and optional navigation. Attributes
include alert/chart/context and an escaped navigation link. `createTime` uses UTC milliseconds; `rawData` contains the
complete JSON Event, and a diagnostic tag carries `incident_id`. Legacy roles, source and alarm IDs need future Event
extensions/adapters and are not fabricated from unrelated native fields.

HTTP 200/201 require a JSON acknowledgment with `status: ok` and a nonempty `id`. HTTP 202 means suppressed, matching
Bash's treatment of blackouts: it is reported separately in the destination error and does not count as delivery.
If every selected destination is suppressed or fails, exit status is `1`; a successful other destination yields `0`.

### Dynatrace

This provider uses [Events API v2](https://docs.dynatrace.com/docs/dynatrace-api/environment-api/events-v2/post-event).
Create an API token with `events.ingest` permission and configure an explicit entity selector:

```yaml
version: 1
destinations:
  dynatrace:
    type: dynatrace
    api_url: ${env:NOTIFY_DYNATRACE_API_URL}
    api_token: ${env:NOTIFY_DYNATRACE_API_TOKEN}
    entity_selector: 'type(HOST),tag("netdata")'
    event_type: CUSTOM_INFO
    source: Netdata Alarm
routing:
  roles:
    monitoring: [dynatrace]
```

`api_url` is the environment base: for example, `https://example.live.dynatrace.com` or
`https://activegate.example.com:9999/e/environment`. The notifier appends `/api/v2/events/ingest` and uses
`Authorization: Api-Token ...`. The API base and token accept whole environment/file references. A token must be
nonempty printable ASCII without whitespace. This native setup replaces Bash's separate server/space/tag settings
and v1 payload; it does not read those shell settings.

`entity_selector` is a required literal of at most 2000 characters, sent unchanged. The server validates
[selector syntax](https://docs.dynatrace.com/docs/dynatrace-api/environment-api/entity-v2/entity-selector);
use a HOST type/tag selector for Bash-style host targeting or an explicit entity ID. Tag/type selection normally
covers entities active within the last 24 hours; entity-ID selection can reach older entities. Choose a selector
that targets the intended hosts. The notifier does not interpolate the Event node into it.

`event_type` defaults to `CUSTOM_INFO`; supported values are `AVAILABILITY_EVENT`, `CUSTOM_ALERT`, `CUSTOM_ANNOTATION`,
`CUSTOM_CONFIGURATION`, `CUSTOM_DEPLOYMENT`, `CUSTOM_INFO`, `ERROR_EVENT`, `MARKED_FOR_TERMINATION`, `PERFORMANCE_EVENT`,
`RESOURCE_CONTENTION_EVENT` and `WARNING`. `source` defaults to `Netdata Alarm` and maps to `dt.event.source`.
Both settings are literals. The title carries node/status/summary; `dt.event.description` contains the same readable
alert facts as Alerta, with `netdata.incident_id` and `netdata.timestamp` retaining identity and original time.
Property values exceeding 4096 characters fail before HTTP delivery, without truncation.

Every status uses the configured event type, including CLEAR. CLEAR describes recovery but **does not explicitly
close an existing Dynatrace problem**. The notifier omits `startTime`, `endTime` and `timeout`, retaining Bash's
use of ingestion time and Dynatrace's default lifecycle. The original Event timestamp is kept as a property.

HTTP 201 requires a positive `reportCount` matching the number of `eventIngestResults`, with every result reporting
`OK` and a nonempty `correlationId`. No matched entities, malformed results or a partial failure fail the destination;
HTTP acceptance alone is insufficient. Both monitoring providers limit acknowledgment bodies to 256 KiB, make one
attempt, follow no redirects and share the invocation deadline. Provider response text and credentials are not logged.

### Local monitoring delivery

Use the local receiver above with synthetic settings:

```yaml
version: 1
destinations:
  alerta_local:
    type: alerta
    api_url: http://127.0.0.1:18080/alerta
    environment: Production
  dynatrace_local:
    type: dynatrace
    api_url: http://127.0.0.1:18080/dynatrace
    api_token: synthetic-token
    entity_selector: 'type(HOST),tag("netdata")'
routing:
  roles:
    monitoring: [alerta_local, dynatrace_local]
```

Save this as `monitoring-local.yaml`, then run:

```sh
/tmp/alarm-notify send --config monitoring-local.yaml --role monitoring < examples/event.json
```

Repeat with WARNING, CRITICAL and CLEAR events. Alerta keeps its correlation fields and changes severity; Dynatrace
keeps its entity selector and configured type while updating descriptive status.

## Prowl notifications

Prowl uses its [Add API](https://www.prowlapp.com/api.php). A destination can hold one API key or a comma-separated
batch of keys; each key must be exactly 40 hexadecimal characters. Keep the entire list in one secret value:

```yaml
version: 1
destinations:
  prowl:
    type: prowl
    api_key: ${env:NOTIFY_PROWL_API_KEY}
routing:
  roles:
    push_ops: [prowl]
```

The request goes to `https://api.prowlapp.com/publicapi/add`. `api_url` overrides the base before `/add`; it accepts
literal, environment or file references. The public API requires HTTPS; custom local/proxy bases may use HTTP.

Each selected destination sends its complete key list in one request. Keys configured as separate named destinations
produce separate requests. Prowl limits API calls by source IP (normally 1000/hour), so use a batch when recipients
should always receive the same notification. Prowl can accept a batch when only some keys remain authorized; its
acknowledgment does not report individual-key delivery results. There are no verification requests or automatic retries.

Application is `Netdata`; priorities are WARNING `1`, CRITICAL `2`, and CLEAR `0`, matching Bash. The event title contains
node, status and summary; the description carries the common plain-text event facts. The optional dashboard URL goes
in Prowl's separate `url` field. Prowl's UTF-8 byte limits are enforced before sending: event 1024, description 10000,
and URL 512 bytes. Oversized fields fail the destination with a safe error; the notifier does not silently shorten them.

Success requires HTTP 200 and one XML `prowl/success` element with `code="200"`, with no error element. Malformed,
trailing or oversized response data fails. Response reading is limited to 256 KiB and the invocation deadline.

## Kavenegar SMS

Kavenegar uses the [v1 SMS Send API](https://kavenegar.com/rest.html) over HTTPS:

```yaml
version: 1
destinations:
  kavenegar:
    type: kavenegar
    api_key: ${env:NOTIFY_KAVENEGAR_API_KEY}
    sender: '+15005550006'
    recipient: '+15005550009'
routing:
  roles:
    sms_ops: [kavenegar]
```

The numbers above are synthetic examples; real delivery requires a sender assigned to the account and the intended
recipient. `sender` and `recipient` are quoted literal strings of digits with an optional leading `+`; leading zeroes
are preserved. Use separate named destinations and existing routing for multiple recipients.

The default base is `https://api.kavenegar.com/v1`; the request appends an escaped API-key segment and `/sms/send.json`.
`api_url` can override the base for a proxy or local receiver, including through an environment or file reference.
The public endpoint requires HTTPS; custom bases may use HTTP. This replaces Bash's plaintext public request.
`api_key` accepts a literal or one whole environment/file secret reference. Errors never include the key-bearing URL.

The form contains `sender`, `receptor` and `message`. Messages contain common plain-text event facts and the optional
navigation URL for WARNING, CRITICAL and CLEAR. Text is sent intact; Kavenegar handles SMS encoding and splitting.
There are no automatic retries or delivery-status polling. Success means HTTP 200, JSON `return.status: 200`, and one
entry with a positive `messageid`. This acknowledges API acceptance, not final delivery to the phone. Responses are
limited to 256 KiB, respect the invocation deadline and are never logged.

### Local Prowl and Kavenegar exercise

Use only synthetic keys with the local receiver above, which prints request bodies:

```yaml
version: 1
destinations:
  prowl_local:
    type: prowl
    api_url: http://127.0.0.1:18080/prowl
    api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
  kavenegar_local:
    type: kavenegar
    api_url: http://127.0.0.1:18080/kavenegar/v1
    api_key: synthetic-key
    sender: '+15005550006'
    recipient: '+15005550009'
routing:
  roles:
    form_ops: [prowl_local, kavenegar_local]
```

Save as `form-local.yaml` and run:

```sh
/tmp/alarm-notify send --config form-local.yaml --role form_ops < examples/event.json
```

Repeat with WARNING, CRITICAL and CLEAR events to inspect the priority and status changes.

## SMSEagle SMS, MMS and calls

SMSEagle uses its [API v2](https://www.smseagle.eu/docs/apiv2/) on a device running firmware 5.0 or later:

```yaml
version: 1
destinations:
  smseagle:
    type: smseagle
    api_url: ${env:NOTIFY_SMSEAGLE_API_URL}
    access_token: ${env:NOTIFY_SMSEAGLE_ACCESS_TOKEN}
    recipients: ['+15005550009', '05005550009']
    message_type: sms
routing:
  roles:
    appliance_ops: [smseagle]
```

The numbers are synthetic examples. `recipients` is a nonempty array of quoted phone-number strings: digits with an
optional leading `+`. Leading zeroes are preserved; exact duplicates and whitespace are rejected. Each selected
named destination sends one request for its entire recipient list.

`api_url` is the device base **before** `/api/v2`, optionally including a reverse-proxy prefix. There is no default.
It accepts HTTP or HTTPS without credentials, query or fragment; HTTPS verifies certificates. Both `api_url` and
`access_token` accept literal strings or whole environment/file references. Authentication uses the `Access-Token`
header; the token must be nonempty printable ASCII without whitespace.

| `message_type` | Appended endpoint | Content and options |
|---|---|---|
| `sms` (default) | `/api/v2/messages/sms` | Plain text, optional navigation URL and automatic encoding |
| `mms` | `/api/v2/messages/mms` | Same text and encoding; text-only MMS, without attachments |
| `ring` | `/api/v2/calls/ring` | Recipients and `call_duration` only |
| `tts` | `/api/v2/calls/tts` | Plain text without navigation URL and `call_duration` |
| `tts_advanced` | `/api/v2/calls/tts_advanced` | TTS content, `call_duration` and `voice_id` |

`call_duration` is a positive integer of seconds, defaults to `10`, and is allowed only for call modes. `voice_id` is
a positive integer, defaults to `1`, and is allowed only for advanced TTS. The device must provide the chosen voice.
TTS text over the documented 960-character limit fails before sending; text is not silently truncated.

SMS and MMS select `encoding: standard` when every character in the rendered message belongs to the GSM-7 default or
extension alphabet, and `encoding: unicode` otherwise. This includes all event fields and the navigation URL. A status
transition such as `CLEAR → WARNING` selects Unicode because of the arrow. Original text is preserved without
transliteration or shortening; Unicode can increase SMS segment count. The appliance handles segmentation.
Automatic selection is an approved change from Bash's implicit standard encoding; there is no manual encoding setting.

All modes carry WARNING, CRITICAL and CLEAR using the current native event. Ring calls do not speak alert text.
Success requires HTTP 200 and a JSON array with one result per recipient, each with `status: queued` and a positive
`id`. A rejected, missing or partial batch fails that destination; another successful destination still gives the
normal any-success invocation result. This confirms appliance queue acceptance, not final phone delivery. Replies
are bounded to 256 KiB and the invocation deadline. There are no retries or delivery-status polling.

### Local SMSEagle exercise

Use the local receiver above with synthetic credentials and recipients:

```yaml
version: 1
destinations:
  sms:
    type: smseagle
    api_url: http://127.0.0.1:18080
    access_token: synthetic-token
    recipients: ['+15005550009', '05005550009']
  mms:
    type: smseagle
    api_url: http://127.0.0.1:18080
    access_token: synthetic-token
    recipients: ['+15005550009']
    message_type: mms
  ring:
    type: smseagle
    api_url: http://127.0.0.1:18080
    access_token: synthetic-token
    recipients: ['+15005550009']
    message_type: ring
    call_duration: 15
  tts:
    type: smseagle
    api_url: http://127.0.0.1:18080
    access_token: synthetic-token
    recipients: ['+15005550009']
    message_type: tts
  advanced:
    type: smseagle
    api_url: http://127.0.0.1:18080
    access_token: synthetic-token
    recipients: ['+15005550009']
    message_type: tts_advanced
    voice_id: 7
routing:
  roles:
    appliance_ops: [sms, mms, ring, tts, advanced]
```

Save as `smseagle-local.yaml` and run:

```sh
/tmp/alarm-notify send --config smseagle-local.yaml --role appliance_ops < examples/event.json
```

Repeat with WARNING, CRITICAL and CLEAR events to inspect each mode's payload. To inspect standard encoding, use
GSM-compatible event text with no differing `previous_status`, so the message contains no transition arrow.

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
