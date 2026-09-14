use super::super::*;

pub(crate) fn observe_v9_decoder_state_from_packet(
    exporter_source: SocketAddr,
    key: &DecoderStateNamespaceKey,
    packet: &V9,
    sampling: &mut SamplingState,
    template_state: &mut TemplateState,
    namespace: &mut DecoderStateNamespace,
    received_at_usec: u64,
) -> DecoderStateObservation {
    let observation_domain_id = packet.header.source_id;
    let mut template_state_changed = false;
    let mut dirty_sampling_namespaces = Vec::new();
    let mut nsel_flowsets = Vec::with_capacity(packet.flowsets.len());

    for flowset in &packet.flowsets {
        nsel_flowsets.push(
            matches!(flowset.body, V9FlowSetBody::Data(_))
                && namespace
                    .v9_templates
                    .get(&flowset.header.flowset_id)
                    .is_some_and(|template| template.nsel),
        );
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
                        .collect::<Vec<_>>();
                    let nsel = is_nsel_template(&fields);
                    template_state_changed |= namespace.set_v9_template(
                        template.template_id,
                        fields,
                        received_at_usec,
                        nsel,
                    );
                    template_state.install(
                        key,
                        PersistedTemplateKind::V9Data,
                        template.template_id,
                        namespace,
                    );
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
                        received_at_usec,
                    );
                    template_state.install(
                        key,
                        PersistedTemplateKind::V9Options,
                        template.template_id,
                        namespace,
                    );
                }
            }
            V9FlowSetBody::OptionsData(options_data) => {
                template_state.touch(
                    key,
                    PersistedTemplateKind::V9Options,
                    flowset.header.flowset_id,
                );
                for key in observe_v9_sampling_options(
                    exporter_source,
                    9,
                    observation_domain_id,
                    sampling,
                    options_data,
                ) {
                    if !dirty_sampling_namespaces.contains(&key) {
                        dirty_sampling_namespaces.push(key);
                    }
                }
            }
            V9FlowSetBody::Data(_) => template_state.touch(
                key,
                PersistedTemplateKind::V9Data,
                flowset.header.flowset_id,
            ),
            _ => {}
        }
    }

    DecoderStateObservation {
        namespace_state_changed: template_state_changed,
        template_state_changed,
        dirty_sampling_namespaces,
        v9_nsel_flowsets: nsel_flowsets,
    }
}
