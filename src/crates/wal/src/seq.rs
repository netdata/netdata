//! Durable WAL sequence allocator.
//!
//! `seq` is one component of the durable global identity
//! `FileId { machine_id, invocation_id, pipeline_id, seq, part_key }`, embedded in every
//! local filename, every remote object key, and every catalog entry.
//! A seq value, once issued, must never be reissued while any artifact
//! bearing its FileId still exists — locally or in remote storage
//! (which is never garbage-collected).
//!
//! Scanning surviving local files at boot is not a safe upper bound:
//! age-based eviction is keyed on each file's data timestamp, not its
//! seq, so a high-seq file holding old data can be evicted while
//! lower-seq files survive. [`SeqAllocator`] therefore persists a
//! monotonic **high-water mark** — the highest seq ever *reserved* (a
//! ceiling) — and never issues a seq above it without first durably
//! raising it.
//!
//! Reservations are batched: raising the ceiling by
//! [`DEFAULT_RESERVE_BATCH`] costs one durable write per batch, and
//! seqs are then handed out from memory. A crash forfeits at most one
//! batch of unissued seqs — gaps are harmless (`FileId`s are a sparse
//! set everywhere they are keyed).
//!
//! On-disk envelope (17 bytes):
//!
//! ```text
//! [ magic: 4 bytes "NSEQ" ]
//! [ version: u8 = 1       ]
//! [ reserved: u64 LE      ]
//! [ crc32: u32 LE  (over the preceding 13 bytes) ]
//! ```
//!
//! The file is written via `tmp → fsync(file) → rename → fsync(dir)`,
//! so a reader observes the complete previous-or-new value, never a
//! torn one. Reading a missing or invalid file yields `None` — the
//! caller falls back to the filesystem scans — and MUST NOT fail boot:
//! a too-low seed only degrades to the scan-only behavior, while
//! `max(scan, high-water)` keeps the file from ever making things
//! worse.

use std::path::{Path, PathBuf};
use std::sync::Mutex;

use crate::Result;

const MAGIC: &[u8; 4] = b"NSEQ";
const VERSION: u8 = 1;
const ENVELOPE_LEN: usize = 4 + 1 + 8 + 4;

/// Default number of seqs reserved per durable ceiling raise. Seqs are
/// allocated per WAL file creation (low frequency), so this makes the
/// fsync cost negligible while keeping the post-crash gap small.
pub const DEFAULT_RESERVE_BATCH: u64 = 256;

/// Read the persisted high-water mark from `path`.
///
/// Returns `None` — never an error — when the file is missing or fails
/// any validation (short read, bad magic, bad version, CRC mismatch).
/// Validation failures are logged loudly; the caller treats `None` as
/// `0` and seeds from its filesystem scans instead. The next
/// reservation rewrites a valid file.
pub fn read_seq_highwater(path: &Path) -> Option<u64> {
    let bytes = match std::fs::read(path) {
        Ok(b) => b,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return None,
        Err(e) => {
            tracing::warn!(
                path = %path.display(),
                "failed to read seq high-water file (falling back to scan): {e}"
            );
            return None;
        }
    };
    match parse_envelope(&bytes) {
        Ok(v) => Some(v),
        Err(reason) => {
            tracing::warn!(
                path = %path.display(),
                "seq high-water file invalid ({reason}); falling back to scan — \
                 seq safety degrades to scan-only until the next reservation rewrites it"
            );
            None
        }
    }
}

fn parse_envelope(bytes: &[u8]) -> std::result::Result<u64, String> {
    if bytes.len() != ENVELOPE_LEN {
        return Err(format!("length {} (expected {ENVELOPE_LEN})", bytes.len()));
    }
    if &bytes[0..4] != MAGIC {
        return Err("bad magic".to_string());
    }
    if bytes[4] != VERSION {
        return Err(format!("unsupported version {}", bytes[4]));
    }
    let stored_crc = u32::from_le_bytes(bytes[13..17].try_into().unwrap());
    let actual_crc = crc32fast::hash(&bytes[0..13]);
    if stored_crc != actual_crc {
        return Err(format!(
            "CRC mismatch (stored {stored_crc:#010x}, computed {actual_crc:#010x})"
        ));
    }
    Ok(u64::from_le_bytes(bytes[5..13].try_into().unwrap()))
}

fn encode_envelope(reserved: u64) -> [u8; ENVELOPE_LEN] {
    let mut buf = [0u8; ENVELOPE_LEN];
    buf[0..4].copy_from_slice(MAGIC);
    buf[4] = VERSION;
    buf[5..13].copy_from_slice(&reserved.to_le_bytes());
    let crc = crc32fast::hash(&buf[0..13]);
    buf[13..17].copy_from_slice(&crc.to_le_bytes());
    buf
}

/// Durably persist `reserved` to `path` via the shared atomic-write
/// sequence (tmp → fsync(file) → rename → fsync(parent dir)). The
/// parent-dir fsync is non-negotiable here — a high-water file lost on
/// power loss would reintroduce the seq-reuse regression it exists to
/// prevent.
///
/// Public counterpart of [`read_seq_highwater`] for tooling and tests;
/// normal allocation goes through [`SeqAllocator`].
pub fn write_seq_highwater(path: &Path, reserved: u64) -> Result<()> {
    file_registry::durable::write_atomic(path, &encode_envelope(reserved))?;
    Ok(())
}

struct State {
    /// Last seq handed out.
    last_issued: u64,
    /// Highest seq covered by the persisted reservation. No seq above
    /// this is ever issued without first durably raising it.
    ceiling: u64,
}

/// Process-wide monotonic seq allocator backed by the high-water file.
///
/// Shared across all per-tenant WAL writers (`Arc<SeqAllocator>`) so
/// file sequence numbers stay globally unique. Allocation is mutex-
/// guarded — it happens once per WAL file creation, so contention and
/// the occasional in-line durable ceiling raise are negligible.
pub struct SeqAllocator {
    state: Mutex<State>,
    /// `None` for ephemeral (test/in-memory) allocators.
    persist: Option<Persist>,
}

struct Persist {
    path: PathBuf,
    batch: u64,
}

impl SeqAllocator {
    /// Durable allocator: seqs start at `seed + 1` and the first batch
    /// (`seed + batch`) is persisted before any seq is issued, so the
    /// on-disk ceiling is always ≥ every issued seq.
    ///
    /// `seed` must already fold in every known lower bound:
    /// `max(WAL scan, SFST scan, catalog scan, read_seq_highwater())`.
    pub fn durable(path: PathBuf, seed: u64, batch: u64) -> Result<Self> {
        let batch = batch.max(1);
        let ceiling = seed.saturating_add(batch);
        write_seq_highwater(&path, ceiling)?;
        Ok(Self {
            state: Mutex::new(State {
                last_issued: seed,
                ceiling,
            }),
            persist: Some(Persist { path, batch }),
        })
    }

    /// In-memory allocator with no persistence — for tests and callers
    /// that manage durability elsewhere. Seqs start at `seed + 1`.
    pub fn ephemeral(seed: u64) -> Self {
        Self {
            state: Mutex::new(State {
                last_issued: seed,
                ceiling: u64::MAX,
            }),
            persist: None,
        }
    }

    /// Allocate the next seq. Raises (and durably persists) the
    /// reservation ceiling first whenever the batch is exhausted —
    /// never returns a seq greater than the persisted ceiling.
    pub fn next(&self) -> Result<u64> {
        let mut state = self.state.lock().expect("seq allocator mutex poisoned");
        let seq = state.last_issued + 1;
        if seq > state.ceiling {
            let persist = self
                .persist
                .as_ref()
                .expect("ephemeral allocator has u64::MAX ceiling");
            let new_ceiling = seq.saturating_add(persist.batch);
            write_seq_highwater(&persist.path, new_ceiling)?;
            state.ceiling = new_ceiling;
        }
        state.last_issued = seq;
        Ok(seq)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hw_path(dir: &Path) -> PathBuf {
        dir.join(".seq_highwater")
    }

    #[test]
    fn missing_file_reads_none() {
        let tmp = tempfile::tempdir().unwrap();
        assert_eq!(read_seq_highwater(&hw_path(tmp.path())), None);
    }

    #[test]
    fn durable_allocator_persists_ceiling_before_issuing() {
        let tmp = tempfile::tempdir().unwrap();
        let path = hw_path(tmp.path());
        let alloc = SeqAllocator::durable(path.clone(), 10, 4).unwrap();

        // First batch persisted at construction: ceiling = seed + batch.
        assert_eq!(read_seq_highwater(&path), Some(14));

        // Every issued seq stays ≤ the persisted ceiling at issue time.
        for expected in 11..=20 {
            let on_disk_before = read_seq_highwater(&path).unwrap();
            let seq = alloc.next().unwrap();
            assert_eq!(seq, expected);
            let on_disk_after = read_seq_highwater(&path).unwrap();
            assert!(seq <= on_disk_after);
            assert!(on_disk_after >= on_disk_before);
        }
    }

    #[test]
    fn restart_seeds_at_or_above_last_issued_even_with_no_files() {
        let tmp = tempfile::tempdir().unwrap();
        let path = hw_path(tmp.path());

        let last_issued = {
            let alloc = SeqAllocator::durable(path.clone(), 0, 8).unwrap();
            let mut last = 0;
            for _ in 0..20 {
                last = alloc.next().unwrap();
            }
            last
        };

        // Simulate restart with ALL data files evicted: the only seed
        // input left is the high-water file.
        let highwater = read_seq_highwater(&path).unwrap();
        assert!(highwater >= last_issued);
        let alloc = SeqAllocator::durable(path.clone(), highwater, 8).unwrap();
        assert!(alloc.next().unwrap() > last_issued);
    }

    #[test]
    fn corrupt_envelope_reads_none_and_next_reservation_rewrites() {
        let tmp = tempfile::tempdir().unwrap();
        let path = hw_path(tmp.path());

        // CRC flip.
        let mut bytes = encode_envelope(42).to_vec();
        let last = bytes.len() - 1;
        bytes[last] ^= 0x01;
        std::fs::write(&path, &bytes).unwrap();
        assert_eq!(read_seq_highwater(&path), None);

        // Bad magic.
        let mut bytes = encode_envelope(42).to_vec();
        bytes[0] = b'X';
        std::fs::write(&path, &bytes).unwrap();
        assert_eq!(read_seq_highwater(&path), None);

        // Bad version.
        let mut bytes = encode_envelope(42).to_vec();
        bytes[4] = 99;
        std::fs::write(&path, &bytes).unwrap();
        assert_eq!(read_seq_highwater(&path), None);

        // Truncated.
        std::fs::write(&path, &encode_envelope(42)[..7]).unwrap();
        assert_eq!(read_seq_highwater(&path), None);

        // Startup proceeds: a fresh durable allocator rewrites a valid
        // envelope.
        let _alloc = SeqAllocator::durable(path.clone(), 0, 4).unwrap();
        assert_eq!(read_seq_highwater(&path), Some(4));
    }

    #[test]
    fn envelope_roundtrip() {
        for v in [0u64, 1, 42, u64::MAX] {
            assert_eq!(parse_envelope(&encode_envelope(v)).unwrap(), v);
        }
    }

    #[test]
    fn write_is_tmp_plus_rename() {
        let tmp = tempfile::tempdir().unwrap();
        let path = hw_path(tmp.path());
        write_seq_highwater(&path, 7).unwrap();
        // No stray tmp file is left behind and the live file is valid.
        assert_eq!(read_seq_highwater(&path), Some(7));
        assert!(!path.with_extension("tmp").exists());
    }

    #[test]
    fn ephemeral_allocates_without_a_file() {
        let alloc = SeqAllocator::ephemeral(5);
        assert_eq!(alloc.next().unwrap(), 6);
        assert_eq!(alloc.next().unwrap(), 7);
    }
}
