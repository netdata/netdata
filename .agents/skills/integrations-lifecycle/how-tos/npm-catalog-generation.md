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
integrations/gen_npm_catalog.py         <-- run locally and by integrations CI
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

## CI ownership

Both `.github/workflows/check-markdown.yml` and
`.github/workflows/generate-integrations.yml` run `gen_npm_catalog.py` before
`gen_integrations.py`. The pull-request workflow validates the exact derived
metadata and public pages without committing them. After the source PR merges,
the post-merge workflow opens the separate generated-artifact PR containing
the NPM `metadata.yaml` and its generated pages.

## Regenerating, end to end

```bash
python3 integrations/gen_npm_catalog.py       # profiles         -> metadata.yaml
python3 integrations/gen_integrations.py      # metadata.yaml    -> integrations.js/.json
python3 integrations/gen_docs_integrations.py # integrations.js  -> per-entry .md pages
```

**The order is mandatory.**
`gen_docs_integrations.py` renders from `integrations/integrations.js`, not from
`metadata.yaml`, and that file is gitignored — it does not exist in a fresh
checkout.

Run `gen_docs_integrations.py` without generating that input first and
`read_integrations_js()` raises a `RuntimeError`. `main()` reports the missing
input and exits nonzero before description preflight or `cleanup()` runs. The
existing committed generated pages therefore remain intact, although they are
still stale relative to any metadata change. Generate `integrations.js` first
and check `git status` before staging.

If `integrations.js` is stale rather than missing, the failure is quieter: the
pages regenerate from the old data, so a `metadata.yaml` change appears to have
had no effect.

Stage only authoritative source, generator, contract, and test files in the
source PR. The post-merge generated-artifact PR owns the NPM `metadata.yaml`
and generated pages. Two runtime/diagnostic outputs are never committed:

- `integrations/integrations.js` and `integrations/integrations.json` — gitignored.
- `src/go/plugin/go.d/collector/snmp/npm-catalog/metrics-metadata-gaps.txt` — a
  side report `gen_npm_catalog.py` writes on every run (`write_gap_report`,
  called from `main`). It is untracked and NOT gitignored, so it shows up as a
  new untracked file after every regeneration. Delete it before staging.

## Catalog composition

`main()` concatenates six builders, in this order:

| Builder | Entries | Category |
|---|---|---|
| `build_device_modules` | one per SNMP device profile | Device Metrics |
| `build_capability_modules` | BGP- and licensing-capable vendors | BGP / Licensing |
| `build_topology_modules` | 7 SNMP discovery methods + 4 non-SNMP producers (network-viewer, streaming, vSphere, Cato) | Topologies |
| `build_syslog_modules` | 1 | Syslog |
| `build_trap_modules` | one per trap-profile vendor | SNMP Traps |
| `build_trap_enrichment_modules` | 3 | SNMP Traps |

Capability (BGP / licensing) is detected by resolving each profile's transitive
`extends:` chain, not by a flag in the profile.

## Shared objects, and the anchors that reveal them

The root cause of a past defect here was `make_entry()` assigning the *same*
module-level `SETUP` dict to every entry it built. Five producers — the four
non-SNMP entries of `build_topology_modules` (`network-viewer.plugin`, the
streaming graph, vSphere, Cato) plus the OpenTelemetry syslog pipeline from
`build_syslog_modules` — therefore rendered SNMP prerequisites and
`edit-config go.d/snmp.conf` despite using no SNMP.

YAML anchors did not cause that; they made it visible. `ruamel.yaml` serializes a
shared Python object as an anchor plus aliases, so the file shows
`setup: &id001 ...` on the first entry and `setup: *id001` on the other thousand.
The alias is the symptom you can grep for.

Two practical consequences when editing the generator:

- **Sharing an object is the default, not an optimization.** If a builder needs
  a different `setup` / `metrics` / `troubleshooting`, it must pass its own
  object; there is no per-entry copy. The same applies to values baked into
  `overview()` — `supported_platforms` and `multi_instance` were once hardcoded
  for every entry, which claimed all-platform support and side-by-side instances
  for singleton, Linux-only producers.
- **The anchor id numbering shifts.** Introducing a new shared object renumbers
  the anchors after it (`&id004` etc.), so a regeneration diff can look larger
  than the semantic change. Check the diff hunk ranges, not the line count:
  a setup-only change should be confined to the affected entries' line ranges.

## Verify catalog entries against the collector's own metadata

Several catalog entries describe collectors that already have an authoritative
`metadata.yaml` elsewhere in the tree — for example
`src/collectors/network-viewer.plugin/metadata.yaml`. That file is the source of
truth for supported platforms, `multi_instance`, and configuration options. When
a catalog entry covers such a collector, copy those facts rather than inventing
them; a catalog entry that contradicts the collector's own metadata is a bug in
the catalog.

Producers with no `metadata.yaml` of their own (`snmp_topology` is one) have the
catalog entry as their *only* public documentation, so its setup block must be
checked directly against the collector source and `config_schema.json`.

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
