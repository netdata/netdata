# Setup Fields: Prerequisites, Options, Examples

The Setup section is where the reader who decided to run the collector goes to make it work. The template
(`integrations/templates/setup-generic.md`) renders, in this order: for go.d collectors with jobs, a fixed
UI-versus-file table and its own `:::important` admonition; `### Prerequisites` with one h4 per entry (or "No action
required."); `### Configuration` with `#### Options` (the `options.description` intro, then the table, then one h5 per
`detailed_description`); the fixed "via UI" and "via File" procedures built from `configuration.file`; then
`##### Examples` with one h6 per example (name, description, fenced YAML). A non-empty `setup.description` replaces
the whole structured section with free prose; short-form notification integrations use it, collectors do not.

Shape rules for every field (structure, admonitions, terms, Markdown safety, depth boundary) are in `SKILL.md` and
`overview.md` section 1. This file adds the per-field contract.

## 1. `configuration.file`

- `name` is the path the operator edits relative to the Netdata config directory (`go.d/<module>.conf`,
  `python.d/<module>.conf`, `netdata.conf`). It MUST be the file the collector actually reads; the template prints
  the `edit-config` command with it.
- `section_name` only for collectors configured inside `netdata.conf` (plugin sections); otherwise omit it.

## 2. `prerequisites`: One Action Each

**Question:** what must I do, outside this config file, before the collector can work?

Renders as one h4 per entry under `### Prerequisites`. Empty renders "No action required.", which MUST be true.

- One entry is one action the operator performs: enable a module on the monitored system, create a user, grant a
  role, open a port, install a package, enable a feature that publishes the data. The `title` is an imperative
  phrase naming that action ("Create a read-only PostgreSQL user", "Enable the status endpoint").
- The `description` answers, in order: why in one sentence, the exact steps (a command, a policy document, a config
  snippet, a console path), and how to verify it worked when verification is not obvious. Prose around a command or
  policy block stays short; the block does the work.
- Entries are ordered as the operator performs them. An entry that applies only in one mode or one deployment says so
  in its first sentence.
- Do NOT put here: descriptions of what the collector does with the grant (`method_description`), cost notes
  (`performance_impact`), option explanations (`detailed_description`), or troubleshooting of the step (Troubleshooting
  family, linked if needed). An entry that is half feature description is two things; split or route.
- No headings inside an entry (the template owns its h4). Lists, tables, code blocks, and one admonition are allowed.

## 3. `options.description`: The Line Above The Table

- One or two sentences on scope: whether options apply globally or per job, and where a shared block comes from
  (`update_every`, `autodetection_retry` defined globally). Nothing else.
- Tables, file locations, and profile directories do not belong here. Profile paths go to a prerequisite ("Install a
  custom profile") or to the profile format documentation; behavior goes to the overview.

## 4. `options.list`: The Rows

**Question:** what does each option do, what values does it take, what is the default, must I set it?

Renders as a table with columns Option, Description, Default, Required, preceded by a Group column when any row sets
`group`. A row's `name` becomes a link when the row has a `detailed_description`.

- `name` is the key as written in the config file. Nested options use the fleet's path notation
  (`credentials[].name`, `tls.skip_verify`), one row per leaf the operator can set.
- `description` is a table cell: no lists, tables, or paragraphs; inline code is fine. The generator joins line
  breaks into one line and turns every `|` into `/`, so write it as one line and avoid pipes (write "a or b", not
  "a | b"). It MUST be identical to the option's `description` in `config_schema.json`, and that text has one owner:
  `.agents/skills/collectors-go-design/config-schema.md` section 2 states what it says and how long it is,
  section 7 the standard option wording (`update_every`, `autodetection_retry`, `timeout`, `vnode`, credentials), and
  section 8 the alignment test that checks tabs and descriptions both ways. Write it once, there, and copy it here.
- `default_value` is what the code uses when the option is absent, written as the operator would write it (`60`,
  `yes`, `/var/run/redis.sock`, empty when there is none). It MUST agree with the schema `default` and the stock
  configuration file. A description never restates the default.
- `required: true` only when the collector does not work without the option. Not "recommended", not "usually set".
- `group` names the schema tab the option lives in, same spelling (or `Tab / Subgroup` when a nested concern needs
  its own doc group; the first segment is the tab). Rows of one group are contiguous so the table renders the group
  once, and groups appear in the tab order of the form. A collector without tabs omits `group` everywhere.

## 5. `detailed_description`: Depth Per Option

Renders as an h5 below the table, reached by the link on the option name. This is the only place on the page for
option depth.

- Use it when the description cannot carry a consequence, an interaction with another option, an
  allowed-values table, a format with an example, or a mode-specific meaning. One paragraph or one short list or one
  small table; if it needs sections, the option is over-scoped for one row and probably wants sub-options.
- Do NOT restate the description, the default, or the schema's `ui:help`; the three channels carry different content
  (the form's help is for someone filling a field, this is for someone reading the page).
- Do NOT explain precedence chains across many options here; if a collector has a resolution order that matters,
  explain it once in the row of the highest-level option and let the others link to it in one sentence.

## 6. `examples`: Scenarios To Copy

**Question:** which of these is my situation, so I can paste it and adjust one or two values?

Renders as one h6 per example: `name`, `description`, then the YAML.

- Every example is a scenario an operator is likely to have, complete enough to paste: a full `jobs:` block (or the
  collector's equivalent), the values that define the scenario, nothing that does not. The reader should recognize
  their case from the name alone.
- The first example is always the basic case: the minimal working job with the defaults. Then the common variants in
  the order operators meet them: authentication (basic, token, key), TLS (custom CA, self-signed, client certificate),
  a remote or non-default address, a proxy, multiple instances in one file. The standard go.d examples for these
  scenarios are fine as they are; reuse their names and shapes so readers find the same case on every page.
- Advanced options appear as scenarios too, not as feature tours: "Monitor only selected databases", "Attach the job
  to a virtual node", "Read credentials from a secret store". One or two options per example; an example that
  demonstrates five options demonstrates none.
- Tuning and cost examples ("Lower resolution to reduce cost", "Refresh discovery less often") belong only to
  collectors with a cost or tuning surface (metered cloud APIs, large-scale discovery). They are a special case, not a
  pattern to copy to other collectors.
- `name` states the scenario ("HTTPS with self-signed certificate"), never the mechanism or the option name.
  `description` says when this applies and, for a variant, what differs from the basic case; one or two sentences. It
  does not explain the configuration model (precedence, inheritance, what an omitted key defaults to); that is
  `detailed_description` territory.
- Configs are short enough to read without scrolling. Around thirty lines is the point to ask whether the example
  covers two scenarios. Values are realistic placeholders in the fleet's style (`127.0.0.1`, `${env:PASSWORD}`,
  `my-bucket`), never real identifiers.

## 7. Review Questions For This Family

- Is every prerequisite one action with an imperative title, and is anything that is not an action routed away?
- Is every row's description identical to the schema's (length per `config-schema.md` section 2), with the unit
  named, and no default restated?
- Does every `default_value` match the code, the schema, and the stock config?
- Does the reader find their case from the example names alone, and is the first example the basic one?
- Can each example be pasted as-is, and does each show only what its scenario needs?
- Are tuning or cost examples present only because this collector has a cost or tuning surface?
