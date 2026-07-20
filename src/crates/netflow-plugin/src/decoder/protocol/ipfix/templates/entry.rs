use super::*;

fn persist_ipfix_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowIPFixTemplate,
    received_at_usec: u64,
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
        received_at_usec,
    )
}

fn persist_ipfix_options_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowIPFixOptionsTemplate,
    received_at_usec: u64,
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
        received_at_usec,
    )
}

fn persist_ipfix_v9_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowV9Template,
    received_at_usec: u64,
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
        received_at_usec,
    )
}

fn persist_ipfix_v9_options_template(
    namespace: &mut DecoderStateNamespace,
    template: &NetflowV9OptionsTemplate,
    received_at_usec: u64,
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
        received_at_usec,
    )
}

pub(crate) fn observe_ipfix_decoder_state_from_packet(
    exporter_source: SocketAddr,
    key: &DecoderStateNamespaceKey,
    packet: &IPFix,
    sampling: &mut SamplingState,
    template_state: &mut TemplateState,
    namespace: &mut DecoderStateNamespace,
    received_at_usec: u64,
) -> DecoderStateObservation {
    let observation_domain_id = packet.header.observation_domain_id;
    let mut template_state_changed = false;
    let mut dirty_sampling_namespaces = Vec::new();

    for flowset in &packet.flowsets {
        match &flowset.body {
            IPFixFlowSetBody::Template(template) => {
                template_state_changed |=
                    persist_ipfix_template(namespace, template, received_at_usec);
                template_state.install(
                    key,
                    PersistedTemplateKind::IpfixData,
                    template.template_id,
                    namespace,
                );
            }
            IPFixFlowSetBody::Templates(templates) => {
                for template in templates {
                    template_state_changed |=
                        persist_ipfix_template(namespace, template, received_at_usec);
                    template_state.install(
                        key,
                        PersistedTemplateKind::IpfixData,
                        template.template_id,
                        namespace,
                    );
                }
            }
            IPFixFlowSetBody::OptionsTemplate(template) => {
                template_state_changed |=
                    persist_ipfix_options_template(namespace, template, received_at_usec);
                template_state.install(
                    key,
                    PersistedTemplateKind::IpfixOptions,
                    template.template_id,
                    namespace,
                );
            }
            IPFixFlowSetBody::OptionsTemplates(templates) => {
                for template in templates {
                    template_state_changed |=
                        persist_ipfix_options_template(namespace, template, received_at_usec);
                    template_state.install(
                        key,
                        PersistedTemplateKind::IpfixOptions,
                        template.template_id,
                        namespace,
                    );
                }
            }
            IPFixFlowSetBody::V9Template(template) => {
                template_state_changed |=
                    persist_ipfix_v9_template(namespace, template, received_at_usec);
                template_state.install(
                    key,
                    PersistedTemplateKind::IpfixV9Data,
                    template.template_id,
                    namespace,
                );
            }
            IPFixFlowSetBody::V9Templates(templates) => {
                for template in templates {
                    template_state_changed |=
                        persist_ipfix_v9_template(namespace, template, received_at_usec);
                    template_state.install(
                        key,
                        PersistedTemplateKind::IpfixV9Data,
                        template.template_id,
                        namespace,
                    );
                }
            }
            IPFixFlowSetBody::V9OptionsTemplate(template) => {
                template_state_changed |=
                    persist_ipfix_v9_options_template(namespace, template, received_at_usec);
                template_state.install(
                    key,
                    PersistedTemplateKind::IpfixV9Options,
                    template.template_id,
                    namespace,
                );
            }
            IPFixFlowSetBody::V9OptionsTemplates(templates) => {
                for template in templates {
                    template_state_changed |=
                        persist_ipfix_v9_options_template(namespace, template, received_at_usec);
                    template_state.install(
                        key,
                        PersistedTemplateKind::IpfixV9Options,
                        template.template_id,
                        namespace,
                    );
                }
            }
            IPFixFlowSetBody::OptionsData(options_data) => {
                template_state.touch(
                    key,
                    PersistedTemplateKind::IpfixOptions,
                    flowset.header.header_id,
                );
                for key in observe_ipfix_sampling_options(
                    exporter_source,
                    10,
                    observation_domain_id,
                    sampling,
                    options_data,
                ) {
                    if !dirty_sampling_namespaces.contains(&key) {
                        dirty_sampling_namespaces.push(key);
                    }
                }
            }
            IPFixFlowSetBody::V9OptionsData(options_data) => {
                template_state.touch(
                    key,
                    PersistedTemplateKind::IpfixV9Options,
                    flowset.header.header_id,
                );
                for key in observe_v9_sampling_options(
                    exporter_source,
                    10,
                    observation_domain_id,
                    sampling,
                    options_data,
                ) {
                    if !dirty_sampling_namespaces.contains(&key) {
                        dirty_sampling_namespaces.push(key);
                    }
                }
            }
            IPFixFlowSetBody::Data(_) => template_state.touch(
                key,
                PersistedTemplateKind::IpfixData,
                flowset.header.header_id,
            ),
            IPFixFlowSetBody::V9Data(_) => template_state.touch(
                key,
                PersistedTemplateKind::IpfixV9Data,
                flowset.header.header_id,
            ),
            _ => {}
        }
    }

    DecoderStateObservation {
        namespace_state_changed: template_state_changed,
        template_state_changed,
        dirty_sampling_namespaces,
        v9_nsel_flowsets: Vec::new(),
    }
}
