//! WAL frame → log-row decoding, shared by every frame consumer.
//!
//! [`decode_frame`] is the one implementation of the OTAP-frame-to-rows
//! decode: Arrow IPC parsing, attribute resolution through the parent-id
//! linkage, the scope/severity column projections, and the three-tier
//! timestamp fallback. Consumers receive the rows through the [`KvSink`]
//! trait rather than as materialized values, so each keeps its own
//! interning/dedup strategy:
//!
//! - the indexer's `RowIndex` (in the `sfst-indexer` crate) interns into
//!   its arena-backed interner and builds posting bitmaps;
//! - a query-time WAL row scan dedups into a per-query pair table and
//!   evaluates filters row by row.
//!
//! Keeping the interning point *inside* the consumer preserves the
//! `_nd_kv_hash` fast path: attribute pairs repeat across thousands of
//! rows, and a hash-only [`KvSink::lookup_hash`] hit skips `key=value`
//! string formatting entirely (see the `arrow_columns` module docs for
//! the producer-side hash contract). Sharing the decode also means two
//! consumers can never disagree about *what rows a frame contains* —
//! only about how they evaluate them.

use std::fmt::Write;

use arrow::array::*;
use arrow::record_batch::RecordBatch;

use super::arrow_columns::{AttrsMap, DictUtf8};
use super::otap_frame::OtapFrame;
use crate::DecodeError;

/// Consumer of decoded log rows.
///
/// The decoder never stores `key=value` strings itself; it asks the sink
/// to intern each pair and hands rows back as token slices. Tokens are
/// whatever id type the sink uses internally.
pub trait KvSink {
    /// The sink's id for an interned `key=value` pair. Copied freely
    /// during decoding; rows reference pairs by token.
    type Token: Copy;

    /// Hash-only fast-path lookup. `hash` is the producer's
    /// `xxhash64("key=value")` carried in the frame's `_nd_kv_hash`
    /// sidecar. Return the token if this pair was interned before;
    /// return `None` to make the decoder format the string and call
    /// [`intern`](Self::intern).
    ///
    /// # Collision safety
    ///
    /// On `Some`, the decoder uses the token *without ever formatting
    /// the string* — there is nothing left to verify against. A sink
    /// may therefore return `Some` **only if** it can guarantee that
    /// exactly one `key=value` string maps to `hash`. xxhash64 is not
    /// collision-free (nor adversary-proof — log content is
    /// attacker-influenced), and a sink that answers by hash alone
    /// hands back the *other* string's token when two pairs collide,
    /// silently corrupting every consumer downstream. The indexer's
    /// interner upholds the rule by answering `None` for any hash it
    /// has seen more than one distinct string for (see
    /// `KeyValueInterner::lookup_hash` in the `sfst-indexer` crate).
    ///
    /// Returning `None` is always safe — only the formatting shortcut
    /// is lost. A sink without a collision-tracking hash index should
    /// simply always return `None` and dedup by string in
    /// [`intern`](Self::intern).
    fn lookup_hash(&mut self, hash: u64) -> Option<Self::Token>;

    /// Intern a formatted `key=value` string, deduplicating by the
    /// **full string**. `hash` is the producer's pre-computed
    /// `xxhash64(kv)` when the pair came from an attribute group with a
    /// `_nd_kv_hash` sidecar, `None` otherwise (projected columns such
    /// as `scope.name` or `severity_text`, or attribute groups without
    /// the sidecar — the sidecar is per-group, not per-frame, so one
    /// frame can mix both cases). Called whenever
    /// [`lookup_hash`](Self::lookup_hash) misses or no hash is
    /// available.
    ///
    /// # Collision safety
    ///
    /// `Some(hash)` does *not* mean the hash is new to the sink: the
    /// collision path arrives exactly this way —
    /// [`lookup_hash`](Self::lookup_hash) declined a
    /// known-but-ambiguous hash, and this call carries the same hash
    /// with a different string. A sink keeping a hash-keyed table must
    /// compare `kv` against the stored string on a hash hit and
    /// allocate a fresh token when they differ; a bare
    /// `entry(hash).or_insert(…)` hands back the *colliding* pair's
    /// token. See `KeyValueInterner::intern_with_hash` in the
    /// `sfst-indexer` crate for the reference implementation.
    fn intern(&mut self, hash: Option<u64>, kv: &str) -> Self::Token;

    /// Hint that up to `additional` rows are about to be delivered.
    /// Default no-op; sinks may reserve capacity.
    fn reserve_rows(&mut self, additional: usize) {
        let _ = additional;
    }

    /// One decoded log row: its timestamp (per the fallback rules on
    /// `decode_frame`) and the tokens of every `key=value` pair it
    /// carries, in stream order. The slice may contain repeated tokens
    /// (multi-valued fields emit one pair per value) and its backing
    /// buffer is reused between rows — copy what you need.
    ///
    /// Infallible by design: the decoder cannot be aborted mid-frame.
    /// If a future sink needs early termination (query cancellation, a
    /// work budget), the extension is mechanical — return
    /// `ControlFlow` here and thread it through `decode_frame`.
    fn row(&mut self, ts_ns: i64, tokens: &[Self::Token]);
}

/// Decode a single WAL frame and stream its log rows into `sink`.
///
/// For each log row in the frame, the sink receives one
/// [`row`](KvSink::row) call carrying:
///
/// - every resource/scope/log attribute as an interned `key=value`
///   token (resolved through the OTAP parent-id linkage);
/// - the projected `scope.name` / `scope.version` / `severity_text` /
///   `severity_number` pairs (top-level columns in the logs batch that
///   the attribute sidecars never carry; empty / zero values are the
///   proto defaults for "unset" and are skipped);
/// - the row's timestamp, following the OpenTelemetry Log Data Model
///   hierarchy: `time_unix_nano` if present and non-zero, else
///   `observed_time_unix_nano`, else the WAL frame's ingestion
///   timestamp `+ 1ns` per subsequent row to preserve intra-frame
///   ordering.
///
/// Returns the number of log rows processed.
pub(crate) fn decode_frame<S: KvSink>(
    wal_frame: &wal::Frame,
    sink: &mut S,
) -> Result<usize, DecodeError> {
    let otap_frame = OtapFrame::decode(wal_frame.data)?;
    let Some(logs_batch) = otap_frame.logs.as_ref() else {
        return Ok(0);
    };
    let ingestion_ns = wal_frame.timestamp_ns.0;

    // Build key=value lookup tables for each attribute level. Interning
    // happens here, once per distinct attribute row, not per log row.
    let resource_attrs = AttrsMap::build(otap_frame.resource_attrs.as_ref(), sink);
    let scope_attrs = AttrsMap::build(otap_frame.scope_attrs.as_ref(), sink);
    let log_attrs = AttrsMap::build(otap_frame.log_attrs.as_ref(), sink);

    // Each log row carries parent_id columns that link it to its
    // resource, scope, and log-level attribute rows.
    let resource_id_col = resolve_column::<UInt16Array>(logs_batch, &["resource", "id"]);
    let scope_id_col = resolve_column::<UInt16Array>(logs_batch, &["scope", "id"]);
    let log_id_col = resolve_column::<UInt16Array>(logs_batch, &["id"]);

    // scope.name and scope.version live as columns in the logs batch's
    // scope struct, not as rows in ScopeAttrs.
    let scope_name_col = resolve_scope_utf8(logs_batch, "name");
    let scope_version_col = resolve_scope_utf8(logs_batch, "version");

    // severity_text and severity_number are top-level LogRecord scalar
    // columns. The OTAP-flattened attrs sidecars never carry them, so
    // the attrs maps above miss them; project them into each row here
    // so the field table gets a low-cardinality, query-useful facet for
    // log-level filtering.
    let severity_text_col = logs_batch
        .column_by_name("severity_text")
        .and_then(|c| DictUtf8::try_from(c.as_ref()));
    let severity_number_col = logs_batch
        .column_by_name("severity_number")
        .and_then(|c| c.as_any().downcast_ref::<Int32Array>());

    let time_col = logs_batch
        .column_by_name("time_unix_nano")
        .and_then(|c| c.as_any().downcast_ref::<TimestampNanosecondArray>());
    let observed_col = logs_batch
        .column_by_name("observed_time_unix_nano")
        .and_then(|c| c.as_any().downcast_ref::<TimestampNanosecondArray>());

    // Cast the WAL ingestion timestamp (u64) once per frame using a
    // saturating conversion: i64::MAX is ~year 2262 in nanoseconds,
    // well past any realistic ingestion clock, but a wrapping `as i64`
    // would silently flip to negative if it ever happened. Same for
    // the per-row offset that preserves intra-frame ordering.
    let base_ns = i64::try_from(ingestion_ns).unwrap_or(i64::MAX);

    sink.reserve_rows(logs_batch.num_rows());
    let mut kv_buf = String::new();
    let mut row_tokens: Vec<S::Token> = Vec::new();

    for row in 0..logs_batch.num_rows() {
        row_tokens.clear();

        for (id_col, attrs) in [
            (resource_id_col, &resource_attrs),
            (scope_id_col, &scope_attrs),
            (log_id_col, &log_attrs),
        ] {
            let Some(col) = id_col else { continue };

            if col.is_null(row) {
                continue;
            }

            row_tokens.extend_from_slice(attrs.get(col.value(row)));
        }

        // Intern scope.{name,version} and severity_text from the logs
        // batch columns. Empty values are the proto-default for
        // unset / "unknown" — skip them so they don't show up as a
        // single-value facet that's just noise.
        for (col, prefix) in [
            (&scope_name_col, "scope.name="),
            (&scope_version_col, "scope.version="),
            (&severity_text_col, "severity_text="),
        ] {
            if let Some(val) = col.as_ref().and_then(|c| c.value(row)) {
                if !val.is_empty() {
                    kv_buf.clear();
                    kv_buf.push_str(prefix);
                    kv_buf.push_str(val);
                    row_tokens.push(sink.intern(None, &kv_buf));
                }
            }
        }

        // severity_number is an int32 enum (OTel `SeverityNumber`).
        // `0` = UNSPECIFIED per the spec — same noise-skip rule as
        // empty strings.
        if let Some(col) = severity_number_col {
            if !col.is_null(row) {
                let n = col.value(row);
                if n != 0 {
                    kv_buf.clear();
                    let _ = write!(kv_buf, "severity_number={n}");
                    row_tokens.push(sink.intern(None, &kv_buf));
                }
            }
        }

        let ts = ts_value(time_col, row)
            .filter(|&v| v != 0)
            .or_else(|| ts_value(observed_col, row))
            .filter(|&v| v != 0)
            .unwrap_or_else(|| {
                let offset = i64::try_from(row).unwrap_or(i64::MAX);
                base_ns.saturating_add(offset)
            });

        sink.row(ts, &row_tokens);
    }

    Ok(logs_batch.num_rows())
}

/// Read the non-null timestamp at `row`, or `None`.
fn ts_value(col: Option<&TimestampNanosecondArray>, row: usize) -> Option<i64> {
    col.and_then(|c| {
        if c.is_null(row) {
            None
        } else {
            Some(c.value(row))
        }
    })
}

/// Resolve a DictUtf8 column from the `scope` struct in the logs batch.
fn resolve_scope_utf8<'a>(batch: &'a RecordBatch, field: &str) -> Option<DictUtf8<'a>> {
    let scope_struct = batch
        .column_by_name("scope")?
        .as_any()
        .downcast_ref::<StructArray>()?;
    let col = scope_struct.column_by_name(field)?;
    DictUtf8::try_from(col.as_ref())
}

/// Resolve a column by path, navigating through nested StructArrays.
fn resolve_column<'a, T: 'static>(batch: &'a RecordBatch, path: &[&str]) -> Option<&'a T> {
    let mut col = batch.column_by_name(path.first()?)?;

    for &segment in &path[1..] {
        col = col
            .as_any()
            .downcast_ref::<StructArray>()?
            .column_by_name(segment)?;
    }

    col.as_any().downcast_ref::<T>()
}
