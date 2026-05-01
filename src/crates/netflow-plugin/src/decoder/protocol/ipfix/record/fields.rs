use super::state::{IPFixRecordBuildState, observe_ipfix_record_value, track_reverse_ipfix_time};
use super::*;

pub(crate) fn apply_ipfix_record_field(
    rec: &mut FlowRecord,
    state: &mut IPFixRecordBuildState,
    field: &IPFixField,
    value: &FieldValue,
    decapsulation_mode: DecapsulationMode,
    export_usec: u64,
) {
    if let IPFixField::IANA(IANAIPFixField::DataLinkFrameSection) = field {
        state.decap_required = true;
        if let FieldValue::Vec(raw_value) | FieldValue::Unknown(raw_value) = value
            && let Some(l3_len) =
                parse_datalink_frame_section_record(raw_value, rec, decapsulation_mode)
        {
            rec.bytes = l3_len;
            rec.packets = 1;
            state.decap_ok = true;
        }
        return;
    }

    if let IPFixField::ReverseInformationElement(reverse_field) = field {
        state.reverse_present = true;
        track_reverse_ipfix_time(state, reverse_field, value, export_usec);

        let value_str = field_value_to_string(value);
        apply_reverse_ipfix_special_mappings(
            &mut state.reverse_overrides,
            reverse_field,
            &value_str,
        );
        if let Some(canonical) = reverse_ipfix_canonical_key(reverse_field) {
            if should_skip_zero_ip(canonical, &value_str) {
                return;
            }
            if matches!(reverse_field, ReverseInformationElement::ReverseIpVersion) {
                return;
            }
            state.reverse_overrides.insert(
                canonical,
                canonical_value(canonical, &value_str).to_string(),
            );
        }
        return;
    }

    if let IPFixField::IANA(IANAIPFixField::SystemInitTimeMilliseconds) = field {
        state.system_init_millis = field_value_unsigned(value);
    }

    let value_str = field_value_to_string(value);
    if let IPFixField::IANA(IANAIPFixField::ResponderOctets) = field {
        state.reverse_present = true;
        state.reverse_overrides.insert("BYTES", value_str);
        return;
    }
    if let IPFixField::IANA(IANAIPFixField::ResponderPackets) = field {
        state.reverse_present = true;
        state.reverse_overrides.insert("PACKETS", value_str);
        return;
    }

    apply_ipfix_special_mappings_record(rec, field, &value_str);
    observe_ipfix_record_value(state, field, value, &value_str);

    if let Some(canonical) = ipfix_canonical_key(field) {
        if should_skip_zero_ip(canonical, &value_str) {
            return;
        }
        if matches!(field, IPFixField::IANA(IANAIPFixField::IpVersion)) {
            return;
        }
        set_record_field(rec, canonical, &value_str);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reverse_ip_version_keeps_mapped_ethertype() {
        let mut rec = FlowRecord::default();
        let mut state = IPFixRecordBuildState::default();

        apply_ipfix_record_field(
            &mut rec,
            &mut state,
            &IPFixField::ReverseInformationElement(ReverseInformationElement::ReverseIpVersion),
            &FieldValue::DataNumber(DataNumber::U8(6)),
            DecapsulationMode::None,
            0,
        );

        assert_eq!(
            state.reverse_overrides.get("ETYPE").map(String::as_str),
            Some("34525")
        );
    }
}
