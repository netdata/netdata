use super::*;

/// Append u32 values as CSV to a String field.
pub(crate) fn append_u32_csv(target: &mut String, values: &[u32]) {
    if values.is_empty() {
        return;
    }
    for v in values {
        if !target.is_empty() {
            target.push(',');
        }
        let mut buf = itoa::Buffer::new();
        target.push_str(buf.format(*v));
    }
}

/// Append large communities as CSV to a String field.
pub(crate) fn append_large_communities_csv(
    target: &mut String,
    values: &[StaticRoutingLargeCommunity],
) {
    for lc in values {
        if !target.is_empty() {
            target.push(',');
        }
        target.push_str(&lc.format());
    }
}
