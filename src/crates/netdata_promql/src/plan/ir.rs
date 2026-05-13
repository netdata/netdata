// SPDX-License-Identifier: GPL-3.0-or-later
//
// Plan IR: a typed, executable representation of a PromQL query lowered
// from the `promql_parser` AST. The IR mirrors the AST nodes the evaluator
// supports, with type errors caught at lowering time rather than at
// execution.
//
// Phase 1 chunk 3 (SOW-0017) covers ~15 AST node kinds. Subqueries, vector
// matching with group_left/group_right, and function calls beyond what's
// needed for rate()/histogram_quantile() are deferred.

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
/// Sum/Avg/Min/Max/Count: Phase 1 (SOW-0017), no parameter.
/// TopK/BottomK/Quantile: Phase 3c (SOW-0021), each takes a single
/// scalar parameter (k or phi). `count_values` is parametrized but
/// takes a string; deferred.
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
}

impl AggrKind {
    /// True when the operator requires a scalar parameter
    /// (e.g. `topk(5, expr)`). Non-parametrized aggregators
    /// reject any param at lowering time.
    pub fn takes_param(self) -> bool {
        matches!(self, AggrKind::TopK | AggrKind::BottomK | AggrKind::Quantile)
    }
}

/// Binary operators supported in Phase 1.
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
}

impl BinopKind {
    pub fn is_comparison(self) -> bool {
        matches!(
            self,
            BinopKind::Eq | BinopKind::Ne | BinopKind::Lt | BinopKind::Le | BinopKind::Gt | BinopKind::Ge
        )
    }
}

/// Grouping clause for an aggregation. `By` keeps the listed labels;
/// `Without` drops them.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Grouping {
    By(Vec<String>),
    Without(Vec<String>),
}

/// Function calls supported in Phase 1. Empty in chunk 3; populated in
/// chunk 4 (rate, irate, increase, delta, histogram_quantile).
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
    PresentOverTime,
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
            "present_over_time" => FuncKind::PresentOverTime,
            _ => return None,
        })
    }

    pub fn return_type(self) -> ValueType {
        ValueType::InstantVector
    }
}

/// The Plan IR.
///
/// Cheap to clone (`Arc` for the heavy heap-allocated children), so the
/// parse cache can hand out copies safely.
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
    },

    /// A range-vector selector. The window is `[t - range_ms, t]` shifted
    /// by `offset_ms`.
    MatrixSelect {
        matchers: Vec<Matcher>,
        range_ms: i64,
        offset_ms: i64,
    },

    /// Unary negation. Operand is Scalar or InstantVector.
    UnaryMinus(Arc<Plan>),

    /// Binary operation. The Plan IR distinguishes type combinations at
    /// lowering time: scalar/scalar, scalar/vector, vector/scalar,
    /// vector/vector. The single variant carries the kind plus the bool
    /// modifier for comparisons.
    Binop {
        op: BinopKind,
        /// True for `comparison bool` operators which return 0/1 rather
        /// than filtering.
        return_bool: bool,
        lhs: Arc<Plan>,
        rhs: Arc<Plan>,
    },

    /// Aggregation over an instant vector.
    Aggregate {
        op: AggrKind,
        grouping: Option<Grouping>,
        /// Parameter for topk/bottomk/quantile (none of which are in Phase
        /// 1). Carried for forward compatibility; lowering rejects values
        /// here today.
        param: Option<Arc<Plan>>,
        expr: Arc<Plan>,
    },

    /// Function call: rate(), histogram_quantile(), etc.
    Call {
        func: FuncKind,
        args: Vec<Plan>,
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
        }
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
