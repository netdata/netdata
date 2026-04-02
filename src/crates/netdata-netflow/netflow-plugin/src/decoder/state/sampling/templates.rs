use super::*;

impl SamplingState {
    pub(crate) fn set_v9_sampling_template(
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

        self.v9_sampling_templates
            .entry(exporter_ip.to_string())
            .or_default()
            .entry(observation_domain_id)
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

    pub(crate) fn set_v9_datalink_template(
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

        self.v9_datalink_templates
            .entry(exporter_ip.to_string())
            .or_default()
            .entry(observation_domain_id)
            .or_default()
            .insert(template_id, V9DataLinkTemplate { fields });
    }

    pub(crate) fn get_v9_sampling_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<V9SamplingTemplate> {
        self.v9_sampling_templates
            .get(exporter_ip)
            .and_then(|domains| domains.get(&observation_domain_id))
            .and_then(|templates| templates.get(&template_id))
            .cloned()
    }

    pub(crate) fn get_v9_datalink_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<V9DataLinkTemplate> {
        self.v9_datalink_templates
            .get(exporter_ip)
            .and_then(|domains| domains.get(&observation_domain_id))
            .and_then(|templates| templates.get(&template_id))
            .cloned()
    }

    pub(crate) fn set_ipfix_datalink_template(
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

        self.ipfix_datalink_templates
            .entry(exporter_ip.to_string())
            .or_default()
            .entry(observation_domain_id)
            .or_default()
            .insert(template_id, IPFixDataLinkTemplate { fields });
    }

    pub(crate) fn get_ipfix_datalink_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<IPFixDataLinkTemplate> {
        self.ipfix_datalink_templates
            .get(exporter_ip)
            .and_then(|domains| domains.get(&observation_domain_id))
            .and_then(|templates| templates.get(&template_id))
            .cloned()
    }

    pub(crate) fn has_any_v9_datalink_templates(&self) -> bool {
        self.v9_datalink_templates.values().any(|m| !m.is_empty())
    }

    pub(crate) fn has_any_ipfix_datalink_templates(&self) -> bool {
        self.ipfix_datalink_templates
            .values()
            .any(|m| !m.is_empty())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn v9_templates_round_trip_and_clear_per_namespace() {
        let mut state = SamplingState::default();
        let field = V9TemplateField {
            field_type: 1,
            field_length: 8,
        };

        state.set_v9_sampling_template("10.0.0.1", 100, 256, vec![field], vec![field]);
        state.set_v9_datalink_template(
            "10.0.0.1",
            100,
            257,
            vec![V9TemplateField {
                field_type: V9_FIELD_LAYER2_PACKET_SECTION_DATA,
                field_length: 32,
            }],
        );
        state.set_v9_sampling_template("10.0.0.1", 200, 300, vec![field], vec![field]);

        assert!(
            state
                .get_v9_sampling_template("10.0.0.1", 100, 256)
                .is_some()
        );
        assert!(
            state
                .get_v9_datalink_template("10.0.0.1", 100, 257)
                .is_some()
        );
        assert!(
            state
                .get_v9_sampling_template("10.0.0.1", 200, 300)
                .is_some()
        );

        state.clear_namespace("10.0.0.1", 100);

        assert!(
            state
                .get_v9_sampling_template("10.0.0.1", 100, 256)
                .is_none()
        );
        assert!(
            state
                .get_v9_datalink_template("10.0.0.1", 100, 257)
                .is_none()
        );
        assert!(
            state
                .get_v9_sampling_template("10.0.0.1", 200, 300)
                .is_some()
        );
    }

    #[test]
    fn ipfix_templates_round_trip_and_clear_per_namespace() {
        let mut state = SamplingState::default();

        state.set_ipfix_datalink_template(
            "10.0.0.2",
            400,
            512,
            vec![IPFixTemplateField {
                field_type: IPFIX_FIELD_DATALINK_FRAME_SECTION,
                field_length: 64,
                enterprise_number: None,
            }],
        );

        assert!(
            state
                .get_ipfix_datalink_template("10.0.0.2", 400, 512)
                .is_some()
        );

        state.clear_namespace("10.0.0.2", 400);

        assert!(
            state
                .get_ipfix_datalink_template("10.0.0.2", 400, 512)
                .is_none()
        );
    }
}
