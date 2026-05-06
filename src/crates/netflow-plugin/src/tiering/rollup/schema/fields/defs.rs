use std::sync::LazyLock;

use super::super::presence::{
    INTERNAL_DIRECTION_PRESENT, INTERNAL_DST_VLAN_PRESENT, INTERNAL_ETYPE_PRESENT,
    INTERNAL_EXPORTER_IP_PRESENT, INTERNAL_FORWARDING_STATUS_PRESENT, INTERNAL_ICMPV4_CODE_PRESENT,
    INTERNAL_ICMPV4_TYPE_PRESENT, INTERNAL_ICMPV6_CODE_PRESENT, INTERNAL_ICMPV6_TYPE_PRESENT,
    INTERNAL_IN_IF_BOUNDARY_PRESENT, INTERNAL_IN_IF_SPEED_PRESENT, INTERNAL_IPTOS_PRESENT,
    INTERNAL_NEXT_HOP_PRESENT, INTERNAL_OUT_IF_BOUNDARY_PRESENT, INTERNAL_OUT_IF_SPEED_PRESENT,
    INTERNAL_SRC_VLAN_PRESENT, INTERNAL_TCP_FLAGS_PRESENT,
};
use super::*;

mod core;
mod exporter;
mod interface;
mod network;
mod presence;

use core::CORE_ROLLUP_FIELD_DEFS;
use exporter::EXPORTER_ROLLUP_FIELD_DEFS;
use interface::INTERFACE_ROLLUP_FIELD_DEFS;
use network::NETWORK_ROLLUP_FIELD_DEFS;
use presence::PRESENCE_ROLLUP_FIELD_DEFS;

#[derive(Clone, Copy)]
pub(crate) struct RollupFieldDef {
    pub(crate) name: &'static str,
    pub(crate) kind: IndexFieldKind,
}

pub(crate) const fn rollup_field_def(name: &'static str, kind: IndexFieldKind) -> RollupFieldDef {
    RollupFieldDef { name, kind }
}

pub(crate) static ROLLUP_FIELD_DEFS: LazyLock<Vec<RollupFieldDef>> = LazyLock::new(|| {
    let total = CORE_ROLLUP_FIELD_DEFS.len()
        + EXPORTER_ROLLUP_FIELD_DEFS.len()
        + INTERFACE_ROLLUP_FIELD_DEFS.len()
        + NETWORK_ROLLUP_FIELD_DEFS.len()
        + PRESENCE_ROLLUP_FIELD_DEFS.len();

    let mut defs = Vec::with_capacity(total);
    defs.extend_from_slice(CORE_ROLLUP_FIELD_DEFS);
    defs.extend_from_slice(EXPORTER_ROLLUP_FIELD_DEFS);
    defs.extend_from_slice(INTERFACE_ROLLUP_FIELD_DEFS);
    defs.extend_from_slice(NETWORK_ROLLUP_FIELD_DEFS);
    defs.extend_from_slice(PRESENCE_ROLLUP_FIELD_DEFS);
    defs
});
