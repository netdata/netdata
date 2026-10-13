use super::*;

pub(super) const EXPORTER_ROLLUP_FIELD_DEFS: &[RollupFieldDef] = &[
    rollup_field_def(INTERNAL_EXPORTER_IP_PRESENT, IndexFieldKind::U8),
    rollup_field_def("EXPORTER_IP", IndexFieldKind::IpAddr),
    rollup_field_def("EXPORTER_PORT", IndexFieldKind::U16),
    rollup_field_def("EXPORTER_NAME", IndexFieldKind::Text),
    rollup_field_def("EXPORTER_GROUP", IndexFieldKind::Text),
    rollup_field_def("EXPORTER_ROLE", IndexFieldKind::Text),
    rollup_field_def("EXPORTER_SITE", IndexFieldKind::Text),
    rollup_field_def("EXPORTER_REGION", IndexFieldKind::Text),
    rollup_field_def("EXPORTER_TENANT", IndexFieldKind::Text),
];
