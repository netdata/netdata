use crate::flow::{FlowDirection, FlowFields, FlowRecord};
use crate::flow_index::{
    FieldKind as IndexFieldKind, FieldSpec as IndexFieldSpec, FieldValue as IndexFieldValue,
    FlowId as IndexedFlowId, FlowIndex, FlowIndexError,
};
use std::net::{IpAddr, Ipv4Addr};

mod encode;
mod emit;
mod materialize;
mod schema;

#[allow(unused_imports)]
pub(crate) use encode::push_rollup_field_ids;
pub(crate) use emit::emit_rollup_row;
#[cfg(test)]
pub(crate) use materialize::dimensions_for_rollup;
#[allow(unused_imports)]
pub(crate) use materialize::{
    bucket_start_usec, compact_index_value_to_string, materialize_rollup_fields, rollup_field_value,
};
#[cfg(test)]
pub(crate) use schema::rollup_presence_field;
#[allow(unused_imports)]
pub(crate) use schema::{
    HOUR_BUCKET_USEC, INTERNAL_DIRECTION_PRESENT, INTERNAL_EXPORTER_IP_PRESENT,
    INTERNAL_NEXT_HOP_PRESENT, build_rollup_flow_index, direction_from_u8,
};
