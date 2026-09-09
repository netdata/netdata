use super::*;

#[test]
fn round_trips() {
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
    // get swallowed by the `u32::MAX` tail sentinel. Pins the `from_wire`
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
    assert_eq!(Cursor::decode(""), None);
    assert_eq!(Cursor::decode("1:2:3"), None); // too few fields (legacy 3-field)
    assert_eq!(Cursor::decode("1:2:3:4:5"), None); // too many fields
    assert_eq!(Cursor::decode("x:2:3:4"), None); // non-integer timestamp
    assert_eq!(Cursor::decode("1:2:3:-4"), None); // negative u32 position
    assert_eq!(Cursor::decode("1:2:3:4 "), None); // trailing whitespace
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
