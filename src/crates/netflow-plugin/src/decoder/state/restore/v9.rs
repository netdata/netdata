use super::*;

pub(super) fn build_v9_restore_packet(
    key: &DecoderStateNamespaceKey,
    namespace: &DecoderStateNamespace,
) -> Result<Option<Vec<u8>>, String> {
    if namespace.v9_templates.is_empty() && namespace.v9_options_templates.is_empty() {
        return Ok(None);
    }

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
                        // RFC 3954 section 6.1 defines these as byte lengths
                        // of the field-definition descriptors in the Options
                        // Template Record, not the later data-record values.
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
            // `netflow_parser::V9::to_be_bytes()` patches the emitted count.
            count: 0,
            sys_up_time: 0,
            unix_secs: 0,
            sequence_number: 0,
            source_id: key.observation_domain_id,
        },
        flowsets,
    };

    packet
        .to_be_bytes()
        .map(Some)
        .map_err(|err| format!("failed to serialize v9 restore packet: {err}"))
}
