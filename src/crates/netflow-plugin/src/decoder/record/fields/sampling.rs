use super::*;

fn decode_v9_scope_sampler_id(
    scope_field: &netflow_parser::variable_versions::v9::ScopeDataField,
) -> Option<u64> {
    let raw = match scope_field {
        netflow_parser::variable_versions::v9::ScopeDataField::System(raw)
        | netflow_parser::variable_versions::v9::ScopeDataField::Interface(raw)
        | netflow_parser::variable_versions::v9::ScopeDataField::LineCard(raw)
        | netflow_parser::variable_versions::v9::ScopeDataField::NetFlowCache(raw)
        | netflow_parser::variable_versions::v9::ScopeDataField::Template(raw)
        | netflow_parser::variable_versions::v9::ScopeDataField::Unknown(_, raw) => raw.as_slice(),
    };
    decode_akvorado_unsigned(raw)
}

pub(crate) fn observe_v9_sampling_options(
    exporter_ip: IpAddr,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
    options_data: &V9OptionsData,
) -> bool {
    let mut changed = false;
    for record in &options_data.fields {
        let mut sampler_id = record
            .scope_fields
            .iter()
            .find_map(decode_v9_scope_sampler_id)
            .unwrap_or(0);
        let mut rate: Option<u64> = None;

        for (field, value) in &record.options_fields {
            let value_str = field_value_to_string(value);
            match field {
                V9Field::FlowSamplerId => {
                    if let Ok(parsed) = value_str.parse::<u64>() {
                        sampler_id = parsed;
                    }
                }
                V9Field::SamplingInterval | V9Field::FlowSamplerRandomInterval => {
                    rate = value_str.parse::<u64>().ok();
                }
                _ => {}
            }
        }

        if let Some(rate) = rate.filter(|rate| *rate > 0) {
            sampling.set(
                exporter_ip,
                version,
                observation_domain_id,
                sampler_id,
                rate,
            );
            changed |= namespace.set_sampling_rate(version, sampler_id, rate);
        }
    }
    changed
}

pub(crate) fn observe_ipfix_sampling_options(
    exporter_ip: IpAddr,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
    options_data: &IPFixOptionsData,
) -> bool {
    let mut changed = false;
    for record in &options_data.fields {
        let mut sampler_id = 0_u64;
        let mut rate: Option<u64> = None;
        let mut packet_interval: Option<u64> = None;
        let mut packet_space: Option<u64> = None;

        for (field, value) in record {
            let value_str = field_value_to_string(value);
            match field {
                IPFixField::IANA(IANAIPFixField::SamplerId)
                | IPFixField::IANA(IANAIPFixField::SelectorId) => {
                    if let Ok(parsed) = value_str.parse::<u64>() {
                        sampler_id = parsed;
                    }
                }
                IPFixField::IANA(IANAIPFixField::SamplingInterval)
                | IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => {
                    rate = value_str.parse::<u64>().ok();
                }
                IPFixField::IANA(IANAIPFixField::SamplingPacketInterval) => {
                    packet_interval = value_str.parse::<u64>().ok();
                }
                IPFixField::IANA(IANAIPFixField::SamplingPacketSpace) => {
                    packet_space = value_str.parse::<u64>().ok();
                }
                _ => {}
            }
        }

        if let (Some(interval), Some(space)) = (packet_interval, packet_space)
            && interval > 0
        {
            rate = Some((interval.saturating_add(space)) / interval);
        }

        if let Some(rate) = rate.filter(|rate| *rate > 0) {
            sampling.set(
                exporter_ip,
                version,
                observation_domain_id,
                sampler_id,
                rate,
            );
            changed |= namespace.set_sampling_rate(version, sampler_id, rate);
        }
    }
    changed
}

#[cfg(test)]
mod tests {
    use super::*;
    use netflow_parser::DataNumber;
    use netflow_parser::variable_versions::v9::{OptionsDataFields, ScopeDataField};

    #[test]
    fn v9_sampling_options_use_scope_sampler_id_when_present() {
        let mut sampling = SamplingState::default();
        let mut namespace = DecoderStateNamespace::default();
        let options = V9OptionsData {
            fields: vec![OptionsDataFields {
                scope_fields: vec![ScopeDataField::System(vec![0, 7])],
                options_fields: vec![(
                    V9Field::SamplingInterval,
                    FieldValue::DataNumber(DataNumber::U16(4000)),
                )],
            }],
        };

        observe_v9_sampling_options(
            IpAddr::V4(Ipv4Addr::new(192, 0, 2, 1)),
            9,
            42,
            &mut sampling,
            &mut namespace,
            &options,
        );

        assert_eq!(
            sampling.get(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 1)), 9, 42, 7),
            Some(4000)
        );
        assert_eq!(
            namespace
                .sampling_rates
                .get(&(9, 7))
                .map(|row| row.sampling_rate),
            Some(4000)
        );
    }
}
