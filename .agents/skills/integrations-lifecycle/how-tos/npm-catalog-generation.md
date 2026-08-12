# How-to: the NPM catalog generation chain

The Network Performance Monitoring catalog is a **second-order generator**: a
script writes a `metadata.yaml`, and the normal pipeline then renders that file
like any other source metadata. This is the only place in the repo where a
`metadata.yaml` is itself generated from something other than an ibm.d
`contexts.yaml`, so it is easy to hand-edit it by mistake.

## The chain

```text
snmp.profiles/default/*.yaml            (176 SNMP device profiles)
snmp.trap-profiles/catalogue.json       (803 trap vendors)
        |
        v
integrations/gen_npm_catalog.py         <-- run BY HAND, not by CI
        |
        v
src/go/plugin/go.d/collector/snmp/npm-catalog/metadata.yaml   (COMMITTED, generated)
        |
        v
integrations/gen_integrations.py        -> integrations.js / integrations.json (gitignored)
integrations/gen_docs_integrations.py   -> npm-catalog/integrations/*.md (COMMITTED, generated)
```

Both `metadata.yaml` and the ~1000 `.md` pages under
`src/go/plugin/go.d/collector/snmp/npm-catalog/integrations/` are committed and
generated. Neither is hand-editable.

## The CI gap that matters

`.github/workflows/generate-integrations.yml` runs `gen_integrations.py`,
`gen_taxonomy.py`, `gen_taxonomy_seed.py`, `gen_docs_integrations.py`,
`gen_doc_collector_page.py`, and `gen_doc_secrets_page.py`. It does **NOT** run
`gen_npm_catalog.py`.

Consequence: a change to an SNMP device profile or trap profile does **not**
propagate to the NPM catalog on its own. Whoever changes the profiles (or the
generator) must run `gen_npm_catalog.py` locally and commit the regenerated
`metadata.yaml`. CI will then regenerate the `.md` pages from it, but it cannot
regenerate the metadata itself.

## Regenerating, end to end

```bash
python3 integrations/gen_npm_catalog.py      # profiles      -> metadata.yaml
python3 integrations/gen_integrations.py     # metadata.yaml -> integrations.js/.json
python3 integrations/gen_docs_integrations.py # metadata.yaml -> per-entry .md pages
```

Stage the changed files explicitly; `gen_integrations.py` also touches the
gitignored `integrations.js` / `integrations.json`, which must never be
committed.

## Catalog composition

`main()` concatenates six builders, in this order:

| Builder | Entries | Category |
|---|---|---|
| `build_device_modules` | one per SNMP device profile | Device Metrics |
| `build_capability_modules` | BGP- and licensing-capable vendors | BGP / Licensing |
| `build_topology_modules` | 7 SNMP discovery methods + 4 non-SNMP producers | Topologies |
| `build_syslog_modules` | 1 | Syslog |
| `build_trap_modules` | one per trap-profile vendor | SNMP Traps |
| `build_trap_enrichment_modules` | 3 | SNMP Traps |

Capability (BGP / licensing) is detected by resolving each profile's transitive
`extends:` chain, not by a flag in the profile.

## The YAML-anchor trap

`ruamel.yaml` emits shared Python objects as YAML anchors and aliases. Because
`make_entry()` reuses the same module-level dicts for `setup`, `troubleshooting`,
and `metrics`, the generated `metadata.yaml` contains `setup: &id001 ...` on the
first entry and `setup: *id001` on the other thousand.

This is what made every non-SNMP producer render SNMP setup instructions: a
single hardcoded `SETUP` dict was aliased onto entries produced by
`network-viewer.plugin`, the streaming graph, vSphere, Cato, and the
OpenTelemetry syslog pipeline, none of which use SNMP.

Two practical consequences when editing the generator:

- **Sharing an object is the default, not an optimization.** If a builder needs
  a different `setup` / `metrics` / `troubleshooting`, it must pass its own
  object; there is no per-entry copy.
- **The anchor id numbering shifts.** Introducing a new shared object renumbers
  the anchors after it (`&id004` etc.), so a regeneration diff can look larger
  than the semantic change. Check the diff hunk ranges, not the line count:
  a setup-only change should be confined to the affected entries' line ranges.

## Verifying a generator change did not disturb the other 990 entries

```bash
python3 integrations/gen_npm_catalog.py
git diff -U0 src/go/plugin/go.d/collector/snmp/npm-catalog/metadata.yaml | grep '^@@'
```

Every hunk should fall inside the line range of the entries you intended to
change. A hunk in the device or trap ranges means the default was altered.

## Setup blocks are per-producer

`make_entry(..., setup=None)` defaults to the SNMP `SETUP`. Producers that are
not SNMP-based pass their own block, built with `setup_block()`. Two template
behaviors make "nothing to configure" a first-class state rather than something
to fake (`integrations/templates/setup-generic.md`):

- an empty `prerequisites` list renders `No action required.`;
- an empty `configuration.file.name` renders `There is no configuration file.`

Never point an entry at a config file it does not use just to fill the section.

Also note the template renders the **UI / File** configuration table only when
`meta.plugin_name == 'go.d.plugin'`. Entries owned by other plugins
(`network-viewer.plugin`, `otel.plugin`, `netdata`) correctly skip it.
