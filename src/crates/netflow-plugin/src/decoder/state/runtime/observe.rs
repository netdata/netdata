use super::*;

impl FlowDecoders {
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn observe_decoder_state_from_payload(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderStateNamespaceKey> {
        let context = Self::decoder_packet_context(source, payload)?;
        self.observe_decoder_state_from_context(source, payload, &context)
            .then_some(context.key)
    }

    pub(crate) fn observe_decoder_state_from_context(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
        context: &DecoderPacketContext,
    ) -> bool {
        self.loaded_decoder_namespaces.insert(context.key.clone());
        let namespace = self
            .decoder_state_namespaces
            .entry(context.key.clone())
            .or_default();

        let observation = match context.version {
            9 => observe_v9_decoder_state_from_raw_payload(
                source,
                payload,
                &mut self.sampling,
                namespace,
            ),
            10 => observe_ipfix_decoder_state_from_raw_payload(
                source,
                payload,
                &mut self.sampling,
                namespace,
            ),
            _ => DecoderStateObservation {
                namespace_state_changed: false,
                template_state_changed: false,
            },
        };

        if observation.namespace_state_changed {
            self.dirty_decoder_namespaces.insert(context.key.clone());
        }

        observation.template_state_changed
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::{IpAddr, Ipv4Addr};

    #[test]
    fn observe_decoder_state_from_short_payload_returns_none() {
        let mut decoders = FlowDecoders::new();
        let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 10)), 2055);

        assert_eq!(
            decoders.observe_decoder_state_from_payload(source, &[]),
            None
        );
        assert_eq!(
            decoders.observe_decoder_state_from_payload(source, &[9]),
            None
        );
        assert!(
            decoders.decoder_state_namespace_keys().is_empty(),
            "short payloads must not create decoder namespaces"
        );
    }
}
