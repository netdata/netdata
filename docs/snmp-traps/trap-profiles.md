<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/trap-profiles.md"
sidebar_label: "Trap Profiles"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'trap profiles', 'mib conversion', 'trap oid', 'trap metrics', 'netops', 'noc', 'sre']
endmeta-->

<!-- markdownlint-disable-file -->

# Trap Profiles

Trap profiles are the meaning layer for SNMP traps. They tell Netdata how to turn a raw trap OID into useful log fields, tags, messages, and optional metrics.

Use this page when you need to understand why a trap has a specific name, category, severity, message, tag, or profile-derived chart when profile metrics are enabled. For listener setup and hardening, see [Configuration](/docs/snmp-traps/configuration.md).

## What profiles do

An SNMP trap packet carries a numeric trap OID and a set of varbinds. A trap profile maps that data to operator-friendly output:

- `TRAP_OID`: the numeric trap OID used to match the loaded profile entry.
- `TRAP_NAME`: the MIB-qualified trap name, such as `SNMPv2-MIB::coldStart`.
- `TRAP_CATEGORY`: one of Netdata's trap categories, such as `state_change`, `config_change`, `security`, or `unknown`.
- `TRAP_SEVERITY`: one of the syslog severity slugs, such as `warning`, `notice`, or `crit`.
- `MESSAGE`: the human-readable trap message rendered from the profile description.
- `TRAP_TAG_*`: profile-defined or override-defined labels that are safe to index and filter.
- `TRAP_VAR_*` and `TRAP_JSON`: decoded varbind fields and the structured varbind payload.
- Profile-defined metrics and charts, when loaded profiles define metric rules and the listener job enables selected rules.

There is no separate `TRAP_MESSAGE` journal field. The profile-rendered message is written to the standard journal `MESSAGE` field.

Profiles also define varbind metadata, including symbolic names, OIDs, types, and enum labels. For enum-backed varbinds, `TRAP_VAR_*` shows the readable enum label in Logs and a sibling `TRAP_VAR_*_RAW` field keeps the original numeric value.

## What ships

Netdata ships with a stock trap profile pack covering **800+ vendors**, **6,000+ MIBs**, and **150,000+ trap definitions**. These profiles cover many common network devices without manual MIB work.

Stock profiles provide:

- Numeric trap OID to `TRAP_NAME` mapping.
- Closed-set `TRAP_CATEGORY` and `TRAP_SEVERITY` values.
- Human-readable `MESSAGE` templates.
- Varbind names, types, and enum labels.

The current stock pack provides trap decoding coverage. It does not ship profile metric rules or chart definitions, so profile-derived charts require operator profile files that define `metrics:` and `charts:` rules.

The category set is fixed: `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, and `unknown`.

The severity set is fixed: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, and `debug`. Netdata maps these to the journal `PRIORITY` field.

## Unknown OIDs

An unknown OID is not the same as broken ingestion.

When Netdata receives and accepts a trap whose OID is not in the loaded profile set:

- the trap is still stored or exported if the configured output backend succeeds;
- `TRAP_OID` keeps the numeric trap OID;
- `TRAP_CATEGORY` is `unknown`;
- `TRAP_SEVERITY` is `notice`;
- `MESSAGE` uses a plain fallback form that includes the OID and source;
- profile-derived names, labels, descriptions, and metrics are not added.

Use unknown OIDs as a coverage signal. If the device is sending a valid trap that your operators need to recognize, add an operator profile file or convert the vendor MIBs into a profile.

Profile lookup tries the exact trap OID first, then tries a single `.0.` insertion or removal around the last arc to handle SMIv1 and SMIv2 trap OID form differences. For example, `1.3.6.1.4.1.14179.2.6.3.24` and `1.3.6.1.4.1.14179.2.6.3.0.24` resolve to the same profile entry. `TRAP_OID` still shows the OID sent by the device.

## Profile locations

| Path | Purpose |
|---|---|
| `/etc/netdata/go.d/snmp.trap-profiles/` | Operator profile files and profile metric rules. Edits here survive package upgrades. |
| `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/` | Stock trap profiles shipped with Netdata. Reference only. |

Depending on installation type, paths may be prefixed with `/opt/netdata`.

Operator profiles should normally stay as editable `.yaml` or `.yml` files. Stock profiles are reference-only; they are not the place for site-specific edits, and package updates can replace them.

For the profile YAML schema, see [SNMP Trap Profile Format](/src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md).

## Custom MIB conversion

Netdata does not compile ASN.1 MIB files while traps arrive. Runtime trap decoding uses loaded YAML profiles.

If you have vendor MIBs that are not covered by the stock pack, convert them offline with the installed helper:

```sh
/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen generate \
  --source-dir ./mibs \
  --all \
  --out-dir ./snmp-trap-profile-gen-output
```

The helper writes generated profile YAML files under:

```text
./snmp-trap-profile-gen-output/profiles/
```

Copy the needed YAML files into:

```text
/etc/netdata/go.d/snmp.trap-profiles/
```

Then confirm the next matching trap resolves to the expected `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `MESSAGE`, and `TRAP_VAR_*` fields in Logs. Generated profiles should be reviewed before use; add or adjust `metrics:` and `charts:` sections only when you intentionally want profile-derived metrics.

## Per-OID overrides vs profile files

Use the smallest control that matches the job.

| Need | Use | Why |
|---|---|---|
| Change category, severity, or labels for one already known OID | `overrides` in `go.d/snmp_traps.conf` | It adjusts loaded profile output without copying profile files. |
| Add a name, message, varbind definitions, or metric rules for an unknown OID | Operator profile file | Overrides do not create a new trap definition. |
| Add many OIDs from a vendor MIB | Custom MIB conversion | It generates profile YAML from the MIB source instead of hand-writing every trap. |
| Replace stock behavior for one vendor file | Operator profile with the same filename | The operator file fully replaces the stock file of the same name. |
| Add site-specific traps without replacing stock vendor files | Operator profile with a different filename; use `extends:` when the file should inherit an existing profile | It adds entries alongside the stock profile set without copying the whole stock vendor file. |

`extends:` entries are bare `.yaml` or `.yml` filenames without path separators. Netdata resolves them across the operator and stock profile directories, operator files first. They can inherit traps, varbinds, metric rules, and chart definitions from another visible profile file, and fields in the extending file win where they overlap. Deeply nested or circular `extends:` chains are rejected at profile load time.

For simple policy overrides, configure the listener job:

```yaml
overrides:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1
    category: config_change
    severity: notice
    labels:
      change_window: business_hours
```

Label keys become `TRAP_TAG_<KEY_UPPERCASE>` fields. For example, `change_window` becomes `TRAP_TAG_CHANGE_WINDOW`. Label keys must start with a lowercase letter and then use only lowercase letters, digits, and underscores.

Job overrides use static label values. Profile-file labels can also use templates, but templated label values must come from bounded sources such as enum-backed varbinds, booleans, small numeric ranges, `TRAP_NAME`, or `TRAP_DEVICE_VENDOR`; unbounded values such as source IPs, hostnames, interface descriptions, MAC addresses, usernames, packet contents, and free-form descriptions are rejected at profile load time.

## Profile reload behavior

- While a listener job is running, edits to operator profiles under `/etc/netdata/go.d/snmp.trap-profiles/` are picked up automatically.
- If a changed operator profile is invalid, the failure is logged and the last valid profiles stay active.
- Stock profile updates apply after the Netdata Agent restarts.
- If no listener job is active, the next listener job creation loads and validates the profile files.

Profile validation failures are visible as collector errors and profile-load-failure metrics. After editing profiles, check Logs and receiver metrics before assuming a change is active; see [Metrics](/docs/snmp-traps/metrics.md) for the receiver diagnostics.

## Verify profile changes

After adding or changing an operator profile:

- Check the Netdata Agent logs for profile reload messages or profile validation errors.
- Check the `profile_load_failed`, `unknown_oid`, and `template_unresolved` dimensions in the SNMP trap processing errors chart.
- Send or wait for a matching trap, then confirm the expected `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `MESSAGE`, `TRAP_VAR_*`, and `TRAP_TAG_*` fields in Logs.
- If `profile_metrics.include` names a missing rule, the listener job fails validation with `profile_metrics.include rule "<name>" not found`. If the rule exists but is disabled in the profile, validation fails with `profile_metrics.include rule "<name>" is disabled by profile`. Fix the rule name, select another rule, or enable the intended rule in the loaded operator profile.

For field-level query details, see [Usage and Output](/docs/snmp-traps/usage-and-output.md) and [Field Reference](/docs/snmp-traps/field-reference.md).

## Profile-defined metrics and cardinality

Profiles can define optional trap-to-metric rules and chart definitions. Listener jobs decide whether to evaluate those rules with `profile_metrics`.

Profile metrics are disabled by default, and the current stock pack does not ship metric rules. To create profile-derived charts today:

1. Add `metrics:` and `charts:` rules to an operator profile file under `/etc/netdata/go.d/snmp.trap-profiles/`.
2. Wait for the automatic reload if a listener job is already running, or start a listener job to load the profile files.
3. Enable `profile_metrics` in the listener job and select the loaded rule names.

Rule names in `include` come from metric rule `name` fields in loaded profile YAML files. If no loaded profile defines metric rules, enabling `profile_metrics` creates no profile-derived charts.

The listener-side `profile_metrics` settings — selection `mode`, `include`, the identity controls, and the cardinality `limits` — are documented in [Configuration](/docs/snmp-traps/configuration.md#profile-metrics). Rule and chart syntax live in [SNMP Trap Profile Format](/src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md).

Use profile metrics for bounded, operator-useful trap signals, such as:

- counting committed configuration change traps;
- tracking trap-derived state where the profile defines a clear problem and clear condition;
- sampling numeric varbind values from traps when the varbind type is supported.

Do not use high-cardinality values as metric labels or resource identities. Avoid MAC addresses, source IPs, usernames, interface descriptions, packet contents, event IDs, and other per-event values as labels. Put those values in `MESSAGE`, `TRAP_VAR_*`, and `TRAP_JSON` instead.

Only committed traps update profile metrics. For the exact update rule (dedup-suppressed and failed-write traps do not update metrics, and an OTLP export failure does not roll back an already-updated metric), see [Metrics](/docs/snmp-traps/metrics.md#profile-defined-metrics).

If metric rules exceed source, resource, chart, or job limits, Netdata skips the over-cap metric instance, keeps accepting traps, and increments profile metric diagnostics.

For full metric rule syntax, validation rules, and examples, see [SNMP Trap Profile Format](/src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md).

## Next steps

- To configure listener jobs, overrides, outputs, and `profile_metrics`, see [Configuration](/docs/snmp-traps/configuration.md).
- To query received traps and read profile-enriched output, see [Usage and Output](/docs/snmp-traps/usage-and-output.md).
- To understand source identity, relay handling, and device context, see [Enrichment](/docs/snmp-traps/enrichment.md).
- To understand every emitted trap field, see [Field Reference](/docs/snmp-traps/field-reference.md).
