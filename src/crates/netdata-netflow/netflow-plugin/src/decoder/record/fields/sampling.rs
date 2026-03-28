use super::*;

pub(crate) fn observe_v9_sampling_options(
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    options_data: V9OptionsData,
) {
    for record in options_data.fields {
        let mut sampler_id = 0_u64;
        let mut rate: Option<u64> = None;

        for fields in record.options_fields {
            for (field, value) in fields {
                let value_str = field_value_to_string(&value);
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
        }

        if let Some(rate) = rate.filter(|rate| *rate > 0) {
            sampling.set(
                exporter_ip,
                version,
                observation_domain_id,
                sampler_id,
                rate,
            );
        }
    }
}

pub(crate) fn observe_ipfix_sampling_options(
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    options_data: IPFixOptionsData,
) {
    for record in options_data.fields {
        let mut sampler_id = 0_u64;
        let mut rate: Option<u64> = None;
        let mut packet_interval: Option<u64> = None;
        let mut packet_space: Option<u64> = None;

        for (field, value) in record {
            let value_str = field_value_to_string(&value);
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
        }
    }
}
