# Description Authoring

Metadata descriptions are public product copy. They appear on
Learn, in integration cards, in generated umbrella pages, and in
some in-app surfaces. Write them for an operator scanning a catalog,
not for a developer reading implementation notes.

## Catalog Description Contract

The Monitor Anything table does not read a dedicated
`catalog_description` field. `integrations/gen_doc_collector_page.py`
extracts the first sentence from the generated `## Overview` section
and falls back to `meta.monitored_instance.description` only when
overview text is unavailable.

For collector-like integrations, that means the first sentence of
`overview.data_collection.metrics_description` **is** the catalog
description. Write that sentence first, deliberately, before adding
detail for the full integration page.

That first sentence must:

- start with an active user-facing verb or action phrase;
- describe what the integration is;
- describe what it monitors, enriches, exports, authenticates, or
  discovers;
- be stable without knowing the user's configuration;
- be short enough for a table cell;
- use user-facing product language.

That first sentence must not:

- describe a configuration option, variable, default value, or setting;
- start with "Set ...", "Configure ...", "When enabled ...", or
  similar setup language;
- contain placeholders such as `<tier>`, `<key>`, or
  `[[ variables.foo ]]`;
- describe limits, sizing, retention, troubleshooting, or caveats;
- mention internal tests, implementation state, reviewer notes, or
  future work.

Required first-sentence style:

- Collectors: `Monitor <thing> ...`, `Collect <data> from <thing> ...`,
  `Keep an eye on <thing> ...`.
- Flow sources: `Collect network flow records from <protocol/exporter> ...`.
- Flow enrichment sources: `Enrich network flows with <fields/context> from
  <source> ...`.
- Flow labeling/classification sources: `Annotate network flows with
  <labels> from <source/rules> ...`.
- Exporters: `Export Netdata metrics to <destination> ...`.
- Service discovery: `Discover <targets> from <source> ...`.

Avoid leading with the provider's publication mechanism (`AWS publishes ...`,
`Microsoft publishes ...`, `Set option ...`). Those facts may be useful in
the full page, but the catalog sentence should first tell users what Netdata
does for them.

## Where Details Belong

Use the right metadata field for the job:

| Content | Field |
|---|---|
| What the integration is / what data it provides | First sentence of `overview.data_collection.metrics_description` |
| How collection works | `overview.data_collection.method_description` |
| Defaults and auto-detection | `overview.default_behavior.auto_detection.description` |
| Limits, retention, sizing, and cardinality | `overview.default_behavior.limits.description` |
| CPU, memory, disk, or network impact | `overview.default_behavior.performance_impact.description` |
| Configuration settings | `setup.configuration.options.list[].description` |
| Example-specific behavior | `setup.configuration.examples.list[].description` |
| Failure modes and fixes | `troubleshooting.problems.list[].description` |

Configuration option descriptions are allowed to describe settings.
Catalog descriptions are not.

## Good And Bad Examples

Bad catalog description:

```yaml
metrics_description: |
  Set `protocols.decapsulation_mode` to `srv6` or `vxlan`.
```

Good catalog description:

```yaml
metrics_description: |
  Enrich network flows with inner source and destination endpoints from VXLAN or SRv6 encapsulated traffic.
```

Bad catalog description:

```yaml
metrics_description: |
  Empty `asn_database` and `geo_database` values enable auto-detection.
```

Good catalog description:

```yaml
metrics_description: |
  Enrich network flows with ASN and geographic context from DB-IP Lite MMDB databases.
```

Bad catalog description:

```yaml
metrics_description: |
  The `journal.tiers.<tier>.duration_of_journal_files` setting controls retention.
```

Good catalog description:

```yaml
metrics_description: |
  Collect network flow records from NetFlow exporters such as routers, switches, and firewalls.
```

## Review Checklist

Before committing `metadata.yaml` changes:

1. Regenerate `src/collectors/COLLECTORS.md`.
2. Read the table row description for the integration.
3. Confirm it answers "what is this integration?" without relying on
   setup context.
4. Move option names, defaults, variables, and limits out of the
   catalog sentence and into the proper setup or default-behavior
   field.
5. Keep the first sentence useful even when rendered alone in a list,
   card, search result, or generated catalog.
