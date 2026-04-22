use super::*;

#[derive(Default)]
pub(crate) struct IPFixRecordBuildState {
    pub(crate) reverse_overrides: FlowFields,
    pub(crate) reverse_present: bool,
    pub(crate) decap_required: bool,
    pub(crate) decap_ok: bool,
    pub(crate) sampler_id: Option<u64>,
    pub(crate) observed_sampling_rate: Option<u64>,
    pub(crate) sampling_packet_interval: Option<u64>,
    pub(crate) sampling_packet_space: Option<u64>,
    pub(crate) system_init_millis: Option<u64>,
    pub(crate) flow_start_seconds: Option<u64>,
    pub(crate) flow_end_seconds: Option<u64>,
    pub(crate) flow_start_millis: Option<u64>,
    pub(crate) flow_end_millis: Option<u64>,
    pub(crate) flow_start_micros: Option<u64>,
    pub(crate) flow_end_micros: Option<u64>,
    pub(crate) flow_start_nanos: Option<u64>,
    pub(crate) flow_end_nanos: Option<u64>,
    pub(crate) flow_start_delta_micros: Option<u64>,
    pub(crate) flow_end_delta_micros: Option<u64>,
    pub(crate) flow_start_sysuptime_millis: Option<u64>,
    pub(crate) flow_end_sysuptime_millis: Option<u64>,
    pub(crate) reverse_flow_start_usec: Option<u64>,
    pub(crate) reverse_flow_end_usec: Option<u64>,
    pub(crate) reverse_flow_start_sysuptime_millis: Option<u64>,
    pub(crate) reverse_flow_end_sysuptime_millis: Option<u64>,
}

impl IPFixRecordBuildState {
    pub(crate) fn apply_sampling_packet_ratio(&mut self) {
        if let (Some(interval), Some(space)) =
            (self.sampling_packet_interval, self.sampling_packet_space)
            && interval > 0
        {
            self.observed_sampling_rate = Some((interval.saturating_add(space)) / interval);
        }
    }

    pub(crate) fn resolve_flow_times(&self, rec: &mut FlowRecord, export_usec: u64) {
        rec.flow_start_usec = resolve_ipfix_time_usec(
            self.flow_start_seconds,
            self.flow_start_millis,
            self.flow_start_micros,
            self.flow_start_nanos,
            self.flow_start_delta_micros,
            self.flow_start_sysuptime_millis,
            self.system_init_millis,
            export_usec,
        )
        .unwrap_or(0);
        rec.flow_end_usec = resolve_ipfix_time_usec(
            self.flow_end_seconds,
            self.flow_end_millis,
            self.flow_end_micros,
            self.flow_end_nanos,
            self.flow_end_delta_micros,
            self.flow_end_sysuptime_millis,
            self.system_init_millis,
            export_usec,
        )
        .unwrap_or(0);
    }

    pub(crate) fn apply_reverse_time_overrides(&mut self) {
        if self.reverse_flow_start_usec.is_none()
            && let (Some(system_init_millis), Some(uptime_millis)) = (
                self.system_init_millis,
                self.reverse_flow_start_sysuptime_millis,
            )
        {
            self.reverse_flow_start_usec = Some(
                system_init_millis
                    .saturating_mul(USEC_PER_MILLISECOND)
                    .saturating_add(uptime_millis.saturating_mul(USEC_PER_MILLISECOND)),
            );
        }
        if self.reverse_flow_end_usec.is_none()
            && let (Some(system_init_millis), Some(uptime_millis)) = (
                self.system_init_millis,
                self.reverse_flow_end_sysuptime_millis,
            )
        {
            self.reverse_flow_end_usec = Some(
                system_init_millis
                    .saturating_mul(USEC_PER_MILLISECOND)
                    .saturating_add(uptime_millis.saturating_mul(USEC_PER_MILLISECOND)),
            );
        }
        if let Some(start_usec) = self.reverse_flow_start_usec {
            self.reverse_overrides
                .insert("FLOW_START_USEC", start_usec.to_string());
        }
        if let Some(end_usec) = self.reverse_flow_end_usec {
            self.reverse_overrides
                .insert("FLOW_END_USEC", end_usec.to_string());
        }
    }
}

pub(super) fn track_reverse_ipfix_time(
    state: &mut IPFixRecordBuildState,
    reverse_field: &ReverseInformationElement,
    value: &FieldValue,
    export_usec: u64,
) {
    match reverse_field {
        ReverseInformationElement::ReverseFlowStartSysUpTime => {
            state.reverse_flow_start_sysuptime_millis = field_value_unsigned(value);
            return;
        }
        ReverseInformationElement::ReverseFlowEndSysUpTime => {
            state.reverse_flow_end_sysuptime_millis = field_value_unsigned(value);
            return;
        }
        _ => {}
    }

    let Some(usec) = reverse_ipfix_timestamp_to_usec(
        reverse_field,
        value,
        export_usec,
        state.system_init_millis,
    ) else {
        return;
    };

    match reverse_field {
        ReverseInformationElement::ReverseFlowStartSeconds
        | ReverseInformationElement::ReverseFlowStartMilliseconds
        | ReverseInformationElement::ReverseFlowStartMicroseconds
        | ReverseInformationElement::ReverseFlowStartNanoseconds
        | ReverseInformationElement::ReverseFlowStartDeltaMicroseconds
        | ReverseInformationElement::ReverseFlowStartSysUpTime
        | ReverseInformationElement::ReverseMinFlowStartMicroseconds
        | ReverseInformationElement::ReverseMinFlowStartNanoseconds => {
            state.reverse_flow_start_usec = Some(usec);
        }
        ReverseInformationElement::ReverseFlowEndSeconds
        | ReverseInformationElement::ReverseFlowEndMilliseconds
        | ReverseInformationElement::ReverseFlowEndMicroseconds
        | ReverseInformationElement::ReverseFlowEndNanoseconds
        | ReverseInformationElement::ReverseFlowEndDeltaMicroseconds
        | ReverseInformationElement::ReverseFlowEndSysUpTime
        | ReverseInformationElement::ReverseMaxFlowEndMicroseconds
        | ReverseInformationElement::ReverseMaxFlowEndNanoseconds => {
            state.reverse_flow_end_usec = Some(usec);
        }
        _ => {}
    }
}

pub(super) fn observe_ipfix_record_value(
    state: &mut IPFixRecordBuildState,
    field: &IPFixField,
    value: &FieldValue,
    value_str: &str,
) {
    match field {
        IPFixField::IANA(IANAIPFixField::SamplerId)
        | IPFixField::IANA(IANAIPFixField::SelectorId) => {
            state.sampler_id = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::SamplingInterval)
        | IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => {
            state.observed_sampling_rate = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::SamplingPacketInterval) => {
            state.sampling_packet_interval = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::SamplingPacketSpace) => {
            state.sampling_packet_space = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowStartSeconds) => {
            state.flow_start_seconds = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowEndSeconds) => {
            state.flow_end_seconds = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowStartMilliseconds)
        | IPFixField::IANA(IANAIPFixField::MinFlowStartMilliseconds) => {
            state.flow_start_millis = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowEndMilliseconds)
        | IPFixField::IANA(IANAIPFixField::MaxFlowEndMilliseconds) => {
            state.flow_end_millis = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowStartMicroseconds)
        | IPFixField::IANA(IANAIPFixField::MinFlowStartMicroseconds) => {
            state.flow_start_micros = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndMicroseconds)
        | IPFixField::IANA(IANAIPFixField::MaxFlowEndMicroseconds) => {
            state.flow_end_micros = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowStartNanoseconds)
        | IPFixField::IANA(IANAIPFixField::MinFlowStartNanoseconds) => {
            state.flow_start_nanos = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndNanoseconds)
        | IPFixField::IANA(IANAIPFixField::MaxFlowEndNanoseconds) => {
            state.flow_end_nanos = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowStartDeltaMicroseconds) => {
            state.flow_start_delta_micros = field_value_unsigned(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndDeltaMicroseconds) => {
            state.flow_end_delta_micros = field_value_unsigned(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowStartSysUpTime) => {
            state.flow_start_sysuptime_millis = field_value_unsigned(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndSysUpTime) => {
            state.flow_end_sysuptime_millis = field_value_unsigned(value);
        }
        _ => {}
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reverse_sysuptime_waits_for_system_init_time() {
        let mut state = IPFixRecordBuildState::default();

        track_reverse_ipfix_time(
            &mut state,
            &ReverseInformationElement::ReverseFlowStartSysUpTime,
            &FieldValue::DataNumber(DataNumber::U32(42)),
            0,
        );
        state.system_init_millis = Some(1_000);
        state.apply_reverse_time_overrides();

        assert_eq!(
            state.reverse_overrides.get("FLOW_START_USEC").map(String::as_str),
            Some("1042000")
        );
    }
}
