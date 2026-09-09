use super::*;

fn ipfix_field(field: &PersistedIPFixTemplateField) -> NetflowIPFixTemplateField {
    NetflowIPFixTemplateField {
        field_type_number: field.field_type
            | if field.enterprise_number.is_some() {
                0x8000
            } else {
                0
            },
        field_length: field.field_length,
        enterprise_number: field.enterprise_number,
        field_type: IPFixField::new(field.field_type, field.enterprise_number),
    }
}

fn v9_field(field: &PersistedV9TemplateField) -> NetflowV9TemplateField {
    NetflowV9TemplateField {
        field_type_number: field.field_type,
        field_type: V9Field::from(field.field_type),
        field_length: field.field_length,
    }
}

pub(super) fn build_ipfix_restore_packet(
    key: &DecoderStateNamespaceKey,
    namespace: &DecoderStateNamespace,
) -> Result<Option<Vec<u8>>, String> {
    if key.protocol != DecoderStateProtocol::Ipfix || namespace.ipfix_templates.is_empty() {
        return Ok(None);
    }

    let mut data = Vec::new();
    let mut options = Vec::new();
    let mut v9_data = Vec::new();
    let mut v9_options = Vec::new();
    for template in namespace.ipfix_templates.values() {
        match &template.definition {
            PersistedIPFixTemplateDefinition::Data { fields } => data.push(NetflowIPFixTemplate {
                template_id: template.template_id,
                field_count: fields.len() as u16,
                fields: fields.iter().map(ipfix_field).collect(),
            }),
            PersistedIPFixTemplateDefinition::Options {
                scope_field_count,
                fields,
            } => options.push(NetflowIPFixOptionsTemplate {
                template_id: template.template_id,
                field_count: fields.len() as u16,
                scope_field_count: *scope_field_count,
                fields: fields.iter().map(ipfix_field).collect(),
            }),
            PersistedIPFixTemplateDefinition::V9Data { fields } => {
                v9_data.push(NetflowV9Template {
                    template_id: template.template_id,
                    field_count: fields.len() as u16,
                    fields: fields.iter().map(v9_field).collect(),
                });
            }
            PersistedIPFixTemplateDefinition::V9Options {
                scope_fields,
                option_fields,
            } => v9_options.push(NetflowV9OptionsTemplate {
                template_id: template.template_id,
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
                option_fields: option_fields.iter().map(v9_field).collect(),
            }),
        }
    }

    let mut flowsets = Vec::new();
    for (header_id, body) in [
        (!data.is_empty()).then(|| (2, IPFixFlowSetBody::Templates(data))),
        (!options.is_empty()).then(|| (3, IPFixFlowSetBody::OptionsTemplates(options))),
        (!v9_data.is_empty()).then(|| (0, IPFixFlowSetBody::V9Templates(v9_data))),
        (!v9_options.is_empty()).then(|| (1, IPFixFlowSetBody::V9OptionsTemplates(v9_options))),
    ]
    .into_iter()
    .flatten()
    {
        flowsets.push(NetflowIPFixFlowSet {
            header: NetflowIPFixFlowSetHeader {
                header_id,
                length: 0,
            },
            body,
        });
    }

    IPFix {
        header: NetflowIPFixHeader {
            version: 10,
            length: 0,
            export_time: 0,
            sequence_number: 0,
            observation_domain_id: key.observation_domain_id,
        },
        flowsets,
    }
    .to_be_bytes()
    .map(Some)
    .map_err(|err| format!("failed to serialize ipfix restore packet: {err}"))
}
