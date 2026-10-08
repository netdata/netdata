use super::*;

mod counter;
mod exporter;
mod interface;
mod network;
mod transport;

use counter::set_record_counter_field;
use exporter::set_record_exporter_field;
use interface::set_record_interface_field;
use network::set_record_network_field;
use transport::set_record_transport_field;

/// Set a field on FlowRecord by canonical name (string dispatch).
/// Used by V9/IPFIX template-driven decode where field names come from template mapping.
pub(crate) fn set_record_field(rec: &mut FlowRecord, key: &str, value: &str) {
    if set_record_exporter_field(rec, key, value)
        || set_record_counter_field(rec, key, value)
        || set_record_network_field(rec, key, value)
        || set_record_interface_field(rec, key, value)
        || set_record_transport_field(rec, key, value)
    {
        return;
    }
}

/// Like set_record_field, but bypasses its first-non-zero-wins rule for IN_IF/OUT_IF:
/// those always overwrite. Other keys delegate to set_record_field unchanged.
pub(crate) fn override_record_field(rec: &mut FlowRecord, key: &str, value: &str) {
    match key {
        "IN_IF" => {
            rec.in_if = value.parse().unwrap_or(0);
        }
        "OUT_IF" => {
            rec.out_if = value.parse().unwrap_or(0);
        }
        _ => set_record_field(rec, key, value),
    }
}

/// Copy bytes/packets into raw_bytes/raw_packets. The raw_* counters stay unscaled:
/// finalize_record scales bytes/packets by the sampling rate but never scales raw_*.
pub(crate) fn sync_raw_metrics_record(rec: &mut FlowRecord) {
    rec.raw_bytes = rec.bytes;
    rec.raw_packets = rec.packets;
}
