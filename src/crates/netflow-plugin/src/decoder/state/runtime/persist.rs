use super::*;

impl FlowDecoders {
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn hydrate_loaded_decoder_state_namespace(
        &mut self,
        key: &DecoderStateNamespaceKey,
        source: SocketAddr,
    ) -> Result<(), String> {
        let source = normalize_template_scope_source(source);
        self.hydrate_loaded_decoder_state_namespace_for_normalized_source(key, source)
    }

    pub(crate) fn hydrate_loaded_decoder_state_namespace_for_normalized_source(
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

        // Validate replay into a fresh parser first so restore failures do not
        // partially mutate the live parser or sampling state.
        let mut validation_parser = super::init::new_netflow_parser(self.max_records_per_flowset);
        Self::replay_namespace_packets_into(&mut validation_parser, key, &namespace, source)?;

        self.replay_namespace_packets(key, &namespace, source)?;
        self.sampling.apply_decoder_state_namespace(key, &namespace);
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
        Self::replay_namespace_packets_into(&mut self.netflow, key, namespace, source)
    }

    fn replay_namespace_packets_into(
        parser: &mut AutoScopedParser,
        key: &DecoderStateNamespaceKey,
        namespace: &DecoderStateNamespace,
        source: SocketAddr,
    ) -> Result<(), String> {
        let mut observed_namespace = DecoderStateNamespace::default();
        let mut observed_sampling = SamplingState::default();

        for (packet_index, packet) in build_namespace_restore_packets(key, namespace)?
            .into_iter()
            .enumerate()
        {
            let result = parser.parse_from_source(source, &packet);
            if let Some(err) = &result.error {
                return Err(format!(
                    "failed to replay persisted namespace packet {} for {} / {} from {}: {}",
                    packet_index, key.exporter_ip, key.observation_domain_id, source, err
                ));
            }

            for packet in &result.packets {
                match packet {
                    NetflowPacket::V9(packet) => {
                        observe_v9_decoder_state_from_packet(
                            source.ip(),
                            packet,
                            &mut observed_sampling,
                            &mut observed_namespace,
                        );
                    }
                    NetflowPacket::IPFix(packet) => {
                        observe_ipfix_decoder_state_from_packet(
                            source.ip(),
                            packet,
                            &mut observed_sampling,
                            &mut observed_namespace,
                        );
                    }
                    _ => {}
                }
            }
        }

        let mut mismatches = Vec::new();
        record_template_mismatch(
            &mut mismatches,
            "v9 templates",
            &namespace.v9_templates,
            &observed_namespace.v9_templates,
        );
        record_template_mismatch(
            &mut mismatches,
            "v9 options templates",
            &namespace.v9_options_templates,
            &observed_namespace.v9_options_templates,
        );
        record_template_mismatch(
            &mut mismatches,
            "IPFIX templates",
            &namespace.ipfix_templates,
            &observed_namespace.ipfix_templates,
        );
        record_template_mismatch(
            &mut mismatches,
            "IPFIX options templates",
            &namespace.ipfix_options_templates,
            &observed_namespace.ipfix_options_templates,
        );
        record_template_mismatch(
            &mut mismatches,
            "IPFIX v9 templates",
            &namespace.ipfix_v9_templates,
            &observed_namespace.ipfix_v9_templates,
        );
        record_template_mismatch(
            &mut mismatches,
            "IPFIX v9 options templates",
            &namespace.ipfix_v9_options_templates,
            &observed_namespace.ipfix_v9_options_templates,
        );

        if !mismatches.is_empty() {
            return Err(format!(
                "persisted template replay was incomplete for {} / {} from {}: {}",
                key.exporter_ip,
                key.observation_domain_id,
                source,
                mismatches.join("; ")
            ));
        }
        Ok(())
    }
}

fn record_template_mismatch<T: PartialEq>(
    mismatches: &mut Vec<String>,
    label: &str,
    expected: &BTreeMap<u16, T>,
    observed: &BTreeMap<u16, T>,
) {
    if expected != observed {
        mismatches.push(format!(
            "{} expected IDs {:?}, observed IDs {:?}",
            label,
            expected.keys().copied().collect::<Vec<_>>(),
            observed.keys().copied().collect::<Vec<_>>()
        ));
    }
}
