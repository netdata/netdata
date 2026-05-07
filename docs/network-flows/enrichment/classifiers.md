<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/network-flows/enrichment/classifiers.md"
sidebar_label: "Classifiers"
learn_status: "Published"
learn_rel_path: "Network Flows/Enrichment"
keywords: ['classifiers', 'akvorado', 'enrichment', 'rules', 'expression']
endmeta-->

# Classifiers

Classifiers tag exporters and interfaces using small expression-based rules. Where [static metadata](/docs/network-flows/enrichment/static-metadata.md) requires you to enumerate every exporter and every ifIndex, classifiers let you write a few rules that match many cases — by name pattern, by IP, by SNMP description, by speed, and so on.

The plugin's classifier language is **Akvorado-compatible** for the documented operators and actions. It is implemented as a hand-written expression parser in Rust, not jq/jaq, and supports a subset of Akvorado's full expression language. If you've written Akvorado classifiers before, your rules will likely work; if you've written `expr-lang` rules with arithmetic, ternaries, or lambdas, those features are not available here.

## Two classifier lists

| Block | Runs | Sees |
|---|---|---|
| `enrichment.exporter_classifiers` | Once per exporter (cached) | Exporter IP and name, current classification fields |
| `enrichment.interface_classifiers` | Once per (exporter, interface) pair, twice per flow (in + out) | Exporter fields, plus interface index/name/description/speed/VLAN, plus current classification |

Rules are evaluated in YAML order. The plugin short-circuits the list when all classification slots are filled.

## What a rule can read

Identifiers available to **exporter classifiers**:

- `Exporter.IP` — the exporter's IP, as a string
- `Exporter.Name` — the exporter's friendly name (from static metadata, or falls back to the IP)
- `CurrentClassification.Group`, `.Role`, `.Site`, `.Region`, `.Tenant` — values already set (by static metadata, or by an earlier rule)

Identifiers available to **interface classifiers**:

- All of the above
- `Interface.Index` — the SNMP ifIndex (integer)
- `Interface.Name`, `Interface.Description` — from static metadata
- `Interface.Speed` — in bits per second
- `Interface.VLAN` — from the flow record's `SRC_VLAN` / `DST_VLAN` (depending on direction)
- `CurrentClassification.Connectivity`, `.Provider`, `.Boundary`, `.Name`, `.Description` — already set

The plugin does NOT poll SNMP itself, so `Interface.Name` / `Description` / `Speed` come only from what you've configured under `metadata_static`. If you haven't configured them, those identifiers will be empty.

## What a rule can do

### Set classification fields

| Action | Result |
|---|---|
| `Classify("v")` or `ClassifyGroup("v")` | Set `EXPORTER_GROUP` |
| `ClassifyRole("v")` | Set `EXPORTER_ROLE` |
| `ClassifySite("v")` | Set `EXPORTER_SITE` |
| `ClassifyRegion("v")` | Set `EXPORTER_REGION` |
| `ClassifyTenant("v")` | Set `EXPORTER_TENANT` |
| `ClassifyProvider("v")` | Set `IN_IF_PROVIDER` / `OUT_IF_PROVIDER` |
| `ClassifyConnectivity("v")` | Set `IN_IF_CONNECTIVITY` / `OUT_IF_CONNECTIVITY` |
| `ClassifyExternal()` / `ClassifyInternal()` | Set `IN_IF_BOUNDARY` / `OUT_IF_BOUNDARY` |
| `SetName("v")` | Set `IN_IF_NAME` / `OUT_IF_NAME` (or exporter name when in an exporter rule) |
| `SetDescription("v")` | Set `IN_IF_DESCRIPTION` / `OUT_IF_DESCRIPTION` |

`Classify*Regex(input, pattern, template)` variants exist for every action above. The pattern is a Rust regex; the template uses `$1`, `$2`, `${name}` capture references.

### Drop the flow

`Reject()` discards the flow record. Always guard it behind a condition — at top level it drops everything.

### Format strings

`Format("...", arg1, arg2)` mimics Go's `fmt.Sprintf` for `%s`, `%v`, `%d`, `%%`. Use it to build values from multiple inputs:

```
ClassifyTenant(Format("tenant-%s", Exporter.Name))
```

## What rules can match against

Operators (highest to lowest precedence):

| Form | Meaning |
|---|---|
| `value == X`, `value != X` | equality / inequality |
| `value > X`, `value >= X`, `value < X`, `value <= X` | numeric or lexicographic comparison |
| `value in [a, b, c]` | membership |
| `value contains "x"` | substring (string only) |
| `value startsWith "x"`, `value endsWith "x"` | prefix / suffix (string only) |
| `value matches "pattern"` | regex match (Rust regex) |
| `cond1 && cond2`, `cond1 and cond2` | logical AND |
| `cond1 \|\| cond2`, `cond1 or cond2` | logical OR |
| `!cond`, `not cond` | negation |
| `(cond)` | grouping |

Whitespace and newlines are ignored, so multi-line rules work. Strings are JSON-quoted.

## Important behavioural rules

### First write wins

Each classification slot is single-write. Once a rule sets `EXPORTER_GROUP`, no subsequent rule can change it. Order rules from most-specific to least-specific.

### Static metadata overrides classifiers entirely

If `metadata_static.exporters` set **any** exporter classification field for this exporter, **none of the exporter classifiers run**. Same for interfaces: if static metadata set any of provider, connectivity, or boundary for an interface, the interface classifiers do not run for that interface.

This is "Akvorado parity" behaviour — operator-provided classification has priority. Don't try to mix them on the same target.

### `Classify*` value normalisation

The string passed to `Classify*` actions is **lowercased and stripped to ASCII alphanumerics + `. + -`**. So `ClassifyRegion("EU West")` becomes `euwest`. If you want to preserve casing or whitespace, use `SetName` or `SetDescription` instead.

### Runtime errors stop the rule list

If a rule throws (e.g., comparing a string with `>`), the plugin stops evaluating further rules in that list and keeps whatever was set so far. Use `matches`, `startsWith`, or `contains` instead of `>`/`<` on string fields to avoid this.

### Cache key includes resolved values

The interface classifier cache keys by `(exporter, exporter classification, interface)`. When the exporter's classification changes — for example, after you push new static metadata and restart — interface caches naturally invalidate.

The cache TTL is `classifier_cache_duration`, default 5 minutes (`enrichment.classifier_cache_duration`). It's a last-access TTL — entries live as long as they're queried.

## Rule examples

### Exporter classifiers

```yaml
enrichment:
  exporter_classifiers:
    # Group exporters by name pattern.
    - 'Exporter.Name matches "^edge-.*" && Classify("edge")'
    - 'Exporter.Name matches "^core-.*" && Classify("core")'

    # Site by IP prefix.
    - 'Exporter.IP startsWith "10.1." && ClassifySite("ny-dc1")'
    - 'Exporter.IP startsWith "10.2." && ClassifySite("par-dc1")'

    # Tenant computed from name.
    - 'ClassifyTenant(Format("tenant-%s", Exporter.Name))'

    # Pull a token out of the name with a regex.
    - 'ClassifyRegionRegex(Exporter.Name, "-([a-z]{2})-[0-9]+$", "$1")'

    # Drop traffic from a test exporter.
    - 'Exporter.IP startsWith "192.0.2." && Reject()'

  classifier_cache_duration: 5m
```

### Interface classifiers

```yaml
enrichment:
  interface_classifiers:
    # Provider from a description prefix.
    - 'Interface.Description startsWith "BACKBONE-LUMEN" && ClassifyProvider("Lumen")'
    - 'Interface.Description startsWith "BACKBONE-COGENT" && ClassifyProvider("Cogent")'

    # Mark transit links by description keyword and tag them external.
    - 'Interface.Description contains "TRANSIT" && ClassifyConnectivity("transit") && ClassifyExternal()'

    # Anything matching the IX peering pattern.
    - 'Interface.Description matches "(?i)^(IX|peering)-.*" && ClassifyConnectivity("peering") && ClassifyExternal()'

    # 100 Gbps interfaces are core uplinks.
    - 'Interface.Speed >= 100000000000 && ClassifyConnectivity("core")'

    # Use exporter classification to scope interface rules.
    - 'CurrentClassification.Role == "edge" && ClassifyExternal()'
```

## What can go wrong

- **A rule fails to parse and the plugin won't start.** Look at the journal — the error message includes the index and a parser context.
- **Classifiers aren't running on an exporter.** Likely cause: static metadata already set a classification field for that exporter, which suppresses all classifier rules for it.
- **A rule sets a value but it appears differently in the dashboard.** `Classify*` actions normalise (lowercase + strip non-alphanumeric). Use `SetName` for human-readable values.
- **The first rule in the list always wins.** First-write-wins per slot. Order rules from most-specific to least-specific.
- **A rule that worked at startup stops matching later.** Cached results expire after `classifier_cache_duration`. If you change rules, restart the plugin so the cache clears completely.
- **Comparison error stops processing.** Comparing a string with `>` throws — subsequent rules in the list are skipped. Use string-safe operators.
- **`ClassifyExternal` doesn't fire on the egress side.** Interface classifiers run twice — once for the input interface, once for the output. Both invocations see the same classifier list. If your rule sets `ClassifyExternal()` on a specific ifIndex, it applies whether that ifIndex is `IN_IF` or `OUT_IF`.

## What's next

- [Static metadata](/docs/network-flows/enrichment/static-metadata.md) — Declarative labelling that runs before classifiers.
- [GeoIP](/docs/network-flows/enrichment/ip-intelligence.md) — Country / city / AS-name labelling.
- [ASN resolution](/docs/network-flows/enrichment/asn-resolution.md) — How `SRC_AS` / `DST_AS` get filled in.
