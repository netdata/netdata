use super::*;

pub(super) const NETWORK_ROLLUP_FIELD_DEFS: &[RollupFieldDef] = &[
    rollup_field_def("SRC_NET_NAME", IndexFieldKind::Text),
    rollup_field_def("DST_NET_NAME", IndexFieldKind::Text),
    rollup_field_def("SRC_NET_ROLE", IndexFieldKind::Text),
    rollup_field_def("DST_NET_ROLE", IndexFieldKind::Text),
    rollup_field_def("SRC_NET_SITE", IndexFieldKind::Text),
    rollup_field_def("DST_NET_SITE", IndexFieldKind::Text),
    rollup_field_def("SRC_NET_REGION", IndexFieldKind::Text),
    rollup_field_def("DST_NET_REGION", IndexFieldKind::Text),
    rollup_field_def("SRC_NET_TENANT", IndexFieldKind::Text),
    rollup_field_def("DST_NET_TENANT", IndexFieldKind::Text),
    rollup_field_def("SRC_COUNTRY", IndexFieldKind::Text),
    rollup_field_def("DST_COUNTRY", IndexFieldKind::Text),
    rollup_field_def("SRC_GEO_STATE", IndexFieldKind::Text),
    rollup_field_def("DST_GEO_STATE", IndexFieldKind::Text),
    rollup_field_def(INTERNAL_NEXT_HOP_PRESENT, IndexFieldKind::U8),
    rollup_field_def("NEXT_HOP", IndexFieldKind::IpAddr),
    rollup_field_def("SRC_VLAN", IndexFieldKind::U16),
    rollup_field_def("DST_VLAN", IndexFieldKind::U16),
];
