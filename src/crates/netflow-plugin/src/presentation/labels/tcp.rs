use super::common::build_u8_label_map;
use super::*;

pub(super) static TCP_FLAGS_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| build_u8_label_map(|value| Some(tcp_flags_name_from_u8(value))));

pub(super) fn tcp_flags_name(value: &str) -> Option<String> {
    value.parse::<u8>().ok().map(tcp_flags_name_from_u8)
}

fn tcp_flags_name_from_u8(value: u8) -> String {
    if value == 0 {
        return "No Flags".to_string();
    }

    let mut flags = Vec::new();
    for (bit, name) in [
        (0x01, "FIN"),
        (0x02, "SYN"),
        (0x04, "RST"),
        (0x08, "PSH"),
        (0x10, "ACK"),
        (0x20, "URG"),
        (0x40, "ECE"),
        (0x80, "CWR"),
    ] {
        if value & bit != 0 {
            flags.push(name);
        }
    }
    flags.join("|")
}
