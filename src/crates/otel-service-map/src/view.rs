//! A window's map: stored and open minutes merged into one set of edges, with
//! peers that name a service seen in the window folded into that service.

use std::collections::{BTreeMap, HashMap};

use crate::facts::ServiceKey;
use crate::stats::{Edge, EdgeStats, LossCounters, MinuteStats, NS_PER_MINUTE, Node};
use crate::store::Loaded;

const SECONDS_PER_MINUTE: u64 = NS_PER_MINUTE / 1_000_000_000;

#[derive(Debug, Default, PartialEq)]
pub struct MapView {
    pub edges: BTreeMap<Edge, EdgeStats>,
    pub losses: LossCounters,
    /// Stored hour files that could not be read for this window.
    pub unreadable_files: u64,
}

/// The minutes overlapping `[after_s, before_s)`, as an inclusive range.
/// Partial minutes at either end are included whole.
pub fn window_minutes(after_s: u64, before_s: u64) -> Option<(u64, u64)> {
    if before_s <= after_s {
        return None;
    }
    Some((
        after_s / SECONDS_PER_MINUTE,
        (before_s - 1) / SECONDS_PER_MINUTE,
    ))
}

pub fn build(stored: Loaded, open: Vec<(u64, MinuteStats)>) -> MapView {
    let mut merged = MinuteStats::default();
    for stats in stored.minutes.values() {
        merged.merge(stats);
    }
    for (_, stats) in &open {
        merged.merge(stats);
    }

    // A client span that named its callee by service name or host name, but
    // whose SERVER span never paired (sampled away, exported late), is still
    // a call to that service when the name is unambiguous.
    let mut services_by_name: HashMap<&str, Vec<&ServiceKey>> = HashMap::new();
    for edge in merged.edges.keys() {
        for node in [&edge.caller, &edge.callee] {
            if let Node::Service(key) = node {
                let keys = services_by_name.entry(key.name.as_ref()).or_default();
                if !keys.contains(&key) {
                    keys.push(key);
                }
            }
        }
    }
    let mut renames: Vec<(Edge, Edge)> = Vec::new();
    for edge in merged.edges.keys() {
        let Node::Peer(peer) = &edge.callee else {
            continue;
        };
        if let Some([service]) = services_by_name.get(peer.name.as_ref()).map(Vec::as_slice) {
            let renamed = Edge {
                caller: edge.caller.clone(),
                callee: Node::Service((*service).clone()),
            };
            if renamed.caller != renamed.callee {
                renames.push((edge.clone(), renamed));
            }
        }
    }

    let mut edges: BTreeMap<Edge, EdgeStats> = merged.edges.into_iter().collect();
    for (from, to) in renames {
        if let Some(stats) = edges.remove(&from) {
            edges.entry(to).or_default().merge(&stats);
        }
    }

    MapView {
        edges,
        losses: merged.losses,
        unreadable_files: stored.unreadable_files,
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::peer::{PeerKey, PeerKind};

    fn service(namespace: Option<&str>, name: &str) -> Node {
        Node::Service(ServiceKey {
            namespace: namespace.map(Arc::from),
            name: Arc::from(name),
        })
    }

    fn peer(name: &str) -> Node {
        Node::Peer(PeerKey {
            kind: PeerKind::Rpc,
            name: Arc::from(name),
        })
    }

    fn minute(edges: &[(Node, Node, u64, u64)]) -> MinuteStats {
        let mut stats = MinuteStats::default();
        for (caller, callee, client, server) in edges {
            let mut e = EdgeStats::default();
            for _ in 0..*client {
                e.client.record(10, false);
            }
            for _ in 0..*server {
                e.server.record(10, false);
            }
            stats.edges.insert(
                Edge {
                    caller: caller.clone(),
                    callee: callee.clone(),
                },
                e,
            );
        }
        stats
    }

    fn calls(view: &MapView) -> Vec<(Node, Node, u64, u64)> {
        let mut out = Vec::new();
        for (edge, stats) in &view.edges {
            out.push((
                edge.caller.clone(),
                edge.callee.clone(),
                stats.client.calls,
                stats.server.calls,
            ));
        }
        out
    }

    #[test]
    fn window_includes_partial_minutes() {
        assert_eq!(window_minutes(60, 180), Some((1, 2)));
        assert_eq!(window_minutes(61, 181), Some((1, 3)));
        assert_eq!(window_minutes(100, 100), None);
        assert_eq!(window_minutes(100, 50), None);
    }

    #[test]
    fn merges_stored_and_open_minutes() {
        let mut stored = Loaded::default();
        stored
            .minutes
            .insert(1, minute(&[(service(None, "a"), service(None, "b"), 2, 2)]));
        stored.unreadable_files = 1;
        let mut open_minute = minute(&[(service(None, "a"), service(None, "b"), 1, 1)]);
        open_minute.losses.late = 3;
        let view = build(stored, vec![(2, open_minute)]);
        assert_eq!(
            calls(&view),
            vec![(service(None, "a"), service(None, "b"), 3, 3)]
        );
        assert_eq!(view.losses.late, 3);
        assert_eq!(view.unreadable_files, 1);
    }

    #[test]
    fn a_peer_naming_one_known_service_folds_into_it() {
        let mut stored = Loaded::default();
        stored.minutes.insert(
            1,
            minute(&[
                (service(None, "frontend"), service(None, "cart"), 5, 5),
                (service(None, "frontend"), peer("cart"), 2, 0),
                (service(None, "frontend"), peer("redis"), 1, 0),
            ]),
        );
        let view = build(stored, Vec::new());
        assert_eq!(
            calls(&view),
            vec![
                (service(None, "frontend"), service(None, "cart"), 7, 5),
                (service(None, "frontend"), peer("redis"), 1, 0),
            ]
        );
    }

    #[test]
    fn ambiguous_or_self_names_stay_peers() {
        let mut stored = Loaded::default();
        stored.minutes.insert(
            1,
            minute(&[
                (service(Some("eu"), "cart"), service(None, "x"), 1, 1),
                (service(Some("us"), "cart"), service(None, "x"), 1, 1),
                (service(None, "y"), peer("cart"), 1, 0),
                (service(None, "x"), peer("x"), 1, 0),
            ]),
        );
        let view = build(stored, Vec::new());
        let peers = view
            .edges
            .keys()
            .filter(|e| matches!(e.callee, Node::Peer(_)))
            .count();
        assert_eq!(peers, 2);
    }
}
