use super::*;

impl FlowDecoders {
    pub(crate) fn observe_decoder_state_from_payload(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderStateNamespaceKey> {
        if payload.len() < 2 {
            return None;
        }

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
