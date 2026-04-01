use super::*;

impl FlowDecoders {
    pub(crate) fn hydrate_loaded_decoder_state_namespace(
        &mut self,
        key: &DecoderStateNamespaceKey,
        source: SocketAddr,
    ) -> Result<(), String> {
        let Some(namespace) = self.decoder_state_namespaces.get(key).cloned() else {
            self.hydrated_namespace_sources
                .entry(key.clone())
                .or_default()
                .insert(source);
            return Ok(());
        };

        self.sampling.apply_decoder_state_namespace(key, &namespace);
        self.replay_namespace_packets(key, &namespace, source)?;
        self.hydrated_namespace_sources
            .entry(key.clone())
            .or_default()
            .insert(source);
        Ok(())
    }

    #[cfg(test)]
    pub(crate) fn import_decoder_state_namespace(
        &mut self,
        expected_key: DecoderStateNamespaceKey,
        source: SocketAddr,
        data: &[u8],
    ) -> Result<(), String> {
        let persisted = decode_persisted_namespace_file(data)?;
        if persisted.key != expected_key {
            return Err(format!(
                "namespace key mismatch in persisted decoder state (got {} / {}, expected {} / {})",
                persisted.key.exporter_ip,
                persisted.key.observation_domain_id,
                expected_key.exporter_ip,
                expected_key.observation_domain_id
            ));
        }

        self.loaded_decoder_namespaces.insert(expected_key.clone());
        self.decoder_state_namespaces
            .insert(expected_key.clone(), persisted.namespace.clone());
        self.dirty_decoder_namespaces.remove(&expected_key);
        self.hydrated_namespace_sources.remove(&expected_key);
        self.hydrate_loaded_decoder_state_namespace(&expected_key, source)
    }

    pub(crate) fn preload_decoder_state_namespace(
        &mut self,
        data: &[u8],
    ) -> Result<DecoderStateNamespaceKey, String> {
        let persisted = decode_persisted_namespace_file(data)?;
        let key = persisted.key.clone();
        self.loaded_decoder_namespaces.insert(key.clone());
        self.decoder_state_namespaces
            .insert(key.clone(), persisted.namespace);
        self.dirty_decoder_namespaces.remove(&key);
        self.hydrated_namespace_sources.remove(&key);
        Ok(key)
    }

    pub(crate) fn export_decoder_state_namespace(
        &self,
        key: &DecoderStateNamespaceKey,
    ) -> Result<Option<Vec<u8>>, String> {
        let Some(namespace) = self.decoder_state_namespaces.get(key) else {
            return Ok(None);
        };
        if namespace.is_empty() {
            return Ok(None);
        }

        encode_persisted_namespace_file(&PersistedDecoderNamespaceFile {
            key: key.clone(),
            namespace: namespace.clone(),
        })
        .map(Some)
    }

    pub(crate) fn replay_namespace_packets(
        &mut self,
        key: &DecoderStateNamespaceKey,
        namespace: &DecoderStateNamespace,
        source: SocketAddr,
    ) -> Result<(), String> {
        for packet in build_namespace_restore_packets(key, namespace)? {
            let _ = self.netflow.parse_from_source(source, &packet);
        }
        Ok(())
    }
}
