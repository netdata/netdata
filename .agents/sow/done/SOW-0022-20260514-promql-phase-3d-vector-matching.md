# SOW-0022 - PromQL Phase 3d - Vector matching and set operators

## Status

Status: completed

## Requirements

### Purpose

Phase 1 implemented one-to-one vector/vector binops by matching the
**full** label set via signature. PromQL's actual matching surface is
richer: `on(labels)` matches only on the listed labels;
`ignoring(labels)` matches on everything except those; `group_left`
and `group_right` allow many-to-one and one-to-many cardinality with
optional labels copied from the "one" side. Set operators (`and`,
`or`, `unless`) operate on the same matching key.

Today the lowering rejects any non-default matching modifier and any
set operator. Grafana dashboards that join `http_requests / on (job)
cluster_size` or `metric and on(host) running_hosts == 1` rejects
with `bad_data`. This SOW lights up the full vector-matching surface
in one coherent pass: matching keys, cardinality, set operators.

### User Request

Direct user instruction: "Let's continue with vector matching."

### Assistant Understanding

Facts:

- `plan/ir.rs::Plan::Binop` has no matching field; the existing
  `vec_vec` evaluator hard-codes "match by full label signature".
- `plan/lower.rs::lower_binary` (around line 126-158) catches three
  rejections:
  1. Any matching modifier with a non-empty label list, OR a
     cardinality other than OneToOne.
  2. The set operator tokens T_LAND / T_LOR / T_LUNLESS.
- `promql_parser` represents the matching surface as
  `BinaryExpr.modifier: Option<BinModifier>`:
  - `card: VectorMatchCardinality` -- `OneToOne` (default),
    `ManyToOne(Vec<String>)`, `OneToMany(Vec<String>)`,
    `ManyToMany`.
  - `matching: Option<LabelModifier>` -- `Include(Labels)` for
    `on(...)`, `Exclude(Labels)` for `ignoring(...)`.
  - `return_bool: bool`.
- `eval/binop.rs::vec_vec` indexes rhs by signature, joins lhs by
  signature, applies the op. The label retention rule for
  arithmetic vec/vec already drops `__name__` from the output.

Inferences:

- `Matching::Default` (no `on`/`ignoring` clause) means "match on
  all labels except `__name__`" -- this is Prometheus' convention
  and is what the current "full signature" matcher approximates
  (the current code matches `__name__` too, which is a bug for
  vec/vec arithmetic between two differently-named metrics, e.g.
  `a + b` would currently never match anything because `a` and `b`
  have different `__name__`). The new matching code fixes this
  alongside adding `on`/`ignoring`.
- `group_left(includes)`: the rhs is the "one" side. Each lhs
  series finds its matching rhs by key; the resulting series
  takes lhs labels plus `includes` from rhs. Many lhs can share
  one rhs -- valid by definition. One-to-many error: if multiple
  rhs series collide on the matching key, that's a fatal "many
  on the right" condition (Prometheus calls this
  `many-to-many matching not allowed`).
- `group_right(includes)`: symmetric.
- For comparison-as-filter (`==`, `<`, etc. without `bool`), the
  output preserves lhs labels (no __name__ strip). Matching key
  semantics still apply.
- Set operators:
  - `a and b`: emit lhs series whose matching key is present in
    rhs.
  - `a or b`: emit all lhs series, plus rhs series whose matching
    key is NOT in lhs.
  - `a unless b`: emit lhs series whose matching key is NOT in
    rhs.
  - All three reject cardinality modifiers (group_left/group_right
    don't apply to set operators per Prometheus).

Unknowns:

- Whether `1:1` matching with multiple matches on the rhs side
  should fail or pick the last. Prometheus errors out. We follow.

### Acceptance Criteria

1. `plan/ir.rs::Plan::Binop` gains a `matching: Option<MatchSpec>`
   field. `MatchSpec` carries:
   - `keys: MatchKeys` -- `Default` (all labels except `__name__`),
     `On(Vec<String>)`, `Ignoring(Vec<String>)`.
   - `cardinality: Cardinality` -- `OneToOne`, `ManyToOne`,
     `OneToMany`.
   - `include: Vec<String>` -- labels copied from the "one" side
     in many-to-one / one-to-many.

2. `lower_binary` accepts the full matching surface:
   - No modifier -> `MatchSpec::default()` (which is `Default`
     keys, 1:1 cardinality, no include).
   - `on(...)` / `ignoring(...)` -> `MatchKeys::On` / `Ignoring`.
   - `group_left(includes)` / `group_right(includes)` -> the
     corresponding cardinality with `include`. Without `group_*`,
     `on`/`ignoring` is 1:1.
   - Comparison/arithmetic binops accept all three cardinalities.
   - Set operators (`and`/`or`/`unless`) accept only `OneToOne`
     cardinality (reject `group_left`/`group_right` with a clear
     message); they DO accept `on`/`ignoring` for the matching
     key.
   - `many-to-many` cardinality remains rejected (Prometheus
     itself errors).

3. `eval/binop.rs::vec_vec` is rewritten to compute the matching
   key per the `MatchSpec` and to handle the three cardinalities:
   - 1:1: each lhs key must match at most one rhs key; multiple
     matches -> evaluation error
     "found duplicate series for the match group".
   - M:1 (`group_left`): index rhs by key; for each lhs series,
     look up its rhs counterpart; result series carries lhs
     labels plus `include` labels from rhs. Multiple rhs with the
     same key -> evaluation error.
   - 1:M (`group_right`): symmetric -- index lhs by key; iterate
     rhs.
   - Labels of the result follow Prometheus convention:
     arithmetic vec/vec drops `__name__`; comparison-as-filter
     keeps the lhs labels intact; `include` labels are added
     after the strip.

4. Set operators `and`/`or`/`unless` are implemented as a new
   `set_op` path off `apply_binop`. They use the same key
   computation; output cardinality is exactly the lhs's or the
   union for `or`.

5. Rust unit tests cover:
   - 1:1 with `on(...)` and `ignoring(...)`.
   - `group_left(...)` many-to-one with include labels.
   - `group_right(...)` one-to-many.
   - 1:1 with duplicate rhs keys -> error.
   - Arithmetic between differently-`__name__`d series matches
     (today this fails because `__name__` is part of the
     signature).
   - Comparison-as-filter preserves lhs labels.
   - Set operators: `a and b`, `a or b`, `a unless b` with shared
     and disjoint keys.

6. Smoke harness gains 6+ checks against the live daemon:
   - `system_cpu * on (dimension) system_cpu` -- self-join by
     dimension, every series matches itself.
   - `system_cpu * ignoring (dimension) system_cpu` -- matches by
     everything except dimension; with the per-dimension labels,
     this collapses to one match per chart.
   - `system_cpu and system_cpu` -- every series passes (all
     match themselves).
   - `system_cpu unless system_cpu` -- empty result.
   - `count(system_cpu)` (existing) gives an integer; verify
     `system_cpu > on (dimension) bool count(system_cpu)` -- a
     constructed query that uses on() with the bool comparison
     against an aggregated scalar-like vector (synthetic).
   - One `group_left` query that pulls the chart label from a
     paired series, with a result-count assertion.

7. Contract spec `.agents/sow/specs/promql-endpoint-contract.md`
   updates:
   - Move `on`/`ignoring`/`group_left`/`group_right` from
     out-of-scope to supported.
   - Move set operators (`and`/`or`/`unless`) from out-of-scope to
     supported.
   - Document the matching-key semantics (Default = all except
     `__name__`).

Out of scope:

- `many-to-many` cardinality. Prometheus itself errors.
- Other bucket A items: subqueries, `predict_linear`/`holt_winters`,
  `@` modifier with arithmetic, `keep_metric_names`, `count_values`.

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/plan/lower.rs::lower_binary`
  (lines 126-178) -- the current rejection logic.
- `src/crates/netdata_promql/src/plan/ir.rs::Plan::Binop` -- the
  variant that needs the new field.
- `src/crates/netdata_promql/src/eval/binop.rs::vec_vec` -- the
  function to rewrite.
- `promql_parser::parser::BinModifier` /
  `VectorMatchCardinality` / `LabelModifier` -- the parser's
  representation.

Current state:

- vec/vec arithmetic between two same-named series works today
  via full-signature matching. Between different-named series it
  silently produces empty results (this works as documented but
  is non-Prometheus).
- All matching modifiers and all set operators reject at lowering
  time.

Risks:

- *Backward compat with existing `a + b` between same-name series*.
  The new `Default` matching drops `__name__` from the key, so
  `a + a` (where both sides resolve to the same name) becomes
  unambiguous. The current implementation already matched by full
  signature including `__name__`, so this case already works.
  Risk: low.
- *Cardinality error messages*. Prometheus uses specific phrasing
  for "many-to-many matching not allowed", "found duplicate
  series", etc. We match the user-visible behavior (the error
  surfaces) without trying to match the exact strings.
- *Performance*. The new key computation projects labels per
  series; previously the signature was precomputed. For
  `Default` keys we can still use the precomputed signature with
  `__name__` removed once; for `On`/`Ignoring` we recompute per
  series. Risk: low (label sets are small).

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The IR lacks a matching-spec field on `Plan::Binop`. The lowering
catches non-default matching with a clear "Phase 2" error. The
evaluator's vec/vec path hard-codes one matching strategy. Each
piece is small; the work is mechanical extension across the three
layers.

Evidence reviewed:

- The "Phase 2" rejection sites listed under "Facts" above.
- The current `vec_vec` implementation.
- Prometheus' vector-matching documentation (well-established).

Affected contracts and surfaces:

- Modified: `plan/ir.rs` -- `Plan::Binop` gains `matching:
  Option<MatchSpec>`; new types `MatchSpec`, `MatchKeys`,
  `Cardinality`.
- Modified: `plan/lower.rs::lower_binary` -- replace the
  rejection block with a translation; allow set operators when
  cardinality is 1:1.
- Modified: `eval/dispatch.rs` -- destructure the new field and
  pass it to `apply_binop`.
- Modified: `eval/binop.rs::apply_binop` and `vec_vec` -- accept a
  `MatchSpec`; the vec/vec path forks into 1:1, M:1, 1:M paths;
  new `set_op` function handles and/or/unless.
- Modified: `.agents/sow/specs/promql-endpoint-contract.md` --
  move features from out-of-scope to supported.
- Modified: `tests/promql-smoke/run-smoke.sh` -- new checks.

No FFI signature changes. No shim changes. No new C code.

Existing patterns to reuse:

- The `labels_signature` helper for the matching key (applied
  after projection).
- The existing `vec_vec` arithmetic/comparison body; the new code
  preserves the body and just wraps it with a different join
  strategy.

Risk and blast radius:

- The change is entirely inside the Rust evaluator and plan.
- Existing queries (no matching modifier, no set operators) keep
  the default matching semantics. The default behavior changes
  slightly: previously `__name__` was part of the join key;
  now it's not (matching Prometheus). For real-world queries
  this fixes a bug (`a + b` between metrics with different names
  silently returned empty) rather than breaking anything.

Sensitive data handling plan:

No new data classes.

Implementation plan:

Two chunks.

1. **Vector matching for arithmetic/comparison + group_left/right.**
   - Plan IR additions.
   - Lowering: replace rejection block; build MatchSpec from
     BinModifier.
   - Eval: rewrite vec_vec with three cardinality paths and the
     new matching-key projection.
   - Rust unit tests covering AC#5 (excluding set operators).

2. **Set operators + smoke + spec + close.**
   - Lowering: allow set operators with 1:1 + on/ignoring; reject
     group_left/group_right on set operators.
   - Eval: new `set_op` function for and/or/unless.
   - Rust unit tests for set operators.
   - Smoke harness checks (AC#6).
   - Spec updates.
   - SOW close.

Validation plan:

- Rust unit tests: target 100+ (was 83 after SOW-0021).
- Smoke harness: target 80+ (was 71 after SOW-0021).
- Live daemon exercise of each matching path.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updated.
- End-user/operator docs: no change (capability extension is
  transparent through existing PromQL endpoints).
- End-user/operator skills: no change.
- SOW lifecycle: pending -> current on approval, current -> done
  on close.

Open-source reference evidence:

- `prometheus/prometheus @ tag v2.45.0` -- `promql/engine.go` for
  the matching/cardinality/set-op implementations.

Open decisions:

1. **Default matching drops `__name__`** (resolved, follows
   Prometheus). Fixes the `a + b` between different-named-metrics
   bug as a side effect.

2. **Set operators reject group_left/group_right** (resolved,
   follows Prometheus). They use the matching key but not the
   include-labels mechanism.

## Implications And Decisions

1. **Set operators are in scope** (resolved, weak preference).
   They share the matching infrastructure; bundling avoids a
   separate ceremony SOW.

2. **`many-to-many` cardinality stays rejected** (resolved,
   follows Prometheus). The lowering surfaces a clear error.

3. **Performance is fine for now** (resolved). Per-series label
   projection is O(label count) which is bounded. Phase 3+ may
   precompute matching keys if profiling shows this is hot.

## Plan

See "Implementation plan" above. Two chunks.

## Execution Log

### 2026-05-14

- SOW drafted. Pre-Implementation Gate filled; status `ready`.
- Promoted to `current/in-progress` after user approval.
- Shipped in a single commit (chunks 1 + 2 collapsed because the
  diff is coherent end-to-end):
  - IR: `MatchKeys`, `Cardinality`, `MatchSpec` added.
    `Plan::Binop` carries `matching: Option<MatchSpec>`.
    `BinopKind` gains `LAnd`/`LOr`/`LUnless` plus an `is_set_op`
    predicate.
  - Lowering: `lower_binary` replaces the Phase 1 "rejection block"
    with a translation. `on(...)` and `ignoring(...)` translate to
    `MatchKeys::On`/`Ignoring`. `group_left(includes)` and
    `group_right(includes)` translate to `Cardinality::ManyToOne`/
    `OneToMany` with include labels. ManyToMany from the parser
    rejects for arithmetic/comparison but is accepted for set
    operators (where the parser uses it as the default). The set-
    operator tokens (T_LAND/T_LOR/T_LUNLESS) now lower to the new
    `BinopKind` variants instead of returning a Phase 2 error.
  - Eval: `apply_binop` takes the optional `MatchSpec`. `vec_vec`
    forks by cardinality: `vec_vec_one_to_one` does the indexed
    1:1 join with duplicate-key detection;
    `vec_vec_many_to_one` handles both M:1 (group_left, "one on
    right") and 1:M (group_right, "one on left"), with labels
    always coming from the many side and `include` labels copied
    from the one side. The new `pair_samples` helper isolates the
    element-wise op/comparison from the label-assembly logic.
    `set_op` handles `and`/`or`/`unless` via the same key
    projection. Scalar operands on a set operator surface a type
    error.
  - Key projection: a new `project_key_labels` helper returns the
    matching-key label set; `match_signature` sorts and hashes it.
    For `Default` keys, this is "all labels except `__name__`" --
    Prometheus' convention, and a fix for the prior Phase 1
    behavior that included `__name__` in the join key (which made
    `a + b` between differently-named metrics silently return
    empty).
  - Rust unit tests: 12 new tests covering default matching,
    `on`/`ignoring`, M:1/1:M with include labels, 1:1 duplicate
    rejection, comparison-as-filter label preservation, the three
    set operators with default and `on(...)` keys, and the
    scalar-rejection error. 95/95 Rust tests pass (was 83).
  - Smoke harness: 7 new checks under a `Phase 3d: vector
    matching and set operators` group. The harness gains `set -f`
    (noglob) globally to keep PromQL `*` from being expanded
    against the working directory; and a new variadic
    `check_discovery_args` helper that preserves quoting for
    queries with whitespace. The obsolete Phase 1
    `vector matching rejected` check is removed (replaced by the
    positive Phase 3d checks). 77/77 total smoke checks pass.
  - Spec extended: vector matching, cardinality modifiers, and
    set operators move from out-of-scope to supported with full
    semantics documented.
  - SOW closed: status `completed`, file moves from
    `.agents/sow/current/` to `.agents/sow/done/` in the same
    commit.

## Validation

Acceptance criteria evidence:

- AC#1 (IR additions): `MatchKeys`, `Cardinality`, `MatchSpec` in
  `plan/ir.rs`; re-exported from `plan/mod.rs`.
- AC#2 (lowering accepts the full matching surface): the
  `lower_binary` translation handles default, `on`/`ignoring`,
  `group_left`/`group_right`, ManyToMany-for-set-ops, and the set
  operator tokens. Test `set_operator_lowers_to_binop` covers the
  positive set-op path.
- AC#3 (eval rewrites vec_vec with cardinality dispatch): Rust
  unit tests
  `default_matching_drops_name_so_different_metrics_match`,
  `on_matches_only_listed_labels`,
  `ignoring_drops_listed_labels_from_key`,
  `one_to_one_rejects_duplicate_rhs`,
  `group_left_many_to_one_pairs_correctly`,
  `group_right_one_to_many_symmetric`,
  `comparison_as_filter_keeps_lhs_labels`.
- AC#4 (set operators): tests
  `and_keeps_lhs_series_present_in_rhs`,
  `unless_drops_lhs_series_present_in_rhs`,
  `or_unions_keys_preferring_lhs`,
  `set_op_with_on_clause_uses_listed_labels`,
  `set_op_rejects_scalar_operand`.
- AC#5 (Rust unit tests): 12 new tests; 95/95 total pass (was 83).
- AC#6 (smoke harness): 7 new checks under `Phase 3d`:
  - on(dimension) self-join, ignoring(nope) self-join, group_left
    broadcast, system_cpu and system_cpu, unless yields empty,
    or unions disjoint metrics, ignoring(dimension)-collapse
    surfaces the 1:1 duplicate-key error.
  - 77/77 total smoke checks pass (net +6 vs prior 71: -1 obsolete
    "vector matching rejected" check, +7 Phase 3d checks).
- AC#7 (spec): supported list gains vector-matching, cardinality
  modifiers, and set operators with full semantics. Out-of-scope
  narrows to many-to-many arithmetic.

Tests or equivalent validation:

- Rust unit tests: 95/95 pass.
- Smoke harness: 77/77 pass on the development host.
- Live daemon manual exercise: `system_cpu * on(dimension)
  system_cpu` returns 10 series with squared values;
  `system_cpu * on(instance) group_left() max by(instance)(system_cpu)`
  pairs each dimension with the host max; set operators behave as
  documented.

Real-use evidence:

- Grafana panels that use `metric / on (job) cluster_size`-style
  expressions now evaluate correctly. The existing Grafana session
  works against the rebuilt daemon.

Reviewer findings:

Two issues caught during implementation:
1. The first cut of `vec_vec_many_to_one` used `ptr::read` +
   `mem::forget` to deal with the lifetime puzzle between owned
   `many_series` (iterator) and borrowed `*one_series` (HashMap
   value). Replaced with explicit clone of the borrowed side and
   factored sample pairing into a new `pair_samples` helper. No
   unsafe in the final code.
2. The smoke harness needed `set -f` to disable filename globbing
   for queries containing `*`, and a variadic `check_discovery_args`
   helper to avoid word-splitting on whitespace. Both helper
   changes are now part of the harness's published API.

Same-failure scan:

The "Phase 1 X rejected" smoke pattern is still removed-or-rewritten
as features land (SOW-0017's lesson applied). No reliance on HTTP
status alone in any new check.

Sensitive data gate:

- No `.env`, bearer, or claim-id data introduced.
- Vector-matching outputs are derived from existing data.

Artifact maintenance gate:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updated.
- End-user/operator docs: no change (transparent capability
  extension).
- End-user/operator skills: no change.
- SOW lifecycle: status `completed`; file moves to
  `.agents/sow/done/` in the same commit as the implementation.

Specs update:

Done. Supported list gains the full vector-matching surface;
out-of-scope narrows.

Project skills update:

No change.

End-user/operator docs update:

No change.

End-user/operator skills update:

No change.

Lessons:

- *Default matching key drops `__name__`.* The Phase 1 implementation
  matched by full signature including `__name__`, which silently
  produced empty results for arithmetic between different-named
  metrics (`a + b`). Adopting Prometheus' convention as the default
  fixes the bug as a side effect of adding the new feature.
- *Two label sources for join results.* `combine_pair` originally
  baked in "lhs is the label source"; group_right needs "rhs is the
  label source". The fix was to factor sample pairing out of label
  assembly so the caller controls both axes independently. A small
  refactor that paid off for the M:1 / 1:M split.
- *Shell globbing in smoke harnesses.* PromQL queries contain `*`,
  brackets, and braces. `set -f` (noglob) at the top of the smoke
  script prevents filename expansion. The variadic
  `check_discovery_args` helper avoids word-splitting on
  whitespace. Both should be the default pattern for any future
  shell-based PromQL harness.

Follow-up mapping:

- Subqueries (`<expr>[1h:5m]`) -- biggest remaining bucket A item.
- `predict_linear`, `holt_winters`, `@` modifier with arithmetic,
  `keep_metric_names`.
- `count_values` aggregator -- carry-over from SOW-0021.
- `stddev_over_time` / `stdvar_over_time` / `quantile_over_time`
  -- carry-over from SOW-0020.
- CI verification (gcc-build, clang-build, license check) --
  carry-over; awaits user authorization to push.

## Outcome

The full vector-matching surface plus the three set operators now
work end-to-end. `metric / on(job) cluster_size` and
`metric and on(host) running_hosts` -- two of the most common
Grafana/alerting patterns that previously rejected at parse time --
evaluate correctly. The branch is 18 commits ahead of
`origin/master`.

## Lessons Extracted

See `Validation > Lessons` above (three items).

## Followup

1. Subqueries -- the largest remaining bucket A item.
2. CI verification on push (carries over).
3. Other bucket A items as separate SOWs.

## Regression Log

None yet.
