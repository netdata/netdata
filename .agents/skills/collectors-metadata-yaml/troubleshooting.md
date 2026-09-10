# Troubleshooting Fields: The Known Errors Catalog

The reader has an error in front of them and the collector page open; the section exists so they find that error and
read the fix. It is written proactively for every failure path the collector has and grows from user reports.

Rendering (`integrations/templates/troubleshooting.md`): `## Troubleshooting` with three h3 groups on a collector page
(a fourth, Test Notification, exists only for agent notifications).

- `### Diagnostics`: template-provided for plugins that have them (go.d, python.d, charts.d): `#### Debug Mode` (the
  plugin debug command) and `#### Getting Logs` (`journalctl` and Docker log commands). Authors never write these.
- `### Known Errors`: one h4 per `troubleshooting.errors.list[]` entry. The heading is the entry's `error`; the body
  renders `when` (if present), then `cause`, then `fix`, always in that order and with those labels.
- `### Other Problems`: one h4 per legacy `troubleshooting.problems.list[]` entry (`name`, `description`). This group
  exists for entries not yet rewritten as errors; new content does not go here.

Schema (`integrations/schemas/shared.json`, `$defs.troubleshooting`): `errors.list[]` has `error` (required), `cause`
(required), `fix` (required), `when` (optional), `source` (optional link to the report). `problems` is optional and
legacy. Both lists are optional: a go.d, python.d, or charts.d collector with no entries still renders the Diagnostics
group; any other collector with no entries renders no Troubleshooting section at all.

Shape rules for every field are in `SKILL.md` and `overview.md` section 1.

## 1. One Entry Per Error

**Question:** I got this error. Why, and what do I do?

- `error` is the literal message as the collector emits it, in the form the operator sees it in the log or the UI,
  with the variable parts replaced by backticked placeholders: ``tls: failed to verify certificate: x509: certificate
  signed by unknown authority``, ``connection refused to `host:port` ``. The whole heading renders as text, so
  placeholders MUST be in backticks (an angle-bracket placeholder breaks the page build).
- When a failure has no stable message (a chart that stays empty, values that look wrong), `error` is the symptom in
  the operator's words: "No charts appear for `<service>`", "Values are zero although the service is busy". Symptom
  entries are the exception; most entries are messages.
- One entry per distinct message. Two messages with the same cause are two entries; the fix text may be identical.
- `when` names the situation in one sentence when it is not obvious from the message: the mode, the option that was
  set, the deployment ("TLS is enabled with a certificate the Agent host does not trust").
- `cause` is one or two sentences saying what is actually wrong, in operator terms: which option, permission, network
  path, or service state produces this message. Not the collector internals.
- `fix` is the action: the option to set with its value, the command to run, the grant to add, the setting to change on
  the monitored system. A code block for a config fragment or a command; one admonition (`:::caution`) only when the
  fix has a security or cost consequence (disabling TLS verification, widening a permission), stating the safer
  alternative first.
- `source` is the link to the user report (GitHub issue, forum thread) for entries added from the field. It is never
  the only content: the entry still carries the exact message, cause, and fix.

## 2. Which Errors To Write

Entries are written before users hit them, from the code, then extended from reports.

- Walk every failure path the collector has and write its entry: connection refused or timed out; DNS failure;
  TLS verification failure with a custom or self-signed certificate (`tls_skip_verify` off, or `tls_ca` unset);
  authentication rejected (wrong credentials, expired token, wrong auth scheme); permission or grant missing (the
  exact permission name in the fix); the monitored feature or endpoint disabled on the target (the enable step in the
  fix); an option value out of range or mutually exclusive with another; a quota, rate limit, or bound reached (the
  option that raises it, or how to split the job). Each collector has its own subset; the walk is against its code
  paths, not this list.
- Take every message from the code or from a reproduction, never from memory. A paraphrased message is not findable.
  For messages produced by a library (TLS, HTTP, SQL drivers), reproduce once and copy the text.
- Collectors that create, delete, or write remote objects write the entry for the state operators fear most: objects
  left behind, cleanup not happening, the job refusing to start because of a changed identity.
- Collectors with a metered API write the entry for cost higher than expected, pointing at the cost drivers in the
  overview and the options that reduce it.
- When a user reports an error that fits, add it with `source`, in the same shape, as part of fixing or answering the
  report. The page is the place operators look first; a fix that lives only in an issue thread is not found.

## 3. What Does Not Belong

- Explanations of how the configuration works (selection, precedence, inheritance). Those are `detailed_description`
  content in Setup; an error entry says only which setting to change.
- Internal bounds and their rationale. If the operator can hit the bound, the entry quotes the message and gives the
  option or the split; why the bound exists is developer documentation.
- The debug-mode and log commands. The Diagnostics group renders them; an entry may say "run the collector in debug
  mode (see Diagnostics)".
- Generic advice with no message and no check ("verify permissions", "check connectivity"). If it cannot be tied to a
  message or a specific symptom, it is not an entry.
- New prose under `problems`. That group is legacy; rewrite an existing entry as errors when you touch it, and delete
  it when it explained the configuration rather than a failure.

## 4. Review Questions For This Family

- Would the operator find every entry by pasting the message from their log into the page search?
- Is every `error` a literal message with backticked placeholders, or an explicit symptom where no message exists?
- Does every entry have a cause in operator terms and a fix that is an action, in that order?
- Was every message taken from code or a reproduction? Which failure paths of this collector have no entry yet?
- Did the change add anything under `problems`, or leave an entry there that could be an error now?
