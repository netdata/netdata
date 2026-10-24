//! Tests for `ChunkReader::open`'s container validation (length, magic,
//! version — checked in that order by the shared container helper) and
//! `read_summary`'s handling of summary-only files.
//!
//! Fixtures are built with the crate's own writer (`write_summary_only`)
//! and patched byte-wise, so each test isolates one header field.
//!
//! Pins:
//!
//! - wrong magic → `Error::InvalidMagic`;
//! - input under the 12-byte header floor → `Error::FileTooShort(have, need)`;
//! - any version but the current one → `Error::UnsupportedVersion` carrying
//!   the file's value, even on an otherwise-valid file (no forward
//!   compatibility);
//! - a summary-only file — the format's minimal valid shape — serves
//!   `read_summary` but is refused by `IndexReader::open`.
//!
//! Not pinned here: chunk addressing, per-chunk decode, CRC-on-access and
//! full writer↔reader round-trips (`src/tests/round_trip.rs`), and the
//! same header checks one layer down on the container itself
//! (chunk-file's own `container.rs` tests).
use super::*;

#[test]
fn error_on_bad_magic() {
    // A 32-byte blob — past the 12-byte header floor, so only the magic
    // check can fire — with a non-SFST magic is rejected at open with
    // `Error::InvalidMagic` (the container's BadMagic, reshaped).
    let data = b"BADXxxxxxxxxxxxxxxxxxxxxxxxxxxxx";
    assert!(matches!(ChunkReader::open(data), Err(Error::InvalidMagic)));
}

#[test]
fn error_on_short_file() {
    // Four bytes — a well-formed magic, but under the 12-byte header —
    // fail on length alone, before the magic is even consulted: the
    // error pairs (actual len, header floor) = (4, 12).
    let data = b"SFST";
    assert!(matches!(
        ChunkReader::open(data),
        Err(Error::FileTooShort(4, 12))
    ));
}

#[test]
fn error_on_other_format_version() {
    // A valid file whose version field is patched to any other version is
    // rejected on open, the error carrying the file's value — readers
    // accept exactly the current version.
    let summary = crate::Summary {
        min_timestamp_s: 0,
        max_timestamp_s: 0,
        record_count: 1,
        content_meta: Vec::new(),
    };
    let mut buf = crate::writer::write_summary_only(std::io::Cursor::new(Vec::new()), &summary)
        .unwrap()
        .into_inner();
    // Header layout: magic(4) | version(4, LE) | num_chunks(4).
    buf[4..8].copy_from_slice(&2u32.to_le_bytes());
    assert!(matches!(
        ChunkReader::open(&buf),
        Err(Error::UnsupportedVersion(2))
    ));
}

#[test]
fn read_summary_serves_summary_only_files() {
    // A summary-only file (only the SUMR chunk) is the format's minimal
    // valid shape. `IndexReader::open` refuses it — open also eagerly
    // decodes META and PRIM, which don't exist here — while `read_summary`
    // touches header + TOC + SUMR only and must succeed, returning the
    // summary exactly as written.
    let summary = Summary {
        min_timestamp_s: 10,
        max_timestamp_s: 20,
        record_count: 3,
        content_meta: vec![1, 2, 3],
    };
    let buf = crate::writer::write_summary_only(std::io::Cursor::new(Vec::new()), &summary)
        .unwrap()
        .into_inner();
    assert!(crate::IndexReader::open(&buf).is_err());
    assert_eq!(crate::read_summary(&buf).unwrap(), summary);
}
