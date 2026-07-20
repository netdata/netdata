use super::*;

const DEFAULT_V9_TEMPLATE_LIFETIME: Duration = Duration::from_secs(90 * 60);

pub(super) fn new_netflow_parser(max_records_per_flowset: usize) -> AutoScopedParser {
    assert!(
        max_records_per_flowset > 0,
        "maximum records per flowset must be positive"
    );
    AutoScopedParser::try_with_builder(
        NetflowParser::builder().with_max_records_per_flowset(max_records_per_flowset),
    )
    .expect("positive flowset record limit must be a valid parser configuration")
}

pub(crate) struct FlowDecoders {
    pub(crate) netflow: AutoScopedParser,
    pub(crate) sampling: SamplingState,
    pub(crate) templates: TemplateState,
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
    pub(crate) max_records_per_flowset: usize,
    pub(crate) v9_template_lifetime: Option<Duration>,
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
        Self::with_protocols_decap_timestamp_and_packet_limit(
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
            decapsulation_mode,
            timestamp_source,
            u16::MAX as usize,
        )
    }

    pub(crate) fn with_protocols_decap_timestamp_and_packet_limit(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
        decapsulation_mode: DecapsulationMode,
        timestamp_source: TimestampSource,
        max_packet_size: usize,
    ) -> Self {
        Self::with_protocols_decap_timestamp_packet_and_state_limits(
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
            decapsulation_mode,
            timestamp_source,
            max_packet_size,
            Some(DEFAULT_V9_TEMPLATE_LIFETIME),
            DEFAULT_SAMPLING_CACHE_MAX_ENTRIES,
            DEFAULT_SAMPLING_CACHE_MAX_ENTRIES_PER_STREAM,
        )
    }

    #[allow(clippy::too_many_arguments)]
    pub(crate) fn with_protocols_decap_timestamp_packet_and_state_limits(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
        decapsulation_mode: DecapsulationMode,
        timestamp_source: TimestampSource,
        max_packet_size: usize,
        v9_template_lifetime: Option<Duration>,
        sampling_cache_max_entries: usize,
        sampling_cache_max_entries_per_stream: usize,
    ) -> Self {
        Self {
            netflow: new_netflow_parser(max_packet_size),
            sampling: SamplingState::new(
                sampling_cache_max_entries,
                sampling_cache_max_entries_per_stream,
            ),
            templates: TemplateState::default(),
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
            // Every data record consumes at least one byte. The listener's
            // bounded datagram size is therefore also a safe upper bound on
            // records in one flowset.
            max_records_per_flowset: max_packet_size,
            v9_template_lifetime,
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
        self.netflow = new_netflow_parser(self.max_records_per_flowset)
            .with_max_sources(max_sources)
            .expect("test parser source limit must be nonzero");
    }

    #[cfg(test)]
    pub(crate) fn set_v9_template_lifetime_for_test(&mut self, lifetime: Option<Duration>) {
        self.v9_template_lifetime = lifetime;
    }
}
