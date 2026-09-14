use crate::enrichment::{StaticRoutingEntry, StaticRoutingLargeCommunity};
use ipnet::IpNet;
use ipnet_trie::IpnetTrie;
use std::net::{IpAddr, SocketAddr};
use std::sync::{Arc, RwLock};

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) struct DynamicRoutingPeerKey {
    pub(crate) exporter: SocketAddr,
    pub(crate) session_id: u64,
    pub(crate) peer_id: String,
}

#[derive(Debug, Clone)]
pub(crate) struct DynamicRoutingUpdate {
    pub(crate) peer: DynamicRoutingPeerKey,
    pub(crate) prefix: IpNet,
    pub(crate) route_key: String,
    pub(crate) next_hop: Option<IpAddr>,
    pub(crate) asn: u32,
    pub(crate) as_path: Vec<u32>,
    pub(crate) communities: Vec<u32>,
    pub(crate) large_communities: Vec<(u32, u32, u32)>,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct DynamicRoutingRuntime {
    state: Arc<RwLock<DynamicRoutingState>>,
}

struct DynamicRoutingState {
    entries: IpnetTrie<Vec<DynamicRoutingRoute>>,
}

impl Default for DynamicRoutingState {
    fn default() -> Self {
        Self {
            entries: IpnetTrie::new(),
        }
    }
}

impl std::fmt::Debug for DynamicRoutingState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("DynamicRoutingState")
            .field("entries", &"<IpnetTrie>")
            .finish()
    }
}

#[derive(Debug, Clone)]
struct DynamicRoutingRoute {
    peer: DynamicRoutingPeerKey,
    route_key: String,
    next_hop: Option<IpAddr>,
    entry: StaticRoutingEntry,
}

impl DynamicRoutingRuntime {
    pub(crate) fn upsert(&self, update: DynamicRoutingUpdate) {
        let entry = StaticRoutingEntry {
            asn: update.asn,
            as_path: update.as_path,
            communities: update.communities,
            large_communities: update
                .large_communities
                .into_iter()
                .map(
                    |(asn, local_data1, local_data2)| StaticRoutingLargeCommunity {
                        asn,
                        local_data1,
                        local_data2,
                    },
                )
                .collect(),
            net_mask: update.prefix.prefix_len(),
            next_hop: update.next_hop,
        };

        let Ok(mut state) = self.state.write() else {
            return;
        };
        // Remove existing routes for this prefix (if any), modify, re-insert.
        let mut routes = state.entries.remove(update.prefix).unwrap_or_default();
        if let Some(route) = routes
            .iter_mut()
            .find(|route| route.peer == update.peer && route.route_key == update.route_key)
        {
            route.next_hop = update.next_hop;
            route.entry = entry;
        } else {
            routes.push(DynamicRoutingRoute {
                peer: update.peer,
                route_key: update.route_key,
                next_hop: update.next_hop,
                entry,
            });
        }
        state.entries.insert(update.prefix, routes);
    }

    pub(crate) fn withdraw(&self, peer: &DynamicRoutingPeerKey, prefix: IpNet, route_key: &str) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        if let Some(mut routes) = state.entries.remove(prefix) {
            routes.retain(|route| !(&route.peer == peer && route.route_key == route_key));
            if !routes.is_empty() {
                state.entries.insert(prefix, routes);
            }
        }
    }

    pub(crate) fn clear_peer(&self, peer: &DynamicRoutingPeerKey) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        // Collect affected prefixes (can't mutate trie while iterating).
        let affected: Vec<IpNet> = state
            .entries
            .iter()
            .filter(|(_, routes)| routes.iter().any(|route| &route.peer == peer))
            .map(|(prefix, _)| prefix)
            .collect();
        for prefix in affected {
            if let Some(mut routes) = state.entries.remove(prefix) {
                routes.retain(|route| &route.peer != peer);
                if !routes.is_empty() {
                    state.entries.insert(prefix, routes);
                }
            }
        }
    }

    pub(crate) fn clear_session(&self, exporter: SocketAddr, session_id: u64) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        let affected: Vec<IpNet> = state
            .entries
            .iter()
            .filter(|(_, routes)| {
                routes.iter().any(|route| {
                    route.peer.exporter == exporter && route.peer.session_id == session_id
                })
            })
            .map(|(prefix, _)| prefix)
            .collect();
        for prefix in affected {
            if let Some(mut routes) = state.entries.remove(prefix) {
                routes.retain(|route| {
                    route.peer.exporter != exporter || route.peer.session_id != session_id
                });
                if !routes.is_empty() {
                    state.entries.insert(prefix, routes);
                }
            }
        }
    }

    pub(crate) fn lookup(
        &self,
        address: IpAddr,
        preferred_next_hop: Option<IpAddr>,
        preferred_exporter: Option<IpAddr>,
    ) -> Option<StaticRoutingEntry> {
        let Ok(state) = self.state.read() else {
            return None;
        };

        // O(prefix_length) trie lookup instead of O(n) linear scan.
        let host_prefix = IpNet::from(address);
        let (_, routes) = state.entries.longest_match(&host_prefix)?;
        if routes.is_empty() {
            return None;
        }

        if let Some(exporter_ip) = preferred_exporter {
            if let Some(next_hop) = preferred_next_hop
                && let Some(route) = routes.iter().find(|route| {
                    route.peer.exporter.ip() == exporter_ip && route.next_hop == Some(next_hop)
                })
            {
                return Some(route.entry.clone());
            }
            if let Some(route) = routes
                .iter()
                .find(|route| route.peer.exporter.ip() == exporter_ip)
            {
                return Some(route.entry.clone());
            }
        }
        if let Some(next_hop) = preferred_next_hop
            && let Some(route) = routes.iter().find(|route| route.next_hop == Some(next_hop))
        {
            return Some(route.entry.clone());
        }
        routes.first().map(|route| route.entry.clone())
    }

    #[cfg(test)]
    pub(crate) fn route_count(&self) -> usize {
        let Ok(state) = self.state.read() else {
            return 0;
        };
        state.entries.iter().map(|(_, routes)| routes.len()).sum()
    }
}
