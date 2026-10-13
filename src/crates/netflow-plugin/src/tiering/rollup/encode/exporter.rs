use super::super::*;
use super::push_field_id;

pub(super) fn push_exporter_field_ids(
    index: &mut FlowIndex,
    rec: &FlowRecord,
    missing_ip: IpAddr,
    scratch_field_ids: &mut Vec<u32>,
) -> Result<(), FlowIndexError> {
    push_field_id(
        index,
        scratch_field_ids,
        15,
        IndexFieldValue::U8(u8::from(rec.exporter_ip.is_some())),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        16,
        IndexFieldValue::IpAddr(rec.exporter_ip.unwrap_or(missing_ip)),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        17,
        IndexFieldValue::U16(rec.exporter_port),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        18,
        IndexFieldValue::Text(rec.exporter_name.as_str()),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        19,
        IndexFieldValue::Text(rec.exporter_group.as_str()),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        20,
        IndexFieldValue::Text(rec.exporter_role.as_str()),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        21,
        IndexFieldValue::Text(rec.exporter_site.as_str()),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        22,
        IndexFieldValue::Text(rec.exporter_region.as_str()),
    )?;
    push_field_id(
        index,
        scratch_field_ids,
        23,
        IndexFieldValue::Text(rec.exporter_tenant.as_str()),
    )
}
