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

        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_datalink_templates
            .entry(scope_key)
            .or_default()
            .insert(template_id, V9DataLinkTemplate { fields });
    }

    pub(crate) fn get_v9_sampling_template(
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

    pub(crate) fn get_v9_datalink_template(
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

        let scope_key = IPFixTemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.ipfix_datalink_templates
            .entry(scope_key)
            .or_default()
            .insert(template_id, IPFixDataLinkTemplate { fields });
    }

    pub(crate) fn get_ipfix_datalink_template(
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

    pub(crate) fn has_any_v9_datalink_templates(&self) -> bool {
        self.v9_datalink_templates.values().any(|m| !m.is_empty())
    }

    pub(crate) fn has_any_ipfix_datalink_templates(&self) -> bool {
        self.ipfix_datalink_templates
            .values()
            .any(|m| !m.is_empty())
    }
}
