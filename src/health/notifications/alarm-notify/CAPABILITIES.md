# Notification migration inventory

The functional baseline is `../alarm-notify.sh.in` and `../health_alarm_notify.conf`. This inventory tracks a parallel
implementation; it does not change the active Bash notifier. Existing functionality is implemented incrementally,
and questionable behavior requires an explicit decision before being changed or dropped. The ineffective Discord
channel-name field and repeated-request loop are omitted by explicit approval; real channel routing uses webhook URLs.
Telegram's optional retries honor the server's requested delay instead of Bash's fixed one second, also by explicit approval.
Flock's ineffective channel-name loop is also omitted by explicit approval; separate webhook URLs select its channels.
ilert uses the Event API and an API alert source by explicit approval. SIGNL4 uses stable incident identity for recovery,
correcting Bash's changing per-event identity by explicit approval. Dynatrace uses Events API v2 by explicit approval;
its configured event-type behavior is retained, including CLEAR without explicit problem closure.
SMSEagle automatically selects GSM-7 or Unicode encoding for SMS/MMS by explicit approval, preserving the original text.
PagerDuty uses stable incident identity in both API versions by explicit approval, correcting Bash v2's per-event key.
Opsgenie uses Alert API v2 with an API Integration by explicit approval, retaining support while the service is available.
Teams uses Workflows MessageCards by explicit approval, with full URLs per destination and inline navigation replacing
unsupported buttons; configurable status icons/colors are retained.
Custom commands require foreground execution and waiting for their children by explicit approval; detached/background
work is unsupported. Native executable/argv/JSON input replaces Bash function/global syntax; legacy adapters remain later work.
SNS preserves message customization through native Event placeholders by explicit approval; Bash shell syntax and
richer event facts remain later work. Kafka HTTP bridges use valid JSON with the existing payload field names and
HTTP 204 acknowledgment by explicit approval, correcting Bash's hand-built JSON-like body and form content type.
Optional duration facts preserve unknown versus zero and are available to public Event consumers and SNS templates.
Email uses correct UTF-8 MIME and separate threading headers by explicit approval, replacing Bash's ineffective
charset-label override. Current native facts and durations ship first; richer email content remains tracked below
for later implementation or explicit disposition, not permanently excluded.
IRC retains nc and plaintext defaults by approval, with corrected registration/join/PING handling, truthful process
and protocol failures, literal text escaping and safe UTF-8 splitting instead of Bash's blind send-and-quit behavior.
HipChat is excluded from the Go migration by explicit approval following its
[end of life](https://www.atlassian.com/partnerships/slack/faq); production Bash remains unchanged.

## Working increments

- Standalone developer build, explicit YAML configuration, JSON input, and one selected generic webhook delivery.
- Configuration validation, literal/environment/file secrets, request timeout/cancellation, and safe diagnostics.
- Central role-to-destination routing, defaults for unmapped roles, explicit suppression, reserved roles, and
  deduplication by destination name. Sequential fan-out records individual results and preserves any-success exits.
- Destination `nowarn`/`noclear` policies for direct and role-based delivery, with skipped results, no secret/transport
  work for filtered destinations and successful all-skipped no-ops.
- Modern Slack app webhooks with status colors, plain-text alert content and an optional navigation link.
- Native Discord webhooks with status-colored embeds, navigation, confirmed delivery and one URL per named destination.
- Telegram bot messages with chat/topic routing, escaped HTML, silent recovery, disabled previews, custom API bases,
  JSON acknowledgments and optional rate-limit retries within the invocation deadline.
- Pushover app/user or group delivery with status priorities, escaped HTML, safe shortening, navigation,
  custom API bases and JSON acknowledgments.
- Pushbullet email/channel recipients, optional source device, native links/notes, access-token secrets,
  custom API bases and created-push acknowledgments.
- Twilio account SID/Auth Token, native sender/recipient fields, form-encoded text, custom API bases and created-message
  acknowledgments, with no automatic retries or shortening.
- MessageBird originator/recipient SMS with access-key secrets, automatic character encoding, custom API bases and
  created-message acknowledgments; no automatic retries or shortening.
- Gotify application-token messages and ntfy topic publishing with status priorities, ntfy tags/navigation,
  anonymous/Basic/token authentication, secret references and server acknowledgments.
- Rocket.Chat webhooks with channel overrides, host alias, status attachments and JSON acknowledgment checks;
  Flock channel webhooks with sender/status attachments; Fleep conversation webhooks with custom sender names.
- ilert API alert-source events and SIGNL4 team webhooks, including stable alert/recovery correlation, current event
  content/navigation and selected-only credential resolution.
- Alerta environments, correlation/severity, optional API-key auth and suppression handling; Dynatrace Events v2
  entity targeting, configurable type/source and per-event result checks.
- Prowl push with batched keys, Bash priorities, navigation and XML acknowledgments; Kavenegar HTTPS SMS with
  sender/recipient, intact native text and JSON API acceptance checks.
- SMSEagle SMS, text-only MMS, ring, TTS and advanced TTS with recipient batches, automatic encoding, call controls,
  appliance API/token references and per-recipient queued acknowledgments.
- PagerDuty Events API v1/v2 trigger/resolve events with stable correlation, integration-key secrets, custom API bases,
  native event details/navigation and matching JSON acknowledgments.
- Opsgenie Alert API v2 create/close requests with Bash priorities, stable aliases, native content/navigation,
  API-key/base references and asynchronous request-acceptance checks.
- Teams Workflows MessageCard webhooks with configurable status icons/colors, plain titles, escaped body text and inline links.
- Matrix v3 room notices with bearer-token/base references, plain text and escaped HTML, fresh transaction IDs and
  event-ID acknowledgments; unencrypted room delivery matches the Bash capability.
- Custom foreground commands with absolute executables, literal argv, native JSON stdin, explicit environment/secret
  references, discarded output, safe exit diagnostics and process-group cancellation on Linux/macOS.
- SMS Server Tools 3 through configured sendsms, one phone per destination, Bash status wording and a 160-character
  limit, with existing routing/fan-out and command cleanup. Recovery duration presentation remains pending.
- Syslog through configured logger, with local/remote targets, facility/status severity or fixed level, message prefix,
  literal extra options and explicit environment. Each destination owns its options/results; shared routing and
  foreground command cleanup apply. Remote tool support and wire format remain properties of the configured logger.

- AWS SNS through configured AWS CLI v2, standard topics/platform endpoints, ARN-derived region, Bash subject/default
  body wording and native message templates. Explicit credential sources, private CLI home/configuration, stdin JSON
  and one publish attempt use the foreground command lifecycle; exit status indicates publish acceptance.
- Kafka HTTP bridge delivery with full URLs, literal sender IPs, the existing field names, valid JSON, supplied duration
  facts and HTTP 204 acknowledgment. Shared routing, selected-only secret resolution and safe HTTP handling apply.

- Email through sendmail with literal recipient lists/local aliases, optional sender, UTF-8 plain/HTML modes,
  chart/alert/node threading, available metadata headers and native content/durations. Explicit environment,
  safe diagnostics and owned foreground cleanup reuse the command lifecycle.

- IRC channel delivery through nc, with explicit host/port/nickname/realname/channel/environment, registration and
  join confirmation, PING responses, bounded protocol parsing, safe text splitting and owned process cleanup.

## Bash providers

All 31 `send_*` functions are accounted for, including providers absent from integration metadata. HipChat is
explicitly excluded; each remaining provider's current API and Bash behavior must be checked when its implementation
is scoped. This is not a claim that all legacy remote services remain available.

| Provider | Bash function | Go migration |
|---|---|---|
| Email | `send_email` | sendmail adapter, recipients/local aliases, optional sender, UTF-8 MIME plain/HTML, threading, native content/durations and available metadata headers implemented; classification, source/edit/expression facts, role headers, active-alert inventories/counts and cloud-specific presentation remain pending by approved staging |
| Pushover | `send_pushover` | Implemented with app/user or group keys, HTML, priorities, timestamp, navigation, safe shortening and acknowledgment checks |
| Pushbullet | `send_pushbullet` | Email/channel targets, access token, optional source device, links/notes and acknowledgment checks implemented |
| Kafka HTTP bridge | `send_kafka` | HTTP POST, full URL/sender IP, complete bridge payload and exact HTTP 204 acknowledgment implemented; valid JSON/content type is an approved correction |
| PagerDuty | `send_pd` | Events API v1 (default) and v2, per-integration routing, trigger/resolve with stable incident keys, event details/navigation, API base/key references and JSON acknowledgment checks implemented |
| Twilio | `send_twilio` | Account/sender/recipient text delivery, form encoding, Basic auth, custom API bases and acknowledgment checks implemented |
| HipChat | `send_hipchat` | Excluded by explicit decision after service/product end of life; Bash retained |
| MessageBird | `send_messagebird` | Originator/recipient SMS, AccessKey auth, automatic character encoding, custom API bases and acknowledgment checks implemented |
| SMSEagle | `send_smseagle` | API v2 SMS/MMS/ring/TTS/advanced TTS, recipient arrays, automatic encoding, duration/voice controls and queued batch checks implemented |
| Kavenegar | `send_kavenegar` | HTTPS v1 SMS, API-key secrets, sender/recipient, native plain text, custom API base and JSON acceptance checks implemented |
| Telegram | `send_telegram` | Implemented with chats/topics, bot-token secrets, custom API bases, silent CLEAR, disabled previews and optional rate-limit retries; server-directed retry timing is an approved correction |
| Microsoft Teams | `send_msteams` | Approved Workflows MessageCard setup, full webhook URLs, status icon/color overrides, native content and inline navigation implemented; HTTP acceptance does not confirm later workflow actions |
| Slack | `send_slack` | Modern app webhooks implemented; legacy channel/user/username/icon overrides pending by explicit staged-delivery decision |
| Rocket.Chat | `send_rocketchat` | Webhook URL, optional channel/user override, host alias, status attachments, alert facts/navigation and acknowledgment checks implemented; extended artwork/presentation pending |
| Alerta | `send_alerta` | API base, optional key, environments, node/chart correlation (including httpcheck), severity/recovery, current content/raw Event and acknowledgment/suppression checks implemented; extended legacy facts pending |
| Flock | `send_flock` | Channel webhook URLs, host sender name, status attachments and alert facts/navigation implemented; ineffective channel loop omitted by explicit decision; extended artwork/presentation pending |
| Discord | `send_discord` | Native webhooks implemented; ineffective channel-name loop omitted by explicit decision; additional artwork/presentation pending |
| Fleep | `send_fleep` | Conversation webhook URLs, custom sender and JSON message content implemented |
| Prowl | `send_prowl` | API-key batches, Bash priorities, native event/description/navigation, byte limits, custom API base and XML acknowledgments implemented |
| IRC | `send_irc` | nc-backed guest channel delivery, explicit connection settings, native alert text, registration/join/PING handling, safe UTF-8 splitting and process/protocol failure checks implemented; plaintext default retained |
| AWS SNS | `send_awssns` | AWS CLI v2 adapter with ARN-derived region, status subject/body, native message templates, explicit static/web-identity/ECS/IMDS credentials, isolated configuration and command cleanup implemented on Linux/macOS |
| Matrix | `send_matrix` | Client-Server v3 PUT, unencrypted m.notice, opaque room IDs, token/base references, fresh transactions, plain/escaped HTML content and event-ID acknowledgments implemented |
| Syslog | `send_syslog` | logger adapter with local/remote host/port, facility/severity overrides, prefix, extra options, explicit environment and command cleanup implemented; native destinations isolate options and use existing any-success aggregation |
| SMS Server Tools 3 | `send_sms` | sendsms command adapter, explicit path/env, named recipients, status-aware text, 160-character truncation and exit-status acceptance implemented on Linux/macOS; recovery duration presentation pending |
| Dynatrace | `send_dynatrace` | Approved Events API v2, API token/base, entity selector, configured event type/source, current alert content and per-event result checks implemented; CLEAR retains configured type without explicit problem closure |
| Opsgenie | `send_opsgenie` | Approved Alert API v2/API Integration, P3/P1 creation and CLEAR closure by stable alias, API key/base references, native Event content and asynchronous acknowledgment checks implemented |
| Gotify | `send_gotify` | Application-token auth, JSON messages, status priorities, custom API base and acknowledgment checks implemented |
| ntfy | `send_ntfy` | Topic URLs, anonymous/Basic/token auth, text messages, priorities/tags/navigation and acknowledgment checks implemented |
| ilert | `send_ilert` | Event API/API alert source, integration key/API base, ALERT/RESOLVE, stable encoded incident key, complete event details and navigation implemented; extended artwork/presentation pending |
| SIGNL4 | `send_signl4` | Team webhook URL, current event content/navigation and new/resolved events with stable incident_id implemented; Bash per-event identity corrected by explicit decision; extended artwork/presentation pending |
| Custom | `send_custom` / `custom_sender` | Native JSON webhook and foreground command/argv/env destinations implemented; detached work excluded by approval, old Bash function/global adapters and richer event facts remain later work |

## Other functionality

| Area | Functional baseline | Go migration |
|---|---|---|
| Routing | Multiple roles, provider recipients, fallback/default recipients, disabled destinations, duplicate handling | Central named-destination routing/defaults/suppression/deduplication implemented; provider-specific native targets implemented; legacy recipient syntax/adapters remain pending |
| Status policy | Supported transitions, initial CLEAR, `critical`, `nowarn`, `noclear`, and modifier combinations | Native destination `nowarn`/`noclear` booleans and their combination implemented for direct/role sends; `critical` and its combinations remain pending. Initial-CLEAR eligibility/override belongs to future producer integration |
| Lifecycle history | Per-recipient tracking used by critical-only notification policies | Pending; ownership changes require a semantic comparison |
| Rendering | Provider content, titles, links, timestamps, durations, values, summaries, email MIME/threading | Slack, Discord, Telegram, Pushover, Pushbullet, Twilio, MessageBird, Gotify, ntfy, Rocket.Chat, Flock, Fleep, ilert, SIGNL4, Alerta, Dynatrace, Prowl, Kavenegar, SMSEagle, PagerDuty, Opsgenie, Teams and Matrix content/link/status formatting plus SMS Server Tools/syslog compact text, SNS subject/body templates and Kafka bridge payloads plus email MIME/threading/native content and IRC plain text implemented; optional duration facts are available, with richer presentation pending |
| Provider configuration | Credentials, endpoints, recipient-specific settings, enable/auto detection, local tool settings | Generic webhook, Slack and Discord URLs; Telegram bot/chat/topic/API/retry; Pushover app/user/API; Pushbullet token/recipient/source/API; Twilio account/token/from/to/API; MessageBird key/originator/recipient/API; Gotify app/API; ntfy URL/auth; Rocket.Chat URL/channel; Flock URL; Fleep URL/sender; ilert integration key/API; SIGNL4 URL; Alerta API/key/environment; Dynatrace API/token/selector/type/source; Prowl API/key batch; Kavenegar API/key/sender/recipient; SMSEagle API/token/recipients/mode/duration/voice; PagerDuty integration key/API/version; Opsgenie API/key; Teams URL/icons/colors; Matrix API/token/room and custom command/sendsms executable/args/env/recipient and syslog facility/level/prefix/host/port and SNS target/credential/template and Kafka URL/sender-IP settings plus email executable/recipients/sender/modes/threading/env and IRC executable/host/port/nickname/realname/channel/env implemented; legacy configuration and automatic tool/provider discovery remain pending |
| Results | Per-target failures and Bash's any-success invocation result | Implemented for all current Go providers, with separate skipped counts and successful all-skipped no-ops; IRC synchronization and subprocess acceptance do not guarantee recipient delivery |
| Utilities | Synthetic test transitions and configured-method reporting (`dump_methods`) | Pending; `validate` is available |
| Logging | Alert-specific structured fields and operational diagnostics | Pending except basic safe command diagnostics |
| Integration | Agent invocation, legacy config adapters, analytics, installation, operator docs, production cutover | Later milestone |

The first twenty-five implementation PRs cover the foundation, routing/fan-out, Slack app webhooks, Discord, Telegram, Pushover,
Pushbullet, Twilio, MessageBird, Gotify, ntfy, Rocket.Chat, Flock, Fleep, ilert, SIGNL4, Alerta, Dynatrace, Prowl, Kavenegar,
SMSEagle, PagerDuty, Opsgenie, Teams, Matrix, custom commands, SMS Server Tools 3, syslog, AWS SNS, Kafka HTTP bridges,
email, IRC and stateless destination filters. Windows command delivery remains a later platform increment; configuration validation and the existing HTTP providers still compile for Windows.
Related providers may share small PRs.
Legacy Slack override support remains pending; choosing modern webhooks first does not permanently remove that functionality.
The internal redesign now precedes the remaining functional increments. Shared event, formatting, secret, HTTP and
process mechanisms have separate packages; the next step gives all providers typed configuration and package ownership.
This cleanup preserves the implemented functionality. Remaining capabilities stay pending until delivered or explicitly excluded.
