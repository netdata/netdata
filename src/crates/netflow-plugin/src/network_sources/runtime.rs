use crate::enrichment::NetworkAttributes;
use ipnet::IpNet;
use std::net::IpAddr;
use std::sync::{Arc, RwLock};

#[derive(Debug, Clone)]
pub(crate) struct NetworkSourceRecord {
    pub(crate) prefix: IpNet,
    pub(crate) attrs: NetworkAttributes,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct NetworkSourcesRuntime {
    records: Arc<RwLock<Vec<NetworkSourceRecord>>>,
}

impl NetworkSourcesRuntime {
    pub(crate) fn replace_records(&self, records: Vec<NetworkSourceRecord>) {
        if let Ok(mut guard) = self.records.write() {
            *guard = records;
        }
    }

    pub(crate) fn matching_attributes_ascending(
        &self,
        address: IpAddr,
    ) -> Vec<(u8, NetworkAttributes)> {
        let Ok(records) = self.records.read() else {
            return Vec::new();
        };

        let mut matches = Vec::new();
        for record in records.iter() {
            if record.prefix.contains(&address) {
                matches.push((record.prefix.prefix_len(), record.attrs.clone()));
            }
        }
        matches.sort_by_key(|(prefix_len, _)| *prefix_len);
        matches
    }
}
