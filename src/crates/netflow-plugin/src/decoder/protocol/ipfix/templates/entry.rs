use super::*;

fn persist_ipfix_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowIPFixTemplate,
) -> bool {
    namespace.set_ipfix_template(
        template.template_id,
        template
            .fields
            .iter()
            .map(|field| PersistedIPFixTemplateField {
                field_type: field.field_type_number,
                field_length: field.field_length,
                enterprise_number: field.enterprise_number,
            })
            .collect(),
    )
}

fn persist_ipfix_options_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowIPFixOptionsTemplate,
) -> bool {
    namespace.set_ipfix_options_template(
        template.template_id,
        template.scope_field_count,
        template
            .fields
            .iter()
            .map(|field| PersistedIPFixTemplateField {
                field_type: field.field_type_number,
                field_length: field.field_length,
                enterprise_number: field.enterprise_number,
            })
            .collect(),
    )
}

fn persist_ipfix_v9_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowV9Template,
) -> bool {
    namespace.set_ipfix_v9_template(
        template.template_id,
        template
            .fields
            .iter()
            .map(|field| PersistedV9TemplateField {
                field_type: field.field_type_number,
                field_length: field.field_length,
            })
            .collect(),
    )
}

fn persist_ipfix_v9_options_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowV9OptionsTemplate,
) -> bool {
    namespace.set_ipfix_v9_options_template(
        template.template_id,
        template
            .scope_fields
            .iter()
            .map(|field| PersistedV9TemplateField {
                field_type: field.field_type_number,
                field_length: field.field_length,
            })
            .collect(),
        template
            .option_fields
            .iter()
            .map(|field| PersistedV9TemplateField {
                field_type: field.field_type_number,
                field_length: field.field_length,
            })
            .collect(),
    )
}

pub(crate) fn observe_ipfix_decoder_state_from_packet(
    exporter_ip: IpAddr,
    packet: &IPFix,
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> DecoderStateObservation {
    let observation_domain_id = packet.header.observation_domain_id;
    let mut template_state_changed = false;
    let mut sampling_state_changed = false;

    for flowset in &packet.flowsets {
        match &flowset.body {
            IPFixFlowSetBody::Template(template) => {
                template_state_changed |= persist_ipfix_template(namespace, template);
            }
            IPFixFlowSetBody::Templates(templates) => {
                for template in templates {
                    template_state_changed |= persist_ipfix_template(namespace, template);
                }
            }
            IPFixFlowSetBody::OptionsTemplate(template) => {
                template_state_changed |= persist_ipfix_options_template(namespace, template);
            }
            IPFixFlowSetBody::OptionsTemplates(templates) => {
                for template in templates {
                    template_state_changed |= persist_ipfix_options_template(namespace, template);
                }
            }
            IPFixFlowSetBody::V9Template(template) => {
                template_state_changed |= persist_ipfix_v9_template(namespace, template);
            }
            IPFixFlowSetBody::V9Templates(templates) => {
                for template in templates {
                    template_state_changed |= persist_ipfix_v9_template(namespace, template);
                }
            }
            IPFixFlowSetBody::V9OptionsTemplate(template) => {
                template_state_changed |= persist_ipfix_v9_options_template(namespace, template);
            }
            IPFixFlowSetBody::V9OptionsTemplates(templates) => {
                for template in templates {
                    template_state_changed |=
                        persist_ipfix_v9_options_template(namespace, template);
                }
            }
            IPFixFlowSetBody::OptionsData(options_data) => {
                sampling_state_changed |= observe_ipfix_sampling_options(
                    exporter_ip,
                    10,
                    observation_domain_id,
                    sampling,
                    namespace,
                    options_data,
                );
            }
            IPFixFlowSetBody::V9OptionsData(options_data) => {
                sampling_state_changed |= observe_v9_sampling_options(
                    exporter_ip,
                    10,
                    observation_domain_id,
                    sampling,
                    namespace,
                    options_data,
                );
            }
            _ => {}
        }
    }

    DecoderStateObservation {
        namespace_state_changed: template_state_changed || sampling_state_changed,
        template_state_changed,
    }
}
