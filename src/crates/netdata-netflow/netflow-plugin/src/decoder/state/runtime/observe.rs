use super::*;

impl FlowDecoders {
    pub(crate) fn observe_decoder_state_from_payload(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderStateNamespaceKey> {
        let Some(key) = Self::decoder_state_namespace_key(source, payload) else {
            return None;
        };
        self.loaded_decoder_namespaces.insert(key.clone());
        let namespace = self
            .decoder_state_namespaces
            .entry(key.clone())
            .or_default();

        let observation = match u16::from_be_bytes([payload[0], payload[1]]) {
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
            self.dirty_decoder_namespaces.insert(key.clone());
        }

        observation.template_state_changed.then_some(key)
    }
}
