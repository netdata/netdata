use super::*;

pub(crate) fn observe_ipfix_decoder_state_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> DecoderStateObservation {
    let template_state_changed =
        observe_ipfix_templates_from_raw_payload(source, payload, sampling, namespace);
    DecoderStateObservation {
        namespace_state_changed: template_state_changed,
        template_state_changed,
    }
}

pub(crate) fn observe_ipfix_templates_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    if payload.len() < 16 {
        return false;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 10 {
        return false;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
    let packet_length = u16::from_be_bytes([payload[2], payload[3]]) as usize;
    let end_limit = payload.len().min(packet_length);
    let mut offset = 16_usize;
    let mut changed = false;

    while offset.saturating_add(4) <= end_limit {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return changed;
        }
        let end = offset.saturating_add(flowset_len);
        if end > end_limit {
            return changed;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == IPFIX_SET_ID_TEMPLATE {
            changed |= observe_ipfix_data_templates(
                &exporter_ip,
                observation_domain_id,
                body,
                sampling,
                namespace,
            );
        } else if flowset_id == 3 {
            changed |= observe_ipfix_options_templates(body, namespace);
        } else if flowset_id == 0 {
            changed |= observe_ipfix_v9_templates(body, namespace);
        } else if flowset_id == 1 {
            changed |= observe_ipfix_v9_options_templates(body, namespace);
        }

        offset = end;
    }

    changed
}
