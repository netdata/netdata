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
    /// Free-form string: `= != =~ !~` with text values.
    Text,
    /// Closed keyword set (kind/status labels): `= !=` with text values.
    Keyword,
    /// Nanosecond quantity: `= != > < >= <=` with integer values.
    Nanos,
    /// Fixed-width id in hex text: `= !=` with text values.
    Id,
}

fn intrinsic_class(i: TraceIntrinsic) -> IntrinsicClass {
    use TraceIntrinsic::*;
    match i {
        Name | StatusMessage | InstrumentationName | InstrumentationVersion | EventName
        | RootName | RootServiceName => IntrinsicClass::Text,
        Kind | Status => IntrinsicClass::Keyword,
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
                IntrinsicClass::Keyword | IntrinsicClass::Id => {
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

    /// Whether stage A evaluates this condition — see the module docs.
    /// `None` = evaluable; `Some(construct)` names the stage-B construct.
    fn stage_b_construct(&self) -> Option<String> {
        use TraceIntrinsic::*;
        match (&self.target, self.op) {
            // Duration bounds are the stage-A baseline; multi-value
            // equality on it is a degenerate disjunction (stage B).
            (PredicateTarget::Intrinsic(Duration), op)
                if op.is_ordering() || (op == CompareOp::Eq && self.values.len() == 1) =>
            {
                None
            }
            (PredicateTarget::Intrinsic(Duration), CompareOp::Eq) => {
                Some("multi-value duration equality".to_string())
            }
            (
                PredicateTarget::Attribute(
                    TagScope::Resource | TagScope::Span | TagScope::Instrumentation,
                    _,
                )
                | PredicateTarget::Intrinsic(
                    Name | Kind | Status | StatusMessage | InstrumentationName
                    | InstrumentationVersion,
                ),
                CompareOp::Eq | CompareOp::Regex,
            ) => {
                if self
                    .values
                    .iter()
                    .all(|v| matches!(v, PredicateValue::Text(_)))
                {
                    None
                } else {
                    Some(format!("numeric comparison on {}", self.target))
                }
            }
            (_, CompareOp::NotEq | CompareOp::NotRegex) => {
                Some(format!("negated comparison on {}", self.target))
            }
            (PredicateTarget::Attribute(TagScope::Event | TagScope::Link, _), _) => {
                Some(format!("{} (structural refine)", self.target))
            }
            (PredicateTarget::UnscopedAttribute(_), _) => {
                Some(format!("{} (resource ∪ span disjunction)", self.target))
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
            if let Some(construct) = condition.stage_b_construct() {
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
    pub(crate) fn to_trace_plan(&self) -> TracePlan {
        let mut terms = Vec::with_capacity(self.conditions.len());
        for condition in &self.conditions {
            match (&condition.target, condition.op) {
                (PredicateTarget::Intrinsic(TraceIntrinsic::Duration), op) => {
                    let PredicateValue::Integer(v) = condition.values[0] else {
                        unreachable!("validated: duration values are integers");
                    };
                    let (min_ns, max_ns) = duration_bounds(op, v);
                    terms.push(PlanTerm::Duration { min_ns, max_ns });
                }
                (target, CompareOp::Eq | CompareOp::Regex) => {
                    let field = match target {
                        PredicateTarget::Attribute(scope, key) => {
                            let prefix = scope
                                .attribute_prefix()
                                .expect("validated: not the Intrinsic scope");
                            format!("{prefix}{key}")
                        }
                        PredicateTarget::Intrinsic(i) => i
                            .dictionary_field()
                            .expect("evaluable intrinsics are dictionary-backed")
                            .to_string(),
                        PredicateTarget::UnscopedAttribute(_) => {
                            unreachable!("unscoped attributes are stage B (ensure_evaluable)")
                        }
                    };
                    let mut exact = Vec::new();
                    let mut patterns = Vec::new();
                    for value in &condition.values {
                        let PredicateValue::Text(text) = value else {
                            unreachable!("validated: evaluable token values are text");
                        };
                        match condition.op {
                            CompareOp::Eq => exact.push(text.clone()),
                            CompareOp::Regex => patterns.push(text.clone()),
                            _ => unreachable!("matched above"),
                        }
                    }
                    terms.push(PlanTerm::Tokens {
                        field,
                        exact,
                        patterns,
                    });
                }
                (target, op) => {
                    unreachable!("stage-B construct {op:?} on {target} passed ensure_evaluable")
                }
            }
        }
        TracePlan { terms }
    }
}

/// Inclusive `[min_ns, max_ns]` bounds for a duration comparison —
/// shared by the plan lowering and the span-side evaluator so the two
/// paths convert exclusivity identically (integer nanoseconds, so
/// `> v ⇔ ≥ v+1`).
fn duration_bounds(op: CompareOp, v: i64) -> (Option<i64>, Option<i64>) {
    match op {
        CompareOp::Eq => (Some(v), Some(v)),
        CompareOp::Gt => (Some(v.saturating_add(1)), None),
        CompareOp::Gte => (Some(v), None),
        CompareOp::Lt => (None, Some(v.saturating_sub(1))),
        CompareOp::Lte => (None, Some(v)),
        CompareOp::NotEq | CompareOp::Regex | CompareOp::NotRegex => {
            unreachable!("not a duration bound op (validated)")
        }
    }
}

/// One term of the span-side evaluator — the compiled twin of
/// [`sfst::PlanTerm`], built FROM the plan (one lowering; see the module
/// docs). Patterns are pre-compiled once per predicate, not per span.
enum EvalTerm {
    Tokens {
        field: String,
        exact: Vec<String>,
        patterns: Vec<regex::bytes::Regex>,
    },
    Duration {
        min_ns: Option<i64>,
        max_ns: Option<i64>,
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
                PlanTerm::Tokens {
                    field,
                    exact,
                    patterns,
                } => EvalTerm::Tokens {
                    field: field.clone(),
                    exact: exact.clone(),
                    patterns: patterns
                        .iter()
                        .map(|p| {
                            sfst::compile_pattern(p)
                                .expect("patterns validated at the request boundary")
                        })
                        .collect(),
                },
                PlanTerm::Duration { min_ns, max_ns } => EvalTerm::Duration {
                    min_ns: *min_ns,
                    max_ns: *max_ns,
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
            EvalTerm::Tokens {
                field,
                exact,
                patterns,
            } => span
                .fields
                .iter()
                .filter(|(k, _)| k == field)
                .any(|(_, v)| {
                    exact.iter().any(|e| e == v)
                        || patterns.iter().any(|p| p.is_match(v.as_bytes()))
                }),
            EvalTerm::Duration { min_ns, max_ns } => {
                min_ns.is_none_or(|lo| span.duration_ns >= lo)
                    && max_ns.is_none_or(|hi| span.duration_ns <= hi)
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
        assert!(matches!(
            err(cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Kind),
                CompareOp::Regex,
                vec![text("SER.*")],
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
    fn stage_a_evaluability_boundary() {
        let evaluable = |c: Condition| {
            Predicate { conditions: vec![c] }.ensure_evaluable().unwrap();
        };
        let rejected = |c: Condition| -> String {
            match (Predicate { conditions: vec![c] }).ensure_evaluable() {
                Err(PredicateError::NotYetEvaluable { construct }) => construct,
                other => panic!("expected NotYetEvaluable, got {other:?}"),
            }
        };

        evaluable(cond(span_attr("x"), CompareOp::Eq, vec![text("a"), text("b")]));
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
        for i in [
            TraceIntrinsic::Name,
            TraceIntrinsic::Kind,
            TraceIntrinsic::Status,
            TraceIntrinsic::StatusMessage,
            TraceIntrinsic::InstrumentationName,
            TraceIntrinsic::InstrumentationVersion,
        ] {
            evaluable(cond(
                PredicateTarget::Intrinsic(i),
                CompareOp::Eq,
                vec![text("v")],
            ));
        }
        for op in [CompareOp::Gt, CompareOp::Lt, CompareOp::Gte, CompareOp::Lte, CompareOp::Eq] {
            evaluable(cond(
                PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
                op,
                vec![PredicateValue::Integer(100)],
            ));
        }

        // Named rejections for the stage-B constructs.
        assert!(rejected(cond(span_attr("x"), CompareOp::NotEq, vec![text("v")]))
            .contains("negated"));
        assert!(rejected(cond(
            PredicateTarget::UnscopedAttribute("x".into()),
            CompareOp::Eq,
            vec![text("v")],
        ))
        .contains("unscoped"));
        assert!(rejected(cond(
            PredicateTarget::Attribute(TagScope::Event, "msg".into()),
            CompareOp::Eq,
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
            span_attr("x"),
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        ))
        .contains("Gt"));
        assert!(rejected(cond(
            span_attr("x"),
            CompareOp::Eq,
            vec![PredicateValue::Integer(5)],
        ))
        .contains("numeric"));
        assert!(rejected(cond(
            PredicateTarget::Intrinsic(TraceIntrinsic::Duration),
            CompareOp::Eq,
            vec![PredicateValue::Integer(1), PredicateValue::Integer(2)],
        ))
        .contains("multi-value"));
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
                PlanTerm::Tokens {
                    field: "resource.attributes.service.name".into(),
                    exact: vec!["svc".into()],
                    patterns: vec![],
                },
                PlanTerm::Tokens {
                    field: "status_code".into(),
                    exact: vec!["ERROR".into()],
                    patterns: vec![],
                },
                PlanTerm::Tokens {
                    field: "attributes.http.method".into(),
                    exact: vec![],
                    patterns: vec!["GET|PUT".into()],
                },
                PlanTerm::Duration {
                    min_ns: Some(101),
                    max_ns: None,
                },
                PlanTerm::Duration {
                    min_ns: None,
                    max_ns: Some(500),
                },
            ]
        );
        assert!(Predicate::all().to_trace_plan().terms.is_empty());
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
        // Window: span-START-in-window, half-open.
        let w = |a, b| Some(TimeWindow::new(a, b).unwrap());
        assert!(matches(vec![], w(1_000, 1_001)));
        assert!(!matches(vec![], w(0, 1_000)));
        assert!(!matches(vec![], w(1_001, 2_000)));
    }
}
