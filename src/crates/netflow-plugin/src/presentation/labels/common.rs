use super::*;

pub(super) fn exact_label_map(pairs: &[(&str, &str)]) -> Map<String, Value> {
    pairs
        .iter()
        .map(|(key, value)| ((*key).to_string(), Value::String((*value).to_string())))
        .collect()
}

pub(super) fn build_u8_label_map(labeler: fn(u8) -> Option<String>) -> Map<String, Value> {
    (0u8..=u8::MAX)
        .filter_map(|value| labeler(value).map(|label| (value.to_string(), Value::String(label))))
        .collect()
}

pub(super) fn exact_label<'a>(
    pairs: &'a [(&'static str, &'static str)],
    value: &str,
) -> Option<&'a str> {
    pairs
        .iter()
        .find_map(|(candidate, label)| (*candidate == value).then_some(*label))
}
