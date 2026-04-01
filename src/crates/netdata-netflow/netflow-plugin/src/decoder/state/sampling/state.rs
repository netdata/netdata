use super::*;

impl SamplingState {
    pub(crate) fn clear_namespace(&mut self, exporter_ip: &str, observation_domain_id: u32) {
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

    pub(crate) fn set(
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

    pub(crate) fn get(
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

    pub(crate) fn apply_decoder_state_namespace(
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
