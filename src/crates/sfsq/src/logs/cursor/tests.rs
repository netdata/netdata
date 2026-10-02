//! Tests for the pagination cursor (`cursor.rs`) at the value level —
//! no fixtures or I/O: plain `Cursor` values, wire strings, comparisons.
//!
//! Pins:
//!
//! - `encode` emits `"{timestamp_ns}:{file_seq}:{part}:{position}"` and
//!   `decode` inverts it; `Tail` rides the wire as the `u32::MAX`
//!   sentinel;
//! - the largest legal index, `u32::MAX - 1`, stays `Indexed` across the
//!   round-trip instead of decoding back as the sentinel's `Tail`;
//! - `decode` accepts exactly four integer fields and rejects every other
//!   shape — including the legacy 3-field form — so a malformed anchor
//!   degrades to "no anchor" at the wire adapter
//!   (otel-ledger `src/ledger/rpc/logs/adapter.rs`);
//! - the derived `Ord` orders by timestamp, then `file_seq`, then `part`,
//!   then `position`, with every `Indexed` before `Tail` (the wire order
//!   `0 < … < u32::MAX`) — the global order the page merge relies on.
//!
//! Not pinned here: negative `timestamp_ns` or extreme `file_seq` /
//! `position` values, [`Cursor::synthetic_max`], and pagination over real
//! files (only incidental via `tests/ng_wal_equivalence.rs` — no anchor
//! paging or has-more pinning exists in-tree; `page/tests.rs` is
//! value-level and reads no files).
use super::*;

#[test]
fn round_trips() {
    // Canonical round-trip: a full-precision nanosecond timestamp and an
    // indexed part pin the exact wire string; decode returns an equal cursor.
    let c = Cursor {
        timestamp_ns: 1_700_000_000_123_456_789,
        file_seq: 42,
        part: Part::Indexed(3),
        position: 7,
    };
    let s = c.encode();
    assert_eq!(s, "1700000000123456789:42:3:7");
    assert_eq!(Cursor::decode(&s), Some(c));
}

#[test]
fn tail_round_trips_via_sentinel() {
    // The tail encodes as the `u32::MAX` sentinel and decodes back to it.
    let c = Cursor {
        timestamp_ns: 100,
        file_seq: 1,
        part: Part::Tail,
        position: 0,
    };
    let s = c.encode();
    assert_eq!(s, "100:1:4294967295:0");
    assert_eq!(Cursor::decode(&s), Some(c));
}

#[test]
fn indexed_high_value_round_trips() {
    // A large indexed value must stay `Indexed` across encode/decode — not
    // be swallowed by the `u32::MAX` tail sentinel. Pins the `from_wire`
    // boundary: the largest legal index is `u32::MAX - 1`.
    let c = Cursor {
        timestamp_ns: 1,
        file_seq: 7,
        part: Part::Indexed(u32::MAX - 1),
        position: 2,
    };
    let s = c.encode();
    assert_eq!(s, "1:7:4294967294:2");
    assert_eq!(Cursor::decode(&s), Some(c));
    assert_ne!(Cursor::decode(&s).unwrap().part, Part::Tail);
}

#[test]
fn decode_rejects_malformed() {
    // Decode is strict: exactly four integer fields, everything else is
    // `None`. Callers treat `None` as "no anchor" (page from the edge),
    // so every malformed shape below must land on `None`.
    assert_eq!(Cursor::decode(""), None);
    assert_eq!(Cursor::decode("1:2:3"), None); // too few fields — the legacy 3-field form
    assert_eq!(Cursor::decode("1:2:3:4:5"), None); // too many fields
    assert_eq!(Cursor::decode("x:2:3:4"), None); // non-integer timestamp
    assert_eq!(Cursor::decode("1:2:3:-4"), None); // negative u32 position
    assert_eq!(Cursor::decode("1:2:3:4 "), None); // trailing whitespace (parse does not trim)
}

#[test]
fn ordering_is_ts_then_seq_then_part_then_position() {
    let c = |timestamp_ns, file_seq, part, position| Cursor {
        timestamp_ns,
        file_seq,
        part,
        position,
    };
    // Same timestamp → lower file_seq sorts first.
    assert!(c(100, 0, Part::Indexed(0), 9) < c(100, 1, Part::Indexed(0), 0));
    // Higher timestamp wins regardless of the rest.
    assert!(c(100, 1, Part::Indexed(0), 0) < c(101, 0, Part::Indexed(0), 0));
    // Same (timestamp, seq) → indexed sources sort by index, all before the
    // tail: Indexed(0) < Indexed(MAX-1) < Tail. This pins the wire order
    // 0 < … < u32::MAX that the derived `Ord` must reproduce.
    assert!(c(100, 5, Part::Indexed(0), 0) < c(100, 5, Part::Indexed(u32::MAX - 1), 0));
    assert!(c(100, 5, Part::Indexed(u32::MAX - 1), 0) < c(100, 5, Part::Tail, 0));
    assert!(c(100, 5, Part::Indexed(2), 99) < c(100, 5, Part::Tail, 0));
    // Same (timestamp, seq, part) → lower position sorts first.
    assert!(c(100, 0, Part::Indexed(0), 9) < c(100, 0, Part::Indexed(0), 10));
}
