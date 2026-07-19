use super::*;

pub(super) fn new_netflow_parser() -> AutoScopedParser {
    AutoScopedParser::new()
}

pub(crate) struct FlowDecoders {
    pub(crate) netflow: AutoScopedParser,
    pub(crate) sampling: SamplingState,
    pub(crate) decoder_state_namespaces: HashMap<DecoderStateNamespaceKey, DecoderStateNamespace>,
    pub(crate) loaded_decoder_namespaces: HashSet<DecoderStateNamespaceKey>,
    pub(crate) dirty_decoder_namespaces: HashSet<DecoderStateNamespaceKey>,
    pub(crate) hydrated_namespace_sources: HashMap<DecoderStateNamespaceKey, HashSet<SocketAddr>>,
    pub(crate) enricher: Option<FlowEnricher>,
    pub(crate) stats: DecodeStats,
    pub(crate) decapsulation_mode: DecapsulationMode,
    pub(crate) timestamp_source: TimestampSource,
    pub(crate) enable_v5: bool,
    pub(crate) enable_v7: bool,
    pub(crate) enable_v9: bool,
    pub(crate) enable_ipfix: bool,
    pub(crate) enable_sflow: bool,
}

impl Default for FlowDecoders {
    fn default() -> Self {
        Self::with_protocols_decap_and_timestamp(
            true,
            true,
            true,
            true,
            true,
            DecapsulationMode::None,
            TimestampSource::Input,
        )
    }
}

impl FlowDecoders {
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn new() -> Self {
        Self::default()
    }

    #[allow(dead_code)]
    pub(crate) fn with_protocols(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
    ) -> Self {
        Self::with_protocols_and_decap(
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
            DecapsulationMode::None,
        )
    }

    #[allow(dead_code)]
    pub(crate) fn with_protocols_and_decap(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
        decapsulation_mode: DecapsulationMode,
    ) -> Self {
        Self::with_protocols_decap_and_timestamp(
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
            decapsulation_mode,
            TimestampSource::Input,
        )
    }

    pub(crate) fn with_protocols_decap_and_timestamp(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
        decapsulation_mode: DecapsulationMode,
        timestamp_source: TimestampSource,
    ) -> Self {
        Self {
            netflow: new_netflow_parser(),
            sampling: SamplingState::default(),
            decoder_state_namespaces: HashMap::new(),
            loaded_decoder_namespaces: HashSet::new(),
            dirty_decoder_namespaces: HashSet::new(),
            hydrated_namespace_sources: HashMap::new(),
            enricher: None,
            stats: DecodeStats::default(),
            decapsulation_mode,
            timestamp_source,
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
        }
    }

    #[allow(dead_code)]
    pub(crate) fn stats(&self) -> DecodeStats {
        self.stats
    }

    pub(crate) fn set_enricher(&mut self, enricher: Option<FlowEnricher>) {
        self.enricher = enricher;
    }

    pub(crate) fn refresh_enrichment_state(&mut self) {
        if let Some(enricher) = &mut self.enricher {
            enricher.refresh_runtime_state();
        }
    }

    #[cfg(test)]
    pub(crate) fn set_parser_source_limit_for_test(&mut self, max_sources: usize) {
        self.netflow = new_netflow_parser()
            .with_max_sources(max_sources)
            .expect("test parser source limit must be nonzero");
    }
}
