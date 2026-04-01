use super::*;

pub(super) fn build_ipfix_restore_packet(
    key: &DecoderStateNamespaceKey,
    namespace: &DecoderStateNamespace,
) -> Result<Option<Vec<u8>>, String> {
    if namespace.ipfix_templates.is_empty()
        && namespace.ipfix_options_templates.is_empty()
        && namespace.ipfix_v9_templates.is_empty()
        && namespace.ipfix_v9_options_templates.is_empty()
    {
        return Ok(None);
    }

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

    packet
        .to_be_bytes()
        .map(Some)
        .map_err(|err| format!("failed to serialize ipfix restore packet: {err}"))
}
