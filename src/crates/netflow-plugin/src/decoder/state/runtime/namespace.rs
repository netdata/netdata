use super::*;

impl FlowDecoders {
    pub(crate) fn decoder_scope_snapshot(&self) -> DecoderScopeSnapshot {
        DecoderScopeSnapshot {
            v9_sources: self.netflow.v9_source_count() as u64,
            ipfix_sources: self.netflow.ipfix_source_count() as u64,
            legacy_sources: self.netflow.legacy_source_count() as u64,
            namespaces: self.decoder_state_namespaces.len() as u64,
            hydrated_sources: self
                .hydrated_namespace_sources
                .values()
                .map(std::collections::HashSet::len)
                .sum::<usize>() as u64,
        }
    }

    pub(crate) fn decoder_state_namespace_key(
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderStateNamespaceKey> {
        let (_version, observation_domain_id) = template_scope(payload)?;
        Some(DecoderStateNamespaceKey {
            exporter_ip: canonicalize_ip_addr(source.ip()).to_string(),
            observation_domain_id,
        })
    }

    pub(crate) fn decoder_state_namespace_filename(key: &DecoderStateNamespaceKey) -> String {
        format!(
            "{}--{:08x}.bin",
            sanitize_namespace_filename_component(&key.exporter_ip),
            key.observation_domain_id
        )
    }

    pub(crate) fn is_decoder_state_namespace_loaded(&self, key: &DecoderStateNamespaceKey) -> bool {
        self.loaded_decoder_namespaces.contains(key)
    }

    pub(crate) fn decoder_state_source_needs_hydration(
        &self,
        key: &DecoderStateNamespaceKey,
        source: SocketAddr,
    ) -> bool {
        let source = normalize_template_scope_source(source);
        !self
            .hydrated_namespace_sources
            .get(key)
            .is_some_and(|sources| sources.contains(&source))
    }

    pub(crate) fn mark_decoder_state_namespace_absent(
        &mut self,
        key: DecoderStateNamespaceKey,
        source: SocketAddr,
    ) {
        let source = normalize_template_scope_source(source);
        self.loaded_decoder_namespaces.insert(key.clone());
        self.decoder_state_namespaces
            .entry(key.clone())
            .or_default();
        self.hydrated_namespace_sources
            .entry(key)
            .or_default()
            .insert(source);
    }

    pub(crate) fn dirty_decoder_state_namespaces(&self) -> Vec<DecoderStateNamespaceKey> {
        let mut keys: Vec<_> = self.dirty_decoder_namespaces.iter().cloned().collect();
        keys.sort_by(|left, right| {
            left.exporter_ip
                .cmp(&right.exporter_ip)
                .then(left.observation_domain_id.cmp(&right.observation_domain_id))
        });
        keys
    }

    pub(crate) fn mark_decoder_state_namespace_persisted(
        &mut self,
        key: &DecoderStateNamespaceKey,
    ) {
        self.dirty_decoder_namespaces.remove(key);
    }

    #[cfg(test)]
    pub(crate) fn decoder_state_namespace_keys(&self) -> Vec<DecoderStateNamespaceKey> {
        let mut keys: Vec<_> = self.decoder_state_namespaces.keys().cloned().collect();
        keys.sort_by(|left, right| {
            left.exporter_ip
                .cmp(&right.exporter_ip)
                .then(left.observation_domain_id.cmp(&right.observation_domain_id))
        });
        keys
    }
}

fn sanitize_namespace_filename_component(value: &str) -> String {
    value
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || ch == '_' || ch == '-' {
                ch
            } else {
                '_'
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decoder_state_namespace_filename_sanitizes_untrusted_exporter_ip() {
        let key = DecoderStateNamespaceKey {
            exporter_ip: "../fe80::1/../../odd name".to_string(),
            observation_domain_id: 0x1234_abcd,
        };

        let filename = FlowDecoders::decoder_state_namespace_filename(&key);

        assert_eq!(filename, "___fe80__1_______odd_name--1234abcd.bin");
    }
}
