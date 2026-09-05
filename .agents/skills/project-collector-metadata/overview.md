# Overview Fields: What The Operator Reads First

The Overview is the top of the integration page. The template (`integrations/templates/overview/collector.md`) renders,
in this order: `metrics_description`, `method_description`, a platform sentence from `supported_platforms`, an instance
sentence from `multi_instance`, `additional_permissions`, the related integrations list when `related_resources` names
any (linked names only), then a "Default Behavior" h3 with three h4s: Auto-Detection, Limits, Performance Impact. The
reader has not scrolled yet. Everything here is decided in the first screen.

Shape rules that apply to every field on this page are in `SKILL.md` (reading model, depth boundary, Markdown safety).
This file adds the per-field contract: the question the field answers, what belongs, what does not, and what an empty
field means.

## 1. Structure Inside A Field

- No Markdown headings inside a field. The template owns h2 to h6 and the page table of contents; an author heading
  breaks both.
- A bold lead-in line MAY caption a list or a table (`**Timing**` above a list). At most two or three per field. A field
  that needs more sections is over-scoped: apply the depth boundary, do not add structure.
- One idea per paragraph, a few sentences. A paragraph that enumerates becomes a list. Three or more items that share
  attributes (modes, permissions per endpoint, services per area, prerequisites per mode) become a table.
- Admonitions carry what the reader must not miss and nothing else: `:::caution` for cost, data loss, destructive or
  irreversible behavior; `:::tip` for a recommended shortcut ("need X, do Y"); `:::note` for a non-obvious fact that is
  not a warning. `:::info` is the fleet's catch-all and is discouraged. At most one admonition per field. Prefer an
  admonition over a blockquote. An admonition never replaces the field's own first paragraph.
- Define a term inline the first time it appears; never ship engine vocabulary. A glossary table is allowed only when
  four or more terms recur across the page and the options table, and then it closes `method_description`, after the
  reader knows what the collector does, never before.
- Links go to user-facing pages only: another integration, a `profile-format.md`, a `docs/guides` page, Learn, vendor
  documentation. Never to `ARCHITECTURE.md`, source files, or tests.

## 2. `metrics_description`: The Overview Proper

**Question:** what is this and what do I get? This is the general overview, not a list of metrics, and the only text
most visitors read.

- Sentence one is the catalog row and the meta description (`integrations-lifecycle/description-authoring.md` owns its
  mechanics). It MUST say what the collector monitors in an action phrase and MUST NOT mention options, defaults,
  limits, or setup.
- Then, in decreasing importance: what the reader gets (areas or scopes covered, the capabilities that distinguish the
  collector), and for a multi-service or multi-device collector one coverage table (area, services) or one pointer to
  the profile library. Exemplars: `azure_monitor/metadata.yaml` (capability list), `s3check/metadata.yaml` (mode
  table), `cloudwatch/metadata.yaml` (coverage table).
- Do NOT put here: how collection works (`method_description`), configuration or file paths (Setup), permissions
  (`additional_permissions`), cost (`performance_impact`), a glossary, shell commands, or a request-a-feature
  procedure. Each of those has an owner further down the page or outside it.

## 3. `method_description`: How It Gets The Data

**Question:** how does it get that, as I will see it from the outside?

- Say what the collector connects to or executes, with what protocol or API, how often, how it authenticates, and what
  it never touches. Name the exact endpoints, commands, files, or API operations when they matter to the operator
  (firewall rules, audit logs, permissions). Exemplar: `azure_monitor/metadata.yaml` (three sentences: API, discovery
  source, authentication).
- For a collector that writes, deletes, or creates remote objects, this field states the boundary of what it touches
  and the direction of the work (which side is written, which is only read) in one paragraph; the safety details
  belong to Setup prerequisites. Exemplar: `s3check/metadata.yaml`, first paragraph of the field.
- Do NOT describe internal stages, caching, ownership resolution, or state handling. Those are `ARCHITECTURE.md`
  content (developer documentation, never linked from the page). "Plan, discover, query" is how the code is organized,
  not how the operator experiences it.

## 4. `supported_platforms` And `multi_instance`

These are data, rendered as fixed sentences.

- `supported_platforms.include` or `.exclude` MUST reflect where the collector actually runs; an empty pair renders
  "supported on all platforms". A collector that reads Linux-only files or Windows-only APIs MUST list its platforms.
- `multi_instance: false` only when one Agent can monitor exactly one instance (a host-local kernel source). Anything
  that takes an address or URL is multi-instance.

## 5. `additional_permissions`: Exactly What To Grant

**Question:** what do I have to grant beyond what the Netdata process already has?

- List each permission, capability, role, or grant with when it is needed. Two or more items, or any item that depends
  on configuration, MUST be a table (permission, needed when). Exemplars: `cloudwatch/metadata.yaml`,
  `azure_monitor/metadata.yaml`.
- One sentence on scope (bucket, prefix, subscription, resource group) and one on what the collector never does with
  the grant are welcome; nothing else is.
- Empty renders nothing. Leave it empty only when the Netdata process's own privileges are sufficient.

## 6. `default_behavior.auto_detection`: What Happens With No Configuration

**Question:** if I install Netdata and do nothing, what does this collector find, and how?

Empty renders "This integration doesn't support auto-detection." That sentence MUST be true when the field is empty.
Three mechanisms make it false; describe every one that applies:

- Built-in defaults: the collector probes fixed local addresses, sockets, or files at startup. List them exactly (the
  URLs, socket paths, ports). Exemplars: `nginx/metadata.yaml`, `postgres/metadata.yaml`, `redis/metadata.yaml`.
- Service discovery: the module is covered by a rule in `src/go/plugin/go.d/config/go.d/sd/*.conf` (`services[].id`
  is the module name unless the rule's `config_template` sets `module:`). The field MUST say which discoverer
  (`net_listeners`: local listening ports and process names; `docker`: container image names), what it matches, that a
  job is created automatically when it matches, and the rule file the operator edits (`go.d/sd/<file>.conf`).
  `http.conf` is not collector-specific (an operator-defined job feed, disabled by default) and does not count.
  `integrations/tests/test_collector_metadata.py` fails when a covered collector leaves the field empty; its backlog
  list names the collectors that still do (26 when the rule was written) and only shrinks.
- Collector-specific discovery: the collector ships its own discoverer (SNMP network scanning). Describe the mechanism,
  whether it is enabled by default, what it needs (credentials, ranges), and its configuration file, as
  `snmp/metadata.yaml` does.

Do NOT explain the configuration model's precedence rules, what an omitted option defaults to per field, or caching
internals. Those are Setup (option rows and `detailed_description`) or nothing.

## 7. `default_behavior.limits`: The Bounds You Will Hit

**Question:** with the defaults, what caps, quotas, or floors will I run into, and how will I notice?

Empty renders "The default configuration for this integration does not impose any limits on data collection." That
MUST be true when the field is empty.

- Name the operator-visible bounds: minimum collection interval, maximum instances or series, API quotas the collector
  respects, retention or lookback windows, and what happens at the bound (charts stop appearing, the job refuses to
  start, a warning is logged). Give the option that changes each bound when one exists. Exemplar:
  `azure_monitor/metadata.yaml` (three bullets: interval, delay, throttling).
- Do NOT list internal safety bounds nobody configures (page counts, byte budgets, atomic refresh semantics) or derive
  formulas. Those are `ARCHITECTURE.md` content.

## 8. `default_behavior.performance_impact`: What The Defaults Cost

**Question:** what do the defaults cost, on the Agent host and on the monitored system or my bill, and which knob
changes it?

Empty renders "The default configuration for this integration is not expected to impose a significant performance
impact on the system." That MUST be true when the field is empty; a collector that calls a metered API or issues
heavy queries fills it.

- State the impact on the Agent host (usually negligible; say so in one sentence) and on the monitored system: request
  counts per collection, query weight, connections held. Name the one or two options that reduce it. Exemplar:
  `azure_monitor/metadata.yaml` (one paragraph plus a defaults table).
- Do NOT document the collector's own activity charts here beyond one sentence pointing at them; the charts are
  described in their metric `description` (Metrics family).

**Cost carve-out.** When the defaults can cost the operator money (a metered cloud API billed per request or per
metric), cost is the most important content on the page and this field MUST be explicit and complete, not brief:

- Open with the field's one `:::caution` naming the billing model (what is billed, at what unit) with a link to the
  provider's pricing page.
- Then, structured: what drives cost (a table or list: instances, metrics, statistics, request frequency, lookback);
  how to estimate it before running (a formula with one worked example is welcome); how to observe it while running
  (the collector's own activity charts, one sentence each); and the options that reduce it, each with its effect.
- Section 1 still applies except the caption cap: the four blocks above are the required structure, one bold caption
  each. Internal accounting that changes no operator decision (estimate attribution across profiles, non-additivity
  proofs) still leaves the page.

## 9. Review Questions For This Family

- Can a reader who stops after `metrics_description` say what the collector is and what they get?
- Does `method_description` name what the collector talks to and how, without a single internal term?
- Is every permission the code requests in the table, and nothing that it does not?
- Is the collector covered by a `go.d/sd/*.conf` rule? Then `auto_detection` names the discoverer and the rule file.
- Are `limits` and `performance_impact` either true-as-placeholder or filled with operator-visible bounds and costs?
- Are there headings, more than one admonition, or a glossary before the first capability? Then restructure or route.
