use super::*;

impl FlowDecoders {
    pub(super) fn expire_v9_templates(&mut self, context: &DecoderPacketContext, now_usec: u64) {
        if context.key.protocol != DecoderStateProtocol::V9 {
            return;
        }
        let Some(lifetime) = self.v9_template_lifetime else {
            return;
        };
        let lifetime_usec = u64::try_from(lifetime.as_micros()).unwrap_or(u64::MAX);
        let Some(namespace) = self.decoder_state_namespaces.get_mut(&context.key) else {
            return;
        };
        let before = namespace.v9_templates.len();
        let mut expired = Vec::new();
        namespace.v9_templates.retain(|template_id, template| {
            let keep = now_usec.saturating_sub(template.received_at_usec) < lifetime_usec;
            if !keep {
                expired.push((*template_id, v9_kind(template)));
            }
            keep
        });
        if namespace.v9_templates.len() == before {
            return;
        }
        for (template_id, kind) in expired {
            self.templates
                .remove_template(&context.key, kind, template_id);
        }

        let key = V9SourceKey {
            addr: context.parser_source,
            source_id: context.observation_domain_id,
        };
        self.netflow.remove_v9_source(&key);
        self.hydrated_namespace_sources.remove(&context.key);
        self.dirty_decoder_namespaces.insert(context.key.clone());

        let surviving = namespace.clone();
        if surviving.v9_templates.is_empty() {
            return;
        }
        match self.replay_namespace_packets(&context.key, &surviving, context.parser_source) {
            Ok(()) => {
                self.hydrated_namespace_sources
                    .entry(context.key.clone())
                    .or_default()
                    .insert(context.parser_source);
            }
            Err(err) => tracing::warn!(
                "failed to rebuild unexpired NetFlow v9 templates for {} / {} from {}: {}",
                context.key.exporter_ip,
                context.key.observation_domain_id,
                context.parser_source,
                err
            ),
        }
    }

    pub(super) fn remove_evicted_parser_source(&mut self, source: AutoSourceKey) {
        let Some(key) = DecoderStateNamespaceKey::from_auto_source(source) else {
            return;
        };

        self.decoder_state_namespaces.remove(&key);
        self.sampling.clear_namespace(&key);
        self.templates.remove_namespace(&key);
        self.hydrated_namespace_sources.remove(&key);
        // Keep a short-lived absent marker until persistence removes any stale
        // file. This prevents the next datagram from restoring an evicted source.
        self.loaded_decoder_namespaces.insert(key.clone());
        self.dirty_decoder_namespaces.insert(key);
    }
}
