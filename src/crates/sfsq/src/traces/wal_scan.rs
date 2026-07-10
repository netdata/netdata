//! Query-time span scan over a **traces** WAL range — the tail evaluator.
//!
//! [`TraceWalScan`] decodes a WAL's trace frames
//! (`ng_flatten::FlattenedTraceRequest`, payload format 3) into spans once
//! and serves them to the shared combiner as a
//! [`SpanSource`](sfst::SpanSource). It exists for data the indexer hasn't
//! reached yet — the bounded, un-indexed *tail* of an active WAL — so a
//! whole-range decode per query is affordable (the same rationale as the
//! logs [`WalScan`](crate::logs::WalScan); dictionaries don't exist here,
//! frame decoding IS the access path).
//!
//! # Semantic equality with the sealed path
//!
//! A span served from the tail must equal the same span served from an
//! SFST sealed over the same frames (the split-many-ways acceptance
//! criterion). The shared guarantees:
//!
//! - fields render through the same [`ng_flatten::build_kv`] in the same
//!   order (resource ++ scope ++ span entries); the sealed path's flat
//!   `events.`/`links.` tokens are filtered out of its fields when the
//!   structure exists, and here the event/link entries are never mixed
//!   into fields at all — same net result;
//! - events/links come from the frame's structured records, exactly what
//!   the seal stores in `EVNB`/`LNKB` (attribute keys prefix-stripped the
//!   same way);
//! - `kind` parses from the same `_kind` entry; ids convert by the same
//!   byte copy the seal uses; `ts`/`duration` are the same frame scalars.
//!
//! Scanning is all-or-nothing at the DECODE boundary — a torn frame, a
//! CRC mismatch, or a bincode-invalid payload fails the scan (the engine
//! reports the source failed; never a silent truncation). Structural
//! validation of successfully DECODED frames follows the platform-wide
//! trust model shared with the logs scanner and both seal paths: WAL
//! frames are CRC-protected and written by this same binary's ingest, so
//! decoded trees/node-ids are trusted; hardening against CRC-valid
//! crafted frames is part of the deferred DoS-hardening follow-up
//! (user ruling, phase 3 — tracked in the plan repo).

use std::path::Path;

use sfst::{SpanRef, SpanSource, TraceEvent, TraceLink, TraceSpan};

/// A failure decoding a traces WAL range.
#[derive(Debug, thiserror::Error)]
pub enum TraceScanError {
    #[error("wal read: {0}")]
    Wal(#[from] wal::Error),
    /// The WAL header names a frame codec other than the flattened-traces
    /// format — refuse before decoding any frame.
    #[error(
        "WAL payload format {found} is not the expected {expected}; \
         refusing to decode frames written by a different codec"
    )]
    PayloadFormat { found: u16, expected: u16 },
    #[error("frame {frame}: trace-frame decode failed: {msg}")]
    Decode { frame: u64, msg: String },
    /// The range holds more spans than `SpanRef.position: u32` can index
    /// — unreachable for a real tail (bounded by rotation), guarded so a
    /// mis-ranged call cannot silently alias positions.
    #[error("range holds {0} spans, more than a scan can index")]
    TooManySpans(usize),
}

/// One decoded span, ready to serve: the sealed-equivalent [`TraceSpan`]
/// plus the lookup/merge keys.
struct DecodedSpan {
    trace_id: sfst::TraceId,
    span: TraceSpan,
}

/// The decoded spans of one WAL range. Build once with
/// [`scan_range`](Self::scan_range); serve any number of trace lookups.
pub struct TraceWalScan {
    spans: Vec<DecodedSpan>,
    /// Field → coalesced scalar kind over the scanned frames, derived
    /// through the SAME converter/lattice as the sealed path
    /// (`ng_index::to_sfst_tree` + `derive_scalar_kinds`), so tail and
    /// sealed kinds agree by construction.
    field_kinds: Vec<(String, sfst::ValueKind)>,
}

impl TraceWalScan {
    /// Decode every trace frame in `range` (frame boundaries; half-open —
    /// see [`wal::FrameRange`]). An empty range yields a zero-span scan.
    pub fn scan_range(path: &Path, range: wal::FrameRange) -> Result<TraceWalScan, TraceScanError> {
        Self::drain(wal::Reader::open_range(path, range)?)
    }

    /// Whole-file counterpart of [`scan_range`](Self::scan_range) (tests,
    /// tooling).
    pub fn scan(path: &Path) -> Result<TraceWalScan, TraceScanError> {
        Self::drain(wal::Reader::open(path)?)
    }

    fn drain(mut reader: wal::Reader) -> Result<TraceWalScan, TraceScanError> {
        let found = reader.header().payload_format;
        if found != ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT {
            return Err(TraceScanError::PayloadFormat {
                found,
                expected: ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
            });
        }
        let mut spans: Vec<DecodedSpan> = Vec::new();
        let mut kv = String::new();
        let mut frame_no = 0u64;
        // Accumulates the frames' typed schema trees — the source of the
        // field-kind map, exactly as the seal accumulates its on-disk tree.
        let mut flattener = ng_flatten::Flattener::new();
        while let Some(frame) = reader.next_frame()? {
            frame_no += 1;
            let flattened = ng_flatten::decode_trace_frame(frame.data).map_err(|e| {
                TraceScanError::Decode {
                    frame: frame_no,
                    msg: e.to_string(),
                }
            })?;
            let tree = &flattened.tree;
            let _ = flattener.merge_tree(tree);
            // Resolve each node's path once per frame (spans reuse nodes).
            let paths: Vec<String> = (0..tree.len() as ng_flatten::NodeId)
                .map(|id| tree.path(id))
                .collect();
            let render = |entries: &[ng_flatten::Entry], kv: &mut String| -> Vec<(String, String)> {
                entries
                    .iter()
                    .map(|e| {
                        ng_flatten::build_kv(&paths[e.node as usize], &e.value, kv);
                        match kv.split_once('=') {
                            Some((k, v)) => (k.to_string(), v.to_string()),
                            None => (kv.clone(), String::new()),
                        }
                    })
                    .collect()
            };
            // An attribute entry rendered with its scope prefix stripped —
            // the tail counterpart of the sealed path's `kv_attr`.
            let render_attr =
                |e: &ng_flatten::Entry, prefix: &str, kv: &mut String| -> (String, String) {
                    ng_flatten::build_kv(&paths[e.node as usize], &e.value, kv);
                    match kv.split_once('=') {
                        Some((k, v)) => (
                            k.strip_prefix(prefix).unwrap_or(k).to_string(),
                            v.to_string(),
                        ),
                        None => (kv.clone(), String::new()),
                    }
                };

            for rg in &flattened.resources {
                let resource_fields = render(&rg.resource, &mut kv);
                for sg in &rg.scopes {
                    let scope_fields = render(&sg.scope, &mut kv);
                    for record in &sg.spans {
                        let mut fields = Vec::with_capacity(
                            resource_fields.len() + scope_fields.len() + record.entries.len(),
                        );
                        fields.extend_from_slice(&resource_fields);
                        fields.extend_from_slice(&scope_fields);
                        fields.extend(render(&record.entries, &mut kv));
                        let kind = fields
                            .iter()
                            .find(|(k, _)| k == "_kind")
                            .and_then(|(_, v)| v.parse().ok())
                            .unwrap_or(0);
                        let events: Vec<TraceEvent> = record
                            .events
                            .iter()
                            .map(|ev| TraceEvent {
                                time_unix_nano: ev.time_unix_nano,
                                name: {
                                    ng_flatten::build_kv(
                                        &paths[ev.name.node as usize],
                                        &ev.name.value,
                                        &mut kv,
                                    );
                                    kv.split_once('=')
                                        .map(|(_, v)| v.to_string())
                                        .unwrap_or_default()
                                },
                                dropped_attributes_count: ev.dropped_attributes_count,
                                attributes: ev
                                    .attributes
                                    .iter()
                                    .map(|e| render_attr(e, "events.attributes.", &mut kv))
                                    .collect(),
                            })
                            .collect();
                        let links: Vec<TraceLink> = record
                            .links
                            .iter()
                            .map(|l| TraceLink {
                                trace_id: sfst::TraceId::from(*l.trace_id.as_bytes()),
                                span_id: sfst::SpanId::from(*l.span_id.as_bytes()),
                                trace_state: l.trace_state.clone(),
                                flags: l.flags,
                                dropped_attributes_count: l.dropped_attributes_count,
                                attributes: l
                                    .attributes
                                    .iter()
                                    .map(|e| render_attr(e, "links.attributes.", &mut kv))
                                    .collect(),
                            })
                            .collect();
                        spans.push(DecodedSpan {
                            trace_id: sfst::TraceId::from(*record.trace_id.as_bytes()),
                            span: TraceSpan {
                                span_id: sfst::SpanId::from(*record.span_id.as_bytes()),
                                parent_span_id: sfst::SpanId::from(
                                    *record.parent_span_id.as_bytes(),
                                ),
                                start_ns: record.ts,
                                duration_ns: record.duration,
                                kind,
                                flags: record.flags,
                                dropped_attributes_count: record.dropped_attributes_count,
                                dropped_events_count: record.dropped_events_count,
                                dropped_links_count: record.dropped_links_count,
                                fields,
                                events,
                                links,
                            },
                        });
                    }
                }
            }
        }
        if spans.len() > u32::MAX as usize {
            return Err(TraceScanError::TooManySpans(spans.len()));
        }
        let field_kinds = ng_index::to_sfst_tree(&flattener.into_tree()).derive_scalar_kinds();
        Ok(TraceWalScan { spans, field_kinds })
    }

    /// Number of decoded spans in the range.
    pub fn num_spans(&self) -> usize {
        self.spans.len()
    }

    /// Field → coalesced scalar kind over the scanned frames (see the
    /// struct field's doc).
    pub fn field_kinds(&self) -> &[(String, sfst::ValueKind)] {
        &self.field_kinds
    }
}

impl SpanSource for TraceWalScan {
    fn span_refs(&mut self, trace_id: sfst::TraceId) -> Result<Vec<SpanRef>, sfst::Error> {
        Ok(self
            .spans
            .iter()
            .enumerate()
            .filter(|(_, d)| d.trace_id == trace_id)
            .map(|(i, d)| SpanRef {
                position: i as u32,
                start_ns: d.span.start_ns,
                span_id: d.span.span_id,
                kind: d.span.kind,
            })
            .collect())
    }

    fn materialize(&mut self, position: u32) -> Result<TraceSpan, sfst::Error> {
        self.spans
            .get(position as usize)
            .map(|d| d.span.clone())
            .ok_or_else(|| {
                sfst::Error::CorruptIndex(format!(
                    "trace tail scan: position {position} out of range"
                ))
            })
    }
}
