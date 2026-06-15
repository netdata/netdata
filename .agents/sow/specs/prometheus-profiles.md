# Prometheus Chart Profiles And App Resolution

## Scope

This spec records the contract for the go.d `prometheus` collector's chart
profiles: the profile file format, how profiles are selected for a scraped
target, how a job's `<app>` context segment is resolved, and how a chart context
is assembled. It applies to `plugin/go.d/collector/prometheus` (runtime,
`chart_template`, `promprofiles`) and the stock profiles under
`config/go.d/prometheus.profiles/`.

## Chart Context Contract

- A prometheus chart context is `prometheus.<app>.<context_namespace>.<context>`.
  The framework skips empty segments, so an absent `<app>` or
  `<context_namespace>` collapses out.
- `<app>` is the per-job deployment/instance identity (the "where" axis). The
  Netdata UI parses this second segment and renders it as a section under the
  "Applications" menu, so distinct apps do not collapse into one section. This is
  an external UI contract; the segment value MUST remain a stable identity.
- `<context_namespace>` is the exporter-type axis (the "what" axis), e.g.
  `haproxy`. It is orthogonal to `<app>`.
- `<context>` is the leaf chart context (curated chart context, or, for autogen,
  the full scraped metric name).
- Menu hierarchy beneath `<app>` is driven by chart `family`, not by
  `<context_namespace>`.

## Profile File Format

- A profile is a YAML file under a profiles directory. Its identity (`Name`) is
  the file basename; the file carries no name field.
- Fields:
  - `match` (REQUIRED): a `pkg/matcher` simple-patterns expression matched
    against scraped metric family base names (the `# TYPE` names).
  - `app` (OPTIONAL): the application identity contributed to `<app>` resolution
    (see below). MUST match `^[a-z][a-z0-9_]*$` when set.
  - `template` (REQUIRED): a standard `charttpl.Group`, validated like any chart
    template (including its author-written `metrics:` visibility list).
- The file basename MUST also match `^[a-z][a-z0-9_]*$`.
- Exporter profiles (e.g. `haproxy`) SHOULD declare both `app:` and a root
  `context_namespace`. Shared/runtime profiles (e.g. `go_*`, `http_*`,
  `process_*`) SHOULD omit `app:` so they render under the job's resolved app.

## Catalog

- Stock profiles live under `config/go.d/prometheus.profiles/default/`. Operator profiles extend the stock set and can override a stock profile by providing a user profile with the same basename (user profile wins).
- Profiles are loaded from the configured directories into a catalog keyed by profile name (case-insensitive selection).

## Profile Selection

Selection is driven by `profiles.mode`:

- `auto`: keep every profile whose `match` hits at least one scraped family.
- `exact`: resolve the explicitly named profiles only. A named profile that is
  unknown, or that matches no scraped family, fails `Check`.
- `combined`: union of `auto` and the named profiles, de-duplicated.
- `none`: select nothing; the job renders with pure autogen.

Unselected families always fall through to the autogen base, so a profile never
suppresses metrics it does not chart.

## App Resolution

`<app>` for a job is resolved in this precedence:

1. the job's `app` config option (`Application`), when non-empty;
2. otherwise, the first selected profile (in selection order) that declares a
   non-empty `app:`;
3. otherwise, the job name.

- When two or more selected profiles declare different non-empty apps, the first
  wins and each differing one is logged once (operator hint to set the job
  `app`). The conflict check is skipped when the job `app` config already
  decides the result.
- The job `app` config and job name are used verbatim; only the profile `app:`
  field is format-validated.

## Context Assembly And Namespace Deduplication

- The autogen base spec sets its context namespace to `prometheus` (no app) or
  `prometheus.<app>` (resolved app non-empty). Autogen emits one chart per
  scraped metric at `prometheus[.<app>].<metric>`.
- Selected profile groups are appended to that base. A profile group's root
  `context_namespace` becomes the `<context_namespace>` sub-segment, yielding
  `prometheus.<app>.<context_namespace>.<context>`.
- When the resolved app EQUALS a profile group's root `context_namespace` (the
  common case where the app fell back to that profile's own `app:`), the merge
  clears the group's `context_namespace` on a local copy so the context stays
  `prometheus.<app>.<context>` and does not double to
  `prometheus.<app>.<app>.<context>`. The shared catalog profile MUST NOT be
  mutated.

## Consistency

- The job `app` config option is documented in `config_schema.json` and
  `metadata.yaml`; its description MUST reflect the resolution precedence above.
- Profile-mode chart contexts differ from `none`-mode autogen contexts only by
  the `<app>`/`<context_namespace>` segments; the leaf identity and dimensions
  are unchanged.
