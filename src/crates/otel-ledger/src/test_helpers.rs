//! Shared test fixtures.
//!
//! Only compiled under `#[cfg(test)]`; use across the crate's test modules
//! to avoid duplicating common construction patterns.

/// Derive the substrate identity fields — the partition key and the opaque
/// `content_meta` blob — for a stream, exactly as production indexing does.
/// Returns `(part_key, content_meta)`.
pub(crate) fn identity_for(stream: &otel_logs_identity::ServiceStream) -> (u64, Vec<u8>) {
    (
        otel_logs_identity::part_key(stream),
        otel_logs_identity::encode_content_meta(stream)
            .expect("test service identity encodes within content_meta limits"),
    )
}

/// A `Summary` for `stream` with the given range and record count, deriving
/// `content_meta` the way production does. The partition key is NOT in the
/// summary — it lives in the file's `FileId`; tests that filter by partition
/// pass the key via [`identity_for`] into the `FileId`.
pub(crate) fn summary_for(
    stream: &otel_logs_identity::ServiceStream,
    record_count: u32,
    min_s: u32,
    max_s: u32,
) -> sfst::Summary {
    sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count,
        content_meta: otel_logs_identity::encode_content_meta(stream)
            .expect("test service identity encodes within content_meta limits"),
    }
}
