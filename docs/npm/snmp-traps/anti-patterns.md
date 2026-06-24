<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/snmp-traps/anti-patterns.md"
sidebar_label: "Anti-Patterns"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'anti-patterns', 'security', 'operations', 'validation', 'capacity']
endmeta-->

<!-- markdownlint-disable-file -->

# Anti-Patterns

Use this page to avoid common SNMP trap collection mistakes before they become incident, security, or storage problems. It is written for NetOps leads, SREs, SecOps teams, and platform owners who need trap data that is trustworthy during operations.

Each anti-pattern explains:

- why it hurts;
- the first symptom operators usually see;
- the better practice to use instead.

For the details behind these recommendations, see [Configuration](/docs/npm/snmp-traps/configuration.md), [Validation and Data Quality](/docs/npm/snmp-traps/validation-and-data-quality.md), [Metrics](/docs/npm/snmp-traps/metrics.md), [Alerts](/docs/npm/snmp-traps/alerts.md), [Sizing and Capacity](/docs/npm/snmp-traps/sizing-and-capacity.md), [Journal and Querying](/docs/npm/snmp-traps/journal-and-querying.md), [Forwarding to SIEM](/docs/npm/snmp-traps/forwarding-to-siem.md), [Investigation Playbooks](/docs/npm/snmp-traps/investigation-playbooks.md), and [Field Reference](/docs/npm/snmp-traps/field-reference.md).

## Copying quick-start config into production

**Anti-pattern:** Treating the first working listener as production-ready.

**Why it hurts:** Quick-start settings are meant to prove that traps arrive. Production jobs need explicit listener addresses, source controls, accepted SNMP versions, credentials, output backends, retention, and rate-limit or deduplication decisions.

**First symptom:** The receiver works in a lab, then production shows unexpected senders, excessive `unknown_oid`, unexpected `dropped_allowlist`, missing local history, or noisy downstream alerts.

**Better practice:** Use the quick start only for first receipt. Before production, review the full job in [Configuration](/docs/npm/snmp-traps/configuration.md) and validate it with [Validation and Data Quality](/docs/npm/snmp-traps/validation-and-data-quality.md).

## Broad open listener with no allowlist

**Anti-pattern:** Leaving a production listener on every interface with an open source allowlist.

**Why it hurts:** A job created with default listener and source settings can bind broadly, and the default `allowlist.source_cidrs` is the catch-all `0.0.0.0/0` and `::/0`. Omitted or empty `source_cidrs` also accepts all source IPs. That exposes the decoder and authentication paths to traffic that should never reach the trap receiver.

**First symptom:** Operators see traps from unexpected UDP peers. With an open allowlist, unexpected peers can reach authentication and decode checks, producing `auth_failures` or `malformed_pdu`. After a narrow allowlist is configured, peers outside it are rejected earlier as `dropped_allowlist`.

**Better practice:** Bind to the intended local address and set `allowlist.source_cidrs` to the smallest practical real sender or relay ranges. Documentation examples may use RFC 5737 ranges such as `192.0.2.0/24`, `198.51.100.0/24`, or `203.0.113.0/24`; treat them as placeholders and replace them with the real ranges for your deployment.

## Broad trusted relays

**Anti-pattern:** Adding device subnets, site subnets, or catch-all prefixes to `source.trusted_relays`.

**Why it hurts:** A trusted relay can supply original source identity through `snmpTrapAddress.0`. If the trusted range is broad, untrusted senders on that path can influence source attribution.

**First symptom:** `TRAP_SOURCE_IP` differs from `TRAP_SOURCE_UDP_PEER` in ways the team cannot explain, enrichment looks wrong, or per-source metrics group traps under the wrong device identity.

**Better practice:** Trust only the known relay peer, for example `192.0.2.10/32`. Validate relayed traps by comparing `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, and `TRAP_ENRICHMENT.source` as described in [Validation and Data Quality](/docs/npm/snmp-traps/validation-and-data-quality.md).

## Inline secrets

**Anti-pattern:** Storing SNMPv1/v2c communities, SNMPv3 auth or privacy keys, or OTLP headers directly in `snmp_traps.conf`.

**Why it hurts:** These values are credentials. Inline secrets can leak through config copies, tickets, shell history, downstream examples, or support artifacts.

**First symptom:** Reviews find real community strings, SNMPv3 keys, or authorization headers in configuration snippets or operational notes.

**Better practice:** Use Netdata secret references for communities, SNMPv3 keys, and OTLP headers. Netdata supports file-style references such as `${file:/run/secrets/snmp-trap-community}` and secret-store references such as `${store:vault:vault_prod:secret/data/snmp-traps#otlp_authorization}`; use the form that matches your deployment. Use placeholders such as `[REDACTED_SECRET]` in examples. Review [Secrets Management](/src/collectors/SECRETS.md) and the sensitive-data cautions in [Field Reference](/docs/npm/snmp-traps/field-reference.md).

## Treating traps as complete device state

**Anti-pattern:** Treating traps as the full current state of a device, or as a replacement for metric collection.

**Why it hurts:** Traps are event messages that devices choose to send. They do not prove the complete device state, and they do not show everything that polling or metric collection can show.

**First symptom:** A dashboard or incident process says a device is healthy because no problem trap arrived, while receiver metrics or device polling show a different picture.

**Better practice:** Use traps as event evidence. Combine them with receiver metrics, device metrics, logs, and validation checks. See [Metrics](/docs/npm/snmp-traps/metrics.md) for what trap metrics prove and what they do not prove.

## Treating silence as proof of health

**Anti-pattern:** Assuming no trap rows means no device problem.

**Why it hurts:** Silence can mean no event happened, but it can also mean devices are sending to the wrong endpoint, the network blocks UDP, source controls reject the sender, credentials do not match, rate limiting or deduplication changed visibility, output writes are failing, or retention removed old rows.

**First symptom:** Operators cannot find an expected trap during an incident, and `received`, `decoded`, `accepted`, or `committed` counters do not move as expected.

**Better practice:** Send a known test trap, check `snmp.trap.pipeline`, inspect `snmp.trap.errors`, and confirm the configured output backend. Use the validation checklist in [Validation and Data Quality](/docs/npm/snmp-traps/validation-and-data-quality.md).

## Alerting on every trap without policy

**Anti-pattern:** Paging on every trap row, or forwarding every trap as the same urgency.

**Why it hurts:** Traps include state changes, configuration changes, security events, diagnostics, informational events, and unknown vendor messages. Treating all of them the same creates noise and hides the events that need action.

**First symptom:** Alert fatigue starts immediately. Operators mute trap alerts, and real high-severity traps are missed in the noise.

**Better practice:** Build alert policy from `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_OID`, `TRAP_NAME`, source identity, and local overrides. Start from the shipped receiver and severity alerts in [Alerts](/docs/npm/snmp-traps/alerts.md), then add local rules only where the policy is clear.

## Blanket-suppressing authenticationFailure traps

**Anti-pattern:** Treating a wave of `authenticationFailure` traps (`SNMPv2-MIB::authenticationFailure`, `TRAP_CATEGORY=auth`) as routine noise and silencing or deduplicating it away.

**Why it hurts:** A surge of authentication-failure traps is a SecOps signal, not just trap noise. It commonly means a scanner is probing the device, a host is using wrong or stale SNMP credentials, or a credential rotation is half-applied. Suppress it and you lose the earliest evidence of unauthorized access attempts or a broken rotation.

**First symptom:** `TRAP_CATEGORY=auth` rises across one or many devices, often from an unexpected `TRAP_SOURCE_IP`, sometimes alongside `auth_failures` or `usm_failures` on the receiver.

**Better practice:** Route auth-category traps to your security workflow and investigate the source before tuning anything. Use the [Critical, security, or authentication traps](/docs/npm/snmp-traps/investigation-playbooks.md#critical-security-or-authentication-traps) playbook to identify the source and intent. Apply deduplication or rate limiting to a confirmed-benign storming source only, never as a blanket auth filter.

## Ignoring unknown OID and profile coverage

**Anti-pattern:** Accepting traps with unknown OIDs indefinitely because rows are still being stored.

**Why it hurts:** Unknown OIDs are data-quality gaps. The receiver can store the row, but operators lose resolved names, categories, severities, and useful varbind labels.

**First symptom:** `TRAP_CATEGORY=unknown`, missing `TRAP_NAME`, or the `unknown_oid` error dimension grows for important devices.

**Better practice:** Treat repeated important unknown OIDs as profile or override work. Validate `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, and `TRAP_VAR_*` coverage with [Validation and Data Quality](/docs/npm/snmp-traps/validation-and-data-quality.md).

## High-cardinality profile metrics or labels

**Anti-pattern:** Enabling broad profile metrics or labels that use unbounded identity, resource, or payload values.

**Why it hurts:** Profile metrics create time-series. Unbounded sources, resources, labels, or custom rules can create too many metric instances. Netdata protects the node with profile metric limits, but over-cap metric instances are skipped and counted by diagnostics.

**First symptom:** Operators see fewer profile-metric instances than expected, while the affected traps still appear as accepted log rows. `snmp.trap.profile_metric_diagnostics` shows `overflow_dropped`, `rule_missed`, `extraction_failed`, `attribution_failed`, or `source_transitions`.

**Better practice:** Keep `profile_metrics` disabled until rules are reviewed. Prefer `profile_metrics.mode: exact`, enable only bounded rules, keep `profile_metrics.identity.source_id_privacy: hash`, and set limits intentionally. See [Configuration](/docs/npm/snmp-traps/configuration.md) and [Metrics](/docs/npm/snmp-traps/metrics.md).

## Disabling journal while expecting local queries

**Anti-pattern:** Setting `journal.enabled: false` while expecting the job to appear in local Logs or `journalctl --directory` queries.

**Why it hurts:** OTLP-only jobs do not create local journal files and do not appear as local job sources in the `snmp:traps` Function. Operators must query the downstream OTLP receiver instead of local Logs or `journalctl --directory` for that job.

**First symptom:** Traps are exported downstream, but local Logs and local journal queries have no source for that job.

**Better practice:** Keep `journal.enabled: true` when local investigation is required. Disable it only when OTLP-only operation is intentional and the downstream receiver is validated. See [Journal and Querying](/docs/npm/snmp-traps/journal-and-querying.md) and [Forwarding to SIEM](/docs/npm/snmp-traps/forwarding-to-siem.md).

## Forwarding full payloads everywhere

**Anti-pattern:** Forwarding full `TRAP_JSON` or OTLP `snmp.varbinds` to every downstream system without review.

**Why it hurts:** Community varbinds are omitted, but other varbinds and enrichment data can still include sensitive inventory, usernames, interface descriptions, MACs, public IPs, asset tags, locations, device identifiers, or vendor text. Full payloads can also be large or high-cardinality.

**First symptom:** SIEM indexes contain sensitive operational context, large payload fields dominate storage, or rules group on unstable JSON instead of stable fields.

**Better practice:** Use specific fields for routine routing and rules: `TRAP_REPORT_TYPE`, `TRAP_JOB`, `TRAP_SOURCE_IP`, `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, and selected `TRAP_VAR_*`. Review full payload forwarding against the sensitive-data guidance in [Field Reference](/docs/npm/snmp-traps/field-reference.md).

## Enabling rate-limit drop mode before measuring

**Anti-pattern:** Turning on `rate_limit.enabled: true` with the default `rate_limit.mode: drop` before measuring normal traffic and storm volume.

**Why it hurts:** In `drop` mode, over-limit traps are counted as `rate_limited` and discarded before they become normal rows. That protects storage and forwarding, but it also removes evidence during first validation.

**First symptom:** `snmp.trap.errors` `rate_limited` rises, operators see fewer rows than packets, and there is no deduplication summary explaining the missing rows.

**Better practice:** During validation, keep rate limiting disabled or use `rate_limit.mode: sample` so over-limit traps are counted and still continue through the receiver. Switch to `drop` only when protecting local journal storage or downstream export is more important than preserving every over-limit trap. See [Configuration](/docs/npm/snmp-traps/configuration.md) and [Sizing and Capacity](/docs/npm/snmp-traps/sizing-and-capacity.md).

## Ignoring rate limit and dedup behavior during validation

**Anti-pattern:** Enabling rate limiting or deduplication before proving what the receiver stores and exports.

**Why it hurts:** Rate limiting can drop over-limit traffic. Deduplication stores a first matching trap and then summarizes repeated matching traps during the window. Dedup-suppressed traps do not update profile-defined metrics.

**First symptom:** Operators see fewer rows than test packets, `deduplication_summary` rows appear, `dedup_suppressed` rises, or `rate_limited` increments during validation.

**Better practice:** During first validation, keep rate limiting and deduplication disabled or set `rate_limit.mode: sample`. In `sample` mode, over-limit traps increment `rate_limited` and still continue through the receiver. When deduplication is enabled, validate `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, and the dedup fingerprint behavior.

## Skipping retention and storage planning

**Anti-pattern:** Accepting default local retention without checking trap volume, investigation needs, and downstream ownership.

**Why it hurts:** Direct-journal retention controls how much local trap history is kept. Too little retention removes forensic evidence before operators need it. Too much retention can consume more local storage than intended.

**First symptom:** An incident review needs older trap rows that were already evicted, or local trap journal files grow beyond the expected operational budget.

**Better practice:** Choose `retention` based on expected trap volume and the required local investigation window. If downstream OTLP storage keeps the long-term trap history and local querying is not needed, validate OTLP-only operation instead of keeping unnecessary local history. See [Sizing and Capacity](/docs/npm/snmp-traps/sizing-and-capacity.md) and [Configuration](/docs/npm/snmp-traps/configuration.md).

## Quick review checklist

Before relying on an SNMP trap listener in production, verify:

- the listener binds only where it should;
- `allowlist.source_cidrs` is narrow enough for the deployment;
- `source.trusted_relays` contains only real relay peers;
- accepted SNMP `versions`, `communities`, and SNMPv3 `usm_users` match the intended senders;
- secrets use references, not inline values;
- unknown OIDs are tracked as coverage gaps;
- alert rules use policy, not raw trap volume alone;
- profile metrics use bounded rules and labels;
- the intended output is confirmed: local journal for local investigation, OTLP as the only output when journal is disabled, or journal plus OTLP export, where the local journal is the source of truth;
- `rate_limit` and `dedup` behavior is understood before validation results are trusted;
- retention matches investigation and storage requirements.
