use super::*;

impl FlowDecoders {
    pub(crate) fn decoder_state_namespace_key(
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderStateNamespaceKey> {
        let (_version, observation_domain_id) = template_scope(payload)?;
        Some(DecoderStateNamespaceKey {
            exporter_ip: source.ip().to_string(),
            observation_domain_id,
        })
    }

    pub(crate) fn decoder_state_namespace_filename(key: &DecoderStateNamespaceKey) -> String {
        format!(
            "{}--{:08x}.bin",
            key.exporter_ip
                .chars()
                .map(|ch| if ch == '.' || ch == ':' { '_' } else { ch })
                .collect::<String>(),
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
