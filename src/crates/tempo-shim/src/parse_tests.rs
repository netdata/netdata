//! Golden parser tests. The "pinned:" cases are the EXACT generated
//! strings the plugin's own tests assert (grafana-tempo-datasource @
//! 95f2697 — `src/language_provider.test.ts` [lp] and
//! `src/SearchTraceQLEditor/utils.test.ts` [utils]; the utils cases pin
//! section strings, wrapped here in the `{}` the query generator adds).
//! Copied into this file per the SOW's golden-test rule — no CI fetch of
//! the plugin.

use sfsq::traces::{
    CompareOp, Condition, Predicate, PredicateTarget, PredicateValue, TagScope, TraceIntrinsic,
};

use super::parse_query;
use crate::error::ParseError;

fn cond(target: PredicateTarget, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
    Condition { target, op, values }
}

fn unscoped(key: &str) -> PredicateTarget {
    PredicateTarget::UnscopedAttribute(key.to_string())
}

fn scoped(scope: TagScope, key: &str) -> PredicateTarget {
    PredicateTarget::Attribute(scope, key.to_string())
}

fn intr(i: TraceIntrinsic) -> PredicateTarget {
    PredicateTarget::Intrinsic(i)
}

fn text(s: &str) -> PredicateValue {
    PredicateValue::Text(s.to_string())
}

#[track_caller]
fn parses_to(q: &str, conditions: Vec<Condition>) {
    assert_eq!(parse_query(q), Ok(Predicate { conditions }), "query: {q}");
}

#[test]
fn pinned_empty_and_absent() {
    // pinned: lp:74 (empty form), lp:78 (valueless filter dropped),
    // lp:189, lp:264 (empty ad-hoc list).
    parses_to("{}", vec![]);
    parses_to("{ }", vec![]);
    // The plugin omits `q` when the raw editor is empty — absent/blank
    // parses as match-all.
    parses_to("", vec![]);
    parses_to("   ", vec![]);
}

#[test]
fn pinned_durations() {
    // pinned: lp:101/:111.
    parses_to(
        "{duration>100ms}",
        vec![cond(
            intr(TraceIntrinsic::Duration),
            CompareOp::Gt,
            vec![PredicateValue::Integer(100_000_000)],
        )],
    );
    // pinned: lp:121.
    parses_to(
        "{traceDuration>100ms}",
        vec![cond(
            intr(TraceIntrinsic::TraceDuration),
            CompareOp::Gt,
            vec![PredicateValue::Integer(100_000_000)],
        )],
    );
    // pinned: utils:118.
    parses_to(
        "{duration=100ms}",
        vec![cond(
            intr(TraceIntrinsic::Duration),
            CompareOp::Eq,
            vec![PredicateValue::Integer(100_000_000)],
        )],
    );
    // Decimals and the µs unit (the form's DurationInput admits them).
    parses_to(
        "{duration>1.2ms}",
        vec![cond(
            intr(TraceIntrinsic::Duration),
            CompareOp::Gt,
            vec![PredicateValue::Integer(1_200_000)],
        )],
    );
    parses_to(
        "{event:timeSinceStart>300µs}",
        vec![cond(
            intr(TraceIntrinsic::EventTimeSinceStart),
            CompareOp::Gt,
            vec![PredicateValue::Integer(300_000)],
        )],
    );
}

#[test]
fn pinned_unscoped_attributes() {
    // pinned: lp:130 (bare value), lp:135 (quoted), lp:143 (number),
    // lp:169 (conjunction), lp:179/:205.
    parses_to(
        "{.footag=foovalue}",
        vec![cond(unscoped("footag"), CompareOp::Eq, vec![text("foovalue")])],
    );
    parses_to(
        "{.footag=\"foovalue\"}",
        vec![cond(unscoped("footag"), CompareOp::Eq, vec![text("foovalue")])],
    );
    parses_to(
        "{.footag>1234}",
        vec![cond(
            unscoped("footag"),
            CompareOp::Gt,
            vec![PredicateValue::Integer(1234)],
        )],
    );
    parses_to(
        "{.footag>=1234 && .bartag=\"barvalue\"}",
        vec![
            cond(
                unscoped("footag"),
                CompareOp::Gte,
                vec![PredicateValue::Integer(1234)],
            ),
            cond(unscoped("bartag"), CompareOp::Eq, vec![text("barvalue")]),
        ],
    );
}

#[test]
fn pinned_scoped_attributes() {
    // pinned: lp:221, lp:237, lp:261; utils:112.
    parses_to(
        "{span.footag>=1234}",
        vec![cond(
            scoped(TagScope::Span, "footag"),
            CompareOp::Gte,
            vec![PredicateValue::Integer(1234)],
        )],
    );
    parses_to(
        "{resource.footag>=1234 && resource.name=\"foo\"}",
        vec![
            cond(
                scoped(TagScope::Resource, "footag"),
                CompareOp::Gte,
                vec![PredicateValue::Integer(1234)],
            ),
            cond(scoped(TagScope::Resource, "name"), CompareOp::Eq, vec![text("foo")]),
        ],
    );
    parses_to(
        "{resource.foo=bar}",
        vec![cond(scoped(TagScope::Resource, "foo"), CompareOp::Eq, vec![text("bar")])],
    );
    parses_to(
        "{event.exception.type=\"E\" && link.rel=\"x\" && instrumentation.lib=\"l\"}",
        vec![
            cond(scoped(TagScope::Event, "exception.type"), CompareOp::Eq, vec![text("E")]),
            cond(scoped(TagScope::Link, "rel"), CompareOp::Eq, vec![text("x")]),
            cond(scoped(TagScope::Instrumentation, "lib"), CompareOp::Eq, vec![text("l")]),
        ],
    );
}

#[test]
fn pinned_bare_adhoc_fields() {
    // pinned: lp:270, lp:281 — dashboard ad-hoc filters generate bare
    // fields with NO leading dot; they carry unscoped semantics.
    parses_to(
        "{footag=\"foovalue\"}",
        vec![cond(unscoped("footag"), CompareOp::Eq, vec![text("foovalue")])],
    );
    parses_to(
        "{footag=\"foovalue\" && bartag=0}",
        vec![
            cond(unscoped("footag"), CompareOp::Eq, vec![text("foovalue")]),
            cond(unscoped("bartag"), CompareOp::Eq, vec![PredicateValue::Integer(0)]),
        ],
    );
}

#[test]
fn pinned_enum_intrinsics() {
    // pinned: lp:287 — bare lowercase keywords map to storage labels.
    parses_to(
        "{kind=server}",
        vec![cond(intr(TraceIntrinsic::Kind), CompareOp::Eq, vec![text("SERVER")])],
    );
    parses_to(
        "{status=error}",
        vec![cond(intr(TraceIntrinsic::Status), CompareOp::Eq, vec![text("ERROR")])],
    );
    // Case-insensitive; quoted keywords tolerated; unset maps to a
    // label the store never carries (validly matches nothing).
    parses_to(
        "{kind=SERVER}",
        vec![cond(intr(TraceIntrinsic::Kind), CompareOp::Eq, vec![text("SERVER")])],
    );
    parses_to(
        "{status=\"ok\"}",
        vec![cond(intr(TraceIntrinsic::Status), CompareOp::Eq, vec![text("OK")])],
    );
    parses_to(
        "{status!=unset}",
        vec![cond(intr(TraceIntrinsic::Status), CompareOp::NotEq, vec![text("UNSET")])],
    );
}

#[test]
fn pinned_text_intrinsics() {
    // pinned: lp:293.
    parses_to(
        "{name=\"my-server\"}",
        vec![cond(intr(TraceIntrinsic::Name), CompareOp::Eq, vec![text("my-server")])],
    );
    parses_to(
        "{statusMessage=\"boom\" && rootServiceName=\"svc\" && rootName=\"GET /\"}",
        vec![
            cond(intr(TraceIntrinsic::StatusMessage), CompareOp::Eq, vec![text("boom")]),
            cond(intr(TraceIntrinsic::RootServiceName), CompareOp::Eq, vec![text("svc")]),
            cond(intr(TraceIntrinsic::RootName), CompareOp::Eq, vec![text("GET /")]),
        ],
    );
}

#[test]
fn pinned_regex_escapes() {
    // pinned: lp:159 — the form regex-escapes string values; on the
    // wire each metacharacter carries a `\\` pair, which the string
    // lexer folds to one backslash (a regex matching the literal text).
    parses_to(
        r#"{name=~"api/v2/variants/by-upc/\\(\\?P<upc>\\[\\s\\S\\]\\*\\)/\\$"}"#,
        vec![cond(
            intr(TraceIntrinsic::Name),
            CompareOp::Regex,
            vec![text(r"api/v2/variants/by-upc/\(\?P<upc>\[\s\S\]\*\)/\$")],
        )],
    );
    // Hand-written (raw-editor) escapes that are NOT wire-doubled pass
    // through verbatim: `\s` stays `\s`.
    parses_to(
        r#"{name=~"a\sb"}"#,
        vec![cond(intr(TraceIntrinsic::Name), CompareOp::Regex, vec![text(r"a\sb")])],
    );
    // pinned: utils:100.
    parses_to(
        r#"{.foo=~"bar.*"}"#,
        vec![cond(unscoped("foo"), CompareOp::Regex, vec![text("bar.*")])],
    );
}

#[test]
fn pinned_multi_value_sections() {
    // pinned: utils:130 / utils:143 — multi-value `=` renders as an
    // OR group and folds into ONE condition (engine `=` values OR).
    let both = vec![cond(
        scoped(TagScope::Span, "foo"),
        CompareOp::Eq,
        vec![text("bar"), text("baz")],
    )];
    parses_to("{(span.foo=bar || span.foo=baz)}", both.clone());
    parses_to("{(span.foo=\"bar\" || span.foo=\"baz\")}", both);
    // pinned: utils:168 / utils:181 — multi-value `!=` renders as an
    // AND group (engine `!=` values AND).
    let neither = vec![cond(
        scoped(TagScope::Span, "foo"),
        CompareOp::NotEq,
        vec![text("bar"), text("baz")],
    )];
    parses_to("{(span.foo!=bar && span.foo!=baz)}", neither.clone());
    parses_to("{(span.foo!=\"bar\" && span.foo!=\"baz\")}", neither);
    // pinned: utils:156 / utils:194 — multi-value regex collapses to a
    // single alternation term, no parens.
    parses_to(
        r#"{span.foo=~"bar|baz"}"#,
        vec![cond(scoped(TagScope::Span, "foo"), CompareOp::Regex, vec![text("bar|baz")])],
    );
    parses_to(
        r#"{span.foo!~"bar|baz"}"#,
        vec![cond(scoped(TagScope::Span, "foo"), CompareOp::NotRegex, vec![text("bar|baz")])],
    );
    // A group combined with further sections.
    parses_to(
        "{(span.foo=bar || span.foo=baz) && kind=client}",
        vec![
            cond(scoped(TagScope::Span, "foo"), CompareOp::Eq, vec![text("bar"), text("baz")]),
            cond(intr(TraceIntrinsic::Kind), CompareOp::Eq, vec![text("CLIENT")]),
        ],
    );
}

#[test]
fn colon_form_ids() {
    parses_to(
        "{span:id=00f067aa0ba902b7}",
        vec![cond(intr(TraceIntrinsic::SpanId), CompareOp::Eq, vec![text("00f067aa0ba902b7")])],
    );
    parses_to(
        "{span:parentID!=00f067aa0ba902b7}",
        vec![cond(
            intr(TraceIntrinsic::ParentSpanId),
            CompareOp::NotEq,
            vec![text("00f067aa0ba902b7")],
        )],
    );
    parses_to(
        "{trace:id=0af7651916cd43dd8448eb211c80319c}",
        vec![cond(
            intr(TraceIntrinsic::TraceId),
            CompareOp::Eq,
            vec![text("0af7651916cd43dd8448eb211c80319c")],
        )],
    );
    parses_to(
        "{link:traceID=\"0af7651916cd43dd8448eb211c80319c\"}",
        vec![cond(
            intr(TraceIntrinsic::LinkTraceId),
            CompareOp::Eq,
            vec![text("0af7651916cd43dd8448eb211c80319c")],
        )],
    );
    parses_to(
        "{event:name=\"exception\"}",
        vec![cond(intr(TraceIntrinsic::EventName), CompareOp::Eq, vec![text("exception")])],
    );
    parses_to(
        "{instrumentation:name=\"otel\" && instrumentation:version=1.0}",
        vec![
            cond(intr(TraceIntrinsic::InstrumentationName), CompareOp::Eq, vec![text("otel")]),
            // A bare `1.0` under a TEXT intrinsic still classifies by
            // shape — the engine rejects non-text on text intrinsics,
            // which is the honest outcome for this raw-editor-only form.
            cond(
                intr(TraceIntrinsic::InstrumentationVersion),
                CompareOp::Eq,
                vec![PredicateValue::Float(1.0)],
            ),
        ],
    );
    // Colon-form aliases of the v1 intrinsics.
    parses_to(
        "{span:name=\"n\" && trace:rootService=\"svc\" && span:duration>1ms}",
        vec![
            cond(intr(TraceIntrinsic::Name), CompareOp::Eq, vec![text("n")]),
            cond(intr(TraceIntrinsic::RootServiceName), CompareOp::Eq, vec![text("svc")]),
            cond(
                intr(TraceIntrinsic::Duration),
                CompareOp::Gt,
                vec![PredicateValue::Integer(1_000_000)],
            ),
        ],
    );
}

#[test]
fn bare_value_classification() {
    parses_to(
        "{.f=1.5}",
        vec![cond(unscoped("f"), CompareOp::Eq, vec![PredicateValue::Float(1.5)])],
    );
    parses_to(
        "{.f=-3}",
        vec![cond(unscoped("f"), CompareOp::Eq, vec![PredicateValue::Integer(-3)])],
    );
    parses_to(
        "{.f=5e3}",
        vec![cond(unscoped("f"), CompareOp::Eq, vec![PredicateValue::Float(5000.0)])],
    );
    // Words that would parse as f64 but are not numeric-shaped stay
    // text (`nan` would otherwise hit the engine's NanValue error).
    parses_to("{.f=nan}", vec![cond(unscoped("f"), CompareOp::Eq, vec![text("nan")])]);
    parses_to("{.f=true}", vec![cond(unscoped("f"), CompareOp::Eq, vec![text("true")])]);
    // Duration-looking words on ATTRIBUTES are text (an attribute
    // holding the string "1.2ms" matches verbatim).
    parses_to("{.f=1.2ms}", vec![cond(unscoped("f"), CompareOp::Eq, vec![text("1.2ms")])]);
    // The flattener's array-element paths (`tags[]`) round-trip: tag
    // autocomplete advertises them, so the parser accepts them back.
    parses_to(
        "{span.tags[]=\"x\" && .arr[].deep=1}",
        vec![
            cond(scoped(TagScope::Span, "tags[]"), CompareOp::Eq, vec![text("x")]),
            cond(unscoped("arr[].deep"), CompareOp::Eq, vec![PredicateValue::Integer(1)]),
        ],
    );
}

#[track_caller]
fn rejects(q: &str, check: impl Fn(&ParseError) -> bool) {
    match parse_query(q) {
        Err(e) => assert!(check(&e), "query {q}: unexpected error {e:?}"),
        Ok(p) => panic!("query {q}: expected an error, parsed {p:?}"),
    }
}

#[test]
fn rejects_out_of_grammar_traceql() {
    let unsupported = |e: &ParseError| matches!(e, ParseError::Unsupported { .. });
    // Pipelines / aggregates / structural ops (raw-editor-only).
    rejects("{} | count_over_time()", unsupported);
    rejects("{name=\"a\"} >> {name=\"b\"}", unsupported);
    rejects("{name=\"a\"} << {name=\"b\"}", unsupported);
    rejects("{name=\"a\"} ~ {name=\"b\"}", unsupported);
    rejects("{name=\"a\"} {name=\"b\"}", unsupported); // trailing content
    // Top-level OR is not a form shape.
    rejects("{.a=1 || .b=2}", unsupported);
    // Quoting styles the form never emits.
    rejects("{name='x'}", unsupported);
    rejects("{name=`x`}", unsupported);
    rejects("{\"name\"=\"x\"}", unsupported);
    // Unsupported scope / unknown colon form.
    rejects("{parent.foo=1}", unsupported);
    rejects("{span:bogus=1}", unsupported);
    // Regex on the closed keyword sets.
    rejects("{kind=~\"s.*\"}", unsupported);
    rejects("{status!~\"e.*\"}", unsupported);
    // Group shapes the form never generates.
    rejects("{(span.foo=bar && span.foo=baz)}", unsupported); // = with &&
    rejects("{(span.foo!=bar || span.foo!=baz)}", unsupported); // != with ||
    rejects("{(span.foo=bar || span.baz=qux)}", unsupported); // field differs
    rejects("{(span.foo=bar || span.foo!=baz)}", unsupported); // op differs
    rejects("{(span.foo=bar)}", unsupported); // single-term parens
}

#[test]
fn rejects_malformed() {
    let malformed = |e: &ParseError| matches!(e, ParseError::Malformed { .. });
    rejects("name=\"x\"", malformed); // no braces
    rejects("{name=\"x\"", malformed); // unterminated
    rejects("{name=}", malformed); // missing value
    rejects("{name=\"x}", malformed); // unterminated string
    rejects("{=\"x\"}", malformed); // missing field
    rejects("{name \"x\"}", malformed); // missing operator
    rejects("{avg(duration)>1ms}", malformed); // function-call shape
    rejects("{.a=1 &}", malformed);
}

#[test]
fn rejects_bad_keywords_and_durations() {
    rejects("{status=false}", |e| matches!(e, ParseError::UnknownKeyword { field: "status", .. }));
    rejects("{status=true}", |e| matches!(e, ParseError::UnknownKeyword { field: "status", .. }));
    rejects("{kind=banana}", |e| matches!(e, ParseError::UnknownKeyword { field: "kind", .. }));
    let bad_duration = |e: &ParseError| matches!(e, ParseError::BadDuration { .. });
    rejects("{duration>5}", bad_duration); // missing unit
    rejects("{duration>\"100ms\"}", bad_duration); // quoted literal
    rejects("{traceDuration>5d}", bad_duration); // unknown unit
    rejects("{event:timeSinceStart>ms}", bad_duration); // missing number
}
