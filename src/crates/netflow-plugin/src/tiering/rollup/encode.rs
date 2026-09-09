use super::*;

mod core;
mod exporter;
mod interface;
mod network;
mod presence;

use core::push_core_field_ids;
use exporter::push_exporter_field_ids;
use interface::push_interface_field_ids;
use network::push_network_field_ids;
use presence::push_presence_field_ids;

pub(crate) fn push_rollup_field_ids(
    index: &mut FlowIndex,
    rec: &FlowRecord,
    scratch_field_ids: &mut Vec<u32>,
) -> Result<(), FlowIndexError> {
    let missing_ip = IpAddr::V4(Ipv4Addr::UNSPECIFIED);

    push_core_field_ids(index, rec, scratch_field_ids)?;
    push_exporter_field_ids(index, rec, missing_ip, scratch_field_ids)?;
    push_interface_field_ids(index, rec, scratch_field_ids)?;
    push_network_field_ids(index, rec, missing_ip, scratch_field_ids)?;
    push_presence_field_ids(index, rec, scratch_field_ids)?;

    Ok(())
}

fn push_field_id(
    index: &mut FlowIndex,
    scratch_field_ids: &mut Vec<u32>,
    field_index: usize,
    value: IndexFieldValue<'_>,
) -> Result<(), FlowIndexError> {
    scratch_field_ids.push(index.get_or_insert_field_value(field_index, value)?);
    Ok(())
}
