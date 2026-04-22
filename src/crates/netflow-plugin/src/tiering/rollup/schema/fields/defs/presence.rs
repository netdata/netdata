use super::*;

pub(super) const PRESENCE_ROLLUP_FIELD_DEFS: &[RollupFieldDef] = &[
    rollup_field_def(INTERNAL_DIRECTION_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_ETYPE_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_FORWARDING_STATUS_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_IPTOS_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_TCP_FLAGS_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_ICMPV4_TYPE_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_ICMPV4_CODE_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_ICMPV6_TYPE_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_ICMPV6_CODE_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_IN_IF_SPEED_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_OUT_IF_SPEED_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_IN_IF_BOUNDARY_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_OUT_IF_BOUNDARY_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_SRC_VLAN_PRESENT, IndexFieldKind::U8),
    rollup_field_def(INTERNAL_DST_VLAN_PRESENT, IndexFieldKind::U8),
];
