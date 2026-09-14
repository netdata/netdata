use super::*;

pub(crate) fn field_value_to_string(value: &FieldValue) -> String {
    match value {
        FieldValue::ApplicationId(app) => {
            format!(
                "{}:{}",
                app.classification_engine_id,
                app.selector_id
                    .as_ref()
                    .map(data_number_to_string)
                    .unwrap_or_default()
            )
        }
        FieldValue::String(v) => v.value.clone(),
        FieldValue::DataNumber(v) => data_number_to_string(v),
        FieldValue::Float64(v) => v.to_string(),
        FieldValue::Duration(v) => v.as_duration().as_millis().to_string(),
        FieldValue::Ip4Addr(v) => v.to_string(),
        FieldValue::Ip6Addr(v) => v.to_string(),
        FieldValue::MacAddr(v) => format!(
            "{:02X}:{:02X}:{:02X}:{:02X}:{:02X}:{:02X}",
            v[0], v[1], v[2], v[3], v[4], v[5]
        ),
        FieldValue::Vec(v) => bytes_to_hex(v),
        FieldValue::ProtocolType(v) => u8::from(*v).to_string(),
        FieldValue::ForwardingStatus(v) => u8::from(*v).to_string(),
        FieldValue::FragmentFlags(v) => u8::from(*v).to_string(),
        FieldValue::TcpControlBits(v, _) => u16::from(*v).to_string(),
        FieldValue::Ipv6ExtensionHeaders(v) => u32::from(*v).to_string(),
        FieldValue::Ipv4Options(v) => u32::from(*v).to_string(),
        FieldValue::TcpOptions(v) => u64::from(*v).to_string(),
        FieldValue::IsMulticast(v) => u8::from(*v).to_string(),
        FieldValue::MplsLabelExp(v) => u8::from(*v).to_string(),
        FieldValue::FlowEndReason(v) => u8::from(*v).to_string(),
        FieldValue::NatEvent(v) => u8::from(*v).to_string(),
        FieldValue::FirewallEvent(v) => u8::from(*v).to_string(),
        FieldValue::MplsTopLabelType(v) => u8::from(*v).to_string(),
        FieldValue::NatOriginatingAddressRealm(v) => u8::from(*v).to_string(),
        other => {
            let mut bytes = Vec::new();
            other
                .write_be_bytes(&mut bytes)
                .map(|()| bytes_to_hex(&bytes))
                .unwrap_or_default()
        }
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
            DataNumber::Vec(v) => decode_be_unsigned(v).and_then(|v| u64::try_from(v).ok()),
            _ => None,
        },
        FieldValue::Vec(bytes) if (1..=size_of::<u64>()).contains(&bytes.len()) => {
            decode_be_unsigned(bytes).and_then(|value| u64::try_from(value).ok())
        }
        _ => None,
    }
}

pub(crate) fn field_value_duration_usec(value: &FieldValue) -> Option<u64> {
    match value {
        FieldValue::Duration(duration) => u64::try_from(duration.as_duration().as_micros()).ok(),
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
        DataNumber::Vec(v) => decode_be_unsigned(v)
            .map(|value| value.to_string())
            .unwrap_or_else(|| bytes_to_hex(v)),
    }
}

fn decode_be_unsigned(bytes: &[u8]) -> Option<u128> {
    if bytes.is_empty() || bytes.len() > size_of::<u128>() {
        return None;
    }

    Some(
        bytes
            .iter()
            .fold(0_u128, |value, byte| (value << 8) | u128::from(*byte)),
    )
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn raw_field_values_decode_unsigned_widths_one_through_eight() {
        for width in 1..=8 {
            let mut bytes = vec![0_u8; width];
            bytes[width - 1] = 0x10;
            assert_eq!(field_value_unsigned(&FieldValue::Vec(bytes)), Some(16));
        }
    }

    #[test]
    fn raw_field_values_reject_empty_and_overwide_integers() {
        assert_eq!(field_value_unsigned(&FieldValue::Vec(Vec::new())), None);
        assert_eq!(field_value_unsigned(&FieldValue::Vec(vec![0_u8; 9])), None);
    }
}
