# Prometheus profile schema: owners and non-derivable notes

A Prometheus profile is an envelope (`match`, `app`, `fallback_type`, `relabeling`, `autogen`) around one embedded
chart-template **group** (`template`). The collector supplies the chart-template `version`, `engine`, and the
`prometheus[.<app>]` context namespace, so a profile never carries a standalone-spec `version`, top-level `groups`, or
`engine` beside `match`. Unknown keys fail strict decoding. Every field is documented by a shipped repository document;
this file says which one owns what and adds only the notes none of them states.

## Owner map

| Field group | Owner (authoritative; read it for the full contract) |
|---|---|
| Envelope: `match` syntax and scope, `app` precedence, `fallback_type` precedence chain, `relabeling` placement, `autogen.selector` semantics, `template`, file naming, strict decoding, the runtime processing order | `src/go/plugin/go.d/collector/prometheus/profile-format.md`, "How profiles work" and "Top-level fields" |
| Stock contribution policy for `autogen.selector` and the relabel grammar (allowlists, exact denies, wildcard drop and rewrite forms, name provenance, identity preservation) | `profile-format.md`, "Stock contribution policy" under `autogen.selector`; every rule has a finding code in `src/go/tools/prometheus-profile-validation/README.md`, "What `PASS` establishes" |
| Chart template: groups, `chart_defaults`, charts, `instances.by_labels` and `optional_by_labels`, dimensions and naming modes, `label_promotion`, `aggregation`, `lifecycle`, `options`, selectors, the validation rules, engine-derived behavior (ID derivation, per-dimension algorithm, priority `<= 0` to `70000`, family composition, multiplier and divisor `0` to `1`) | `src/go/plugin/framework/charttpl/README.md`, "Field Reference", "Validation Rules", "Engine-Derived Behavior" (the design roles and consequences of `label_promotion` and `optional_by_labels` are in `chart-design.md`) |
| Relabel actions, the ordered stages, histogram and summary safety, profile precedence on the shared stream | `src/go/plugin/go.d/collector/prometheus/relabel/README.md` |
| Validator CLI, safe job policy, what `PASS` establishes, every warning class, the evidence boundary | `src/go/tools/prometheus-profile-validation/README.md` |
| Flattened metric names per Prometheus type and what the writer accepts or rejects | `metric-types.md` in this skill |
| Label roles, identity lattice, reducers, the optional-dimension two-route pattern, hierarchy, presentation | `chart-design.md` in this skill |
| Stock contribution policy on the chart-template surface (what to omit, what to state explicitly) | `SKILL.md`, "Stock contribution rule sheet" |

## Notes no owner states

- `metrics` on a group authorizes exact metric names for dimension selectors in that group and its descendants. It
  does not route, chart, keep, or drop a series. Declare a metric at the narrowest group that needs it: a broad root
  declaration lets unrelated subtrees select the same series by accident, and a narrow one makes ownership visible so
  the compiler can catch cross-subsystem mistakes. Remove declarations that no authored dimension in scope uses; they
  do not preserve denied or writer-rejected data and make a stale or excluded family look intentionally covered.
- Exact-mode validation forces the candidate profile by name, so a `PASS` in that mode does not prove that automatic
  profile selection is unique or safe for the exporter.
- Two templates can derive the same rendered chart ID even when their YAML paths differ, and chart IDs, contexts, and
  dynamic dimension names are sanitized again at public wire emission. The validator reports empty values and distinct
  effective values that collapse to one wire value as objective failures. Intentional reuse of the same raw context
  across charts remains a dashboard-design judgment, not a uniqueness error.
- The profile schema has no top-level `drop:` shorthand and no computed `average` dimension. Exporter-owned exclusions
  go through profile `relabeling` after selection; deployment policy and anything that must happen before selection go
  through the job selector or job relabeling. Chart only values the collector writes; never document or emit
  speculative keys.
- The runtime format is a general user-facing surface. The restrictions in the stock rule sheet are contribution
  policy, not parser limitations; a deployment-owned user profile may use the full surface when its consequences are
  intended.
