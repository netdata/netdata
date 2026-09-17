# Experimental Go notifier

This standalone Go module routes JSON notifications to webhook, Slack, Discord, Telegram, Pushover, Pushbullet,
Twilio, MessageBird, Gotify, ntfy, Rocket.Chat, Flock, Fleep, ilert, SIGNL4, Alerta, Dynatrace, Prowl, Kavenegar,
SMSEagle, PagerDuty, Opsgenie, Microsoft Teams, Matrix, custom commands, SMS Server Tools 3, syslog, AWS SNS, Kafka HTTP
bridges, email through sendmail and IRC through nc.
It has no imports from the existing `src/go` module. It is for local development and is not installed, packaged, or
invoked by the Agent.
The active notifier remains `../alarm-notify.sh.in` and its shell configuration.

The current increments provide explicit delivery, role-based routing, modern Slack and native Discord webhooks,
Telegram bot messages, Pushover/Pushbullet/Gotify/ntfy notifications, Twilio/MessageBird text messages and
Rocket.Chat/Flock/Fleep webhooks, ilert/SIGNL4 incident events and recovery, Alerta/Dynatrace monitoring events,
Prowl push notifications, Kavenegar SMS, SMSEagle SMS/MMS and voice calls, PagerDuty v1/v2 incident events,
Opsgenie alert creation and closure, Teams Workflows cards, Matrix room notices, foreground command delivery, syslog,
AWS SNS, Kafka HTTP bridges, email through sendmail, IRC through nc and destination status/history filters.
[CAPABILITIES.md](CAPABILITIES.md) tracks the remaining Bash functionality. Configuration and code may change substantially
before production adoption. Shared mechanisms have separate packages, and each provider owns its typed configuration
and delivery implementation behind a common sender interface.

## Build and run

From this directory, using the Go version in `go.mod`:

```sh
go build -o /tmp/alarm-notify .
/tmp/alarm-notify check-legacy --config ../health_alarm_notify.conf
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
        opsgenie = self.path.endswith("/v2/alerts") or (
            "/v2/alerts/" in self.path and self.path.endswith("/close?identifierType=alias")
        )
        matrix = "/_matrix/client/v3/rooms/" in self.path and "/send/m.room.message/" in self.path
        teams = self.path.split("?", 1)[0].endswith("/teams")
        status = 200
        if teams or opsgenie or self.path.endswith(("/events", "/v2/enqueue")):
            status = 202
        elif self.path.endswith(("/Messages.json", "/messages", "/signl4", "/alert", "/api/v2/events/ingest")):
            status = 201
        elif self.path.split("?", 1)[0].endswith("/kafka"):
            status = 204
        self.send_response(status)
        self.send_header("Content-Type", "application/xml" if self.path.endswith("/add") else "application/json")
        self.end_headers()
        if status == 204:
            return
        if matrix:
            self.wfile.write(b'{"event_id":"$test-event"}')
        elif opsgenie:
            self.wfile.write(b'{"result":"Request will be processed","requestId":"test-request"}')
        elif self.path.endswith(("/api/v2/messages/sms", "/api/v2/messages/mms",
                               "/api/v2/calls/ring", "/api/v2/calls/tts", "/api/v2/calls/tts_advanced")):
            recipients = json.loads(body)["to"]
            self.wfile.write(json.dumps([
                {"status": "queued", "message": "OK", "number": number, "id": index}
                for index, number in enumerate(recipients, 1)
            ]).encode())
        elif self.path.endswith(("/generic/2010-04-15/create_event.json", "/v2/enqueue")):
            key = "dedup_key" if self.path.endswith("/v2/enqueue") else "incident_key"
            self.wfile.write(json.dumps({"status": "success", key: json.loads(body)[key]}).encode())
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

    do_PUT = do_POST

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

## Legacy configuration reader

The experimental `check-legacy` command checks whether old `health_alarm_notify.conf` files use the supported
configuration syntax. It runs natively without Bash, including on Windows. Supply files explicitly in load order;
there is no automatic stock-file discovery:

```sh
/tmp/alarm-notify check-legacy --config ../health_alarm_notify.conf --config /path/to/user/health_alarm_notify.conf
```

A successful check means **syntax support only**. It does not evaluate variable values, validate provider names,
credentials, recipients or routing, or establish that a custom function will work. It reads no event from stdin,
resolves no secrets and executes no configuration code. `send` and `validate` continue to accept YAML only.
Use `send-legacy` below for supported old-format delivery. The optional Unix Bash custom-function runtime remains
a later increment.

The reader supports this declarative subset:

| Construct | Supported behavior |
|---|---|
| Comments and separators | Blank lines, shell comments, newlines, semicolons and backslash-newline continuation |
| Scalars | `NAME=value`, including empty values, single/double quotes, adjacent quoted parts and shell backslash escaping |
| References | `$NAME` and `${NAME}` referring to scalar variables; no special/positional variables or array references |
| Recipient entries | `role_recipients_email[sysadmin]="ops@example.com"`; map names must start with `role_recipients_`, and keys must be nonempty literal strings; quote keys with punctuation or spaces |
| Recipient maps | `role_recipients_email=([sysadmin]=ops@example.com [dba]=db@example.com)`, `=()` to reset, and optional `declare -A` declarations of these maps |
| Functions | Plain `name() { ...; }` or `function name { ...; }` declarations, including helpers; bodies must parse as Bash and are retained without execution |

Top-level commands, `source`, conditionals, loops, redirects, pipelines, background work and command/process
substitution are rejected. So are arithmetic, advanced parameter expansion, unquoted tilde expansion, ANSI-C/localized
quoting, `export`/`readonly`, append assignments, indexed arrays and arbitrary associative maps. Quote a literal tilde
where Bash would otherwise expand it. These restrictions apply to declarative settings; function bodies are retained
as Bash code and are not checked against the settings subset. Function declarations themselves must have a plain
brace body without attached redirections or command modifiers. Unquoted padding around recipient keys is rejected;
use `[" sysadmin "]` when the spaces belong to the key.

The internal evaluator applies files and assignments in order using explicitly supplied initial scalar variables.
It never reads the process environment. Undefined variables expand to empty strings. Expansion happens once at
assignment time: changing `DEFAULT_RECIPIENT_EMAIL` later does not retroactively change a role entry assigned from it.
Expanded values remain literal, including strings resembling native `${env:...}` or `${file:...}` secret references.
Later files replace only assigned scalars, recipient keys and functions. A whole-map initializer replaces that map;
`declare -A` without an initializer preserves its entries. The reader retains recipient modifiers as literal strings;
the separate legacy resolver interprets them. Function text is retained exactly, including internal comments.

Each input file is limited to 1 MiB. Evaluation also limits individual values to 1 MiB and total stored names/values
and function source to 4 MiB, preventing repeated expansion from growing without bounds. The syntax check does not
perform evaluation and therefore does not check those evaluated-value limits. Errors identify the input file by its
one-based argument order and, where available, a line/column; they do not print configuration text or values.
File-open errors retain the filesystem cause while omitting the configured path. All specified files must be readable
and pass the check. `check-legacy` accepts the same positive `--timeout` as the other
commands and returns `0` on supported syntax or `1` on errors/cancellation.

## Legacy recipient routing

`send-legacy` uses `internal/legacyrouting.Resolve` to resolve evaluated settings for the applicable methods and
requested roles. It returns eligible method/recipient pairs before sender construction. `check-legacy` still checks
syntax only and does not invoke this resolver.

The resolver uses `role_recipients_<method>` and `DEFAULT_RECIPIENT_<METHOD>`. Its rules preserve the legacy format
without changing native YAML routing:

- Roles and recipient lists split on commas, spaces, tabs and newlines. Other whitespace remains literal. There is no
  shell quoting pass or filename globbing after evaluation, and strings resembling native secret references stay literal.
- A missing or exactly empty role entry falls back to that method's default. A whitespace-only entry selects nobody;
  it does not fall back. The exact roles `silent` and `disabled` are ignored individually. The exact recipient token
  `disabled` is omitted; other selected roles and recipients still apply. These names are case-sensitive.
- Recipient suffixes `|nowarn`, `|noclear` and `|critical` are case-insensitive and may be combined. Empty/repeated
  modifier segments are tolerated as in Bash. Unknown modifiers and an empty base recipient are errors. An error
  returns no partial targets and never echoes recipient or modifier values, which can contain credentials.
- Multiple occurrences of the same recipient within one method form a union: if any occurrence permits the alert,
  select that recipient once. Different methods remain distinct. Results follow method order, role order and first
  recipient appearance, including an earlier filtered occurrence that a later role permits.
- Policies reuse the native status/history evaluator. `critical` uses the producer-supplied
  `critical_seen_since_clear` fact; there are no per-recipient state files. Stateless skips need no history. An
  unrestricted permitting occurrence also makes history unnecessary for that recipient. If eligibility still depends
  on missing history, the whole resolution fails before returning any targets.
- No eligible recipients is a successful empty result. Resolution never constructs senders, resolves credentials,
  executes functions or performs transport operations. Returned recipients may themselves be secrets and must not be
  used as diagnostic labels by later adapters.

Two intentional corrections to Bash apply: unknown modifiers fail instead of logging and permitting delivery, and
role/recipient wildcard characters are literal instead of expanding against local filenames. Production Bash remains
unchanged. Unix `custom_sender()` support, including helpers, event context and final-recipient batching, remains pending.

## Legacy configuration delivery

`send-legacy` applies the supported shell-format settings to the existing Go providers. It does not require Bash for
HTTP delivery. Supply stock and user files explicitly in order, and the same JSON event used by native `send`:

```sh
/tmp/alarm-notify send-legacy \
  --config ../health_alarm_notify.conf \
  --config /path/to/user/health_alarm_notify.conf \
  --role sysadmin --method telegram < examples/event.json
```

Repeat `--role` or `--method` as needed. Without `--method`, all configured methods are considered. With it, only
those methods are considered; names use the Bash suffixes below (`pd` and `sms`, not `pagerduty` and `smstools3`).
Explicitly selecting an unknown or unimplemented method is an error. All files must parse even when selection is narrowed.

Activation requires exact `SEND_<METHOD>=YES` (also `AUTO` for email), the method's nonempty prerequisites and,
for recipient-based methods, an eligible recipient. Without stock configuration, recipient-based methods and Kafka
default to `YES`. Dynatrace, SIGNL4, Opsgenie and ilert need an explicit `YES`, normally supplied by the stock file.
Missing prerequisites or required recipients disable that method, except that an obsolete ilert source URL is rejected
before credential gating (see below). Kafka, SIGNL4, Dynatrace, Opsgenie and ilert are global: they send once when
enabled and configured, regardless of roles, including reserved roles. Gotify, Discord and Flock send once only when at least one
recipient survives filtering. Email, Prowl and SMSEagle batch their final eligible recipients. Teams sends once per distinct final URL after recipient filtering. Other mappings send
once per distinct eligible target.

All selected routing policies and eligible provider configurations are checked **before any delivery**. Missing
required critical history, an eligible unsupported method, or invalid supported settings reject the whole invocation.
No eligible targets is a successful no-op. Delivery uses the existing sequential, any-success result handling and
invocation deadline. Logs use safe labels such as `telegram-1`, never recipient keys or URLs. The legacy summary counts
attempted successes and failures; filtering happens before the plan is built, so it does not report a skipped count.

| Method | Legacy settings and recipient meaning |
|---|---|
| `alerta` | `ALERTA_WEBHOOK_URL`, optional `ALERTA_API_KEY`; recipient is the environment |
| `awssns` | `aws`, `AWSSNS_CREDENTIAL_SOURCE`, mode-specific `AWS_*` assignments, optional `AWSSNS_MESSAGE_FORMAT`; recipient is an SNS topic or platform endpoint ARN (see below) |
| `discord` | `DISCORD_WEBHOOK_URL`; recipients gate the single native webhook send |
| `dynatrace` | `DYNATRACE_SERVER`, `DYNATRACE_SPACE`, `DYNATRACE_TOKEN`, `DYNATRACE_TAG_VALUE`, `DYNATRACE_EVENT`, optional `DYNATRACE_ANNOTATION_TYPE`; global Events API v2 send |
| `email` | `sendmail`, `EMAIL_SENDER`, `EMAIL_PLAINTEXT_ONLY=YES`, `EMAIL_THREADING` (enabled unless `NO`); recipients are mail addresses/local users |
| `fleep` | `FLEEP_SENDER` (initially the event node); each recipient is a hook ID appended to `https://fleep.io/hook/` |
| `flock` | `FLOCK_WEBHOOK_URL`; recipients gate the single webhook send |
| `gotify` | `GOTIFY_APP_URL`, `GOTIFY_APP_TOKEN`; recipients gate the single application send |
| `ilert` | `ILERT_INTEGRATION_KEY`, optional `ILERT_API_URL`; global Event API send using an API alert source; obsolete `ILERT_ALERT_SOURCE_URL` must be empty |
| `irc` | `nc`, `IRC_NETWORK`, `IRC_PORT` (initially `6667`), `IRC_NICKNAME`, `IRC_REALNAME`; recipient is a channel |
| `kavenegar` | `KAVENEGAR_API_KEY`, `KAVENEGAR_SENDER`; recipient is a phone number |
| `matrix` | `MATRIX_HOMESERVER`, `MATRIX_ACCESSTOKEN`; recipient is a room ID |
| `messagebird` | `MESSAGEBIRD_ACCESS_KEY`, `MESSAGEBIRD_NUMBER`; recipient is a phone number |
| `msteams` | `MSTEAMS_WEBHOOK_URL`, `MSTEAMS_ICON_<STATUS>`, `MSTEAMS_COLOR_<STATUS>`; recipient replaces each `CHANNEL` in the URL; singular `MSTEAM_*` aliases are supported |
| `ntfy` | Recipient is a full topic URL; a complete `NTFY_USERNAME`/`NTFY_PASSWORD` pair takes precedence over `NTFY_ACCESS_TOKEN`; an incomplete pair is ignored |
| `opsgenie` | `OPSGENIE_API_KEY`, optional `OPSGENIE_API_URL`; global Alert API v2 send using an API Integration |
| `pd` | Recipient is the integration key; exact `USE_PD_VERSION=2` selects v2, other values select v1 |
| `prowl` | Recipients are API keys, submitted together |
| `pushbullet` | `PUSHBULLET_ACCESS_TOKEN`, optional `PUSHBULLET_SOURCE_DEVICE`; recipient is an email or `#channel-tag` |
| `pushover` | `PUSHOVER_APP_TOKEN`; recipient is a user/group key |
| `rocketchat` | `ROCKETCHAT_WEBHOOK_URL`; `#` is prepended to each legacy channel recipient |
| `slack` | `SLACK_WEBHOOK_URL`; recipient is a bare channel, `#channel`, `@user`, or `#` for the webhook default; uses the host-derived sender name and optional `images_base_url` |
| `sms` | `sendsms`; recipient is a phone number, delivered through SMS Server Tools 3 |
| `syslog` | `logger`, `logger_options`, `SYSLOG_FACILITY`; recipient syntax is `[[facility.level][@host[:port]]/]prefix`, with bracketed IPv6 supported |
| `telegram` | `TELEGRAM_BOT_TOKEN`, optional `TELEGRAM_API_URL`, `TELEGRAM_RETRIES_ON_LIMIT`; recipient is `chat` or `chat:topic` |
| `twilio` | `TWILIO_ACCOUNT_SID`, `TWILIO_ACCOUNT_TOKEN`, `TWILIO_NUMBER`; recipient is a phone number |
| `smseagle` | `SMSEAGLE_API_URL`, `SMSEAGLE_API_ACCESSTOKEN`, `SMSEAGLE_MSG_TYPE` (initially `sms`); `SMSEAGLE_CALL_DURATION` (initially `10`) and `SMSEAGLE_VOICE_ID` (initially `1`) apply only to their call modes |
| `kafka` | `KAFKA_URL`, `KAFKA_SENDER_IP`; global send |
| `signl4` | `SIGNL4_WEBHOOK_URL`; global send |

The five implemented command mappings (email, SMS, syslog, IRC and AWS SNS) use an explicit absolute executable path or
discovery in the invoking process's `PATH`. A missing discovered tool disables its method; an explicit path is
validated by the native provider and attempted during delivery.
`SEND_EMAIL=AUTO` checks sendmail availability. Child processes retain the native isolated environment, not the parent
or configuration's arbitrary variables. `logger_options` splits on spaces, tabs and newlines into literal arguments;
there is no second quoting pass, expansion or globbing. Native command delivery currently supports Linux/macOS.

Evaluated values remain literal through validation and sending, even when resembling `${env:...}` or `${file:...}`.
Use single quotes, such as `'${env:NAME}'`, to preserve these strings as literals in shell-format files. Unquoted
and double-quoted forms are parsed as unsupported shell parameter expansion and rejected.
Native field validation still applies; for example, an invalid token is rejected rather than interpreted as a secret
reference. Native YAML retains its existing lazy secret-reference behavior. The internal input-mode setting is not a
YAML option.

Available initial event scalars are `roles`, `host`, `args_host`, `when` (Unix seconds), `name`, `chart`, `context`,
`status`, `old_status`, `units`, `info`, `summary`, `value`, `old_value`, `duration`, `non_clear_duration`,
`date`, `value_string`, `old_value_string` and `status_message`. The optional [producer context](#producer-context)
adds the other Bash input scalars listed below. `date` is the event timestamp in UTC RFC3339. Supplied value strings
are preserved; otherwise they include units when the numeric value is present. Missing optional numeric facts are
empty, while zero remains `0` (or `0 <units>`). `status_message` is `needs attention`, `is critical` or `recovered`.
All these facts are initialized before configuration evaluation. There is no ambient environment import or invented
alarm/event ID. Producer-context keys are reserved event scalars even when their input is omitted.
The senders use the current native event content and provider formatting, including already documented corrections;
this is not byte-for-byte Bash output. Unknown scalar settings can serve as intermediate assignment variables but
have no independent delivery effect.

These configured behaviors are rejected when applicable to eligible delivery: nonempty `curl`/`curl_options` for
HTTP methods; non-UTF-8 `EMAIL_CHARSET` for email; configured nonempty `PATH` for command methods; nonempty
`date_format`; nonempty `images_base_url` when any eligible method is not Slack; `use_fqdn=YES`/`clear_alarm_always=YES`;
and final evaluated values of any event or producer-context scalar that differ from their initial values. Temporary
assignments are allowed if those values are restored; settings derived during evaluation retain their assignment-time expansions. Use the native event/configuration
contract to change event facts. Empty charset, `UTF-8` and `UTF8` (case-insensitive) use the existing UTF-8 email
implementation.

The remaining legacy mapping is Unix `custom_sender()` execution. Enabled, configured selections fail before
any sends once applicable recipient requirements are met, including a role recipient without a global default.
HipChat is explicitly excluded and also errors when eligible. Function bodies remain inert, including
`custom_sender()` and helpers; no original configuration file is sourced.

Six Bash defects are intentionally corrected by approval: email AUTO checks sendmail instead of curl;
PagerDuty/Prowl/ntfy can use role recipients without a global default; ntfy sends the resolved filtered recipients;
Fleep does not require the unused `FLEEP_SERVER` setting; Teams sends once per distinct resolved URL; and SNS
message assignments can use `date` and `status_message` before configuration evaluation instead of expanding them
to empty strings. Production Bash and installation remain unchanged.

### Legacy AWS SNS settings

SNS uses the existing [AWS CLI v2 sender](#aws-sns), with the same isolated environment, private temporary home,
ARN-derived region and delivery limits. Configure `aws` as an absolute path or leave it empty to discover `aws`
in the invoking process's `PATH`. Missing discovery disables SNS. An eligible SNS target requires an explicit
`AWSSNS_CREDENTIAL_SOURCE`; credentials from the invoking process, home directory or AWS profiles are not inherited.
Use **ordinary assignments**, not `export` statements:

```sh
SEND_AWSSNS=YES
aws=/usr/local/bin/aws
AWSSNS_CREDENTIAL_SOURCE=static
AWS_ACCESS_KEY_ID='REPLACE_WITH_ACCESS_KEY'
AWS_SECRET_ACCESS_KEY='REPLACE_WITH_SECRET_KEY'
DEFAULT_RECIPIENT_AWSSNS='arn:aws:sns:us-east-1:123456789012:alerts'
role_recipients_awssns[ops]="$DEFAULT_RECIPIENT_AWSSNS"
AWSSNS_MESSAGE_FORMAT="${status} on ${host} at ${date}: ${chart} ${value_string}"
```

The credential source and fields match the native provider:

| `AWSSNS_CREDENTIAL_SOURCE` | Required assignments | Optional assignments |
|---|---|---|
| `static` | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | `AWS_SESSION_TOKEN` |
| `web_identity` | `AWS_ROLE_ARN`, `AWS_WEB_IDENTITY_TOKEN_FILE` (absolute path) | `AWS_ROLE_SESSION_NAME` |
| `ecs` | `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` (path on the fixed ECS metadata endpoint) | None |
| `imds` | None; explicitly permits the standard EC2 metadata credential source | None |

Empty optional fields are omitted. Nonempty `AWS_*` assignments outside the selected mode are rejected before any
selected delivery, including wrong-mode credentials, profiles and endpoint overrides. These values stay literal:
`${env:...}` and `${file:...}` are native YAML features, not secret references in shell-format configuration.
`AWS_WEB_IDENTITY_TOKEN_FILE` retains its AWS CLI meaning: a file read by the CLI when it needs the identity token.
There is no implicit credential source or fallback to profiles. Use the explicit source that matches the deployment.

`AWSSNS_MESSAGE_FORMAT` is expanded **once, at assignment time**, by the native legacy reader. The resulting text
is sent literally; native `{{field}}` placeholders, shell-looking text and `file://` prefixes are not interpreted again.
Single-quoted references stay literal. A later variable assignment does not change an earlier message assignment;
reassign the format in the later file to use new values. Empty/unset format uses the native default body.
The subject keeps the native status wording. Messages must be valid UTF-8 and at most 262144 bytes; subjects must
be under 100 characters without controls or line breaks. There is no Bash locale/date-format processing.
Message assignments can reference the optional producer-context scalars as well as the core event scalars.

### Legacy ilert, Opsgenie and Dynatrace settings

These mappings use the same APIs, payloads and incident identity as the native YAML providers. Reading shell-format
settings does not preserve the old service-side integration setup. All three require exact `SEND_<METHOD>=YES` and
send once without recipients or recipient policies, even for `silent`/`disabled` roles.

For [ilert](#ilert), create an **API alert source** and configure its integration key. This is different from the old
Netdata-specific alert source and URL; the adapter does not extract a credential from that URL.

```sh
SEND_ILERT=YES
ILERT_ALERT_SOURCE_URL=''
ILERT_INTEGRATION_KEY='replace-with-api-source-key'
# Optional; this is the default base, before /events:
ILERT_API_URL='https://api.ilert.com/api'
```

When ilert is selected and enabled, any nonempty `ILERT_ALERT_SOURCE_URL` rejects the entire invocation before any
send, including when `ILERT_INTEGRATION_KEY` is also set. Clear or remove the old URL assignment, including any value
in an earlier loaded file. Disabled/unselected ilert ignores it. With neither a key nor an old URL, ilert is inactive.
WARNING/CRITICAL send `ALERT`; CLEAR sends `RESOLVE` with the same stable incident key.

For [Opsgenie](#opsgenie-alerts), use the key from an **API Integration**, replacing the old Netdata integration setup.
The existing setting names remain:

```sh
SEND_OPSGENIE=YES
OPSGENIE_API_KEY='replace-with-api-integration-key'
# Optional; omit or leave empty for https://api.opsgenie.com:
OPSGENIE_API_URL='https://api.eu.opsgenie.com'
```

WARNING/CRITICAL create P3/P1 alerts; CLEAR closes the stable incident alias. API acceptance is asynchronous and
uses the same acknowledgment checks as native delivery.

For [Dynatrace](#dynatrace), the API token needs `events.ingest`. The existing server/space settings form the environment
base `DYNATRACE_SERVER/e/DYNATRACE_SPACE`, followed by `/api/v2/events/ingest`. One optional trailing slash on the
server is reused as the separator; additional slashes and encoded path segments are retained. No hostname or SaaS
URL is inferred. Use native YAML's explicit `api_url` for a different environment URL layout.

```sh
SEND_DYNATRACE=YES
DYNATRACE_SERVER='https://monitor.example.test'
DYNATRACE_SPACE='environment-id'
DYNATRACE_TOKEN='replace-with-events-ingest-token'
DYNATRACE_TAG_VALUE='netdata'
DYNATRACE_EVENT='CUSTOM_INFO'
DYNATRACE_ANNOTATION_TYPE='Netdata Alarm'
```

Server, space, token, tag and event type must all be nonempty to activate delivery. An empty or unset annotation type
uses the native `Netdata Alarm` source. The tag selects HOST entities with that one literal manual tag, for example
`type(HOST),tag("netdata")`. Commas/parentheses remain quoted data; literal colons are backslash-escaped to avoid
key/value interpretation, following the [entity-selector syntax](https://docs.dynatrace.com/docs/dynatrace-api/environment-api/entity-v2/entity-selector).
Tags with boundary whitespace, leading `[`, quotes, backslashes, tildes or control characters are rejected because
the adapter cannot establish their literal selector interpretation. Use native YAML's explicit `entity_selector` for
these cases or richer targeting. Spaces are URL-encoded in the environment ID; dot segments, path separators,
percent encodings and control characters are rejected. Server URLs cannot contain credentials, queries or fragments.
The configured event type applies to every status, including CLEAR; a recovery event does not explicitly close an
existing Dynatrace problem. Native selector, event-type and content limits still apply.

### Legacy Slack and Teams routing

Slack shell-format delivery requires a [legacy incoming webhook](https://docs.slack.dev/legacy/legacy-custom-integrations/legacy-custom-integrations-incoming-webhooks/)
for runtime channel/user and sender-identity overrides. Modern app webhooks bind these settings at installation;
a URL alone does not identify which kind it is. Native YAML Slack destinations keep the modern app behavior.

```sh
SLACK_WEBHOOK_URL='https://example.test/slack-hook'
DEFAULT_RECIPIENT_SLACK='#operations @oncall #'
```

Each distinct eligible recipient produces a request. A bare name becomes `#name`, `#channel` and `@user` remain intact,
and `#` omits the channel field to use the webhook's default. The sender name is `netdata on <event node>`. The icon is
`https://registry.my-netdata.io/images/banner-icon-144x144.png`; a nonempty `images_base_url` replaces its base.
An unset or empty value uses the default. The final icon URL must be absolute HTTP(S) and at most 255 characters.
This customization is currently supported only when Slack is the sole eligible method; artwork for other providers remains pending. Slack keeps the native alert content,
status colors and escaping. These legacy overrides are internal to `send-legacy`, not new YAML fields.

Teams uses the already supported [Workflows setup](#microsoft-teams-workflows), with its trigger set to **Anyone**.
An old connector URL must be replaced with a URL created by Workflows; the adapter cannot convert it automatically.
Microsoft documents the [final connector retirement rollout in May 2026](https://devblogs.microsoft.com/microsoft365dev/retirement-of-office-365-connectors-within-microsoft-teams/).
For one destination, configure the full URL and any nonempty recipient label:

```sh
MSTEAMS_WEBHOOK_URL='https://example.test/workflow?sig=synthetic-value'
DEFAULT_RECIPIENT_MSTEAMS='operations'
```

For several destinations, use `CHANNEL` as the entire template and full URLs as recipients:

```sh
MSTEAMS_WEBHOOK_URL='CHANNEL'
DEFAULT_RECIPIENT_MSTEAMS='https://example.test/workflow-one?sig=one https://example.test/workflow-two?sig=two'
role_recipients_msteams[operations]="$DEFAULT_RECIPIENT_MSTEAMS"
```

The adapter replaces every literal `CHANNEL` with the eligible recipient, without another expansion or URL
normalization. Recipient splitting and policy suffixes follow the existing rules, so full-URL recipients cannot
contain raw commas, whitespace or `|`. Signed query strings retain their bytes and order. Teams sends once per distinct
resulting URL **after filtering**; different recipient labels for a fixed URL no longer cause repeated messages.
Existing per-recipient history requirements still apply before URL deduplication.

`MSTEAMS_ICON_WARNING/CRITICAL/CLEAR` and `MSTEAMS_COLOR_WARNING/CRITICAL/CLEAR` override the native styles. Explicit
empty values omit the icon/color. Default-status settings do not apply because the event contract permits only these
three statuses. Old singular `MSTEAM_*`, `SEND_MSTEAM`, `DEFAULT_RECIPIENT_MSTEAM` and `role_recipients_msteam` aliases
are applied after all files load: nonempty scalar aliases override plural settings; recipient aliases copy even
empty entries. Teams retains MessageCard payloads and the inline alert link.

## Command and configuration contract

- `validate --config FILE` checks one YAML document. Unknown fields, duplicate keys, unsupported provider types, and
  invalid literal settings fail. It does not resolve secrets, read event input, or make network requests.
- `send --config FILE --destination NAME` reads exactly one JSON event from stdin and delivers it to that named
  destination. Unknown destinations fail. Other configured destinations are validated but not resolved or contacted.
- `send --config FILE --role ROLE [--role ROLE ...]` uses YAML routing to select destinations. Choose either an explicit
  destination or roles; mixing the selectors fails. Roles are exact, case-sensitive names, without comma splitting,
  wildcards, or implicit role selection. They do not become fields in the webhook payload.
- All commands accept a positive `--timeout` (default `10s`) covering input/configuration reads and delivery.
  Deadline expiration, Ctrl-C, or SIGTERM stops the invocation. Run this developer tool as an ordinary user.
- Exit status is `0` when at least one delivery succeeds, no destinations are eligible, validation or the syntax check
  succeeds, or help is requested. Invalid options/configuration/input, all attempted deliveries failing, or command cancellation/timeout
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
array, with optional message/call settings described below.
PagerDuty requires `type: pagerduty` and `integration_key`; `api_version` and `api_url` are optional.
Opsgenie requires `type: opsgenie` and `api_key`, with an optional `api_url`.
Teams requires `type: msteams` and `url`, with optional `icons`/`colors` maps.
Matrix requires `type: matrix`, `api_url`, `access_token` and `room_id`.
Custom commands require `type: command` and `executable`, with optional `args` and `env`.
SMS Server Tools 3 requires `type: smstools3`, `executable` and `to`, with optional `env`.
Syslog requires `type: syslog` and `executable`, with optional facility, level, prefix, remote target and command settings.
AWS SNS requires `type: awssns`, `executable`, `target_arn` and `credential_source`, with mode-specific `env` and an optional
`message_template`.
Kafka HTTP bridges require `type: kafka`, a full `url` and literal `sender_ip`.
Email requires `type: email`, `executable` and a `recipients` array; `from`, `plain_text_only`, `threading` and `env` are optional.
IRC requires `type: irc`, `executable`, `host`, `nickname`, `realname` and `channel`; `port` and `env` are optional.
Destination names are nonsecret identifiers.
The URL must be an absolute HTTP or HTTPS URL with a host and without embedded user/password information
or a fragment. HTTP allows deliberate local or self-hosted delivery; HTTPS verifies certificates. Proxy selection
follows Go's HTTP_PROXY/HTTPS_PROXY/NO_PROXY rules.

URLs, bearer tokens, Telegram bot tokens, Pushover app/user keys, Pushbullet access tokens, Twilio account credentials,
MessageBird access keys, Gotify app tokens, ntfy credentials, ilert/PagerDuty integration keys, Alerta/Prowl/Kavenegar
and Opsgenie API keys, Dynatrace API tokens, SMSEagle/Matrix access tokens and command environment values accept literal strings or a whole `${env:VARIABLE}` or
`${file:/absolute/path}` reference.
On Windows the file operand must be an absolute Windows path. File reads use native Go I/O under the invoking user's
identity and reject content larger than 1 MiB, measured before trimming whitespace. Resolved values have surrounding
whitespace trimmed and must be nonempty. Interpolation, command execution,
and secret-store references are unsupported. Examples prefer environment references so credentials stay outside YAML.
`validate` checks reference syntax only; `send` resolves and validates eligible selected destinations before sending.
Destinations skipped by status policy do not resolve secret references.

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
destination name is considered once. Different names remain distinct destinations even if they use the same URL. In the
example, `--role sysadmin --role dba` sends to `local` and `audit` once each; an unknown role selects `local`.

All configuration and event structure is validated before delivery, even for destinations that will be skipped.
Eligible selected destinations are then attempted sequentially; a secret-resolution, HTTP, or transport error does
not stop the next destination. The total timeout still covers the whole invocation, so a slow destination can exhaust
the remaining time. Cancellation stops the
invocation even after earlier successful deliveries; no further destinations are started once it is observed.

Each completed delivery reports its quoted destination name and outcome to stderr. Filtered destinations report
`skipped: nowarn`, `skipped: noclear`, or `skipped: critical`. The final summary counts `succeeded`, `failed`, and
`skipped` separately.
Partial failure returns `0` when another delivery succeeded, matching Bash's any-success behavior; inspect the
individual results to see failures. If all attempted deliveries fail, the command returns `1` even when other
destinations were skipped. No selected destinations or all selected destinations skipped returns `0` after successful
configuration/input validation. On interruption, counts cover results reported before cancellation and do not claim
an outcome for interrupted or unstarted deliveries.

### Destination status filters

Optional `routing.policies` entries apply to named destinations for both `--destination` and `--role` sends, after
selection and deduplication. They apply to every provider, including commands. Each entry must name a configured
destination and contain only the optional boolean flags `critical`, `nowarn` and `noclear`, all defaulting to `false`.
Use YAML `true` or `false`; strings, null flags and unknown options are rejected. An empty mapping `{}` means no
filters; a null policy entry is rejected.

```yaml
version: 1
destinations:
  critical_alerts:
    type: webhook
    url: http://127.0.0.1:18080/critical
  audit:
    type: webhook
    url: http://127.0.0.1:18080/audit
routing:
  roles:
    sysadmin: [critical_alerts, audit]
  policies:
    critical_alerts:
      nowarn: true
      noclear: true
```

| Policy | WARNING | CRITICAL | CLEAR |
|---|---|---|---|
| All flags omitted or false | Send | Send | Send |
| `nowarn: true` | Skip | Send | Send |
| `noclear: true` | Send | Send | Skip |
| `nowarn: true` and `noclear: true` | Skip | Send | Skip |

In the example, a WARNING sent to `sysadmin` reaches only `audit`; a CRITICAL reaches both destinations.
A WARNING sent directly to `critical_alerts` is a successful no-op. Filtering happens before secret resolution,
HTTP requests or subprocess startup, so skipped destinations do not require their referenced secrets to be available.
A policy does not select its destination and does not affect other names pointing to the same endpoint.

The table above assumes `critical` is false. Setting `critical: true` always allows CRITICAL and allows later
WARNING/CLEAR only after CRITICAL has occurred in that alert lifecycle. The caller supplies this history as the
optional top-level JSON boolean `critical_seen_since_clear`:

| Current status | History omitted/null | History false | History true |
|---|---|---|---|
| CRITICAL | Send | Send | Send |
| WARNING | Input error | Skip: critical | Send |
| CLEAR | Input error | Skip: critical | Send |

`nowarn` and `noclear` take precedence: a WARNING suppressed by `nowarn`, or a CLEAR suppressed by `noclear`, needs
no history. Unselected destinations also impose no history requirement, including sends to only reserved
`silent`/`disabled` roles. Otherwise, missing/null history needed by any selected critical policy rejects the
**whole invocation before any delivery**, including unrelated destinations ordered earlier. No delivery results are reported. A malformed history value (anything other than a JSON
boolean or null) always fails input validation, even if no selected policy needs history.

The history is alert-global and describes the lifecycle, not successful deliveries. A destination added after CRITICAL
can receive a later WARNING/CLEAR even if it did not receive CRITICAL, or the earlier delivery failed. This deliberately
differs from Bash's per-recipient marker files. The producer must supply the closing lifecycle's history on CLEAR,
then reset it for the next lifecycle. The notifier neither persists nor infers history from `previous_status`.
`critical_seen_since_clear` is input-only: it is never forwarded in webhook, command or other provider payloads.
Global initial-CLEAR eligibility still belongs to the producer before invocation.

With the local receiver from **Build and run** listening, try the history-enabled example:

```sh
/tmp/alarm-notify validate --config examples/critical-history.yaml
/tmp/alarm-notify send --config examples/critical-history.yaml --role sysadmin < examples/critical-history.json
```

Its post-CRITICAL WARNING reaches both destinations. Changing history to `false` skips `critical_alerts` and sends to
`audit`; removing history fails before either destination sends. `examples/event.json` remains valid for configurations
whose selected policies do not require history.

### Transport results

Deliveries use POST. Twilio, MessageBird, Prowl and Kavenegar use `application/x-www-form-urlencoded`;
ntfy uses UTF-8 `text/plain`;
the other providers use `application/json`.
Generic webhooks may add `Authorization: Bearer ...`.
Generic webhooks accept HTTP 200–299; Slack, Discord, Flock and Fleep accept HTTP 200. ilert accepts HTTP 202;
SIGNL4 accepts HTTP 200, 201 or 202. Teams Workflows accepts HTTP 200–299. These providers make one attempt and
close response bodies without buffering or interpreting them. Telegram, Pushover, Pushbullet, Twilio, MessageBird,
Gotify, ntfy, Rocket.Chat, Alerta, Dynatrace, Kavenegar, SMSEagle, PagerDuty, Opsgenie and Matrix check bounded JSON acknowledgments;
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
configuration. Runtime channel/user/username/icon overrides are available through
[legacy configuration delivery](#legacy-slack-and-teams-routing), not through these native YAML destinations.

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
and v1 payload; native YAML does not read those shell settings. The [legacy adapter](#legacy-ilert-opsgenie-and-dynatrace-settings)
maps them separately to Events API v2.

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

## PagerDuty incidents

Configure one named destination per integration key. Set `api_version` to match the integration's Events API version;
it defaults to `1`, as in Bash. Both versions support WARNING/CRITICAL triggers and CLEAR resolves.
Keys and API bases accept literal, environment or file references. v1 keys must contain 32 hexadecimal characters;
v2 keys must contain 32 printable ASCII characters without whitespace, including ruleset keys.

```yaml
version: 1
destinations:
  pagerduty_v1:
    type: pagerduty
    integration_key: ${env:NOTIFY_PAGERDUTY_V1_KEY}
  pagerduty_v2:
    type: pagerduty
    api_version: 2
    integration_key: ${env:NOTIFY_PAGERDUTY_V2_KEY}
routing:
  roles:
    pagerduty_ops: [pagerduty_v1, pagerduty_v2]
```

`api_url` defaults to `https://events.pagerduty.com`; EU accounts can use `https://events.eu.pagerduty.com`.
Both official hosts require HTTPS. Custom bases can include a proxy path prefix and use HTTP for deliberate local
delivery. The notifier appends `/generic/2010-04-15/create_event.json` for v1 or `/v2/enqueue` for v2.
See PagerDuty's [Events API v1](https://developer.pagerduty.com/docs/events-api-v1/overview/),
[Events API v2](https://developer.pagerduty.com/docs/events-api-v2/overview/) and
[service regions](https://support.pagerduty.com/main/docs/service-regions).

The caller must keep `incident_id` identical across the incident's WARNING, CRITICAL and CLEAR events, and use the
same integration for recovery. Both APIs receive the lowercase hexadecimal SHA-256 digest of that opaque ID as
their correlation key (`incident_key` in v1, `dedup_key` in v2). Changes to status, timestamp or event text do not
change the key; distinct IDs produce distinct keys. This is an approved correction to Bash v2's per-event key,
which could prevent a later CLEAR from matching its trigger. No local incident history or REST lookup is required.

Both versions send a node/status/summary title and the complete native Event as details. Titles over 1024 Unicode
characters are shortened with `...`; full text remains in details. v1 adds the Netdata client and optional navigation
URL on triggers. v2 adds node as source, chart as class, timestamp with timezone, warning/critical/info severity,
and an optional **View alert** link. v2 retains the payload on CLEAR so downstream routing can inspect the same fields.

Requests over 512 KiB fail before sending without dropping event details. Success requires HTTP 200 for v1 or 202
for v2, plus a JSON acknowledgment with `status: success` and the matching correlation key. Replies are bounded to
256 KiB and the invocation deadline. There are no retries, redirects or incident-status polling. Success confirms
API acceptance; PagerDuty's integration configuration and routing rules determine the resulting incident behavior.

### Local PagerDuty exercise

Use the receiver above with synthetic keys:

```yaml
version: 1
destinations:
  v1:
    type: pagerduty
    api_url: http://127.0.0.1:18080
    integration_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  v2:
    type: pagerduty
    api_version: 2
    api_url: http://127.0.0.1:18080
    integration_key: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
routing:
  roles:
    pagerduty_ops: [v1, v2]
```

Save as `pagerduty-local.yaml` and run:

```sh
/tmp/alarm-notify send --config pagerduty-local.yaml --role pagerduty_ops < examples/event.json
```

Repeat with WARNING, CRITICAL and CLEAR events, keeping `incident_id` unchanged. The printed requests show matching
keys across all three statuses. The local receiver checks the wire contract; it does not simulate PagerDuty incidents.

## Opsgenie alerts

Use an Opsgenie **API Integration** that permits alert creation and closure, with responders configured in Opsgenie.
This provider uses the [Alert API v2](https://docs.opsgenie.com/docs/alert-api), an approved change from Bash's
Netdata-specific webhook. The integration key and service-side rules from that webhook are not automatically adapted.
Opsgenie remains supported in this migration; its announced shutdown is
[April 5, 2027](https://www.atlassian.com/licensing/opsgenie).

```yaml
version: 1
destinations:
  opsgenie_us:
    type: opsgenie
    api_key: ${env:NOTIFY_OPSGENIE_US_KEY}
  opsgenie_eu:
    type: opsgenie
    api_url: https://api.eu.opsgenie.com
    api_key: ${env:NOTIFY_OPSGENIE_EU_KEY}
routing:
  roles:
    opsgenie_ops: [opsgenie_us, opsgenie_eu]
```

Configure one named destination per API integration. `api_key` and `api_url` accept literal, environment or file
references. The key must be nonempty printable ASCII without whitespace and is sent in `Authorization: GenieKey ...`.
`api_url` defaults to `https://api.opsgenie.com`; EU accounts use `https://api.eu.opsgenie.com`. Both official hosts
require HTTPS. Custom bases may include a proxy prefix or use HTTP for deliberate local delivery.

WARNING sends a create request with priority P3, CRITICAL sends one with P1, and CLEAR sends a close request.
The alias is the lowercase hexadecimal SHA-256 digest of the caller's opaque `incident_id`. Keep that ID stable
through WARNING, CRITICAL and CLEAR; changing timestamps or text does not change the alias. Separate incident IDs
must identify separate incidents. Aliases can deduplicate across integrations in the same Opsgenie account;
different destination names or keys do not create separate incident identities.

Creation uses `/v2/alerts`; closure uses `/v2/alerts/<alias>/close?identifierType=alias`. The integration's permissions
and rules determine final alert handling. The notifier neither creates integration settings nor looks up alert IDs.

Create requests contain a node/status/summary title, the common plain-text alert content with navigation, node as
`source`, alert name as `entity`, `user: Netdata`, and the complete native Event JSON as the string `details.event`.
Close requests contain node, user, and a recovery note with the plain-text content and complete Event JSON.
Unknown values remain null and zero values remain zero in that JSON.

Titles are capped at 130 Unicode characters with `...`, preserving the full summary in description/details.
Other limits fail the destination before sending without dropping content: source 100 characters, entity 512,
description 15000, total details keys/values 8000, and recovery note 25000. Details/note counts include the serialized
Event JSON and its escapes. The entity, description and details limits apply to creation; closure uses the note limit.

Success requires HTTP 202 and a bounded JSON acknowledgment with `result: Request will be processed` and a nonempty
`requestId`. It confirms asynchronous API acceptance, not completed creation or closure; it does not establish that
an alias matched an existing alert. There are no retries, redirects or request-status polling. Replies are bounded
to 256 KiB and the invocation deadline. Existing routing, any-success results and safe diagnostics apply.

### Local Opsgenie exercise

Use the receiver above with a synthetic key:

```yaml
version: 1
destinations:
  opsgenie:
    type: opsgenie
    api_url: http://127.0.0.1:18080
    api_key: synthetic-opsgenie-key
routing:
  roles:
    opsgenie_ops: [opsgenie]
```

Save as `opsgenie-local.yaml` and run:

```sh
/tmp/alarm-notify send --config opsgenie-local.yaml --role opsgenie_ops < examples/event.json
```

Repeat with WARNING, CRITICAL and CLEAR while retaining the same `incident_id`. The receiver prints the create and
close bodies and returns synthetic acknowledgments; it does not create actual Opsgenie alerts.

## Microsoft Teams Workflows

Use a Workflows webhook with the **When a Teams webhook request is received** trigger, set to **Anyone**, and an
appropriate posting action for a MessageCard. The workflow selects the chat/channel. Keep the complete callback URL
secret; this mode must not send an Authorization header. Tenant-authenticated triggers and OAuth token acquisition
are not implemented. Follow Microsoft's [Workflows setup](https://learn.microsoft.com/en-us/connectors/teams/#when-a-teams-webhook-request-is-received).
Assign a co-owner so the workflow can remain maintained when its owner changes.

This is an approved replacement for Bash's retired Office 365 Connector setup. Configure a complete URL for each
named destination; multiple destinations preserve channel fan-out without `CHANNEL` substitution.

```yaml
version: 1
destinations:
  teams:
    type: msteams
    url: ${env:NOTIFY_TEAMS_WORKFLOW_URL}
    icons:
      warning: '⚠️'
      critical: '🔥'
      clear: '💚'
    colors:
      warning: 'FFA500'
      critical: 'D93F3C'
      clear: '65A677'
routing:
  roles:
    teams_ops: [teams]
```

`url` accepts literal, environment or file references. `icons` and `colors` are optional maps using only `warning`,
`critical` and `clear`; omitted entries use the defaults shown above. Icons are literal text without control characters;
colors are six hexadecimal digits without `#`. An explicit empty string suppresses that status's icon or color.

The MessageCard carries the status/title, common native alert facts, timestamp, summary and information. The title
preserves the node name and configured icon as plain text, following the
[Teams formatting rules](https://learn.microsoft.com/en-us/microsoftteams/platform/task-modules-and-cards/cards/cards-format).
Event text in the body is escaped as Markdown. Navigation uses a clickable inline link because Workflows does not
render MessageCard buttons. See [Microsoft's migration notice](https://devblogs.microsoft.com/microsoft365dev/retirement-of-office-365-connectors-within-microsoft-teams/).

Cards exceeding 28 KiB of serialized JSON fail before sending, without truncation. Workflow processing can impose
additional limits. Any HTTP 2xx response means the webhook accepted the request; it does not confirm that the later
workflow action posted successfully. Response bodies are closed without reading. There are no retries or redirects.

### Local Teams exercise

Use the receiver above:

```yaml
version: 1
destinations:
  teams:
    type: msteams
    url: http://127.0.0.1:18080/teams
routing:
  roles:
    teams_ops: [teams]
```

Save as `teams-local.yaml` and run:

```sh
/tmp/alarm-notify send --config teams-local.yaml --role teams_ops < examples/event.json
```

## Matrix room notices

Use a Matrix account already joined to an **unencrypted room**, with permission to send messages. Supply its access
token and the full room ID from your client, not a room alias. The provider uses the
[Client-Server API](https://spec.matrix.org/latest/client-server-api/#put_matrixclientv3roomsroomidsendeventtypetxnid).
Like Bash, it sends unencrypted `m.notice` events; it does not log in, join rooms or perform end-to-end encryption.

```yaml
version: 1
destinations:
  matrix:
    type: matrix
    api_url: https://matrix.example.org
    access_token: ${env:NOTIFY_MATRIX_ACCESS_TOKEN}
    room_id: '!room:example.org'
routing:
  roles:
    matrix_ops: [matrix]
```

`api_url` is the homeserver's client API base and may contain a reverse-proxy prefix. It accepts literal, environment
or file references, requires a nonempty HTTP(S) URL and rejects query strings, fragments and embedded credentials.
The public `matrix.org` and `matrix-client.matrix.org` hosts require HTTPS; custom bases permit deliberate local HTTP.
`access_token` also accepts literal/environment/file references and must resolve to printable ASCII without whitespace.

Use one named destination per room; route a role to several destinations for fan-out. `room_id` is a literal, opaque,
case-sensitive `!`-prefixed ID without whitespace or controls. Both older IDs containing a server name and newer IDs
without one are accepted. Path-sensitive characters are escaped when building the request URL.

Each send uses `PUT /_matrix/client/v3/rooms/<room>/send/m.room.message/<transaction>` with a fresh random transaction
ID and Bearer authentication. Repeating an invocation creates another notice, including when the incident ID and event
are unchanged. WARNING/CRITICAL/CLEAR retain their status icons; notices contain plain text plus escaped HTML with the
same native facts and navigation. Client notification settings may suppress `m.notice` push notifications.

Success requires HTTP 200 and a bounded JSON `event_id` acknowledgment. Replies are limited to 256 KiB and the
invocation deadline. Homeserver errors, including permission and complete-event size limits, fail the destination
without logging response content. The notifier does not truncate content, retry requests or follow redirects.

### Local Matrix exercise

The receiver above accepts PUT and returns a synthetic event ID:

```yaml
version: 1
destinations:
  matrix:
    type: matrix
    api_url: http://127.0.0.1:18080
    access_token: synthetic-token
    room_id: '!room:example.org'
routing:
  roles:
    matrix_ops: [matrix]
```

Save as `matrix-local.yaml` and run:

```sh
/tmp/alarm-notify send --config matrix-local.yaml --role matrix_ops < examples/event.json
```

The local receiver verifies request shape; it does not simulate Matrix membership, encryption or client rendering.

## Email through sendmail

`email` destinations submit MIME messages to an explicitly configured sendmail-compatible executable on Linux or
macOS. Configure the MTA or submission client separately, including its SMTP server, authentication and TLS settings.
The notifier does not open SMTP connections or discover a mail executable.

```yaml
version: 1
destinations:
  mail_ops:
    type: email
    executable: /usr/sbin/sendmail
    recipients:
      - Operations <ops@example.com>
      - root
    from: Netdata Alerts <netdata@example.com>
    plain_text_only: false
    threading: true
routing:
  roles:
    sysadmin: [mail_ops]
```

Save as `email.yaml`. After configuring the chosen MTA, validate and send with:

```sh
/tmp/alarm-notify validate --config email.yaml
/tmp/alarm-notify send --config email.yaml --role sysadmin < examples/event.json
```

Each recipient entry identifies one mailbox (optionally with a display name) or a local alias such as `root`.
Local aliases use letters, digits, underscores and, after the first character, dots, plus signs or hyphens. Addresses
and `from` are literal configuration, not secret references. Control characters, address lists, command/file targets,
and addresses beginning with an option are rejected. Quoted mailbox local parts and UTF-8 display names are supported;
non-ASCII mailbox addresses themselves require an MTA with internationalized address support. Recipients share a
visible `To` header. Use separate destinations when recipients should not see each other's addresses.

The optional `from` sets both the `From` header (including any display name) and the envelope sender through `-f`.
Omitting it delegates sender identity and the `From` header to the configured MTA. Local aliases also work as senders;
the MTA must qualify them into complete addresses for remote delivery. The notifier supplies submission-time `Date`
and a fresh `Message-ID`; the message body separately retains the event timestamp. The MTA may rewrite headers or
restrict sender identities according to its configuration.

The default message is UTF-8 `multipart/alternative`, with quoted-printable plain text followed by escaped HTML.
`plain_text_only: true` selects a single `text/plain` body. There is no charset override: UTF-8 correctly describes the
bytes, replacing Bash's label-only charset setting. Both modes include the native alert facts, incident ID, supplied
`duration`/`non_clear_duration` in seconds, and the optional event URL. Unknown durations are omitted; explicit zero
is retained. HTML includes a clickable navigation link. `X-Netdata-Severity`, `X-Netdata-Alert-Name`, `X-Netdata-Chart`
and `X-Netdata-Host` headers retain the available metadata; text headers use MIME encoded words.

Threading defaults on. `In-Reply-To` and `References` use a stable identifier derived from node, chart and alert,
retaining Bash's grouping across status changes and incidents. `Message-ID` stays unique per submission.
`threading: false` omits both grouping headers. These are best-effort grouping hints, not a stored conversation tree;
mail clients decide whether to group messages and may also consider subjects and recipients.

The executable receives `-t -i`, optional `-f <sender>`, and the message on stdin. `-t` takes recipients from headers;
`-i` prevents a dot-only body line from terminating input on compatible MTAs. The same explicit environment,
foreground cleanup, timeout and safe diagnostics as [custom commands](#custom-commands) apply. The command runner
supplies only its default PATH; configure `env` explicitly when the mail client needs HOME, locale, proxy or other
environment settings. Environment values accept whole environment/file secret references. For example, a
submission client using a user configuration file may need `env: {HOME: /var/lib/netdata}`. No shell is involved and
no capability probe or retry is performed. Windows validates configurations but command execution remains pending.

Exit zero means the local submission program accepted the message; it does not prove remote inbox delivery or
individual recipient acceptance. Nonzero exit, launch failure or timeout fails the destination, and normal any-success
routing applies. Child output is discarded and not included in diagnostics.

This increment uses the native Event content. Classification, source/edit/expression details, role headers,
active-alert counts/listings, cloud-specific links/artwork and richer presentation remain explicitly tracked for
later implementation or a separate removal decision in [CAPABILITIES.md](CAPABILITIES.md). Production Bash is unchanged.

## IRC through nc

`irc` destinations use an explicitly configured nc-compatible executable on Linux or macOS. Each named destination
opens one connection and sends to one channel. Use role routing for multiple channels; each connection needs a
nickname the server will accept. The notifier does not discover nc or fall back to another nickname.

```yaml
version: 1
destinations:
  irc_ops:
    type: irc
    executable: /usr/bin/nc
    host: irc.example.com
    port: 6667
    nickname: netdata-alerts
    realname: Netdata alerts
    channel: '#operations'
routing:
  roles:
    sysadmin: [irc_ops]
```

Save as `irc.yaml`, adjust the executable and server settings, then validate and send:

```sh
/tmp/alarm-notify validate --config irc.yaml
/tmp/alarm-notify send --config irc.yaml --role sysadmin < examples/event.json
```

The executable receives only the literal host and decimal port as separate arguments. The default port is `6667`;
transport remains plaintext, matching the current Bash default. Choosing another port does not enable TLS. This
increment supports guest access to channels without server passwords, SASL, channel keys or service authentication.
Configure a hostname or an unbracketed IPv4/IPv6 address, a protocol-safe ASCII nickname and a nonempty realname.
IPv6 zones such as `%eth0` or `%2` are supported; zone text must also pass the host character restrictions.
A channel starts with `#`, `&`, `+` or `!`, has at most 50 UTF-8 bytes and contains no spaces, controls or commas.
These settings are literal; only optional `env` values accept secret references. Arbitrary nc arguments are not exposed.

The session sends `NICK`/`USER`, answers registration `PING` challenges, waits for the server's welcome, then waits
for its own channel join. Notifications contain the native summary/info, node, alert, status transition, chart,
context, values, timestamp and optional URL. Newlines become comma-space, other controls are visibly escaped and
backslashes stay literal. Long messages split at UTF-8 boundaries, reserving room for the sender prefix reported
by the server so forwarded lines fit the 512-byte IRC limit. There is no automatic retry or nickname fallback.

Each message chunk is followed by a synchronization `PING`. A matching `PONG` permits the next chunk or `QUIT`;
completion also requires connection closure and a zero process exit. This confirms the exchange progressed without
an observed rejection, not that another client received or read the alert. Numeric errors `400`–`599` fail delivery,
except `422` (no MOTD). Registration/join failures, early EOF, malformed or oversized protocol frames, process errors
and timeout also fail. If both the protocol and the process fail, diagnostics retain both errors, including the
process exit status when available. An ordinary server `ERROR` closing the connection after our `QUIT` is expected.

The invocation deadline covers the whole exchange. Increase `--timeout` for servers with slow registration or flood
limits. Explicit environment, foreground process ownership and platform restrictions follow
[custom commands](#custom-commands). Protocol output is parsed with bounded buffers; raw replies and child stderr
are never logged. Cancellation closes the owned protocol pipes and waits for cleanup; inherited pipes from a command
that exits early have the same 250 ms cleanup allowance. No external IRC network is needed by the test suite.

These behaviors deliberately correct Bash's blind send-and-quit sequence, unhandled PING challenges, unchecked nc
exit status, backslash interpretation and unbounded message lines. Production Bash and its configuration are unchanged.

## Custom commands

`command` destinations execute a configured program on Linux or macOS. Use an absolute executable path and an optional
list of literal arguments. The program receives one native JSON event followed by a newline on stdin. The notifier
does not start a shell, interpolate arguments, source Bash functions or export Bash alert variables. Executable scripts
with a valid interpreter line work like other programs.

```yaml
version: 1
destinations:
  custom:
    type: command
    executable: /opt/notifications/send-alert
    args: [--recipient, ops]
    env:
      NOTIFY_TOKEN: ${env:NOTIFY_CUSTOM_TOKEN}
routing:
  roles:
    custom_ops: [custom]
```

The only default environment entry is `PATH=/usr/local/bin:/usr/bin:/bin`. The optional `env` map extends or replaces
these defaults; nothing else is inherited, including HOME, locale, proxy settings or credentials. Environment names
use letters, digits and underscores and cannot start with a digit. Values accept literals and whole environment/file
secret references; referenced values use the existing trimming/nonempty rules. Empty literal values are allowed.
NUL bytes are rejected. References are resolved only for selected destinations. Arguments are always literal, including
`${...}`, spaces and shell metacharacters. The executable is a literal path; it is never looked up through PATH.
The child inherits the notifier's working directory and operating-system identity; this is not a privilege boundary.

Commands **must stay in the foreground, wait for their children and keep them in the same process group**. Daemonizing,
detaching or handing work to a persistent background process is unsupported. On cancellation or timeout, the notifier
sends SIGKILL to the running command's process group and waits for the direct child and its stdin-copy cleanup before
exiting. Process groups are cancellation support, not a sandbox for arbitrary programs. An additional cleanup allowance
of up to 250 ms bounds an inherited stdin pipe if a command violates the foreground contract by exiting early.

Exit status zero means success; any other exit status or launch failure fails that destination. Output goes directly
to the null device, with no buffering or logging. Errors report launch/exit/cancellation categories without including
the executable, arguments, environment or child output. Commands are not retried. Arguments can still be visible in
the operating system's process listing; use the event on stdin or explicit environment for sensitive input.

`validate` checks the configuration without inspecting the executable, resolving secrets or running programs.
Windows and other platforms can validate configurations, but selected command delivery currently returns an explicit
unsupported-platform error. Native Windows process management remains a later increment.

### Local command exercise

This example appends the received JSON to a local file using `tee` (adjust its absolute path if needed):

```yaml
version: 1
destinations:
  custom:
    type: command
    executable: /usr/bin/tee
    args: [-a, /tmp/netdata-notify-events.jsonl]
```

Save as `command-local.yaml`, then run:

```sh
/tmp/alarm-notify validate --config command-local.yaml
/tmp/alarm-notify send --config command-local.yaml --destination custom < examples/event.json
cat /tmp/netdata-notify-events.jsonl
```

## SMS Server Tools 3

Install and configure [SMS Server Tools 3](https://smstools3.kekekasvi.com/index.php?p=run), including its `sendsms`
program, modem and `smsd` service. The invoking user needs the spool-directory permissions required by that installation.
This provider uses the same foreground runner and platform support as custom commands.

```yaml
version: 1
destinations:
  sms:
    type: smstools3
    executable: /usr/local/bin/sendsms
    to: '15005550009'
    env:
      LANG: C.UTF-8
routing:
  roles:
    sms_ops: [sms]
```

Supply one phone number (digits with an optional leading `+`) per named destination. Route to multiple destinations
for multiple recipients. `executable`, `to` and optional `env` are the only provider settings; arbitrary `args` are
reserved for custom commands. Choose a locale supported by the gateway host when configuring `env`.

Each delivery runs `sendsms <phone> <message>` as two literal arguments with empty stdin. The message uses Bash's
status wording (needs attention, is critical, recovered), node, optional chart and summary, with the current value and
units outside recovery. Summary underscores become spaces, and the text is truncated to 160 Unicode characters.
Recovery durations remain pending with richer shared event facts. The gateway controls encoding and SMS segmentation;
160 characters do not necessarily fit in one SMS. An exit status of zero reports tool acceptance, not handset delivery.

## Syslog

`syslog` uses an explicitly configured `logger` executable, with the same environment, foreground execution,
cancellation and safe diagnostics as custom commands. Command delivery currently runs on Linux and macOS.
The executable must support `-p facility.level` and a `--` argument terminator for local logging. Remote targets
require `-n host` and optional `-P port`, as supported by
[util-linux logger](https://man7.org/linux/man-pages/man1/logger.1.html). The macOS system logger supports local
logging, but not these remote options.

```yaml
version: 1
destinations:
  local_log:
    type: syslog
    executable: /usr/bin/logger
  remote_log:
    type: syslog
    executable: /usr/bin/logger
    facility: daemon
    level: notice
    prefix: netdata
    host: logs.example.org
    port: 1514
    args: [--tcp, --rfc3164]
routing:
  roles:
    log_ops: [local_log, remote_log]
```

`facility` defaults to `local6`; use a standard lowercase syslog facility such as `auth`, `daemon`, `mail` or
`local0` through `local7`. Without `level`, CRITICAL maps to `crit`, WARNING to `warning`, and CLEAR to `info`.
An explicit `level` overrides the severity for every status: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`,
`info` or `debug`. The logger aliases `security`, `panic`, `error` and `warn` are accepted in the corresponding fields.

`prefix` defaults to `netdata` and is literal text at the start of the message, not logger's header tag.
The message contains status, node, an RFC3339 event timestamp, optional chart and current value/units, including on
CLEAR. Null values are omitted and zero remains zero. Rendered NUL characters are rejected before launching the tool.
Other control characters and Unicode line/paragraph separators are written as visible backslash escapes (for example,
`\n`, `\r`, `\x1b` and `\u2028`) to prevent multiline or forged-looking log records. Ordinary Unicode, quotes and literal
backslashes are preserved.
The notifier passes one message argument; logger controls any wire-format limits or truncation.

Omit `host` to use local logging. Remote `host` is a literal hostname or unbracketed IPv4/IPv6 address; use the separate
integer `port` field (1–65535) when needed. IPv6 zones such as `%eth0` or `%2` are supported and must pass the host
character restrictions. An omitted port leaves the logger's default in effect. Remote logging retains
the existing plaintext behavior; no TLS transport is added in this increment.

Optional `args` preserves Bash's `logger_options` capability as a list of literal, complete logger options. These
follow the generated priority/host/port options and precede `--` and the message. Options may override earlier settings
according to the chosen logger; its supported flags determine what is available. For example, util-linux accepts
`--tcp`, `--udp`, `--rfc3164`, `--rfc5424`, `--socket-errors=on`, or `-t` followed by a tag. Optional `env` works as
described for custom commands. Neither event text nor option strings are evaluated by a shell.

Each named destination builds its own options and reports its own result. Routes deduplicate destination names and
keep the existing any-success exit rule. This avoids Bash's accumulated remote options and last-recipient-only status.
Zero exit status means logger accepted the command, not that a remote collector or local daemon durably stored it.
Some loggers can report success despite local socket errors; use supported logger options to select stricter reporting.

Save the example above as `syslog.yaml`, then validate it without writing logs:

```sh
/tmp/alarm-notify validate --config syslog.yaml
```

Running `send` with this configuration writes to the configured log targets. The module's syslog tests instead use
owned helper processes to inspect argv, environment and results without invoking a real logger.

## AWS SNS

`awssns` uses an explicitly configured [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/reference/sns/publish.html)
executable on Linux or macOS. Use a current CLI v2 installation and grant its selected identity `sns:Publish` for the
configured target. Each destination names a literal standard topic ARN or platform endpoint ARN; the region comes
from that ARN. Platform application names accept 1-256 ASCII letters, digits, underscores, hyphens or periods;
periods are not allowed in standard topic names. FIFO topics require additional publishing fields and are not supported
by this adapter.

```yaml
version: 1
destinations:
  sns_ops:
    type: awssns
    executable: /usr/local/bin/aws
    target_arn: arn:aws:sns:us-east-1:123456789012:netdata-alerts
    credential_source: static
    env:
      AWS_ACCESS_KEY_ID: ${env:NOTIFY_AWS_ACCESS_KEY_ID}
      AWS_SECRET_ACCESS_KEY: ${file:/run/secrets/notify-aws-secret}
      # Include AWS_SESSION_TOKEN when using temporary static credentials.
    message_template: |-
      {{status}} on {{node}}: {{summary}}
      {{chart}} {{value_string}}
      {{url}}
routing:
  roles:
    ops: [sns_ops]
```

Choose exactly one explicit credential source. The `env` map accepts only the variables listed for that mode:

| `credential_source` | Required `env` variables | Optional `env` variables |
|---|---|---|
| `static` | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | `AWS_SESSION_TOKEN` |
| `web_identity` | `AWS_ROLE_ARN`, `AWS_WEB_IDENTITY_TOKEN_FILE` | `AWS_ROLE_SESSION_NAME` |
| `ecs` | `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` | None |
| `imds` | None | None |

Values support literal values and whole `${env:NAME}` / `${file:/absolute/path}` references. References are resolved
only for selected destinations. A web-identity token file value must resolve to an absolute filename; the CLI reads
that token file itself. An ECS value must be a relative path beginning with `/`, fetched from the fixed ECS metadata
address `169.254.170.2`. IMDS mode explicitly enables the CLI's EC2 instance-role provider. Other modes disable it.
Missing or invalid credentials fail delivery; they do not fall back to an ambient profile.

The CLI receives a private temporary home, empty shared AWS/Boto configuration, and an allowlisted environment.
Ambient AWS credentials, profiles, credential processes, custom model directories, endpoint overrides and proxies
are not inherited. The adapter sets the region, UTF-8 file encoding, standard AWS endpoints, `NO_PROXY=*`, disabled
pager/auto-prompt, and one publish attempt. No arbitrary CLI options or environment overrides are accepted. AWS
credential providers may make their own retrieval attempts within the invocation deadline. This deliberately requires
explicit configuration even when an interactive `aws` command already works in the developer's shell.

The subject follows Bash's wording: `<node> needs attention - <alert> - <chart>` for WARNING, `is critical` for
CRITICAL and `recovered` for CLEAR. Alert underscores become spaces; an absent chart is omitted. Subjects must contain
fewer than 100 Unicode characters and no control characters or line breaks. Invalid subjects fail before launch.
The default body is `<status> on <node> at <RFC3339 timestamp>: <chart> <value> <units>`, omitting absent chart/value
facts. CLEAR includes the current value, and zero is retained. Bodies must be nonempty UTF-8 and at most 262144 bytes;
there is no automatic truncation or protocol-specific message structure.

`message_template` customizes the body with `{{field}}` placeholders. Every native Event field is available:
`version`, `incident_id`, `timestamp`, `node`, `alert`, `chart`, `context`, `status`, `previous_status`, `summary`,
`info`, `value`, `previous_value`, `duration`, `non_clear_duration`, `units`, and `url`.
Durations render as whole seconds. Additional fields are `status_message` (the subject's status
wording), `value_string` and `previous_value_string` (number plus units). Missing optional values become empty strings.
Whitespace around a placeholder name is ignored. Write `{{{{` to emit a literal `{{`. Unknown or unclosed placeholders
fail configuration validation. Substitution happens once: inserted text is not evaluated, shell syntax is literal,
and templates do not resolve secret references. For shell-format message assignments, use the
[legacy SNS mapping](#legacy-aws-sns-settings). Richer Bash facts remain later work.

The adapter sends `TargetArn`, `Subject` and `Message` as JSON on stdin via `--cli-input-json file:///dev/stdin`.
This avoids putting event text on the command line or treating text beginning with `file://` as a file to read.
The CLI's output is discarded; exit status zero means the publish call succeeded, not that subscribers received it.
Failure or cancellation may occur after AWS accepted a publish, so the adapter never retries it automatically.
The foreground process cleanup described for custom commands applies; its private home is removed before the
invocation returns, including failures and cancellation.

Save the example as `awssns.yaml`, then validate without reading secrets, launching the CLI or contacting AWS:

```sh
/tmp/alarm-notify validate --config awssns.yaml
```

`send` publishes to the configured AWS target. The module tests use owned helper executables and synthetic credentials;
they do not contact AWS or metadata endpoints. Windows builds support validation but do not yet run command providers.

## Kafka HTTP bridge

`kafka` posts to a configured HTTP bridge URL. It does not connect to Kafka brokers or implement a specific vendor's
REST API. The receiver must accept the document below and return HTTP **204**; other responses, including 200, 201
and 202, fail. Acceptance by the bridge does not confirm downstream Kafka delivery. Requests are not retried and
redirects are not followed.

```yaml
version: 1
destinations:
  bridge:
    type: kafka
    url: http://127.0.0.1:18080/kafka
    sender_ip: 192.0.2.1
routing:
  roles:
    sysadmin: [bridge]
```

`url` is a full HTTP(S) URL, literal or a whole `${env:KAFKA_URL}` / `${file:/absolute/path}` reference. References are
resolved only for selected destinations. URLs may contain a path/query, but not user information or fragments.
Treat URLs containing credentials as secrets. `sender_ip` is a required literal IPv4 or IPv6 address without a port,
network prefix or zone. It labels the message's `host_ip`; it does not select the outbound interface or resolve `node`.

The request uses `Content-Type: application/json`. Its fixed fields retain the Bash bridge's names:

| Bridge field | Native source |
|---|---|
| `host_ip` | Destination `sender_ip` |
| `when` | `timestamp` converted to Unix seconds; subsecond precision is discarded |
| `name`, `chart` | `alert`, `chart` |
| `status`, `old_status` | `status`, `previous_status` |
| `value`, `old_value` | `value`, `previous_value` |
| `duration`, `non_clear_duration` | Optional event duration facts, in seconds |
| `units`, `info` | `units`, `info` |

Missing numeric values/durations are JSON `null`; explicit zero remains zero. Missing strings are empty. Quotes,
newlines, Unicode and other string content are JSON encoded. This intentionally corrects Bash's unquoted keys,
unescaped interpolation and default form content type. A bridge depending on that old syntax needs adjustment.

Save this example as `kafka.yaml`. With the local receiver from **Build and run** listening, validate and send:

```sh
/tmp/alarm-notify validate --config kafka.yaml
/tmp/alarm-notify send --config kafka.yaml --role sysadmin < examples/event.json
```

That event has no duration facts, so both are sent as `null`. To exercise measured durations, add `"duration": 0` and
`"non_clear_duration": 123` to the event document. The notifier accepts these facts from its caller; it does not infer
them from local state or track history.

## Event document

The webhook and custom command receive the typed event as JSON. `version` must be `1`. Required fields are `incident_id`, `timestamp`
(RFC 3339), `node`, `alert`, `status`, and `summary`. `incident_id` is an opaque stable incident identifier supplied by
the caller. Current statuses are `WARNING`, `CRITICAL`, and `CLEAR`.

Optional fields are `chart`, `context`, `previous_status`, `info`, `value`, `previous_value`, `duration`,
`non_clear_duration`, `units`, and `url`.
`previous_status` accepts the three current statuses plus `UNINITIALIZED`, `UNDEFINED`, and `REMOVED`.
`url` is a caller-supplied alert navigation link: absolute HTTP(S), without embedded user/password information;
fragments are allowed. Use a URL suitable for disclosure in notifications. The notifier does not fetch this link.
Generic webhooks include `url` when it is supplied and omit it otherwise.
Values are finite JSON numbers or null; missing values are sent as null and zero remains zero. Unknown fields and
trailing documents are rejected. Strings are encoded as JSON, including quotes, newlines, and Unicode.

`duration` is the time spent in the previous alert state; `non_clear_duration` is the total elapsed time the alert
is/was non-clear. Both are caller-supplied whole seconds from 0 through 4294967295, matching the Agent's unsigned
32-bit notification arguments. Negative, fractional, exponent-form and string values are rejected. Omitted or null
durations mean unknown and are omitted from the public Event JSON; explicit zero is retained. They are public facts:
when supplied they also appear in webhook/custom-command input and Alerta/Opsgenie raw event details, and are available
to SNS message templates. Other providers' duration presentation remains pending in the capability inventory.

The input additionally accepts `critical_seen_since_clear`, a routing fact kept outside the public Event. A JSON
boolean supplies known history; omitted/null means unknown. It is required only when a selected `critical` policy
needs it, as described under **Destination status filters**, and never appears in provider payloads. No initial-CLEAR
eligibility or history is inferred by the notifier; both belong to the producer. The optional `producer_context`
object below also stays outside the public Event.

### Producer context

Both `send` and `send-legacy` accept an optional top-level `producer_context` object alongside the event fields.
It carries additional caller-supplied facts for legacy settings and the future Bash custom-function adapter.
Omitting it, using `null`, or using `{}` keeps existing input valid. All members are optional; unknown members or
wrong JSON types reject the entire invocation before delivery, even when no destination needs these facts.
Repeated `producer_context` fields and duplicate member names are rejected, including case-insensitive or
JSON-escape-equivalent spellings.

| Members | JSON type | Meaning / Bash scalar names |
|---|---|---|
| `unique_id`, `alarm_id`, `event_id` | Integer or null | Producer notification, alarm and per-alarm event IDs; same scalar names |
| `src` | String or null | Alert configuration source location |
| `value_string`, `old_value_string` | String or null | Caller-formatted current and previous values |
| `calc_expression`, `calc_param_values` | String or null | Alert expression and its evaluation details |
| `total_warnings`, `total_critical` | Integer or null | Counts of other active WARNING/CRITICAL alerts, excluding this alarm |
| `total_warn_alarms`, `total_crit_alarms` | String or null | Producer's list text; the Agent uses comma-separated `name=Unix-timestamp` entries |
| `classification`, `component`, `type` | String or null | Alert classification, component and type |
| `edit_command_line` | String or null | Configuration-edit command text, carried as data |
| `child_machine_guid`, `transition_id` | String or null | Producer host and transition identities, carried as opaque strings |

Integers must be from `0` through `4294967295`, without fractions, exponents or quotes. Omitted/null numbers are
unknown and expand to empty strings in legacy settings; explicit zero expands to `0`. IDs are not inferred from
`incident_id` and do not replace the stable identity used by native incident providers. Counts and list strings
are supplied independently; the notifier does not compute or reconcile them.

Strings preserve whitespace, Unicode and shell-looking text; NUL is rejected because shell variables cannot carry it.
Omitted/null strings become empty, except `value_string` and `old_value_string`: those fall back to the existing
numeric-value-plus-units formatting. An explicitly supplied empty formatted string stays empty. Supplied formatted
values are accepted even when the corresponding numeric value is unknown.

For example, add this member to `examples/event.json`:

```json
"producer_context": {
  "unique_id": 42,
  "alarm_id": 7,
  "event_id": 3,
  "value_string": "42.50 C",
  "total_warnings": 2,
  "total_critical": 0,
  "calc_expression": "$this > 40"
}
```

Then a legacy assignment such as `AWSSNS_MESSAGE_FORMAT="$alarm_id: $value_string ($calc_expression)"` expands to
`7: 42.50 C ($this > 40)` once, before delivery. No expression or command text is executed or expanded again.

Producer context is input-only. It is excluded from webhook JSON, command stdin, provider raw-event attachments and
native message templates. Legacy settings can explicitly include its scalars in supported message assignments.
It does not change ordinary provider formatting. The Agent does not yet generate this JSON; development callers
supply it. Bash custom execution and derived presentation/helpers remain the next increment.

## Internal packages

All packages belong to this standalone module. Providers implement a single delivery contract:

```go
type Sender interface {
    Send(context.Context, event.Event) error
}
```

A sender represents one configured destination. Success means the provider accepted the request; it does not promise
that a human received the notification. The context carries the whole invocation deadline.
The engine receives a `notifier.Notification` containing the public Event, input-only policy facts and producer context.
It checks all selected policies before delivery, then passes only the public Event to each eligible sender.

| Package | Ownership |
|---|---|
| `internal/app` | CLI options, event input, output and invocation resources |
| `internal/notifier` | Sender contract, input-only notification facts, routing, status/history policies, sequential fan-out and delivery results |
| `internal/config` | Strict YAML document decoding and typed factories supplied through an explicit registry |
| `internal/config/field` | Reusable configuration scalar validation, including strict integers |
| `internal/providers` | Explicit built-in registration |
| `internal/providers/<provider>` | Typed configuration, validation, rendering, delivery and acknowledgment for one provider |
| `internal/event` | Public notification data and JSON field tags; internal routing facts belong separately |
| `internal/message` | Common fields, plain text, control escaping and shared severity colors |
| `internal/secret` | Whole-value reference syntax and lazy environment/file resolution |
| `internal/httpclient` | Request construction, client settings, safe errors and bounded response reads |
| `internal/commandexec` | Command options/environment, foreground processes, interactive sessions and cleanup |
| `internal/testutil` | Provider-independent test fixtures and helpers |

To add a provider, create its package with a typed `Config`, validating `New` constructor and `Send` method, register
its constructor in `providers.Builtin`, and add its tests and documentation. The engine and other providers need no
changes. Provider packages do not import each other or the application. Payloads, endpoint rules, acknowledgment
checks and retries stay with the provider; shared mechanisms contain no provider dispatch.

Constructors validate all configured destinations without resolving secrets or performing delivery. Configuration
keeps its flat YAML shape and supports aliases and merges. A provider rejects undeclared fields even when their
value is empty, zero or null. Typed factories decode the original YAML node graph to preserve cross-destination
aliases while checking field names, duplicate keys and scalar types. Decoder diagnostics never echo input values.

The application owns one HTTP client and command runner per invocation. Providers close response bodies; the
application closes idle HTTP connections. Secrets resolve only for eligible sends, into local copies of each
provider's configuration. Constructors retain their own copies of mutable options.

Cancel the invocation context before calling `commandexec.Runner.CloseAndWait`, which rejects new child work and
waits for admitted work to finish cleanup. The runner owns process-group cancellation, session I/O and private
command homes. Blocked caller-owned input or secret-file reads remain outside that shutdown wait. The application
alone writes CLI output, including after cancellation.

Payload fixtures and private renderer/response/config tests live beside their provider. App tests exercise real
YAML-to-HTTP and command paths; engine tests exercise routing and filtering with recording senders. The registry
checks the complete provider inventory and field isolation, including explicitly empty foreign fields.

## Validation

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

Tests use tables keyed by case name, local HTTP receivers and owned helper processes; no provider account or credentials are needed.
CI runs this module's tests on Linux. To check Windows compilation locally, use:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/alarm-notify.exe .
```

A Windows cross-build checks compilation, not native Windows runtime behavior.
