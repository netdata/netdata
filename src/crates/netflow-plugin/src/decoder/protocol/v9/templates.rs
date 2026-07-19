use super::super::*;

pub(crate) fn observe_v9_decoder_state_from_packet(
    exporter_ip: IpAddr,
    packet: &V9,
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> DecoderStateObservation {
    let observation_domain_id = packet.header.source_id;
    let mut template_state_changed = false;
    let mut sampling_state_changed = false;

    for flowset in &packet.flowsets {
        match &flowset.body {
            V9FlowSetBody::Template(templates) => {
                for template in &templates.templates {
                    let fields = template
                        .fields
                        .iter()
                        .map(|field| PersistedV9TemplateField {
                            field_type: field.field_type_number,
                            field_length: field.field_length,
                        })
                        .collect();
                    template_state_changed |=
                        namespace.set_v9_template(template.template_id, fields);
                }
            }
            V9FlowSetBody::OptionsTemplate(templates) => {
                for template in &templates.templates {
                    let scope_fields = template
                        .scope_fields
                        .iter()
                        .map(|field| PersistedV9TemplateField {
                            field_type: field.field_type_number,
                            field_length: field.field_length,
                        })
                        .collect();
                    let option_fields = template
                        .option_fields
                        .iter()
                        .map(|field| PersistedV9TemplateField {
                            field_type: field.field_type_number,
                            field_length: field.field_length,
                        })
                        .collect();
                    template_state_changed |= namespace.set_v9_options_template(
                        template.template_id,
                        scope_fields,
                        option_fields,
                    );
                }
            }
            V9FlowSetBody::OptionsData(options_data) => {
                sampling_state_changed |= observe_v9_sampling_options(
                    exporter_ip,
                    9,
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
