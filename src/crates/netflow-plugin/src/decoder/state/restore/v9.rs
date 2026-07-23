use super::*;

pub(super) fn build_v9_restore_packet(
    key: &DecoderStateNamespaceKey,
    namespace: &DecoderStateNamespace,
) -> Result<Option<Vec<u8>>, String> {
    if key.protocol != DecoderStateProtocol::V9 || namespace.v9_templates.is_empty() {
        return Ok(None);
    }

    let mut data_templates = Vec::new();
    let mut options_templates = Vec::new();
    for template in namespace.v9_templates.values() {
        match &template.definition {
            PersistedV9TemplateDefinition::Data { fields } => {
                data_templates.push(NetflowV9Template {
                    template_id: template.template_id,
                    field_count: fields.len() as u16,
                    fields: fields
                        .iter()
                        .map(|field| NetflowV9TemplateField {
                            field_type_number: field.field_type,
                            field_type: V9Field::from(field.field_type),
                            field_length: field.field_length,
                        })
                        .collect(),
                });
            }
            PersistedV9TemplateDefinition::Options {
                scope_fields,
                option_fields,
            } => {
                options_templates.push(NetflowV9OptionsTemplate {
                    template_id: template.template_id,
                    // RFC 3954 section 6.1 defines descriptor byte lengths.
                    options_scope_length: (scope_fields.len() * 4) as u16,
                    options_length: (option_fields.len() * 4) as u16,
                    scope_fields: scope_fields
                        .iter()
                        .map(|field| NetflowV9OptionsTemplateScopeField {
                            field_type_number: field.field_type,
                            field_type:
                                netflow_parser::variable_versions::v9::lookup::ScopeFieldType::from(
                                    field.field_type,
                                ),
                            field_length: field.field_length,
                        })
                        .collect(),
                    option_fields: option_fields
                        .iter()
                        .map(|field| NetflowV9TemplateField {
                            field_type_number: field.field_type,
                            field_type: V9Field::from(field.field_type),
                            field_length: field.field_length,
                        })
                        .collect(),
                });
            }
        }
    }

    let mut flowsets = Vec::new();
    if !data_templates.is_empty() {
        flowsets.push(NetflowV9FlowSet {
            header: NetflowV9FlowSetHeader {
                flowset_id: 0,
                length: 0,
            },
            body: V9FlowSetBody::Template(NetflowV9Templates {
                templates: data_templates,
                padding: Vec::new(),
            }),
        });
    }
    if !options_templates.is_empty() {
        flowsets.push(NetflowV9FlowSet {
            header: NetflowV9FlowSetHeader {
                flowset_id: 1,
                length: 0,
            },
            body: V9FlowSetBody::OptionsTemplate(NetflowV9OptionsTemplates {
                templates: options_templates,
                padding: Vec::new(),
            }),
        });
    }

    V9 {
        header: NetflowV9Header {
            version: 9,
            count: 0,
            sys_up_time: 0,
            unix_secs: 0,
            sequence_number: 0,
            source_id: key.observation_domain_id,
        },
        flowsets,
    }
    .to_be_bytes()
    .map(Some)
    .map_err(|err| format!("failed to serialize v9 restore packet: {err}"))
}
