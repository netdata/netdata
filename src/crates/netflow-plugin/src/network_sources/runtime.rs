use crate::enrichment::NetworkAttributes;
use ipnet::IpNet;
use std::collections::HashMap;
use std::fmt;
use std::net::IpAddr;
use std::sync::{Arc, RwLock};

pub(crate) const IPV4_INDEX_MIN_FAMILY_RECORDS: usize = 500;
pub(crate) const IPV6_INDEX_MIN_FAMILY_RECORDS: usize = 2_000;

#[derive(Debug, Clone)]
pub(crate) struct NetworkSourceRecord {
    pub(crate) prefix: IpNet,
    pub(crate) attrs: NetworkAttributes,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct NetworkSourcesRuntime {
    state: Arc<RwLock<NetworkSourcesState>>,
}

#[derive(Default)]
struct NetworkSourcesState {
    records: Vec<NetworkSourceRecord>,
    prefixes: HashMap<IpNet, Vec<usize>>,
    ipv4_indices: Vec<usize>,
    ipv6_indices: Vec<usize>,
}

impl fmt::Debug for NetworkSourcesState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("NetworkSourcesState")
            .field("records_len", &self.records.len())
            .field("prefixes_len", &self.prefixes.len())
            .finish()
    }
}

impl NetworkSourcesRuntime {
    pub(crate) fn replace_records(&self, records: Vec<NetworkSourceRecord>) {
        let mut next = NetworkSourcesState::default();

        for (index, record) in records.iter().enumerate() {
            let prefix = record.prefix.trunc();
            next.prefixes.entry(prefix).or_default().push(index);
            match prefix {
                IpNet::V4(_) => next.ipv4_indices.push(index),
                IpNet::V6(_) => next.ipv6_indices.push(index),
            }
        }
        next.records = records;

        if let Ok(mut guard) = self.state.write() {
            *guard = next;
        }
    }

    pub(crate) fn matching_attributes_ascending(
        &self,
        address: IpAddr,
    ) -> Vec<(u8, NetworkAttributes)> {
        let Ok(state) = self.state.read() else {
            return Vec::new();
        };
        if state.records.is_empty() {
            return Vec::new();
        }

        let (max_prefix_len, index_min_family_records, family_indices) = match address {
            IpAddr::V4(_) => (
                32,
                IPV4_INDEX_MIN_FAMILY_RECORDS,
                state.ipv4_indices.as_slice(),
            ),
            IpAddr::V6(_) => (
                128,
                IPV6_INDEX_MIN_FAMILY_RECORDS,
                state.ipv6_indices.as_slice(),
            ),
        };
        if family_indices.is_empty() {
            return Vec::new();
        }
        if family_indices.len() < index_min_family_records {
            if family_indices.len() == state.records.len()
                || state.records.len() < index_min_family_records
            {
                return matching_attributes_ascending_linear(&state.records, address);
            }
            return matching_attributes_ascending_linear_indices(
                &state.records,
                family_indices,
                address,
            );
        }

        let mut matches = Vec::new();
        for prefix_len in 0..=max_prefix_len {
            let prefix = IpNet::new_assert(address, prefix_len).trunc();
            let Some(indices) = state.prefixes.get(&prefix) else {
                continue;
            };
            for index in indices {
                if let Some(record) = state.records.get(*index) {
                    matches.push((record.prefix.prefix_len(), record.attrs.clone()));
                }
            }
        }

        matches
    }
}

fn matching_attributes_ascending_linear(
    records: &[NetworkSourceRecord],
    address: IpAddr,
) -> Vec<(u8, NetworkAttributes)> {
    let mut matches = Vec::new();
    for record in records {
        if record.prefix.contains(&address) {
            matches.push((record.prefix.prefix_len(), record.attrs.clone()));
        }
    }
    matches.sort_by_key(|(prefix_len, _)| *prefix_len);
    matches
}

fn matching_attributes_ascending_linear_indices(
    records: &[NetworkSourceRecord],
    indices: &[usize],
    address: IpAddr,
) -> Vec<(u8, NetworkAttributes)> {
    let mut matches = Vec::new();
    for index in indices {
        let Some(record) = records.get(*index) else {
            continue;
        };
        if record.prefix.contains(&address) {
            matches.push((record.prefix.prefix_len(), record.attrs.clone()));
        }
    }
    matches.sort_by_key(|(prefix_len, _)| *prefix_len);
    matches
}
