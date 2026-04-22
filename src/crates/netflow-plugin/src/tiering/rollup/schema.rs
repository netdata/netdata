use super::*;

mod direction;
mod fields;
mod presence;

pub(crate) const HOUR_BUCKET_USEC: u64 = 60 * 60 * 1_000_000;

pub(crate) use direction::{direction_as_rollup_value, direction_from_u8};
pub(crate) use fields::{ROLLUP_FIELD_DEFS, build_rollup_flow_index, rollup_field_index};
#[cfg(test)]
pub(crate) use presence::rollup_presence_field;
pub(crate) use presence::{
    INTERNAL_DIRECTION_PRESENT, INTERNAL_EXPORTER_IP_PRESENT, INTERNAL_NEXT_HOP_PRESENT,
    ROLLUP_PRESENCE_FIELDS, is_internal_rollup_presence_field,
};
