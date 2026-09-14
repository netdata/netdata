use super::common::build_u8_label_map;
use super::*;

pub(super) static IPTOS_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| build_u8_label_map(ip_tos_name_from_u8));

fn dscp_name(value: u8) -> Option<&'static str> {
    match value {
        0 => Some("CS0"),
        1 => Some("LE"),
        8 => Some("CS1"),
        10 => Some("AF11"),
        12 => Some("AF12"),
        14 => Some("AF13"),
        16 => Some("CS2"),
        18 => Some("AF21"),
        20 => Some("AF22"),
        22 => Some("AF23"),
        24 => Some("CS3"),
        26 => Some("AF31"),
        28 => Some("AF32"),
        30 => Some("AF33"),
        32 => Some("CS4"),
        34 => Some("AF41"),
        36 => Some("AF42"),
        38 => Some("AF43"),
        40 => Some("CS5"),
        44 => Some("VOICE-ADMIT"),
        45 => Some("NQB"),
        46 => Some("EF"),
        48 => Some("CS6"),
        56 => Some("CS7"),
        _ => None,
    }
}

fn ecn_name(value: u8) -> &'static str {
    match value {
        0 => "Not-ECT",
        1 => "ECT(1)",
        2 => "ECT(0)",
        3 => "CE",
        _ => unreachable!("ECN is 2 bits"),
    }
}

pub(super) fn ip_tos_name(value: &str) -> Option<String> {
    value.parse::<u8>().ok().and_then(ip_tos_name_from_u8)
}

fn ip_tos_name_from_u8(value: u8) -> Option<String> {
    let dscp = value >> 2;
    let ecn = value & 0b11;
    let dscp_name = dscp_name(dscp)?;
    Some(format!("{dscp_name} / {}", ecn_name(ecn)))
}
