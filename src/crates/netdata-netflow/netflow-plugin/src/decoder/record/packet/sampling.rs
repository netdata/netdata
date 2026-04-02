use super::super::*;

fn has_positive_sampling_rate_field(fields: &FlowFields) -> bool {
    fields
        .get("SAMPLING_RATE")
        .and_then(|value| value.parse::<u64>().ok())
        .is_some_and(|rate| rate > 0)
}

pub(crate) fn apply_sampling_state_record(
    rec: &mut FlowRecord,
    exporter_ip: IpAddr,
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

    if !rec.has_sampling_rate() || rec.sampling_rate == 0 {
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
    exporter_ip: IpAddr,
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

    if has_positive_sampling_rate_field(fields) {
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::BTreeMap;

    #[test]
    fn apply_sampling_state_fields_replaces_zero_with_learned_rate() {
        let mut fields = BTreeMap::from([("SAMPLING_RATE", "0".to_string())]);
        let mut sampling = SamplingState::default();
        let exporter_ip = "192.0.2.10".parse().unwrap();
        sampling.set(exporter_ip, 9, 20, 7, 4000);

        apply_sampling_state_fields(&mut fields, exporter_ip, 9, 20, Some(7), None, &sampling);

        assert_eq!(
            fields.get("SAMPLING_RATE").map(String::as_str),
            Some("4000")
        );
    }

    #[test]
    fn apply_sampling_state_fields_replaces_invalid_with_learned_rate() {
        let mut fields = BTreeMap::from([("SAMPLING_RATE", "invalid".to_string())]);
        let mut sampling = SamplingState::default();
        let exporter_ip = "192.0.2.10".parse().unwrap();
        sampling.set(exporter_ip, 9, 20, 7, 4000);

        apply_sampling_state_fields(&mut fields, exporter_ip, 9, 20, Some(7), None, &sampling);

        assert_eq!(
            fields.get("SAMPLING_RATE").map(String::as_str),
            Some("4000")
        );
    }

    #[test]
    fn apply_sampling_state_record_replaces_zero_with_learned_rate() {
        let mut rec = FlowRecord::default();
        rec.set_sampling_rate(0);

        let mut sampling = SamplingState::default();
        let exporter_ip = "192.0.2.10".parse().unwrap();
        sampling.set(exporter_ip, 9, 20, 7, 4000);

        apply_sampling_state_record(&mut rec, exporter_ip, 9, 20, Some(7), None, &sampling);

        assert!(rec.has_sampling_rate());
        assert_eq!(rec.sampling_rate, 4000);
    }
}
