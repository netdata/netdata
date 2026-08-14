use super::*;

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum DecoderStateProtocol {
    V9,
    Ipfix,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub(crate) struct DecoderStateNamespaceKey {
    pub(crate) protocol: DecoderStateProtocol,
    pub(crate) exporter_ip: String,
    pub(crate) source_port: u16,
    pub(crate) observation_domain_id: u32,
}

impl DecoderStateNamespaceKey {
    pub(crate) fn parser_source(&self, source: SocketAddr) -> SocketAddr {
        let exporter_ip = canonicalize_ip_addr(source.ip());
        match self.protocol {
            DecoderStateProtocol::V9 => SocketAddr::new(exporter_ip, source.port()),
            DecoderStateProtocol::Ipfix => SocketAddr::new(exporter_ip, 0),
        }
    }

    pub(crate) fn from_auto_source(source: AutoSourceKey) -> Option<Self> {
        match source {
            AutoSourceKey::V9(source) => Some(Self {
                protocol: DecoderStateProtocol::V9,
                exporter_ip: canonicalize_ip_addr(source.addr.ip()).to_string(),
                source_port: source.addr.port(),
                observation_domain_id: source.source_id,
            }),
            AutoSourceKey::Ipfix(source) => Some(Self {
                protocol: DecoderStateProtocol::Ipfix,
                exporter_ip: canonicalize_ip_addr(source.addr.ip()).to_string(),
                source_port: 0,
                observation_domain_id: source.observation_domain_id,
            }),
            AutoSourceKey::Legacy(_) => None,
            _ => None,
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DecoderPacketContext {
    pub(crate) version: u16,
    pub(crate) exporter_ip: IpAddr,
    pub(crate) observation_domain_id: u32,
    // Derived once so packet-path callers reuse the same scope without reallocating it.
    pub(crate) parser_source: SocketAddr,
    pub(crate) key: DecoderStateNamespaceKey,
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
pub(crate) enum PersistedV9TemplateDefinition {
    Data {
        fields: Vec<PersistedV9TemplateField>,
    },
    Options {
        scope_fields: Vec<PersistedV9TemplateField>,
        option_fields: Vec<PersistedV9TemplateField>,
    },
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedV9Template {
    pub(crate) template_id: u16,
    pub(crate) received_at_usec: u64,
    pub(crate) nsel: bool,
    pub(crate) definition: PersistedV9TemplateDefinition,
}

impl PersistedV9Template {
    #[cfg(test)]
    pub(crate) fn data_fields(&self) -> Option<&[PersistedV9TemplateField]> {
        match &self.definition {
            PersistedV9TemplateDefinition::Data { fields } => Some(fields),
            PersistedV9TemplateDefinition::Options { .. } => None,
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedIPFixTemplateField {
    pub(crate) field_type: u16,
    pub(crate) field_length: u16,
    pub(crate) enterprise_number: Option<u32>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) enum PersistedIPFixTemplateDefinition {
    Data {
        fields: Vec<PersistedIPFixTemplateField>,
    },
    Options {
        scope_field_count: u16,
        fields: Vec<PersistedIPFixTemplateField>,
    },
    V9Data {
        fields: Vec<PersistedV9TemplateField>,
    },
    V9Options {
        scope_fields: Vec<PersistedV9TemplateField>,
        option_fields: Vec<PersistedV9TemplateField>,
    },
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedIPFixTemplate {
    pub(crate) template_id: u16,
    pub(crate) received_at_usec: u64,
    pub(crate) definition: PersistedIPFixTemplateDefinition,
}

impl PersistedIPFixTemplate {
    #[cfg(test)]
    pub(crate) fn data_fields(&self) -> Option<&[PersistedIPFixTemplateField]> {
        match &self.definition {
            PersistedIPFixTemplateDefinition::Data { fields } => Some(fields),
            _ => None,
        }
    }
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct DecoderStateNamespace {
    pub(crate) v9_templates: BTreeMap<u16, PersistedV9Template>,
    pub(crate) ipfix_templates: BTreeMap<u16, PersistedIPFixTemplate>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub(crate) struct PersistedDecoderNamespaceFile {
    pub(crate) key: DecoderStateNamespaceKey,
    pub(crate) namespace: DecoderStateNamespace,
    pub(crate) sampling_rates: Vec<PersistedSamplingRate>,
}

#[derive(Debug, Clone)]
pub(crate) struct DecoderStateObservation {
    pub(crate) namespace_state_changed: bool,
    pub(crate) template_state_changed: bool,
    pub(crate) dirty_sampling_namespaces: Vec<DecoderStateNamespaceKey>,
    pub(crate) v9_nsel_flowsets: Vec<bool>,
}

#[derive(Debug, Default)]
pub(crate) struct DecoderStateBatchObservation {
    pub(crate) template_state_changed: bool,
    pub(crate) v9_nsel_flowsets_by_packet: Vec<Option<Vec<bool>>>,
}

impl DecoderStateNamespace {
    pub(crate) fn is_empty(&self) -> bool {
        self.v9_templates.is_empty() && self.ipfix_templates.is_empty()
    }

    pub(crate) fn set_v9_template(
        &mut self,
        template_id: u16,
        fields: Vec<PersistedV9TemplateField>,
        received_at_usec: u64,
        nsel: bool,
    ) -> bool {
        let template = PersistedV9Template {
            template_id,
            received_at_usec,
            nsel,
            definition: PersistedV9TemplateDefinition::Data { fields },
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
        received_at_usec: u64,
    ) -> bool {
        let template = PersistedV9Template {
            template_id,
            received_at_usec,
            nsel: false,
            definition: PersistedV9TemplateDefinition::Options {
                scope_fields,
                option_fields,
            },
        };
        self.v9_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    pub(crate) fn set_ipfix_template(
        &mut self,
        template_id: u16,
        fields: Vec<PersistedIPFixTemplateField>,
        received_at_usec: u64,
    ) -> bool {
        let template = PersistedIPFixTemplate {
            template_id,
            received_at_usec,
            definition: PersistedIPFixTemplateDefinition::Data { fields },
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
        received_at_usec: u64,
    ) -> bool {
        let template = PersistedIPFixTemplate {
            template_id,
            received_at_usec,
            definition: PersistedIPFixTemplateDefinition::Options {
                scope_field_count,
                fields,
            },
        };
        self.ipfix_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    pub(crate) fn set_ipfix_v9_template(
        &mut self,
        template_id: u16,
        fields: Vec<PersistedV9TemplateField>,
        received_at_usec: u64,
    ) -> bool {
        let template = PersistedIPFixTemplate {
            template_id,
            received_at_usec,
            definition: PersistedIPFixTemplateDefinition::V9Data { fields },
        };
        self.ipfix_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }

    pub(crate) fn set_ipfix_v9_options_template(
        &mut self,
        template_id: u16,
        scope_fields: Vec<PersistedV9TemplateField>,
        option_fields: Vec<PersistedV9TemplateField>,
        received_at_usec: u64,
    ) -> bool {
        let template = PersistedIPFixTemplate {
            template_id,
            received_at_usec,
            definition: PersistedIPFixTemplateDefinition::V9Options {
                scope_fields,
                option_fields,
            },
        };
        self.ipfix_templates
            .insert(template_id, template.clone())
            .as_ref()
            != Some(&template)
    }
}
