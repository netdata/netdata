//! The neutral, owner-aware search predicate AST (design-record
//! decision 26A) — the FULL recorded form grammar as typed data over
//! the [`vocab`](super::vocab) enums, never grammar strings (decision
//! 16A: each wire adapter is a pure translator onto this type and owns
//! its own rendering).
//!
//! The whole grammar parses into this AST and validates structurally in
//! every build ([`Predicate::validate`]); only the currently evaluable
//! subset is evaluated — a structurally valid construct outside that
//! subset is rejected at the request boundary with the named
//! [`PredicateError::NotYetEvaluable`] error: a clean gap, never a
//! silently wrong answer. The AST never changes shape; widening the
//! evaluable set only adds arms to [`Condition::unevaluable_construct`],
//! which owns the exact boundary.
//!
//! # One lowering, two evaluators
//!
//! [`Predicate::to_trace_plan`] lowers the span-local conditions into
//! the neutral [`sfst::TracePlan`] (storage names constructed ONLY
//! through the vocabulary), and the span-side evaluator
//! ([`span_matches`] / `EvalPredicate`) is built from that same plan —
//! the raw index path and the canonical span path cannot disagree on
//! what a condition means, because there is exactly one lowering.

use sfst::{PlanTerm, TracePlan, TraceSpan};

use super::vocab::{AttributeKey, AttributeOwner, BuiltinField};
use super::window::TimeWindow;

/// What a condition tests. Attribute keys carry an [`AttributeOwner`]
/// ([`AttributeOwner::Builtin`] is not an attribute owner — request
/// error); builtins are the full fixed [`BuiltinField`] set. A
/// vocabulary key names its target through [`PredicateTarget::from_key`].
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PredicateTarget {
    Attribute(AttributeOwner, String),
    Builtin(BuiltinField),
    /// Free text: a case-insensitive substring of any VALUE of the span
    /// name, the status message, event names, or any attribute
    /// (resource, span, scope, event, link) — never of a field name.
    /// Takes [`CompareOp::Contains`] and one text value of at least
    /// [`TEXT_MIN_CHARS`] characters and at most [`TEXT_MAX_BYTES`].
    FreeText,
}

impl PredicateTarget {
    /// The target a parsed vocabulary key names
    /// ([`super::vocab::parse_key`]).
    pub fn from_key(owner: AttributeOwner, key: AttributeKey) -> Self {
        match key {
            AttributeKey::Builtin(b) => PredicateTarget::Builtin(b),
            AttributeKey::Attribute(a) => PredicateTarget::Attribute(owner, a),
        }
    }
}

impl std::fmt::Display for PredicateTarget {
    /// Diagnostic rendering (error messages only).
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PredicateTarget::Attribute(owner, key) => write!(f, "{owner:?} attribute {key:?}"),
            PredicateTarget::Builtin(i) => write!(f, "builtin field {i:?}"),
            PredicateTarget::FreeText => write!(f, "free text"),
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
    /// Case-insensitive substring, free text only (the rule of
    /// suggest's contains class, [`sfst::ContainsMatcher`]).
    Contains,
}

impl CompareOp {
    fn is_ordering(self) -> bool {
        matches!(
            self,
            CompareOp::Gt | CompareOp::Lt | CompareOp::Gte | CompareOp::Lte
        )
    }

    fn is_regex(self) -> bool {
        matches!(self, CompareOp::Regex | CompareOp::NotRegex)
    }
}

/// The [`CompareOp::Regex`] pattern of "starts with `text`": the escaped
/// text, then anything, line breaks included (regex conditions match the
/// whole value).
pub fn starts_with_pattern(text: &str) -> String {
    format!("{}(?s:.*)", regex::escape(text))
}

/// One comparison value. Duration comparisons take
/// [`Integer`](Self::Integer) nanoseconds only — the wire carries its
/// duration bounds as integer nanoseconds, and a text value there is a
/// request error ([`PredicateError::NonIntegerDuration`]);
/// [`Float`](Self::Float)/`Integer` against attributes are the
/// dictionary-numeric comparisons.
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

/// One search-bar token: conditions one span must satisfy together (a
/// span-local group — free text included), exactly one trace-level
/// condition, or exactly one `trace_id` condition. A query's tokens AND
/// at trace level: different tokens may match different spans.
///
/// `negated` is NOT EXISTS over the group: no span of the trace
/// satisfies it (for a trace-level or `trace_id` token, the trace does
/// not). It differs from an in-group `!=`, which keeps the condition's
/// presence ∩ complement meaning on one span.
#[derive(Debug, Clone, PartialEq)]
pub struct Token {
    pub group: Predicate,
    pub negated: bool,
}

/// What a validated [`Token`] constrains.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum TokenKind {
    SpanLocal,
    TraceLevel,
    TraceId,
}

impl Token {
    /// The token's kind. Meaningful after [`TokenQuery::validate`]: a
    /// trace-level or `trace_id` condition is then its token's only one.
    pub(crate) fn kind(&self) -> TokenKind {
        use BuiltinField::*;
        match self.group.conditions.first().map(|c| &c.target) {
            Some(PredicateTarget::Builtin(RootName | RootServiceName | TraceDuration)) => {
                TokenKind::TraceLevel
            }
            Some(PredicateTarget::Builtin(TraceId)) => TokenKind::TraceId,
            _ => TokenKind::SpanLocal,
        }
    }

    /// A token matched by any trace one of whose spans satisfies `group`.
    pub fn positive(group: Predicate) -> Self {
        Self {
            group,
            negated: false,
        }
    }

    /// A token matched by any trace none of whose spans satisfies
    /// `group`.
    pub fn negated(group: Predicate) -> Self {
        Self {
            group,
            negated: true,
        }
    }
}

/// A search-bar query: the conjunction of its tokens (none = match
/// everything). Holds at most [`TOKENS_MAX`] tokens.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct TokenQuery {
    pub tokens: Vec<Token>,
}

impl TokenQuery {
    /// Request validation, runnable before any source opens (a wire can
    /// reject a bad request before admission): the token count, every
    /// token non-empty with a trace-level or `trace_id` condition alone
    /// in its token, and every condition structurally valid and
    /// evaluable.
    pub fn validate(&self) -> Result<(), PredicateError> {
        if self.tokens.len() > TOKENS_MAX {
            return Err(PredicateError::TooManyTokens {
                max: TOKENS_MAX,
                got: self.tokens.len(),
            });
        }
        for token in &self.tokens {
            let conditions = &token.group.conditions;
            if conditions.is_empty() {
                return Err(PredicateError::EmptyToken);
            }
            token.group.validate()?;
            token.group.ensure_evaluable()?;
            if conditions.len() > 1
                && let Some(target) = token.group.trace_level_target()
            {
                return Err(PredicateError::TokenNotAlone {
                    target: target.to_string(),
                });
            }
        }
        Ok(())
    }
}

impl From<Predicate> for TokenQuery {
    /// A flat predicate as tokens with its meaning kept: its span-local
    /// conditions as ONE positive group (same span), and each
    /// trace-level and `trace_id` condition as its own positive token,
    /// in-group polarity unchanged.
    fn from(predicate: Predicate) -> Self {
        let (rest, alone): (Vec<Condition>, Vec<Condition>) =
            predicate.conditions.into_iter().partition(|c| {
                let single = Predicate {
                    conditions: vec![c.clone()],
                };
                single.trace_level_target().is_none()
            });
        let mut tokens = Vec::with_capacity(alone.len() + 1);
        if !rest.is_empty() {
            tokens.push(Token::positive(Predicate { conditions: rest }));
        }
        for condition in alone {
            tokens.push(Token::positive(Predicate {
                conditions: vec![condition],
            }));
        }
        Self { tokens }
    }
}

/// The most tokens one [`TokenQuery`] carries (the engine keeps one
/// match bit per token in a `u32`).
pub const TOKENS_MAX: usize = 32;
/// The shortest free-text term, in characters (Unicode scalar values):
/// shorter terms match too much to be useful.
pub const TEXT_MIN_CHARS: usize = 3;
/// The longest free-text term, in bytes.
pub const TEXT_MAX_BYTES: usize = 1024;

/// A predicate request error — the caller built an invalid predicate (or
/// one this build cannot evaluate yet); nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum PredicateError {
    #[error("a condition on {target} carries no values")]
    NoValues { target: String },
    #[error("{op:?} takes exactly one value, got {got}")]
    SingleValueArity { op: CompareOp, got: usize },
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
    #[error("free text needs at least {min} characters, got {got}")]
    TextTooShort { min: usize, got: usize },
    #[error("free text is {got} bytes, over the limit of {max}")]
    TextTooLong { max: usize, got: usize },
    #[error("{got} tokens exceed the limit of {max}")]
    TooManyTokens { max: usize, got: usize },
    #[error("a token carries no conditions")]
    EmptyToken,
    #[error("{target} must be the only condition of its token")]
    TokenNotAlone { target: String },
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
        Name
        | Kind
        | Status
        | StatusMessage
        | InstrumentationName
        | InstrumentationVersion
        | EventName
        | RootName
        | RootServiceName => BuiltinClass::Text,
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
        if (self.op.is_ordering() || self.op == CompareOp::Contains) && self.values.len() != 1 {
            return Err(PredicateError::SingleValueArity {
                op: self.op,
                got: self.values.len(),
            });
        }
        let all_text = self
            .values
            .iter()
            .all(|v| matches!(v, PredicateValue::Text(_)));
        let all_integer = self
            .values
            .iter()
            .all(|v| matches!(v, PredicateValue::Integer(_)));
        let all_numeric = self
            .values
            .iter()
            .all(|v| matches!(v, PredicateValue::Integer(_) | PredicateValue::Float(_)));
        // One condition compares against ONE value class: mixing text
        // and numbers has no coherent multi-value semantics (and the
        // form grammar never generates it).
        if !all_text && !all_numeric {
            return Err(PredicateError::MixedValueTypes { target });
        }
        // A NaN matches nothing — reject it up front.
        if self
            .values
            .iter()
            .any(|v| matches!(v, PredicateValue::Float(f) if f.is_nan()))
        {
            return Err(PredicateError::NanValue { target });
        }

        // Contains is the free-text operator, and free text takes nothing
        // else.
        if (self.op == CompareOp::Contains) != (self.target == PredicateTarget::FreeText) {
            return Err(PredicateError::InvalidOpForTarget {
                op: self.op,
                target,
            });
        }
        match &self.target {
            PredicateTarget::FreeText => {
                let [PredicateValue::Text(term)] = self.values.as_slice() else {
                    return Err(PredicateError::TextValueRequired {
                        op: self.op,
                        target,
                    });
                };
                let chars = term.chars().count();
                if chars < TEXT_MIN_CHARS {
                    return Err(PredicateError::TextTooShort {
                        min: TEXT_MIN_CHARS,
                        got: chars,
                    });
                }
                if term.len() > TEXT_MAX_BYTES {
                    return Err(PredicateError::TextTooLong {
                        max: TEXT_MAX_BYTES,
                        got: term.len(),
                    });
                }
                // Built once here so the evaluators' builds cannot fail.
                if sfst::ContainsMatcher::new(term).is_none() {
                    return Err(PredicateError::InvalidPattern {
                        pattern: term.clone(),
                        msg: "the free-text matcher does not compile".to_string(),
                    });
                }
            }
            PredicateTarget::Attribute(AttributeOwner::Builtin, key) => {
                return Err(PredicateError::AttributeUnderBuiltinOwner { key: key.clone() });
            }
            // Attributes take every op; regex needs text patterns,
            // ordering needs numbers (the dictionary-numeric path).
            PredicateTarget::Attribute(..) => {
                if self.op.is_regex() && !all_text {
                    return Err(PredicateError::TextValueRequired {
                        op: self.op,
                        target,
                    });
                }
                if self.op.is_ordering() && !all_numeric {
                    return Err(PredicateError::NumericValueRequired {
                        op: self.op,
                        target,
                    });
                }
            }
            PredicateTarget::Builtin(i) => match builtin_class(*i) {
                BuiltinClass::Text => {
                    if self.op.is_ordering() {
                        return Err(PredicateError::InvalidOpForTarget {
                            op: self.op,
                            target,
                        });
                    }
                    if !all_text {
                        return Err(PredicateError::TextValueRequired {
                            op: self.op,
                            target,
                        });
                    }
                }
                BuiltinClass::Id => {
                    if !matches!(self.op, CompareOp::Eq | CompareOp::NotEq) {
                        return Err(PredicateError::InvalidOpForTarget {
                            op: self.op,
                            target,
                        });
                    }
                    if !all_text {
                        return Err(PredicateError::TextValueRequired {
                            op: self.op,
                            target,
                        });
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
                        if hex.len() != width || !hex.bytes().all(|b| b.is_ascii_hexdigit()) {
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
                        return Err(PredicateError::InvalidOpForTarget {
                            op: self.op,
                            target,
                        });
                    }
                    if !all_integer {
                        return Err(PredicateError::NonIntegerDuration);
                    }
                }
            },
        }

        // Patterns must compile in the engine's anchored form: a bad
        // pattern is a request error, never a mid-query failure.
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
    /// not-yet-evaluable construct. Evaluable: everything span-local —
    /// attributes (every owner, every op), the dictionary
    /// builtins, duration bounds/equality, and the span/parent/trace
    /// ids (both polarities) — plus the POSITIVE event/link subgroup
    /// forms and the trace-level builtins. The gaps are the NEGATED
    /// subgroup forms — link ids, `events.name`, event/link attributes,
    /// `events.time_since_start` — which sit on the recorded open
    /// subgroup-semantics question. Assumes structural validity
    /// ([`Condition::validate`] runs first).
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
            // Attribute owners: every op — tokens for text, the
            // dictionary-numeric path for numbers, presence ∩ complement
            // for negation.
            (
                PredicateTarget::Attribute(
                    AttributeOwner::Resource
                    | AttributeOwner::Span
                    | AttributeOwner::Instrumentation,
                    _,
                ),
                _,
            ) => None,
            // Dictionary-backed builtins except events.name (refine).
            (
                PredicateTarget::Builtin(
                    Name
                    | Kind
                    | Status
                    | StatusMessage
                    | InstrumentationName
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
            (PredicateTarget::Builtin(LinkSpanId | LinkTraceId), CompareOp::Eq) => None,
            (PredicateTarget::Builtin(LinkSpanId | LinkTraceId), CompareOp::NotEq) => Some(
                format!("negated {} (subgroup semantics, refine step)", self.target),
            ),
            // events.name POSITIVE forms evaluate through the event
            // subgroup (any number of conditions — a single event must
            // satisfy them all); negated forms wait for the
            // subgroup-semantics decision.
            (PredicateTarget::Builtin(EventName), CompareOp::Eq | CompareOp::Regex) => None,
            (PredicateTarget::Builtin(EventName), CompareOp::NotEq | CompareOp::NotRegex) => Some(
                format!("negated {} (subgroup semantics, refine step)", self.target),
            ),
            // Event/link-scoped attributes: POSITIVE forms evaluate
            // through the subgroup refine; negated forms sit on the
            // recorded open question (flat negation vs single-item
            // subgroup semantics).
            (
                PredicateTarget::Attribute(AttributeOwner::Event | AttributeOwner::Link, _),
                CompareOp::Eq
                | CompareOp::Regex
                | CompareOp::Gt
                | CompareOp::Lt
                | CompareOp::Gte
                | CompareOp::Lte,
            ) => None,
            (PredicateTarget::Attribute(AttributeOwner::Event | AttributeOwner::Link, _), _) => {
                Some(format!(
                    "negated {} (subgroup semantics, refine step)",
                    self.target
                ))
            }
            // events.time_since_start computes in the refine; != would be a
            // negated per-event condition — deferred with the fork for a
            // uniform rule.
            (PredicateTarget::Builtin(EventTimeSinceStart), op)
                if op.is_ordering() || op == CompareOp::Eq =>
            {
                None
            }
            (PredicateTarget::Builtin(EventTimeSinceStart), _) => Some(format!(
                "negated {} (subgroup semantics, refine step)",
                self.target
            )),
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
            // Free text resolves per file over the text scope.
            (PredicateTarget::FreeText, CompareOp::Contains) => None,
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

    /// The first condition that is not span-local — a trace-level
    /// builtin (`root_name`, `root_service_name`, `trace_duration`) or a
    /// `trace_id` constraint — or `None` when every condition is
    /// span-local. Such a condition must be its token's only one; the
    /// list of fields is owned HERE so a wire cannot drift from the
    /// engine.
    pub fn trace_level_target(&self) -> Option<&PredicateTarget> {
        use BuiltinField::*;
        self.conditions.iter().map(|c| &c.target).find(|t| {
            matches!(
                t,
                PredicateTarget::Builtin(RootName | RootServiceName | TraceDuration | TraceId)
            )
        })
    }

    /// Strip `trace:id` conditions out of the predicate — they are
    /// neither span-local (a `TraceSpan` carries no trace id) nor
    /// trace-level builtins; they constrain the ENGINE's candidate
    /// set: `=` values intersect into a PIN set (phase-1 discovery is
    /// skipped — pins go straight to assembly), `!=` values union into
    /// an exclusion filter at pool admission ([`LoweredTokens`] folds
    /// each `trace_id` token's constraints across the query). Hex
    /// parsing is infallible post-validation.
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
                (PredicateTarget::Builtin(BuiltinField::LinkSpanId), CompareOp::Eq) => {
                    link_group.push(sfst::GroupCondition::LinkSpanIds(hex_span_ids(
                        &condition.values,
                    )));
                }
                (PredicateTarget::Builtin(BuiltinField::LinkTraceId), CompareOp::Eq) => {
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
                (
                    PredicateTarget::Attribute(
                        scope @ (AttributeOwner::Event | AttributeOwner::Link),
                        key,
                    ),
                    op,
                ) => {
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
                (PredicateTarget::FreeText, _) => {
                    let PredicateValue::Text(term) = &condition.values[0] else {
                        unreachable!("validated: free text takes one text value");
                    };
                    let scope = super::vocab::text_scope();
                    terms.push(PlanTerm::FieldsIn {
                        exact: scope.exact,
                        prefixes: scope.prefixes,
                        matcher: sfst::PlanMatcher::Contains(term.clone()),
                    });
                }
                (target, op) => {
                    let fields: Vec<String> = match target {
                        PredicateTarget::Attribute(owner, key) => {
                            let prefix = owner
                                .attribute_prefix()
                                .expect("validated: an attribute owner (not Builtin)");
                            vec![format!("{prefix}{key}")]
                        }
                        PredicateTarget::Builtin(i) => vec![
                            i.dictionary_field()
                                .expect("evaluable builtins are dictionary-backed")
                                .to_string(),
                        ],
                        PredicateTarget::FreeText => unreachable!("lowered above"),
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
        // Integers stay integers: the comparator compares them exactly.
        let values: Vec<sfst::NumberValue> = values
            .iter()
            .map(|v| match v {
                PredicateValue::Integer(n) => sfst::NumberValue::Int(*n),
                PredicateValue::Float(f) => sfst::NumberValue::Float(*f),
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
            CompareOp::Contains => unreachable!("validated: contains is free text only"),
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
#[derive(Default)]
pub(crate) struct TraceIdConstraints {
    /// `Some` = the candidate set is EXACTLY these ids (already the
    /// intersection of every `trace:id =` condition; may be empty).
    pub(crate) pins: Option<std::collections::BTreeSet<sfst::TraceId>>,
    /// Ids excluded by `trace:id !=` conditions.
    pub(crate) excluded: std::collections::BTreeSet<sfst::TraceId>,
}

impl TraceIdConstraints {
    /// Fold one validated `trace_id` token in: pins intersect,
    /// exclusions union. The id is single-valued per trace, so a negated
    /// token swaps the two: NOT (id ∈ S) is id ∉ S and NOT (id ∉ S) is
    /// id ∈ S.
    pub(crate) fn add_token(&mut self, token: &Token) {
        let (_, mut c) = token.group.split_trace_id_conditions();
        if token.negated {
            let pins = (!c.excluded.is_empty()).then(|| std::mem::take(&mut c.excluded));
            c.excluded = c.pins.take().unwrap_or_default();
            c.pins = pins;
        }
        if let Some(pins) = c.pins {
            self.pins = Some(match self.pins.take() {
                None => pins,
                Some(prev) => prev.intersection(&pins).copied().collect(),
            });
        }
        self.excluded.extend(c.excluded);
    }

    /// Whether a trace id passes the constraints.
    pub(crate) fn admits(&self, trace_id: sfst::TraceId) -> bool {
        self.pins
            .as_ref()
            .is_none_or(|pins| pins.contains(&trace_id))
            && !self.excluded.contains(&trace_id)
    }
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
        CompareOp::NotEq | CompareOp::Regex | CompareOp::NotRegex | CompareOp::Contains => {
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
        values: Vec<sfst::NumberValue>,
    },
    /// `None`: the empty term, which matches nothing (the plan's rule).
    Contains(Option<sfst::ContainsMatcher>),
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
            EvalMatcher::Contains(matcher) => matcher
                .as_ref()
                .is_some_and(|m| m.is_match(value.as_bytes())),
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
    FieldsIn {
        exact: Vec<String>,
        prefixes: Vec<String>,
        matcher: EvalMatcher,
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
    Attr {
        bare_key: String,
        matcher: EvalMatcher,
    },
    TimeSince(Vec<(Option<i64>, Option<i64>)>),
    LinkSpanIds(Vec<sfst::SpanId>),
    LinkTraceIds(Vec<sfst::TraceId>),
}

/// The span-side predicate evaluator — the single source of truth for
/// what a span-local predicate MEANS (pin R2-9): the tail's phase 1
/// evaluates it per decoded span, phase 2 re-evaluates it per retained
/// canonical span, and a trace's rank (its newest matching span) is
/// defined by it.
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
                PlanTerm::FieldsIn {
                    exact,
                    prefixes,
                    matcher,
                } => EvalTerm::FieldsIn {
                    exact: exact.clone(),
                    prefixes: prefixes.clone(),
                    matcher: eval_matcher(matcher),
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
                if *negated {
                    present && !any_match
                } else {
                    any_match
                }
            }
            EvalTerm::FieldsIn {
                exact,
                prefixes,
                matcher,
            } => {
                // Storage names: row facets as stored; event names and
                // event/link attributes under their storage spellings
                // (spans carry them structured and prefix-stripped when
                // the file has the structure, as row facets otherwise).
                let in_scope = |head: &str, tail: &str| scope_has(exact, prefixes, head, tail);
                let event_name = BuiltinField::EventName
                    .dictionary_field()
                    .expect("dictionary-backed");
                let event_attr = AttributeOwner::Event
                    .attribute_prefix()
                    .expect("attribute owner");
                let link_attr = AttributeOwner::Link
                    .attribute_prefix()
                    .expect("attribute owner");
                span.fields
                    .iter()
                    .any(|(k, v)| in_scope(k, "") && matcher.value_matches(v))
                    || span.events.iter().any(|e| {
                        (in_scope(event_name, "") && matcher.value_matches(&e.name))
                            || e.attributes
                                .iter()
                                .any(|(k, v)| in_scope(event_attr, k) && matcher.value_matches(v))
                    })
                    || span.links.iter().any(|l| {
                        l.attributes
                            .iter()
                            .any(|(k, v)| in_scope(link_attr, k) && matcher.value_matches(v))
                    })
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
                // root span never satisfies `parent_span_id != x`.
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

/// Whether the storage name `head` + `tail` is in a free-text scope —
/// equal to an `exact` name or under one of `prefixes` — without
/// building the name.
fn scope_has(exact: &[String], prefixes: &[String], head: &str, tail: &str) -> bool {
    let len = head.len() + tail.len();
    let equals =
        |name: &str| name.len() == len && name.starts_with(head) && &name[head.len()..] == tail;
    let under = |prefix: &str| {
        if prefix.len() <= head.len() {
            head.starts_with(prefix)
        } else {
            prefix.starts_with(head) && tail.starts_with(&prefix[head.len()..])
        }
    };
    exact.iter().any(|e| equals(e)) || prefixes.iter().any(|p| under(p))
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
        sfst::PlanMatcher::Contains(term) => EvalMatcher::Contains((!term.is_empty()).then(|| {
            sfst::ContainsMatcher::new(term).expect("text terms validated at the request boundary")
        })),
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

/// One span-local token lowered once: the per-file plan and its
/// span-side twin (one lowering, module docs).
pub(crate) struct SpanToken {
    pub(crate) plan: TracePlan,
    pub(crate) eval: EvalPredicate,
}

impl SpanToken {
    pub(crate) fn new(group: &Predicate) -> Self {
        let plan = group.to_trace_plan();
        let eval = EvalPredicate::new(&plan);
        Self { plan, eval }
    }
}

/// A validated [`TokenQuery`] lowered for the engines: span-local tokens
/// as [`SpanToken`]s by polarity, trace-level tokens as evaluators, and
/// `trace_id` tokens folded into candidate constraints.
pub(crate) struct LoweredTokens {
    /// Positive span-local tokens, in token order.
    pub(crate) positive: Vec<SpanToken>,
    /// Negated span-local tokens' positive forms.
    pub(crate) negated: Vec<SpanToken>,
    /// The positive trace-level tokens' conditions, AND-combined.
    pub(crate) trace_level: Option<TraceLevelEval>,
    /// One evaluator per negated trace-level token.
    trace_level_negated: Vec<TraceLevelEval>,
    pub(crate) trace_ids: TraceIdConstraints,
}

impl LoweredTokens {
    pub(crate) fn new(query: &TokenQuery) -> Self {
        let mut positive = Vec::new();
        let mut negated = Vec::new();
        let mut trace_level = Vec::new();
        let mut trace_level_negated = Vec::new();
        let mut trace_ids = TraceIdConstraints::default();
        for token in &query.tokens {
            match token.kind() {
                TokenKind::SpanLocal if token.negated => negated.push(SpanToken::new(&token.group)),
                TokenKind::SpanLocal => positive.push(SpanToken::new(&token.group)),
                TokenKind::TraceLevel if token.negated => {
                    trace_level_negated.push(TraceLevelEval::new(&token.group));
                }
                TokenKind::TraceLevel => trace_level.extend(token.group.conditions.iter().cloned()),
                TokenKind::TraceId => trace_ids.add_token(token),
            }
        }
        Self {
            positive,
            negated,
            trace_level: (!trace_level.is_empty()).then(|| {
                TraceLevelEval::new(&Predicate {
                    conditions: trace_level,
                })
            }),
            trace_level_negated,
            trace_ids,
        }
    }

    /// Whether any trace-level token (either polarity) applies.
    pub(crate) fn has_trace_level(&self) -> bool {
        self.trace_level.is_some() || !self.trace_level_negated.is_empty()
    }

    /// Whether a verdict needs every span of the trace: a trace-level
    /// token reads its root and envelope, a negated span-local token
    /// holds only if NO span matches. An inexact assembly is then
    /// indeterminate and excluded (decision 15). Positive span-local
    /// tokens need no such care: a match found is a match.
    pub(crate) fn needs_whole_trace(&self) -> bool {
        self.has_trace_level() || !self.negated.is_empty()
    }

    /// Whether a trace-level token reads the root: an unknown root
    /// (a withheld claim) is then indeterminate, either polarity.
    pub(crate) fn reads_root(&self) -> bool {
        self.trace_level
            .as_ref()
            .is_some_and(TraceLevelEval::reads_root)
            || self
                .trace_level_negated
                .iter()
                .any(TraceLevelEval::reads_root)
    }

    /// The trace-level verdict on an exact assembled trace's TRUE-root
    /// values and envelope duration: every positive trace-level token
    /// holds and no negated one does (a rootless trace satisfies no
    /// root condition, so it passes a negated one).
    pub(crate) fn trace_level_matches(
        &self,
        root_name: Option<&str>,
        root_service: Option<&str>,
        trace_duration_ns: i64,
    ) -> bool {
        self.trace_level
            .as_ref()
            .is_none_or(|tl| tl.matches(root_name, root_service, trace_duration_ns))
            && !self
                .trace_level_negated
                .iter()
                .any(|tl| tl.matches(root_name, root_service, trace_duration_ns))
    }

    /// The rank of an assembled trace (spans in ascending start order)
    /// under the span-local tokens, over spans starting in `window`:
    /// the OLDEST of each positive token's newest matching span start
    /// (the trace is as recent as its least recent required match), or
    /// the newest in-window span start without positive tokens. `None`
    /// when a positive token matches no span, a negated token matches
    /// one, or no span starts in the window.
    pub(crate) fn span_rank(&self, spans: &[TraceSpan], window: Option<TimeWindow>) -> Option<i64> {
        if self
            .negated
            .iter()
            .any(|t| spans.iter().any(|s| t.eval.matches(s, window)))
        {
            return None;
        }
        if self.positive.is_empty() {
            let newest = spans
                .iter()
                .rposition(|s| window.is_none_or(|w| w.contains(s.start_ns)))?;
            return Some(spans[newest].start_ns);
        }
        let mut rank = i64::MAX;
        for token in &self.positive {
            let newest = spans.iter().rposition(|s| token.eval.matches(s, window))?;
            rank = rank.min(spans[newest].start_ns);
        }
        Some(rank)
    }
}

/// The post-assembly evaluator of trace-level conditions (decision 15
/// / pin R3-2): boolean over the assembled trace's root
/// name, root service, and envelope duration. The ENGINE owns the
/// tri-state — an assembled trace with unreliable (capped or degraded)
/// values is excluded as indeterminate before this evaluator runs. The
/// root inputs are the TRUE root's values only (decision 1D — see
/// [`search`](super::search)'s module docs): a rootless trace passes
/// `None`s, never the display-side promoted root. Absent values never
/// satisfy a condition, negated forms included; values are single-valued
/// per trace, so no subgroup fork exists.
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
    RootName {
        matcher: EvalMatcher,
        negated: bool,
    },
    RootService {
        matcher: EvalMatcher,
        negated: bool,
    },
    Duration {
        intervals: Vec<(Option<i64>, Option<i64>)>,
        negated: bool,
    },
}

impl TraceLevelEval {
    /// Whether a condition reads the root (name or service).
    pub(crate) fn reads_root(&self) -> bool {
        self.conditions
            .iter()
            .any(|c| !matches!(c, TraceLevelCondition::Duration { .. }))
    }

    /// Compile the trace-level predicate half (validated, evaluable).
    pub(crate) fn new(trace_level: &Predicate) -> Self {
        let conditions = trace_level
            .conditions
            .iter()
            .map(|condition| {
                let negated = matches!(condition.op, CompareOp::NotEq | CompareOp::NotRegex);
                match &condition.target {
                    PredicateTarget::Builtin(BuiltinField::RootName) => {
                        TraceLevelCondition::RootName {
                            matcher: eval_matcher(&condition_matcher(
                                condition.op,
                                &condition.values,
                            )),
                            negated,
                        }
                    }
                    PredicateTarget::Builtin(BuiltinField::RootServiceName) => {
                        TraceLevelCondition::RootService {
                            matcher: eval_matcher(&condition_matcher(
                                condition.op,
                                &condition.values,
                            )),
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
            TraceLevelCondition::RootName {
                matcher,
                negated: false,
            } => Some((GateRootField::Name, matcher)),
            TraceLevelCondition::RootService {
                matcher,
                negated: false,
            } => Some((GateRootField::Service, matcher)),
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
            let TraceLevelCondition::Duration {
                intervals,
                negated: false,
            } = c
            else {
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
/// seam (R2-9); engine loops use the pre-compiled `EvalPredicate` this
/// delegates to, so the two can never diverge.
///
/// `predicate` must be validated and span-local (partitioned); a
/// trace-level or stage-B condition here is an engine bug and panics
/// loudly rather than under-approximating silently.
pub fn span_matches(span: &TraceSpan, predicate: &Predicate, window: Option<TimeWindow>) -> bool {
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
        let ok = |c: Condition| {
            Predicate {
                conditions: vec![c],
            }
            .validate()
            .unwrap()
        };
        let err = |c: Condition| {
            Predicate {
                conditions: vec![c],
            }
            .validate()
            .unwrap_err()
        };

        ok(cond(span_attr("x"), CompareOp::Eq, vec![text("v")]));
        ok(cond(
            span_attr("x"),
            CompareOp::NotEq,
            vec![text("a"), text("b")],
        ));
        ok(cond(
            span_attr("x"),
            CompareOp::Gt,
            vec![PredicateValue::Float(1.5)],
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
            PredicateError::SingleValueArity { got: 2, .. }
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
            err(cond(span_attr("x"), CompareOp::Gte, vec![text("nan")],)),
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

    /// The evaluability boundary: the evaluable set passes; every
    /// not-yet-evaluable construct is named, not silently
    /// mis-evaluated.
    #[test]
    fn evaluability_boundary() {
        let evaluable = |c: Condition| {
            Predicate {
                conditions: vec![c],
            }
            .ensure_evaluable()
            .unwrap();
        };
        let rejected = |c: Condition| -> String {
            match (Predicate {
                conditions: vec![c],
            })
            .ensure_evaluable()
            {
                Err(PredicateError::NotYetEvaluable { construct }) => construct,
                other => panic!("expected NotYetEvaluable, got {other:?}"),
            }
        };

        // Attribute owners and the any-owner disjunction: every op.
        evaluable(cond(
            span_attr("x"),
            CompareOp::Eq,
            vec![text("a"), text("b")],
        ));
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
        // Dictionary numerics on attributes.
        evaluable(cond(
            span_attr("x"),
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        ));
        evaluable(cond(
            span_attr("x"),
            CompareOp::Eq,
            vec![PredicateValue::Integer(5), PredicateValue::Float(1.5)],
        ));
        // Dictionary-backed builtins except events.name: all four ops.
        for i in [
            BuiltinField::Name,
            BuiltinField::Kind,
            BuiltinField::Status,
            BuiltinField::StatusMessage,
            BuiltinField::InstrumentationName,
            BuiltinField::InstrumentationVersion,
        ] {
            for op in [
                CompareOp::Eq,
                CompareOp::Regex,
                CompareOp::NotEq,
                CompareOp::NotRegex,
            ] {
                evaluable(cond(PredicateTarget::Builtin(i), op, vec![text("v")]));
            }
        }
        // Duration: bounds, multi-value equality, negated equality.
        for op in [
            CompareOp::Gt,
            CompareOp::Lt,
            CompareOp::Gte,
            CompareOp::Lte,
            CompareOp::Eq,
        ] {
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
        assert!(
            rejected(cond(
                PredicateTarget::Attribute(AttributeOwner::Event, "msg".into()),
                CompareOp::NotEq,
                vec![text("v")],
            ))
            .contains("subgroup")
        );
        assert!(
            rejected(cond(
                PredicateTarget::Attribute(AttributeOwner::Link, "rel".into()),
                CompareOp::NotEq,
                vec![text("v")],
            ))
            .contains("subgroup")
        );
        // The colon set: span/parent/trace ids both polarities; link
        // ids and events.name positive-only (the negated subgroup
        // semantics are the recorded open question).
        for i in [BuiltinField::SpanId, BuiltinField::ParentSpanId] {
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
        assert!(
            rejected(cond(
                PredicateTarget::Builtin(BuiltinField::LinkSpanId),
                CompareOp::NotEq,
                vec![text("00f067aa0ba902b7")],
            ))
            .contains("subgroup")
        );
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
        assert!(
            rejected(cond(
                PredicateTarget::Builtin(BuiltinField::EventName),
                CompareOp::NotEq,
                vec![text("v")],
            ))
            .contains("subgroup")
        );
        // TWO events.name conditions form a single-event subgroup —
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
        for op in [
            CompareOp::Eq,
            CompareOp::NotEq,
            CompareOp::Regex,
            CompareOp::NotRegex,
        ] {
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
                cond(
                    span_attr("http.method"),
                    CompareOp::Regex,
                    vec![text("GET|PUT")],
                ),
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
                PlanTerm::tokens("status.code", vec!["ERROR".into()], vec![]),
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
        // Stage-B lowerings: negation, a resource attribute, numerics,
        // multi-value and negated duration equality.
        let stage_b = Predicate {
            conditions: vec![
                cond(span_attr("x"), CompareOp::NotEq, vec![text("a"), text("b")]),
                cond(
                    PredicateTarget::Attribute(AttributeOwner::Resource, "env".into()),
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
                    fields: vec!["resource.attributes.env".into()],
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
                        values: vec![sfst::NumberValue::Int(500)],
                    },
                    negated: false,
                },
                PlanTerm::Fields {
                    fields: vec!["attributes.ratio".into()],
                    matcher: sfst::PlanMatcher::Number {
                        cmp: sfst::NumberCmp::Eq,
                        values: vec![sfst::NumberValue::Float(0.5), sfst::NumberValue::Int(2)],
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
                ("status.code".into(), "ERROR".into()),
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
            vec![cond(
                span_attr("absent"),
                CompareOp::NotRegex,
                vec![text("v.*")]
            )],
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
        // ── Stage B: numerics via the shared comparator ─────────────
        let span2 = TraceSpan {
            fields: vec![
                ("attributes.code".into(), "500".into()),
                ("attributes.label".into(), "not-a-number".into()),
            ],
            ..span.clone()
        };
        let m2 = |c: Condition| {
            span_matches(
                &span2,
                &Predicate {
                    conditions: vec![c],
                },
                None,
            )
        };
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
        // events.time_since_start saturates far-future event times (a
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

    fn free_text(term: &str) -> Condition {
        cond(
            PredicateTarget::FreeText,
            CompareOp::Contains,
            vec![text(term)],
        )
    }

    fn group(conditions: Vec<Condition>) -> Predicate {
        Predicate { conditions }
    }

    /// Token validation: the token cap, empty tokens, trace-level and
    /// `trace_id` conditions alone in their token, the free-text rules
    /// (operator, one text value, length in characters and bytes), and
    /// every condition checked as in a predicate.
    #[test]
    fn token_validation_matrix() {
        let root = cond(
            PredicateTarget::Builtin(BuiltinField::RootServiceName),
            CompareOp::Eq,
            vec![text("svc")],
        );
        let trace_id = cond(
            PredicateTarget::Builtin(BuiltinField::TraceId),
            CompareOp::Eq,
            vec![text(&"ab".repeat(16))],
        );
        let duration = cond(
            PredicateTarget::Builtin(BuiltinField::TraceDuration),
            CompareOp::Gte,
            vec![PredicateValue::Integer(5)],
        );
        let attr = cond(span_attr("x"), CompareOp::Eq, vec![text("v")]);
        let check = |tokens: Vec<Token>| TokenQuery { tokens }.validate();
        let alone = |c: &Condition| Token::positive(group(vec![c.clone()]));

        // Valid: no tokens; free text in a same-span group; negated
        // tokens of every kind; three characters of any width; the byte
        // limit; the token limit.
        check(vec![]).unwrap();
        check(vec![Token::positive(group(vec![
            attr.clone(),
            free_text("abc"),
        ]))])
        .unwrap();
        check(vec![
            Token::negated(group(vec![attr.clone()])),
            Token::negated(group(vec![root.clone()])),
            Token::negated(group(vec![trace_id.clone()])),
            alone(&duration),
        ])
        .unwrap();
        check(vec![alone(&free_text("日本語"))]).unwrap();
        check(vec![alone(&free_text(&"a".repeat(TEXT_MAX_BYTES)))]).unwrap();
        check((0..TOKENS_MAX).map(|_| alone(&attr)).collect()).unwrap();

        let err = |tokens: Vec<Token>| check(tokens).unwrap_err();
        assert!(matches!(
            err((0..=TOKENS_MAX).map(|_| alone(&attr)).collect()),
            PredicateError::TooManyTokens { max: 32, got: 33 }
        ));
        assert!(matches!(
            err(vec![Token::negated(Predicate::all())]),
            PredicateError::EmptyToken
        ));
        for single in [&root, &trace_id, &duration] {
            assert!(matches!(
                err(vec![Token::positive(group(vec![
                    attr.clone(),
                    single.clone()
                ]))]),
                PredicateError::TokenNotAlone { .. }
            ));
        }
        assert!(matches!(
            err(vec![alone(&free_text("éé"))]),
            PredicateError::TextTooShort { min: 3, got: 2 }
        ));
        assert!(matches!(
            err(vec![alone(&free_text(&"a".repeat(TEXT_MAX_BYTES + 1)))]),
            PredicateError::TextTooLong {
                max: 1024,
                got: 1025
            }
        ));
        assert!(matches!(
            err(vec![alone(&cond(
                PredicateTarget::FreeText,
                CompareOp::Eq,
                vec![text("abc")]
            ))]),
            PredicateError::InvalidOpForTarget { .. }
        ));
        assert!(matches!(
            err(vec![alone(&cond(
                span_attr("x"),
                CompareOp::Contains,
                vec![text("abc")]
            ))]),
            PredicateError::InvalidOpForTarget { .. }
        ));
        assert!(matches!(
            err(vec![alone(&cond(
                PredicateTarget::FreeText,
                CompareOp::Contains,
                vec![text("abc"), text("def")]
            ))]),
            PredicateError::SingleValueArity { got: 2, .. }
        ));
        assert!(matches!(
            err(vec![alone(&cond(
                PredicateTarget::FreeText,
                CompareOp::Contains,
                vec![PredicateValue::Integer(123)]
            ))]),
            PredicateError::TextValueRequired { .. }
        ));
        assert!(matches!(
            err(vec![alone(&cond(
                span_attr("x"),
                CompareOp::Regex,
                vec![text("(")]
            ))]),
            PredicateError::InvalidPattern { .. }
        ));
        assert!(matches!(
            err(vec![alone(&cond(
                PredicateTarget::Builtin(BuiltinField::EventName),
                CompareOp::NotEq,
                vec![text("e")]
            ))]),
            PredicateError::NotYetEvaluable { .. }
        ));
    }

    /// A flat predicate keeps its meaning as tokens: its span-local
    /// conditions (free text included) as one positive group, each
    /// trace-level and `trace_id` condition as its own positive token.
    #[test]
    fn a_predicate_becomes_tokens_with_its_meaning() {
        let root = cond(
            PredicateTarget::Builtin(BuiltinField::RootName),
            CompareOp::NotEq,
            vec![text("r")],
        );
        let trace_id = cond(
            PredicateTarget::Builtin(BuiltinField::TraceId),
            CompareOp::Eq,
            vec![text(&"ab".repeat(16))],
        );
        let attr = cond(span_attr("x"), CompareOp::NotEq, vec![text("v")]);
        let words = free_text("abc");
        let tokens = TokenQuery::from(group(vec![
            attr.clone(),
            root.clone(),
            words.clone(),
            trace_id.clone(),
        ]));
        assert_eq!(
            tokens.tokens,
            vec![
                Token::positive(group(vec![attr, words])),
                Token::positive(group(vec![root])),
                Token::positive(group(vec![trace_id])),
            ]
        );
        assert!(TokenQuery::from(Predicate::all()).tokens.is_empty());
        // Trace-level only: no span-local token, so discovery runs on the
        // empty plan (C-4).
        let duration = cond(
            PredicateTarget::Builtin(BuiltinField::TraceDuration),
            CompareOp::Gt,
            vec![PredicateValue::Integer(5)],
        );
        assert_eq!(
            TokenQuery::from(group(vec![duration.clone()])).tokens,
            vec![Token::positive(group(vec![duration]))]
        );
    }

    /// "Starts with" is a literal prefix of the whole value: regex
    /// metacharacters stay literal and the rest may span lines.
    #[test]
    fn starts_with_pattern_is_a_literal_prefix() {
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
                ("name".into(), "a.b(c)\nrest".into()),
                ("attributes.k".into(), "axb(c)".into()),
            ],
            events: vec![],
            links: vec![],
        };
        for (target, prefix, want) in [
            (PredicateTarget::Builtin(BuiltinField::Name), "a.b(", true),
            (
                PredicateTarget::Builtin(BuiltinField::Name),
                "a.b(c)\nrest",
                true,
            ),
            (
                PredicateTarget::Builtin(BuiltinField::Name),
                "a.b(c)\nrest!",
                false,
            ),
            (PredicateTarget::Builtin(BuiltinField::Name), "b(c)", false),
            (span_attr("k"), "a.b", false),
            (span_attr("k"), "axb(", true),
        ] {
            let predicate = Predicate {
                conditions: vec![cond(
                    target.clone(),
                    CompareOp::Regex,
                    vec![text(&starts_with_pattern(prefix))],
                )],
            };
            assert_eq!(
                span_matches(&span, &predicate, None),
                want,
                "{target} ^= {prefix:?}"
            );
        }
    }

    /// Free text lowers to one field-set term over the text scope: the
    /// name, status message and event names, and every attribute
    /// owner's fields — not `kind`, `status.code` or the scope name.
    #[test]
    fn free_text_lowers_to_the_text_scope() {
        let plan = group(vec![free_text("Abc")]).to_trace_plan();
        let strings = |v: &[&str]| v.iter().map(|s| s.to_string()).collect::<Vec<_>>();
        assert_eq!(
            plan.terms,
            vec![PlanTerm::FieldsIn {
                exact: strings(&["name", "status.message", "events.name"]),
                prefixes: strings(&[
                    "resource.attributes.",
                    "attributes.",
                    "scope.attributes.",
                    "events.attributes.",
                    "links.attributes.",
                ]),
                matcher: sfst::PlanMatcher::Contains("Abc".to_string()),
            }]
        );
        // The span-side scope test, without building names.
        let (exact, prefixes) = match &plan.terms[0] {
            PlanTerm::FieldsIn {
                exact, prefixes, ..
            } => (exact, prefixes),
            _ => unreachable!(),
        };
        for (head, tail, want) in [
            ("name", "", true),
            ("events.name", "", true),
            ("events.attributes.", "k", true),
            ("links.attributes.", "a.b", true),
            ("attributes.http.url", "", true),
            ("kind", "", false),
            ("status.code", "", false),
            ("scope.name", "", false),
            ("events.", "name", true),
            ("events.", "names", false),
            ("links.", "attributes.x", true),
        ] {
            assert_eq!(scope_has(exact, prefixes, head, tail), want, "{head}{tail}");
        }
    }
}
