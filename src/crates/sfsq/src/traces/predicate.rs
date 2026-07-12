//! The neutral, scope-aware search predicate AST (phase-4c decision 26A)
//! — the FULL recorded form grammar as typed data over the
//! [`vocab`](super::vocab) enums, never grammar strings (decision 16A:
//! each wire adapter owns its rendering; the phase-5 Tempo shim is a pure
//! translator onto this type).
//!
//! The whole grammar PARSES into this AST and validates structurally in
//! every build; stage A EVALUATES only the positive subset (conjunctions
//! of `=`/`=~` terms on resource/span/instrumentation attributes and the
//! dictionary-backed intrinsics except `event:name`, plus duration
//! bounds). A structurally valid construct outside that subset is
//! rejected at the request boundary with the named
//! [`PredicateError::NotYetEvaluable`] error — a clean gap, never a
//! silently wrong answer. Stage B adds evaluation arms only; this type
//! does not change shape between stages.
//!
//! # One lowering, two evaluators
//!
//! [`Predicate::to_trace_plan`] lowers the span-local conditions into the
//! neutral [`sfst::TracePlan`] (storage names constructed ONLY through
//! the vocabulary), and the span-side evaluator ([`span_matches`] /
//! [`EvalPredicate`]) is BUILT FROM THAT SAME PLAN — the raw index path
//! and the canonical span path cannot disagree on what a condition means,
//! because there is exactly one lowering (the phase-1/phase-2 consistency
//! risk pinned in the SOW).

use sfst::{PlanTerm, TracePlan, TraceSpan};

use super::vocab::{TagScope, TraceIntrinsic};
use super::window::TimeWindow;

/// What a condition tests. Attribute scopes come from the tag vocabulary
/// ([`TagScope::Intrinsic`] is not an attribute scope — request error);
/// an UNSCOPED attribute is the form grammar's `.tag` (resource ∪ span
/// disjunction, stage B); intrinsics are the full fixed set, colon forms
/// included (`span:id` = [`TraceIntrinsic::SpanId`], …).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PredicateTarget {
    Attribute(TagScope, String),
    UnscopedAttribute(String),
    Intrinsic(TraceIntrinsic),
}

impl std::fmt::Display for PredicateTarget {
    /// Diagnostic rendering (error messages only — no wire grammar).
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PredicateTarget::Attribute(scope, key) => write!(f, "{scope:?} attribute {key:?}"),
            PredicateTarget::UnscopedAttribute(key) => write!(f, "unscoped attribute {key:?}"),
            PredicateTarget::Intrinsic(i) => write!(f, "intrinsic {i:?}"),
        }
    }
}

/// The form grammar's comparison operators.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CompareOp {
    Eq,
    NotEq,
    Gt,
    Lt,
    Gte,
    Lte,
    /// Full-value-anchored regex (the `=~` op, Tempo semantics).
    Regex,
    /// Negated full-value-anchored regex (`!~`).
    NotRegex,
}

impl CompareOp {
    fn is_ordering(self) -> bool {
        matches!(self, CompareOp::Gt | CompareOp::Lt | CompareOp::Gte | CompareOp::Lte)
    }

    fn is_regex(self) -> bool {
        matches!(self, CompareOp::Regex | CompareOp::NotRegex)
    }
}

/// One comparison value. Durations arrive as [`Integer`](Self::Integer)
/// nanoseconds (the wire adapter converts its duration literals);
/// [`Float`](Self::Float)/`Integer` against attributes are the
/// dictionary-numeric comparisons (stage B).
#[derive(Debug, Clone, PartialEq)]
pub enum PredicateValue {
    Text(String),
    Integer(i64),
    Float(f64),
}

/// One condition: `target op values`. Multi-value semantics follow the
/// recorded grammar: for `=`/`=~` the values OR (any may match); for
/// `!=`/`!~` they AND ("field present and NO value equals/matches");
/// ordering ops take exactly one value.
#[derive(Debug, Clone, PartialEq)]
pub struct Condition {
    pub target: PredicateTarget,
    pub op: CompareOp,
    pub values: Vec<PredicateValue>,
}

/// The predicate: a conjunction of conditions within one spanset (pinned
/// spanset semantics, C-3: a trace matches when at least one retained
/// canonical span — with its resource/scope context — satisfies ALL
/// conditions and the window). Empty = match everything (the `{}` query).
#[derive(Debug, Clone, Default, PartialEq)]
pub struct Predicate {
    pub conditions: Vec<Condition>,
}

impl Predicate {
    /// The match-everything predicate (the empty form, `{}`).
    pub fn all() -> Self {
        Self::default()
    }

    pub fn is_all(&self) -> bool {
        self.conditions.is_empty()
    }
}

/// A predicate request error — the caller built an invalid predicate (or
/// one this build cannot evaluate yet); nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum PredicateError {
    #[error("a condition on {target} carries no values")]
    NoValues { target: String },
    #[error("{op:?} takes exactly one value, got {got}")]
    OrderingArity { op: CompareOp, got: usize },
    #[error("attribute {key:?} cannot live under the Intrinsic scope")]
    AttributeInIntrinsicScope { key: String },
    #[error("{op:?} is not a valid comparison on {target}")]
    InvalidOpForTarget { op: CompareOp, target: String },
    #[error("{op:?} on {target} requires text values")]
    TextValueRequired { op: CompareOp, target: String },
    #[error("{op:?} on {target} requires numeric values")]
    NumericValueRequired { op: CompareOp, target: String },
    #[error("duration comparisons take integer nanoseconds")]
    NonIntegerDuration,
    #[error("invalid regex pattern {pattern:?}: {msg}")]
    InvalidPattern { pattern: String, msg: String },
    #[error("a condition on {target} mixes text and numeric values")]
    MixedValueTypes { target: String },
    #[error("a condition on {target} compares against NaN, which matches nothing")]
    NanValue { target: String },
    /// The construct is valid grammar but outside this build's evaluable
    /// subset (the stage-A/stage-B boundary, decision 26A) — named so the
    /// caller sees exactly which construct to avoid, never a silently
    /// wrong result.
    #[error("not yet evaluable: {construct}")]
    NotYetEvaluable { construct: String },
}

/// The intrinsic's comparison class — which operators and value types
/// are structurally valid on it (full grammar, both stages).
enum IntrinsicClass {
    /// String-valued (free-form names/messages AND the closed kind/
    /// status label sets — regex over labels is valid grammar, pin 26A):
    /// `= != =~ !~` with text values.
    Text,
    /// Nanosecond quantity: `= != > < >= <=` with integer values.
    Nanos,
    /// Fixed-width id in hex text: `= !=` with text values.
    Id,
}

fn intrinsic_class(i: TraceIntrinsic) -> IntrinsicClass {
    use TraceIntrinsic::*;
    match i {
        Name | Kind | Status | StatusMessage | InstrumentationName | InstrumentationVersion
        | EventName | RootName | RootServiceName => IntrinsicClass::Text,
        Duration | TraceDuration | EventTimeSinceStart => IntrinsicClass::Nanos,
        SpanId | ParentSpanId | TraceId | LinkSpanId | LinkTraceId => IntrinsicClass::Id,
    }
}

impl Condition {
    /// Structural validation against the FULL grammar (both stages):
    /// arity, scope/target shape, op/target compatibility, value types,
    /// and regex compilability — everything checkable without data.
    fn validate(&self) -> Result<(), PredicateError> {
        let target = self.target.to_string();
        if self.values.is_empty() {
            return Err(PredicateError::NoValues { target });
        }
        if self.op.is_ordering() && self.values.len() != 1 {
            return Err(PredicateError::OrderingArity {
                op: self.op,
                got: self.values.len(),
            });
        }
        let all_text = self.values.iter().all(|v| matches!(v, PredicateValue::Text(_)));
        let all_integer = self
            .values
            .iter()
            .all(|v| matches!(v, PredicateValue::Integer(_)));
        let all_numeric = self.values.iter().all(|v| {
            matches!(v, PredicateValue::Integer(_) | PredicateValue::Float(_))
        });
        // One condition compares against ONE value class: mixing text
        // and numbers has no coherent multi-value semantics (and the
        // form grammar never generates it); a NaN never compares.
        if !all_text && !all_numeric {
            return Err(PredicateError::MixedValueTypes { target });
        }
        if self
            .values
            .iter()
            .any(|v| matches!(v, PredicateValue::Float(f) if f.is_nan()))
        {
            return Err(PredicateError::NanValue { target });
        }

        match &self.target {
            PredicateTarget::Attribute(TagScope::Intrinsic, key) => {
                return Err(PredicateError::AttributeInIntrinsicScope { key: key.clone() });
            }
            // Attributes take every op; regex needs text patterns,
            // ordering needs numbers (dictionary-numeric, stage B).
            PredicateTarget::Attribute(..) | PredicateTarget::UnscopedAttribute(_) => {
                if self.op.is_regex() && !all_text {
                    return Err(PredicateError::TextValueRequired { op: self.op, target });
                }
                if self.op.is_ordering() && !all_numeric {
                    return Err(PredicateError::NumericValueRequired { op: self.op, target });
                }
            }
            PredicateTarget::Intrinsic(i) => match intrinsic_class(*i) {
                IntrinsicClass::Text => {
                    if self.op.is_ordering() {
                        return Err(PredicateError::InvalidOpForTarget { op: self.op, target });
                    }
                    if !all_text {
                        return Err(PredicateError::TextValueRequired { op: self.op, target });
                    }
                }
                IntrinsicClass::Id => {
                    if !matches!(self.op, CompareOp::Eq | CompareOp::NotEq) {
                        return Err(PredicateError::InvalidOpForTarget { op: self.op, target });
                    }
                    if !all_text {
                        return Err(PredicateError::TextValueRequired { op: self.op, target });
                    }
                }
                IntrinsicClass::Nanos => {
                    if self.op.is_regex() {
                        return Err(PredicateError::InvalidOpForTarget { op: self.op, target });
                    }
                    if !all_integer {
                        return Err(PredicateError::NonIntegerDuration);
                    }
                }
            },
        }

        if self.op.is_regex() {
            for value in &self.values {
                let PredicateValue::Text(pattern) = value else {
                    unreachable!("all_text checked above");
                };
                sfst::compile_pattern(pattern).map_err(|e| PredicateError::InvalidPattern {
                    pattern: pattern.clone(),
                    msg: e.to_string(),
                })?;
            }
        }
        Ok(())
    }

    /// Whether this build evaluates this condition — see the module
    /// docs. `None` = evaluable; `Some(construct)` names the
    /// not-yet-evaluable construct. After stage-B step 6 the evaluable
    /// set covers negation, the unscoped disjunction, and
    /// dictionary-numeric comparisons; the remaining gaps are the
    /// colon-set ids, event/link structural refine, and the
    /// trace-level intrinsics (steps 7-9).
    fn unevaluable_construct(&self) -> Option<String> {
        use TraceIntrinsic::*;
        match (&self.target, self.op) {
            // Duration: bounds, equality (any arity — an interval set),
            // and negated equality all evaluate on the DURN column.
            (PredicateTarget::Intrinsic(Duration), op)
                if op.is_ordering() || matches!(op, CompareOp::Eq | CompareOp::NotEq) =>
            {
                None
            }
            // Attribute scopes (incl. the unscoped disjunction): every
            // op — tokens for text, the dictionary-numeric path for
            // numbers, presence ∩ complement for negation.
            (
                PredicateTarget::Attribute(
                    TagScope::Resource | TagScope::Span | TagScope::Instrumentation,
                    _,
                )
                | PredicateTarget::UnscopedAttribute(_),
                _,
            ) => None,
            // Dictionary-backed intrinsics except event:name (refine).
            (
                PredicateTarget::Intrinsic(
                    Name | Kind | Status | StatusMessage | InstrumentationName
                    | InstrumentationVersion,
                ),
                CompareOp::Eq | CompareOp::Regex | CompareOp::NotEq | CompareOp::NotRegex,
            ) => None,
            (PredicateTarget::Attribute(TagScope::Event | TagScope::Link, _), _) => {
                Some(format!("{} (structural refine)", self.target))
            }
            (PredicateTarget::Intrinsic(RootName | RootServiceName | TraceDuration), _) => {
                Some(format!("trace-level {}", self.target))
            }
            (target, op) => Some(format!("{op:?} on {target}")),
        }
    }
}

impl Predicate {
    /// Structural validation against the FULL grammar. Run at the request
    /// boundary before any source opens.
    pub(crate) fn validate(&self) -> Result<(), PredicateError> {
        for condition in &self.conditions {
            condition.validate()?;
        }
        Ok(())
    }

    /// The stage boundary (decision 26A): error on the first condition
    /// outside this build's evaluable subset. Stage B relaxes this as it
    /// adds evaluation arms; the AST itself never changes shape.
    pub(crate) fn ensure_evaluable(&self) -> Result<(), PredicateError> {
        for condition in &self.conditions {
            if let Some(construct) = condition.unevaluable_construct() {
                return Err(PredicateError::NotYetEvaluable { construct });
            }
        }
        Ok(())
    }

    /// Split into `(span_local, trace_level)` (pin R3-2): trace-level
    /// intrinsics (`rootName`, `rootServiceName`, `traceDuration`)
    /// evaluate post-assembly as tri-state (decision 15); EVERYTHING the
    /// span-side evaluator and the file plans see — and everything
    /// `matched_count`/`spss` are defined over — is the span-local part.
    pub(crate) fn partition(&self) -> (Predicate, Predicate) {
        use TraceIntrinsic::*;
        let (trace_level, span_local): (Vec<Condition>, Vec<Condition>) =
            self.conditions.iter().cloned().partition(|c| {
                matches!(
                    c.target,
                    PredicateTarget::Intrinsic(RootName | RootServiceName | TraceDuration)
                )
            });
        (
            Predicate {
                conditions: span_local,
            },
            Predicate {
                conditions: trace_level,
            },
        )
    }

    /// Lower the (span-local, validated, evaluable) conditions to the
    /// neutral per-file plan — THE single lowering both evaluation paths
    /// share (module docs). Storage names come only from the vocabulary.
    ///
    /// Duration lowering: conjunctive bounds (orderings and single-value
    /// equality) INTERSECT into one positive interval term (one DURN
    /// pass however many bounds); a multi-value equality becomes one
    /// term with point intervals; a `!=` becomes one NEGATED term with
    /// point intervals ("in no interval" = no value equals).
    pub(crate) fn to_trace_plan(&self) -> TracePlan {
        let mut terms = Vec::with_capacity(self.conditions.len());
        let mut bound: Option<(Option<i64>, Option<i64>)> = None;
        for condition in &self.conditions {
            let integers = || -> Vec<i64> {
                condition
                    .values
                    .iter()
                    .map(|v| match v {
                        PredicateValue::Integer(n) => *n,
                        _ => unreachable!("validated: duration values are integers"),
                    })
                    .collect()
            };
            match (&condition.target, condition.op) {
                (PredicateTarget::Intrinsic(TraceIntrinsic::Duration), CompareOp::NotEq) => {
                    terms.push(PlanTerm::Duration {
                        intervals: integers().into_iter().map(|v| (Some(v), Some(v))).collect(),
                        negated: true,
                    });
                }
                (PredicateTarget::Intrinsic(TraceIntrinsic::Duration), CompareOp::Eq)
                    if condition.values.len() > 1 =>
                {
                    terms.push(PlanTerm::Duration {
                        intervals: integers().into_iter().map(|v| (Some(v), Some(v))).collect(),
                        negated: false,
                    });
                }
                (PredicateTarget::Intrinsic(TraceIntrinsic::Duration), op) => {
                    let (min_ns, max_ns) = duration_bounds(op, integers()[0]);
                    bound = Some(match bound {
                        None => (min_ns, max_ns),
                        Some((lo, hi)) => (
                            std::cmp::max(lo, min_ns), // None < Some: unbounded loses
                            match (hi, max_ns) {
                                (Some(a), Some(b)) => Some(a.min(b)),
                                (a, b) => a.or(b),
                            },
                        ),
                    });
                }
                (target, op) => {
                    let fields: Vec<String> = match target {
                        PredicateTarget::Attribute(scope, key) => {
                            let prefix = scope
                                .attribute_prefix()
                                .expect("validated: not the Intrinsic scope");
                            vec![format!("{prefix}{key}")]
                        }
                        // The unscoped attribute is the pinned
                        // resource ∪ span disjunction.
                        PredicateTarget::UnscopedAttribute(key) => vec![
                            format!(
                                "{}{key}",
                                TagScope::Resource
                                    .attribute_prefix()
                                    .expect("attribute scope")
                            ),
                            format!(
                                "{}{key}",
                                TagScope::Span.attribute_prefix().expect("attribute scope")
                            ),
                        ],
                        PredicateTarget::Intrinsic(i) => vec![
                            i.dictionary_field()
                                .expect("evaluable intrinsics are dictionary-backed")
                                .to_string(),
                        ],
                    };
                    let negated = matches!(op, CompareOp::NotEq | CompareOp::NotRegex);
                    let numeric = condition
                        .values
                        .iter()
                        .any(|v| matches!(v, PredicateValue::Integer(_) | PredicateValue::Float(_)));
                    let matcher = if numeric {
                        // Dictionary-numeric: equality against any of the
                        // values, or the single ordering comparison.
                        let values: Vec<f64> = condition
                            .values
                            .iter()
                            .map(|v| match v {
                                PredicateValue::Integer(n) => *n as f64,
                                PredicateValue::Float(f) => *f,
                                PredicateValue::Text(_) => {
                                    unreachable!("validated: values are homogeneous")
                                }
                            })
                            .collect();
                        let cmp = match op {
                            CompareOp::Eq | CompareOp::NotEq => sfst::NumberCmp::Eq,
                            CompareOp::Gt => sfst::NumberCmp::Gt,
                            CompareOp::Lt => sfst::NumberCmp::Lt,
                            CompareOp::Gte => sfst::NumberCmp::Gte,
                            CompareOp::Lte => sfst::NumberCmp::Lte,
                            CompareOp::Regex | CompareOp::NotRegex => {
                                unreachable!("validated: regex takes text patterns")
                            }
                        };
                        sfst::PlanMatcher::Number { cmp, values }
                    } else {
                        let mut exact = Vec::new();
                        let mut patterns = Vec::new();
                        for value in &condition.values {
                            let PredicateValue::Text(text) = value else {
                                unreachable!("validated: values are homogeneous");
                            };
                            match op {
                                CompareOp::Eq | CompareOp::NotEq => exact.push(text.clone()),
                                CompareOp::Regex | CompareOp::NotRegex => {
                                    patterns.push(text.clone())
                                }
                                _ => unreachable!("validated: ordering needs numeric values"),
                            }
                        }
                        sfst::PlanMatcher::Tokens { exact, patterns }
                    };
                    terms.push(PlanTerm::Fields {
                        fields,
                        matcher,
                        negated,
                    });
                }
            }
        }
        if let Some((min_ns, max_ns)) = bound {
            terms.push(PlanTerm::duration(min_ns, max_ns));
        }
        TracePlan { terms }
    }
}

/// Inclusive `[min_ns, max_ns]` bounds for a duration comparison —
/// shared by the plan lowering and the span-side evaluator so the two
/// paths convert exclusivity identically (integer nanoseconds, so
/// `> v ⇔ ≥ v+1`). A comparison no i64 satisfies (`> i64::MAX`,
/// `< i64::MIN`) becomes the explicit EMPTY interval — saturating there
/// would wrongly keep the extreme value itself matching.
fn duration_bounds(op: CompareOp, v: i64) -> (Option<i64>, Option<i64>) {
    const EMPTY: (Option<i64>, Option<i64>) = (Some(i64::MAX), Some(i64::MIN));
    match op {
        CompareOp::Eq => (Some(v), Some(v)),
        CompareOp::Gt => match v.checked_add(1) {
            Some(min) => (Some(min), None),
            None => EMPTY,
        },
        CompareOp::Gte => (Some(v), None),
        CompareOp::Lt => match v.checked_sub(1) {
            Some(max) => (None, Some(max)),
            None => EMPTY,
        },
        CompareOp::Lte => (None, Some(v)),
        CompareOp::NotEq | CompareOp::Regex | CompareOp::NotRegex => {
            unreachable!("not a duration bound op (validated)")
        }
    }
}

/// The compiled token/number matcher of one span-side term.
enum EvalMatcher {
    Tokens {
        exact: Vec<String>,
        patterns: Vec<regex::bytes::Regex>,
    },
    Number {
        cmp: sfst::NumberCmp,
        values: Vec<f64>,
    },
}

impl EvalMatcher {
    /// Whether one rendered field value satisfies the matcher — token
    /// equality / anchored regex, or the SAME numeric comparator the
    /// dictionary walks use ([`sfst::numeric_token_matches`]), so the
    /// two paths cannot diverge on parsing or comparison.
    fn value_matches(&self, value: &str) -> bool {
        match self {
            EvalMatcher::Tokens { exact, patterns } => {
                exact.iter().any(|e| e == value)
                    || patterns.iter().any(|p| p.is_match(value.as_bytes()))
            }
            EvalMatcher::Number { cmp, values } => values
                .iter()
                .any(|&rhs| sfst::numeric_token_matches(value, *cmp, rhs)),
        }
    }
}

/// One term of the span-side evaluator — the compiled twin of
/// [`sfst::PlanTerm`], built FROM the plan (one lowering; see the module
/// docs). Patterns are pre-compiled once per predicate, not per span.
enum EvalTerm {
    Fields {
        fields: Vec<String>,
        matcher: EvalMatcher,
        negated: bool,
    },
    Duration {
        intervals: Vec<(Option<i64>, Option<i64>)>,
        negated: bool,
    },
}

/// The span-side predicate evaluator — the single source of truth for
/// what a span-local predicate MEANS (pin R2-9): the tail's phase 1
/// evaluates it per decoded span, phase 2 re-evaluates it per retained
/// canonical span, and `matched_count`/`spss` are defined by it.
pub(crate) struct EvalPredicate {
    terms: Vec<EvalTerm>,
}

impl EvalPredicate {
    /// Compile from the lowered plan. The predicate was validated at the
    /// request boundary, so pattern compilation cannot fail here.
    pub(crate) fn new(plan: &TracePlan) -> Self {
        let terms = plan
            .terms
            .iter()
            .map(|term| match term {
                PlanTerm::Fields {
                    fields,
                    matcher,
                    negated,
                } => EvalTerm::Fields {
                    fields: fields.clone(),
                    matcher: match matcher {
                        sfst::PlanMatcher::Tokens { exact, patterns } => EvalMatcher::Tokens {
                            exact: exact.clone(),
                            patterns: patterns
                                .iter()
                                .map(|p| {
                                    sfst::compile_pattern(p)
                                        .expect("patterns validated at the request boundary")
                                })
                                .collect(),
                        },
                        sfst::PlanMatcher::Number { cmp, values } => EvalMatcher::Number {
                            cmp: *cmp,
                            values: values.clone(),
                        },
                    },
                    negated: *negated,
                },
                PlanTerm::Duration { intervals, negated } => EvalTerm::Duration {
                    intervals: intervals.clone(),
                    negated: *negated,
                },
            })
            .collect();
        Self { terms }
    }

    /// Whether `span` satisfies every term and (when given) starts inside
    /// `window` (decision 5: span-START-in-window).
    pub(crate) fn matches(&self, span: &TraceSpan, window: Option<TimeWindow>) -> bool {
        if let Some(w) = window
            && !w.contains(span.start_ns)
        {
            return false;
        }
        self.terms.iter().all(|term| match term {
            EvalTerm::Fields {
                fields,
                matcher,
                negated,
            } => {
                // Values gathered across the term's fields (several for
                // the unscoped disjunction; multi-valued fields yield
                // several entries). Negation is the pinned
                // presence ∩ complement: present with NO matching value.
                let mut present = false;
                let mut any_match = false;
                for (k, v) in &span.fields {
                    if fields.iter().any(|f| f == k) {
                        present = true;
                        if matcher.value_matches(v) {
                            any_match = true;
                            break;
                        }
                    }
                }
                if *negated { present && !any_match } else { any_match }
            }
            EvalTerm::Duration { intervals, negated } => {
                let in_any = intervals.iter().any(|&(lo, hi)| {
                    lo.is_none_or(|lo| span.duration_ns >= lo)
                        && hi.is_none_or(|hi| span.duration_ns <= hi)
                });
                in_any != *negated
            }
        })
    }
}

/// Whether one span (with its resource/scope context — a span's fields
/// carry the flattened resource and scope entries) satisfies the
/// SPAN-LOCAL `predicate` and the optional window. The pinned span-side
/// seam (R2-9); engine loops use the pre-compiled [`EvalPredicate`] this
/// delegates to, so the two can never diverge.
///
/// `predicate` must be validated and span-local (partitioned); a
/// trace-level or stage-B condition here is an engine bug and panics
/// loudly rather than under-approximating silently.
pub fn span_matches(
    span: &TraceSpan,
    predicate: &Predicate,
    window: Option<TimeWindow>,
) -> bool {
    EvalPredicate::new(&predicate.to_trace_plan()).matches(span, window)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn text(v: &str) -> PredicateValue {
        PredicateValue::Text(v.to_string())
    }

    fn cond(target: PredicateTarget, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
        Condition { target, op, values }
    }

    fn span_attr(key: &str) -> PredicateTarget {
        PredicateTarget::Attribute(TagScope::Span, key.to_string())
    }

    /// Structural validation: the full-grammar rules, independent of the
    /// stage boundary.
    #[test]
    fn structural_validation() {
        let ok = |c: Condition| Predicate { conditions: vec![c] }.validate().unwrap();
        let err = |c: Condition| Predicate { conditions: vec![c] }.validate().unwrap_err();

        ok(cond(span_attr("x"), CompareOp::Eq, vec![text("v")]));
        ok(cond(span_attr("x"), CompareOp::NotEq, vec![text("a"), text("b")]));
        ok(cond(
            span_attr("x"),
            CompareOp::Gt,
            vec![PredicateValue::Float(1.5)],
        ));
        ok(cond(
            PredicateTarget::UnscopedAttribute("x".into()),
            CompareOp::Regex,
            vec![text("a|b")],
        ));
        ok(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::TraceDuration),
            CompareOp::Gte,
            vec![PredicateValue::Integer(5)],
        ));
        ok(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::SpanId),
            CompareOp::Eq,
            vec![text("00f067aa0ba902b7")],
        ));

        assert!(matches!(
            err(cond(span_attr("x"), CompareOp::Eq, vec![])),
            PredicateError::NoValues { .. }
        ));
        assert!(matches!(
            err(cond(
                span_attr("x"),
                CompareOp::Gt,
                vec![PredicateValue::Integer(1), PredicateValue::Integer(2)],
            )),
            PredicateError::OrderingArity { got: 2, .. }
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Attribute(TagScope::Intrinsic, "x".into()),
                CompareOp::Eq,
                vec![text("v")],
            )),
            PredicateError::AttributeInIntrinsicScope { .. }
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Name),
                CompareOp::Gt,
                vec![text("v")],
            )),
            PredicateError::InvalidOpForTarget { .. }
        ));
        // Regex over the kind/status LABEL sets is valid grammar (26A).
        ok(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::Kind),
            CompareOp::Regex,
            vec![text("SER.*")],
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Kind),
                CompareOp::Gt,
                vec![text("SERVER")],
            )),
            PredicateError::InvalidOpForTarget { .. }
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                CompareOp::Gt,
                vec![text("100ms")],
            )),
            PredicateError::NonIntegerDuration
        ));
        assert!(matches!(
            err(cond(
                span_attr("x"),
                CompareOp::Gte,
                vec![text("nan")],
            )),
            PredicateError::NumericValueRequired { .. }
        ));
        assert!(matches!(
            err(cond(span_attr("x"), CompareOp::Regex, vec![text("(")])),
            PredicateError::InvalidPattern { .. }
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Status),
                CompareOp::Eq,
                vec![PredicateValue::Integer(2)],
            )),
            PredicateError::TextValueRequired { .. }
        ));
    }

    /// The stage boundary: the positive subset passes; every recorded
    /// B-construct is named, not silently mis-evaluated.
    #[test]
    fn evaluability_boundary() {
        let evaluable = |c: Condition| {
            Predicate { conditions: vec![c] }.ensure_evaluable().unwrap();
        };
        let rejected = |c: Condition| -> String {
            match (Predicate { conditions: vec![c] }).ensure_evaluable() {
                Err(PredicateError::NotYetEvaluable { construct }) => construct,
                other => panic!("expected NotYetEvaluable, got {other:?}"),
            }
        };

        // Attribute scopes and the unscoped disjunction: every op.
        evaluable(cond(span_attr("x"), CompareOp::Eq, vec![text("a"), text("b")]));
        evaluable(cond(span_attr("x"), CompareOp::NotEq, vec![text("v")]));
        evaluable(cond(span_attr("x"), CompareOp::NotRegex, vec![text("v.*")]));
        evaluable(cond(
            PredicateTarget::Attribute(TagScope::Resource, "service.name".into()),
            CompareOp::Regex,
            vec![text("svc-.*")],
        ));
        evaluable(cond(
            PredicateTarget::Attribute(TagScope::Instrumentation, "lib".into()),
            CompareOp::Eq,
            vec![text("v")],
        ));
        evaluable(cond(
            PredicateTarget::UnscopedAttribute("x".into()),
            CompareOp::Eq,
            vec![text("v")],
        ));
        evaluable(cond(
            PredicateTarget::UnscopedAttribute("x".into()),
            CompareOp::NotRegex,
            vec![text("v.*")],
        ));
        // Dictionary numerics on attributes.
        evaluable(cond(span_attr("x"), CompareOp::Gt, vec![PredicateValue::Integer(5)]));
        evaluable(cond(
            span_attr("x"),
            CompareOp::Eq,
            vec![PredicateValue::Integer(5), PredicateValue::Float(1.5)],
        ));
        // Dictionary-backed intrinsics except event:name: all four ops.
        for i in [
            TraceIntrinsic::Name,
            TraceIntrinsic::Kind,
            TraceIntrinsic::Status,
            TraceIntrinsic::StatusMessage,
            TraceIntrinsic::InstrumentationName,
            TraceIntrinsic::InstrumentationVersion,
        ] {
            for op in [CompareOp::Eq, CompareOp::Regex, CompareOp::NotEq, CompareOp::NotRegex] {
                evaluable(cond(
                    PredicateTarget::Intrinsic(i),
                    op,
                    vec![text("v")],
                ));
            }
        }
        // Duration: bounds, multi-value equality, negated equality.
        for op in [CompareOp::Gt, CompareOp::Lt, CompareOp::Gte, CompareOp::Lte, CompareOp::Eq] {
            evaluable(cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                op,
                vec![PredicateValue::Integer(100)],
            ));
        }
        evaluable(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
            CompareOp::Eq,
            vec![PredicateValue::Integer(1), PredicateValue::Integer(2)],
        ));
        evaluable(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(3)],
        ));

        // Named rejections for the constructs steps 7-9 still own.
        assert!(rejected(cond(
            PredicateTarget::Attribute(TagScope::Event, "msg".into()),
            CompareOp::Eq,
            vec![text("v")],
        ))
        .contains("refine"));
        assert!(rejected(cond(
            PredicateTarget::Attribute(TagScope::Link, "rel".into()),
            CompareOp::NotEq,
            vec![text("v")],
        ))
        .contains("refine"));
        assert!(rejected(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::EventName),
            CompareOp::Eq,
            vec![text("v")],
        ))
        .contains("EventName"));
        assert!(rejected(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::RootServiceName),
            CompareOp::Eq,
            vec![text("v")],
        ))
        .contains("trace-level"));
        assert!(rejected(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::TraceDuration),
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        ))
        .contains("trace-level"));
        assert!(rejected(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::EventTimeSinceStart),
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        ))
        .contains("EventTimeSinceStart"));
        assert!(rejected(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::SpanId),
            CompareOp::Eq,
            vec![text("00f067aa0ba902b7")],
        ))
        .contains("SpanId"));
    }

    /// Partition (R3-2): trace-level intrinsics split out; everything
    /// else stays span-local, order preserved within each part.
    #[test]
    fn partition_splits_trace_level_conditions() {
        let p = Predicate {
            conditions: vec![
                cond(span_attr("x"), CompareOp::Eq, vec![text("v")]),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::TraceDuration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(5)],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(5)],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::RootName),
                    CompareOp::Eq,
                    vec![text("r")],
                ),
            ],
        };
        let (span_local, trace_level) = p.partition();
        assert_eq!(span_local.conditions.len(), 2);
        assert_eq!(trace_level.conditions.len(), 2);
        assert!(span_local.conditions.iter().all(|c| !matches!(
            c.target,
            PredicateTarget::Intrinsic(
                TraceIntrinsic::RootName
                    | TraceIntrinsic::RootServiceName
                    | TraceIntrinsic::TraceDuration
            )
        )));
        // All-trace-level → span-local is the empty form (C-4: phase-1
        // candidates come from `All` over the window).
        let only_trace = Predicate {
            conditions: vec![cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::TraceDuration),
                CompareOp::Gt,
                vec![PredicateValue::Integer(5)],
            )],
        };
        let (span_local, _) = only_trace.partition();
        assert!(span_local.is_all());
    }

    /// The lowering constructs storage names only through the vocabulary
    /// and converts duration exclusivity to inclusive integer bounds.
    #[test]
    fn lowering_to_the_file_plan() {
        let p = Predicate {
            conditions: vec![
                cond(
                    PredicateTarget::Attribute(TagScope::Resource, "service.name".into()),
                    CompareOp::Eq,
                    vec![text("svc")],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Status),
                    CompareOp::Eq,
                    vec![text("ERROR")],
                ),
                cond(span_attr("http.method"), CompareOp::Regex, vec![text("GET|PUT")]),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(100)],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Lte,
                    vec![PredicateValue::Integer(500)],
                ),
            ],
        };
        let plan = p.to_trace_plan();
        assert_eq!(
            plan.terms,
            vec![
                PlanTerm::tokens(
                    "resource.attributes.service.name",
                    vec!["svc".into()],
                    vec![],
                ),
                PlanTerm::tokens("status_code", vec!["ERROR".into()], vec![]),
                PlanTerm::tokens("attributes.http.method", vec![], vec!["GET|PUT".into()]),
                // Both duration bounds intersect into ONE interval term.
                PlanTerm::duration(Some(101), Some(500)),
            ]
        );
        assert!(Predicate::all().to_trace_plan().terms.is_empty());
        // Contradictory bounds intersect to an empty interval.
        let contradictory = Predicate {
            conditions: vec![
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(500)],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Lt,
                    vec![PredicateValue::Integer(100)],
                ),
            ],
        };
        assert_eq!(
            contradictory.to_trace_plan().terms,
            vec![PlanTerm::duration(Some(501), Some(99))]
        );
        // Stage-B lowerings: negation, unscoped disjunction, numerics,
        // multi-value and negated duration equality.
        let stage_b = Predicate {
            conditions: vec![
                cond(span_attr("x"), CompareOp::NotEq, vec![text("a"), text("b")]),
                cond(
                    PredicateTarget::UnscopedAttribute("env".into()),
                    CompareOp::Eq,
                    vec![text("prod")],
                ),
                cond(
                    span_attr("code"),
                    CompareOp::Gte,
                    vec![PredicateValue::Integer(500)],
                ),
                cond(
                    span_attr("ratio"),
                    CompareOp::Eq,
                    vec![PredicateValue::Float(0.5), PredicateValue::Integer(2)],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Eq,
                    vec![PredicateValue::Integer(10), PredicateValue::Integer(20)],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::NotEq,
                    vec![PredicateValue::Integer(30)],
                ),
            ],
        };
        assert_eq!(
            stage_b.to_trace_plan().terms,
            vec![
                PlanTerm::Fields {
                    fields: vec!["attributes.x".into()],
                    matcher: sfst::PlanMatcher::Tokens {
                        exact: vec!["a".into(), "b".into()],
                        patterns: vec![],
                    },
                    negated: true,
                },
                PlanTerm::Fields {
                    fields: vec![
                        "resource.attributes.env".into(),
                        "attributes.env".into(),
                    ],
                    matcher: sfst::PlanMatcher::Tokens {
                        exact: vec!["prod".into()],
                        patterns: vec![],
                    },
                    negated: false,
                },
                PlanTerm::Fields {
                    fields: vec!["attributes.code".into()],
                    matcher: sfst::PlanMatcher::Number {
                        cmp: sfst::NumberCmp::Gte,
                        values: vec![500.0],
                    },
                    negated: false,
                },
                PlanTerm::Fields {
                    fields: vec!["attributes.ratio".into()],
                    matcher: sfst::PlanMatcher::Number {
                        cmp: sfst::NumberCmp::Eq,
                        values: vec![0.5, 2.0],
                    },
                    negated: false,
                },
                PlanTerm::Duration {
                    intervals: vec![(Some(10), Some(10)), (Some(20), Some(20))],
                    negated: false,
                },
                PlanTerm::Duration {
                    intervals: vec![(Some(30), Some(30))],
                    negated: true,
                },
            ]
        );
    }

    /// The span-side evaluator: token terms (exact + anchored regex,
    /// multi-valued fields), duration bounds, window, and All.
    #[test]
    fn span_side_evaluation() {
        let span = TraceSpan {
            span_id: sfst::SpanId::from([1u8; 8]),
            parent_span_id: sfst::SpanId::from([0u8; 8]),
            start_ns: 1_000,
            duration_ns: 250,
            kind: 2,
            flags: 0,
            dropped_attributes_count: 0,
            dropped_events_count: 0,
            dropped_links_count: 0,
            fields: vec![
                ("resource.attributes.service.name".into(), "svc".into()),
                ("name".into(), "GET /".into()),
                ("kind".into(), "SERVER".into()),
                ("status_code".into(), "ERROR".into()),
                ("attributes.tag".into(), "a".into()),
                ("attributes.tag".into(), "b".into()),
            ],
            events: vec![],
            links: vec![],
        };
        let matches = |conditions: Vec<Condition>, window: Option<TimeWindow>| {
            span_matches(&span, &Predicate { conditions }, window)
        };

        assert!(matches(vec![], None));
        assert!(matches(
            vec![cond(
                PredicateTarget::Attribute(TagScope::Resource, "service.name".into()),
                CompareOp::Eq,
                vec![text("svc")],
            )],
            None
        ));
        // Multi-valued field: ANY value satisfies `=`.
        assert!(matches(
            vec![cond(span_attr("tag"), CompareOp::Eq, vec![text("b")])],
            None
        ));
        assert!(!matches(
            vec![cond(span_attr("tag"), CompareOp::Eq, vec![text("c")])],
            None
        ));
        // Regex is full-value anchored ("GET" alone must not match "GET /").
        assert!(!matches(
            vec![cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Name),
                CompareOp::Regex,
                vec![text("GET")],
            )],
            None
        ));
        assert!(matches(
            vec![cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Name),
                CompareOp::Regex,
                vec![text("GET.*")],
            )],
            None
        ));
        // Absent attribute matches nothing.
        assert!(!matches(
            vec![cond(span_attr("absent"), CompareOp::Eq, vec![text("v")])],
            None
        ));
        // Conjunction + duration bounds.
        assert!(matches(
            vec![
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Status),
                    CompareOp::Eq,
                    vec![text("ERROR")],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(249)],
                ),
                cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Lte,
                    vec![PredicateValue::Integer(250)],
                ),
            ],
            None
        ));
        assert!(!matches(
            vec![cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                CompareOp::Gt,
                vec![PredicateValue::Integer(250)],
            )],
            None
        ));
        // Unsatisfiable extremes match nothing — even a span whose
        // duration IS the extreme (the saturating-conversion trap).
        let mut extreme = span.clone();
        extreme.duration_ns = i64::MAX;
        assert!(!span_matches(
            &extreme,
            &Predicate {
                conditions: vec![cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(i64::MAX)],
                )],
            },
            None
        ));
        extreme.duration_ns = i64::MIN;
        assert!(!span_matches(
            &extreme,
            &Predicate {
                conditions: vec![cond(
                    PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                    CompareOp::Lt,
                    vec![PredicateValue::Integer(i64::MIN)],
                )],
            },
            None
        ));
        // Window: span-START-in-window, half-open.
        let w = |a, b| Some(TimeWindow::new(a, b).unwrap());
        assert!(matches(vec![], w(1_000, 1_001)));
        assert!(!matches(vec![], w(0, 1_000)));
        assert!(!matches(vec![], w(1_001, 2_000)));

        // ── Stage B: negation — present-and-no-value-matches ────────
        assert!(matches(
            vec![cond(span_attr("tag"), CompareOp::NotEq, vec![text("c")])],
            None
        ));
        // A multi-valued field with ONE matching value fails `!=`.
        assert!(!matches(
            vec![cond(span_attr("tag"), CompareOp::NotEq, vec![text("a")])],
            None
        ));
        // An ABSENT attribute never satisfies a negated comparison.
        assert!(!matches(
            vec![cond(span_attr("absent"), CompareOp::NotEq, vec![text("v")])],
            None
        ));
        assert!(!matches(
            vec![cond(span_attr("absent"), CompareOp::NotRegex, vec![text("v.*")])],
            None
        ));
        // Negated regex.
        assert!(matches(
            vec![cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Name),
                CompareOp::NotRegex,
                vec![text("POST.*")],
            )],
            None
        ));
        // ── Stage B: the unscoped disjunction gathers both scopes ───
        assert!(matches(
            vec![cond(
                PredicateTarget::UnscopedAttribute("service.name".into()),
                CompareOp::Eq,
                vec![text("svc")],
            )],
            None
        ));
        assert!(matches(
            vec![cond(
                PredicateTarget::UnscopedAttribute("tag".into()),
                CompareOp::Eq,
                vec![text("b")],
            )],
            None
        ));
        assert!(!matches(
            vec![cond(
                PredicateTarget::UnscopedAttribute("nowhere".into()),
                CompareOp::NotEq,
                vec![text("v")],
            )],
            None
        ));
        // ── Stage B: numerics via the shared comparator ─────────────
        let span2 = TraceSpan {
            fields: vec![
                ("attributes.code".into(), "500".into()),
                ("attributes.label".into(), "not-a-number".into()),
            ],
            ..span.clone()
        };
        let m2 = |c: Condition| span_matches(&span2, &Predicate { conditions: vec![c] }, None);
        assert!(m2(cond(
            span_attr("code"),
            CompareOp::Gte,
            vec![PredicateValue::Integer(500)],
        )));
        assert!(m2(cond(
            span_attr("code"),
            CompareOp::Eq,
            vec![PredicateValue::Float(500.0)],
        )));
        assert!(!m2(cond(
            span_attr("code"),
            CompareOp::Gt,
            vec![PredicateValue::Integer(500)],
        )));
        // Unparseable values never match — positively OR negatively…
        assert!(!m2(cond(
            span_attr("label"),
            CompareOp::Gte,
            vec![PredicateValue::Integer(0)],
        )));
        // …but a present unparseable value DOES satisfy a negated
        // numeric equality (present, and no value numerically equals).
        assert!(m2(cond(
            span_attr("label"),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(5)],
        )));
        // Negated duration equality.
        assert!(m2(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(999)],
        )));
        assert!(!m2(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(250)],
        )));
    }
}
