//! Pairing and per-minute counting. Pure: the caller passes the wall-clock
//! time, so tests drive arrival orders and expiry deterministically.
//!
//! A call between services is a SERVER/CONSUMER span whose parent is a
//! CLIENT/PRODUCER span of another service. The two halves are exported by
//! different processes and reach us seconds apart in either order, so every
//! span waits in `recent` for `Limits::wait_ns`. A callee whose parent has not
//! arrived waits in `waiting`, keyed by that parent id. When the wait ends:
//!
//! - an innermost caller span that never met its callee is a call to a
//!   virtual peer named from its attributes (a database, queue, external API);
//! - a callee whose parent never arrived is a call from the unknown caller.
//!
//! Each call is counted once, in the minute its span ended.

use std::collections::{BTreeMap, HashMap, VecDeque};
use std::sync::Arc;

use crate::facts::{SpanFacts, SpanId, TraceId};
use crate::peer::{PeerKey, PeerKind, UNKNOWN_PEER};
use crate::stats::{Edge, LossCounters, MinuteStats, NS_PER_MINUTE, Node};

#[derive(Clone, Debug)]
pub struct Limits {
    /// How long a span waits for its other half.
    pub wait_ns: u64,
    /// How long after a minute ends it stays open for late calls; it is then
    /// handed out for writing.
    pub horizon_ns: u64,
    /// Spans ending further than this in the future are dropped.
    pub future_ns: u64,
    /// Cap on spans held for pairing, over all tenants.
    pub max_in_flight: usize,
    /// Cap on distinct edges per tenant and minute; the rest count as one
    /// overflow edge.
    pub max_edges_per_minute: usize,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            wait_ns: 30_000_000_000,
            horizon_ns: 120_000_000_000,
            future_ns: 120_000_000_000,
            max_in_flight: 250_000,
            max_edges_per_minute: 5_000,
        }
    }
}

/// A minute that will not change any more, ready to be written.
#[derive(Debug)]
pub struct ClosedMinute {
    pub tenant: Arc<str>,
    pub minute: u64,
    pub stats: MinuteStats,
}

type Key = (TraceId, SpanId);

struct Entry {
    facts: SpanFacts,
    arrival_ns: u64,
    paired: bool,
    has_caller_child: bool,
}

struct Waiting {
    facts: SpanFacts,
    arrival_ns: u64,
}

enum Expiry {
    Recent(Key),
    Waiting(Key),
    CallerChild(Key),
}

#[derive(Default)]
struct Pairing {
    recent: HashMap<Key, Entry>,
    waiting: HashMap<Key, Vec<Waiting>>,
    /// Parent ids of CLIENT/PRODUCER spans whose parent has not arrived yet:
    /// that parent is then an outer caller span (a nested client call) and is
    /// not a call of its own.
    caller_children: HashMap<Key, u64>,
    queue: VecDeque<(u64, Expiry)>,
}

#[derive(Default)]
struct Tally {
    minutes: BTreeMap<u64, MinuteStats>,
    /// Minutes before this one were handed out and must not change.
    closed_before: Option<u64>,
}

impl Tally {
    fn losses(&mut self, now_ns: u64) -> &mut LossCounters {
        &mut self
            .minutes
            .entry(now_ns / NS_PER_MINUTE)
            .or_default()
            .losses
    }

    fn count(
        &mut self,
        limits: &Limits,
        now_ns: u64,
        edge: Edge,
        client: Option<&SpanFacts>,
        server: Option<&SpanFacts>,
    ) {
        let Some(end_ns) = server.or(client).map(|s| s.end_ns) else {
            return;
        };
        let minute = end_ns / NS_PER_MINUTE;
        if self.closed_before.is_some_and(|closed| minute < closed) {
            self.losses(now_ns).late += 1;
            return;
        }
        let stats = self.minutes.entry(minute).or_default();
        let edge = if stats.edges.len() >= limits.max_edges_per_minute
            && !stats.edges.contains_key(&edge)
        {
            Edge {
                caller: Node::Overflow,
                callee: Node::Overflow,
            }
        } else {
            edge
        };
        let entry = stats.edges.entry(edge).or_default();
        if let Some(c) = client {
            entry.client.record(c.duration_us, c.error);
        }
        if let Some(s) = server {
            entry.server.record(s.duration_us, s.error);
        }
    }
}

#[derive(Default)]
struct Tenant {
    pairing: Pairing,
    tally: Tally,
}

#[derive(Default)]
pub struct Engine {
    limits: Limits,
    tenants: HashMap<Arc<str>, Tenant>,
    in_flight: usize,
}

impl Engine {
    pub fn new(limits: Limits) -> Self {
        Self {
            limits,
            tenants: HashMap::new(),
            in_flight: 0,
        }
    }

    pub fn in_flight(&self) -> usize {
        self.in_flight
    }

    /// A new tenant starts with the minutes before the horizon closed, like
    /// every tenant after its first flush, so old spans count as late.
    fn tenant(&mut self, tenant: &str, now_ns: u64) -> &mut Tenant {
        let open_from = now_ns.saturating_sub(self.limits.horizon_ns) / NS_PER_MINUTE;
        self.tenants
            .entry(Arc::from(tenant))
            .or_insert_with(|| Tenant {
                pairing: Pairing::default(),
                tally: Tally {
                    minutes: BTreeMap::new(),
                    closed_before: Some(open_from),
                },
            })
    }

    pub fn ingest(&mut self, tenant: &str, spans: Vec<SpanFacts>, invalid: u64, now_ns: u64) {
        let limits = self.limits.clone();
        let mut in_flight = self.in_flight;
        let t = self.tenant(tenant, now_ns);
        if invalid > 0 {
            t.tally.losses(now_ns).invalid += invalid;
        }
        for facts in spans {
            ingest_span(t, &limits, &mut in_flight, facts, now_ns);
        }
        self.in_flight = in_flight;
    }

    pub fn note_handoff_drops(&mut self, tenant: &str, drops: u64, now_ns: u64) {
        if drops > 0 {
            self.tenant(tenant, now_ns)
                .tally
                .losses(now_ns)
                .handoff_drops += drops;
        }
    }

    pub fn note_unwritten_minutes(&mut self, tenant: &str, minutes: u64, now_ns: u64) {
        if minutes > 0 {
            self.tenant(tenant, now_ns)
                .tally
                .losses(now_ns)
                .unwritten_minutes += minutes;
        }
    }

    /// Ends the wait of every span that arrived `wait_ns` or more ago.
    pub fn expire(&mut self, now_ns: u64) {
        let limits = self.limits.clone();
        let mut in_flight = self.in_flight;
        for t in self.tenants.values_mut() {
            expire_tenant(t, &limits, &mut in_flight, now_ns);
        }
        self.in_flight = in_flight;
    }

    /// Hands out every minute that ended `horizon_ns` or more ago. Later calls
    /// into those minutes count as `late`.
    pub fn take_closed(&mut self, now_ns: u64) -> Vec<ClosedMinute> {
        let mut out = Vec::new();
        let open_from = now_ns.saturating_sub(self.limits.horizon_ns) / NS_PER_MINUTE;
        // A minute m ends at (m + 1) * 60 s; it closes once that is at least
        // `horizon_ns` ago, so every minute below `open_from` closes.
        for (name, t) in &mut self.tenants {
            let open = t.tally.minutes.split_off(&open_from);
            let closed = std::mem::replace(&mut t.tally.minutes, open);
            for (minute, stats) in closed {
                out.push(ClosedMinute {
                    tenant: name.clone(),
                    minute,
                    stats,
                });
            }
            // Follows the clock both ways: after a backward correction the
            // reopened minutes count again, and their new counts merge with
            // the stored ones when written.
            t.tally.closed_before = Some(open_from);
        }
        out
    }

    /// Copies of the open minutes in `[from_minute, to_minute]`.
    pub fn open_minutes(
        &self,
        tenant: &str,
        from_minute: u64,
        to_minute: u64,
    ) -> Vec<(u64, MinuteStats)> {
        let mut out = Vec::new();
        if let Some(t) = self.tenants.get(tenant) {
            for (minute, stats) in t.tally.minutes.range(from_minute..=to_minute) {
                out.push((*minute, stats.clone()));
            }
        }
        out
    }

    /// Counts every span still waiting as lost and hands out every minute.
    pub fn shutdown(&mut self, now_ns: u64) -> Vec<ClosedMinute> {
        let mut out = Vec::new();
        for (name, t) in &mut self.tenants {
            let mut lost = t.pairing.waiting.values().map(Vec::len).sum::<usize>() as u64;
            for entry in t.pairing.recent.values() {
                lost += u64::from(
                    entry.facts.kind.is_caller() && !entry.paired && !entry.has_caller_child,
                );
            }
            if lost > 0 {
                t.tally.losses(now_ns).shutdown_lost += lost;
            }
            t.pairing = Pairing::default();
            for (minute, stats) in std::mem::take(&mut t.tally.minutes) {
                out.push(ClosedMinute {
                    tenant: name.clone(),
                    minute,
                    stats,
                });
            }
        }
        self.in_flight = 0;
        out
    }
}

fn ingest_span(
    t: &mut Tenant,
    limits: &Limits,
    in_flight: &mut usize,
    facts: SpanFacts,
    now_ns: u64,
) {
    if facts.end_ns > now_ns.saturating_add(limits.future_ns) {
        t.tally.losses(now_ns).future += 1;
        return;
    }
    let key = (facts.trace_id, facts.span_id);
    if t.pairing.recent.contains_key(&key) {
        t.tally.losses(now_ns).duplicates += 1;
        return;
    }

    // A root callee needs no pairing; count it before the capacity check.
    if facts.kind.is_callee() && facts.parent_span_id.is_none() {
        let edge = Edge {
            caller: Node::UnknownCaller,
            callee: Node::Service(facts.service.clone()),
        };
        t.tally.count(limits, now_ns, edge, None, Some(&facts));
    }

    if *in_flight >= limits.max_in_flight {
        t.tally.losses(now_ns).capacity_drops += 1;
        return;
    }

    let pairing = &mut t.pairing;
    if let Some(parent) = facts.parent_span_id {
        let parent_key = (facts.trace_id, parent);
        if facts.kind.is_caller() {
            if let Some(entry) = pairing.recent.get_mut(&parent_key) {
                entry.has_caller_child = true;
            } else if let std::collections::hash_map::Entry::Vacant(slot) =
                pairing.caller_children.entry(parent_key)
            {
                slot.insert(now_ns);
                pairing
                    .queue
                    .push_back((now_ns, Expiry::CallerChild(parent_key)));
                *in_flight += 1;
            }
        }
        if facts.kind.is_callee() {
            if let Some(entry) = pairing.recent.get_mut(&parent_key) {
                resolve(&mut t.tally, limits, now_ns, entry, facts.clone());
            } else {
                pairing
                    .waiting
                    .entry(parent_key)
                    .or_default()
                    .push(Waiting {
                        facts: facts.clone(),
                        arrival_ns: now_ns,
                    });
                pairing
                    .queue
                    .push_back((now_ns, Expiry::Waiting(parent_key)));
                *in_flight += 1;
            }
        }
    }

    let has_caller_child = pairing.caller_children.remove(&key).is_some();
    if has_caller_child {
        *in_flight -= 1;
    }
    let children = pairing.waiting.remove(&key);
    let mut entry = Entry {
        facts,
        arrival_ns: now_ns,
        paired: false,
        has_caller_child,
    };
    if let Some(children) = children {
        *in_flight -= children.len();
        for child in children {
            resolve(&mut t.tally, limits, now_ns, &mut entry, child.facts);
        }
    }
    pairing.recent.insert(key, entry);
    pairing.queue.push_back((now_ns, Expiry::Recent(key)));
    *in_flight += 1;
}

/// `child` is a callee span whose parent is `parent`.
fn resolve(tally: &mut Tally, limits: &Limits, now_ns: u64, parent: &mut Entry, child: SpanFacts) {
    if !parent.facts.kind.is_caller() {
        // A callee nested in a SERVER or INTERNAL span: not a call.
        return;
    }
    if parent.facts.service == child.service {
        // A service calling itself found its callee, so the caller span is
        // not a call to an unknown peer; self-calls are not drawn.
        parent.paired = true;
        return;
    }
    let edge = Edge {
        caller: Node::Service(parent.facts.service.clone()),
        callee: Node::Service(child.service.clone()),
    };
    // A caller span is one call: a second callee child (a retry answered
    // twice, a fan-out proxy) adds only its server side.
    let client = (!parent.paired).then_some(&parent.facts);
    tally.count(limits, now_ns, edge, client, Some(&child));
    parent.paired = true;
}

fn expire_tenant(t: &mut Tenant, limits: &Limits, in_flight: &mut usize, now_ns: u64) {
    let pairing = &mut t.pairing;
    while let Some((arrival_ns, _)) = pairing.queue.front() {
        if arrival_ns.saturating_add(limits.wait_ns) > now_ns {
            break;
        }
        let (arrival_ns, expiry) = pairing.queue.pop_front().expect("front exists");
        match expiry {
            Expiry::Recent(key) => {
                // The key may have been re-inserted by a later resend.
                if pairing
                    .recent
                    .get(&key)
                    .is_none_or(|e| e.arrival_ns != arrival_ns)
                {
                    continue;
                }
                let entry = pairing.recent.remove(&key).expect("checked above");
                *in_flight -= 1;
                if entry.facts.kind.is_caller() && !entry.paired && !entry.has_caller_child {
                    let peer = entry.facts.peer.clone().unwrap_or_else(|| PeerKey {
                        kind: PeerKind::Other,
                        name: Arc::from(UNKNOWN_PEER),
                    });
                    let edge = Edge {
                        caller: Node::Service(entry.facts.service.clone()),
                        callee: Node::Peer(peer),
                    };
                    t.tally
                        .count(limits, now_ns, edge, Some(&entry.facts), None);
                }
            }
            Expiry::Waiting(key) => {
                let Some(list) = pairing.waiting.get_mut(&key) else {
                    continue;
                };
                let mut expired = Vec::new();
                let mut i = 0;
                while i < list.len() {
                    if list[i].arrival_ns.saturating_add(limits.wait_ns) <= now_ns {
                        expired.push(list.swap_remove(i));
                    } else {
                        i += 1;
                    }
                }
                if list.is_empty() {
                    pairing.waiting.remove(&key);
                }
                *in_flight -= expired.len();
                for w in expired {
                    t.tally.losses(now_ns).unpaired_servers += 1;
                    let edge = Edge {
                        caller: Node::UnknownCaller,
                        callee: Node::Service(w.facts.service.clone()),
                    };
                    t.tally.count(limits, now_ns, edge, None, Some(&w.facts));
                }
            }
            Expiry::CallerChild(key) => {
                if pairing.caller_children.get(&key) == Some(&arrival_ns) {
                    pairing.caller_children.remove(&key);
                    *in_flight -= 1;
                }
            }
        }
    }
}
