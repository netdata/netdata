//! Naming the callee of a CLIENT/PRODUCER span that has no SERVER/CONSUMER
//! child (an uninstrumented database, queue or external API).
//!
//! The name follows the order most tracing backends converge on: an explicit
//! peer service, then the database/queue/RPC identity, then a host name, then
//! the protocol system. Old and new semantic-convention keys are both read,
//! since senders mix SDK versions. An IP address is never used as a name: it
//! identifies a container or pod, not a dependency a user recognizes.

use std::net::IpAddr;
use std::sync::Arc;

use opentelemetry_proto::tonic::common::v1::KeyValue;
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
pub enum PeerKind {
    Database,
    Messaging,
    Rpc,
    Http,
    Other,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
pub struct PeerKey {
    pub kind: PeerKind,
    pub name: Arc<str>,
}

pub const UNKNOWN_PEER: &str = "unknown";

const PEER_SERVICE: &[&str] = &["service.peer.name", "peer.service"];
const DB_SYSTEM: &[&str] = &["db.system.name", "db.system"];
const DB_NAMESPACE: &[&str] = &["db.namespace", "db.name"];
const MESSAGING_SYSTEM: &[&str] = &["messaging.system"];
const MESSAGING_DESTINATION: &[&str] = &["messaging.destination.name", "messaging.destination"];
const RPC_SYSTEM: &[&str] = &["rpc.system.name", "rpc.system"];
const RPC_SERVICE: &[&str] = &["rpc.service"];
const SERVER_ADDRESS: &[&str] = &["server.address", "net.peer.name"];
const URL: &[&str] = &["url.full", "http.url"];
const HTTP_METHOD: &[&str] = &["http.request.method", "http.method"];

pub(crate) fn name_peer(attributes: &[KeyValue]) -> PeerKey {
    let db_system = first_string(attributes, DB_SYSTEM);
    let messaging_system = first_string(attributes, MESSAGING_SYSTEM);
    let rpc_system = first_string(attributes, RPC_SYSTEM);

    let kind = if db_system.is_some() {
        PeerKind::Database
    } else if messaging_system.is_some() {
        PeerKind::Messaging
    } else if rpc_system.is_some() {
        PeerKind::Rpc
    } else if first_string(attributes, URL).is_some()
        || first_string(attributes, HTTP_METHOD).is_some()
    {
        PeerKind::Http
    } else {
        PeerKind::Other
    };

    let host = first_string(attributes, SERVER_ADDRESS)
        .filter(|h| !is_ip(h))
        .or_else(|| {
            first_string(attributes, URL)
                .and_then(url_host)
                .filter(|h| !is_ip(h))
        });

    let name = first_string(attributes, PEER_SERVICE)
        .or_else(|| match kind {
            PeerKind::Database => first_string(attributes, DB_NAMESPACE),
            PeerKind::Messaging => first_string(attributes, MESSAGING_DESTINATION),
            PeerKind::Rpc => first_string(attributes, RPC_SERVICE),
            PeerKind::Http | PeerKind::Other => None,
        })
        .or(host)
        .or(db_system)
        .or(messaging_system)
        .or(rpc_system)
        .unwrap_or(UNKNOWN_PEER);

    PeerKey {
        kind,
        name: Arc::from(name),
    }
}

fn first_string<'a>(attributes: &'a [KeyValue], keys: &[&str]) -> Option<&'a str> {
    for key in keys {
        for kv in attributes {
            if kv.key != *key {
                continue;
            }
            if let Some(Value::StringValue(s)) = kv.value.as_ref().and_then(|v| v.value.as_ref()) {
                let s = s.trim();
                if !s.is_empty() {
                    return Some(s);
                }
            }
        }
    }
    None
}

fn is_ip(host: &str) -> bool {
    host.trim_start_matches('[')
        .trim_end_matches(']')
        .parse::<IpAddr>()
        .is_ok()
}

/// The host of an absolute URL, without user info or port.
fn url_host(url: &str) -> Option<&str> {
    let rest = url.split_once("://")?.1;
    let authority = rest.split(['/', '?', '#']).next()?;
    let host_port = authority.rsplit_once('@').map_or(authority, |(_, h)| h);
    let host = if let Some(bracketed) = host_port.strip_prefix('[') {
        bracketed.split_once(']').map(|(h, _)| h)?
    } else {
        host_port.split(':').next()?
    };
    (!host.is_empty()).then_some(host)
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::AnyValue;

    fn attrs(pairs: &[(&str, &str)]) -> Vec<KeyValue> {
        let mut out = Vec::new();
        for (k, v) in pairs {
            out.push(KeyValue {
                key: (*k).to_string(),
                value: Some(AnyValue {
                    value: Some(Value::StringValue((*v).to_string())),
                }),
                ..Default::default()
            });
        }
        out
    }

    fn peer(kind: PeerKind, name: &str) -> PeerKey {
        PeerKey {
            kind,
            name: Arc::from(name),
        }
    }

    #[test]
    fn naming_priority() {
        type Case<'a> = (&'a str, Vec<(&'a str, &'a str)>, PeerKey);
        let cases: Vec<Case> = vec![
            (
                "explicit peer service wins",
                vec![
                    ("peer.service", "billing"),
                    ("db.system", "redis"),
                    ("server.address", "cache"),
                ],
                peer(PeerKind::Database, "billing"),
            ),
            (
                "new peer service key",
                vec![("service.peer.name", "billing")],
                peer(PeerKind::Other, "billing"),
            ),
            (
                "database namespace before host",
                vec![
                    ("db.system.name", "postgresql"),
                    ("db.namespace", "orders"),
                    ("server.address", "pg-1"),
                ],
                peer(PeerKind::Database, "orders"),
            ),
            (
                "database host before system",
                vec![("db.system", "redis"), ("server.address", "valkey-cart")],
                peer(PeerKind::Database, "valkey-cart"),
            ),
            (
                "database known only by IP falls back to its system",
                vec![("db.system", "redis"), ("server.address", "172.21.0.4")],
                peer(PeerKind::Database, "redis"),
            ),
            (
                "messaging destination",
                vec![
                    ("messaging.system", "kafka"),
                    ("messaging.destination.name", "orders"),
                ],
                peer(PeerKind::Messaging, "orders"),
            ),
            (
                "rpc service",
                vec![
                    ("rpc.system", "grpc"),
                    ("rpc.service", "oteldemo.CartService"),
                    ("server.address", "10.0.0.1"),
                ],
                peer(PeerKind::Rpc, "oteldemo.CartService"),
            ),
            (
                "http url host",
                vec![
                    ("http.request.method", "GET"),
                    ("url.full", "https://user@api.example.com:8443/x?y"),
                ],
                peer(PeerKind::Http, "api.example.com"),
            ),
            (
                "http to an IP has no name",
                vec![("http.method", "GET"), ("http.url", "http://[::1]:8010/")],
                peer(PeerKind::Http, UNKNOWN_PEER),
            ),
            ("nothing known", vec![], peer(PeerKind::Other, UNKNOWN_PEER)),
            (
                "blank values are skipped",
                vec![("peer.service", "  "), ("server.address", "agent")],
                peer(PeerKind::Other, "agent"),
            ),
        ];
        for (name, pairs, expected) in cases {
            assert_eq!(name_peer(&attrs(&pairs)), expected, "{name}");
        }
    }

    #[test]
    fn url_hosts() {
        assert_eq!(url_host("http://a.b:80/c"), Some("a.b"));
        assert_eq!(url_host("https://[2001:db8::1]:443/"), Some("2001:db8::1"));
        assert_eq!(url_host("not a url"), None);
        assert_eq!(url_host("http:///path"), None);
    }
}
