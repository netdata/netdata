//! Tests for the typed on-disk payloads declared in `schema.rs` (this file is
//! its child `mod tests`): the high-card/stream-batch arenas, the `serde_bytes`
//! blob annotation, and the schema tree with its read-time derivations.
//!
//! Fixtures are in-memory values built with the crate's own constructors
//! (`for_write`, `SchemaTree::flat`/`from_nodes`), encoded and decoded
//! directly through bincode (the chunk-payload codec) or via
//! `writer::pack`/`reader::unpack` (bincode + zstd) — no files are written.
//!
//! Pins:
//!
//! - the arenas round-trip with their `#[serde(skip)]` offsets rebuilt on
//!   load, and the key/row accessors (including `binary_search`) work on the
//!   decoded value;
//! - `#[serde(with = "serde_bytes")]` is wire-identical to a plain `Vec<u8>`
//!   under bincode — a decode-speed change only, no `VERSION` bump;
//! - `derive_field_table` orders low → mid → high then by name and collapses
//!   polymorphic paths to one entry; `derive_scalar_kinds` applies the
//!   coalescing lattice;
//! - `validate` rejects every malformed tree shape as `Error::CorruptIndex`
//!   at the META decode trust boundary;
//! - `fill_field_stats` then `derive_field_table` reproduces the input table
//!   exactly (the build-time invariant), and inconsistent arenas fail
//!   `rebuild_offsets` instead of corrupting later reads.
//!
//! Not pinned here: cardinality-threshold classification (every fixture
//! carries pre-assigned tiers), `BitmapValue`, stream-batch mask semantics,
//! and full writer↔reader file round-trips (`src/tests/round_trip.rs`).
/// The high-card string arena (`HighField`, the `HF{i}` chunk body)
/// round-trips through bincode, and after `rebuild_offsets` — what the
/// reader does on load, since `offsets` is `#[serde(skip)]` — its keys are
/// reachable again (`key(i)`, sorted-order `binary_search` hit and miss);
/// the per-key masks round-trip with the arena.
#[test]
fn high_field_arena_round_trips() {
    let keys = ["alpha", "bravo", "charlie"];
    let masks = vec![0b0000_0001u8, 0b0000_0011, 0b1000_0000];
    let high = crate::HighField::for_write(&keys, masks);

    let bytes = bincode::serde::encode_to_vec(&high, bincode::config::standard()).unwrap();
    let (mut decoded, _): (crate::HighField, _) =
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
    assert!(decoded.rebuild_offsets(), "consistent chunk rebuilds");

    assert_eq!(decoded, high);
    assert_eq!(decoded.len(), 3);
    assert_eq!(decoded.key(0), b"alpha");
    assert_eq!(decoded.key(2), b"charlie");
    assert_eq!(decoded.binary_search(b"bravo"), Ok(1));
    assert_eq!(decoded.binary_search(b"zzz"), Err(3));
    assert_eq!(decoded.masks, vec![0b0000_0001, 0b0000_0011, 0b1000_0000]);
}

/// The stream-batch fixed-width arena (`StreamBatch`, the `SB{i}` chunk
/// body) round-trips through bincode and its rows read back after
/// `rebuild_offsets`. `KvId(70_000)` pins the fixed 4-byte little-endian
/// stride (a value past two bytes); the empty and single-id rows pin the
/// row-length extremes.
#[test]
fn stream_batch_arena_round_trips() {
    use crate::KvId;
    let rows = vec![vec![KvId(0), KvId(1), KvId(70_000)], vec![], vec![KvId(5)]];
    let batch = crate::StreamBatch::for_write(&rows);

    let bytes = bincode::serde::encode_to_vec(&batch, bincode::config::standard()).unwrap();
    let (mut decoded, _): (crate::StreamBatch, _) =
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
    assert!(decoded.rebuild_offsets(), "consistent chunk rebuilds");

    assert_eq!(decoded, batch);
    assert_eq!(decoded.num_rows(), 3);
    assert_eq!(
        decoded.row(0).collect::<Vec<_>>(),
        vec![KvId(0), KvId(1), KvId(70_000)]
    );
    assert!(decoded.row(1).next().is_none());
    assert_eq!(decoded.row(2).collect::<Vec<_>>(), vec![KvId(5)]);
}

/// `#[serde(with = "serde_bytes")]` on the blob fields changes only the
/// decode path (one bulk copy vs serde's per-byte seq loop), never the
/// on-disk bytes: under bincode both paths encode `[varint len][raw
/// bytes]`, so files written before the annotation still decode after it
/// and `VERSION` need not bump.
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

    // A 1000-byte payload, so the bincode length prefix is a multi-byte
    // varint.
    let data: Vec<u8> = (0..1000u32).map(|i| (i % 251) as u8).collect();
    let cfg = bincode::config::standard();

    let plain = bincode::serde::encode_to_vec(&PlainSeq { blob: data.clone() }, cfg).unwrap();
    let bytes = bincode::serde::encode_to_vec(&WithBytes { blob: data.clone() }, cfg).unwrap();

    // Identical on disk — the whole point.
    assert_eq!(plain, bytes, "serde_bytes changed the on-disk encoding");

    // And bytes written the plain way decode through the annotated
    // struct — adding `serde_bytes` never changes what is readable.
    let (decoded, _): (WithBytes, _) = bincode::serde::decode_from_slice(&plain, cfg).unwrap();
    assert_eq!(decoded.blob, data);
}

// ── Typed schema tree (the on-disk field descriptor) ─────────────

use crate::{
    FieldEntry, FieldTable, FieldTier, LeafStats, SchemaEdge, SchemaNode, SchemaTree, Step,
    ValueKind,
};

/// `derive_field_table` over a `flat` tree hands the input table back,
/// canonically reordered low → mid → high then by name regardless of the
/// order it was built in. The tier machinery (`num_mid`/`locate_field`/
/// `high_kv_id`) and build-time KvId assignment depend on this order.
#[test]
fn derive_field_table_is_canonically_ordered() {
    let fields: FieldTable = vec![
        FieldEntry {
            name: "zeta".into(),
            cardinality: 5,
            tier: FieldTier::High,
        },
        FieldEntry {
            name: "alpha".into(),
            cardinality: 2,
            tier: FieldTier::Low,
        },
        FieldEntry {
            name: "mid_b".into(),
            cardinality: 200,
            tier: FieldTier::Mid,
        },
        FieldEntry {
            name: "mid_a".into(),
            cardinality: 300,
            tier: FieldTier::Mid,
        },
    ]
    .into();

    let derived = SchemaTree::flat(&fields).derive_field_table();

    let names: Vec<&str> = derived.names().collect();
    assert_eq!(names, vec!["alpha", "mid_a", "mid_b", "zeta"]);
    assert_eq!(derived.get("zeta").unwrap().cardinality, 5);
    assert_eq!(derived.get("zeta").unwrap().tier, FieldTier::High);
    assert_eq!(derived.get("alpha").unwrap().tier, FieldTier::Low);
}

/// A polymorphic path (several leaf kinds at one path) collapses to a
/// single `FieldEntry` — storage is path-keyed, one field name per path,
/// whose sibling leaves share the path-level stats.
#[test]
fn derive_field_table_collapses_polymorphic_path() {
    let stats = LeafStats {
        cardinality: 7,
        tier: FieldTier::Low,
    };
    let leaf = |name: &str, kind| SchemaNode {
        kind,
        edge: Some(SchemaEdge {
            parent: 0,
            step: Step::Field(name.into()),
        }),
        leaf: Some(stats),
    };
    let tree = SchemaTree::from_nodes(vec![
        SchemaNode {
            kind: ValueKind::Kvlist,
            edge: None,
            leaf: None,
        },
        leaf("id", ValueKind::Int),
        leaf("id", ValueKind::Str),
    ]);

    let derived = tree.derive_field_table();
    assert_eq!(derived.len(), 1);
    assert_eq!(derived.get("id").unwrap().cardinality, 7);
}

/// The scalar coalescing lattice, path-ordered: `Null` drops; containers,
/// empty or not, contribute no scalar; `Int ⊔ Double = Double`; any other
/// scalar mix → `Str`; a scalar-plus-container path surfaces its scalar
/// leaf (the container occurrences live at child paths).
#[test]
fn scalar_coalescing_lattice() {
    let stats = LeafStats {
        cardinality: 1,
        tier: FieldTier::Low,
    };
    let node = |parent: u32, name: &str, kind: ValueKind| SchemaNode {
        kind,
        edge: Some(SchemaEdge {
            parent,
            step: Step::Field(name.into()),
        }),
        leaf: if kind.is_leaf() { Some(stats) } else { None },
    };
    let tree = SchemaTree::from_nodes(vec![
        SchemaNode {
            kind: ValueKind::Kvlist,
            edge: None,
            leaf: None,
        }, // 0 root
        node(0, "nullstr", ValueKind::Null),     // 1
        node(0, "nullstr", ValueKind::Str),      // 2  -> Str
        node(0, "intdouble", ValueKind::Int),    // 3
        node(0, "intdouble", ValueKind::Double), // 4  -> Double
        node(0, "intstr", ValueKind::Int),       // 5
        node(0, "intstr", ValueKind::Str),       // 6  -> Str
        node(0, "arr", ValueKind::EmptyArray),   // 7  leaf
        node(0, "arr", ValueKind::Array),        // 8  interior -> excluded
        node(0, "scalarobj", ValueKind::Str),    // 9  leaf -> Str
        node(0, "scalarobj", ValueKind::Kvlist), // 10 interior
        node(0, "nullobj", ValueKind::Null),     // 11 leaf
        node(0, "nullobj", ValueKind::Kvlist),   // 12 interior -> excluded
    ]);

    let scalars = tree.derive_scalar_kinds();
    assert_eq!(
        scalars,
        vec![
            ("intdouble".to_string(), ValueKind::Double),
            ("intstr".to_string(), ValueKind::Str),
            ("nullstr".to_string(), ValueKind::Str),
            ("scalarobj".to_string(), ValueKind::Str),
        ]
    );
}

/// `validate` is the trust boundary `ChunkReader::metadata` applies to a
/// decoded tree: no root, an edge on node 0, a non-root without an edge,
/// or a parent not strictly below its own id (out-of-range, self-cycle,
/// and forward edges are that one rule) must degrade to `CorruptIndex` —
/// never panic the unchecked `node`/`steps` walks. The bad trees use the
/// struct literal (this child module sees the private `nodes` field) to
/// bypass `from_nodes`' debug-assert, mimicking a bincode-decoded tree.
#[test]
fn validate_rejects_malformed_trees() {
    use crate::Error;
    let root = || SchemaNode {
        kind: ValueKind::Kvlist,
        edge: None,
        leaf: None,
    };
    let leaf = |parent: u32, name: &str| SchemaNode {
        kind: ValueKind::Str,
        edge: Some(SchemaEdge {
            parent,
            step: Step::Field(name.into()),
        }),
        leaf: Some(LeafStats {
            cardinality: 1,
            tier: FieldTier::Low,
        }),
    };

    // Well-formed: root + a child pointing back to it.
    assert!(
        SchemaTree {
            nodes: vec![root(), leaf(0, "a")]
        }
        .validate()
        .is_ok()
    );

    let bad = [
        SchemaTree { nodes: vec![] }, // no root
        SchemaTree {
            nodes: vec![leaf(0, "x")],
        }, // node 0 has an edge
        SchemaTree {
            nodes: vec![root(), leaf(99, "x")],
        }, // out-of-range parent
        SchemaTree {
            nodes: vec![root(), leaf(1, "x")],
        }, // self-cycle (parent == id)
        SchemaTree {
            nodes: vec![root(), leaf(2, "x"), leaf(0, "y")],
        }, // forward edge
        SchemaTree {
            nodes: vec![
                root(),
                SchemaNode {
                    kind: ValueKind::Str,
                    edge: None,
                    leaf: None,
                },
            ],
        }, // non-root node missing its edge
    ];
    for (i, tree) in bad.iter().enumerate() {
        assert!(
            matches!(tree.validate(), Err(Error::CorruptIndex(_))),
            "malformed tree #{i} should be rejected as CorruptIndex"
        );
    }
}

/// `SchemaTree::default()` is the canonical empty descriptor — a root-only
/// tree, not an empty arena — so it passes `validate`, derives an empty
/// field table, and equals `flat(&FieldTable::default())`.
#[test]
fn default_tree_is_valid_root_only() {
    let d = SchemaTree::default();
    assert_eq!(d.len(), 1);
    assert!(d.validate().is_ok());
    assert_eq!(d.derive_field_table(), FieldTable::default());
    assert_eq!(d, SchemaTree::flat(&FieldTable::default()));
}

/// A full `Metadata` carrying a typed tree round-trips through the on-disk
/// codec (`pack`/`unpack` — bincode + zstd), and the derived field table
/// survives the trip in canonical order (`level` low before `host` high).
#[test]
fn metadata_tree_round_trips() {
    use crate::{Histogram, IdRanges, KvId, Metadata};
    let fields: FieldTable = vec![
        FieldEntry {
            name: "host".into(),
            cardinality: 300,
            tier: FieldTier::High,
        },
        FieldEntry {
            name: "level".into(),
            cardinality: 2,
            tier: FieldTier::Low,
        },
    ]
    .into();
    let meta = Metadata {
        histogram: Histogram {
            timestamps: vec![1],
            counts: vec![1],
        },
        id_ranges: IdRanges {
            low_end: KvId(2),
            mid_end: KvId(2),
            high_end: KvId(302),
        },
        tree: SchemaTree::flat(&fields),
        columns: Default::default(),
    };
    let packed = crate::writer::pack(&meta, 1).unwrap();
    let got: Metadata = crate::reader::unpack(&packed).unwrap();
    assert_eq!(got, meta);
    assert_eq!(
        got.tree.derive_field_table(),
        meta.tree.derive_field_table()
    );
    assert_eq!(
        got.tree.derive_field_table().names().collect::<Vec<_>>(),
        vec!["level", "host"]
    );
}

/// `fill_field_stats` attaches per-path cardinality/tier to a structurally
/// built tree (the `ng-index` path: kinds known, stats `None`), and the
/// derived table then reproduces the input field table exactly — the
/// build-time invariant. Covers a nested path, a polymorphic path
/// (deduped), and an interior node (excluded).
#[test]
fn fill_field_stats_then_derive_matches_fields() {
    let node = |parent: u32, name: &str, kind: ValueKind| SchemaNode {
        kind,
        edge: Some(SchemaEdge {
            parent,
            step: Step::Field(name.into()),
        }),
        leaf: None, // stats unset — as ng-index supplies it
    };
    let mut tree = SchemaTree::from_nodes(vec![
        SchemaNode {
            kind: ValueKind::Kvlist,
            edge: None,
            leaf: None,
        }, // 0 root
        node(0, "level", ValueKind::Str),  // 1
        node(0, "id", ValueKind::Int),     // 2 polymorphic
        node(0, "id", ValueKind::Str),     // 3 polymorphic
        node(0, "obj", ValueKind::Kvlist), // 4 interior
        node(4, "x", ValueKind::Str),      // 5 -> path "obj.x"
        node(0, "host", ValueKind::Str),   // 6
    ]);

    let fields: FieldTable = vec![
        FieldEntry {
            name: "id".into(),
            cardinality: 7,
            tier: FieldTier::Low,
        },
        FieldEntry {
            name: "level".into(),
            cardinality: 2,
            tier: FieldTier::Low,
        },
        FieldEntry {
            name: "obj.x".into(),
            cardinality: 200,
            tier: FieldTier::Mid,
        },
        FieldEntry {
            name: "host".into(),
            cardinality: 300,
            tier: FieldTier::High,
        },
    ]
    .into();

    tree.fill_field_stats(&fields);
    let derived = tree.derive_field_table();

    // One entry per distinct leaf path (polymorphic `id` collapsed; interior
    // `obj` excluded), canonically ordered low → mid → high then by name.
    let names: Vec<&str> = derived.names().collect();
    assert_eq!(names, vec!["id", "level", "obj.x", "host"]);
    assert_eq!(
        *derived, *fields,
        "derived table must reproduce the input fields"
    );
}

/// A CRC passes a decodable-but-inconsistent arena, so `rebuild_offsets`
/// is the structural check: a length that disagrees with the blob, or a
/// wrapping overflow, must fail the rebuild — never surface later as a
/// panic or a silently wrong slice.
#[test]
fn inconsistent_arenas_fail_the_offset_rebuild() {
    let keys = ["alpha", "bravo"];
    let high = crate::HighField::for_write(&keys, vec![1, 1]);
    let bytes = bincode::serde::encode_to_vec(&high, bincode::config::standard()).unwrap();
    let (mut decoded, _): (crate::HighField, _) =
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
    decoded.key_lens[1] = 100; // claims more bytes than the blob holds
    assert!(!decoded.rebuild_offsets(), "length mismatch is corruption");
    decoded.key_lens[1] = u32::MAX; // wrapping sum
    assert!(!decoded.rebuild_offsets(), "overflow is corruption");

    use crate::KvId;
    let batch = crate::StreamBatch::for_write(&[vec![KvId(1)], vec![KvId(2)]]);
    let bytes = bincode::serde::encode_to_vec(&batch, bincode::config::standard()).unwrap();
    let (mut decoded, _): (crate::StreamBatch, _) =
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
    decoded.row_lens[0] = 7;
    assert!(!decoded.rebuild_offsets(), "length mismatch is corruption");
    decoded.row_lens[0] = u32::MAX;
    assert!(!decoded.rebuild_offsets(), "overflow is corruption");
}
