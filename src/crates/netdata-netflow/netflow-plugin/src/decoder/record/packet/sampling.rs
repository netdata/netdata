use super::super::*;

pub(crate) fn apply_sampling_state_record(
    rec: &mut FlowRecord,
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampler_id: Option<u64>,
    observed_sampling_rate: Option<u64>,
    sampling: &SamplingState,
) {
    if let Some(rate) = observed_sampling_rate.filter(|rate| *rate > 0) {
        rec.set_sampling_rate(rate);
        return;
    }

    if !rec.has_sampling_rate() {
        if let Some(id) = sampler_id
            && let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, id)
        {
            rec.set_sampling_rate(rate);
            return;
        }

        if let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, 0) {
            rec.set_sampling_rate(rate);
        }
    }
}

pub(crate) fn apply_sampling_state_fields(
    fields: &mut FlowFields,
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampler_id: Option<u64>,
    observed_sampling_rate: Option<u64>,
    sampling: &SamplingState,
) {
    if let Some(rate) = observed_sampling_rate.filter(|rate| *rate > 0) {
        fields.insert("SAMPLING_RATE", rate.to_string());
        return;
    }

    if fields.contains_key("SAMPLING_RATE") {
        return;
    }

    if let Some(id) = sampler_id
        && let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, id)
    {
        fields.insert("SAMPLING_RATE", rate.to_string());
        return;
    }

    if let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, 0) {
        fields.insert("SAMPLING_RATE", rate.to_string());
    }
}

pub(crate) fn looks_like_sampling_option_record_from_rec(
    rec: &FlowRecord,
    observed_sampling_rate: Option<u64>,
) -> bool {
    if observed_sampling_rate.unwrap_or(0) == 0 {
        return false;
    }
    // No endpoints = likely a sampling option record, not a data flow
    rec.src_addr.is_none() && rec.dst_addr.is_none()
}
