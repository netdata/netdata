//! Shared test fixtures.
//!
//! Only compiled under `#[cfg(test)]`; use across the crate's test modules
//! to avoid duplicating common construction patterns.

/// Derive the substrate identity fields — the partition key and the opaque
/// `content_meta` blob — for a stream, exactly as production indexing does.
/// Returns `(part_key, content_meta)`.
pub(crate) fn identity_for(stream: &sfst::ServiceStream) -> (u64, Vec<u8>) {
    (
        otel_logs_identity::part_key(stream),
        otel_logs_identity::encode_content_meta(stream)
            .expect("test service identity encodes within content_meta limits"),
    )
}

/// A `Summary` for `stream` with the given range and record count, deriving
/// `part_key`/`content_meta` the way production does.
pub(crate) fn summary_for(
    stream: &sfst::ServiceStream,
    record_count: u32,
    min_s: u32,
    max_s: u32,
) -> sfst::Summary {
    let (part_key, content_meta) = identity_for(stream);
    sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count,
        part_key,
        content_meta,
    }
}

/// A `Summary` populated with zero-valued fields and an empty stream
/// identity. Used by registry/recovery tests that need to call
/// `Registry::track` without caring about the summary's contents.
pub(crate) fn empty_summary() -> sfst::Summary {
    summary_for(&sfst::ServiceStream::new("", ""), 0, 0, 0)
}
