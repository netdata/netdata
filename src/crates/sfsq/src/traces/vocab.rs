//! The key vocabulary: OpenTelemetry's own, typed. ATTRIBUTES belong to
//! an owner (the OTLP message carrying them — Resource,
//! InstrumentationScope, Span, Event, Link); BUILTIN FIELDS are the fixed
//! members of those messages themselves (name, kind, status, ...), plus
//! the derived fields OTLP has no member for (`duration`, `root_name`,
//! ...). Which builtins are TRACE-LEVEL — stored span groups cannot
//! answer them (`root_name`, `root_service_name`, `trace_duration`, the
//! `trace_id` constraint) — is not encoded here: `predicate.rs` owns that
//! list (`Predicate::trace_level_target`) so a consumer cannot drift from
//! the engine.
//!
//! Every key is spelled as its OTLP field path, which is also the name
//! storage gives the field: `resource.attributes.service.name`,
//! `scope.name`, `status.code`, `events.attributes.exception.type`
//! ([`parse_key`] / [`render_key`]). That spelling is the one vocabulary
//! every consumer shares (the netdata wire, the CLI); consumers own only
//! their envelopes. The predicate AST and suggestions consume the typed
//! form ([`AttributeOwner`], [`AttributeKey`]).
//!
//! The storage side lives here too: [`classify_storage`] sorts a stored
//! field name into a key, `trace_state`, an internal field or an unknown
//! name; [`AttributeOwner::attribute_prefix`] and
//! [`BuiltinField::dictionary_field`] serve value lookups and predicate
//! lowering — so there is exactly one table to keep right.

/// Which OTLP message a key belongs to. `Resource`/`Span`/
/// `Instrumentation`/`Event`/`Link` hold attribute keys (the attribute's
/// flattened path, without its owner prefix); `Builtin` holds the fixed
/// [`BuiltinField`] set.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum AttributeOwner {
    Resource,
    Span,
    Instrumentation,
    Event,
    Link,
    Builtin,
}

impl AttributeOwner {
    /// The owners that hold attribute keys.
    pub const ATTRIBUTE_OWNERS: [AttributeOwner; 5] = [
        AttributeOwner::Resource,
        AttributeOwner::Span,
        AttributeOwner::Instrumentation,
        AttributeOwner::Event,
        AttributeOwner::Link,
    ];

    /// The path prefix of this owner's attribute keys, in keys and in
    /// storage alike; `None` for [`Builtin`](AttributeOwner::Builtin)
    /// (builtin fields are not attributes). No prefix is a prefix of
    /// another, and no fixed key starts with one.
    pub fn attribute_prefix(self) -> Option<&'static str> {
        match self {
            AttributeOwner::Resource => Some("resource.attributes."),
            AttributeOwner::Span => Some("attributes."),
            AttributeOwner::Instrumentation => Some("scope.attributes."),
            AttributeOwner::Event => Some("events.attributes."),
            AttributeOwner::Link => Some("links.attributes."),
            AttributeOwner::Builtin => None,
        }
    }
}

/// The fixed builtin-field set — engine capabilities, not data
/// properties, which is why key enumeration lists ALL of them
/// unconditionally: filtering on `Status` is valid on a
/// corpus with no statuses (it matches nothing).
///
/// Dictionary-backed builtins
/// ([`dictionary_field`](BuiltinField::dictionary_field) = `Some`)
/// support value enumeration; the rest are VIRTUAL — backed by per-row
/// columns (`Duration`, `SpanId`, `ParentSpanId`, `TraceId`), by the
/// EVNB/LNKB event/link structures (`LinkSpanId`, `LinkTraceId`,
/// `EventTimeSinceStart`), or by post-assembly derivation (`RootName`,
/// `RootServiceName`, `TraceDuration`) — and value enumeration on them
/// is a request error.
///
/// `trace_state` is deliberately NOT a builtin: the key
/// vocabulary exists for filter autocomplete, and no query path filters
/// on it; it stays visible in trace-by-id results.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum BuiltinField {
    // ── Dictionary-backed ───────────────────────────────────────────
    Name,
    Kind,
    Status,
    StatusMessage,
    InstrumentationName,
    InstrumentationVersion,
    EventName,
    // ── Virtual ─────────────────────────────────────────────────────
    Duration,
    SpanId,
    ParentSpanId,
    TraceId,
    LinkSpanId,
    LinkTraceId,
    EventTimeSinceStart,
    RootName,
    RootServiceName,
    TraceDuration,
}

impl BuiltinField {
    /// Every builtin field, in declaration order (dictionary-backed first).
    pub const ALL: [BuiltinField; 17] = [
        BuiltinField::Name,
        BuiltinField::Kind,
        BuiltinField::Status,
        BuiltinField::StatusMessage,
        BuiltinField::InstrumentationName,
        BuiltinField::InstrumentationVersion,
        BuiltinField::EventName,
        BuiltinField::Duration,
        BuiltinField::SpanId,
        BuiltinField::ParentSpanId,
        BuiltinField::TraceId,
        BuiltinField::LinkSpanId,
        BuiltinField::LinkTraceId,
        BuiltinField::EventTimeSinceStart,
        BuiltinField::RootName,
        BuiltinField::RootServiceName,
        BuiltinField::TraceDuration,
    ];

    /// The key: the OTLP field path (`status.code`, `scope.name`,
    /// `links.trace_id`), or the field's name when OTLP has no member for
    /// it (`duration`, `events.time_since_start`, `root_name`,
    /// `root_service_name`, `trace_duration`).
    pub fn key(self) -> &'static str {
        match self {
            BuiltinField::Name => "name",
            BuiltinField::Kind => "kind",
            BuiltinField::Status => "status.code",
            BuiltinField::StatusMessage => "status.message",
            BuiltinField::InstrumentationName => "scope.name",
            BuiltinField::InstrumentationVersion => "scope.version",
            BuiltinField::EventName => "events.name",
            BuiltinField::Duration => "duration",
            BuiltinField::SpanId => "span_id",
            BuiltinField::ParentSpanId => "parent_span_id",
            BuiltinField::TraceId => "trace_id",
            BuiltinField::LinkSpanId => "links.span_id",
            BuiltinField::LinkTraceId => "links.trace_id",
            BuiltinField::EventTimeSinceStart => "events.time_since_start",
            BuiltinField::RootName => "root_name",
            BuiltinField::RootServiceName => "root_service_name",
            BuiltinField::TraceDuration => "trace_duration",
        }
    }

    /// The storage dictionary field backing this builtin — its
    /// [`key`](BuiltinField::key), since storage names fields by their
    /// OTLP path; `None` means VIRTUAL (no value dictionary anywhere — see
    /// the type docs).
    ///
    /// Values come back as the STORAGE labels (`kind` ∈
    /// `UNSPECIFIED/INTERNAL/SERVER/CLIENT/PRODUCER/CONSUMER`, `status` ∈
    /// `UNSET/OK/ERROR`, or the decimal number of a variant the writer did
    /// not know).
    pub fn dictionary_field(self) -> Option<&'static str> {
        match self {
            BuiltinField::Name
            | BuiltinField::Kind
            | BuiltinField::Status
            | BuiltinField::StatusMessage
            | BuiltinField::InstrumentationName
            | BuiltinField::InstrumentationVersion
            | BuiltinField::EventName => Some(self.key()),
            _ => None,
        }
    }
}

/// One key: a data-derived attribute (its flattened path without the
/// owner prefix) or a member of the fixed builtin-field set. Meaningful
/// only as part of a `(AttributeOwner, AttributeKey)` pair —
/// `Attribute("name")` under [`AttributeOwner::Span`] and
/// [`BuiltinField::Name`] are different keys.
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum AttributeKey {
    Attribute(String),
    Builtin(BuiltinField),
}

/// Why a string is not a key.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum KeyError {
    /// An attribute prefix with no attribute after it (`attributes.`).
    EmptyAttribute,
    /// Neither an attribute path nor a builtin key.
    Unknown,
}

/// Parse a key: an attribute owner's prefix followed by a non-empty
/// attribute path (kept verbatim, dots included:
/// `resource.attributes.service.name` → the `service.name` resource
/// attribute), or a builtin's [`key`](BuiltinField::key).
pub fn parse_key(key: &str) -> Result<(AttributeOwner, AttributeKey), KeyError> {
    for owner in AttributeOwner::ATTRIBUTE_OWNERS {
        let prefix = owner.attribute_prefix().expect("attribute owner");
        if let Some(rest) = key.strip_prefix(prefix) {
            if rest.is_empty() {
                return Err(KeyError::EmptyAttribute);
            }
            return Ok((owner, AttributeKey::Attribute(rest.to_string())));
        }
    }
    BuiltinField::ALL
        .into_iter()
        .find(|b| b.key() == key)
        .map(|b| (AttributeOwner::Builtin, AttributeKey::Builtin(b)))
        .ok_or(KeyError::Unknown)
}

/// Render a key — the exact inverse of [`parse_key`].
pub fn render_key(owner: AttributeOwner, key: &AttributeKey) -> String {
    match key {
        AttributeKey::Builtin(b) => b.key().to_string(),
        AttributeKey::Attribute(a) => format!(
            "{}{a}",
            owner
                .attribute_prefix()
                .expect("attribute keys carry an attribute owner")
        ),
    }
}

/// The forms a key takes, for error messages: the attribute prefixes,
/// then every builtin key.
pub fn key_forms() -> String {
    let prefixes: Vec<String> = AttributeOwner::ATTRIBUTE_OWNERS
        .iter()
        .map(|o| format!("{}<key>", o.attribute_prefix().expect("attribute owner")))
        .collect();
    let builtins: Vec<&str> = BuiltinField::ALL.iter().map(|b| b.key()).collect();
    format!("{}, or one of {}", prefixes.join(", "), builtins.join(", "))
}

/// The internal fields: the raw-int copies of the label-carrying `kind`
/// and `status.code` fields. They never leave the engine as keys.
const INTERNAL_FIELDS: [&str; 2] = ["_kind", "_status_code"];

/// The span's W3C trace state: stored and shown, never a key (no query
/// path filters on it).
pub const TRACE_STATE_FIELD: &str = "trace_state";

/// What a stored field name is.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum StorageName {
    /// A key: an attribute with a non-empty path, or a dictionary-backed
    /// builtin.
    Key(AttributeOwner, AttributeKey),
    /// [`TRACE_STATE_FIELD`].
    TraceState,
    /// One of the internal raw-int fields.
    Internal,
    /// Anything else: a name no writer of this build produces (a damaged
    /// file, a file written by another version), an attribute prefix with
    /// no attribute, or a virtual builtin's key (never stored).
    Unknown,
}

/// Classify one stored field name.
pub fn classify_storage(name: &str) -> StorageName {
    if INTERNAL_FIELDS.contains(&name) {
        return StorageName::Internal;
    }
    if name == TRACE_STATE_FIELD {
        return StorageName::TraceState;
    }
    match parse_key(name) {
        Ok((_, AttributeKey::Builtin(b))) if b.dictionary_field().is_none() => StorageName::Unknown,
        Ok((owner, key)) => StorageName::Key(owner, key),
        Err(_) => StorageName::Unknown,
    }
}

/// Map one storage field name to its key, or `None` when the field is
/// not a key ([`classify_storage`]). That `None` is the vocabulary's one
/// deliberate silent filter for key suggestions — a dropped name never
/// surfaces as a filterable key (see suggest.rs).
pub fn storage_to_attribute(storage: &str) -> Option<(AttributeOwner, AttributeKey)> {
    match classify_storage(storage) {
        StorageName::Key(owner, key) => Some((owner, key)),
        StorageName::TraceState | StorageName::Internal | StorageName::Unknown => None,
    }
}

/// The storage fields free text searches (decision 18: values only): the
/// span name, the status message and event names, plus every attribute
/// owner's values. Never field names, the `kind`/`status.code` labels, or
/// ids.
pub(crate) struct TextScope {
    /// Fields searched by exact storage name.
    pub(crate) exact: Vec<String>,
    /// Field-name prefixes: every field under one is searched.
    pub(crate) prefixes: Vec<String>,
}

/// The free-text scope, built from the vocabulary.
pub(crate) fn text_scope() -> TextScope {
    let builtins = [
        BuiltinField::Name,
        BuiltinField::StatusMessage,
        BuiltinField::EventName,
    ];
    let owners = AttributeOwner::ATTRIBUTE_OWNERS;
    TextScope {
        exact: builtins
            .iter()
            .map(|b| b.dictionary_field().expect("dictionary-backed").to_string())
            .collect(),
        prefixes: owners
            .iter()
            .map(|o| o.attribute_prefix().expect("attribute owner").to_string())
            .collect(),
    }
}

/// The resource `service.name` attribute's full storage field — the one
/// composite spelling several folds need (`summarize`, the trace
/// aggregates). Built from the vocabulary, never hand-written.
pub(crate) fn resource_service_field() -> String {
    render_key(
        AttributeOwner::Resource,
        &AttributeKey::Attribute("service.name".to_string()),
    )
}

/// A span's resolved facet value by storage field name, borrowed (no
/// per-lookup allocation — hot-loop callers check e.g. `== "ERROR"`).
pub(crate) fn span_field<'a>(span: &'a sfst::TraceSpan, field: &str) -> Option<&'a str> {
    span.fields
        .iter()
        .find(|(k, _)| k == field)
        .map(|(_, v)| v.as_str())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn attr(owner: AttributeOwner, k: &str) -> StorageName {
        StorageName::Key(owner, AttributeKey::Attribute(k.to_string()))
    }

    fn builtin(b: BuiltinField) -> StorageName {
        StorageName::Key(AttributeOwner::Builtin, AttributeKey::Builtin(b))
    }

    /// Every kind of stored name classifies as pinned, and every key
    /// renders back to the storage name it came from.
    #[test]
    fn storage_names_classify_as_pinned_and_render_back() {
        let table = [
            (
                "resource.attributes.host",
                attr(AttributeOwner::Resource, "host"),
            ),
            (
                "attributes.http.method",
                attr(AttributeOwner::Span, "http.method"),
            ),
            (
                "attributes.process.command_args[]",
                attr(AttributeOwner::Span, "process.command_args[]"),
            ),
            (
                "scope.attributes.lib",
                attr(AttributeOwner::Instrumentation, "lib"),
            ),
            ("events.attributes.msg", attr(AttributeOwner::Event, "msg")),
            ("links.attributes.rel", attr(AttributeOwner::Link, "rel")),
            ("name", builtin(BuiltinField::Name)),
            ("kind", builtin(BuiltinField::Kind)),
            ("status.code", builtin(BuiltinField::Status)),
            ("status.message", builtin(BuiltinField::StatusMessage)),
            ("scope.name", builtin(BuiltinField::InstrumentationName)),
            (
                "scope.version",
                builtin(BuiltinField::InstrumentationVersion),
            ),
            ("events.name", builtin(BuiltinField::EventName)),
            ("_kind", StorageName::Internal),
            ("_status_code", StorageName::Internal),
            ("trace_state", StorageName::TraceState),
            // An attribute prefix with no attribute, a virtual builtin's
            // key, the status names of earlier versions, and garbage.
            ("attributes.", StorageName::Unknown),
            ("events.attributes.", StorageName::Unknown),
            ("duration", StorageName::Unknown),
            ("links.trace_id", StorageName::Unknown),
            ("status_code", StorageName::Unknown),
            ("status_message", StorageName::Unknown),
            ("span.http.method", StorageName::Unknown),
            ("", StorageName::Unknown),
        ];
        for (storage, want) in table {
            let got = classify_storage(storage);
            assert_eq!(got, want, "classification of {storage:?}");
            assert_eq!(
                storage_to_attribute(storage).is_some(),
                matches!(got, StorageName::Key(..)),
                "storage_to_attribute of {storage:?}"
            );
            if let StorageName::Key(owner, key) = got {
                assert_eq!(render_key(owner, &key), storage, "render of {storage:?}");
            }
        }
    }

    /// Every builtin and attribute key round-trips through render and
    /// parse; a dictionary-backed builtin's key is its storage field.
    #[test]
    fn keys_round_trip_and_equal_storage_names() {
        for b in BuiltinField::ALL {
            let key = AttributeKey::Builtin(b);
            let text = render_key(AttributeOwner::Builtin, &key);
            assert_eq!(text, b.key());
            assert_eq!(parse_key(&text), Ok((AttributeOwner::Builtin, key)));
            if let Some(field) = b.dictionary_field() {
                assert_eq!(field, text, "{b:?}: key and storage field differ");
            }
        }
        for owner in AttributeOwner::ATTRIBUTE_OWNERS {
            for a in ["k", "a.b.c", "name", "status.code", "x[]", "attributes.y"] {
                let key = AttributeKey::Attribute(a.to_string());
                let text = render_key(owner, &key);
                assert_eq!(
                    text,
                    format!("{}{a}", owner.attribute_prefix().unwrap()),
                    "storage spelling"
                );
                assert_eq!(parse_key(&text), Ok((owner, key)), "{text}");
            }
        }
    }

    /// No key has two meanings: no prefix is a prefix of another, no
    /// builtin key starts with a prefix, and builtin keys are distinct.
    #[test]
    fn keys_are_unambiguous() {
        let prefixes: Vec<&str> = AttributeOwner::ATTRIBUTE_OWNERS
            .iter()
            .map(|o| o.attribute_prefix().unwrap())
            .collect();
        for (i, a) in prefixes.iter().enumerate() {
            for (j, b) in prefixes.iter().enumerate() {
                assert!(i == j || !b.starts_with(a), "{a} prefixes {b}");
            }
            for builtin in BuiltinField::ALL {
                assert!(!builtin.key().starts_with(a), "{builtin:?} under {a}");
            }
        }
        let mut keys: Vec<&str> = BuiltinField::ALL.iter().map(|b| b.key()).collect();
        keys.sort_unstable();
        keys.dedup();
        assert_eq!(
            keys.len(),
            BuiltinField::ALL.len(),
            "duplicate builtin keys"
        );
        assert_eq!(AttributeOwner::Builtin.attribute_prefix(), None);
    }

    /// What is not a key, and why.
    #[test]
    fn non_keys_are_rejected() {
        for (text, want) in [
            ("attributes.", KeyError::EmptyAttribute),
            ("resource.attributes.", KeyError::EmptyAttribute),
            ("links.attributes.", KeyError::EmptyAttribute),
            ("", KeyError::Unknown),
            (".service.name", KeyError::Unknown),
            ("resource.service.name", KeyError::Unknown),
            ("span.http.method", KeyError::Unknown),
            ("status", KeyError::Unknown),
            ("instrumentation_name", KeyError::Unknown),
            ("trace_state", KeyError::Unknown),
            ("_kind", KeyError::Unknown),
            ("Name", KeyError::Unknown),
        ] {
            assert_eq!(parse_key(text), Err(want), "{text:?}");
        }
    }

    /// A span attribute literally named like a builtin's key stays a
    /// SPAN attribute: the owner prefix decides.
    #[test]
    fn attribute_named_like_a_builtin_does_not_collide() {
        assert_eq!(
            classify_storage("attributes.trace_state"),
            attr(AttributeOwner::Span, "trace_state")
        );
        assert_eq!(
            classify_storage("attributes.name"),
            attr(AttributeOwner::Span, "name")
        );
        assert_eq!(
            classify_storage("resource.attributes.scope.name"),
            attr(AttributeOwner::Resource, "scope.name")
        );
    }

    /// The virtual/dictionary split is total and matches the pinned
    /// sets; ALL enumerates every variant exactly once.
    #[test]
    fn builtin_split_is_pinned() {
        use BuiltinField::*;
        let dictionary: Vec<BuiltinField> = BuiltinField::ALL
            .into_iter()
            .filter(|i| i.dictionary_field().is_some())
            .collect();
        assert_eq!(
            dictionary,
            vec![
                Name,
                Kind,
                Status,
                StatusMessage,
                InstrumentationName,
                InstrumentationVersion,
                EventName
            ]
        );
        let mut all = BuiltinField::ALL.to_vec();
        all.sort();
        all.dedup();
        assert_eq!(all.len(), BuiltinField::ALL.len(), "ALL has duplicates");
    }
}
