#[derive(Debug, Default)]
struct SamplingState {
    by_exporter: HashMap<String, HashMap<SamplingKey, u64>>,
    v9_sampling_templates: HashMap<V9TemplateScopeKey, HashMap<u16, V9SamplingTemplate>>,
    v9_datalink_templates: HashMap<V9TemplateScopeKey, HashMap<u16, V9DataLinkTemplate>>,
    ipfix_datalink_templates: HashMap<IPFixTemplateScopeKey, HashMap<u16, IPFixDataLinkTemplate>>,
}
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
struct SamplingKey {
    version: u16,
    observation_domain_id: u32,
    sampler_id: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct V9TemplateScopeKey {
    exporter_ip: String,
    observation_domain_id: u32,
}

#[derive(Debug, Clone, Copy)]
struct V9TemplateField {
    field_type: u16,
    field_length: usize,
}

#[derive(Debug, Clone)]
struct V9SamplingTemplate {
    scope_fields: Vec<V9TemplateField>,
    option_fields: Vec<V9TemplateField>,
    record_length: usize,
}

#[derive(Debug, Clone)]
struct V9DataLinkTemplate {
    fields: Vec<V9TemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct IPFixTemplateScopeKey {
    exporter_ip: String,
    observation_domain_id: u32,
}

#[derive(Debug, Clone, Copy)]
struct IPFixTemplateField {
    field_type: u16,
    field_length: u16,
    enterprise_number: Option<u32>,
}

#[derive(Debug, Clone)]
struct IPFixDataLinkTemplate {
    fields: Vec<IPFixTemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub(crate) struct DecoderStateNamespaceKey {
    exporter_ip: String,
    observation_domain_id: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedSamplingRate {
    version: u16,
    sampler_id: u64,
    sampling_rate: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedV9TemplateField {
    field_type: u16,
    field_length: u16,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedV9Template {
    template_id: u16,
    fields: Vec<PersistedV9TemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedV9OptionsTemplate {
    template_id: u16,
    scope_fields: Vec<PersistedV9TemplateField>,
    option_fields: Vec<PersistedV9TemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedIPFixTemplateField {
    field_type: u16,
    field_length: u16,
    enterprise_number: Option<u32>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedIPFixTemplate {
    template_id: u16,
    fields: Vec<PersistedIPFixTemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedIPFixOptionsTemplate {
    template_id: u16,
    scope_field_count: u16,
    fields: Vec<PersistedIPFixTemplateField>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
struct DecoderStateNamespace {
    sampling_rates: BTreeMap<(u16, u64), PersistedSamplingRate>,
    v9_templates: BTreeMap<u16, PersistedV9Template>,
    v9_options_templates: BTreeMap<u16, PersistedV9OptionsTemplate>,
    ipfix_templates: BTreeMap<u16, PersistedIPFixTemplate>,
    ipfix_options_templates: BTreeMap<u16, PersistedIPFixOptionsTemplate>,
    ipfix_v9_templates: BTreeMap<u16, PersistedV9Template>,
    ipfix_v9_options_templates: BTreeMap<u16, PersistedV9OptionsTemplate>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct PersistedDecoderNamespaceFile {
    key: DecoderStateNamespaceKey,
    namespace: DecoderStateNamespace,
}

#[derive(Debug, Clone, Copy)]
struct DecoderStateObservation {
    namespace_state_changed: bool,
    template_state_changed: bool,
}

impl DecoderStateNamespace {
    fn is_empty(&self) -> bool {
        self.sampling_rates.is_empty()
            && self.v9_templates.is_empty()
            && self.v9_options_templates.is_empty()
            && self.ipfix_templates.is_empty()
            && self.ipfix_options_templates.is_empty()
            && self.ipfix_v9_templates.is_empty()
            && self.ipfix_v9_options_templates.is_empty()
    }

    fn set_sampling_rate(&mut self, version: u16, sampler_id: u64, sampling_rate: u64) -> bool {
        let row = PersistedSamplingRate {
            version,
            sampler_id,
            sampling_rate,
        };
        self.sampling_rates
            .insert((version, sampler_id), row)
            .as_ref()
            != Some(&PersistedSamplingRate {
                version,
                sampler_id,
                sampling_rate,
            })
    }

    fn set_v9_template(&mut self, template_id: u16, fields: Vec<PersistedV9TemplateField>) -> bool {
        let template = PersistedV9Template {
            template_id,
            fields,
        };
        self.v9_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    fn set_v9_options_template(
        &mut self,
        template_id: u16,
        scope_fields: Vec<PersistedV9TemplateField>,
        option_fields: Vec<PersistedV9TemplateField>,
    ) -> bool {
        let template = PersistedV9OptionsTemplate {
            template_id,
            scope_fields,
            option_fields,
        };
        self.v9_options_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    fn set_ipfix_template(
        &mut self,
        template_id: u16,
        fields: Vec<PersistedIPFixTemplateField>,
    ) -> bool {
        let template = PersistedIPFixTemplate {
            template_id,
            fields,
        };
        self.ipfix_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    fn set_ipfix_options_template(
        &mut self,
        template_id: u16,
        scope_field_count: u16,
        fields: Vec<PersistedIPFixTemplateField>,
    ) -> bool {
        let template = PersistedIPFixOptionsTemplate {
            template_id,
            scope_field_count,
            fields,
        };
        self.ipfix_options_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    fn set_ipfix_v9_template(
        &mut self,
        template_id: u16,
        fields: Vec<PersistedV9TemplateField>,
    ) -> bool {
        let template = PersistedV9Template {
            template_id,
            fields,
        };
        self.ipfix_v9_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    fn set_ipfix_v9_options_template(
        &mut self,
        template_id: u16,
        scope_fields: Vec<PersistedV9TemplateField>,
        option_fields: Vec<PersistedV9TemplateField>,
    ) -> bool {
        let template = PersistedV9OptionsTemplate {
            template_id,
            scope_fields,
            option_fields,
        };
        self.ipfix_v9_options_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }
}

impl SamplingState {
    fn clear_namespace(&mut self, exporter_ip: &str, observation_domain_id: u32) {
        if let Some(rates) = self.by_exporter.get_mut(exporter_ip) {
            rates.retain(|key, _| key.observation_domain_id != observation_domain_id);
            if rates.is_empty() {
                self.by_exporter.remove(exporter_ip);
            }
        }

        let v9_scope = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        let ipfix_scope = IPFixTemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };

        self.v9_sampling_templates.remove(&v9_scope);
        self.v9_datalink_templates.remove(&v9_scope);
        self.ipfix_datalink_templates.remove(&ipfix_scope);
    }

    fn set(
        &mut self,
        exporter_ip: &str,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
        sampling_rate: u64,
    ) {
        if sampling_rate == 0 {
            return;
        }
        let key = SamplingKey {
            version,
            observation_domain_id,
            sampler_id,
        };
        self.by_exporter
            .entry(exporter_ip.to_string())
            .or_default()
            .insert(key, sampling_rate);
    }

    fn get(
        &self,
        exporter_ip: &str,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
    ) -> Option<u64> {
        let map = self.by_exporter.get(exporter_ip)?;
        map.get(&SamplingKey {
            version,
            observation_domain_id,
            sampler_id,
        })
        .copied()
    }

    fn set_v9_sampling_template(
        &mut self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
        scope_fields: Vec<V9TemplateField>,
        option_fields: Vec<V9TemplateField>,
    ) {
        if option_fields.is_empty() {
            return;
        }

        let record_length = scope_fields
            .iter()
            .chain(option_fields.iter())
            .fold(0_usize, |acc, field| acc.saturating_add(field.field_length));
        if record_length == 0 {
            return;
        }

        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_sampling_templates
            .entry(scope_key)
            .or_default()
            .insert(
                template_id,
                V9SamplingTemplate {
                    scope_fields,
                    option_fields,
                    record_length,
                },
            );
    }

    fn set_v9_datalink_template(
        &mut self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
        fields: Vec<V9TemplateField>,
    ) {
        if !fields
            .iter()
            .any(|f| f.field_type == V9_FIELD_LAYER2_PACKET_SECTION_DATA)
        {
            return;
        }

        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_datalink_templates
            .entry(scope_key)
            .or_default()
            .insert(template_id, V9DataLinkTemplate { fields });
    }

    fn get_v9_sampling_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<V9SamplingTemplate> {
        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_sampling_templates
            .get(&scope_key)
            .and_then(|m| m.get(&template_id))
            .cloned()
    }

    fn get_v9_datalink_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<V9DataLinkTemplate> {
        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_datalink_templates
            .get(&scope_key)
            .and_then(|m| m.get(&template_id))
            .cloned()
    }

    fn set_ipfix_datalink_template(
        &mut self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
        fields: Vec<IPFixTemplateField>,
    ) {
        if !fields.iter().any(|f| {
            (f.enterprise_number.is_none()
                && (f.field_type == IPFIX_FIELD_DATALINK_FRAME_SECTION
                    || is_ipfix_mpls_label_field(f.field_type)))
                || (f.enterprise_number == Some(JUNIPER_PEN)
                    && f.field_type == JUNIPER_COMMON_PROPERTIES_ID)
        }) {
            return;
        }

        let scope_key = IPFixTemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.ipfix_datalink_templates
            .entry(scope_key)
            .or_default()
            .insert(template_id, IPFixDataLinkTemplate { fields });
    }

    fn get_ipfix_datalink_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<IPFixDataLinkTemplate> {
        let scope_key = IPFixTemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.ipfix_datalink_templates
            .get(&scope_key)
            .and_then(|m| m.get(&template_id))
            .cloned()
    }

    fn has_any_v9_datalink_templates(&self) -> bool {
        self.v9_datalink_templates.values().any(|m| !m.is_empty())
    }

    fn has_any_ipfix_datalink_templates(&self) -> bool {
        self.ipfix_datalink_templates
            .values()
            .any(|m| !m.is_empty())
    }

    fn apply_decoder_state_namespace(
        &mut self,
        key: &DecoderStateNamespaceKey,
        namespace: &DecoderStateNamespace,
    ) {
        self.clear_namespace(&key.exporter_ip, key.observation_domain_id);

        for row in namespace.sampling_rates.values() {
            self.set(
                &key.exporter_ip,
                row.version,
                key.observation_domain_id,
                row.sampler_id,
                row.sampling_rate,
            );
        }

        for row in namespace.v9_options_templates.values() {
            self.set_v9_sampling_template(
                &key.exporter_ip,
                key.observation_domain_id,
                row.template_id,
                row.scope_fields
                    .iter()
                    .map(|f| V9TemplateField {
                        field_type: f.field_type,
                        field_length: usize::from(f.field_length),
                    })
                    .collect(),
                row.option_fields
                    .iter()
                    .map(|f| V9TemplateField {
                        field_type: f.field_type,
                        field_length: usize::from(f.field_length),
                    })
                    .collect(),
            );
        }

        for row in namespace.v9_templates.values() {
            self.set_v9_datalink_template(
                &key.exporter_ip,
                key.observation_domain_id,
                row.template_id,
                row.fields
                    .iter()
                    .map(|f| V9TemplateField {
                        field_type: f.field_type,
                        field_length: usize::from(f.field_length),
                    })
                    .collect(),
            );
        }

        for row in namespace.ipfix_templates.values() {
            self.set_ipfix_datalink_template(
                &key.exporter_ip,
                key.observation_domain_id,
                row.template_id,
                row.fields
                    .iter()
                    .map(|f| IPFixTemplateField {
                        field_type: f.field_type,
                        field_length: f.field_length,
                        enterprise_number: f.enterprise_number,
                    })
                    .collect(),
            );
        }
    }
}

pub(crate) struct FlowDecoders {
    netflow: AutoScopedParser,
    sampling: SamplingState,
    decoder_state_namespaces: HashMap<DecoderStateNamespaceKey, DecoderStateNamespace>,
    loaded_decoder_namespaces: HashSet<DecoderStateNamespaceKey>,
    dirty_decoder_namespaces: HashSet<DecoderStateNamespaceKey>,
    hydrated_namespace_sources: HashMap<DecoderStateNamespaceKey, HashSet<SocketAddr>>,
    enricher: Option<FlowEnricher>,
    stats: DecodeStats,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    enable_v5: bool,
    enable_v7: bool,
    enable_v9: bool,
    enable_ipfix: bool,
    enable_sflow: bool,
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
            netflow: AutoScopedParser::new(),
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

    pub(crate) fn decoder_state_namespace_key(
        source: SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderStateNamespaceKey> {
        let (_version, observation_domain_id) = template_scope(payload)?;
        Some(DecoderStateNamespaceKey {
            exporter_ip: source.ip().to_string(),
            observation_domain_id,
        })
    }

    pub(crate) fn decoder_state_namespace_filename(key: &DecoderStateNamespaceKey) -> String {
        format!(
            "{}--{:08x}.bin",
            key.exporter_ip
                .chars()
                .map(|ch| if ch == '.' || ch == ':' { '_' } else { ch })
                .collect::<String>(),
            key.observation_domain_id
        )
    }

    pub(crate) fn is_decoder_state_namespace_loaded(&self, key: &DecoderStateNamespaceKey) -> bool {
        self.loaded_decoder_namespaces.contains(key)
    }

    pub(crate) fn decoder_state_source_needs_hydration(
        &self,
        key: &DecoderStateNamespaceKey,
        source: SocketAddr,
    ) -> bool {
        !self
            .hydrated_namespace_sources
            .get(key)
            .is_some_and(|sources| sources.contains(&source))
    }

    pub(crate) fn mark_decoder_state_namespace_absent(
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

    pub(crate) fn hydrate_loaded_decoder_state_namespace(
        &mut self,
        key: &DecoderStateNamespaceKey,
        source: SocketAddr,
    ) -> Result<(), String> {
        let Some(namespace) = self.decoder_state_namespaces.get(key).cloned() else {
            self.hydrated_namespace_sources
                .entry(key.clone())
                .or_default()
                .insert(source);
            return Ok(());
        };

        self.sampling.apply_decoder_state_namespace(key, &namespace);
        self.replay_namespace_packets(key, &namespace, source)?;
        self.hydrated_namespace_sources
            .entry(key.clone())
            .or_default()
            .insert(source);
        Ok(())
    }

    #[cfg(test)]
    pub(crate) fn import_decoder_state_namespace(
        &mut self,
        expected_key: DecoderStateNamespaceKey,
        source: SocketAddr,
        data: &[u8],
    ) -> Result<(), String> {
        let persisted = decode_persisted_namespace_file(data)?;
        if persisted.key != expected_key {
            return Err(format!(
                "namespace key mismatch in persisted decoder state (got {} / {}, expected {} / {})",
                persisted.key.exporter_ip,
                persisted.key.observation_domain_id,
                expected_key.exporter_ip,
                expected_key.observation_domain_id
            ));
        }

        self.loaded_decoder_namespaces.insert(expected_key.clone());
        self.decoder_state_namespaces
            .insert(expected_key.clone(), persisted.namespace.clone());
        self.dirty_decoder_namespaces.remove(&expected_key);
        self.hydrated_namespace_sources.remove(&expected_key);
        self.hydrate_loaded_decoder_state_namespace(&expected_key, source)
    }

    pub(crate) fn preload_decoder_state_namespace(
        &mut self,
        data: &[u8],
    ) -> Result<DecoderStateNamespaceKey, String> {
        let persisted = decode_persisted_namespace_file(data)?;
        let key = persisted.key.clone();
        self.loaded_decoder_namespaces.insert(key.clone());
        self.decoder_state_namespaces
            .insert(key.clone(), persisted.namespace);
        self.dirty_decoder_namespaces.remove(&key);
        self.hydrated_namespace_sources.remove(&key);
        Ok(key)
    }

    pub(crate) fn dirty_decoder_state_namespaces(&self) -> Vec<DecoderStateNamespaceKey> {
        let mut keys: Vec<_> = self.dirty_decoder_namespaces.iter().cloned().collect();
        keys.sort_by(|left, right| {
            left.exporter_ip
                .cmp(&right.exporter_ip)
                .then(left.observation_domain_id.cmp(&right.observation_domain_id))
        });
        keys
    }

    pub(crate) fn export_decoder_state_namespace(
        &self,
        key: &DecoderStateNamespaceKey,
    ) -> Result<Option<Vec<u8>>, String> {
        let Some(namespace) = self.decoder_state_namespaces.get(key) else {
            return Ok(None);
        };
        if namespace.is_empty() {
            return Ok(None);
        }

        encode_persisted_namespace_file(&PersistedDecoderNamespaceFile {
            key: key.clone(),
            namespace: namespace.clone(),
        })
        .map(Some)
    }

    pub(crate) fn mark_decoder_state_namespace_persisted(
        &mut self,
        key: &DecoderStateNamespaceKey,
    ) {
        self.dirty_decoder_namespaces.remove(key);
    }

    #[cfg(test)]
    fn decoder_state_namespace_keys(&self) -> Vec<DecoderStateNamespaceKey> {
        let mut keys: Vec<_> = self.decoder_state_namespaces.keys().cloned().collect();
        keys.sort_by(|left, right| {
            left.exporter_ip
                .cmp(&right.exporter_ip)
                .then(left.observation_domain_id.cmp(&right.observation_domain_id))
        });
        keys
    }

    fn observe_decoder_state_from_payload(
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

    fn replay_namespace_packets(
        &mut self,
        key: &DecoderStateNamespaceKey,
        namespace: &DecoderStateNamespace,
        source: SocketAddr,
    ) -> Result<(), String> {
        for packet in build_namespace_restore_packets(key, namespace)? {
            let _ = self.netflow.parse_from_source(source, &packet);
        }
        Ok(())
    }
}

fn decoder_state_bincode_options() -> impl Options {
    bincode::DefaultOptions::new()
        .with_fixint_encoding()
        .with_little_endian()
}

fn xxhash64(data: &[u8]) -> u64 {
    let mut hasher = XxHash64::default();
    hasher.write(data);
    hasher.finish()
}

fn encode_persisted_namespace_file(
    file: &PersistedDecoderNamespaceFile,
) -> Result<Vec<u8>, String> {
    let payload = decoder_state_bincode_options()
        .serialize(file)
        .map_err(|err| format!("failed to encode decoder namespace state: {err}"))?;
    let payload_hash = xxhash64(&payload);
    let payload_len = payload.len() as u64;

    let mut out = Vec::with_capacity(DECODER_STATE_HEADER_LEN + payload.len());
    out.extend_from_slice(DECODER_STATE_MAGIC);
    out.extend_from_slice(&DECODER_STATE_SCHEMA_VERSION.to_le_bytes());
    out.extend_from_slice(&payload_hash.to_le_bytes());
    out.extend_from_slice(&payload_len.to_le_bytes());
    out.extend_from_slice(&payload);
    Ok(out)
}

fn decode_persisted_namespace_file(data: &[u8]) -> Result<PersistedDecoderNamespaceFile, String> {
    if data.len() < DECODER_STATE_HEADER_LEN {
        return Err("truncated decoder namespace state header".to_string());
    }
    if &data[..4] != DECODER_STATE_MAGIC {
        return Err("invalid decoder namespace state magic".to_string());
    }

    let version = u32::from_le_bytes(data[4..8].try_into().unwrap());
    if version != DECODER_STATE_SCHEMA_VERSION {
        return Err(format!(
            "unsupported decoder namespace schema version {} (expected {})",
            version, DECODER_STATE_SCHEMA_VERSION
        ));
    }

    let expected_hash = u64::from_le_bytes(data[8..16].try_into().unwrap());
    let payload_len = u64::from_le_bytes(data[16..24].try_into().unwrap()) as usize;
    let payload = &data[DECODER_STATE_HEADER_LEN..];
    if payload.len() != payload_len {
        return Err(format!(
            "decoder namespace payload length mismatch (header {}, actual {})",
            payload_len,
            payload.len()
        ));
    }

    let actual_hash = xxhash64(payload);
    if actual_hash != expected_hash {
        return Err(format!(
            "decoder namespace payload hash mismatch (expected {}, got {})",
            expected_hash, actual_hash
        ));
    }

    decoder_state_bincode_options()
        .deserialize(payload)
        .map_err(|err| format!("failed to decode decoder namespace state: {err}"))
}

fn build_namespace_restore_packets(
    key: &DecoderStateNamespaceKey,
    namespace: &DecoderStateNamespace,
) -> Result<Vec<Vec<u8>>, String> {
    let mut packets = Vec::new();

    if !namespace.v9_templates.is_empty() || !namespace.v9_options_templates.is_empty() {
        let mut flowsets = Vec::new();
        if !namespace.v9_templates.is_empty() {
            flowsets.push(NetflowV9FlowSet {
                header: NetflowV9FlowSetHeader {
                    flowset_id: 0,
                    length: 0,
                },
                body: V9FlowSetBody::Template(NetflowV9Templates {
                    templates: namespace
                        .v9_templates
                        .values()
                        .map(|template| NetflowV9Template {
                            template_id: template.template_id,
                            field_count: template.fields.len() as u16,
                            fields: template
                                .fields
                                .iter()
                                .map(|field| NetflowV9TemplateField {
                                    field_type_number: field.field_type,
                                    field_type: V9Field::from(field.field_type),
                                    field_length: field.field_length,
                                })
                                .collect(),
                        })
                        .collect(),
                    padding: Vec::new(),
                }),
            });
        }
        if !namespace.v9_options_templates.is_empty() {
            flowsets.push(NetflowV9FlowSet {
                header: NetflowV9FlowSetHeader {
                    flowset_id: 1,
                    length: 0,
                },
                body: V9FlowSetBody::OptionsTemplate(NetflowV9OptionsTemplates {
                    templates: namespace
                        .v9_options_templates
                        .values()
                        .map(|template| NetflowV9OptionsTemplate {
                            template_id: template.template_id,
                            options_scope_length: (template.scope_fields.len() * 4) as u16,
                            options_length: (template.option_fields.len() * 4) as u16,
                            scope_fields: template
                                .scope_fields
                                .iter()
                                .map(|field| NetflowV9OptionsTemplateScopeField {
                                    field_type_number: field.field_type,
                                    field_type: netflow_parser::variable_versions::v9_lookup::ScopeFieldType::from(field.field_type),
                                    field_length: field.field_length,
                                })
                                .collect(),
                            option_fields: template
                                .option_fields
                                .iter()
                                .map(|field| NetflowV9TemplateField {
                                    field_type_number: field.field_type,
                                    field_type: V9Field::from(field.field_type),
                                    field_length: field.field_length,
                                })
                                .collect(),
                        })
                        .collect(),
                    padding: Vec::new(),
                }),
            });
        }

        let packet = V9 {
            header: NetflowV9Header {
                version: 9,
                count: 0,
                sys_up_time: 0,
                unix_secs: 0,
                sequence_number: 0,
                source_id: key.observation_domain_id,
            },
            flowsets,
        };
        packets.push(
            packet
                .to_be_bytes()
                .map_err(|err| format!("failed to serialize v9 restore packet: {err}"))?,
        );
    }

    if !namespace.ipfix_templates.is_empty()
        || !namespace.ipfix_options_templates.is_empty()
        || !namespace.ipfix_v9_templates.is_empty()
        || !namespace.ipfix_v9_options_templates.is_empty()
    {
        let mut flowsets = Vec::new();
        if !namespace.ipfix_templates.is_empty() {
            flowsets.push(NetflowIPFixFlowSet {
                header: NetflowIPFixFlowSetHeader {
                    header_id: 2,
                    length: 0,
                },
                body: IPFixFlowSetBody::Templates(
                    namespace
                        .ipfix_templates
                        .values()
                        .map(|template| NetflowIPFixTemplate {
                            template_id: template.template_id,
                            field_count: template.fields.len() as u16,
                            fields: template
                                .fields
                                .iter()
                                .map(|field| NetflowIPFixTemplateField {
                                    field_type_number: field.field_type
                                        | if field.enterprise_number.is_some() {
                                            0x8000
                                        } else {
                                            0
                                        },
                                    field_length: field.field_length,
                                    enterprise_number: field.enterprise_number,
                                    field_type: IPFixField::new(
                                        field.field_type,
                                        field.enterprise_number,
                                    ),
                                })
                                .collect(),
                        })
                        .collect(),
                ),
            });
        }
        if !namespace.ipfix_options_templates.is_empty() {
            flowsets.push(NetflowIPFixFlowSet {
                header: NetflowIPFixFlowSetHeader {
                    header_id: 3,
                    length: 0,
                },
                body: IPFixFlowSetBody::OptionsTemplates(
                    namespace
                        .ipfix_options_templates
                        .values()
                        .map(|template| NetflowIPFixOptionsTemplate {
                            template_id: template.template_id,
                            field_count: template.fields.len() as u16,
                            scope_field_count: template.scope_field_count,
                            fields: template
                                .fields
                                .iter()
                                .map(|field| NetflowIPFixTemplateField {
                                    field_type_number: field.field_type
                                        | if field.enterprise_number.is_some() {
                                            0x8000
                                        } else {
                                            0
                                        },
                                    field_length: field.field_length,
                                    enterprise_number: field.enterprise_number,
                                    field_type: IPFixField::new(
                                        field.field_type,
                                        field.enterprise_number,
                                    ),
                                })
                                .collect(),
                        })
                        .collect(),
                ),
            });
        }
        if !namespace.ipfix_v9_templates.is_empty() {
            flowsets.push(NetflowIPFixFlowSet {
                header: NetflowIPFixFlowSetHeader {
                    header_id: 0,
                    length: 0,
                },
                body: IPFixFlowSetBody::V9Templates(
                    namespace
                        .ipfix_v9_templates
                        .values()
                        .map(|template| NetflowV9Template {
                            template_id: template.template_id,
                            field_count: template.fields.len() as u16,
                            fields: template
                                .fields
                                .iter()
                                .map(|field| NetflowV9TemplateField {
                                    field_type_number: field.field_type,
                                    field_type: V9Field::from(field.field_type),
                                    field_length: field.field_length,
                                })
                                .collect(),
                        })
                        .collect(),
                ),
            });
        }
        if !namespace.ipfix_v9_options_templates.is_empty() {
            flowsets.push(NetflowIPFixFlowSet {
                header: NetflowIPFixFlowSetHeader {
                    header_id: 1,
                    length: 0,
                },
                body: IPFixFlowSetBody::V9OptionsTemplates(
                    namespace
                        .ipfix_v9_options_templates
                        .values()
                        .map(|template| NetflowV9OptionsTemplate {
                            template_id: template.template_id,
                            options_scope_length: (template.scope_fields.len() * 4) as u16,
                            options_length: (template.option_fields.len() * 4) as u16,
                            scope_fields: template
                                .scope_fields
                                .iter()
                                .map(|field| NetflowV9OptionsTemplateScopeField {
                                    field_type_number: field.field_type,
                                    field_type: netflow_parser::variable_versions::v9_lookup::ScopeFieldType::from(field.field_type),
                                    field_length: field.field_length,
                                })
                                .collect(),
                            option_fields: template
                                .option_fields
                                .iter()
                                .map(|field| NetflowV9TemplateField {
                                    field_type_number: field.field_type,
                                    field_type: V9Field::from(field.field_type),
                                    field_length: field.field_length,
                                })
                                .collect(),
                        })
                        .collect(),
                ),
            });
        }

        let packet = IPFix {
            header: NetflowIPFixHeader {
                version: 10,
                length: 0,
                export_time: 0,
                sequence_number: 0,
                observation_domain_id: key.observation_domain_id,
            },
            flowsets,
        };
        packets.push(
            packet
                .to_be_bytes()
                .map_err(|err| format!("failed to serialize ipfix restore packet: {err}"))?,
        );
    }

    Ok(packets)
}
