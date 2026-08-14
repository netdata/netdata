use super::*;
use lru::LruCache;
use std::num::NonZeroUsize;

impl SamplingState {
    pub(crate) fn new(global_capacity: usize, per_stream_capacity: usize) -> Self {
        let global_capacity = NonZeroUsize::new(global_capacity)
            .expect("sampling global cache capacity must be positive");
        let per_stream_capacity = NonZeroUsize::new(per_stream_capacity.min(global_capacity.get()))
            .expect("sampling per-stream cache capacity must be positive");
        Self {
            values: HashMap::new(),
            global_lru: LruCache::new(global_capacity),
            per_stream_lru: HashMap::new(),
            per_stream_capacity,
        }
    }

    pub(crate) fn namespace_key(
        source: SocketAddr,
        version: u16,
        observation_domain_id: u32,
    ) -> Option<DecoderStateNamespaceKey> {
        let exporter_ip = canonicalize_ip_addr(source.ip());
        match version {
            9 => Some(DecoderStateNamespaceKey {
                protocol: DecoderStateProtocol::V9,
                exporter_ip: exporter_ip.to_string(),
                source_port: source.port(),
                observation_domain_id,
            }),
            10 => Some(DecoderStateNamespaceKey {
                protocol: DecoderStateProtocol::Ipfix,
                exporter_ip: exporter_ip.to_string(),
                source_port: 0,
                observation_domain_id,
            }),
            _ => None,
        }
    }

    pub(crate) fn clear_namespace(&mut self, namespace: &DecoderStateNamespaceKey) -> bool {
        let Some(entries) = self.per_stream_lru.remove(namespace) else {
            return false;
        };
        let mut changed = false;
        for (sampler_id, ()) in entries {
            let entry = SamplingEntryKey {
                namespace: namespace.clone(),
                sampler_id,
            };
            changed |= self.values.remove(&entry).is_some();
            self.global_lru.pop(&entry);
        }
        changed
    }

    pub(crate) fn set(
        &mut self,
        source: SocketAddr,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
        sampling_rate: u64,
    ) -> Vec<DecoderStateNamespaceKey> {
        let Some(namespace) = Self::namespace_key(source, version, observation_domain_id) else {
            return Vec::new();
        };
        self.set_for_namespace(namespace, sampler_id, sampling_rate)
    }

    pub(crate) fn set_for_namespace(
        &mut self,
        namespace: DecoderStateNamespaceKey,
        sampler_id: u64,
        sampling_rate: u64,
    ) -> Vec<DecoderStateNamespaceKey> {
        if sampling_rate == 0 {
            return Vec::new();
        }

        let entry = SamplingEntryKey {
            namespace: namespace.clone(),
            sampler_id,
        };
        if let Some(value) = self.values.get_mut(&entry) {
            let changed = *value != sampling_rate;
            *value = sampling_rate;
            self.global_lru.get(&entry);
            if let Some(lru) = self.per_stream_lru.get_mut(&namespace) {
                lru.get(&sampler_id);
            }
            return changed.then_some(namespace).into_iter().collect();
        }

        let mut dirty = vec![namespace.clone()];
        let per_stream = self
            .per_stream_lru
            .entry(namespace.clone())
            .or_insert_with(|| LruCache::new(self.per_stream_capacity));
        if let Some((evicted_sampler, ())) = per_stream.push(sampler_id, ()) {
            let evicted = SamplingEntryKey {
                namespace: namespace.clone(),
                sampler_id: evicted_sampler,
            };
            self.values.remove(&evicted);
            self.global_lru.pop(&evicted);
        }

        self.values.insert(entry.clone(), sampling_rate);
        if let Some((evicted, ())) = self.global_lru.push(entry, ()) {
            self.values.remove(&evicted);
            if let Some(lru) = self.per_stream_lru.get_mut(&evicted.namespace) {
                lru.pop(&evicted.sampler_id);
                if lru.is_empty() {
                    self.per_stream_lru.remove(&evicted.namespace);
                }
            }
            if !dirty.contains(&evicted.namespace) {
                dirty.push(evicted.namespace);
            }
        }
        dirty
    }

    pub(crate) fn get(
        &mut self,
        source: SocketAddr,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
    ) -> Option<u64> {
        let namespace = Self::namespace_key(source, version, observation_domain_id)?;
        let entry = SamplingEntryKey {
            namespace: namespace.clone(),
            sampler_id,
        };
        let value = self.values.get(&entry).copied()?;
        self.global_lru.get(&entry);
        self.per_stream_lru
            .get_mut(&namespace)
            .and_then(|lru| lru.get(&sampler_id));
        Some(value)
    }

    pub(crate) fn snapshot_namespace(
        &self,
        namespace: &DecoderStateNamespaceKey,
    ) -> Vec<PersistedSamplingRate> {
        let Some(entries) = self.per_stream_lru.get(namespace) else {
            return Vec::new();
        };
        let version = match namespace.protocol {
            DecoderStateProtocol::V9 => 9,
            DecoderStateProtocol::Ipfix => 10,
        };
        let mut rows = entries
            .iter()
            .filter_map(|(sampler_id, ())| {
                let entry = SamplingEntryKey {
                    namespace: namespace.clone(),
                    sampler_id: *sampler_id,
                };
                self.values
                    .get(&entry)
                    .map(|sampling_rate| PersistedSamplingRate {
                        version,
                        sampler_id: *sampler_id,
                        sampling_rate: *sampling_rate,
                    })
            })
            .collect::<Vec<_>>();
        rows.sort_by_key(|row| row.sampler_id);
        rows
    }

    pub(crate) fn replace_namespace(
        &mut self,
        namespace: &DecoderStateNamespaceKey,
        rows: &[PersistedSamplingRate],
    ) -> Vec<DecoderStateNamespaceKey> {
        self.clear_namespace(namespace);
        let mut dirty = Vec::new();
        for row in rows {
            for key in self.set_for_namespace(namespace.clone(), row.sampler_id, row.sampling_rate)
            {
                if !dirty.contains(&key) {
                    dirty.push(key);
                }
            }
        }
        dirty
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn global_limit_evicts_the_least_recently_used_sampling_entry() {
        let mut state = SamplingState::new(2, 2);
        let a: SocketAddr = "192.0.2.1:2055".parse().unwrap();
        let b: SocketAddr = "192.0.2.2:2055".parse().unwrap();
        let c: SocketAddr = "192.0.2.3:2055".parse().unwrap();

        state.set(a, 9, 1, 1, 100);
        state.set(b, 9, 1, 1, 200);
        assert_eq!(state.get(a, 9, 1, 1), Some(100));
        let dirty = state.set(c, 9, 1, 1, 300);

        assert_eq!(state.get(a, 9, 1, 1), Some(100));
        assert_eq!(state.get(b, 9, 1, 1), None);
        assert_eq!(state.get(c, 9, 1, 1), Some(300));
        assert_eq!(dirty.len(), 2, "new and evicted streams must be persisted");
    }

    #[test]
    fn per_stream_limit_evicts_only_within_that_stream() {
        let mut state = SamplingState::new(10, 1);
        let source: SocketAddr = "192.0.2.1:2055".parse().unwrap();

        state.set(source, 9, 1, 1, 100);
        state.set(source, 9, 1, 2, 200);

        assert_eq!(state.get(source, 9, 1, 1), None);
        assert_eq!(state.get(source, 9, 1, 2), Some(200));
    }

    #[test]
    fn v9_sampling_is_source_port_scoped_but_ipfix_keeps_existing_identity() {
        let mut state = SamplingState::new(10, 10);
        let first: SocketAddr = "192.0.2.1:2055".parse().unwrap();
        let second: SocketAddr = "192.0.2.1:9999".parse().unwrap();

        state.set(first, 9, 1, 1, 100);
        assert_eq!(state.get(second, 9, 1, 1), None);

        state.set(first, 10, 1, 1, 200);
        assert_eq!(state.get(second, 10, 1, 1), Some(200));
    }

    #[test]
    fn persisted_snapshot_round_trips_through_the_bounded_owner() {
        let mut state = SamplingState::new(10, 10);
        let source: SocketAddr = "192.0.2.1:2055".parse().unwrap();
        let namespace = SamplingState::namespace_key(source, 9, 1).unwrap();
        state.set(source, 9, 1, 7, 4_000);

        let rows = state.snapshot_namespace(&namespace);
        let mut restored = SamplingState::new(10, 10);
        let _ = restored.replace_namespace(&namespace, &rows);

        assert_eq!(restored.get(source, 9, 1, 7), Some(4_000));
    }
}
