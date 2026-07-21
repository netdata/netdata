use super::*;

impl FlowDecoders {
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
        let mut validation_removals = Vec::new();
        Self::replay_namespace_packets_into(
            &mut validation_parser,
            key,
            &namespace,
            source,
            &mut validation_removals,
        )?;
        debug_assert!(validation_removals.is_empty());

        self.replay_namespace_packets(key, &namespace, source)?;
        self.hydrated_namespace_sources
            .entry(key.clone())
            .or_default()
            .insert(source);
        Ok(())
    }

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

        let parser_source = expected_key.parser_source(source);
        let mut validation_parser = super::init::new_netflow_parser(self.max_records_per_flowset);
        let mut validation_removals = Vec::new();
        Self::replay_namespace_packets_into(
            &mut validation_parser,
            &expected_key,
            &persisted.namespace,
            parser_source,
            &mut validation_removals,
        )?;
        debug_assert!(validation_removals.is_empty());
        self.replay_namespace_packets(&expected_key, &persisted.namespace, parser_source)?;

        self.loaded_decoder_namespaces.insert(expected_key.clone());
        self.decoder_state_namespaces
            .insert(expected_key.clone(), persisted.namespace.clone());
        if let Some(namespace) = self.decoder_state_namespaces.get_mut(&expected_key) {
            self.templates.load_namespace(&expected_key, namespace);
        }
        self.dirty_decoder_namespaces.extend(
            self.sampling
                .replace_namespace(&expected_key, &persisted.sampling_rates),
        );
        self.dirty_decoder_namespaces.remove(&expected_key);
        self.hydrated_namespace_sources.remove(&expected_key);
        self.hydrated_namespace_sources
            .entry(expected_key)
            .or_default()
            .insert(parser_source);
        Ok(())
    }

    pub(crate) fn export_decoder_state_namespace(
        &self,
        key: &DecoderStateNamespaceKey,
    ) -> Result<Option<Vec<u8>>, String> {
        let namespace = self
            .decoder_state_namespaces
            .get(key)
            .cloned()
            .unwrap_or_default();
        let sampling_rates = self.sampling.snapshot_namespace(key);
        if namespace.is_empty() && sampling_rates.is_empty() {
            return Ok(None);
        }

        encode_persisted_namespace_file(&PersistedDecoderNamespaceFile {
            key: key.clone(),
            namespace,
            sampling_rates,
        })
        .map(Some)
    }

    pub(crate) fn replay_namespace_packets(
        &mut self,
        key: &DecoderStateNamespaceKey,
        namespace: &DecoderStateNamespace,
        source: SocketAddr,
    ) -> Result<(), String> {
        let mut removals = Vec::new();
        let result = Self::replay_namespace_packets_into(
            &mut self.netflow,
            key,
            namespace,
            source,
            &mut removals,
        );
        self.pending_parser_source_evictions = self
            .pending_parser_source_evictions
            .saturating_add(removals.len() as u64);
        for removal in removals {
            self.remove_evicted_parser_source(removal);
        }
        result
    }

    fn replay_namespace_packets_into(
        parser: &mut AutoScopedParser,
        key: &DecoderStateNamespaceKey,
        namespace: &DecoderStateNamespace,
        source: SocketAddr,
        removals: &mut Vec<AutoSourceKey>,
    ) -> Result<(), String> {
        let mut observed_namespace = DecoderStateNamespace::default();
        let mut observed_sampling = SamplingState::default();
        let mut observed_templates = TemplateState::default();

        for (packet_index, packet) in build_namespace_restore_packets(key, namespace)?
            .into_iter()
            .enumerate()
        {
            let result = parser.parse_from_source_with_reporter(source, &packet, &mut |removal| {
                removals.push(removal.source);
                Ok(())
            });
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
                            source,
                            key,
                            packet,
                            &mut observed_sampling,
                            &mut observed_templates,
                            &mut observed_namespace,
                            0,
                        );
                    }
                    NetflowPacket::IPFix(packet) => {
                        observe_ipfix_decoder_state_from_packet(
                            source,
                            key,
                            packet,
                            &mut observed_sampling,
                            &mut observed_templates,
                            &mut observed_namespace,
                            0,
                        );
                    }
                    _ => {}
                }
            }
        }

        let mut mismatches = Vec::new();
        record_v9_template_mismatch(
            &mut mismatches,
            "v9 templates",
            &namespace.v9_templates,
            &observed_namespace.v9_templates,
        );
        record_ipfix_template_mismatch(
            &mut mismatches,
            "IPFIX templates",
            &namespace.ipfix_templates,
            &observed_namespace.ipfix_templates,
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

fn record_v9_template_mismatch(
    mismatches: &mut Vec<String>,
    label: &str,
    expected: &BTreeMap<u16, PersistedV9Template>,
    observed: &BTreeMap<u16, PersistedV9Template>,
) {
    let matches = expected.len() == observed.len()
        && expected.iter().all(|(id, template)| {
            observed.get(id).is_some_and(|other| {
                template.definition == other.definition && template.nsel == other.nsel
            })
        });
    if !matches {
        mismatches.push(format!(
            "{} expected IDs {:?}, observed IDs {:?}",
            label,
            expected.keys().copied().collect::<Vec<_>>(),
            observed.keys().copied().collect::<Vec<_>>()
        ));
    }
}

fn record_ipfix_template_mismatch(
    mismatches: &mut Vec<String>,
    label: &str,
    expected: &BTreeMap<u16, PersistedIPFixTemplate>,
    observed: &BTreeMap<u16, PersistedIPFixTemplate>,
) {
    let matches = expected.len() == observed.len()
        && expected.iter().all(|(id, template)| {
            observed
                .get(id)
                .is_some_and(|other| template.definition == other.definition)
        });
    if !matches {
        mismatches.push(format!(
            "{} expected IDs {:?}, observed IDs {:?}",
            label,
            expected.keys().copied().collect::<Vec<_>>(),
            observed.keys().copied().collect::<Vec<_>>()
        ));
    }
}
