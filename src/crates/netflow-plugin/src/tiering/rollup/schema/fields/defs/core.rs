use super::*;

pub(super) const CORE_ROLLUP_FIELD_DEFS: &[RollupFieldDef] = &[
    rollup_field_def("DIRECTION", IndexFieldKind::U8),
    rollup_field_def("PROTOCOL", IndexFieldKind::U8),
    rollup_field_def("ETYPE", IndexFieldKind::U16),
    rollup_field_def("FORWARDING_STATUS", IndexFieldKind::U8),
    rollup_field_def("FLOW_VERSION", IndexFieldKind::Text),
    rollup_field_def("IPTOS", IndexFieldKind::U8),
    rollup_field_def("TCP_FLAGS", IndexFieldKind::U8),
    rollup_field_def("ICMPV4_TYPE", IndexFieldKind::U8),
    rollup_field_def("ICMPV4_CODE", IndexFieldKind::U8),
    rollup_field_def("ICMPV6_TYPE", IndexFieldKind::U8),
    rollup_field_def("ICMPV6_CODE", IndexFieldKind::U8),
    rollup_field_def("SRC_AS", IndexFieldKind::U32),
    rollup_field_def("DST_AS", IndexFieldKind::U32),
    rollup_field_def("SRC_AS_NAME", IndexFieldKind::Text),
    rollup_field_def("DST_AS_NAME", IndexFieldKind::Text),
];
