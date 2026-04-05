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
        let template_state_changed = self.observe_decoder_state_from_payload(source, payload);

        let mut batch = if is_sflow_payload(payload) && self.enable_sflow {
            decode_sflow(
                source,
                payload,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
            )
        } else {
            decode_netflow(
                &mut self.netflow,
                &mut self.sampling,
                source,
                payload,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
                self.enable_v5,
                self.enable_v7,
                self.enable_v9,
                self.enable_ipfix,
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

        if let Some(key) = template_state_changed {
            let hydrated = self.hydrated_namespace_sources.entry(key).or_default();
            hydrated.clear();
            hydrated.insert(source);
        }

        self.stats.merge(&batch.stats);
        batch
    }
}
