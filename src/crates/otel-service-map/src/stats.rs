//! Per-minute aggregates: one entry per (caller, callee) edge, with the
//! caller's (client side) and the callee's (server side) view kept apart,
//! plus the minute's loss counters. Everything here merges by addition, so a
//! window's map is the merge of its minutes.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::facts::ServiceKey;
use crate::histogram::Histogram;
use crate::peer::PeerKey;

pub const NS_PER_MINUTE: u64 = 60_000_000_000;

#[derive(Clone, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
pub enum Node {
    Service(ServiceKey),
    Peer(PeerKey),
    /// Calls whose caller is not instrumented, or whose caller's span never
    /// arrived within the pairing window.
    UnknownCaller,
    /// Edges beyond the per-minute edge cap.
    Overflow,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
pub struct Edge {
    pub caller: Node,
    pub callee: Node,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct SideStats {
    pub calls: u64,
    pub errors: u64,
    pub max_us: u64,
    pub latency_us: Histogram,
}

impl SideStats {
    /// A call without a known duration counts, but adds no latency.
    pub fn record(&mut self, duration_us: Option<u64>, error: bool) {
        self.calls += 1;
        self.errors += u64::from(error);
        if let Some(d) = duration_us {
            self.max_us = self.max_us.max(d);
            self.latency_us.record(d);
        }
    }

    pub fn merge(&mut self, other: &SideStats) {
        self.calls += other.calls;
        self.errors += other.errors;
        self.max_us = self.max_us.max(other.max_us);
        self.latency_us.merge(&other.latency_us);
    }
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct EdgeStats {
    pub client: SideStats,
    pub server: SideStats,
}

impl EdgeStats {
    pub fn merge(&mut self, other: &EdgeStats) {
        self.client.merge(&other.client);
        self.server.merge(&other.server);
    }
}

/// What the map could not count, so an undercount never looks like a quiet
/// service.
#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct LossCounters {
    /// Export requests skipped because the hand-off queue was full.
    pub handoff_drops: u64,
    /// Spans skipped because the pairing store was at its cap.
    pub capacity_drops: u64,
    /// Calls that ended in a minute already written to disk.
    pub late: u64,
    /// Spans ending too far in the future.
    pub future: u64,
    /// Spans seen twice within the pairing window (client retries).
    pub duplicates: u64,
    /// Spans with a malformed trace or span id.
    pub invalid: u64,
    /// SERVER/CONSUMER spans whose parent never arrived: counted as calls
    /// from the unknown caller.
    pub unpaired_servers: u64,
    /// Spans still waiting for their other half at shutdown.
    pub shutdown_lost: u64,
    /// Closed minutes that could not be written to disk.
    pub unwritten_minutes: u64,
}

impl LossCounters {
    pub fn merge(&mut self, other: &LossCounters) {
        self.handoff_drops += other.handoff_drops;
        self.capacity_drops += other.capacity_drops;
        self.late += other.late;
        self.future += other.future;
        self.duplicates += other.duplicates;
        self.invalid += other.invalid;
        self.unpaired_servers += other.unpaired_servers;
        self.shutdown_lost += other.shutdown_lost;
        self.unwritten_minutes += other.unwritten_minutes;
    }
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct MinuteStats {
    pub edges: HashMap<Edge, EdgeStats>,
    pub losses: LossCounters,
}

impl MinuteStats {
    pub fn merge(&mut self, other: &MinuteStats) {
        for (edge, stats) in &other.edges {
            self.edges.entry(edge.clone()).or_default().merge(stats);
        }
        self.losses.merge(&other.losses);
    }
}
