//! Trace query sources and their validated identity.
//!
//! The library never inspects `part_key` or any other content of a
//! source's identity (part_key is opaque — the caller enumerates
//! ALL of a tenant's trace files, whatever their stream). What it DOES
//! validate, on every operation, is set hygiene:
//!
//! - exact [`SourceId`] duplicates are rejected — a duplicated source
//!   would double UNSET-span-id spans, which deliberately never
//!   deduplicate;
//! - WAL-derived sources (in-memory chunk SFSTs and tails) carry
//!   [`WalCoverage`], and byte ranges of the same WAL must not intersect
//!   (half-open; adjacent is fine) — overlapping coverage double-counts
//!   frames.
//!
//! What it CANNOT validate — documented, caller-owned: a sealed file and
//! an active WAL holding the same data (the seal-vs-rotation window) must
//! not both be offered; the caller's registry snapshot owns that
//! invariant, exactly as it does for the logs engine.

use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::Arc;

use crate::source::Source;

/// Caller-supplied opaque source identity: equality and diagnostics only.
///
/// Uniqueness is the caller's contract; the documented production
/// derivation is the full `FileId` (machine, instance, pipeline,
/// part_key, seq) plus the chunk index for in-memory chunks and the byte
/// range for tails — a bare `seq` is unique only within one process
/// instance and is NOT sufficient.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct SourceId(Arc<str>);

impl SourceId {
    pub fn new(id: impl Into<Arc<str>>) -> Self {
        Self(id.into())
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl std::fmt::Display for SourceId {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}

/// Which WAL bytes a WAL-derived source covers — the structure an opaque
/// [`SourceId`] cannot carry, needed to validate that two sources never
/// serve the same frames.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WalCoverage {
    /// The WAL's identity, opaque to the library (same derivation rules
    /// as [`SourceId`]).
    pub wal_id: Arc<str>,
    /// The half-open byte range of the WAL this source was built from /
    /// scans.
    pub range: wal::FrameRange,
}

/// A sealed or in-memory SFST source for trace queries.
#[derive(Clone)]
pub struct TraceSfstCandidate {
    pub source_id: SourceId,
    /// Cheap time/stream/size facts ([`sfst::Summary`]). Trace-by-id does
    /// not consume it (TBLM prunes better than time ranges for a by-id
    /// probe); it is part of the candidate shape for the search phase's
    /// window pruning (4c) and for parity with the logs candidates.
    pub summary: sfst::Summary,
    /// Where the bytes come from ([`Source::File`] sealed on disk,
    /// [`Source::Memory`] an in-memory chunk image).
    pub source: Source,
    /// Present exactly when the SFST is an in-memory chunk of an active
    /// WAL (`build_sfst_traces_range`); a sealed file has no WAL
    /// coverage. ENFORCED by [`validate_sources`]: a
    /// [`Source::Memory`]-backed candidate without coverage is rejected —
    /// omitting it would silently bypass the overlap protection.
    pub coverage: Option<WalCoverage>,
}

/// An active WAL's un-indexed tail, evaluated by a trace-frame row scan.
///
/// The scanned byte range IS `coverage.range` — one field, one truth: a
/// separate scan range could silently diverge from the validated
/// coverage and bypass the overlap protection.
#[derive(Clone)]
pub struct TraceWalTail {
    pub source_id: SourceId,
    pub path: PathBuf,
    /// The tail's WAL coverage: the same `wal_id` as the WAL's chunk
    /// sources (so chunk/tail overlap is caught) and the half-open,
    /// frame-aligned byte range the scan reads.
    pub coverage: WalCoverage,
}

/// A source of trace data: an SFST (sealed file or in-memory chunk)
/// evaluated through the indexed reader, or a WAL tail row scan.
/// Cloning is cheap — sources are descriptors (paths, ids, coverage),
/// never data.
#[derive(Clone)]
pub enum TraceSource {
    Sfst(TraceSfstCandidate),
    Tail(TraceWalTail),
}

impl TraceSource {
    pub fn source_id(&self) -> &SourceId {
        match self {
            TraceSource::Sfst(c) => &c.source_id,
            TraceSource::Tail(t) => &t.source_id,
        }
    }

    fn coverage(&self) -> Option<&WalCoverage> {
        match self {
            TraceSource::Sfst(c) => c.coverage.as_ref(),
            TraceSource::Tail(t) => Some(&t.coverage),
        }
    }
}

/// A source-set hygiene violation — a request error (the caller built a
/// bad set), not a per-source degrade.
#[derive(Debug, thiserror::Error)]
pub enum SourceSetError {
    #[error("duplicate source id {0}")]
    DuplicateSource(SourceId),
    #[error(
        "in-memory chunk {0} carries no WAL coverage — the overlap \
         protection cannot see it; supply the chunk's WalCoverage"
    )]
    MemoryChunkWithoutCoverage(SourceId),
    #[error(
        "overlapping coverage of WAL {wal_id}: [{a_start}, {a_end}) and [{b_start}, {b_end}) \
         intersect — two sources would serve the same frames"
    )]
    OverlappingCoverage {
        wal_id: Arc<str>,
        a_start: u64,
        a_end: u64,
        b_start: u64,
        b_end: u64,
    },
}

/// Validate a source set's hygiene: no duplicate [`SourceId`]s, no
/// intersecting [`WalCoverage`] ranges within one WAL (half-open;
/// adjacent ranges are fine). Run by every operation before any I/O.
pub fn validate_sources(sources: &[TraceSource]) -> Result<(), SourceSetError> {
    let mut seen: HashSet<&SourceId> = HashSet::with_capacity(sources.len());
    for source in sources {
        if !seen.insert(source.source_id()) {
            return Err(SourceSetError::DuplicateSource(source.source_id().clone()));
        }
        // An in-memory chunk always derives from a WAL range; without its
        // coverage the overlap check below cannot see it, and the same
        // frames could be served twice (doubling UNSET-id spans).
        if let TraceSource::Sfst(c) = source
            && matches!(c.source, Source::Memory(_))
            && c.coverage.is_none()
        {
            return Err(SourceSetError::MemoryChunkWithoutCoverage(
                c.source_id.clone(),
            ));
        }
    }

    // Coverage overlap: sort per WAL by range start, then adjacent-pair
    // check. Half-open ranges [a, b) and [c, d) with a <= c intersect iff
    // c < b.
    let mut covered: Vec<(&Arc<str>, u64, u64)> = sources
        .iter()
        .filter_map(|s| s.coverage())
        .map(|c| (&c.wal_id, c.range.start(), c.range.end()))
        .collect();
    covered.sort_by(|a, b| (a.0, a.1).cmp(&(b.0, b.1)));
    for pair in covered.windows(2) {
        let (a_wal, a_start, a_end) = pair[0];
        let (b_wal, b_start, b_end) = pair[1];
        if a_wal == b_wal && b_start < a_end {
            return Err(SourceSetError::OverlappingCoverage {
                wal_id: Arc::clone(a_wal),
                a_start,
                a_end,
                b_start,
                b_end,
            });
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Coverage-less = a sealed FILE source (a coverage-less Memory
    /// source is invalid by rule — see the dedicated test); with coverage
    /// = an in-memory chunk.
    fn sfst(id: &str, coverage: Option<(&str, u64, u64)>) -> TraceSource {
        let source = if coverage.is_some() {
            Source::Memory(Arc::new(Vec::new()))
        } else {
            Source::File(PathBuf::from("/dev/null"))
        };
        TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(id),
            summary: sfst::Summary {
                min_timestamp_s: 0,
                max_timestamp_s: 0,
                record_count: 0,
                content_meta: Vec::new(),
            },
            source,
            coverage: coverage.map(|(wal, s, e)| WalCoverage {
                wal_id: wal.into(),
                range: wal::FrameRange::new(s, e),
            }),
        })
    }

    /// The invalid shape the rule exists for: Memory bytes, no coverage.
    fn memory_without_coverage(id: &str) -> TraceSource {
        TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(id),
            summary: sfst::Summary {
                min_timestamp_s: 0,
                max_timestamp_s: 0,
                record_count: 0,
                content_meta: Vec::new(),
            },
            source: Source::Memory(Arc::new(Vec::new())),
            coverage: None,
        })
    }

    fn tail(id: &str, wal: &str, start: u64, end: u64) -> TraceSource {
        TraceSource::Tail(TraceWalTail {
            source_id: SourceId::new(id),
            path: PathBuf::from("/dev/null"),
            coverage: WalCoverage {
                wal_id: wal.into(),
                range: wal::FrameRange::new(start, end),
            },
        })
    }

    #[test]
    fn memory_chunk_without_coverage_rejected() {
        let set = [memory_without_coverage("chunk")];
        assert!(matches!(
            validate_sources(&set),
            Err(SourceSetError::MemoryChunkWithoutCoverage(id)) if id.as_str() == "chunk"
        ));
    }

    #[test]
    fn duplicate_source_ids_rejected() {
        let set = [sfst("a", None), sfst("a", None)];
        assert!(matches!(
            validate_sources(&set),
            Err(SourceSetError::DuplicateSource(id)) if id.as_str() == "a"
        ));
    }

    #[test]
    fn overlapping_ranges_of_one_wal_rejected() {
        // Chunk [0, 100) and tail [90, 200) of the same WAL intersect.
        let set = [sfst("chunk", Some(("wal-1", 0, 100))), tail("tail", "wal-1", 90, 200)];
        assert!(matches!(
            validate_sources(&set),
            Err(SourceSetError::OverlappingCoverage { .. })
        ));
    }

    #[test]
    fn adjacent_ranges_and_distinct_wals_accepted() {
        // Adjacent [0, 100) + [100, 200) on one WAL; identical range on a
        // DIFFERENT WAL; a sealed file with no coverage at all.
        let set = [
            sfst("chunk-0", Some(("wal-1", 0, 100))),
            tail("tail-1", "wal-1", 100, 200),
            sfst("chunk-other", Some(("wal-2", 0, 100))),
            sfst("sealed", None),
        ];
        assert!(validate_sources(&set).is_ok());
    }

    #[test]
    fn sealed_files_have_no_coverage_to_collide() {
        // Two sealed files never coverage-collide (distinct ids suffice).
        let set = [sfst("sealed-1", None), sfst("sealed-2", None)];
        assert!(validate_sources(&set).is_ok());
    }
}
