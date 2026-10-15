use std::collections::HashMap;
use std::sync::Arc;

use crate::engine::{Engine, Limits};
use crate::facts::{Kind, ServiceKey, SpanFacts};
use crate::peer::{PeerKey, PeerKind};
use crate::stats::{Edge, EdgeStats, LossCounters, NS_PER_MINUTE, Node};

const SEC: u64 = 1_000_000_000;
/// Minute-aligned start, far from zero.
const T0: u64 = 1_000 * NS_PER_MINUTE;
const TENANT: &str = "default";

fn service(name: &str) -> ServiceKey {
    ServiceKey {
        namespace: None,
        name: Arc::from(name),
    }
}

fn peer(name: &str) -> PeerKey {
    PeerKey {
        kind: PeerKind::Database,
        name: Arc::from(name),
        foldable: false,
    }
}

fn span(kind: Kind, svc: &str, id: u8, parent: Option<u8>) -> SpanFacts {
    SpanFacts {
        trace_id: [7; 16],
        span_id: [id; 8],
        parent_span_id: parent.map(|p| [p; 8]),
        kind,
        service: service(svc),
        peer: kind.is_caller().then(|| peer("redis")),
        end_ns: T0 + 10 * SEC,
        duration_us: Some(1_000 * u64::from(id)),
        error: false,
    }
}

fn svc_edge(caller: &str, callee: &str) -> Edge {
    Edge {
        caller: Node::Service(service(caller)),
        callee: Node::Service(service(callee)),
    }
}

/// Feeds `spans` one per second from T0 + 10 s, waits out every pairing
/// window and returns all counted edges and losses.
fn run(spans: Vec<SpanFacts>) -> (HashMap<Edge, EdgeStats>, LossCounters, Engine) {
    let mut engine = Engine::new(Limits::default());
    let mut now = T0 + 10 * SEC;
    for s in spans {
        engine.ingest(TENANT, vec![s], 0, now);
        now += SEC;
    }
    engine.expire(now + 60 * SEC);
    let (edges, losses) = collect(&mut engine, now + 3_600 * SEC);
    (edges, losses, engine)
}

fn collect(engine: &mut Engine, now: u64) -> (HashMap<Edge, EdgeStats>, LossCounters) {
    let mut edges: HashMap<Edge, EdgeStats> = HashMap::new();
    let mut losses = LossCounters::default();
    for closed in engine.take_closed(now) {
        for (edge, stats) in &closed.stats.edges {
            edges.entry(edge.clone()).or_default().merge(stats);
        }
        losses.merge(&closed.stats.losses);
    }
    (edges, losses)
}

fn calls(edges: &HashMap<Edge, EdgeStats>) -> HashMap<Edge, (u64, u64)> {
    let mut out = HashMap::new();
    for (edge, stats) in edges {
        out.insert(edge.clone(), (stats.client.calls, stats.server.calls));
    }
    out
}

#[test]
fn pairs_in_either_arrival_order() {
    let client = span(Kind::Client, "frontend", 1, None);
    let server = span(Kind::Server, "checkout", 2, Some(1));
    let cases: HashMap<&str, Vec<SpanFacts>> = HashMap::from([
        ("client first", vec![client.clone(), server.clone()]),
        ("server first", vec![server.clone(), client.clone()]),
    ]);
    for (name, spans) in cases {
        let (edges, losses, engine) = run(spans);
        let expected = HashMap::from([(svc_edge("frontend", "checkout"), (1, 1))]);
        assert_eq!(calls(&edges), expected, "{name}");
        assert_eq!(losses, LossCounters::default(), "{name}");
        assert_eq!(engine.in_flight(), 0, "{name}");
    }
}

#[test]
fn sides_keep_their_own_latency_and_errors() {
    let mut client = span(Kind::Client, "frontend", 1, None);
    client.duration_us = Some(9_000);
    client.error = true;
    let mut server = span(Kind::Server, "checkout", 2, Some(1));
    server.duration_us = Some(4_000);
    let (edges, _, _) = run(vec![client, server]);
    let stats = &edges[&svc_edge("frontend", "checkout")];
    assert_eq!((stats.client.errors, stats.client.max_us), (1, 9_000));
    assert_eq!((stats.server.errors, stats.server.max_us), (0, 4_000));
    let p50 = stats.client.latency_us.quantile(0.5).unwrap();
    assert!(p50.abs_diff(9_000) <= 90, "p50 {p50} within 1% of 9000");
}

#[test]
fn root_callee_comes_from_the_unknown_caller() {
    let (edges, losses, _) = run(vec![span(Kind::Server, "frontend-proxy", 1, None)]);
    let expected = HashMap::from([(
        Edge {
            caller: Node::UnknownCaller,
            callee: Node::Service(service("frontend-proxy")),
        },
        (0, 1),
    )]);
    assert_eq!(calls(&edges), expected);
    assert_eq!(losses.unpaired_servers, 0, "a root is not an unpaired half");
}

#[test]
fn unpaired_halves_expire_to_a_peer_and_to_the_unknown_caller() {
    let (edges, losses, engine) = run(vec![
        span(Kind::Client, "cart", 1, None),
        span(Kind::Server, "flagd", 2, Some(9)),
    ]);
    let expected = HashMap::from([
        (
            Edge {
                caller: Node::Service(service("cart")),
                callee: Node::Peer(peer("redis")),
            },
            (1, 0),
        ),
        (
            Edge {
                caller: Node::UnknownCaller,
                callee: Node::Service(service("flagd")),
            },
            (0, 1),
        ),
    ]);
    assert_eq!(calls(&edges), expected);
    assert_eq!(losses.unpaired_servers, 1);
    assert_eq!(engine.in_flight(), 0);
}

#[test]
fn only_the_innermost_client_span_is_a_call() {
    let outer = span(Kind::Client, "cart", 1, None);
    let inner = span(Kind::Client, "cart", 2, Some(1));
    let server = span(Kind::Server, "flagd", 3, Some(2));
    let orders: HashMap<&str, Vec<SpanFacts>> = HashMap::from([
        (
            "outer first",
            vec![outer.clone(), inner.clone(), server.clone()],
        ),
        (
            "inner first",
            vec![inner.clone(), outer.clone(), server.clone()],
        ),
        (
            "server first",
            vec![server.clone(), inner.clone(), outer.clone()],
        ),
    ]);
    for (name, spans) in orders {
        let (edges, _, engine) = run(spans);
        let expected = HashMap::from([(svc_edge("cart", "flagd"), (1, 1))]);
        assert_eq!(calls(&edges), expected, "{name}");
        assert_eq!(engine.in_flight(), 0, "{name}");
    }

    let (edges, _, _) = run(vec![inner, outer]);
    let expected = HashMap::from([(
        Edge {
            caller: Node::Service(service("cart")),
            callee: Node::Peer(peer("redis")),
        },
        (1, 0),
    )]);
    assert_eq!(
        calls(&edges),
        expected,
        "an unanswered inner client is the one peer call"
    );
}

#[test]
fn nested_server_and_internal_spans_are_not_calls() {
    let root = span(Kind::Server, "flagd", 1, None);
    let nested = span(Kind::Server, "flagd", 2, Some(1));
    let internal = span(Kind::Internal, "flagd", 3, Some(1));
    let under_internal = span(Kind::Server, "flagd", 4, Some(3));
    let root_only = HashMap::from([(
        Edge {
            caller: Node::UnknownCaller,
            callee: Node::Service(service("flagd")),
        },
        (0, 1),
    )]);
    let orders: HashMap<&str, Vec<SpanFacts>> = HashMap::from([
        (
            "parents first",
            vec![
                root.clone(),
                nested.clone(),
                internal.clone(),
                under_internal.clone(),
            ],
        ),
        (
            "children first",
            vec![under_internal, nested, internal, root],
        ),
    ]);
    for (name, spans) in orders {
        let (edges, losses, _) = run(spans);
        assert_eq!(calls(&edges), root_only, "{name}");
        assert_eq!(losses.unpaired_servers, 0, "{name}");
    }
}

#[test]
fn self_calls_are_neither_drawn_nor_peer_calls() {
    let (edges, _, _) = run(vec![
        span(Kind::Client, "frontend", 1, None),
        span(Kind::Server, "frontend", 2, Some(1)),
    ]);
    assert_eq!(calls(&edges), HashMap::new());
}

#[test]
fn a_second_answer_to_one_client_adds_only_its_server_side() {
    let (edges, _, _) = run(vec![
        span(Kind::Client, "frontend", 1, None),
        span(Kind::Server, "checkout", 2, Some(1)),
        span(Kind::Server, "checkout", 3, Some(1)),
    ]);
    assert_eq!(
        calls(&edges),
        HashMap::from([(svc_edge("frontend", "checkout"), (1, 2))])
    );
}

#[test]
fn producer_and_consumer_pair_like_client_and_server() {
    let (edges, _, _) = run(vec![
        span(Kind::Consumer, "accounting", 2, Some(1)),
        span(Kind::Producer, "checkout", 1, None),
    ]);
    assert_eq!(
        calls(&edges),
        HashMap::from([(svc_edge("checkout", "accounting"), (1, 1))])
    );
}

#[test]
fn tenants_do_not_pair_with_each_other() {
    let mut engine = Engine::new(Limits::default());
    let now = T0 + 10 * SEC;
    engine.ingest("a", vec![span(Kind::Client, "frontend", 1, None)], 0, now);
    engine.ingest(
        "b",
        vec![span(Kind::Server, "checkout", 2, Some(1))],
        0,
        now,
    );
    engine.expire(now + 60 * SEC);
    let mut by_tenant: HashMap<String, Vec<Edge>> = HashMap::new();
    for closed in engine.take_closed(now + 3_600 * SEC) {
        for edge in closed.stats.edges.keys() {
            by_tenant
                .entry(closed.tenant.to_string())
                .or_default()
                .push(edge.clone());
        }
    }
    assert_eq!(by_tenant["a"].len(), 1);
    assert_eq!(by_tenant["a"][0].callee, Node::Peer(peer("redis")));
    assert_eq!(by_tenant["b"][0].caller, Node::UnknownCaller);
}

#[test]
fn losses_are_counted() {
    let limits = Limits {
        max_in_flight: 2,
        ..Limits::default()
    };
    let mut engine = Engine::new(limits);
    let now = T0 + 10 * SEC;
    let mut future = span(Kind::Client, "a", 9, None);
    future.end_ns = now + 3 * NS_PER_MINUTE;
    engine.ingest(
        TENANT,
        vec![
            span(Kind::Client, "a", 1, None),
            span(Kind::Client, "a", 1, None),
            span(Kind::Client, "a", 2, None),
            span(Kind::Client, "a", 3, None),
            future,
        ],
        4,
        now,
    );
    engine.note_handoff_drops(TENANT, 5, now);
    engine.expire(now + 60 * SEC);
    let (_, losses) = collect(&mut engine, now + 3_600 * SEC);
    let expected = LossCounters {
        handoff_drops: 5,
        capacity_drops: 1,
        future: 1,
        duplicates: 1,
        invalid: 4,
        ..LossCounters::default()
    };
    assert_eq!(losses, expected);
}

#[test]
fn calls_into_a_closed_minute_are_late() {
    let mut engine = Engine::new(Limits::default());
    let now = T0 + 10 * SEC;
    engine.ingest(
        TENANT,
        vec![span(Kind::Server, "checkout", 2, Some(1))],
        0,
        now,
    );
    // The minute of T0 closes once it ended `horizon` ago.
    let closed = engine.take_closed(T0 + NS_PER_MINUTE + Limits::default().horizon_ns);
    assert!(
        closed.is_empty(),
        "nothing counted yet: the server half is waiting"
    );
    engine.ingest(
        TENANT,
        vec![span(Kind::Client, "frontend", 1, None)],
        0,
        now + 200 * SEC,
    );
    let (edges, losses) = collect(&mut engine, now + 3_600 * SEC);
    assert!(edges.is_empty());
    assert_eq!(losses.late, 1);
}

#[test]
fn a_minute_closes_exactly_horizon_after_it_ends() {
    let mut engine = Engine::new(Limits::default());
    engine.ingest(
        TENANT,
        vec![span(Kind::Server, "a", 1, None)],
        0,
        T0 + 10 * SEC,
    );
    let close_at = T0 + NS_PER_MINUTE + Limits::default().horizon_ns;
    assert!(
        engine
            .take_closed(close_at - 1)
            .iter()
            .all(|c| c.minute != T0 / NS_PER_MINUTE)
    );
    let closed = engine.take_closed(close_at);
    assert!(closed.iter().any(|c| c.minute == T0 / NS_PER_MINUTE));
}

#[test]
fn edges_beyond_the_cap_go_to_overflow() {
    let limits = Limits {
        max_edges_per_minute: 1,
        ..Limits::default()
    };
    let mut engine = Engine::new(limits);
    let now = T0 + 10 * SEC;
    engine.ingest(
        TENANT,
        vec![
            span(Kind::Server, "a", 1, None),
            span(Kind::Server, "b", 2, None),
        ],
        0,
        now,
    );
    let (edges, _) = collect(&mut engine, now + 3_600 * SEC);
    let overflow = Edge {
        caller: Node::Overflow,
        callee: Node::Overflow,
    };
    assert_eq!(edges.len(), 2);
    assert_eq!(edges[&overflow].server.calls, 1);
}

#[test]
fn open_minutes_are_visible_before_they_close() {
    let mut engine = Engine::new(Limits::default());
    engine.ingest(
        TENANT,
        vec![span(Kind::Server, "a", 1, None)],
        0,
        T0 + 10 * SEC,
    );
    let minute = T0 / NS_PER_MINUTE;
    let open = engine.open_minutes(TENANT, minute, minute);
    assert_eq!(open.len(), 1);
    assert_eq!(open[0].1.edges.len(), 1);
    assert!(engine.open_minutes("other", minute, minute).is_empty());
}

#[test]
fn shutdown_hands_out_everything_and_counts_waiting_spans() {
    let mut engine = Engine::new(Limits::default());
    let now = T0 + 10 * SEC;
    engine.ingest(
        TENANT,
        vec![
            span(Kind::Client, "frontend", 1, None),
            span(Kind::Server, "flagd", 3, Some(8)),
            span(Kind::Server, "a", 4, None),
        ],
        0,
        now,
    );
    let mut losses = LossCounters::default();
    let mut edges = 0;
    for closed in engine.shutdown(now + SEC) {
        losses.merge(&closed.stats.losses);
        edges += closed.stats.edges.len();
    }
    assert_eq!(losses.shutdown_lost, 2);
    assert_eq!(edges, 1, "the root call was already counted");
    assert_eq!(engine.in_flight(), 0);
}

#[test]
fn a_clock_corrected_backwards_reopens_minutes() {
    let mut engine = Engine::new(Limits::default());
    let now = T0 + 10 * SEC;
    engine.ingest(TENANT, vec![span(Kind::Server, "a", 1, None)], 0, now);
    // The clock jumps ten hours ahead, then is corrected.
    engine.take_closed(now + 36_000 * SEC);
    engine.take_closed(now + 30 * SEC);
    let mut late_span = span(Kind::Server, "a", 2, None);
    late_span.end_ns = now + 20 * SEC;
    engine.ingest(TENANT, vec![late_span], 0, now + 30 * SEC);
    let (edges, losses) = collect(&mut engine, now + 3_600 * SEC);
    assert_eq!(losses.late, 0);
    assert_eq!(edges.values().map(|e| e.server.calls).sum::<u64>(), 1);
}

#[test]
fn a_new_tenant_does_not_accept_spans_older_than_the_horizon() {
    let mut engine = Engine::new(Limits::default());
    let now = T0 + 10 * SEC;
    let mut old = span(Kind::Server, "a", 1, None);
    old.end_ns = now - 3_600 * SEC;
    engine.ingest("fresh", vec![old], 0, now);
    let (edges, losses) = collect(&mut engine, now + 3_600 * SEC);
    assert!(edges.is_empty());
    assert_eq!(losses.late, 1);
}

#[test]
fn a_client_nested_through_an_internal_span_is_one_call() {
    let outer = span(Kind::Client, "cart", 1, None);
    let internal = span(Kind::Internal, "cart", 2, Some(1));
    let inner = span(Kind::Client, "cart", 3, Some(2));
    let orders: HashMap<&str, Vec<SpanFacts>> = HashMap::from([
        (
            "outer first",
            vec![outer.clone(), internal.clone(), inner.clone()],
        ),
        (
            "inner first",
            vec![inner.clone(), internal.clone(), outer.clone()],
        ),
        (
            "internal first",
            vec![internal.clone(), inner.clone(), outer.clone()],
        ),
        (
            "inner, outer, internal",
            vec![inner.clone(), outer.clone(), internal.clone()],
        ),
    ]);
    for (name, spans) in orders {
        let (edges, _, engine) = run(spans);
        let expected = HashMap::from([(
            Edge {
                caller: Node::Service(service("cart")),
                callee: Node::Peer(peer("redis")),
            },
            (1, 0),
        )]);
        assert_eq!(calls(&edges), expected, "{name}");
        assert_eq!(engine.in_flight(), 0, "{name}");
    }
}

/// Random arrival orders, duplicates, self-calls, resends after expiry,
/// capacity limits and backward clock steps: the pairing store's size never
/// drifts, stays near its cap, and drains to zero.
#[test]
fn in_flight_accounting_never_drifts() {
    let mut seed = 0x9E37_79B9_7F4A_7C15u64;
    let mut next = move |n: u64| {
        seed ^= seed << 13;
        seed ^= seed >> 7;
        seed ^= seed << 17;
        seed % n
    };
    for run in 0..300 {
        let cap = 1 + next(30) as usize;
        let mut engine = Engine::new(Limits {
            max_in_flight: cap,
            ..Limits::default()
        });
        let mut now = T0 + 10 * SEC;
        let mut latest = now;
        for _ in 0..300 {
            match next(10) {
                0 => engine.expire(now),
                1 => {
                    engine.take_closed(now);
                }
                2 => now = now.saturating_sub(next(40) * SEC),
                _ => {
                    let kind = [
                        Kind::Server,
                        Kind::Client,
                        Kind::Internal,
                        Kind::Producer,
                        Kind::Consumer,
                    ][next(5) as usize];
                    let id = 1 + next(12) as u8;
                    let parent = (next(4) != 0).then(|| 1 + next(12) as u8);
                    let svc = ["a", "b", "c"][next(3) as usize];
                    let mut s = span(kind, svc, id, parent);
                    s.trace_id = [next(3) as u8; 16];
                    s.end_ns = now;
                    engine.ingest(TENANT, vec![s], 0, now);
                }
            }
            assert!(
                engine.in_flight() <= cap + 3,
                "run {run}: {} over cap {cap}",
                engine.in_flight()
            );
            now += next(5) * SEC;
            latest = latest.max(now);
        }
        engine.expire(latest + 60 * SEC);
        assert_eq!(engine.in_flight(), 0, "run {run}");
    }
}
