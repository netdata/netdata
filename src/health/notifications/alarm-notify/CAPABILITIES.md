# Notification migration inventory

The functional baseline is `../alarm-notify.sh.in` and `../health_alarm_notify.conf`. This inventory tracks a parallel
implementation; it does not change the active Bash notifier. Existing functionality is implemented incrementally,
and questionable behavior requires an explicit decision before being changed or dropped. The ineffective Discord
channel-name field and repeated-request loop are omitted by explicit approval; real channel routing uses webhook URLs.
Telegram's optional retries honor the server's requested delay instead of Bash's fixed one second, also by explicit approval.

## Working increments

- Standalone developer build, explicit YAML configuration, JSON input, and one selected generic webhook delivery.
- Configuration validation, literal/environment/file secrets, request timeout/cancellation, and safe diagnostics.
- Central role-to-destination routing, defaults for unmapped roles, explicit suppression, reserved roles, and
  deduplication by destination name. Sequential fan-out records individual results and preserves any-success exits.
- Modern Slack app webhooks with status colors, plain-text alert content and an optional navigation link.
- Native Discord webhooks with status-colored embeds, navigation, confirmed delivery and one URL per named destination.
- Telegram bot messages with chat/topic routing, escaped HTML, silent recovery, disabled previews, custom API bases,
  JSON acknowledgments and optional rate-limit retries within the invocation deadline.
- The generic webhook is an initial development capability. It does not complete migration of Bash's custom sender.

## Bash providers

All 31 `send_*` functions enter the baseline, including providers absent from integration metadata. Each provider's
current API and Bash behavior must be checked when its implementation is scoped. This is not a claim that all legacy
remote services remain available.

| Provider | Bash function | Go migration |
|---|---|---|
| Email | `send_email` | Pending |
| Pushover | `send_pushover` | Pending |
| Pushbullet | `send_pushbullet` | Pending |
| Kafka HTTP bridge | `send_kafka` | Pending |
| PagerDuty | `send_pd` | Pending |
| Twilio | `send_twilio` | Pending |
| HipChat | `send_hipchat` | Pending |
| MessageBird | `send_messagebird` | Pending |
| SMSEagle | `send_smseagle` | Pending |
| Kavenegar | `send_kavenegar` | Pending |
| Telegram | `send_telegram` | Implemented with chats/topics, bot-token secrets, custom API bases, silent CLEAR, disabled previews and optional rate-limit retries; server-directed retry timing is an approved correction |
| Microsoft Teams | `send_msteams` | Pending |
| Slack | `send_slack` | Modern app webhooks implemented; legacy channel/user/username/icon overrides pending by explicit staged-delivery decision |
| Rocket.Chat | `send_rocketchat` | Pending |
| Alerta | `send_alerta` | Pending |
| Flock | `send_flock` | Pending |
| Discord | `send_discord` | Native webhooks implemented; ineffective channel-name loop omitted by explicit decision; additional artwork/presentation pending |
| Fleep | `send_fleep` | Pending |
| Prowl | `send_prowl` | Pending |
| IRC | `send_irc` | Pending |
| AWS SNS | `send_awssns` | Pending |
| Matrix | `send_matrix` | Pending |
| Syslog | `send_syslog` | Pending |
| SMS Server Tools 3 | `send_sms` | Pending |
| Dynatrace | `send_dynatrace` | Pending |
| Opsgenie | `send_opsgenie` | Pending |
| Gotify | `send_gotify` | Pending |
| ntfy | `send_ntfy` | Pending |
| ilert | `send_ilert` | Pending |
| SIGNL4 | `send_signl4` | Pending |
| Custom | `send_custom` / `custom_sender` | Pending |

## Other functionality

| Area | Functional baseline | Go migration |
|---|---|---|
| Routing | Multiple roles, provider recipients, fallback/default recipients, disabled destinations, duplicate handling | Central named-destination routing/defaults/suppression/deduplication implemented; provider-recipient details pending with providers |
| Status policy | Supported transitions, initial CLEAR, `critical`, `nowarn`, `noclear`, and modifier combinations | Pending |
| Lifecycle history | Per-recipient tracking used by critical-only notification policies | Pending; ownership changes require a semantic comparison |
| Rendering | Provider content, titles, links, timestamps, durations, values, summaries, email MIME/threading | Slack, Discord and Telegram content/link/status formatting implemented; other providers and richer presentation pending |
| Provider configuration | Credentials, endpoints, recipient-specific settings, enable/auto detection, local tool settings | Generic webhook, Slack and Discord URL settings, plus Telegram bot/chat/topic/API/retry settings implemented; remaining provider configuration pending |
| Results | Per-target failures and Bash's any-success invocation result | Implemented for generic webhook, Slack, Discord and Telegram fan-out; provider subtarget details pending with providers |
| Utilities | Synthetic test transitions and configured-method reporting (`dump_methods`) | Pending; `validate` is available |
| Logging | Alert-specific structured fields and operational diagnostics | Pending except basic safe command diagnostics |
| Integration | Agent invocation, legacy config adapters, analytics, installation, operator docs, production cutover | Later milestone |

The first five increments cover the foundation, routing/fan-out, modern Slack app webhooks, native Discord, and Telegram.
Legacy Slack override support remains pending; choosing modern webhooks first does not permanently remove that functionality.
More small PRs follow until the functional baseline and explicitly approved exceptions are complete. Final
architecture and broad refactoring are discussed after that working baseline exists.
