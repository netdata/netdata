# Description Authoring

Metadata descriptions are public product copy: they appear on Learn, in integration cards, in generated umbrella pages,
and in some in-app surfaces. Write them for an operator scanning a catalog, not for a developer reading implementation
notes. This file owns the two contracts that apply to every integration type: the catalog sentence and the generated
page meta description. The content of every collector `metadata.yaml` field is owned by
`.agents/skills/collectors-metadata-yaml/`.

## Catalog Description Contract

The Monitor Anything table reads no dedicated field. `integrations/gen_doc_collector_page.py` extracts the first
sentence of the generated `## Overview` section and falls back to `meta.monitored_instance.description` only when
overview text is unavailable. For collector-like integrations the first sentence of
`overview.data_collection.metrics_description` therefore **is** the catalog description; write it first, deliberately.

That first sentence must:

- start with an active user-facing verb or action phrase;
- describe what the integration is and what it monitors, enriches, exports, authenticates, or discovers;
- be stable without knowing the user's configuration, short enough for a table cell, in user-facing product language.

That first sentence must not:

- describe a configuration option, variable, default value, or setting, or start with setup language ("Set ...",
  "Configure ...", "When enabled ...");
- contain placeholders such as `<tier>`, `<key>`, or `[[ variables.foo ]]`;
- describe limits, sizing, retention, troubleshooting, or caveats;
- mention internal tests, implementation state, reviewer notes, or future work.

Required first-sentence style:

- Collectors: `Monitor <thing> ...`, `Collect <data> from <thing> ...`, `Keep an eye on <thing> ...`.
- Flow sources: `Collect network flow records from <protocol/exporter> ...`.
- Flow enrichment sources: `Enrich network flows with <fields/context> from <source> ...`.
- Flow labeling/classification sources: `Annotate network flows with <labels> from <source/rules> ...`.
- Exporters: `Export Netdata metrics to <destination> ...`.
- Service discovery: `Discover <targets> from <source> ...`.

Do not lead with the provider's publication mechanism (`AWS publishes ...`, `Set option ...`); the catalog sentence
first tells users what Netdata does for them.

## Generated Page Meta Description Contract

`integrations/gen_docs_integrations.py` emits one `description:` frontmatter field for every generated page in all ten
documentation modes: collector, flows, device, exporter, agent notification, cloud notification, logs, authentication,
secret store, and service discovery.

The generator resolves each description in this order:

1. Use the explicit override when one exists:
   - `meta.monitored_instance.description` for collector, flows, and device records;
   - `meta.description` for every other mode.
   - The value must already be trimmed. The generator rejects leading or trailing whitespace and emits accepted text exactly.
   - It never removes markup, collapses whitespace, truncates, or falls back to overview prose when an explicit value is invalid.
2. Otherwise, derive it mechanically from the first useful prose in the rendered overview.
   - Markdown and HTML are reduced to plain text.
   - Underscores are preserved rather than silently deleting part of an identifier. Final validation then rejects the derived value,
     requiring an explicit plain-text override that describes the concept without corrupting the identifier.
   - Sentences are included until the description reaches 50 characters.
   - Text longer than 160 characters is trimmed at a word boundary with a terminal Unicode ellipsis. Final validation rejects that
     incomplete result, so its source record needs a complete explicit override.
3. Fail generation when the result is missing, invalid, or duplicated by another generated page.

An explicit description MUST:

- be 50–160 characters;
- be already trimmed, with no leading or trailing whitespace;
- be one line of plain text with no C0/C1 control character, including tabs, no surrogate code point or Unicode line/paragraph
  separator, no Markdown-special character (`*`, `_`, `[`, `]`, `<`, `>`, `#`, backtick, or `~`), no CommonMark list marker or
  hyphen thematic break at the beginning, and no URL, double quote, or backslash;
- not begin with `- `, `+ `, `* `, or a one-to-nine-digit ordered-list marker such as `1.` followed by a space or `1)` followed by a space, and not consist only of
  three or more hyphens separated by optional spaces. Internal hyphens, plus signs, and digits remain valid plain text;
- be a complete statement: it must not end with `:`, the Unicode ellipsis `…`, or the ASCII ellipsis `...`, and every round
  parenthesis must be balanced. Nested balanced parentheses are valid;
- be unique across every generated integration page. Duplicate identity is case-insensitive and NFC-normalized, but accepted authored
  text is emitted exactly and is never silently normalized;
- accurately describe the specific integration in active, user-facing language.

Mechanical overview extraction and explicit author input are deliberately separate paths. Markdown stripping, whitespace
normalization, sentence selection, and word-boundary truncation apply only to a mechanically derived description.

Double quotes and backslashes are rejected because Learn's legacy frontmatter parser cannot preserve their escaping. Use unquoted wording
or Unicode typographic quotation marks when quotation is essential.

Use an explicit description only when the mechanical result is missing, too short, duplicated, or otherwise inaccurate.
Do not duplicate every overview sentence into metadata: the fallback is the normal path and the override is the exception.

The Monitor Anything catalog deliberately retains its existing overview-first precedence. Adding an explicit meta override
MUST NOT silently change a catalog table row.

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
  The `journal.tiers.<tier>.duration_of_journal_files` setting controls retention.
```

Good catalog description:

```yaml
metrics_description: |
  Collect network flow records from NetFlow exporters such as routers, switches, and firewalls.
```

## Validation

Before committing `metadata.yaml` changes:

1. Regenerate and validate the integration data:

   ```bash
   python3 integrations/gen_integrations.py
   python3 integrations/gen_docs_integrations.py --check
   python3 -m unittest integrations.tests.test_descriptions
   python3 -m unittest integrations.tests.test_collector_metadata
   ```

2. Regenerate `src/collectors/COLLECTORS.md`.
3. Read the table row description and generated page frontmatter for the integration. Both must answer "what is this
   integration?" without relying on setup context and stay useful when rendered alone in a list, card, or search result.
4. For a collector, run the review checklist in `.agents/skills/collectors-metadata-yaml/SKILL.md` over the rest of
   the page.
