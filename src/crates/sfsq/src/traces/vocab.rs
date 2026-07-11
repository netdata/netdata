//! The typed, wire-neutral tag vocabulary (phase-4b decision 16A):
//! scopes and keys are ENUMS, never grammar strings. Each wire adapter —
//! the phase-5 Tempo shim, a future Netdata UI — owns its own string
//! rendering of this vocabulary, in both directions, and must round-trip
//! it; the engine stays wire-neutral (Grafana/TraceQL is a stopgap, so
//! its words must not leak in here). The 4c predicate AST consumes these
//! same enums.
//!
//! The storage↔vocabulary mapping lives here in BOTH directions —
//! [`storage_to_tag`] for key enumeration; [`TagScope::attribute_prefix`]
//! and [`TraceIntrinsic::dictionary_field`] for value lookups and 4c
//! lowering — so there is exactly one table to keep right.

/// Where a tag lives. `Resource`/`Span`/`Instrumentation`/`Event`/`Link`
/// hold attribute keys (prefix-stripped storage names — the prefixes are
/// artifacts of this crate family's flattening, not something a consumer
/// should see); `Intrinsic` holds the fixed [`TraceIntrinsic`] set.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum TagScope {
    Resource,
    Span,
    Instrumentation,
    Event,
    Link,
    Intrinsic,
}

impl TagScope {
    /// The storage prefix this scope's attribute keys carry on disk;
    /// `None` for [`Intrinsic`](TagScope::Intrinsic) (intrinsics are not
    /// attributes). Stripping/prepending happens against the FULL prefix
    /// (`events.attributes.`, not `events.`) — a scope-word-only strip
    /// would leave a stray `attributes.` on every key.
    pub fn attribute_prefix(self) -> Option<&'static str> {
        match self {
            TagScope::Resource => Some("resource.attributes."),
            TagScope::Span => Some("attributes."),
            TagScope::Instrumentation => Some("scope.attributes."),
            TagScope::Event => Some("events.attributes."),
            TagScope::Link => Some("links.attributes."),
            TagScope::Intrinsic => None,
        }
    }
}

/// The fixed intrinsic set — engine capabilities, not data properties,
/// which is why key enumeration lists ALL of them unconditionally
/// (decision 18B): filtering on `Status` is valid on a corpus with no
/// statuses (it matches nothing).
///
/// Dictionary-backed intrinsics ([`dictionary_field`]
/// (Self::dictionary_field) = `Some`) support value enumeration; the
/// rest are VIRTUAL — backed by per-row columns (`Duration`, `SpanId`,
/// `ParentSpanId`, `TraceId`), by the EVNB/LNKB structures (`LinkSpanId`,
/// `LinkTraceId`, `EventTimeSinceStart`), or by post-assembly derivation
/// (`RootName`, `RootServiceName`, `TraceDuration`) — and value
/// enumeration on them is a request error.
///
/// `trace_state` is deliberately NOT an intrinsic (decision 17A): tags
/// exist for filter autocomplete, and no query path filters on it; it
/// stays visible in trace-by-id results.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum TraceIntrinsic {
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

impl TraceIntrinsic {
    /// Every intrinsic, in declaration order (dictionary-backed first).
    pub const ALL: [TraceIntrinsic; 17] = [
        TraceIntrinsic::Name,
        TraceIntrinsic::Kind,
        TraceIntrinsic::Status,
        TraceIntrinsic::StatusMessage,
        TraceIntrinsic::InstrumentationName,
        TraceIntrinsic::InstrumentationVersion,
        TraceIntrinsic::EventName,
        TraceIntrinsic::Duration,
        TraceIntrinsic::SpanId,
        TraceIntrinsic::ParentSpanId,
        TraceIntrinsic::TraceId,
        TraceIntrinsic::LinkSpanId,
        TraceIntrinsic::LinkTraceId,
        TraceIntrinsic::EventTimeSinceStart,
        TraceIntrinsic::RootName,
        TraceIntrinsic::RootServiceName,
        TraceIntrinsic::TraceDuration,
    ];

    /// The storage dictionary field backing this intrinsic; `None` means
    /// VIRTUAL (no value dictionary anywhere — see the type docs).
    ///
    /// Values come back as the STORAGE labels (`kind` ∈
    /// `INTERNAL/SERVER/CLIENT/PRODUCER/CONSUMER`, `status` ∈
    /// `OK/ERROR`); mapping those to a wire vocabulary (e.g. TraceQL's
    /// lowercase keywords) is the wire adapter's job, next to its name
    /// table (decision 19, dissolved into 16A).
    pub fn dictionary_field(self) -> Option<&'static str> {
        match self {
            TraceIntrinsic::Name => Some("name"),
            TraceIntrinsic::Kind => Some("kind"),
            TraceIntrinsic::Status => Some("status_code"),
            TraceIntrinsic::StatusMessage => Some("status_message"),
            TraceIntrinsic::InstrumentationName => Some("scope.name"),
            TraceIntrinsic::InstrumentationVersion => Some("scope.version"),
            TraceIntrinsic::EventName => Some("events.name"),
            _ => None,
        }
    }
}

/// One tag key: a data-derived attribute (bare, prefix-stripped) or a
/// member of the fixed intrinsic set. Meaningful only as part of a
/// `(TagScope, TagKey)` pair — `Attribute("name")` under
/// [`TagScope::Span`] and [`TraceIntrinsic::Name`] are different tags.
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum TagKey {
    Attribute(String),
    Intrinsic(TraceIntrinsic),
}

/// Internal facets that must never surface as tags: the raw-int shadow
/// entries of the label-carrying `kind` / `status_code` fields.
const INTERNAL_FIELDS: [&str; 2] = ["_kind", "_status_code"];

/// Map one storage field name to its tag, or `None` when the field is
/// not part of the tag vocabulary: the internal facets above,
/// `trace_state` (17A), and anything unrecognized (a foreign field in a
/// file this engine was pointed at is not vocabulary).
pub fn storage_to_tag(storage: &str) -> Option<(TagScope, TagKey)> {
    if INTERNAL_FIELDS.contains(&storage) || storage == "trace_state" {
        return None;
    }
    for intrinsic in TraceIntrinsic::ALL {
        if intrinsic.dictionary_field() == Some(storage) {
            return Some((TagScope::Intrinsic, TagKey::Intrinsic(intrinsic)));
        }
    }
    for scope in [
        TagScope::Resource,
        TagScope::Span,
        TagScope::Instrumentation,
        TagScope::Event,
        TagScope::Link,
    ] {
        let prefix = scope.attribute_prefix().expect("attribute scopes");
        if let Some(bare) = storage.strip_prefix(prefix) {
            return Some((scope, TagKey::Attribute(bare.to_string())));
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The complete storage→(scope, key) table from the 4b SOW, pinned:
    /// every dictionary field maps exactly as recorded, and the mapping
    /// round-trips back to the storage name.
    #[test]
    fn storage_table_is_pinned_and_round_trips() {
        let attr = |scope: TagScope, k: &str| Some((scope, TagKey::Attribute(k.to_string())));
        let intr = |i: TraceIntrinsic| Some((TagScope::Intrinsic, TagKey::Intrinsic(i)));
        let table: [(&str, Option<(TagScope, TagKey)>); 15] = [
            ("resource.attributes.host", attr(TagScope::Resource, "host")),
            ("attributes.http.method", attr(TagScope::Span, "http.method")),
            ("scope.attributes.lib", attr(TagScope::Instrumentation, "lib")),
            ("events.attributes.msg", attr(TagScope::Event, "msg")),
            ("links.attributes.rel", attr(TagScope::Link, "rel")),
            ("name", intr(TraceIntrinsic::Name)),
            ("kind", intr(TraceIntrinsic::Kind)),
            ("status_code", intr(TraceIntrinsic::Status)),
            ("status_message", intr(TraceIntrinsic::StatusMessage)),
            ("scope.name", intr(TraceIntrinsic::InstrumentationName)),
            ("scope.version", intr(TraceIntrinsic::InstrumentationVersion)),
            ("events.name", intr(TraceIntrinsic::EventName)),
            // Excluded from the vocabulary:
            ("_kind", None),
            ("_status_code", None),
            ("trace_state", None),
        ];
        for (storage, want) in table {
            let got = storage_to_tag(storage);
            assert_eq!(got, want, "mapping of {storage:?}");
            // Round trip: the tag resolves back to the same storage name.
            if let Some((scope, key)) = got {
                let back = match (&key, scope.attribute_prefix()) {
                    (TagKey::Attribute(bare), Some(prefix)) => format!("{prefix}{bare}"),
                    (TagKey::Intrinsic(i), None) => {
                        i.dictionary_field().expect("mapped intrinsic").to_string()
                    }
                    other => panic!("inconsistent pair {other:?}"),
                };
                assert_eq!(back, storage, "round trip of {storage:?}");
            }
        }
    }

    /// A span attribute literally named like an intrinsic's storage field
    /// stays a SPAN attribute (typed keys cannot collide — the 17A
    /// collision argument dissolved for exactly this reason).
    #[test]
    fn attribute_named_like_an_intrinsic_does_not_collide() {
        assert_eq!(
            storage_to_tag("attributes.trace_state"),
            Some((TagScope::Span, TagKey::Attribute("trace_state".to_string())))
        );
        assert_eq!(
            storage_to_tag("attributes.name"),
            Some((TagScope::Span, TagKey::Attribute("name".to_string())))
        );
    }

    /// The virtual/dictionary split is total and matches the pinned
    /// sets; ALL enumerates every variant exactly once.
    #[test]
    fn intrinsic_split_is_pinned() {
        use TraceIntrinsic::*;
        let dictionary: Vec<TraceIntrinsic> = TraceIntrinsic::ALL
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
        let mut all = TraceIntrinsic::ALL.to_vec();
        all.sort();
        all.dedup();
        assert_eq!(all.len(), TraceIntrinsic::ALL.len(), "ALL has duplicates");
    }
}
