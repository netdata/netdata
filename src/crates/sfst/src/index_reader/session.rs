//! The per-file trace query session — the batch primitive of the
//! cross-source engine and the substrate of the single-file
//! [`IndexReader::trace_by_id`].
//!
//! One session per open file serves MANY trace-id lookups while decoding
//! each shared structure exactly once (the phase-4 guardrails):
//!
//! - the TBLM bloom is decoded once and tested per id (a corrupt bloom
//!   degrades to the exact lookup with ONE demoted warning per session,
//!   not one per lookup);
//! - the lookup structures (TIDX + TRCE) decode on the first lookup the
//!   bloom lets through, and the per-row columns (SPAN/PSPN/DURN/FLAG/
//!   DRAC + timestamps) only on the first lookup that finds rows — a
//!   definite bloom miss touches neither, a false positive only the
//!   lookup pair (which a one-id owned session drops again);
//! - an owned session ([`TraceFileSession::open_owned`]) starts from the
//!   file's metadata view and opens the full reader (`PRIM`, for `_kind`
//!   and string resolution) only once a lookup finds rows;
//! - stream batches and EVNB/LNKB decode lazily, once;
//! - KvId→string resolutions accumulate in a per-session cache, each
//!   value cut to [`RESOLVED_VALUE_MAX_BYTES`].
//!
//! The session implements [`SpanSource`], so the shared combiner
//! ([`crate::trace_combine::combine`]) drives it: cheap [`SpanRef`]s up
//! front (no row materialization — `start_ns` from the timestamps column,
//! `kind` from the low-card `_kind` facet bitmaps), full spans only when
//! the merge accepts (or must tie-break) them.
//!
//! Fallibility is per-source, never a panic: a file contradicting its
//! own bounds raises `CorruptIndex` (position ≥ `record_count`, no
//! stream batch for a position, a row past its batch's length, a
//! missing timestamp or column value); the combiner records it in
//! `CombineOutcome::failures` and drops that source's remaining
//! candidates, and `trace_by_id` — one file being the whole result —
//! propagates it. The plan (`trace_plan.rs`) mirrors these re-checks on
//! its side (column lengths against each range). `span_refs` alone
//! raises `UnsetTraceId` — this module holds the variant's only raise
//! site; `trace_by_id` resolves UNSET to the documented empty before
//! any session is opened.

use std::collections::HashMap;

use crate::TraceId;
use crate::index_reader::{
    IndexReader, MetaReader, TraceEvent, TraceLink, TraceSpan, kv_attr, kv_value,
};
use crate::reader::ChunkReader;
use crate::trace_combine::{SpanRef, SpanSource};

/// The bloom gate, resolved once per session.
enum BloomGate {
    /// No bloom chunk: every id falls through to the exact lookup.
    Pass,
    /// The bloom failed to decode (warned once): every id falls through
    /// to the exact lookup, and a caller can see the file is damaged.
    Unreadable,
    Filter(crate::TraceIdBloom),
}

/// The file a session reads through. A borrowed full reader (search
/// shares one per file), or an owned view: the metadata view until a
/// lookup finds the trace, then the full reader, whose `PRIM` decode
/// only a file holding the trace pays for.
enum View<'r, 'a> {
    Borrowed(&'r IndexReader<'a>),
    Meta(Box<MetaReader<'a>>),
    Full(Box<IndexReader<'a>>),
    /// The upgrade to the full reader failed; its error went to the
    /// caller, and every later read fails too.
    Failed,
}

impl<'a> View<'_, 'a> {
    fn chunk(&self) -> Result<&ChunkReader<'a>, crate::Error> {
        match self {
            View::Borrowed(r) => Ok(&r.sfst),
            View::Meta(m) => Ok(&m.sfst),
            View::Full(r) => Ok(&r.sfst),
            View::Failed => Err(upgrade_failed()),
        }
    }

    fn record_count(&self) -> Result<u32, crate::Error> {
        match self {
            View::Borrowed(r) => Ok(r.summary().record_count),
            View::Meta(m) => Ok(m.summary().record_count),
            View::Full(r) => Ok(r.summary().record_count),
            View::Failed => Err(upgrade_failed()),
        }
    }

    /// The full reader, upgrading an owned metadata view on first use.
    fn full(&mut self) -> Result<&IndexReader<'a>, crate::Error> {
        if matches!(self, View::Meta(_)) {
            let View::Meta(meta) = std::mem::replace(self, View::Failed) else {
                unreachable!("matched above");
            };
            *self = View::Full(Box::new((*meta).into_index_reader()?));
        }
        match self {
            View::Borrowed(r) => Ok(r),
            View::Full(r) => Ok(r),
            View::Meta(_) => unreachable!("upgraded above"),
            View::Failed => Err(upgrade_failed()),
        }
    }
}

/// The longest value a session resolves, in bytes: ten times the largest
/// value seen in practice (a ~6 KB stack trace). It bounds what one
/// materialized span, and the session's string cache, can hold under
/// hostile input, before any caller's own cap; a longer value is cut at
/// a character boundary.
const RESOLVED_VALUE_MAX_BYTES: usize = 64 * 1024;

/// Cut the value of a `key=value` string to [`RESOLVED_VALUE_MAX_BYTES`].
fn cap_resolved(kv: &mut String) {
    let value_start = kv.find('=').map_or(0, |i| i + 1);
    let mut end = value_start.saturating_add(RESOLVED_VALUE_MAX_BYTES);
    if kv.len() <= end {
        return;
    }
    while !kv.is_char_boundary(end) {
        end -= 1;
    }
    kv.truncate(end);
}

fn upgrade_failed() -> crate::Error {
    crate::Error::CorruptIndex("trace session: the file failed to open fully".into())
}

/// What finds a trace's rows: decoded on the first lookup the bloom
/// lets through.
struct Lookup {
    index: crate::TraceIdIndex,
    trace_ids: crate::TraceIds,
}

/// The per-row columns a found trace's spans read — decoded together,
/// once, on the first lookup that finds rows.
struct Rows {
    span_ids: crate::SpanIds,
    parents: crate::ParentSpanIds,
    durations: crate::Durations,
    flags: crate::Flags,
    dropped: crate::DroppedAttributeCounts,
    timestamps: crate::Timestamps,
}

/// The optional event/link structures — decoded together, once, on the
/// first materialization (absent chunks mean the file's spans carry no
/// events/links — a correct empty, not a failure).
struct Extras {
    events: Option<crate::EventIndex>,
    links: Option<crate::LinkIndex>,
}

/// See the module docs. Open one per file; look up many ids.
pub struct TraceFileSession<'r, 'a> {
    view: View<'r, 'a>,
    bloom: BloomGate,
    lookup: Option<Lookup>,
    /// Drop the lookup structures after a lookup that finds nothing:
    /// a one-id session (an owned one) never needs them again.
    transient_lookup: bool,
    rows: Option<Rows>,
    extras: Option<Extras>,
    /// Stream-batch cache, pre-sized to the file's batch count on the
    /// first `batch` call; each batch decodes on first touch.
    batches: Vec<Option<crate::StreamBatch>>,
    /// KvId → `key=value` string, accumulated across materializations.
    strings: HashMap<u32, String>,
    /// Row position → raw `_kind` int, filled per looked-up trace from
    /// the low-card facet bitmaps (no row access).
    kinds: HashMap<u32, i32>,
}

impl<'r, 'a> TraceFileSession<'r, 'a> {
    /// Open a session over a full reader the caller keeps (several
    /// lookups share its decodes). Resolves the bloom gate (once;
    /// corrupt → one demoted warning + exact lookups); everything else
    /// decodes lazily.
    pub fn open(reader: &'r IndexReader<'a>) -> Self {
        Self::new(View::Borrowed(reader), false)
    }

    /// Open a session that owns its file's metadata view: the bloom
    /// decides first; a "maybe" decodes only the trace-id index and the
    /// trace column (dropped again when they find nothing); only a file
    /// that holds the trace decodes its row columns and upgrades to the
    /// full reader. A file the bloom rules out costs its summary,
    /// metadata and bloom.
    pub fn open_owned(meta: MetaReader<'a>) -> Self {
        Self::new(View::Meta(Box::new(meta)), true)
    }

    fn new(view: View<'r, 'a>, transient_lookup: bool) -> Self {
        let bloom = match view.chunk() {
            Ok(chunk) if chunk.has_trace_id_bloom() => match chunk.trace_id_bloom() {
                Ok(filter) => BloomGate::Filter(filter),
                Err(e) => {
                    // The bloom is a skip hint, not authoritative: a
                    // corrupt one must not make findable traces
                    // unfindable. One warning per session, however many
                    // ids the fan-out probes.
                    tracing::warn!(
                        "trace_id bloom unreadable; session falls back to exact lookups: {e}"
                    );
                    BloomGate::Unreadable
                }
            },
            _ => BloomGate::Pass,
        };
        Self {
            view,
            bloom,
            lookup: None,
            transient_lookup,
            rows: None,
            extras: None,
            batches: Vec::new(),
            strings: HashMap::new(),
            kinds: HashMap::new(),
        }
    }

    /// Keep the lookup structures after a lookup that finds nothing: a
    /// caller looking up many ids in this file (search) decodes them
    /// once.
    pub fn keep_lookup(mut self) -> Self {
        self.transient_lookup = false;
        self
    }

    /// The file's metadata (decoded once, cached by the reader).
    pub fn metadata(&self) -> Result<&crate::Metadata, crate::Error> {
        self.view.chunk()?.metadata()
    }

    /// The full reader, opened on first use for an owned session (its
    /// `PRIM` decode), for callers that compile plans or resolve values.
    pub fn reader(&mut self) -> Result<&IndexReader<'a>, crate::Error> {
        self.view.full()
    }

    /// Whether the file carries the per-file trace rollup (`TRSU`).
    pub fn has_trace_rollup(&self) -> bool {
        self.view
            .chunk()
            .is_ok_and(|chunk| chunk.has_trace_rollup())
    }

    /// Decode and validate the per-file trace rollup (`TRSU`).
    pub fn trace_rollup(&self) -> Result<crate::TraceRollup, crate::Error> {
        self.view.chunk()?.trace_rollup()
    }

    /// The file's bloom failed to decode: lookups fall back to the exact
    /// path, and the file is damaged.
    pub fn bloom_unreadable(&self) -> bool {
        matches!(self.bloom, BloomGate::Unreadable)
    }

    /// Bloom test: `false` is a definite miss (skip this file without
    /// touching TIDX/TRCE); `true` means "maybe" (or no usable bloom).
    pub fn might_contain(&self, trace_id: TraceId) -> bool {
        match &self.bloom {
            BloomGate::Pass | BloomGate::Unreadable => true,
            BloomGate::Filter(bloom) => bloom.might_contain(trace_id),
        }
    }

    /// The rows holding `trace_id`, decoding the lookup structures on
    /// first use.
    fn positions(&mut self, trace_id: TraceId) -> Result<Vec<u32>, crate::Error> {
        if self.lookup.is_none() {
            let chunk = self.view.chunk()?;
            self.lookup = Some(Lookup {
                index: chunk.trace_id_index()?,
                trace_ids: chunk.trace_ids()?,
            });
        }
        let lookup = self.lookup.as_ref().expect("just populated");
        Ok(lookup.index.positions(trace_id, &lookup.trace_ids).to_vec())
    }

    /// The per-row columns, decoding them on the first lookup that finds
    /// rows.
    fn rows(&mut self) -> Result<&Rows, crate::Error> {
        if self.rows.is_none() {
            let chunk = self.view.chunk()?;
            self.rows = Some(Rows {
                span_ids: chunk.span_ids()?,
                parents: chunk.parent_span_ids()?,
                durations: chunk.durations()?,
                flags: chunk.flags()?,
                dropped: chunk.dropped_attribute_counts()?,
                timestamps: crate::Timestamps::new(chunk.timestamps()?),
            });
        }
        Ok(self.rows.as_ref().expect("just populated"))
    }

    /// The event/link indexes, decoding them on the first
    /// materialization.
    fn extras(&mut self) -> Result<&Extras, crate::Error> {
        if self.extras.is_none() {
            let chunk = self.view.chunk()?;
            self.extras = Some(Extras {
                events: if chunk.has_event_index() {
                    Some(chunk.event_index()?)
                } else {
                    None
                },
                links: if chunk.has_link_index() {
                    Some(chunk.link_index()?)
                } else {
                    None
                },
            });
        }
        Ok(self.extras.as_ref().expect("just populated"))
    }

    /// The decoded stream batch containing `pos`.
    fn batch(&mut self, pos: u32) -> Result<&crate::StreamBatch, crate::Error> {
        let total = self.view.record_count()?;
        if pos >= total {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} >= record_count {total}"
            )));
        }
        if self.batches.is_empty() {
            self.batches = (0..crate::num_stream_batches(total))
                .map(|_| None)
                .collect();
        }
        let b = (pos / crate::stream_batch_size(total)) as usize;
        if self.batches.get(b).is_none() {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} maps to missing stream batch {b}"
            )));
        }
        if self.batches[b].is_none() {
            self.batches[b] = Some(self.view.chunk()?.stream_batch(b as u8)?);
        }
        Ok(self.batches[b].as_ref().expect("just populated"))
    }

    /// Resolve `ids` through the per-session cache, fetching only the
    /// missing ones from the file.
    fn resolve(&mut self, ids: &mut Vec<u32>) -> Result<(), crate::Error> {
        ids.sort_unstable();
        ids.dedup();
        ids.retain(|id| !self.strings.contains_key(id));
        if !ids.is_empty() {
            let mut resolved = self.view.full()?.resolve_kv_strings(ids)?;
            resolved.values_mut().for_each(cap_resolved);
            self.strings.extend(resolved);
        }
        Ok(())
    }
}

impl SpanSource for TraceFileSession<'_, '_> {
    fn span_refs(&mut self, trace_id: TraceId) -> Result<Vec<SpanRef>, crate::Error> {
        // The batch-primitive contract: an UNSET id in a candidate set is
        // a request error (TIDX omits UNSET while a tail scan could serve
        // it — accepting it here would make lookups layout-dependent).
        if trace_id.is_unset() {
            return Err(crate::Error::UnsetTraceId);
        }
        if !self.might_contain(trace_id) {
            return Ok(Vec::new());
        }
        let positions = self.positions(trace_id)?;
        if positions.is_empty() {
            // A bloom false positive: an owned session frees what found
            // nothing.
            if self.transient_lookup {
                self.lookup = None;
            }
            return Ok(Vec::new());
        }

        // `kind` per position from the low-card `_kind` facet bitmaps
        // (materialize_field on a low field never touches stream
        // batches). Absent field / value → 0 (files written before
        // UNSPECIFIED was stored skip it). A corrupt multi-valued row
        // takes the first value — deterministic, and impossible from the
        // production flattener.
        let missing: Vec<u32> = positions
            .iter()
            .copied()
            .filter(|p| !self.kinds.contains_key(p))
            .collect();
        if !missing.is_empty() {
            let values = self.view.full()?.materialize_field("_kind", &missing)?;
            for (p, vals) in missing.iter().zip(values) {
                debug_assert!(
                    vals.len() <= 1,
                    "a row carries multiple _kind values (impossible from the flattener)"
                );
                let kind = vals.first().and_then(|v| v.parse().ok()).unwrap_or(0);
                self.kinds.insert(*p, kind);
            }
        }

        self.rows()?;
        let kinds = &self.kinds;
        let cols = self.rows.as_ref().expect("populated above");
        positions
            .into_iter()
            .map(|pos| {
                let start_ns = cols.timestamps.at(pos).ok_or_else(|| {
                    crate::Error::CorruptIndex(format!(
                        "trace session: position {pos} has no timestamp"
                    ))
                })?;
                Ok(SpanRef {
                    position: pos,
                    start_ns,
                    span_id: cols.span_ids.get(pos as usize),
                    kind: kinds.get(&pos).copied().unwrap_or(0),
                })
            })
            .collect()
    }

    fn materialize(&mut self, pos: u32) -> Result<TraceSpan, crate::Error> {
        // Row tokens first (also validates the position), then the
        // event/link refs — everything this span needs resolves through
        // the session cache in one pass.
        let local = (pos % crate::stream_batch_size(self.view.record_count()?)) as usize;
        let batch = self.batch(pos)?;
        // A CRC-valid but structurally short batch must surface as
        // `CorruptIndex`, never an out-of-bounds panic (the same check
        // the reader's `materialize_rows` applies).
        if local >= batch.num_rows() {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} row {local} >= batch length {}",
                batch.num_rows()
            )));
        }
        let row_ids: Vec<crate::KvId> = batch.row(local).collect();
        let extras = self.extras()?;
        let has_events = extras.events.is_some();
        let has_links = extras.links.is_some();
        let mut needed: Vec<u32> = row_ids.iter().map(|kv| kv.0).collect();
        if let Some(ev) = &extras.events {
            needed.extend(ev.all_refs_for_row(pos).map(|id| id.0));
        }
        if let Some(lk) = &extras.links {
            needed.extend(lk.all_refs_for_row(pos).map(|id| id.0));
        }
        self.resolve(&mut needed)?;

        // Row facets: the `events.`/`links.` tokens exist for search; in
        // the reconstructed span they appear structured instead, so drop
        // the flat duplicates (only when the structure is present).
        let fields: Vec<(String, String)> = row_ids
            .iter()
            .map(|kv| {
                let s = self.strings.get(&kv.0).map(String::as_str).unwrap_or("");
                match s.split_once('=') {
                    Some((k, v)) => (k.to_string(), v.to_string()),
                    None => (s.to_string(), String::new()),
                }
            })
            .filter(|(k, _)| {
                !((has_events && k.starts_with("events."))
                    || (has_links && k.starts_with("links.")))
            })
            .collect();

        let extras = self.extras.as_ref().expect("populated above by extras()");
        let events: Vec<TraceEvent> = match &extras.events {
            Some(ev) => ev
                .events_for_row(pos)
                .map(|e| TraceEvent {
                    time_unix_nano: e.time_unix_nano,
                    name: kv_value(&self.strings, e.name),
                    dropped_attributes_count: e.dropped_attributes_count,
                    attributes: e
                        .attr_refs
                        .iter()
                        .map(|&id| kv_attr(&self.strings, id, "events.attributes."))
                        .collect(),
                })
                .collect(),
            None => Vec::new(),
        };
        let links: Vec<TraceLink> = match &extras.links {
            Some(lk) => lk
                .links_for_row(pos)
                .map(|l| TraceLink {
                    trace_id: l.trace_id,
                    span_id: l.span_id,
                    trace_state: String::from_utf8_lossy(l.trace_state).into_owned(),
                    flags: l.flags,
                    dropped_attributes_count: l.dropped_attributes_count,
                    attributes: l
                        .attr_refs
                        .iter()
                        .map(|&id| kv_attr(&self.strings, id, "links.attributes."))
                        .collect(),
                })
                .collect(),
            None => Vec::new(),
        };
        let dropped_events_count = extras
            .events
            .as_ref()
            .map_or(0, |ev| ev.row_dropped_count(pos));
        let dropped_links_count = extras
            .links
            .as_ref()
            .map_or(0, |lk| lk.row_dropped_count(pos));

        // `kind`: from the facet map when `span_refs` populated it (the
        // combiner always calls span_refs first); the field parse is the
        // defensive fallback for direct callers.
        let kind = self.kinds.get(&pos).copied().unwrap_or_else(|| {
            fields
                .iter()
                .find(|(k, _)| k == "_kind")
                .and_then(|(_, v)| v.parse().ok())
                .unwrap_or(0)
        });

        let cols = self.rows()?;
        let i = pos as usize;
        let bounds = |what: &str| {
            crate::Error::CorruptIndex(format!("trace session: {what} missing at position {pos}"))
        };
        Ok(TraceSpan {
            span_id: cols.span_ids.get(i),
            parent_span_id: cols.parents.get(i),
            start_ns: cols.timestamps.at(pos).ok_or_else(|| bounds("timestamp"))?,
            duration_ns: *cols.durations.0.get(i).ok_or_else(|| bounds("duration"))?,
            kind,
            flags: *cols.flags.0.get(i).ok_or_else(|| bounds("flags"))?,
            dropped_attributes_count: *cols.dropped.0.get(i).ok_or_else(|| bounds("dropped"))?,
            dropped_events_count,
            dropped_links_count,
            fields,
            events,
            links,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolved_values_are_cut_at_a_character_boundary() {
        let mut kv = format!("k={}", "é".repeat(RESOLVED_VALUE_MAX_BYTES));
        cap_resolved(&mut kv);
        // "é" is two bytes: the cut lands on a boundary at the cap.
        assert_eq!(kv.len(), 2 + RESOLVED_VALUE_MAX_BYTES);
        let mut kv = format!("k={}x", "a".repeat(RESOLVED_VALUE_MAX_BYTES - 1));
        let before = kv.clone();
        cap_resolved(&mut kv);
        assert_eq!(kv, before, "at the cap: untouched");
    }
}
