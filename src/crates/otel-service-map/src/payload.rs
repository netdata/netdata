//! The `netdata.topology.v1` answer for a window's map.
//!
//! The static part (types, presentation, column declarations, envelope) is
//! `topology_template.json`, a payload with no rows that passes the topology
//! schema and semantic validator; the builder fills in the rows, the string
//! dictionary, the window and the counters. Every column is written with the
//! plain `values` codec, in the template's column order.

use std::collections::{BTreeMap, HashMap};
use std::sync::OnceLock;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use serde_json::{Value, json};

use crate::facts::ServiceKey;
use crate::peer::{PeerKey, PeerKind};
use crate::stats::{Node, SideStats};
use crate::view::MapView;

const TEMPLATE: &str = include_str!("topology_template.json");

fn template() -> &'static Value {
    static TEMPLATE_VALUE: OnceLock<Value> = OnceLock::new();
    TEMPLATE_VALUE
        .get_or_init(|| serde_json::from_str(TEMPLATE).expect("topology template is valid JSON"))
}

/// The answer to an `info` request: the data envelope without `data`.
pub fn info() -> Value {
    let mut out = template().clone();
    if let Some(envelope) = out.as_object_mut() {
        envelope.remove("data");
    }
    out
}

/// The answer for `view` over `[after_s, before_s)` of `tenant`.
pub fn build(view: &MapView, tenant: &str, after_s: u64, before_s: u64, now: SystemTime) -> Value {
    let mut out = template().clone();
    // The map holds whole minutes, so the answer covers the minutes the
    // window touches, and rates divide by that many minutes.
    let (first_minute, last_minute) =
        crate::view::window_minutes(after_s, before_s).unwrap_or((after_s / 60, after_s / 60));
    let window_minutes = (last_minute - first_minute + 1) as f64;
    let mut dict = Dictionary::from_template(&out["data"]["dictionaries"]["strings"]);

    // Actors: every node of a counted edge, in a stable order so the UI keeps
    // positions across refreshes. The overflow bucket is not an actor.
    let mut unique: std::collections::BTreeSet<&Node> = std::collections::BTreeSet::new();
    for edge in view.edges.keys() {
        for node in [&edge.caller, &edge.callee] {
            if !matches!(node, Node::Overflow) {
                unique.insert(node);
            }
        }
    }
    let nodes: Vec<&Node> = unique.into_iter().collect();
    let row_of: HashMap<&Node, usize> = nodes.iter().enumerate().map(|(i, n)| (*n, i)).collect();
    let shared_names = shared_service_names(&nodes);

    let mut links = Columns::default();
    let mut calls_in = vec![0u64; nodes.len()];
    let mut calls_out = vec![0u64; nodes.len()];
    let mut errors_in = vec![0u64; nodes.len()];
    let mut overflow = (0u64, 0u64);
    let mut total_calls = 0u64;
    for (edge, stats) in &view.edges {
        let (Some(&src), Some(&dst)) = (row_of.get(&edge.caller), row_of.get(&edge.callee)) else {
            overflow.0 += stats.client.calls;
            overflow.1 += stats.server.calls;
            continue;
        };
        let primary = if stats.client.calls > 0 {
            &stats.client
        } else {
            &stats.server
        };
        calls_out[src] += primary.calls;
        calls_in[dst] += primary.calls;
        errors_in[dst] += primary.errors;
        total_calls += primary.calls;
        let link_type = match (&edge.caller, &edge.callee) {
            (Node::UnknownCaller, _) => "call_from_unknown",
            (_, Node::Peer(_)) => "call_to_peer",
            _ => "call",
        };
        links.push("type", json!(dict.intern(link_type)));
        links.push("src_actor", json!(src));
        links.push("dst_actor", json!(dst));
        links.push("calls", json!(primary.calls));
        links.push("errors", json!(primary.errors));
        links.push(
            "weight",
            json!(round(primary.calls as f64 / window_minutes, 3)),
        );
        push_side(&mut links, "client", &stats.client);
        push_side(&mut links, "server", &stats.server);
    }

    let mut actors = Columns::default();
    let mut labels = Columns::default();
    let mut services = 0u64;
    let mut peers = 0u64;
    let mut unknown_callers = 0u64;
    for (row, node) in nodes.iter().enumerate() {
        let error_pct_in = pct(errors_in[row], calls_in[row]);
        let mut label = |key: &str, value: &str, source: &str| {
            labels.push("actor", json!(row));
            labels.push("key", json!(dict.intern(key)));
            labels.push("value", json!(dict.intern(value)));
            labels.push("source", json!(dict.intern(source)));
            labels.push("value_index", json!(0));
        };
        let (actor_type, id, display, service, peer): (
            &str,
            String,
            String,
            Option<&ServiceKey>,
            Option<&PeerKey>,
        ) = match node {
            Node::Service(key) => {
                services += 1;
                label("service.name", &key.name, "resource");
                if let Some(ns) = &key.namespace {
                    label("service.namespace", ns, "resource");
                }
                let display = match &key.namespace {
                    Some(ns) if shared_names.contains(key.name.as_ref()) => {
                        format!("{ns}/{}", key.name)
                    }
                    _ => key.name.to_string(),
                };
                let id = format!(
                    "svc:{}:{}",
                    escape(key.namespace.as_deref().unwrap_or("")),
                    escape(&key.name)
                );
                ("service", id, display, Some(key), None)
            }
            Node::Peer(key) => {
                peers += 1;
                label("peer.kind", peer_kind(key.kind), "service_map");
                label("peer.name", &key.name, "service_map");
                let id = format!("peer:{}:{}", peer_kind(key.kind), escape(&key.name));
                (
                    peer_type(key.kind),
                    id,
                    key.name.to_string(),
                    None,
                    Some(key),
                )
            }
            Node::UnknownCaller => {
                unknown_callers += 1;
                (
                    "unknown_caller",
                    "unknown_caller".to_string(),
                    "unknown caller".to_string(),
                    None,
                    None,
                )
            }
            Node::Overflow => unreachable!("overflow is not an actor"),
        };
        if !matches!(node, Node::UnknownCaller) {
            label("calls_in", &calls_in[row].to_string(), "service_map");
            let pct_text = error_pct_in.map_or_else(|| "n/a".to_string(), |p| format!("{p:.2}%"));
            label("error_pct_in", &pct_text, "service_map");
        }
        if matches!(node, Node::Service(_) | Node::UnknownCaller) {
            label("calls_out", &calls_out[row].to_string(), "service_map");
        }

        actors.push("type", json!(dict.intern(actor_type)));
        actors.push("layer", json!(dict.intern("service")));
        actors.push("id", json!(id));
        actors.push("display_name", json!(display));
        actors.push(
            "service_namespace",
            json!(service.and_then(|s| s.namespace.as_deref())),
        );
        actors.push("service_name", json!(service.map(|s| s.name.as_ref())));
        actors.push(
            "peer_kind",
            json!(peer.map(|p| dict.intern(peer_kind(p.kind)))),
        );
        actors.push("peer_name", json!(peer.map(|p| p.name.as_ref())));
        actors.push("calls_in", json!(calls_in[row]));
        actors.push("calls_out", json!(calls_out[row]));
        actors.push("errors_in", json!(errors_in[row]));
        actors.push("error_pct_in", json!(error_pct_in));
    }

    let data = &mut out["data"];
    data["collected_at"] = json!(rfc3339(now));
    data["valid_after"] = json!(rfc3339(
        UNIX_EPOCH + Duration::from_secs(first_minute.saturating_mul(60))
    ));
    data["valid_until"] = json!(rfc3339(
        UNIX_EPOCH + Duration::from_secs((last_minute + 1).saturating_mul(60))
    ));
    data["producer"]["instance"] = json!(tenant);
    actors.fill(&mut data["actors"], nodes.len());
    links.fill(&mut data["links"], view.edges.len() - overflow_edges(view));
    let label_rows = labels.rows();
    labels.fill(
        &mut data["tables"]["actor"]["actor_labels"]["table"],
        label_rows,
    );
    data["dictionaries"]["strings"] = json!(dict.strings);

    let losses = &view.losses;
    let stats = &mut data["stats"];
    stats["services"] = json!(services);
    stats["virtual_peers"] = json!(peers);
    stats["unknown_callers"] = json!(unknown_callers);
    stats["links"] = json!(view.edges.len() - overflow_edges(view));
    stats["calls"] = json!(total_calls);
    stats["loss_handoff_drops"] = json!(losses.handoff_drops);
    stats["loss_capacity_drops"] = json!(losses.capacity_drops);
    stats["loss_late"] = json!(losses.late);
    stats["loss_future"] = json!(losses.future);
    stats["loss_duplicates"] = json!(losses.duplicates);
    stats["loss_invalid"] = json!(losses.invalid);
    stats["loss_unpaired_servers"] = json!(losses.unpaired_servers);
    stats["loss_shutdown_lost"] = json!(losses.shutdown_lost);
    stats["loss_unwritten_minutes"] = json!(losses.unwritten_minutes);
    stats["loss_unreadable_files"] = json!(view.unreadable_files);
    stats["overflow_client_calls"] = json!(overflow.0);
    stats["overflow_server_calls"] = json!(overflow.1);
    out
}

fn overflow_edges(view: &MapView) -> usize {
    view.edges
        .keys()
        .filter(|e| matches!(e.caller, Node::Overflow) || matches!(e.callee, Node::Overflow))
        .count()
}

fn push_side(links: &mut Columns, side: &str, stats: &SideStats) {
    // A side with no calls is absent, not zero: the other half was never seen.
    let present = stats.calls > 0;
    let ms = |q: f64| {
        present
            .then(|| stats.latency_us.quantile(q))
            .flatten()
            .map(|us| round(us.min(stats.max_us) as f64 / 1_000.0, 3))
    };
    links.push(
        &format!("{side}_calls"),
        json!(present.then_some(stats.calls)),
    );
    links.push(
        &format!("{side}_errors"),
        json!(present.then_some(stats.errors)),
    );
    links.push(
        &format!("{side}_error_pct"),
        json!(pct(stats.errors, stats.calls)),
    );
    links.push(&format!("{side}_p50_ms"), json!(ms(0.50)));
    links.push(&format!("{side}_p95_ms"), json!(ms(0.95)));
    links.push(&format!("{side}_p99_ms"), json!(ms(0.99)));
    links.push(
        &format!("{side}_max_ms"),
        json!(present.then(|| round(stats.max_us as f64 / 1_000.0, 3))),
    );
}

/// Column values by column id, written in the template's column order.
#[derive(Default)]
struct Columns {
    values: HashMap<String, Vec<Value>>,
}

impl Columns {
    fn push(&mut self, column: &str, value: Value) {
        self.values
            .entry(column.to_string())
            .or_default()
            .push(value);
    }

    fn rows(&self) -> usize {
        self.values.values().map(Vec::len).max().unwrap_or(0)
    }

    fn fill(mut self, table: &mut Value, rows: usize) {
        let mut encoded = Vec::new();
        if let Some(columns) = table["columns"].as_array() {
            for column in columns {
                let id = column["id"].as_str().unwrap_or_default();
                let values = self.values.remove(id).unwrap_or_default();
                debug_assert_eq!(values.len(), rows, "column {id}");
                encoded.push(json!({"codec": "values", "values": values}));
            }
        }
        table["rows"] = json!(rows);
        table["values"] = Value::Array(encoded);
    }
}

/// The payload's one string dictionary, seeded with the template's entries
/// (the type ids its declarations already reference).
struct Dictionary {
    strings: Vec<String>,
    index: HashMap<String, usize>,
}

impl Dictionary {
    fn from_template(seed: &Value) -> Self {
        let mut dict = Self {
            strings: Vec::new(),
            index: HashMap::new(),
        };
        if let Some(seed) = seed.as_array() {
            for s in seed {
                if let Some(s) = s.as_str() {
                    dict.intern(s);
                }
            }
        }
        dict
    }

    fn intern(&mut self, s: &str) -> usize {
        if let Some(&i) = self.index.get(s) {
            return i;
        }
        self.strings.push(s.to_string());
        self.index.insert(s.to_string(), self.strings.len() - 1);
        self.strings.len() - 1
    }
}

/// Service names carried by more than one namespace: those display as
/// `namespace/name`.
fn shared_service_names<'a>(nodes: &[&'a Node]) -> std::collections::HashSet<&'a str> {
    let mut namespaces: BTreeMap<&str, usize> = BTreeMap::new();
    for node in nodes {
        if let Node::Service(key) = node {
            *namespaces.entry(key.name.as_ref()).or_insert(0) += 1;
        }
    }
    namespaces
        .into_iter()
        .filter(|(_, n)| *n > 1)
        .map(|(name, _)| name)
        .collect()
}

fn peer_type(kind: PeerKind) -> &'static str {
    match kind {
        PeerKind::Database => "peer_database",
        PeerKind::Messaging => "peer_messaging",
        PeerKind::Rpc | PeerKind::Http => "peer_external",
        PeerKind::Other => "peer_other",
    }
}

fn peer_kind(kind: PeerKind) -> &'static str {
    match kind {
        PeerKind::Database => "database",
        PeerKind::Messaging => "messaging",
        PeerKind::Rpc => "rpc",
        PeerKind::Http => "http",
        PeerKind::Other => "other",
    }
}

/// Keeps `:` as the id separator unambiguous.
fn escape(part: &str) -> String {
    part.replace('%', "%25").replace(':', "%3A")
}

fn pct(part: u64, whole: u64) -> Option<f64> {
    (whole > 0).then(|| round(100.0 * part as f64 / whole as f64, 2))
}

fn round(value: f64, decimals: i32) -> f64 {
    let factor = 10f64.powi(decimals);
    (value * factor).round() / factor
}

/// Infallible: humantime cannot format years past 9999, so later times clamp.
fn rfc3339(t: SystemTime) -> String {
    const LAST_FORMATTABLE_S: u64 = 253_402_300_799;
    let t = t.min(UNIX_EPOCH + Duration::from_secs(LAST_FORMATTABLE_S));
    humantime::format_rfc3339_seconds(t).to_string()
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::stats::{Edge, EdgeStats, LossCounters};

    fn service(name: &str) -> Node {
        Node::Service(ServiceKey {
            namespace: Some(Arc::from("demo")),
            name: Arc::from(name),
        })
    }

    fn side(calls: u64, errors: u64, us: u64) -> SideStats {
        let mut s = SideStats::default();
        for i in 0..calls {
            s.record(Some(us), i < errors);
        }
        s
    }

    fn sample() -> MapView {
        let mut view = MapView::default();
        let mut add = |caller: Node, callee: Node, client: SideStats, server: SideStats| {
            view.edges
                .insert(Edge { caller, callee }, EdgeStats { client, server });
        };
        add(
            service("frontend"),
            service("checkout"),
            side(10, 1, 9_000),
            side(10, 0, 4_000),
        );
        add(
            service("cart"),
            Node::Peer(PeerKey {
                kind: PeerKind::Database,
                name: Arc::from("valkey-cart"),
                foldable: false,
            }),
            side(4, 0, 300),
            SideStats::default(),
        );
        add(
            Node::UnknownCaller,
            service("frontend"),
            SideStats::default(),
            side(6, 3, 20_000),
        );
        add(Node::Overflow, Node::Overflow, side(2, 0, 1), side(1, 0, 1));
        view.losses = LossCounters {
            late: 3,
            ..LossCounters::default()
        };
        view.unreadable_files = 1;
        view
    }

    fn column<'a>(table: &'a Value, id: &str) -> &'a Vec<Value> {
        let columns = table["columns"].as_array().unwrap();
        let i = columns.iter().position(|c| c["id"] == id).unwrap();
        table["values"][i]["values"].as_array().unwrap()
    }

    fn strings(out: &Value, column: &[Value]) -> Vec<String> {
        let dict = out["data"]["dictionaries"]["strings"].as_array().unwrap();
        column
            .iter()
            .map(|v| {
                dict[v.as_u64().unwrap() as usize]
                    .as_str()
                    .unwrap()
                    .to_string()
            })
            .collect()
    }

    #[test]
    fn rows_counts_and_sides() {
        let now = UNIX_EPOCH + Duration::from_secs(1_000_000);
        let out = build(&sample(), "default", 999_400, 1_000_000, now);
        let data = &out["data"];
        assert_eq!(out["type"], "topology");
        assert_eq!(data["producer"]["instance"], "default");
        assert_eq!(
            data["valid_after"], "1970-01-12T13:36:00Z",
            "minute aligned"
        );
        assert_eq!(data["valid_until"], "1970-01-12T13:47:00Z");

        let actors = &data["actors"];
        assert_eq!(actors["rows"], 5);
        let names: Vec<&str> = column(actors, "display_name")
            .iter()
            .map(|v| v.as_str().unwrap())
            .collect();
        assert_eq!(
            names,
            vec![
                "cart",
                "checkout",
                "frontend",
                "valkey-cart",
                "unknown caller"
            ]
        );
        assert_eq!(
            strings(&out, column(actors, "type")),
            vec![
                "service",
                "service",
                "service",
                "peer_database",
                "unknown_caller"
            ]
        );
        assert_eq!(column(actors, "id")[0], "svc:demo:cart");
        assert_eq!(
            column(actors, "calls_in")[2],
            6,
            "frontend is called by the unknown caller"
        );
        assert_eq!(column(actors, "error_pct_in")[2], 50.0);
        assert_eq!(
            column(actors, "error_pct_in")[0],
            Value::Null,
            "nobody calls cart"
        );

        let links = &data["links"];
        assert_eq!(links["rows"], 3, "the overflow bucket is not a link");
        let types = strings(&out, column(links, "type"));
        let first = types.iter().position(|t| t == "call").unwrap();
        assert_eq!(column(links, "calls")[first], 10);
        assert_eq!(
            column(links, "weight")[first],
            0.909,
            "10 calls over the 11 minutes the window touches"
        );
        assert_eq!(column(links, "client_errors")[first], 1);
        assert_eq!(column(links, "server_error_pct")[first], 0.0);
        let p95 = column(links, "client_p95_ms")[first].as_f64().unwrap();
        assert!((p95 - 9.0).abs() < 0.1);
        let peer = types.iter().position(|t| t == "call_to_peer").unwrap();
        assert_eq!(
            column(links, "server_calls")[peer],
            Value::Null,
            "a peer has no server side"
        );
        let unknown = types.iter().position(|t| t == "call_from_unknown").unwrap();
        assert_eq!(column(links, "client_calls")[unknown], Value::Null);
        assert_eq!(
            column(links, "calls")[unknown],
            6,
            "the primary side falls back to the server side"
        );

        let stats = &data["stats"];
        assert_eq!(stats["services"], 3);
        assert_eq!(stats["links"], 3);
        assert_eq!(stats["loss_late"], 3);
        assert_eq!(stats["loss_unreadable_files"], 1);
        assert_eq!(
            (
                stats["overflow_client_calls"].clone(),
                stats["overflow_server_calls"].clone()
            ),
            (json!(2), json!(1))
        );

        // Every column of every table has one value per row.
        for table in [
            &data["actors"],
            &data["links"],
            &data["tables"]["actor"]["actor_labels"]["table"],
        ] {
            let rows = table["rows"].as_u64().unwrap() as usize;
            for v in table["values"].as_array().unwrap() {
                assert_eq!(v["values"].as_array().unwrap().len(), rows);
            }
        }
    }

    #[test]
    fn empty_view_keeps_every_column() {
        let out = build(&MapView::default(), "default", 0, 900, UNIX_EPOCH);
        assert_eq!(out["data"]["actors"]["rows"], 0);
        assert_eq!(out["data"]["links"]["rows"], 0);
        let columns = out["data"]["links"]["columns"].as_array().unwrap().len();
        assert_eq!(
            out["data"]["links"]["values"].as_array().unwrap().len(),
            columns
        );
    }

    #[test]
    fn ids_escape_the_separator_and_shared_names_get_their_namespace() {
        let mut view = MapView::default();
        let key = |ns: &str, name: &str| {
            Node::Service(ServiceKey {
                namespace: Some(Arc::from(ns)),
                name: Arc::from(name),
            })
        };
        view.edges.insert(
            Edge {
                caller: key("eu", "cart"),
                callee: key("us", "cart"),
            },
            EdgeStats {
                client: side(1, 0, 1),
                server: side(1, 0, 1),
            },
        );
        view.edges.insert(
            Edge {
                caller: key("a:b", "x"),
                callee: key("us", "cart"),
            },
            EdgeStats {
                client: side(1, 0, 1),
                server: SideStats::default(),
            },
        );
        let out = build(&view, "default", 0, 60, UNIX_EPOCH);
        let actors = &out["data"]["actors"];
        let ids: Vec<&str> = column(actors, "id")
            .iter()
            .map(|v| v.as_str().unwrap())
            .collect();
        assert_eq!(ids, vec!["svc:a%3Ab:x", "svc:eu:cart", "svc:us:cart"]);
        let names: Vec<&str> = column(actors, "display_name")
            .iter()
            .map(|v| v.as_str().unwrap())
            .collect();
        assert_eq!(names, vec!["x", "eu/cart", "us/cart"]);
    }

    /// Every actor and link type, for the cross-producer fixture that the Go
    /// topology validator checks (`TestTopologyV1FixturesValidate`).
    fn fixture_view() -> MapView {
        let mut view = sample();
        let peer = |kind: PeerKind, name: &str| {
            Node::Peer(PeerKey {
                kind,
                name: Arc::from(name),
                foldable: false,
            })
        };
        let mut add = |caller: Node, callee: Node, client: SideStats| {
            view.edges.insert(
                Edge { caller, callee },
                EdgeStats {
                    client,
                    server: SideStats::default(),
                },
            );
        };
        add(
            service("checkout"),
            peer(PeerKind::Messaging, "orders"),
            side(5, 0, 2_000),
        );
        add(
            service("checkout"),
            peer(PeerKind::Rpc, "oteldemo.PaymentService"),
            side(5, 1, 30_000),
        );
        add(
            service("frontend"),
            peer(PeerKind::Http, "api.example.com"),
            side(3, 3, 150_000),
        );
        add(
            service("frontend"),
            peer(PeerKind::Other, "unknown"),
            side(1, 0, 10),
        );
        view
    }

    #[test]
    fn golden_fixture_matches() {
        let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../../go/tools/functions-validation/fixtures/topology-v1/tracing.json");
        let now = UNIX_EPOCH + Duration::from_secs(1_000_000);
        let out = build(&fixture_view(), "default", 999_400, 1_000_000, now);
        let pretty = serde_json::to_string_pretty(&out).unwrap() + "\n";
        if std::env::var_os("UPDATE_GOLDEN").is_some() {
            std::fs::write(&path, &pretty).unwrap();
        }
        // Compared as JSON values: key order depends on whether another crate
        // in the build turns on serde_json's `preserve_order`.
        let stored: Value = std::fs::read_to_string(&path)
            .ok()
            .and_then(|s| serde_json::from_str(&s).ok())
            .unwrap_or(Value::Null);
        assert!(
            stored == out,
            "{} is stale: rerun with UPDATE_GOLDEN=1",
            path.display()
        );
    }

    #[test]
    fn timestamps_never_fail() {
        assert_eq!(rfc3339(UNIX_EPOCH), "1970-01-01T00:00:00Z");
        let far = UNIX_EPOCH + Duration::from_secs(300_000_000_000);
        assert_eq!(rfc3339(far), "9999-12-31T23:59:59Z");
    }

    #[test]
    fn info_has_no_data() {
        let out = info();
        assert!(out.get("data").is_none());
        assert_eq!(out["v"], 3);
        assert_eq!(
            out["accepted_params"],
            json!(["info", "after", "before", "tenant"])
        );
    }
}
