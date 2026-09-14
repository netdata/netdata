use super::*;

impl FlowDecoders {
    pub(crate) fn decoder_scope_snapshot(&self) -> DecoderScopeSnapshot {
        DecoderScopeSnapshot {
            v9_sources: self.netflow.v9_source_count() as u64,
            ipfix_sources: self.netflow.ipfix_source_count() as u64,
            legacy_sources: self.netflow.legacy_source_count() as u64,
            namespaces: self.decoder_state_namespaces.len() as u64,
            hydrated_sources: self
                .hydrated_namespace_sources
                .values()
                .map(std::collections::HashSet::len)
                .sum::<usize>() as u64,
        }
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn decoder_state_namespace_key(
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderStateNamespaceKey> {
        Self::decoder_packet_context(source, payload).map(|context| context.key)
    }

    pub(crate) fn decoder_packet_context(
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderPacketContext> {
        let (version, observation_domain_id) = template_scope(payload)?;
        let exporter_ip = canonicalize_ip_addr(source.ip());
        let (protocol, source_port, parser_source) = match version {
            9 => (
                DecoderStateProtocol::V9,
                source.port(),
                SocketAddr::new(exporter_ip, source.port()),
            ),
            10 => (
                DecoderStateProtocol::Ipfix,
                0,
                SocketAddr::new(exporter_ip, 0),
            ),
            _ => return None,
        };
        Some(DecoderPacketContext {
            version,
            exporter_ip,
            observation_domain_id,
            parser_source,
            key: DecoderStateNamespaceKey {
                protocol,
                exporter_ip: exporter_ip.to_string(),
                source_port,
                observation_domain_id,
            },
        })
    }

    pub(crate) fn decoder_state_namespace_filename(key: &DecoderStateNamespaceKey) -> String {
        format!(
            "v5--{}--{}--{:05}--{:08x}.bin",
            match key.protocol {
                DecoderStateProtocol::V9 => "v9",
                DecoderStateProtocol::Ipfix => "ipfix",
            },
            sanitize_namespace_filename_component(&key.exporter_ip),
            key.source_port,
            key.observation_domain_id
        )
    }

    pub(crate) fn is_decoder_state_namespace_loaded(&self, key: &DecoderStateNamespaceKey) -> bool {
        self.loaded_decoder_namespaces.contains(key)
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn decoder_state_source_needs_hydration(
        &self,
        key: &DecoderStateNamespaceKey,
        source: SocketAddr,
    ) -> bool {
        let source = key.parser_source(source);
        self.decoder_state_normalized_source_needs_hydration(key, source)
    }

    pub(crate) fn decoder_state_normalized_source_needs_hydration(
        &self,
        key: &DecoderStateNamespaceKey,
        source: SocketAddr,
    ) -> bool {
        !self
            .hydrated_namespace_sources
            .get(key)
            .is_some_and(|sources| sources.contains(&source))
    }

    pub(crate) fn mark_decoder_state_namespace_absent_for_normalized_source(
        &mut self,
        key: DecoderStateNamespaceKey,
        source: SocketAddr,
    ) {
        self.loaded_decoder_namespaces.insert(key.clone());
        self.decoder_state_namespaces
            .entry(key.clone())
            .or_default();
        self.hydrated_namespace_sources
            .entry(key)
            .or_default()
            .insert(source);
    }

    pub(crate) fn dirty_decoder_state_namespaces(&self) -> Vec<DecoderStateNamespaceKey> {
        let mut keys: Vec<_> = self.dirty_decoder_namespaces.iter().cloned().collect();
        keys.sort_by(|left, right| {
            left.protocol
                .cmp(&right.protocol)
                .then(left.exporter_ip.cmp(&right.exporter_ip))
                .then(left.source_port.cmp(&right.source_port))
                .then(left.observation_domain_id.cmp(&right.observation_domain_id))
        });
        keys
    }

    pub(crate) fn mark_decoder_state_namespace_persisted(
        &mut self,
        key: &DecoderStateNamespaceKey,
    ) {
        self.dirty_decoder_namespaces.remove(key);
        if !self.decoder_state_namespaces.contains_key(key)
            && self.sampling.snapshot_namespace(key).is_empty()
        {
            self.loaded_decoder_namespaces.remove(key);
            self.hydrated_namespace_sources.remove(key);
        }
    }

    #[cfg(test)]
    pub(crate) fn decoder_state_namespace_keys(&self) -> Vec<DecoderStateNamespaceKey> {
        let mut keys: Vec<_> = self.decoder_state_namespaces.keys().cloned().collect();
        keys.sort_by(|left, right| {
            left.protocol
                .cmp(&right.protocol)
                .then(left.exporter_ip.cmp(&right.exporter_ip))
                .then(left.source_port.cmp(&right.source_port))
                .then(left.observation_domain_id.cmp(&right.observation_domain_id))
        });
        keys
    }
}

fn sanitize_namespace_filename_component(value: &str) -> String {
    value
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || ch == '_' || ch == '-' {
                ch
            } else {
                '_'
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decoder_state_namespace_filename_sanitizes_untrusted_exporter_ip() {
        let key = DecoderStateNamespaceKey {
            protocol: DecoderStateProtocol::V9,
            exporter_ip: "../fe80::1/../../odd name".to_string(),
            source_port: 2055,
            observation_domain_id: 0x1234_abcd,
        };

        let filename = FlowDecoders::decoder_state_namespace_filename(&key);

        assert_eq!(
            filename,
            "v5--v9--___fe80__1_______odd_name--02055--1234abcd.bin"
        );
    }
}
