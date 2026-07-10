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
//! - TIDX and the per-row columns (TRCE/SPAN/PSPN/DURN/FLAG/DRAC +
//!   timestamps) decode once, on the first non-miss lookup — a
//!   definite bloom miss never touches them;
//! - stream batches and EVNB/LNKB decode lazily, once;
//! - KvId→string resolutions accumulate in a per-session cache.
//!
//! The session implements [`SpanSource`], so the shared combiner
//! ([`crate::trace_combine::combine`]) drives it: cheap [`SpanRef`]s up
//! front (no row materialization — `start_ns` from the timestamps column,
//! `kind` from the low-card `_kind` facet bitmaps), full spans only when
//! the merge accepts (or must tie-break) them.

use std::collections::HashMap;

use crate::index_reader::{IndexReader, TraceEvent, TraceLink, TraceSpan, kv_attr, kv_value};
use crate::trace_combine::{SpanRef, SpanSource};
use crate::TraceId;

/// The bloom gate, resolved once per session.
enum BloomGate {
    /// No bloom chunk, or it failed to decode (warned once): every id
    /// falls through to the exact lookup.
    Pass,
    Filter(crate::TraceIdBloom),
}

/// The columns a trace lookup reads — decoded together, once, on the
/// first non-miss lookup.
struct Columns {
    index: crate::TraceIdIndex,
    trace_ids: crate::TraceIds,
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
    reader: &'r IndexReader<'a>,
    bloom: BloomGate,
    columns: Option<Columns>,
    extras: Option<Extras>,
    /// Stream-batch cache, sized on first materialization.
    batches: Vec<Option<crate::StreamBatch>>,
    /// KvId → `key=value` string, accumulated across materializations.
    strings: HashMap<u32, String>,
    /// Row position → raw `_kind` int, filled per looked-up trace from
    /// the low-card facet bitmaps (no row access).
    kinds: HashMap<u32, i32>,
}

impl<'r, 'a> TraceFileSession<'r, 'a> {
    /// Open a session: resolves the bloom gate (once; corrupt → one
    /// demoted warning + exact lookups); everything else decodes lazily.
    pub fn open(reader: &'r IndexReader<'a>) -> Self {
        let bloom = if reader.has_trace_id_bloom() {
            match reader.trace_id_bloom() {
                Ok(filter) => BloomGate::Filter(filter),
                Err(e) => {
                    // The bloom is a skip hint, not authoritative: a
                    // corrupt one must not make findable traces
                    // unfindable. One warning per session, however many
                    // ids the fan-out probes.
                    tracing::warn!(
                        "trace_id bloom unreadable; session falls back to exact lookups: {e}"
                    );
                    BloomGate::Pass
                }
            }
        } else {
            BloomGate::Pass
        };
        Self {
            reader,
            bloom,
            columns: None,
            extras: None,
            batches: Vec::new(),
            strings: HashMap::new(),
            kinds: HashMap::new(),
        }
    }

    /// Bloom test: `false` is a definite miss (skip this file without
    /// touching TIDX/TRCE); `true` means "maybe" (or no usable bloom).
    pub fn might_contain(&self, trace_id: TraceId) -> bool {
        match &self.bloom {
            BloomGate::Pass => true,
            BloomGate::Filter(bloom) => bloom.might_contain(trace_id),
        }
    }

    fn columns(&mut self) -> Result<&Columns, crate::Error> {
        if self.columns.is_none() {
            let reader = self.reader;
            self.columns = Some(Columns {
                index: reader.trace_id_index()?,
                trace_ids: reader.trace_ids()?,
                span_ids: reader.span_ids()?,
                parents: reader.parent_span_ids()?,
                durations: reader.durations()?,
                flags: reader.flags()?,
                dropped: reader.dropped_attribute_counts()?,
                timestamps: reader.load_timestamps()?,
            });
        }
        Ok(self.columns.as_ref().expect("just populated"))
    }

    fn extras(&mut self) -> Result<&Extras, crate::Error> {
        if self.extras.is_none() {
            let reader = self.reader;
            self.extras = Some(Extras {
                events: if reader.has_event_index() {
                    Some(reader.event_index()?)
                } else {
                    None
                },
                links: if reader.has_link_index() {
                    Some(reader.link_index()?)
                } else {
                    None
                },
            });
        }
        Ok(self.extras.as_ref().expect("just populated"))
    }

    /// The decoded stream batch containing `pos`.
    fn batch(&mut self, pos: u32) -> Result<&crate::StreamBatch, crate::Error> {
        let total = self.reader.summary().record_count;
        if pos >= total {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} >= record_count {total}"
            )));
        }
        if self.batches.is_empty() {
            self.batches = (0..self.reader.num_stream_batches()).map(|_| None).collect();
        }
        let b = (pos / crate::stream_batch_size(total)) as usize;
        if self.batches.get(b).is_none() {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} maps to missing stream batch {b}"
            )));
        }
        if self.batches[b].is_none() {
            self.batches[b] = Some(self.reader.sfst.stream_batch(b as u8)?);
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
            self.strings.extend(self.reader.resolve_kv_strings(ids)?);
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
        let cols = self.columns()?;
        let positions: Vec<u32> = cols.index.positions(trace_id, &cols.trace_ids).to_vec();
        if positions.is_empty() {
            return Ok(Vec::new());
        }

        // `kind` per position from the low-card `_kind` facet bitmaps
        // (materialize_field on a low field never touches stream
        // batches). Absent field / value → 0 (UNSPECIFIED is skipped at
        // flatten). A corrupt multi-valued row takes the first value —
        // deterministic, and impossible from the production flattener.
        let missing: Vec<u32> = positions
            .iter()
            .copied()
            .filter(|p| !self.kinds.contains_key(p))
            .collect();
        if !missing.is_empty() {
            let values = self.reader.materialize_field("_kind", &missing)?;
            for (p, vals) in missing.iter().zip(values) {
                debug_assert!(
                    vals.len() <= 1,
                    "a row carries multiple _kind values (impossible from the flattener)"
                );
                let kind = vals.first().and_then(|v| v.parse().ok()).unwrap_or(0);
                self.kinds.insert(*p, kind);
            }
        }

        let kinds = &self.kinds;
        let cols = self.columns.as_ref().expect("populated above");
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
        let local = (pos % crate::stream_batch_size(self.reader.summary().record_count)) as usize;
        let batch = self.batch(pos)?;
        // A CRC-valid but structurally short batch must be a SourceFailure,
        // not an out-of-bounds panic (the same check the reader's
        // materialize_rows applies).
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
        let dropped_events_count = extras.events.as_ref().map_or(0, |ev| ev.row_dropped_count(pos));
        let dropped_links_count = extras.links.as_ref().map_or(0, |lk| lk.row_dropped_count(pos));

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

        let cols = self.columns()?;
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
