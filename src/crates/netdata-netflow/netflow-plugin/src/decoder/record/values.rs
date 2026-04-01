use super::*;

pub(crate) fn field_value_to_string(value: &FieldValue) -> String {
    match value {
        FieldValue::ApplicationId(app) => {
            format!(
                "{}:{}",
                app.classification_engine_id,
                data_number_to_string(&app.selector_id)
            )
        }
        FieldValue::String(v) => v.clone(),
        FieldValue::DataNumber(v) => data_number_to_string(v),
        FieldValue::Float64(v) => v.to_string(),
        FieldValue::Duration(v) => v.as_millis().to_string(),
        FieldValue::Ip4Addr(v) => v.to_string(),
        FieldValue::Ip6Addr(v) => v.to_string(),
        FieldValue::MacAddr(v) => v.to_string(),
        FieldValue::Vec(v) | FieldValue::Unknown(v) => bytes_to_hex(v),
        FieldValue::ProtocolType(v) => u8::from(*v).to_string(),
    }
}

pub(crate) fn field_value_unsigned(value: &FieldValue) -> Option<u64> {
    match value {
        FieldValue::DataNumber(number) => match number {
            DataNumber::U8(v) => Some(u64::from(*v)),
            DataNumber::U16(v) => Some(u64::from(*v)),
            DataNumber::U24(v) => Some(u64::from(*v)),
            DataNumber::U32(v) => Some(u64::from(*v)),
            DataNumber::U64(v) => Some(*v),
            DataNumber::I8(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I16(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I24(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I32(v) if *v >= 0 => Some(*v as u64),
            DataNumber::I64(v) if *v >= 0 => Some(*v as u64),
            DataNumber::U128(v) => u64::try_from(*v).ok(),
            DataNumber::I128(v) if *v >= 0 => u64::try_from(*v).ok(),
            _ => None,
        },
        _ => None,
    }
}

pub(crate) fn field_value_duration_usec(value: &FieldValue) -> Option<u64> {
    match value {
        FieldValue::Duration(duration) => u64::try_from(duration.as_micros()).ok(),
        _ => None,
    }
}

pub(crate) fn data_number_to_string(value: &DataNumber) -> String {
    match value {
        DataNumber::U8(v) => v.to_string(),
        DataNumber::I8(v) => v.to_string(),
        DataNumber::U16(v) => v.to_string(),
        DataNumber::I16(v) => v.to_string(),
        DataNumber::U24(v) => v.to_string(),
        DataNumber::I24(v) => v.to_string(),
        DataNumber::U32(v) => v.to_string(),
        DataNumber::U64(v) => v.to_string(),
        DataNumber::I64(v) => v.to_string(),
        DataNumber::U128(v) => v.to_string(),
        DataNumber::I128(v) => v.to_string(),
        DataNumber::I32(v) => v.to_string(),
    }
}

pub(crate) fn bytes_to_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

pub(crate) fn reverse_ipfix_timestamp_to_usec(
    field: &ReverseInformationElement,
    value: &FieldValue,
    export_usec: u64,
    system_init_millis: Option<u64>,
) -> Option<u64> {
    match field {
        ReverseInformationElement::ReverseFlowStartSeconds
        | ReverseInformationElement::ReverseFlowEndSeconds => {
            field_value_unsigned(value).map(unix_seconds_to_usec)
        }
        ReverseInformationElement::ReverseFlowStartMilliseconds
        | ReverseInformationElement::ReverseFlowEndMilliseconds => {
            field_value_unsigned(value).map(|v| v.saturating_mul(USEC_PER_MILLISECOND))
        }
        ReverseInformationElement::ReverseFlowStartMicroseconds
        | ReverseInformationElement::ReverseFlowEndMicroseconds
        | ReverseInformationElement::ReverseMinFlowStartMicroseconds
        | ReverseInformationElement::ReverseMaxFlowEndMicroseconds => {
            field_value_duration_usec(value)
        }
        ReverseInformationElement::ReverseFlowStartNanoseconds
        | ReverseInformationElement::ReverseFlowEndNanoseconds
        | ReverseInformationElement::ReverseMinFlowStartNanoseconds
        | ReverseInformationElement::ReverseMaxFlowEndNanoseconds => {
            field_value_duration_usec(value)
        }
        ReverseInformationElement::ReverseFlowStartDeltaMicroseconds
        | ReverseInformationElement::ReverseFlowEndDeltaMicroseconds => {
            field_value_unsigned(value).map(|delta| export_usec.saturating_sub(delta))
        }
        ReverseInformationElement::ReverseFlowStartSysUpTime
        | ReverseInformationElement::ReverseFlowEndSysUpTime => {
            let system_init_usec = system_init_millis?.saturating_mul(USEC_PER_MILLISECOND);
            field_value_unsigned(value).map(|uptime_millis| {
                system_init_usec.saturating_add(uptime_millis.saturating_mul(USEC_PER_MILLISECOND))
            })
        }
        _ => None,
    }
}

pub(crate) fn resolve_ipfix_time_usec(
    seconds: Option<u64>,
    millis: Option<u64>,
    micros: Option<u64>,
    nanos: Option<u64>,
    delta_micros: Option<u64>,
    sys_uptime_millis: Option<u64>,
    system_init_millis: Option<u64>,
    export_usec: u64,
) -> Option<u64> {
    seconds
        .map(unix_seconds_to_usec)
        .or_else(|| millis.map(|value| value.saturating_mul(USEC_PER_MILLISECOND)))
        .or(micros)
        .or(nanos)
        .or_else(|| delta_micros.map(|value| export_usec.saturating_sub(value)))
        .or_else(|| {
            let system_init_usec = system_init_millis?.saturating_mul(USEC_PER_MILLISECOND);
            let uptime_millis = sys_uptime_millis?;
            Some(
                system_init_usec.saturating_add(uptime_millis.saturating_mul(USEC_PER_MILLISECOND)),
            )
        })
}

pub(crate) fn decode_sampling_interval(raw: u16) -> u32 {
    let interval = raw & 0x3fff;
    if interval == 0 { 1 } else { interval as u32 }
}

pub(crate) fn template_scope(payload: &[u8]) -> Option<(u16, u32)> {
    if payload.len() < 2 {
        return None;
    }
    let version = u16::from_be_bytes([payload[0], payload[1]]);
    match version {
        9 => {
            if payload.len() < 20 {
                return None;
            }
            let source_id =
                u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
            Some((version, source_id))
        }
        10 => {
            if payload.len() < 16 {
                return None;
            }
            let observation_domain_id =
                u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
            Some((version, observation_domain_id))
        }
        _ => None,
    }
}

pub(crate) fn unix_timestamp_to_usec(seconds: u64, nanos: u64) -> u64 {
    seconds
        .saturating_mul(1_000_000)
        .saturating_add(nanos / 1_000)
}

pub(crate) fn now_usec() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_micros() as u64)
        .unwrap_or(0)
}

#[cfg(test)]
pub(crate) fn to_field_token(name: &str) -> String {
    let mut out = String::with_capacity(name.len() + 8);
    let mut prev_is_sep = true;
    let mut prev_is_lower_or_digit = false;

    for ch in name.chars() {
        if ch.is_ascii_alphanumeric() {
            if ch.is_ascii_uppercase() && prev_is_lower_or_digit && !out.ends_with('_') {
                out.push('_');
            }
            if ch.is_ascii_digit() && prev_is_lower_or_digit && !out.ends_with('_') {
                out.push('_');
            }
            out.push(ch.to_ascii_uppercase());
            prev_is_sep = false;
            prev_is_lower_or_digit = ch.is_ascii_lowercase() || ch.is_ascii_digit();
        } else {
            if !prev_is_sep && !out.ends_with('_') {
                out.push('_');
            }
            prev_is_sep = true;
            prev_is_lower_or_digit = false;
        }
    }

    while out.ends_with('_') {
        out.pop();
    }

    if out.is_empty() {
        "UNKNOWN".to_string()
    } else {
        out
    }
}
