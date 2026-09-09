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
        let mut parser_source_evictions = 0_u64;
        let mut v9_nsel_flowsets_by_packet = Vec::new();
        let is_sflow = is_sflow_payload(payload);

        if let Some(context) = packet_context {
            self.expire_v9_templates(context, input_realtime_usec);
        }

        let mut batch = if is_sflow {
            decode_sflow(
                source,
                payload,
                self.enable_sflow,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
            )
        } else {
            parser_source_evictions = std::mem::take(&mut self.pending_parser_source_evictions);
            let parser_source = packet_context
                .map(|context| context.parser_source)
                .unwrap_or_else(|| normalize_template_scope_source(source));
            let mut removals = Vec::new();
            let result = self.netflow.parse_from_source_with_reporter(
                parser_source,
                payload,
                &mut |removal| {
                    removals.push(removal.source);
                    Ok(())
                },
            );
            parser_source_evictions = parser_source_evictions.saturating_add(removals.len() as u64);
            for removal in removals {
                self.remove_evicted_parser_source(removal);
            }

            if let Some(context) = packet_context {
                let observation = self.observe_decoder_state_from_packets(
                    context,
                    &result.packets,
                    input_realtime_usec,
                );
                template_state_changed = observation.template_state_changed;
                v9_nsel_flowsets_by_packet = observation.v9_nsel_flowsets_by_packet;
            }

            decode_netflow_result(
                result,
                &mut self.sampling,
                &v9_nsel_flowsets_by_packet,
                source,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
                self.enable_v5,
                self.enable_v7,
                self.enable_v9,
                self.enable_ipfix,
                1,
            )
        };

        if !is_sflow {
            batch.stats.parser_source_evictions = batch
                .stats
                .parser_source_evictions
                .saturating_add(parser_source_evictions);
        }

        for flow in &mut batch.flows {
            apply_missing_flow_time_fallback(flow, input_realtime_usec);
        }

        batch.stats.decoded_rows = batch.flows.len() as u64;
        if let Some(enricher) = &mut self.enricher {
            let decoded_rows = batch.flows.len();
            batch
                .flows
                .retain_mut(|flow| enricher.enrich_record(&mut flow.record));
            batch.stats.enrichment_filtered_rows =
                decoded_rows.saturating_sub(batch.flows.len()) as u64;
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
