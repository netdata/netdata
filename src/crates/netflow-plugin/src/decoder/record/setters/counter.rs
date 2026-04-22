use super::*;

pub(super) fn set_record_counter_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "PROTOCOL" => {
            rec.protocol = value.parse().unwrap_or(0);
            true
        }
        "BYTES" => {
            rec.bytes = value.parse().unwrap_or(0);
            true
        }
        "PACKETS" => {
            rec.packets = value.parse().unwrap_or(0);
            true
        }
        "FLOWS" => {
            rec.flows = value.parse().unwrap_or(0);
            true
        }
        "RAW_BYTES" => {
            rec.raw_bytes = value.parse().unwrap_or(0);
            true
        }
        "RAW_PACKETS" => {
            rec.raw_packets = value.parse().unwrap_or(0);
            true
        }
        _ => false,
    }
}
