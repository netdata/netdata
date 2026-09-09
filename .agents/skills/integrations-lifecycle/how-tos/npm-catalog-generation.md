# How-to: the NPM catalog generation chain

The Network Performance Monitoring catalog is a second-order generator: `integrations/gen_npm_catalog.py` writes a
`metadata.yaml`, and the normal pipeline renders that file like any other source. It is the only `metadata.yaml`
generated from something other than ibm.d inputs, so it is easy to hand-edit by mistake. Both the generated
`src/go/plugin/go.d/collector/snmp/npm-catalog/metadata.yaml` and its 1013 pages under `npm-catalog/integrations/` are
committed and generated; neither is hand-editable. Every run also writes the untracked
`npm-catalog/metrics-metadata-gaps.txt` (`write_gap_report`): read it to see which profile metrics lack metadata, then
delete it. Delivery (source PR versus the post-merge regeneration PR, the gitignored catalogs, the side report) is in
`../consistency.md`, "Delivery boundary".

## The chain

```text
snmp.profiles/default/*.yaml  +  snmp.trap-profiles/catalogue.json
        |
        v
integrations/gen_npm_catalog.py           <-- run locally and first in both integration workflows
        |
        v
npm-catalog/metadata.yaml                 (committed, generated; integration_type: device)
        |
        v
integrations/gen_integrations.py          -> integrations.js / integrations.json (gitignored)
integrations/gen_docs_integrations.py     -> npm-catalog/integrations/*.md (committed, generated)
```

The order is mandatory: `gen_docs_integrations.py` renders from `integrations.js`, not from `metadata.yaml`, and that
file does not exist in a fresh checkout. Run without it and `read_integrations_js` fails before cleanup, leaving the
committed pages intact but stale; run with a stale one and the pages quietly regenerate from old data, so a metadata
change appears to have no effect.

## Catalog composition

`main()` concatenates six builders, in this order:

| Builder | Entries | Category |
|---|---|---|
| `build_device_modules` | one per SNMP device profile | Device Metrics |
| `build_capability_modules` | BGP- and licensing-capable vendors | BGP / Licensing |
| `build_topology_modules` | the SNMP discovery methods plus the non-SNMP producers (network-viewer, streaming, vSphere, Cato) | Topologies |
| `build_syslog_modules` | the OpenTelemetry syslog pipeline | Syslog |
| `build_trap_modules` | one per trap-profile vendor | SNMP Traps |
| `build_trap_enrichment_modules` | the trap enrichment sources | SNMP Traps |

Capability (BGP, licensing) is detected by resolving each profile's transitive `extends:` chain, not by a flag.

## Shared objects, and the anchors that reveal them

A past defect: `make_entry()` assigned the same module-level `SETUP` dict to every entry, so the non-SNMP topology
producers and the syslog pipeline rendered SNMP prerequisites and `edit-config go.d/snmp.conf`. YAML anchors did not
cause it; they made it visible: `ruamel.yaml` serializes a shared Python object as `setup: &id001 ...` on the first
entry and `setup: *id001` on every other, so the alias is the symptom to grep for. Consequences when editing the
generator:

- Sharing an object is the default, not an optimization. A builder that needs a different `setup`, `metrics`, or
  `troubleshooting` MUST pass its own object. The same applied to values baked into `overview()`: `supported_platforms`
  and `multi_instance` were once hardcoded for every entry, claiming all-platform support for singleton Linux-only
  producers.
- Anchor numbering shifts when a new shared object appears, so a regeneration diff can look larger than the change.
  Check hunk ranges, not line counts.

## Setup blocks are per producer

`make_entry(..., setup=None)` defaults to the SNMP `SETUP`; non-SNMP producers pass a block built with `setup_block()`.
`integrations/templates/setup-generic.md` makes "nothing to configure" a first-class state: an empty `prerequisites`
list renders `No action required.` and an empty `configuration.file.name` renders `There is no configuration file.`
Never point an entry at a config file it does not use to fill the section. The template renders the UI / File
configuration table only for `meta.plugin_name == 'go.d.plugin'` entries without `setup.single_job`, so entries owned by
other plugins skip it correctly.

## Verify entries against the collector's own metadata

Several entries describe collectors that have an authoritative `metadata.yaml` elsewhere
(`src/collectors/network-viewer.plugin/metadata.yaml` for example). That file is the source of truth for supported
platforms, `multi_instance`, and options; copy those facts, because a catalog entry that contradicts the collector's own
metadata is a bug in the catalog. Producers with no `metadata.yaml` of their own (`snmp_topology` is one) have the
catalog entry as their only public documentation, so check its setup block directly against the collector source and
`config_schema.json`.

## Verify a generator change did not disturb the other entries

```bash
python3 integrations/gen_npm_catalog.py
git diff -U0 src/go/plugin/go.d/collector/snmp/npm-catalog/metadata.yaml | grep '^@@'
```

Every hunk should fall inside the line range of the entries you meant to change. A hunk in the device or trap ranges
means a default was altered.
