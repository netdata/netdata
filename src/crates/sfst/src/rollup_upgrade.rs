//! Add the `TRSS` rollup stats to a traces file sealed before they
//! existed. The rollup and its stats are derived again from the file's
//! own rows — the seal's inputs read back — and the derivation is proven
//! by comparing its rollup with the stored `TRSU` before the container is
//! rewritten with `TRSS` after it. A development path (benchmark corpora);
//! no reader depends on it.

use crate::kv_interner::KvSlot;
use crate::trace_rollup::TraceRollupRows;
use crate::{IndexReader, KvId, TraceRollup, TraceRollupStats};

/// The seal's capture keys (`ng-index`'s `sfst_build.rs`): the first
/// matching token of each in a row is the one the seal recorded.
const SERVICE_PREFIX: &str = "resource.attributes.service.name=";
const NAME_PREFIX: &str = "name=";
const KIND_PREFIX: &str = "_kind=";
const STATUS_PREFIX: &str = "status_code=";

/// The rollup and stats the seal would write for this file, derived from
/// its per-row columns and each row's stored tokens.
pub fn derive_trace_rollup(
    reader: &IndexReader<'_>,
) -> Result<(TraceRollup, TraceRollupStats), crate::Error> {
    let corrupt = |what: &str| crate::Error::CorruptIndex(format!("rollup derivation: {what}"));
    let strings = reader.build_string_table(reader.field_table())?;
    let trace_ids = reader.trace_ids()?;
    let span_ids = reader.span_ids()?;
    let parents = reader.parent_span_ids()?;
    let durations = reader.durations()?;
    let timestamps = reader.load_timestamps()?;
    let mut rows = TraceRollupRows::new();
    let mut pos = 0usize;
    for b in 0..reader.num_stream_batches() {
        let batch = reader.load_stream_batch(b)?;
        for i in 0..batch.num_rows() {
            let (mut service, mut name, mut kind, mut is_error) = (None, None, None, None);
            for kv in batch.row(i) {
                let entry = strings
                    .get(kv.0 as usize)
                    .ok_or_else(|| corrupt("a row token is outside the string table"))?;
                if entry.starts_with(SERVICE_PREFIX) {
                    service.get_or_insert(KvSlot(kv.0));
                } else if entry.starts_with(NAME_PREFIX) {
                    name.get_or_insert(KvSlot(kv.0));
                } else if let Some(value) = entry.strip_prefix(KIND_PREFIX) {
                    if kind.is_none()
                        && let Ok(k) = value.parse::<i64>()
                    {
                        kind = Some(k as i32);
                    }
                } else if let Some(value) = entry.strip_prefix(STATUS_PREFIX) {
                    is_error.get_or_insert(value == "ERROR");
                }
            }
            let start_ns = timestamps
                .at(pos as u32)
                .ok_or_else(|| corrupt("a row has no timestamp"))?;
            let duration_ns = *durations
                .0
                .get(pos)
                .ok_or_else(|| corrupt("a row has no duration"))?;
            if pos >= trace_ids.len() || pos >= span_ids.len() || pos >= parents.len() {
                return Err(corrupt("the id columns are shorter than the rows"));
            }
            rows.record_span(
                trace_ids.get(pos),
                span_ids.get(pos),
                parents.get(pos).is_unset(),
                start_ns,
                duration_ns,
                kind.unwrap_or(0),
                is_error.unwrap_or(false),
                service,
                name,
            );
            pos += 1;
        }
    }
    if pos != reader.summary().record_count as usize {
        return Err(corrupt("the stream batches do not cover the rows"));
    }
    let identity: Vec<KvId> = (0..strings.len() as u32).map(KvId).collect();
    Ok(rows.sealed_with_stats(&identity))
}

/// The outcome of [`add_trace_rollup_stats`].
#[derive(Debug)]
pub enum StatsUpgrade {
    /// The rewritten file: every chunk copied, `TRSS` after `TRSU`.
    Upgraded(Vec<u8>),
    /// The file already carries `TRSS`.
    HasStats,
    /// The file has no `TRSU` (pre-rollup, or no set trace id).
    NoRollup,
    /// The derived rollup differs from the stored one: the rows could not
    /// be read back as the seal saw them, so no stats are written.
    RollupMismatch,
}

/// Rewrite a sealed traces file with the `TRSS` stats of its rollup.
pub fn add_trace_rollup_stats(bytes: &[u8]) -> Result<StatsUpgrade, crate::Error> {
    let reader = IndexReader::open(bytes)?;
    if reader.has_trace_rollup_stats() {
        return Ok(StatsUpgrade::HasStats);
    }
    if !reader.has_trace_rollup() {
        return Ok(StatsUpgrade::NoRollup);
    }
    let stored = reader.trace_rollup()?;
    let (derived, stats) = derive_trace_rollup(&reader)?;
    if derived != stored {
        return Ok(StatsUpgrade::RollupMismatch);
    }
    let container = chunk_file::container::Container::open(bytes, crate::MAGIC, crate::VERSION)?;
    let ids: Vec<chunk_file::ChunkId> = container.chunk_ids().collect();
    let mut w = chunk_file::container::StreamingWriter::new(
        std::io::Cursor::new(Vec::new()),
        *crate::MAGIC,
        crate::VERSION,
        ids.len() as u32 + 1,
    )?;
    for id in ids {
        w.write_chunk(id, container.chunk(id)?)?;
        if id == crate::CHUNK_TRACE_ROLLUP {
            let packed = crate::writer::pack(&stats, crate::ZSTD_LEVEL_DEFAULT)?;
            w.write_chunk(crate::CHUNK_TRACE_ROLLUP_STATS, &packed)?;
        }
    }
    Ok(StatsUpgrade::Upgraded(w.finish()?.into_inner()))
}
