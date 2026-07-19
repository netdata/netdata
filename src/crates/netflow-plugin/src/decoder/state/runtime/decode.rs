use super::*;

impl FlowDecoders {
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn decode_udp_payload(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
    ) -> DecodedBatch {
        self.decode_udp_payload_at(source, payload, now_usec())
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn decode_udp_payload_at(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
        input_realtime_usec: u64,
    ) -> DecodedBatch {
        let packet_context = Self::decoder_packet_context(source, payload);
        self.decode_udp_payload_at_with_context(
            source,
            payload,
            input_realtime_usec,
            packet_context.as_ref(),
        )
    }

    pub(crate) fn decode_udp_payload_at_with_context(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
        input_realtime_usec: u64,
        packet_context: Option<&DecoderPacketContext>,
    ) -> DecodedBatch {
        let computed_context = if packet_context.is_none() {
            Self::decoder_packet_context(source, payload)
        } else {
            None
        };
        let packet_context = packet_context.or(computed_context.as_ref());
        let mut template_state_changed = false;

        let mut batch = if is_sflow_payload(payload) && self.enable_sflow {
            decode_sflow(
                source,
                payload,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
            )
        } else {
            let parser_source = packet_context
                .map(|context| context.parser_source)
                .unwrap_or_else(|| normalize_template_scope_source(source));
            let mut parse_attempts = 1;
            let mut result = self.netflow.parse_from_source(parser_source, payload);
            let missing_ids = missing_template_ids(&result.packets);

            if let Some(context) = packet_context
                && !missing_ids.is_empty()
                && let Some(namespace) = self.decoder_state_namespaces.get(&context.key)
            {
                let subset = namespace.template_subset(context.version, &missing_ids);
                if !subset.is_empty() {
                    match self.replay_namespace_packets(&context.key, &subset, parser_source) {
                        Ok(()) => {
                            result = self.netflow.parse_from_source(parser_source, payload);
                            parse_attempts = 2;
                        }
                        Err(err) => {
                            tracing::warn!(
                                "failed to recover NetFlow templates for {} / {} from {}: {}",
                                context.key.exporter_ip,
                                context.key.observation_domain_id,
                                parser_source,
                                err
                            );
                        }
                    }
                }
            }

            if let Some(context) = packet_context {
                template_state_changed =
                    self.observe_decoder_state_from_packets(context, &result.packets);
            }

            decode_netflow_result(
                result,
                &mut self.sampling,
                source,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
                self.enable_v5,
                self.enable_v7,
                self.enable_v9,
                self.enable_ipfix,
                parse_attempts,
            )
        };

        for flow in &mut batch.flows {
            apply_missing_flow_time_fallback(flow, input_realtime_usec);
        }

        if let Some(enricher) = &mut self.enricher {
            batch
                .flows
                .retain_mut(|flow| enricher.enrich_record(&mut flow.record));
        }

        if template_state_changed {
            debug_assert!(
                packet_context.is_some(),
                "template state changed without decoder packet context"
            );
            if let Some(context) = packet_context {
                let hydrated = self
                    .hydrated_namespace_sources
                    .entry(context.key.clone())
                    .or_default();
                hydrated.clear();
                hydrated.insert(context.parser_source);
            }
        }

        self.stats.merge(&batch.stats);
        batch
    }
}
