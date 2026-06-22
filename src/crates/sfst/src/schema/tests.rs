/// The high-card string-arena round-trips through bincode (the on-disk
/// codec) and its keys are accessible by index after `rebuild_offsets` —
/// which is what the reader does on load (`offsets` is `#[serde(skip)]`).
#[test]
fn high_field_arena_round_trips() {
    let keys = ["alpha", "bravo", "charlie"];
    let masks = vec![0b0000_0001u8, 0b0000_0011, 0b1000_0000];
    let high = crate::HighField::for_write(&keys, masks);

    let bytes = bincode::serde::encode_to_vec(&high, bincode::config::standard()).unwrap();
    let (mut decoded, _): (crate::HighField, _) =
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
    decoded.rebuild_offsets();

    assert_eq!(decoded, high);
    assert_eq!(decoded.len(), 3);
    assert_eq!(decoded.key(0), b"alpha");
    assert_eq!(decoded.key(2), b"charlie");
    assert_eq!(decoded.binary_search(b"bravo"), Ok(1));
    assert_eq!(decoded.binary_search(b"zzz"), Err(3));
    assert_eq!(decoded.masks, vec![0b0000_0001, 0b0000_0011, 0b1000_0000]);
}

/// The stream-batch fixed-width arena round-trips through bincode and its
/// rows are readable after `rebuild_offsets`. Covers a large `KvId` (4-byte),
/// an empty row, and a single-id row.
#[test]
fn stream_batch_arena_round_trips() {
    use crate::KvId;
    let rows = vec![vec![KvId(0), KvId(1), KvId(70_000)], vec![], vec![KvId(5)]];
    let batch = crate::StreamBatch::for_write(&rows);

    let bytes = bincode::serde::encode_to_vec(&batch, bincode::config::standard()).unwrap();
    let (mut decoded, _): (crate::StreamBatch, _) =
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
    decoded.rebuild_offsets();

    assert_eq!(decoded, batch);
    assert_eq!(decoded.num_rows(), 3);
    assert_eq!(
        decoded.row(0).collect::<Vec<_>>(),
        vec![KvId(0), KvId(1), KvId(70_000)]
    );
    assert!(decoded.row(1).next().is_none());
    assert_eq!(decoded.row(2).collect::<Vec<_>>(), vec![KvId(5)]);
}

/// `#[serde(with = "serde_bytes")]` on the `Vec<u8>` blob fields changes the
/// decode path (one bulk copy vs serde's per-byte seq loop) but **not** the
/// on-disk bytes. This guards the format-transparency claim: under bincode a
/// `Vec<u8>` encodes identically via the seq path and the bytes path (both
/// `[varint len][raw bytes]`), so files written before the annotation still
/// decode after it and `VERSION` need not bump.
#[test]
fn serde_bytes_is_wire_compatible_with_plain_vec_u8() {
    use serde::{Deserialize, Serialize};

    // The *old* shape: a plain `Vec<u8>` field (serde's generic seq path).
    #[derive(Serialize)]
    struct PlainSeq {
        blob: Vec<u8>,
    }
    // The *new* shape: the same field routed through `serde_bytes`.
    #[derive(Serialize, Deserialize, PartialEq, Debug)]
    struct WithBytes {
        #[serde(with = "serde_bytes")]
        blob: Vec<u8>,
    }

    // Non-trivial payload spanning a length that needs a multi-byte varint.
    let data: Vec<u8> = (0..1000u32).map(|i| (i % 251) as u8).collect();
    let cfg = bincode::config::standard();

    let plain = bincode::serde::encode_to_vec(&PlainSeq { blob: data.clone() }, cfg).unwrap();
    let bytes = bincode::serde::encode_to_vec(&WithBytes { blob: data.clone() }, cfg).unwrap();

    // Identical on disk — the whole point.
    assert_eq!(plain, bytes, "serde_bytes changed the on-disk encoding");

    // And bytes written the *old* way decode through the *new* (annotated)
    // struct — i.e. a pre-change v4 file is still readable.
    let (decoded, _): (WithBytes, _) = bincode::serde::decode_from_slice(&plain, cfg).unwrap();
    assert_eq!(decoded.blob, data);
}
