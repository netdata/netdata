// SPDX-License-Identifier: GPL-3.0-or-later
//
// Plan IR: a typed, executable representation of a PromQL query lowered
// from the `promql_parser` AST. The IR mirrors the AST nodes the evaluator
// supports, with type errors caught at lowering time rather than at
// execution.

use std::sync::Arc;

use crate::storage::Matcher;

/// PromQL value type. Determined for each Plan node at lowering time so
/// the evaluator can dispatch without runtime type checks.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ValueType {
    Scalar,
    InstantVector,
    RangeVector,
    String,
}

/// Aggregation operators supported in the evaluator.
///
/// Non-parametrized aggregators (`Sum`/`Avg`/`Min`/`Max`/`Count`/`Stddev`/
/// `Stdvar`/`Group`) take one instant-vector argument. Parametrized
/// aggregators (`TopK`/`BottomK`/`Quantile`/`LimitK`/`LimitRatio`) take a
/// scalar parameter. `CountValues` takes a string parameter (the output
/// label name).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AggrKind {
    Sum,
    Avg,
    Min,
    Max,
    Count,
    TopK,
    BottomK,
    Quantile,
    CountValues,
    // Missing-aggregators batch.
    /// `stddev(v)` — population standard deviation per group.
    /// Distinct from `stddev_over_time`, which is a rollup.
    Stddev,
    /// `stdvar(v)` — population variance per group.
    Stdvar,
    /// `limitk(k, v)` — first k series per group in signature
    /// order (Prometheus 2.40+).
    LimitK,
    /// `limit_ratio(ratio, v)` — deterministic-random selection
    /// of approximately `ratio` of input series per group
    /// (Prometheus 2.40+).
    LimitRatio,
    /// `group(v)` — emits 1 per output bucket whenever any
    /// input series had a non-NaN value. Used as a join-key
    /// fabricator.
    Group,
}

impl AggrKind {
    /// True when the operator requires a scalar parameter
    /// (e.g. `topk(5, expr)`). Non-parametrized aggregators
    /// reject any param at lowering time.
    pub fn takes_param(self) -> bool {
        matches!(
            self,
            AggrKind::TopK
                | AggrKind::BottomK
                | AggrKind::Quantile
                | AggrKind::LimitK
                | AggrKind::LimitRatio
        )
    }

    /// True when the operator requires a string parameter (the
    /// label name for `count_values("label", expr)`). String- and
    /// scalar-param aggregators are mutually exclusive.
    pub fn takes_string_param(self) -> bool {
        matches!(self, AggrKind::CountValues)
    }
}

/// Binary operators supported in the evaluator.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BinopKind {
    // Arithmetic
    Add,
    Sub,
    Mul,
    Div,
    Mod,
    Pow,
    // Comparison
    Eq,
    Ne,
    Lt,
    Le,
    Gt,
    Ge,
    // Set operators. These take vector/vector operands and a matching
    // spec; they do not accept the `bool` modifier or the
    // `group_left`/`group_right` cardinality modifiers.
    LAnd,
    LOr,
    LUnless,
}

impl BinopKind {
    pub fn is_comparison(self) -> bool {
        matches!(
            self,
            BinopKind::Eq
                | BinopKind::Ne
                | BinopKind::Lt
                | BinopKind::Le
                | BinopKind::Gt
                | BinopKind::Ge
        )
    }

    pub fn is_set_op(self) -> bool {
        matches!(self, BinopKind::LAnd | BinopKind::LOr | BinopKind::LUnless)
    }
}

/// The `@` modifier on a vector or matrix selector. When present, the
/// selector evaluates at the resolved timestamp instead of the
/// EvalContext's grid.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AtMod {
    /// Fixed Unix timestamp in milliseconds.
    AtTs(i64),
    /// Outer query range's start; for an instant query, equals the
    /// query time.
    Start,
    /// Outer query range's end; for an instant query, equals the
    /// query time.
    End,
}

/// Grouping clause for an aggregation. `By` keeps the listed labels;
/// `Without` drops them.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Grouping {
    By(Vec<String>),
    Without(Vec<String>),
}

/// Vector-matching join key.
///
/// `Default` matches by every label except `__name__` (Prometheus
/// convention). `On(labels)` matches by exactly the listed labels.
/// `Ignoring(labels)` matches by everything except the listed labels
/// (and `__name__`).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MatchKeys {
    Default,
    On(Vec<String>),
    Ignoring(Vec<String>),
}

/// Cardinality of a vector/vector binary operation.
///
/// `OneToOne` is the default (no `group_left` / `group_right`). Each
/// lhs series matches at most one rhs series; duplicates on either
/// side are an error.
///
/// `ManyToOne` (`group_left`) treats rhs as the "one" side: each
/// matching rhs key can be paired with many lhs series. Multiple rhs
/// series with the same key is still an error.
///
/// `OneToMany` (`group_right`) is symmetric.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Cardinality {
    OneToOne,
    ManyToOne,
    OneToMany,
}

/// Vector-matching spec for `Plan::Binop`.
///
/// `include` is the optional list of labels copied from the "one"
/// side onto the result series in `ManyToOne` / `OneToMany`. Unused
/// when cardinality is `OneToOne`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MatchSpec {
    pub keys: MatchKeys,
    pub cardinality: Cardinality,
    pub include: Vec<String>,
}

impl Default for MatchSpec {
    fn default() -> Self {
        Self {
            keys: MatchKeys::Default,
            cardinality: Cardinality::OneToOne,
            include: Vec::new(),
        }
    }
}

/// PromQL functions supported by the evaluator.
///
/// Each variant maps to a function callable in PromQL expressions.
/// [`FuncKind::from_name`] resolves a function name string to its variant.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FuncKind {
    Rate,
    IRate,
    Increase,
    Delta,
    HistogramQuantile,
    AvgOverTime,
    SumOverTime,
    MinOverTime,
    MaxOverTime,
    CountOverTime,
    LastOverTime,
    FirstOverTime,
    PresentOverTime,
    StddevOverTime,
    StdvarOverTime,
    QuantileOverTime,
    PredictLinear,
    HoltWinters,
    // Per-sample math transforms on instant vectors.
    Abs,
    Ceil,
    Floor,
    Sgn,
    Ln,
    Log2,
    Log10,
    Exp,
    Sqrt,
    // Bounded transforms / rounding.
    Clamp,
    ClampMin,
    ClampMax,
    Round,
    // Vector restructuring.
    Vector,
    Scalar,
    Sort,
    SortDesc,
    // Timestamp / time.
    Time,
    Timestamp,
    // Range-vector reductions.
    Deriv,
    IDelta,
    Changes,
    Resets,
}

impl FuncKind {
    pub fn from_name(name: &str) -> Option<FuncKind> {
        Some(match name {
            "rate" => FuncKind::Rate,
            "irate" => FuncKind::IRate,
            "increase" => FuncKind::Increase,
            "delta" => FuncKind::Delta,
            "histogram_quantile" => FuncKind::HistogramQuantile,
            "avg_over_time" => FuncKind::AvgOverTime,
            "sum_over_time" => FuncKind::SumOverTime,
            "min_over_time" => FuncKind::MinOverTime,
            "max_over_time" => FuncKind::MaxOverTime,
            "count_over_time" => FuncKind::CountOverTime,
            "last_over_time" => FuncKind::LastOverTime,
            "first_over_time" => FuncKind::FirstOverTime,
            "present_over_time" => FuncKind::PresentOverTime,
            "stddev_over_time" => FuncKind::StddevOverTime,
            "stdvar_over_time" => FuncKind::StdvarOverTime,
            "quantile_over_time" => FuncKind::QuantileOverTime,
            "predict_linear" => FuncKind::PredictLinear,
            "holt_winters" => FuncKind::HoltWinters,
            "abs" => FuncKind::Abs,
            "ceil" => FuncKind::Ceil,
            "floor" => FuncKind::Floor,
            "sgn" => FuncKind::Sgn,
            "ln" => FuncKind::Ln,
            "log2" => FuncKind::Log2,
            "log10" => FuncKind::Log10,
            "exp" => FuncKind::Exp,
            "sqrt" => FuncKind::Sqrt,
            "clamp" => FuncKind::Clamp,
            "clamp_min" => FuncKind::ClampMin,
            "clamp_max" => FuncKind::ClampMax,
            "round" => FuncKind::Round,
            "vector" => FuncKind::Vector,
            "scalar" => FuncKind::Scalar,
            "sort" => FuncKind::Sort,
            "sort_desc" => FuncKind::SortDesc,
            "time" => FuncKind::Time,
            "timestamp" => FuncKind::Timestamp,
            "deriv" => FuncKind::Deriv,
            "idelta" => FuncKind::IDelta,
            "changes" => FuncKind::Changes,
            "resets" => FuncKind::Resets,
            _ => return None,
        })
    }

    pub fn return_type(self) -> ValueType {
        match self {
            // `scalar(v)` is the one function whose return type is
            // genuinely scalar rather than instant vector. `time()`
            // is also scalar at the language level but Prometheus
            // represents it as a scalar -- we match.
            FuncKind::Scalar | FuncKind::Time => ValueType::Scalar,
            _ => ValueType::InstantVector,
        }
    }
}

/// Label-mutation kind. Carried by [`Plan::LabelOp`] rather than being
/// a regular [`FuncKind`] because these operators take string-literal
/// arguments (not lowerable Plans), and the lowering layer pre-extracts
/// them.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum LabelOpKind {
    /// `label_replace(v, dst, repl, src, regex)`. The regex is
    /// anchored to the full label value; capture groups `$1`, `$2`,
    /// etc. expand inside `replacement`.
    Replace {
        dst: String,
        replacement: String,
        src: String,
        regex: String,
    },
    /// `label_join(v, dst, sep, src1, src2, ...)`. Concatenates the
    /// listed source label values with `sep` into `dst`.
    Join {
        dst: String,
        sep: String,
        srcs: Vec<String>,
    },
}

/// The Plan IR — a typed, executable representation of a PromQL query.
///
/// Cheap to clone (`Arc` for heap-allocated children), so the parse cache
/// can hand out copies safely. Each variant carries a known [`ValueType`]
/// resolved at lowering time.
#[derive(Debug, Clone)]
pub enum Plan {
    /// A scalar number literal.
    Number(f64),

    /// An instant-vector selector. Matchers include `__name__` if a name
    /// was given.
    VectorSelect {
        matchers: Vec<Matcher>,
        /// Positive offset shifts the evaluation backwards in time; negative
        /// forwards.
        offset_ms: i64,
        /// `@` modifier. When `Some`, the selector evaluates at the
        /// resolved timestamp instead of the EvalContext's grid.
        at: Option<AtMod>,
    },

    /// A range-vector selector. The window is `[t - range_ms, t]` shifted
    /// by `offset_ms`.
    MatrixSelect {
        matchers: Vec<Matcher>,
        range_ms: i64,
        offset_ms: i64,
        /// `@` modifier; same semantics as on VectorSelect.
        at: Option<AtMod>,
    },

    /// Unary negation. Operand is Scalar or InstantVector.
    UnaryMinus(Arc<Plan>),

    /// Binary operation. The Plan IR distinguishes type combinations at
    /// lowering time: scalar/scalar, scalar/vector, vector/scalar,
    /// vector/vector. Carries the kind, the `bool` modifier for
    /// comparisons, and the vector-matching spec.
    Binop {
        op: BinopKind,
        /// True for `comparison bool` operators which return 0/1 rather
        /// than filtering.
        return_bool: bool,
        /// Vector-matching spec. `None` for scalar/scalar; `Some` for
        /// any binop that touches a vector, with default match keys
        /// and 1:1 cardinality when no `on`/`ignoring`/`group_*`
        /// clause is present.
        matching: Option<MatchSpec>,
        lhs: Arc<Plan>,
        rhs: Arc<Plan>,
    },

    /// Aggregation over an instant vector.
    Aggregate {
        op: AggrKind,
        grouping: Option<Grouping>,
        /// Scalar parameter for topk/bottomk/quantile (the `k` or `phi`).
        /// `None` for everything else.
        param: Option<Arc<Plan>>,
        /// String parameter for `count_values("label", expr)`. The
        /// supplied label name is used as the key for the value-bucket
        /// label on each output series. `None` for every other operator.
        param_string: Option<String>,
        expr: Arc<Plan>,
    },

    /// Function call: rate(), histogram_quantile(), etc.
    Call { func: FuncKind, args: Vec<Plan> },

    /// Subquery `<expr>[range_ms:step_ms] [@<ts>] [offset <d>]`. The
    /// inner expression must lower to `InstantVector`; evaluating the
    /// subquery yields a `RangeVector` by re-evaluating the inner at
    /// every grid point in `[t - range_ms, t]` at `step_ms` stride.
    Subquery {
        expr: Arc<Plan>,
        range_ms: i64,
        /// Resolution. Parser permits `None` (defaults to the global
        /// evaluation interval upstream); we default to 1000 ms at
        /// lowering time.
        step_ms: i64,
        offset_ms: i64,
        at: Option<AtMod>,
    },

    /// `label_replace` / `label_join`. Carries the pre-extracted string
    /// args alongside the input expression.
    LabelOp { op: LabelOpKind, expr: Arc<Plan> },

    /// `absent(v)` / `absent_over_time(v[w])`. When the inner returns
    /// empty, emits a single series with `labels` taken from the inner's
    /// static `=` matchers (when the inner is a vector/matrix selector)
    /// or an empty label set (when the inner is a more complex expression).
    /// The evaluator decides instant-vs-range based on `expr.value_type()`.
    Absent {
        labels: Vec<(String, String)>,
        expr: Arc<Plan>,
    },

    /// `aggr(rollup(matrix-selector | subquery))` fused into a single
    /// streaming pass. Emitted by the lowering layer when the inner
    /// expression matches the pattern and both the aggregator and the
    /// rollup are on the supported list (see `eval::fused`).
    ///
    /// The unfused `Plan::Aggregate{expr: Plan::Call{...}}` shape is
    /// still emitted for combinations outside the supported list
    /// (parametrized aggregators, predict_linear/holt_winters/etc.).
    FusedAggrRollup {
        aggr: AggrKind,
        grouping: Option<Grouping>,
        rollup: FuncKind,
        /// Selector source. Either a vector/matrix-selector's matchers
        /// directly, or a subquery whose inner expression supplies the
        /// per-step series. The two variants below carry the same
        /// `(range_ms, offset_ms, at)` window descriptors that the
        /// unfused matrix-selector / subquery nodes carry.
        source: FusedSource,
    },
}

#[derive(Debug, Clone)]
pub enum FusedSource {
    /// A matrix selector inside the fused rollup -- the canonical
    /// `aggr(rate(metric[range]))` shape.
    Matrix {
        matchers: Vec<Matcher>,
        range_ms: i64,
        offset_ms: i64,
        at: Option<AtMod>,
    },
    /// A subquery inside the fused rollup -- `aggr(rate(<expr>[range:step]))`.
    Subquery {
        expr: Arc<Plan>,
        range_ms: i64,
        step_ms: i64,
        offset_ms: i64,
        at: Option<AtMod>,
    },
}

impl Plan {
    pub fn value_type(&self) -> ValueType {
        match self {
            Plan::Number(_) => ValueType::Scalar,
            Plan::VectorSelect { .. } => ValueType::InstantVector,
            Plan::MatrixSelect { .. } => ValueType::RangeVector,
            Plan::UnaryMinus(inner) => inner.value_type(),
            Plan::Binop { lhs, rhs, .. } => binop_result_type(lhs.value_type(), rhs.value_type()),
            Plan::Aggregate { .. } => ValueType::InstantVector,
            Plan::Call { func, .. } => func.return_type(),
            Plan::Subquery { .. } => ValueType::RangeVector,
            Plan::LabelOp { .. } => ValueType::InstantVector,
            Plan::Absent { .. } => ValueType::InstantVector,
            Plan::FusedAggrRollup { .. } => ValueType::InstantVector,
        }
    }

    /// True when the plan contains any operator that needs exact
    /// distribution shape from the storage layer and therefore cannot
    /// run against the bucket-aggregated samples that higher tiers
    /// store.
    ///
    /// Operators that force tier 0:
    /// - `min_over_time`, `max_over_time` — need per-sample extremes
    /// - `quantile_over_time` — needs the full distribution
    /// - `stddev_over_time`, `stdvar_over_time` — need raw samples for
    ///   variance (variance of bucket averages is a different statistic)
    /// - `topk`, `bottomk` — select among series by exact value
    ///
    /// Plans without any of these operators can run on auto-selected
    /// tiers and benefit from the longer retention.
    pub fn requires_tier_zero(&self) -> bool {
        match self {
            Plan::Number(_) | Plan::VectorSelect { .. } | Plan::MatrixSelect { .. } => false,
            Plan::UnaryMinus(inner) => inner.requires_tier_zero(),
            Plan::Binop { lhs, rhs, .. } => lhs.requires_tier_zero() || rhs.requires_tier_zero(),
            Plan::Aggregate { op, expr, .. } => {
                matches!(op, AggrKind::TopK | AggrKind::BottomK) || expr.requires_tier_zero()
            }
            Plan::Call { func, args } => {
                func.requires_tier_zero() || args.iter().any(|a| a.requires_tier_zero())
            }
            Plan::Subquery { expr, .. } => expr.requires_tier_zero(),
            Plan::LabelOp { expr, .. } => expr.requires_tier_zero(),
            Plan::Absent { expr, .. } => expr.requires_tier_zero(),
            Plan::FusedAggrRollup {
                aggr,
                rollup,
                source,
                ..
            } => {
                matches!(aggr, AggrKind::TopK | AggrKind::BottomK)
                    || rollup.requires_tier_zero()
                    || match source {
                        FusedSource::Matrix { .. } => false,
                        FusedSource::Subquery { expr, .. } => expr.requires_tier_zero(),
                    }
            }
        }
    }
}

impl FuncKind {
    /// True when this rollup function needs raw samples (cannot be
    /// computed correctly over higher-tier bucket aggregates).
    /// See `Plan::requires_tier_zero` for the rationale.
    pub fn requires_tier_zero(self) -> bool {
        matches!(
            self,
            FuncKind::MinOverTime
                | FuncKind::MaxOverTime
                | FuncKind::QuantileOverTime
                | FuncKind::StddevOverTime
                | FuncKind::StdvarOverTime
        )
    }
}

/// Binary-op result type rules (Prometheus semantics):
///   scalar op scalar -> scalar
///   scalar op vector -> vector
///   vector op scalar -> vector
///   vector op vector -> vector (one-to-one matching; Phase 2 adds many-to-one)
pub(crate) fn binop_result_type(l: ValueType, r: ValueType) -> ValueType {
    match (l, r) {
        (ValueType::Scalar, ValueType::Scalar) => ValueType::Scalar,
        _ => ValueType::InstantVector,
    }
}
