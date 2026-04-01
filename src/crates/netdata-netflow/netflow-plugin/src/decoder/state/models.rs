use super::*;

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub(crate) struct DecoderStateNamespaceKey {
    pub(crate) exporter_ip: String,
    pub(crate) observation_domain_id: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedSamplingRate {
    pub(crate) version: u16,
    pub(crate) sampler_id: u64,
    pub(crate) sampling_rate: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedV9TemplateField {
    pub(crate) field_type: u16,
    pub(crate) field_length: u16,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedV9Template {
    pub(crate) template_id: u16,
    pub(crate) fields: Vec<PersistedV9TemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedV9OptionsTemplate {
    pub(crate) template_id: u16,
    pub(crate) scope_fields: Vec<PersistedV9TemplateField>,
    pub(crate) option_fields: Vec<PersistedV9TemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedIPFixTemplateField {
    pub(crate) field_type: u16,
    pub(crate) field_length: u16,
    pub(crate) enterprise_number: Option<u32>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedIPFixTemplate {
    pub(crate) template_id: u16,
    pub(crate) fields: Vec<PersistedIPFixTemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedIPFixOptionsTemplate {
    pub(crate) template_id: u16,
    pub(crate) scope_field_count: u16,
    pub(crate) fields: Vec<PersistedIPFixTemplateField>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct DecoderStateNamespace {
    pub(crate) sampling_rates: BTreeMap<(u16, u64), PersistedSamplingRate>,
    pub(crate) v9_templates: BTreeMap<u16, PersistedV9Template>,
    pub(crate) v9_options_templates: BTreeMap<u16, PersistedV9OptionsTemplate>,
    pub(crate) ipfix_templates: BTreeMap<u16, PersistedIPFixTemplate>,
    pub(crate) ipfix_options_templates: BTreeMap<u16, PersistedIPFixOptionsTemplate>,
    pub(crate) ipfix_v9_templates: BTreeMap<u16, PersistedV9Template>,
    pub(crate) ipfix_v9_options_templates: BTreeMap<u16, PersistedV9OptionsTemplate>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedDecoderNamespaceFile {
    pub(crate) key: DecoderStateNamespaceKey,
    pub(crate) namespace: DecoderStateNamespace,
}

#[derive(Debug, Clone, Copy)]
pub(crate) struct DecoderStateObservation {
    pub(crate) namespace_state_changed: bool,
    pub(crate) template_state_changed: bool,
}

impl DecoderStateNamespace {
    pub(crate) fn is_empty(&self) -> bool {
        self.sampling_rates.is_empty()
            && self.v9_templates.is_empty()
            && self.v9_options_templates.is_empty()
            && self.ipfix_templates.is_empty()
            && self.ipfix_options_templates.is_empty()
            && self.ipfix_v9_templates.is_empty()
            && self.ipfix_v9_options_templates.is_empty()
    }

    pub(crate) fn set_sampling_rate(
        &mut self,
        version: u16,
        sampler_id: u64,
        sampling_rate: u64,
    ) -> bool {
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

    pub(crate) fn set_v9_template(
        &mut self,
        template_id: u16,
        fields: Vec<PersistedV9TemplateField>,
    ) -> bool {
        let template = PersistedV9Template {
            template_id,
            fields,
        };
        self.v9_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    pub(crate) fn set_v9_options_template(
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

    pub(crate) fn set_ipfix_template(
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

    pub(crate) fn set_ipfix_options_template(
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

    pub(crate) fn set_ipfix_v9_template(
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

    pub(crate) fn set_ipfix_v9_options_template(
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
