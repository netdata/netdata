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
    if let IPFixField::Enterprise {
        enterprise_number: JUNIPER_PEN,
        field_number: JUNIPER_COMMON_PROPERTIES_ID,
    } = field
    {
        if let FieldValue::Vec(raw_value) = value
            && raw_value.len() == 2
            && ((raw_value[0] & 0xfc) >> 2) == 0x02
            && let Some(value) = decode_akvorado_unsigned(raw_value)
        {
            let status = if value & 0x03ff == 0 { "64" } else { "128" };
            set_record_field(rec, "FORWARDING_STATUS", status);
        }
        return;
    }

    if let IPFixField::IANA(IANAIPFixField::DataLinkFrameSection) = field {
        state.decap_required = true;
        if let FieldValue::Vec(raw_value) = value
            && let Some(l3_len) =
                parse_datalink_frame_section_record(raw_value, rec, decapsulation_mode)
        {
            state.ordinary_counters.observe_sampled_frame(l3_len);
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
    if matches!(
        field,
        IPFixField::IANA(IANAIPFixField::InitiatorOctets)
            | IPFixField::IANA(IANAIPFixField::InitiatorPackets)
            | IPFixField::IANA(IANAIPFixField::ResponderOctets)
            | IPFixField::IANA(IANAIPFixField::ResponderPackets)
    ) {
        state.session_counters_present = true;
    }

    if let IPFixField::IANA(IANAIPFixField::ResponderOctets) = field {
        state.reverse_present = true;
        state.session_reverse_bytes = Some(value_str);
        return;
    }
    if let IPFixField::IANA(IANAIPFixField::ResponderPackets) = field {
        state.reverse_present = true;
        state.session_reverse_packets = Some(value_str);
        return;
    }

    if matches!(
        field,
        IPFixField::IANA(IANAIPFixField::InitiatorOctets)
            | IPFixField::IANA(IANAIPFixField::InitiatorPackets)
    ) {
        state.ordinary_counters.observe_other_whole_flow_field();
    }

    if state.ordinary_counters.observe_ipfix(field, value) {
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

    fn apply_test_field(
        rec: &mut FlowRecord,
        state: &mut IPFixRecordBuildState,
        field: IPFixField,
        value: FieldValue,
    ) {
        apply_ipfix_record_field(rec, state, &field, &value, DecapsulationMode::None, 0);
    }

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

    #[test]
    fn reverse_information_elements_do_not_select_the_session_model() {
        let mut rec = FlowRecord::default();
        let mut state = IPFixRecordBuildState::default();

        apply_test_field(
            &mut rec,
            &mut state,
            IPFixField::ReverseInformationElement(
                ReverseInformationElement::ReverseInitiatorOctets,
            ),
            FieldValue::DataNumber(DataNumber::U64(42)),
        );
        state.apply_session_reverse_counters();

        assert!(!state.session_counters_present);
        assert_eq!(
            state.reverse_overrides.get("BYTES").map(String::as_str),
            Some("42")
        );
    }

    #[test]
    fn session_reverse_counters_beat_rfc_reverse_counters_in_both_orders() {
        for session_first in [false, true] {
            let mut rec = FlowRecord::default();
            let mut state = IPFixRecordBuildState::default();
            let rfc_fields = [
                (
                    IPFixField::ReverseInformationElement(
                        ReverseInformationElement::ReverseOctetDeltaCount,
                    ),
                    FieldValue::DataNumber(DataNumber::U64(900)),
                ),
                (
                    IPFixField::ReverseInformationElement(
                        ReverseInformationElement::ReversePacketDeltaCount,
                    ),
                    FieldValue::DataNumber(DataNumber::U64(90)),
                ),
                (
                    IPFixField::ReverseInformationElement(
                        ReverseInformationElement::ReverseSourceTransportPort,
                    ),
                    FieldValue::DataNumber(DataNumber::U16(5555)),
                ),
            ];
            let session_fields = [
                (
                    IPFixField::IANA(IANAIPFixField::ResponderOctets),
                    FieldValue::DataNumber(DataNumber::U64(42)),
                ),
                (
                    IPFixField::IANA(IANAIPFixField::ResponderPackets),
                    FieldValue::DataNumber(DataNumber::U64(3)),
                ),
            ];

            let groups = if session_first {
                [&session_fields[..], &rfc_fields[..]]
            } else {
                [&rfc_fields[..], &session_fields[..]]
            };
            for group in groups {
                for (field, value) in group {
                    apply_test_field(&mut rec, &mut state, field.clone(), value.clone());
                }
            }
            state.apply_session_reverse_counters();

            assert_eq!(
                state.reverse_overrides.get("BYTES").map(String::as_str),
                Some("42"),
                "session_first={session_first}"
            );
            assert_eq!(
                state.reverse_overrides.get("PACKETS").map(String::as_str),
                Some("3"),
                "session_first={session_first}"
            );
            assert_eq!(
                state.reverse_overrides.get("SRC_PORT").map(String::as_str),
                Some("5555"),
                "session_first={session_first}"
            );
        }
    }

    #[test]
    fn any_session_counter_suppresses_rfc_reverse_counter_metrics() {
        for (session_field, session_value, expected_bytes, expected_packets) in [
            (IANAIPFixField::InitiatorOctets, 1, None, None),
            (IANAIPFixField::ResponderOctets, 0, Some("0"), None),
            (IANAIPFixField::ResponderPackets, 0, None, Some("0")),
        ] {
            let mut rec = FlowRecord::default();
            let mut state = IPFixRecordBuildState::default();

            apply_test_field(
                &mut rec,
                &mut state,
                IPFixField::ReverseInformationElement(
                    ReverseInformationElement::ReverseOctetDeltaCount,
                ),
                FieldValue::DataNumber(DataNumber::U64(900)),
            );
            apply_test_field(
                &mut rec,
                &mut state,
                IPFixField::ReverseInformationElement(
                    ReverseInformationElement::ReversePacketDeltaCount,
                ),
                FieldValue::DataNumber(DataNumber::U64(90)),
            );
            apply_test_field(
                &mut rec,
                &mut state,
                IPFixField::IANA(session_field),
                FieldValue::DataNumber(DataNumber::U64(session_value)),
            );
            state.apply_session_reverse_counters();

            assert_eq!(
                state.reverse_overrides.get("BYTES").map(String::as_str),
                expected_bytes
            );
            assert_eq!(
                state.reverse_overrides.get("PACKETS").map(String::as_str),
                expected_packets
            );
        }
    }
}
