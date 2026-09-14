//! The neutral, owner-aware search predicate AST (phase-4c decision
//! 26A) — the FULL recorded form grammar as typed data over the
//! [`vocab`](super::vocab) enums, never grammar strings (decision 16A:
//! each wire adapter is a pure translator onto this type and owns its
//! own rendering).
//!
//! The whole grammar PARSES into this AST and validates structurally in
//! every build; stage A EVALUATES only the positive subset (conjunctions
//! of `=`/`=~` terms on resource/span/instrumentation attributes and the
//! dictionary-backed builtins except `event:name`, plus duration
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

use super::vocab::{AttributeOwner, BuiltinField};
use super::window::TimeWindow;

/// What a condition tests. Attribute owners come from the key vocabulary
/// ([`AttributeOwner::Builtin`] is not an attribute owner — request
/// error); [`AttributeOwner::Any`] is the owner-agnostic attribute
/// (pinned as the resource ∪ span disjunction, stage B); builtins are
/// the full fixed set, colon forms included (`span:id` =
/// [`BuiltinField::SpanId`], …).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PredicateTarget {
    Attribute(AttributeOwner, String),
    Builtin(BuiltinField),
}

impl std::fmt::Display for PredicateTarget {
    /// Diagnostic rendering (error messages only — no wire grammar).
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PredicateTarget::Attribute(AttributeOwner::Any, key) => {
                write!(f, "any-owner attribute {key:?}")
            }
            PredicateTarget::Attribute(owner, key) => write!(f, "{owner:?} attribute {key:?}"),
            PredicateTarget::Builtin(i) => write!(f, "builtin field {i:?}"),
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
    /// Full-value-anchored regex: `=~` matches the WHOLE value, not a
    /// substring (the anchoring the experiment's wire grammar pinned).
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

/// The predicate: a conjunction of conditions over one per-trace span
/// group (pinned group semantics, C-3: a trace matches when at least one retained
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
    #[error("attribute {key:?} cannot live under the Builtin owner")]
    AttributeUnderBuiltinOwner { key: String },
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
    #[error("{target} takes {width}-char hex ids, got {value:?}")]
    InvalidIdValue {
        target: String,
        value: String,
        width: usize,
    },
    /// The construct is valid grammar but outside this build's evaluable
    /// subset (the stage-A/stage-B boundary, decision 26A) — named so the
    /// caller sees exactly which construct to avoid, never a silently
    /// wrong result.
    #[error("not yet evaluable: {construct}")]
    NotYetEvaluable { construct: String },
}

/// The builtin's comparison class — which operators and value types
/// are structurally valid on it (full grammar, both stages).
enum BuiltinClass {
    /// String-valued (free-form names/messages AND the closed kind/
    /// status label sets — regex over labels is valid grammar, pin 26A):
    /// `= != =~ !~` with text values.
    Text,
    /// Nanosecond quantity: `= != > < >= <=` with integer values.
    Nanos,
    /// Fixed-width id in hex text: `= !=` with text values.
    Id,
}

fn builtin_class(i: BuiltinField) -> BuiltinClass {
    use BuiltinField::*;
    match i {
        Name | Kind | Status | StatusMessage | InstrumentationName | InstrumentationVersion
        | EventName | RootName | RootServiceName => BuiltinClass::Text,
        Duration | TraceDuration | EventTimeSinceStart => BuiltinClass::Nanos,
        SpanId | ParentSpanId | TraceId | LinkSpanId | LinkTraceId => BuiltinClass::Id,
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
            PredicateTarget::Attribute(AttributeOwner::Builtin, key) => {
                return Err(PredicateError::AttributeUnderBuiltinOwner { key: key.clone() });
            }
            // Attributes take every op; regex needs text patterns,
            // ordering needs numbers (dictionary-numeric, stage B).
            PredicateTarget::Attribute(..) => {
                if self.op.is_regex() && !all_text {
                    return Err(PredicateError::TextValueRequired { op: self.op, target });
                }
                if self.op.is_ordering() && !all_numeric {
                    return Err(PredicateError::NumericValueRequired { op: self.op, target });
                }
            }
            PredicateTarget::Builtin(i) => match builtin_class(*i) {
                BuiltinClass::Text => {
                    if self.op.is_ordering() {
                        return Err(PredicateError::InvalidOpForTarget { op: self.op, target });
                    }
                    if !all_text {
                        return Err(PredicateError::TextValueRequired { op: self.op, target });
                    }
                }
                BuiltinClass::Id => {
                    if !matches!(self.op, CompareOp::Eq | CompareOp::NotEq) {
                        return Err(PredicateError::InvalidOpForTarget { op: self.op, target });
                    }
                    if !all_text {
                        return Err(PredicateError::TextValueRequired { op: self.op, target });
                    }
                    // Ids are fixed-width hex text (W3C rendering):
                    // 16 chars for span ids, 32 for trace ids.
                    let width = match i {
                        BuiltinField::TraceId | BuiltinField::LinkTraceId => 32,
                        _ => 16,
                    };
                    for value in &self.values {
                        let PredicateValue::Text(hex) = value else {
                            unreachable!("all_text checked above");
                        };
                        if hex.len() != width
                            || !hex.bytes().all(|b| b.is_ascii_hexdigit())
                        {
                            return Err(PredicateError::InvalidIdValue {
                                target,
                                value: hex.clone(),
                                width,
                            });
                        }
                    }
                }
                BuiltinClass::Nanos => {
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
    /// set covers negation, the any-owner disjunction, and
    /// dictionary-numeric comparisons; the remaining gaps are the
    /// colon-set ids, event/link structural refine, and the
    /// trace-level builtins (steps 7-9).
    fn unevaluable_construct(&self) -> Option<String> {
        use BuiltinField::*;
        match (&self.target, self.op) {
            // Duration: bounds, equality (any arity — an interval set),
            // and negated equality all evaluate on the DURN column.
            (PredicateTarget::Builtin(Duration), op)
                if op.is_ordering() || matches!(op, CompareOp::Eq | CompareOp::NotEq) =>
            {
                None
            }
            // Attribute owners (incl. the any-owner disjunction): every
            // op — tokens for text, the dictionary-numeric path for
            // numbers, presence ∩ complement for negation.
            (
                PredicateTarget::Attribute(
                    AttributeOwner::Resource
                    | AttributeOwner::Span
                    | AttributeOwner::Instrumentation
                    | AttributeOwner::Any,
                    _,
                ),
                _,
            ) => None,
            // Dictionary-backed builtins except event:name (refine).
            (
                PredicateTarget::Builtin(
                    Name | Kind | Status | StatusMessage | InstrumentationName
                    | InstrumentationVersion,
                ),
                CompareOp::Eq | CompareOp::Regex | CompareOp::NotEq | CompareOp::NotRegex,
            ) => None,
            // The colon-set ids: span/parent by column scan (both
            // polarities — single-valued, no subgroup fork); trace:id
            // pins/filters the candidate set; link ids POSITIVE only
            // (the negated subgroup semantics are an open design
            // question for the refine step).
            (
                PredicateTarget::Builtin(SpanId | ParentSpanId | TraceId),
                CompareOp::Eq | CompareOp::NotEq,
            ) => None,
            (
                PredicateTarget::Builtin(LinkSpanId | LinkTraceId),
                CompareOp::Eq,
            ) => None,
            (PredicateTarget::Builtin(LinkSpanId | LinkTraceId), CompareOp::NotEq) => {
                Some(format!("negated {} (subgroup semantics, refine step)", self.target))
            }
            // event:name POSITIVE forms evaluate through the event
            // subgroup (any number of conditions — a single event must
            // satisfy them all); negated forms wait for the
            // subgroup-semantics decision.
            (PredicateTarget::Builtin(EventName), CompareOp::Eq | CompareOp::Regex) => None,
            (PredicateTarget::Builtin(EventName), CompareOp::NotEq | CompareOp::NotRegex) => {
                Some(format!("negated {} (subgroup semantics, refine step)", self.target))
            }
            // Event/link-scoped attributes: POSITIVE forms evaluate
            // through the subgroup refine; negated forms sit on the
            // recorded open question (flat negation vs single-item
            // subgroup semantics).
            (
                PredicateTarget::Attribute(AttributeOwner::Event | AttributeOwner::Link, _),
                CompareOp::Eq | CompareOp::Regex | CompareOp::Gt | CompareOp::Lt
                | CompareOp::Gte | CompareOp::Lte,
            ) => None,
            (PredicateTarget::Attribute(AttributeOwner::Event | AttributeOwner::Link, _), _) => {
                Some(format!("negated {} (subgroup semantics, refine step)", self.target))
            }
            // event:timeSinceStart computes in the refine; != would be a
            // negated per-event condition — deferred with the fork for a
            // uniform rule.
            (PredicateTarget::Builtin(EventTimeSinceStart), op)
                if op.is_ordering() || op == CompareOp::Eq =>
            {
                None
            }
            (PredicateTarget::Builtin(EventTimeSinceStart), _) => {
                Some(format!("negated {} (subgroup semantics, refine step)", self.target))
            }
            // Trace-level builtins evaluate post-assembly (tri-state,
            // decision 15). Values are single-valued per trace, so
            // negation carries no subgroup fork.
            (
                PredicateTarget::Builtin(RootName | RootServiceName),
                CompareOp::Eq | CompareOp::NotEq | CompareOp::Regex | CompareOp::NotRegex,
            ) => None,
            (PredicateTarget::Builtin(TraceDuration), op)
                if op.is_ordering() || matches!(op, CompareOp::Eq | CompareOp::NotEq) =>
            {
                None
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
    /// builtins (`RootName`, `RootServiceName`, `TraceDuration`)
    /// evaluate post-assembly as tri-state (decision 15); EVERYTHING the
    /// span-side evaluator and the file plans see — and everything
    /// `matched_count`/`spans_per_trace` are defined over — is the span-local part.
    pub(crate) fn partition(&self) -> (Predicate, Predicate) {
        use BuiltinField::*;
        let (trace_level, span_local): (Vec<Condition>, Vec<Condition>) =
            self.conditions.iter().cloned().partition(|c| {
                matches!(
                    c.target,
                    PredicateTarget::Builtin(RootName | RootServiceName | TraceDuration)
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

    /// Strip `trace:id` conditions out of the predicate — they are
    /// neither span-local (a `TraceSpan` carries no trace id) nor
    /// trace-level builtins; they constrain the ENGINE's candidate
    /// set: `=` values intersect into a PIN set (phase-1 discovery is
    /// skipped — pins go straight to assembly), `!=` values union into
    /// an exclusion filter at pool admission. Run BEFORE the R3-2
    /// partition. Hex parsing is infallible post-validation.
    pub(crate) fn split_trace_id_conditions(&self) -> (Predicate, TraceIdConstraints) {
        let mut rest = Vec::with_capacity(self.conditions.len());
        let mut pins: Option<std::collections::BTreeSet<sfst::TraceId>> = None;
        let mut excluded: std::collections::BTreeSet<sfst::TraceId> =
            std::collections::BTreeSet::new();
        for condition in &self.conditions {
            if !matches!(
                condition.target,
                PredicateTarget::Builtin(BuiltinField::TraceId)
            ) {
                rest.push(condition.clone());
                continue;
            }
            let ids = condition.values.iter().map(|v| {
                let PredicateValue::Text(hex) = v else {
                    unreachable!("validated: id values are text");
                };
                sfst::TraceId::from(parse_hex::<16>(hex))
            });
            match condition.op {
                CompareOp::Eq => {
                    let set: std::collections::BTreeSet<sfst::TraceId> = ids.collect();
                    pins = Some(match pins {
                        None => set,
                        Some(prev) => prev.intersection(&set).copied().collect(),
                    });
                }
                CompareOp::NotEq => excluded.extend(ids),
                _ => unreachable!("validated: trace:id takes = / !="),
            }
        }
        (
            Predicate { conditions: rest },
            TraceIdConstraints { pins, excluded },
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
        // Event/link-scoped conditions collect into ONE subgroup each
        // (pinned span-group semantics: a single event/link satisfies the
        // whole scoped conjunction), appended after the span terms.
        let mut event_group: Vec<sfst::GroupCondition> = Vec::new();
        let mut link_group: Vec<sfst::GroupCondition> = Vec::new();
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
                (PredicateTarget::Builtin(BuiltinField::Duration), CompareOp::NotEq) => {
                    terms.push(PlanTerm::Duration {
                        intervals: integers().into_iter().map(|v| (Some(v), Some(v))).collect(),
                        negated: true,
                    });
                }
                (PredicateTarget::Builtin(BuiltinField::Duration), CompareOp::Eq)
                    if condition.values.len() > 1 =>
                {
                    terms.push(PlanTerm::Duration {
                        intervals: integers().into_iter().map(|v| (Some(v), Some(v))).collect(),
                        negated: false,
                    });
                }
                (PredicateTarget::Builtin(BuiltinField::Duration), op) => {
                    let PredicateValue::Integer(n) = condition.values[0] else {
                        unreachable!("validated: duration values are integers");
                    };
                    let (min_ns, max_ns) = duration_bounds(op, n);
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
                (
                    PredicateTarget::Builtin(
                        i @ (BuiltinField::SpanId | BuiltinField::ParentSpanId),
                    ),
                    op,
                ) => {
                    terms.push(PlanTerm::IdColumn {
                        column: match i {
                            BuiltinField::SpanId => sfst::IdColumnKind::SpanId,
                            _ => sfst::IdColumnKind::ParentSpanId,
                        },
                        ids: hex_span_ids(&condition.values),
                        negated: matches!(op, CompareOp::NotEq),
                    });
                }
                (
                    PredicateTarget::Builtin(BuiltinField::LinkSpanId),
                    CompareOp::Eq,
                ) => {
                    link_group.push(sfst::GroupCondition::LinkSpanIds(hex_span_ids(
                        &condition.values,
                    )));
                }
                (
                    PredicateTarget::Builtin(BuiltinField::LinkTraceId),
                    CompareOp::Eq,
                ) => {
                    link_group.push(sfst::GroupCondition::LinkTraceIds(
                        condition
                            .values
                            .iter()
                            .map(|v| {
                                let PredicateValue::Text(hex) = v else {
                                    unreachable!("validated: id values are text");
                                };
                                sfst::TraceId::from(parse_hex::<16>(hex))
                            })
                            .collect(),
                    ));
                }
                (PredicateTarget::Builtin(BuiltinField::EventName), op) => {
                    event_group.push(sfst::GroupCondition::Field {
                        field: BuiltinField::EventName
                            .dictionary_field()
                            .expect("dictionary-backed")
                            .to_string(),
                        matcher: condition_matcher(op, &condition.values),
                    });
                }
                (PredicateTarget::Builtin(BuiltinField::EventTimeSinceStart), op) => {
                    let intervals: Vec<(Option<i64>, Option<i64>)> = condition
                        .values
                        .iter()
                        .map(|v| {
                            let PredicateValue::Integer(n) = v else {
                                unreachable!("validated: nanos values are integers");
                            };
                            duration_bounds(op, *n)
                        })
                        .collect();
                    event_group.push(sfst::GroupCondition::TimeSinceStart { intervals });
                }
                (PredicateTarget::Attribute(scope @ (AttributeOwner::Event | AttributeOwner::Link), key), op) => {
                    let field = format!(
                        "{}{key}",
                        scope.attribute_prefix().expect("attribute scope")
                    );
                    let group = if *scope == AttributeOwner::Event {
                        &mut event_group
                    } else {
                        &mut link_group
                    };
                    group.push(sfst::GroupCondition::Field {
                        field,
                        matcher: condition_matcher(op, &condition.values),
                    });
                }
                (PredicateTarget::Builtin(BuiltinField::TraceId), _) => {
                    unreachable!("trace:id conditions are split out before lowering")
                }
                (target, op) => {
                    let fields: Vec<String> = match target {
                        // The any-owner attribute is the pinned
                        // resource ∪ span disjunction.
                        PredicateTarget::Attribute(AttributeOwner::Any, key) => vec![
                            format!(
                                "{}{key}",
                                AttributeOwner::Resource
                                    .attribute_prefix()
                                    .expect("attribute owner")
                            ),
                            format!(
                                "{}{key}",
                                AttributeOwner::Span.attribute_prefix().expect("attribute owner")
                            ),
                        ],
                        PredicateTarget::Attribute(owner, key) => {
                            let prefix = owner
                                .attribute_prefix()
                                .expect("validated: a concrete attribute owner (not Builtin or Any)");
                            vec![format!("{prefix}{key}")]
                        }
                        PredicateTarget::Builtin(i) => vec![
                            i.dictionary_field()
                                .expect("evaluable builtins are dictionary-backed")
                                .to_string(),
                        ],
                    };
                    let negated = matches!(op, CompareOp::NotEq | CompareOp::NotRegex);
                    terms.push(PlanTerm::Fields {
                        fields,
                        matcher: condition_matcher(op, &condition.values),
                        negated,
                    });
                }
            }
        }
        if let Some((min_ns, max_ns)) = bound {
            terms.push(PlanTerm::duration(min_ns, max_ns));
        }
        if !event_group.is_empty() {
            terms.push(PlanTerm::EventGroup {
                conditions: event_group,
            });
        }
        if !link_group.is_empty() {
            terms.push(PlanTerm::LinkGroup {
                conditions: link_group,
            });
        }
        TracePlan { terms }
    }
}

/// The dictionary matcher of one condition — token selection for text
/// values, the dictionary-numeric comparison for numbers (negation is
/// the TERM's flag, so `!=`/`!~` produce the same matcher as `=`/`=~`).
fn condition_matcher(op: CompareOp, values: &[PredicateValue]) -> sfst::PlanMatcher {
    let numeric = values
        .iter()
        .any(|v| matches!(v, PredicateValue::Integer(_) | PredicateValue::Float(_)));
    if numeric {
        let values: Vec<f64> = values
            .iter()
            .map(|v| match v {
                PredicateValue::Integer(n) => *n as f64,
                PredicateValue::Float(f) => *f,
                PredicateValue::Text(_) => unreachable!("validated: values are homogeneous"),
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
        for value in values {
            let PredicateValue::Text(text) = value else {
                unreachable!("validated: values are homogeneous");
            };
            match op {
                CompareOp::Eq | CompareOp::NotEq => exact.push(text.clone()),
                CompareOp::Regex | CompareOp::NotRegex => patterns.push(text.clone()),
                _ => unreachable!("validated: ordering needs numeric values"),
            }
        }
        sfst::PlanMatcher::Tokens { exact, patterns }
    }
}

/// The engine-level candidate constraints of `trace:id` conditions
/// (see [`Predicate::split_trace_id_conditions`]).
pub(crate) struct TraceIdConstraints {
    /// `Some` = the candidate set is EXACTLY these ids (already the
    /// intersection of every `trace:id =` condition; may be empty).
    pub(crate) pins: Option<std::collections::BTreeSet<sfst::TraceId>>,
    /// Ids excluded by `trace:id !=` conditions.
    pub(crate) excluded: std::collections::BTreeSet<sfst::TraceId>,
}

/// Parse a validated fixed-width hex id (validation guarantees length
/// and hex digits).
fn parse_hex<const N: usize>(hex: &str) -> [u8; N] {
    let mut bytes = [0u8; N];
    for (i, chunk) in hex.as_bytes().chunks(2).enumerate() {
        bytes[i] = u8::from_str_radix(std::str::from_utf8(chunk).expect("ascii hex"), 16)
            .expect("validated hex");
    }
    bytes
}

fn hex_span_ids(values: &[PredicateValue]) -> Vec<sfst::SpanId> {
    values
        .iter()
        .map(|v| {
            let PredicateValue::Text(hex) = v else {
                unreachable!("validated: id values are text");
            };
            sfst::SpanId::from(parse_hex::<8>(hex))
        })
        .collect()
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

/// The compiled token/number matcher of one span-side term. `pub(crate)`
/// for ONE extra consumer: the trace-level gate applies the exact same
/// compiled matcher to rollup-resolved root values (the one-lowering
/// rule — the gate must never re-derive matching from raw conditions).
pub(crate) enum EvalMatcher {
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
    pub(crate) fn value_matches(&self, value: &str) -> bool {
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
    IdColumn {
        column: sfst::IdColumnKind,
        ids: Vec<sfst::SpanId>,
        negated: bool,
    },
    EventGroup {
        conditions: Vec<EvalGroupCondition>,
    },
    LinkGroup {
        conditions: Vec<EvalGroupCondition>,
    },
}

/// A subgroup condition on the span side, with the R3-3 storage→bare
/// key translation applied once at build: materialized spans expose
/// event/link attributes PREFIX-STRIPPED and event names structured.
enum EvalGroupCondition {
    EventName(EvalMatcher),
    Attr { bare_key: String, matcher: EvalMatcher },
    TimeSince(Vec<(Option<i64>, Option<i64>)>),
    LinkSpanIds(Vec<sfst::SpanId>),
    LinkTraceIds(Vec<sfst::TraceId>),
}

/// The span-side predicate evaluator — the single source of truth for
/// what a span-local predicate MEANS (pin R2-9): the tail's phase 1
/// evaluates it per decoded span, phase 2 re-evaluates it per retained
/// canonical span, and `matched_count`/`spans_per_trace` are defined by it.
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
                    matcher: eval_matcher(matcher),
                    negated: *negated,
                },
                PlanTerm::Duration { intervals, negated } => EvalTerm::Duration {
                    intervals: intervals.clone(),
                    negated: *negated,
                },
                PlanTerm::IdColumn {
                    column,
                    ids,
                    negated,
                } => EvalTerm::IdColumn {
                    column: *column,
                    ids: ids.clone(),
                    negated: *negated,
                },
                PlanTerm::EventGroup { conditions } => EvalTerm::EventGroup {
                    conditions: conditions.iter().map(eval_group_condition).collect(),
                },
                PlanTerm::LinkGroup { conditions } => EvalTerm::LinkGroup {
                    conditions: conditions.iter().map(eval_group_condition).collect(),
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
                // the any-owner disjunction; multi-valued fields yield
                // several entries; event/link names and attributes live
                // in the subgroup terms, never here). Negation is the
                // pinned presence ∩ complement: present with NO matching
                // value.
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
            EvalTerm::IdColumn {
                column,
                ids,
                negated,
            } => {
                let id = match column {
                    sfst::IdColumnKind::SpanId => span.span_id,
                    sfst::IdColumnKind::ParentSpanId => span.parent_span_id,
                };
                // The UNSET sentinel is ABSENT for both polarities: a
                // root span never satisfies `span:parentID != x`.
                if id.is_unset() {
                    return false;
                }
                ids.contains(&id) != *negated
            }
            // The pinned subgroup semantics: ONE event/link satisfies
            // every condition of its group.
            EvalTerm::EventGroup { conditions } => span.events.iter().any(|e| {
                conditions.iter().all(|c| match c {
                    EvalGroupCondition::EventName(m) => m.value_matches(&e.name),
                    EvalGroupCondition::Attr { bare_key, matcher } => e
                        .attributes
                        .iter()
                        .any(|(k, v)| k == bare_key && matcher.value_matches(v)),
                    EvalGroupCondition::TimeSince(intervals) => {
                        // Saturating, mirroring the index-side refine.
                        let t = i64::try_from(e.time_unix_nano).unwrap_or(i64::MAX);
                        let dt = t.saturating_sub(span.start_ns);
                        intervals.iter().any(|&(lo, hi)| {
                            lo.is_none_or(|lo| dt >= lo) && hi.is_none_or(|hi| dt <= hi)
                        })
                    }
                    _ => false, // link-only conditions never hold on events
                })
            }),
            EvalTerm::LinkGroup { conditions } => span.links.iter().any(|l| {
                conditions.iter().all(|c| match c {
                    EvalGroupCondition::Attr { bare_key, matcher } => l
                        .attributes
                        .iter()
                        .any(|(k, v)| k == bare_key && matcher.value_matches(v)),
                    EvalGroupCondition::LinkSpanIds(ids) => ids.contains(&l.span_id),
                    EvalGroupCondition::LinkTraceIds(ids) => ids.contains(&l.trace_id),
                    _ => false, // event-only conditions never hold on links
                })
            }),
        })
    }
}

/// Compile one plan matcher for span-side use (patterns validated at
/// the request boundary).
fn eval_matcher(matcher: &sfst::PlanMatcher) -> EvalMatcher {
    match matcher {
        sfst::PlanMatcher::Tokens { exact, patterns } => EvalMatcher::Tokens {
            exact: exact.clone(),
            patterns: patterns
                .iter()
                .map(|p| {
                    sfst::compile_pattern(p).expect("patterns validated at the request boundary")
                })
                .collect(),
        },
        sfst::PlanMatcher::Number { cmp, values } => EvalMatcher::Number {
            cmp: *cmp,
            values: values.clone(),
        },
    }
}

/// Build one span-side subgroup condition from its plan form — the
/// single site of the R3-3 prefix translation.
fn eval_group_condition(c: &sfst::GroupCondition) -> EvalGroupCondition {
    match c {
        sfst::GroupCondition::Field { field, matcher } => {
            let matcher = eval_matcher(matcher);
            if field == "events.name" {
                EvalGroupCondition::EventName(matcher)
            } else {
                let bare = field
                    .strip_prefix("events.attributes.")
                    .or_else(|| field.strip_prefix("links.attributes."))
                    .expect("subgroup fields are events.name or event/link attributes");
                EvalGroupCondition::Attr {
                    bare_key: bare.to_string(),
                    matcher,
                }
            }
        }
        sfst::GroupCondition::TimeSinceStart { intervals } => {
            EvalGroupCondition::TimeSince(intervals.clone())
        }
        sfst::GroupCondition::LinkSpanIds(ids) => EvalGroupCondition::LinkSpanIds(ids.clone()),
        sfst::GroupCondition::LinkTraceIds(ids) => EvalGroupCondition::LinkTraceIds(ids.clone()),
    }
}

/// The post-assembly evaluator of the TRACE-LEVEL partition half
/// (decision 15 / pin R3-2): boolean over the assembled trace's root
/// name, root service, and envelope duration — the ENGINE owns the
/// tri-state (an assembled trace whose values are unreliable — capped
/// or degraded — is excluded as indeterminate before this evaluator is
/// consulted). The root inputs are the TRUE root's values only
/// (decision 1D — see [`search`](super::search)'s module docs): a
/// rootless trace passes `None`s, never the display-side promoted
/// root. Absent root values never satisfy a condition, negated
/// forms included (the absent-never-satisfies rule; values are
/// single-valued per trace, so no subgroup fork exists).
pub(crate) struct TraceLevelEval {
    conditions: Vec<TraceLevelCondition>,
}

/// Which root field a gate-prunable condition targets. The gate maps
/// this to the storage spelling (`name` /
/// [`resource_service_field`](super::vocab::resource_service_field)) —
/// the same spellings the rollup capture used.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum GateRootField {
    Service,
    Name,
}

enum TraceLevelCondition {
    RootName { matcher: EvalMatcher, negated: bool },
    RootService { matcher: EvalMatcher, negated: bool },
    Duration {
        intervals: Vec<(Option<i64>, Option<i64>)>,
        negated: bool,
    },
}

impl TraceLevelEval {
    /// Compile the trace-level predicate half (validated, evaluable).
    pub(crate) fn new(trace_level: &Predicate) -> Self {
        let conditions = trace_level
            .conditions
            .iter()
            .map(|condition| {
                let negated =
                    matches!(condition.op, CompareOp::NotEq | CompareOp::NotRegex);
                match &condition.target {
                    PredicateTarget::Builtin(BuiltinField::RootName) => {
                        TraceLevelCondition::RootName {
                            matcher: eval_matcher(&condition_matcher(condition.op, &condition.values)),
                            negated,
                        }
                    }
                    PredicateTarget::Builtin(BuiltinField::RootServiceName) => {
                        TraceLevelCondition::RootService {
                            matcher: eval_matcher(&condition_matcher(condition.op, &condition.values)),
                            negated,
                        }
                    }
                    PredicateTarget::Builtin(BuiltinField::TraceDuration) => {
                        let intervals = condition
                            .values
                            .iter()
                            .map(|v| {
                                let PredicateValue::Integer(n) = v else {
                                    unreachable!("validated: nanos values are integers");
                                };
                                duration_bounds(
                                    if negated { CompareOp::Eq } else { condition.op },
                                    *n,
                                )
                            })
                            .collect();
                        TraceLevelCondition::Duration { intervals, negated }
                    }
                    other => unreachable!("not a trace-level target: {other}"),
                }
            })
            .collect();
        Self { conditions }
    }

    /// The gate-prunable POSITIVE root conditions: `(field, matcher)`.
    /// Negated (`!=`, `!~`) root conditions are deliberately absent —
    /// the gate cannot PROVE an assembled root equals a specific value
    /// (multiple candidate roots plus the ruled divergence mechanisms
    /// documented in [`search`](super::search)'s module docs), so they
    /// never prune and the gate never sees them.
    pub(crate) fn prunable_root_conditions(
        &self,
    ) -> impl Iterator<Item = (GateRootField, &EvalMatcher)> {
        self.conditions.iter().filter_map(|c| match c {
            TraceLevelCondition::RootName { matcher, negated: false } => {
                Some((GateRootField::Name, matcher))
            }
            TraceLevelCondition::RootService { matcher, negated: false } => {
                Some((GateRootField::Service, matcher))
            }
            _ => None,
        })
    }

    /// The single duration lower bound the gate may prune on, if any:
    /// per POSITIVE duration condition, the MIN over its compiled
    /// intervals' `lo` values (an interval without a `lo` means the
    /// condition has no lower bound at all); across conjunctive
    /// conditions, the MAX. Negated conditions are skipped on the
    /// NEGATED FLAG, never on interval shape — `!=` compiles to `Eq`
    /// point-intervals whose `lo` would false-prune. The unsatisfiable
    /// EMPTY interval keeps its `lo = i64::MAX`, which is sound (the
    /// condition rejects everything; so does the gate) — do not "fix"
    /// it. Upper bounds never contribute: the rollup envelope
    /// over-estimates the canonical duration, so only a lower-bound
    /// violation is provable.
    pub(crate) fn duration_lower_bound(&self) -> Option<i64> {
        let mut best: Option<i64> = None;
        for c in &self.conditions {
            let TraceLevelCondition::Duration { intervals, negated: false } = c else {
                continue;
            };
            let mut cond_lo: Option<i64> = None;
            let mut unbounded = false;
            for &(lo, _) in intervals {
                match lo {
                    None => {
                        unbounded = true;
                        break;
                    }
                    Some(lo) => cond_lo = Some(cond_lo.map_or(lo, |a| a.min(lo))),
                }
            }
            if unbounded {
                continue;
            }
            if let Some(lo) = cond_lo {
                best = Some(best.map_or(lo, |b| b.max(lo)));
            }
        }
        best
    }

    /// Whether ANY root condition exists, either polarity. Under the
    /// true-root filter semantics (decision 1D) a proven-rootless
    /// candidate fails EVERY root condition — negated included
    /// (absent-never-satisfies) — so any root condition supports the
    /// gate's no-root prune even when it cannot support the
    /// all-claims-fail rule.
    pub(crate) fn has_root_conditions(&self) -> bool {
        self.conditions.iter().any(|c| {
            matches!(
                c,
                TraceLevelCondition::RootName { .. } | TraceLevelCondition::RootService { .. }
            )
        })
    }

    /// Whether ANY condition gives the gate something to prune on —
    /// the engagement check.
    pub(crate) fn has_prunable_condition(&self) -> bool {
        self.has_root_conditions() || self.duration_lower_bound().is_some()
    }

    /// Whether the assembled trace's values satisfy every condition.
    pub(crate) fn matches(
        &self,
        root_name: Option<&str>,
        root_service: Option<&str>,
        trace_duration_ns: i64,
    ) -> bool {
        let text = |value: Option<&str>, matcher: &EvalMatcher, negated: bool| match value {
            None => false, // absent never satisfies, negated included
            Some(v) => matcher.value_matches(v) != negated,
        };
        self.conditions.iter().all(|c| match c {
            TraceLevelCondition::RootName { matcher, negated } => {
                text(root_name, matcher, *negated)
            }
            TraceLevelCondition::RootService { matcher, negated } => {
                text(root_service, matcher, *negated)
            }
            TraceLevelCondition::Duration { intervals, negated } => {
                let in_any = intervals.iter().any(|&(lo, hi)| {
                    lo.is_none_or(|lo| trace_duration_ns >= lo)
                        && hi.is_none_or(|hi| trace_duration_ns <= hi)
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
        PredicateTarget::Attribute(AttributeOwner::Span, key.to_string())
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
            PredicateTarget::Attribute(AttributeOwner::Any, "x".into()),
            CompareOp::Regex,
            vec![text("a|b")],
        ));
        ok(cond(
            PredicateTarget::Builtin(BuiltinField::TraceDuration),
            CompareOp::Gte,
            vec![PredicateValue::Integer(5)],
        ));
        ok(cond(
            PredicateTarget::Builtin(BuiltinField::SpanId),
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
                PredicateTarget::Attribute(AttributeOwner::Builtin, "x".into()),
                CompareOp::Eq,
                vec![text("v")],
            )),
            PredicateError::AttributeUnderBuiltinOwner { .. }
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Builtin(BuiltinField::Name),
                CompareOp::Gt,
                vec![text("v")],
            )),
            PredicateError::InvalidOpForTarget { .. }
        ));
        // Regex over the kind/status LABEL sets is valid grammar (26A).
        ok(cond(
            PredicateTarget::Builtin(BuiltinField::Kind),
            CompareOp::Regex,
            vec![text("SER.*")],
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Builtin(BuiltinField::Kind),
                CompareOp::Gt,
                vec![text("SERVER")],
            )),
            PredicateError::InvalidOpForTarget { .. }
        ));
        assert!(matches!(
            err(cond(
                PredicateTarget::Builtin(BuiltinField::Duration),
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
                PredicateTarget::Builtin(BuiltinField::Status),
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

        // Attribute owners and the any-owner disjunction: every op.
        evaluable(cond(span_attr("x"), CompareOp::Eq, vec![text("a"), text("b")]));
        evaluable(cond(span_attr("x"), CompareOp::NotEq, vec![text("v")]));
        evaluable(cond(span_attr("x"), CompareOp::NotRegex, vec![text("v.*")]));
        evaluable(cond(
            PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
            CompareOp::Regex,
            vec![text("svc-.*")],
        ));
        evaluable(cond(
            PredicateTarget::Attribute(AttributeOwner::Instrumentation, "lib".into()),
            CompareOp::Eq,
            vec![text("v")],
        ));
        evaluable(cond(
            PredicateTarget::Attribute(AttributeOwner::Any, "x".into()),
            CompareOp::Eq,
            vec![text("v")],
        ));
        evaluable(cond(
            PredicateTarget::Attribute(AttributeOwner::Any, "x".into()),
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
        // Dictionary-backed builtins except event:name: all four ops.
        for i in [
            BuiltinField::Name,
            BuiltinField::Kind,
            BuiltinField::Status,
            BuiltinField::StatusMessage,
            BuiltinField::InstrumentationName,
            BuiltinField::InstrumentationVersion,
        ] {
            for op in [CompareOp::Eq, CompareOp::Regex, CompareOp::NotEq, CompareOp::NotRegex] {
                evaluable(cond(
                    PredicateTarget::Builtin(i),
                    op,
                    vec![text("v")],
                ));
            }
        }
        // Duration: bounds, multi-value equality, negated equality.
        for op in [CompareOp::Gt, CompareOp::Lt, CompareOp::Gte, CompareOp::Lte, CompareOp::Eq] {
            evaluable(cond(
                PredicateTarget::Builtin(BuiltinField::Duration),
                op,
                vec![PredicateValue::Integer(100)],
            ));
        }
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::Duration),
            CompareOp::Eq,
            vec![PredicateValue::Integer(1), PredicateValue::Integer(2)],
        ));
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::Duration),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(3)],
        ));

        // Subgroup positives are evaluable through the refine…
        evaluable(cond(
            PredicateTarget::Attribute(AttributeOwner::Event, "msg".into()),
            CompareOp::Eq,
            vec![text("v")],
        ));
        evaluable(cond(
            PredicateTarget::Attribute(AttributeOwner::Link, "rel".into()),
            CompareOp::Regex,
            vec![text("v.*")],
        ));
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::EventTimeSinceStart),
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        ));
        // …their NEGATED forms sit on the recorded open question.
        assert!(rejected(cond(
            PredicateTarget::Attribute(AttributeOwner::Event, "msg".into()),
            CompareOp::NotEq,
            vec![text("v")],
        ))
        .contains("subgroup"));
        assert!(rejected(cond(
            PredicateTarget::Attribute(AttributeOwner::Link, "rel".into()),
            CompareOp::NotEq,
            vec![text("v")],
        ))
        .contains("subgroup"));
        // The colon set: span/parent/trace ids both polarities; link
        // ids and event:name positive-only (the negated subgroup
        // semantics are the recorded open question).
        for i in [
            BuiltinField::SpanId,
            BuiltinField::ParentSpanId,
        ] {
            for op in [CompareOp::Eq, CompareOp::NotEq] {
                evaluable(cond(
                    PredicateTarget::Builtin(i),
                    op,
                    vec![text("00f067aa0ba902b7")],
                ));
            }
        }
        for op in [CompareOp::Eq, CompareOp::NotEq] {
            evaluable(cond(
                PredicateTarget::Builtin(BuiltinField::TraceId),
                op,
                vec![text("4bf92f3577b34da6a3ce929d0e0e4736")],
            ));
        }
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::LinkSpanId),
            CompareOp::Eq,
            vec![text("00f067aa0ba902b7")],
        ));
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::LinkTraceId),
            CompareOp::Eq,
            vec![text("4bf92f3577b34da6a3ce929d0e0e4736")],
        ));
        assert!(rejected(cond(
            PredicateTarget::Builtin(BuiltinField::LinkSpanId),
            CompareOp::NotEq,
            vec![text("00f067aa0ba902b7")],
        ))
        .contains("subgroup"));
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::EventName),
            CompareOp::Eq,
            vec![text("v")],
        ));
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::EventName),
            CompareOp::Regex,
            vec![text("v.*")],
        ));
        assert!(rejected(cond(
            PredicateTarget::Builtin(BuiltinField::EventName),
            CompareOp::NotEq,
            vec![text("v")],
        ))
        .contains("subgroup"));
        // TWO event:name conditions form a single-event subgroup —
        // well-defined (one event named both = unsatisfiable) and
        // evaluable through the refine.
        let two = Predicate {
            conditions: vec![
                cond(
                    PredicateTarget::Builtin(BuiltinField::EventName),
                    CompareOp::Eq,
                    vec![text("a")],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::EventName),
                    CompareOp::Eq,
                    vec![text("b")],
                ),
            ],
        };
        two.ensure_evaluable().unwrap();
        // Malformed hex ids fail STRUCTURAL validation.
        assert!(matches!(
            (Predicate {
                conditions: vec![cond(
                    PredicateTarget::Builtin(BuiltinField::SpanId),
                    CompareOp::Eq,
                    vec![text("not-hex")],
                )]
            })
            .validate(),
            Err(PredicateError::InvalidIdValue { width: 16, .. })
        ));
        // Trace-level builtins are evaluable post-assembly.
        for op in [CompareOp::Eq, CompareOp::NotEq, CompareOp::Regex, CompareOp::NotRegex] {
            evaluable(cond(
                PredicateTarget::Builtin(BuiltinField::RootServiceName),
                op,
                vec![text("v")],
            ));
        }
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::TraceDuration),
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        ));
        evaluable(cond(
            PredicateTarget::Builtin(BuiltinField::TraceDuration),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(5)],
        ));
    }

    /// Partition (R3-2): trace-level builtins split out; everything
    /// else stays span-local, order preserved within each part.
    #[test]
    fn partition_splits_trace_level_conditions() {
        let p = Predicate {
            conditions: vec![
                cond(span_attr("x"), CompareOp::Eq, vec![text("v")]),
                cond(
                    PredicateTarget::Builtin(BuiltinField::TraceDuration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(5)],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(5)],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::RootName),
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
            PredicateTarget::Builtin(
                BuiltinField::RootName
                    | BuiltinField::RootServiceName
                    | BuiltinField::TraceDuration
            )
        )));
        // All-trace-level → span-local is the empty form (C-4: phase-1
        // candidates come from `All` over the window).
        let only_trace = Predicate {
            conditions: vec![cond(
                PredicateTarget::Builtin(BuiltinField::TraceDuration),
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
                    PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
                    CompareOp::Eq,
                    vec![text("svc")],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Status),
                    CompareOp::Eq,
                    vec![text("ERROR")],
                ),
                cond(span_attr("http.method"), CompareOp::Regex, vec![text("GET|PUT")]),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(100)],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
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
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(500)],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Lt,
                    vec![PredicateValue::Integer(100)],
                ),
            ],
        };
        assert_eq!(
            contradictory.to_trace_plan().terms,
            vec![PlanTerm::duration(Some(501), Some(99))]
        );
        // Stage-B lowerings: negation, any-owner disjunction, numerics,
        // multi-value and negated duration equality.
        let stage_b = Predicate {
            conditions: vec![
                cond(span_attr("x"), CompareOp::NotEq, vec![text("a"), text("b")]),
                cond(
                    PredicateTarget::Attribute(AttributeOwner::Any, "env".into()),
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
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Eq,
                    vec![PredicateValue::Integer(10), PredicateValue::Integer(20)],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
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
                ("attributes.team".into(), "a".into()),
                ("attributes.team".into(), "b".into()),
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
                PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into()),
                CompareOp::Eq,
                vec![text("svc")],
            )],
            None
        ));
        // Multi-valued field: ANY value satisfies `=`.
        assert!(matches(
            vec![cond(span_attr("team"), CompareOp::Eq, vec![text("b")])],
            None
        ));
        assert!(!matches(
            vec![cond(span_attr("team"), CompareOp::Eq, vec![text("c")])],
            None
        ));
        // Regex is full-value anchored ("GET" alone must not match "GET /").
        assert!(!matches(
            vec![cond(
                PredicateTarget::Builtin(BuiltinField::Name),
                CompareOp::Regex,
                vec![text("GET")],
            )],
            None
        ));
        assert!(matches(
            vec![cond(
                PredicateTarget::Builtin(BuiltinField::Name),
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
                    PredicateTarget::Builtin(BuiltinField::Status),
                    CompareOp::Eq,
                    vec![text("ERROR")],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(249)],
                ),
                cond(
                    PredicateTarget::Builtin(BuiltinField::Duration),
                    CompareOp::Lte,
                    vec![PredicateValue::Integer(250)],
                ),
            ],
            None
        ));
        assert!(!matches(
            vec![cond(
                PredicateTarget::Builtin(BuiltinField::Duration),
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
                    PredicateTarget::Builtin(BuiltinField::Duration),
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
                    PredicateTarget::Builtin(BuiltinField::Duration),
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
            vec![cond(span_attr("team"), CompareOp::NotEq, vec![text("c")])],
            None
        ));
        // A multi-valued field with ONE matching value fails `!=`.
        assert!(!matches(
            vec![cond(span_attr("team"), CompareOp::NotEq, vec![text("a")])],
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
                PredicateTarget::Builtin(BuiltinField::Name),
                CompareOp::NotRegex,
                vec![text("POST.*")],
            )],
            None
        ));
        // ── Stage B: the any-owner disjunction gathers both owners ──
        assert!(matches(
            vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Any, "service.name".into()),
                CompareOp::Eq,
                vec![text("svc")],
            )],
            None
        ));
        assert!(matches(
            vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Any, "team".into()),
                CompareOp::Eq,
                vec![text("b")],
            )],
            None
        ));
        assert!(!matches(
            vec![cond(
                PredicateTarget::Attribute(AttributeOwner::Any, "nowhere".into()),
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
        // event:timeSinceStart saturates far-future event times (a
        // wrapping cast would flip them negative and fail `> 0`).
        let mut far = span.clone();
        far.events = vec![sfst::TraceEvent {
            time_unix_nano: u64::MAX,
            name: "far".into(),
            dropped_attributes_count: 0,
            attributes: vec![],
        }];
        assert!(span_matches(
            &far,
            &Predicate {
                conditions: vec![cond(
                    PredicateTarget::Builtin(BuiltinField::EventTimeSinceStart),
                    CompareOp::Gt,
                    vec![PredicateValue::Integer(0)],
                )],
            },
            None
        ));
        // Negated duration equality.
        assert!(m2(cond(
            PredicateTarget::Builtin(BuiltinField::Duration),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(999)],
        )));
        assert!(!m2(cond(
            PredicateTarget::Builtin(BuiltinField::Duration),
            CompareOp::NotEq,
            vec![PredicateValue::Integer(250)],
        )));
    }
}
