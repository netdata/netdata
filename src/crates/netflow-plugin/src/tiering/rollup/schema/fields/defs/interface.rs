use super::*;

pub(super) const INTERFACE_ROLLUP_FIELD_DEFS: &[RollupFieldDef] = &[
    rollup_field_def("IN_IF", IndexFieldKind::U32),
    rollup_field_def("OUT_IF", IndexFieldKind::U32),
    rollup_field_def("IN_IF_NAME", IndexFieldKind::Text),
    rollup_field_def("OUT_IF_NAME", IndexFieldKind::Text),
    rollup_field_def("IN_IF_DESCRIPTION", IndexFieldKind::Text),
    rollup_field_def("OUT_IF_DESCRIPTION", IndexFieldKind::Text),
    rollup_field_def("IN_IF_SPEED", IndexFieldKind::U64),
    rollup_field_def("OUT_IF_SPEED", IndexFieldKind::U64),
    rollup_field_def("IN_IF_PROVIDER", IndexFieldKind::Text),
    rollup_field_def("OUT_IF_PROVIDER", IndexFieldKind::Text),
    rollup_field_def("IN_IF_CONNECTIVITY", IndexFieldKind::Text),
    rollup_field_def("OUT_IF_CONNECTIVITY", IndexFieldKind::Text),
    rollup_field_def("IN_IF_BOUNDARY", IndexFieldKind::U8),
    rollup_field_def("OUT_IF_BOUNDARY", IndexFieldKind::U8),
];
