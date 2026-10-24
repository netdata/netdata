use super::*;

pub(super) fn encode_interface_journal_fields(
    record: &FlowRecord,
    writer: &mut JournalBufWriter<'_>,
) {
    writer.push_u32("IN_IF", record.in_if);
    writer.push_u32("OUT_IF", record.out_if);
    writer.push_str("IN_IF_NAME", &record.in_if_name);
    writer.push_str("OUT_IF_NAME", &record.out_if_name);
    writer.push_str("IN_IF_DESCRIPTION", &record.in_if_description);
    writer.push_str("OUT_IF_DESCRIPTION", &record.out_if_description);
    writer.push_u64_when(record.has_in_if_speed(), "IN_IF_SPEED", record.in_if_speed);
    writer.push_u64_when(
        record.has_out_if_speed(),
        "OUT_IF_SPEED",
        record.out_if_speed,
    );
    writer.push_str("IN_IF_PROVIDER", &record.in_if_provider);
    writer.push_str("OUT_IF_PROVIDER", &record.out_if_provider);
    writer.push_str("IN_IF_CONNECTIVITY", &record.in_if_connectivity);
    writer.push_str("OUT_IF_CONNECTIVITY", &record.out_if_connectivity);
    writer.push_u8_when(
        record.has_in_if_boundary(),
        "IN_IF_BOUNDARY",
        record.in_if_boundary,
    );
    writer.push_u8_when(
        record.has_out_if_boundary(),
        "OUT_IF_BOUNDARY",
        record.out_if_boundary,
    );
}
