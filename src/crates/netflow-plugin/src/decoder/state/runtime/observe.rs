use super::*;

impl FlowDecoders {
    pub(crate) fn observe_decoder_state_from_packets(
        &mut self,
        context: &DecoderPacketContext,
        packets: &[NetflowPacket],
        received_at_usec: u64,
    ) -> DecoderStateBatchObservation {
        self.loaded_decoder_namespaces.insert(context.key.clone());
        let namespace = self
            .decoder_state_namespaces
            .entry(context.key.clone())
            .or_default();
        let mut namespace_state_changed = false;
        let mut template_state_changed = false;
        let mut v9_nsel_flowsets_by_packet = vec![None; packets.len()];

        for (packet_index, packet) in packets.iter().enumerate() {
            let observation = match packet {
                NetflowPacket::V9(packet)
                    if context.version == 9
                        && packet.header.source_id == context.observation_domain_id =>
                {
                    observe_v9_decoder_state_from_packet(
                        context.parser_source,
                        &context.key,
                        packet,
                        &mut self.sampling,
                        &mut self.templates,
                        namespace,
                        received_at_usec,
                    )
                }
                NetflowPacket::IPFix(packet)
                    if context.version == 10
                        && packet.header.observation_domain_id == context.observation_domain_id =>
                {
                    observe_ipfix_decoder_state_from_packet(
                        context.parser_source,
                        &context.key,
                        packet,
                        &mut self.sampling,
                        &mut self.templates,
                        namespace,
                        received_at_usec,
                    )
                }
                _ => continue,
            };
            namespace_state_changed |= observation.namespace_state_changed;
            template_state_changed |= observation.template_state_changed;
            if !observation.v9_nsel_flowsets.is_empty() {
                v9_nsel_flowsets_by_packet[packet_index] = Some(observation.v9_nsel_flowsets);
            }
            self.dirty_decoder_namespaces
                .extend(observation.dirty_sampling_namespaces);
        }

        if namespace_state_changed {
            self.dirty_decoder_namespaces.insert(context.key.clone());
        }

        DecoderStateBatchObservation {
            template_state_changed,
            v9_nsel_flowsets_by_packet,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn observing_no_packets_does_not_dirty_the_namespace() {
        let mut decoders = FlowDecoders::new();
        let context = DecoderPacketContext {
            version: 9,
            exporter_ip: "192.0.2.10".parse().unwrap(),
            observation_domain_id: 7,
            parser_source: "192.0.2.10:2055".parse().unwrap(),
            key: DecoderStateNamespaceKey {
                protocol: DecoderStateProtocol::V9,
                exporter_ip: "192.0.2.10".to_string(),
                source_port: 2055,
                observation_domain_id: 7,
            },
        };

        let observation = decoders.observe_decoder_state_from_packets(&context, &[], 1);
        assert!(!observation.template_state_changed);
        assert!(observation.v9_nsel_flowsets_by_packet.is_empty());
        assert!(decoders.dirty_decoder_state_namespaces().is_empty());
    }
}
