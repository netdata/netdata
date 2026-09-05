# Metrics Fields: A Mirror Of The Charts

The Metrics section tells the reader which charts they will get and what one instance of each scope is. It is the one
part of the page that is a contract with the code, not prose: every row must match what the collector emits.

Rendering (`integrations/templates/metrics.md`): `## Metrics`, two fixed paragraphs explaining scopes, then
`metrics.description`, then one `### Per <scope name>` per `metrics.scopes[]` entry with its `description`, a labels
table (label, description), and a metrics table (Metric, Description when any row in the scope has one, Dimensions,
Unit, plus one column per `metrics.availability` value). Every cell is flattened to one line by the generator.
Collectors without scopes render `## Metrics` and `metrics.description` only. Prometheus profile pages with
`profile_coverage` render a generated coverage section instead and ignore `metrics.description`.

Shape rules for every field are in `SKILL.md` and `overview.md` section 1.

## 1. Metric Rows Mirror The Code

**Question:** which charts exist, and what does each measure?

- `name` is the chart context exactly as emitted (`postgres.connections_utilization`). `description` is the chart title
  exactly as defined in the code (V2 `charts.yaml` `title`, V1 chart definition): title case, no trailing period, no
  added sentence. `unit` is the code's unit string verbatim. `chart_type` is the code's type. `dimensions[].name` are
  the code's dimension names, all of them, in the code's order.
- The row is a drift check, not a place to explain: a description that is a sentence rather than the code's title is
  a defect. Anything a reader needs beyond the title (what a dimension means, how a value is derived, why it can be
  empty) goes to `metrics.description` when it applies to every chart, to the `detailed_description` of the option
  that controls it, or nowhere.
- Unit vocabulary is decided where charts are designed (the collector design and V2 skills), not here. If the code's
  unit is wrong, fix the code and then the row.
- Every context the collector emits with a static definition has a row, and every row has a context in the code.
  Contexts generated at runtime from profiles or discovered data are covered by `dynamic_context_prefixes` (section 4),
  not by rows.
- `dimensions[].description` is accepted by the schema but rendered nowhere (the table shows dimension names only).
  Leave it out, or treat it as a source comment that no reader sees.

## 2. Scopes

**Question:** what is one instance of this scope, and when does it exist?

- `name` is the entity in operator words, using the same words the charts and labels use: "database", "replication
  application", "AWS account and region", "device licensing". Not the code's struct name, not "global" for anything
  that has labels. `global` is the fleet's name for the instance-less scope; the generator rewrites it to
  "<Collector> instance" before rendering.
- `description` is required and is one sentence: what one instance of this scope is, and when the scope appears at
  all if it is conditional (a mode, an opt-in option, a matched profile). Exemplar: `s3check/metadata.yaml`
  ("Replication modes only.").
- Every label gets one line: what it identifies and an example value where the format is not obvious (`account_id`: the
  resolved AWS account of the target, a 12-digit id). Label descriptions are never empty.
- Scopes are ordered from the whole to the parts: the instance-less scope first, then entity scopes in the order an
  operator would look for them.

## 3. `metrics.description`: What Applies To Every Chart

**Question:** what do I need to know that holds for all of these charts at once?

Rendered once, above the scopes. It carries only what cannot live in a row and applies to every chart:

- label conventions shared by every chart (a `mode` or `reason` label with a fixed vocabulary, listed);
- gap-versus-zero semantics ("work that did not happen leaves a gap, not a zero");
- where the charts land (the job's virtual node when configured, otherwise the host);
- for profile-driven collectors: that charts come from the matched profiles, where the profile library is, and how to
  see what a given instance collects (the Metrics tab of its dashboard). Exemplar: `snmp/metadata.yaml`, first
  paragraph.

A few bullets or two short paragraphs. It never lists contexts, dimensions, or units (the scope tables do), never
repeats the overview, and never explains how metrics are collected (`method_description`). A `metrics.description`
that grows past a screen is a sign that rows or the overview are missing content, not that it needs headings.

**Profile-driven collectors** (charts come from profiles, so the scope tables cannot list them) MAY list their
profiles once, here and nowhere else on the page: one table for the default-enabled profiles and one for the opt-in
profiles, each row a linked profile name, its context prefix (`cloudwatch.ec2.*`), and one line of what it covers.
The overview keeps only area-level coverage (services by area), not the profile list. This is the profile-library
pointer done as a table; the size rule above does not count it. Exemplar: `cloudwatch/metadata.yaml`.

## 4. `dynamic_context_prefixes` And `availability`

- `dynamic_context_prefixes` declares every prefix under which the collector emits contexts that have no static row
  (profile-driven charts, discovered entity charts). One entry per prefix with a one-sentence `reason` in operator
  terms ("SNMP profiles emit device-specific contexts at runtime under the snmp namespace."). Static contexts that
  share the prefix still get rows.
- `availability` adds one column per value to every metrics table, marking which rows exist in which variant
  (product editions, a basic versus extended mode). Use it only when rows really differ by variant; the values are
  the variant names an operator recognizes.

## 5. Review Questions For This Family

- Does every row match the code's context, title, unit, type, and dimensions exactly? Is any row a sentence?
- Does every context with a static definition have a row, and is every runtime prefix declared with a reason?
- Is every scope named in operator words with a one-sentence description, and does every label have a line?
- Does `metrics.description` contain anything the scope tables or the overview already say?
