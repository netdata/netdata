use super::super::schema::direction_as_rollup_value;
use super::super::*;
use super::push_field_id;

pub(super) fn push_core_field_ids(
    index: &mut FlowIndex,
    rec: &FlowRecord,
    scratch_field_ids: &mut Vec<u32>,
) -> Result<(), FlowIndexError> {
    push_field_id(
        index,
        scratch_field_ids,
        0,
        IndexFieldValue::U8(direction_as_rollup_value(rec.direction)),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        1,
        IndexFieldValue::U8(rec.protocol),
    )?;
    push_field_id(index, scratch_field_ids, 2, IndexFieldValue::U16(rec.etype))?;
    push_field_id(
        index,
        scratch_field_ids,
        3,
        IndexFieldValue::U8(rec.forwarding_status),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        4,
        IndexFieldValue::Text(rec.flow_version),
    )?;
    push_field_id(index, scratch_field_ids, 5, IndexFieldValue::U8(rec.iptos))?;
    push_field_id(
        index,
        scratch_field_ids,
        6,
        IndexFieldValue::U8(rec.tcp_flags),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        7,
        IndexFieldValue::U8(rec.icmpv4_type),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        8,
        IndexFieldValue::U8(rec.icmpv4_code),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        9,
        IndexFieldValue::U8(rec.icmpv6_type),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        10,
        IndexFieldValue::U8(rec.icmpv6_code),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        11,
        IndexFieldValue::U32(rec.src_as),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        12,
        IndexFieldValue::U32(rec.dst_as),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        13,
        IndexFieldValue::Text(rec.src_as_name.as_str()),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        14,
        IndexFieldValue::Text(rec.dst_as_name.as_str()),
    )
}
