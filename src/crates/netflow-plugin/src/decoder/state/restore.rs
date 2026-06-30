use super::*;

mod ipfix;
mod v9;

use ipfix::build_ipfix_restore_packet;
use v9::build_v9_restore_packet;

pub(super) fn sum_v9_field_lengths(fields: &[PersistedV9TemplateField]) -> u16 {
    fields.iter().map(|field| field.field_length).sum()
}

pub(crate) fn build_namespace_restore_packets(
    key: &DecoderStateNamespaceKey,
    namespace: &DecoderStateNamespace,
) -> Result<Vec<Vec<u8>>, String> {
    let mut packets = Vec::new();

    if let Some(packet) = build_v9_restore_packet(key, namespace)? {
        packets.push(packet);
    }

    if let Some(packet) = build_ipfix_restore_packet(key, namespace)? {
        packets.push(packet);
    }

    Ok(packets)
}
