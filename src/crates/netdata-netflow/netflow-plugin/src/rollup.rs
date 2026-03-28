use crate::flow::FlowFields;

const EXCLUDED_DIMENSIONS: [&str; 4] = ["SRC_ADDR", "DST_ADDR", "SRC_PORT", "DST_PORT"];

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) struct RollupKey(pub(crate) Vec<(&'static str, String)>);

pub(crate) fn build_rollup_key(dimensions: &FlowFields) -> RollupKey {
    let mut key = Vec::with_capacity(dimensions.len());
    for (&name, value) in dimensions {
        if is_excluded(name) {
            continue;
        }
        key.push((name, value.clone()));
    }
    RollupKey(key)
}

pub(crate) fn is_excluded(name: &str) -> bool {
    EXCLUDED_DIMENSIONS.contains(&name)
}

#[cfg(test)]
mod tests {
    use super::{build_rollup_key, is_excluded};
    use std::collections::BTreeMap;

    #[test]
    fn excludes_only_ip_and_ports() {
        let mut dims = BTreeMap::new();
        dims.insert("SRC_ADDR", "10.0.0.1".to_string());
        dims.insert("DST_ADDR", "10.0.0.2".to_string());
        dims.insert("SRC_PORT", "12345".to_string());
        dims.insert("DST_PORT", "443".to_string());
        dims.insert("PROTOCOL", "6".to_string());
        dims.insert("SRC_AS", "64512".to_string());
        dims.insert("DST_AS", "15169".to_string());
        dims.insert("DIRECTION", "ingress".to_string());
        dims.insert("EXPORTER_IP", "192.0.2.10".to_string());

        let key = build_rollup_key(&dims);

        assert_eq!(key.0.len(), 5);
        assert!(key.0.iter().any(|(k, _)| *k == "PROTOCOL"));
        assert!(key.0.iter().any(|(k, _)| *k == "SRC_AS"));
        assert!(key.0.iter().any(|(k, _)| *k == "DST_AS"));
        assert!(key.0.iter().any(|(k, _)| *k == "DIRECTION"));
        assert!(key.0.iter().any(|(k, _)| *k == "EXPORTER_IP"));
        assert!(!key.0.iter().any(|(k, _)| *k == "SRC_ADDR"));
        assert!(!key.0.iter().any(|(k, _)| *k == "DST_ADDR"));
        assert!(!key.0.iter().any(|(k, _)| *k == "SRC_PORT"));
        assert!(!key.0.iter().any(|(k, _)| *k == "DST_PORT"));
    }

    #[test]
    fn stable_key_order_is_lexicographic() {
        let mut dims = BTreeMap::new();
        dims.insert("PROTOCOL", "17".to_string());
        dims.insert("DIRECTION", "egress".to_string());
        dims.insert("EXPORTER_IP", "198.51.100.1".to_string());

        let key = build_rollup_key(&dims);
        let names: Vec<_> = key.0.into_iter().map(|(k, _)| k).collect();

        assert_eq!(names, vec!["DIRECTION", "EXPORTER_IP", "PROTOCOL"]);
    }

    #[test]
    fn exclusion_match_is_exact() {
        assert!(is_excluded("SRC_ADDR"));
        assert!(is_excluded("DST_ADDR"));
        assert!(is_excluded("SRC_PORT"));
        assert!(is_excluded("DST_PORT"));

        assert!(!is_excluded("SRC_AS"));
        assert!(!is_excluded("DST_AS"));
        assert!(!is_excluded("SRC_ADDR_PREFIX"));
    }
}
