//! Offline discovery of query sources from on-disk WAL/SFST directories.
//!
//! SFST files are sealed and indexed: an `sfst::Registry` reads each file's
//! summary (which carries real timestamps), so candidate selection time-prunes
//! and stream-filters cheaply.
//!
//! WAL files carry no on-disk timestamp index and no durable-prefix marker
//! (recovery sets both to "unknown"), so `wal::Registry::candidates` would drop
//! them all offline. Instead we enumerate every `*.wal` file directly, find its
//! intact byte range with `scan_frame_boundaries` (which stops cleanly at a
//! torn tail), and hand the whole range to the engine as a row-scanned tail —
//! the engine applies the time window and filters during the scan.
//!
//! Dedup mirrors the live planner: a WAL whose sequence is already sealed into
//! an SFST is skipped (SFST wins). Stream filtering mirrors the live planner
//! too: both tiers match by `ns_hash` — the SFST `Query` carries the stream's
//! hash, and the WAL is matched on its `FileId.ns_hash`.
//!
//! The dedup set is built from the *windowed* SFST candidates, so an SFST
//! entirely outside the query window is not added and its WAL twin is scanned
//! as a tail. Results stay correct — the engine's row-scan filters those rows
//! out by time — at the cost of a little extra I/O; a full dedup would need a
//! window-independent SFST scan for no practical gain.

use std::collections::HashSet;
use std::ops::Range;

use anyhow::Result;
use file_registry::{FileDir, Query, ServiceStream};
use sfsq::logs::{LogSource, Part, SfstCandidate, Source, WalTail};

use crate::config::Dirs;

/// The query sources plus a human-readable list of the files consulted (for
/// `--show-files`).
pub struct Discovered {
    pub sources: Vec<LogSource>,
    pub consulted: Vec<String>,
}

/// Build the engine source list for `tenant` over `window` (epoch seconds,
/// half-open), optionally restricted to a single `stream`.
pub fn discover(
    dirs: &Dirs,
    tenant: &str,
    stream: Option<&ServiceStream>,
    window: Range<u32>,
) -> Result<Discovered> {
    let mut sources = Vec::new();
    let mut consulted = Vec::new();

    // --- SFST (sealed, indexed): time-pruned + stream-filtered candidates ---
    let sfst_dir = dirs.sfst.join(tenant);
    // `Registry::recover` swallows directory-scan errors (`unwrap_or_default`),
    // so probe the dir explicitly first: a permission/IO error on the index dir
    // must be visible, exactly like the WAL side below — an empty result must be
    // distinguishable from an unreadable one. `FileDir::scan` maps a missing dir
    // to `Ok(empty)`, so this warns only on a real error.
    if let Err(e) = FileDir::new(&sfst_dir, "sfst").scan() {
        tracing::warn!("cannot scan SFST dir {}: {e}", sfst_dir.display());
    }
    let mut sfst_seqs: HashSet<u64> = HashSet::new();
    let mut registry = sfst::Registry::new(&sfst_dir);
    registry.recover();
    let query = Query {
        time_range: window,
        // Single-stream CLI filter → a one-element hash set (empty = all).
        stream_hashes: stream.map(|s| vec![s.ns_hash()]).unwrap_or_default(),
    };
    for file in registry.candidates(&query) {
        sfst_seqs.insert(file.id.seq);
        let path = registry.file_path(file.id);
        consulted.push(format!("sfst {}", path.display()));
        sources.push(LogSource::Sfst(SfstCandidate {
            summary: file.summary.clone(),
            file_seq: file.id.seq,
            part: Part::Indexed(0),
            source: Source::File(path),
        }));
    }

    // --- WAL (un-indexed tail): enumerate, dedup vs SFST, row-scan range ---
    // Canonical identity hash: empty fields collapse to absent, matching how the
    // ingestor names WAL files (see `ServiceStream::ns_hash`). Using the raw
    // primitive with `Some(&s.namespace)` would hash an absent namespace as
    // `Some("")` and miss every absent-namespace WAL file.
    let stream_hash = stream.map(|s| s.ns_hash());
    let wal_dir = FileDir::new(&dirs.wal.join(tenant), "wal");
    match wal_dir.scan() {
        Ok(mut files) => {
            // Stable, reproducible `--show-files` order (read_dir is unordered);
            // results are unaffected — the engine re-sorts rows by cursor.
            files.sort_by_key(|(id, _)| id.seq);
            for (id, meta) in files {
                // SFST wins over WAL for the same sequence (already sealed).
                if sfst_seqs.contains(&id.seq) {
                    continue;
                }
                // Stream filter: a WAL file is single-stream by construction,
                // identified by the ns_hash in its FileId.
                if let Some(hash) = stream_hash {
                    if id.ns_hash != hash {
                        continue;
                    }
                }
                let path = wal_dir.file_path(id);
                // A valid WAL is at least a full header; a shorter file is a
                // truncated/corrupt artifact. Guard before FrameRange::new,
                // whose debug_assert(start <= end) would otherwise panic in
                // debug builds.
                if meta.len() < wal::HEADER_SIZE as u64 {
                    tracing::warn!(
                        "skipping WAL {}: {} bytes is shorter than the header ({})",
                        path.display(),
                        meta.len(),
                        wal::HEADER_SIZE
                    );
                    continue;
                }
                // meta.len() is the offline analogue of the agent's recorded
                // valid_up_to: scan_frame_boundaries clamps to the last intact
                // frame, so a torn/partial tail is dropped. Correct for sealed
                // files; against a live agent's active WAL, a mid-write read is
                // degraded by the engine to that source's empty result.
                let full = wal::FrameRange::new(wal::HEADER_SIZE as u64, meta.len());
                let boundaries = match wal::scan_frame_boundaries(&path, full) {
                    Ok(b) => b,
                    Err(e) => {
                        tracing::warn!("skipping WAL {}: {e}", path.display());
                        continue;
                    }
                };
                // The last intact frame's end is the durable extent to scan;
                // an empty list means no complete frame (torn/empty file). Warn
                // rather than skip silently — for a forensic tool, "header-only
                // or torn" must be distinguishable from "no data".
                let Some(last) = boundaries.last() else {
                    tracing::warn!(
                        "skipping WAL {}: no complete frames ({} bytes; header-only or torn)",
                        path.display(),
                        meta.len()
                    );
                    continue;
                };
                consulted.push(format!("wal  {}", path.display()));
                sources.push(LogSource::Tail(WalTail {
                    file_seq: id.seq,
                    path,
                    range: wal::FrameRange::new(wal::HEADER_SIZE as u64, last.end_offset),
                }));
            }
        }
        Err(e) => {
            // FileDir::scan maps a missing dir to Ok(empty), so this arm fires
            // only for real I/O errors (e.g. permission denied). Non-fatal —
            // matching SFSTs may still have been found.
            tracing::warn!(
                "cannot scan WAL dir {}: {e}",
                dirs.wal.join(tenant).display()
            );
        }
    }

    Ok(Discovered { sources, consulted })
}
