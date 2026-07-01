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

// ── Typed schema tree (v9 field descriptor) ──────────────────────

use crate::{
    FieldEntry, FieldTable, FieldTier, LeafStats, SchemaEdge, SchemaNode, SchemaTree, Step,
    ValueKind,
};

/// `derive_field_table` reproduces the flat table a `flat` tree was built from,
/// in the canonical low → mid → high then-by-name order — regardless of input
/// order. This is the order the tier machinery (`num_mid`/`locate_field`/
/// `high_kv_id`) and KvId assignment depend on.
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

/// A polymorphic path (multiple leaf kinds at the same path) collapses to a
/// single `FieldEntry` in the derived table — matching path-keyed storage,
/// where one field name carries all of a path's `key=value` terms.
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

/// The scalar coalescing lattice: drop `Null`; empty/non-empty
/// containers contribute no scalar; `Int ⊔ Double = Double`; any other scalar
/// mix → `Str`; a scalar-vs-container path surfaces its scalar leaf (the
/// container occurrences live at child paths).
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

/// `validate` accepts well-formed trees and rejects every malformed shape a
/// decoded (untrusted) file could carry, so `Reader::metadata` degrades to
/// `CorruptIndex` instead of panicking on unchecked indexing (`node`/`steps`)
/// or hanging the parent walk. The bad trees are built via the struct literal
/// (the `schema::tests` child module sees the private `nodes` field) to bypass
/// `from_nodes`' debug-assert and mimic a bincode-decoded tree.
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

/// `SchemaTree::default()` is the canonical empty descriptor — a valid root-only
/// tree (not an empty arena), so it passes `validate`, derives an empty field
/// table, and equals `flat(&FieldTable::default())`.
#[test]
fn default_tree_is_valid_root_only() {
    let d = SchemaTree::default();
    assert_eq!(d.len(), 1);
    assert!(d.validate().is_ok());
    assert_eq!(d.derive_field_table(), FieldTable::default());
    assert_eq!(d, SchemaTree::flat(&FieldTable::default()));
}

/// A full `Metadata` carrying a typed tree round-trips through the on-disk
/// codec (zstd + bincode via `pack`/`unpack`), and the derived field table
/// survives.
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

/// `fill_field_stats` attaches per-path cardinality/tier to a structurally-built
/// tree (the `ng-index` path: kinds known, stats `None`), and the derived field
/// table then reproduces the input field table exactly — the build-time
/// correctness invariant. Covers nesting, a polymorphic path (deduped), and an
/// interior node (excluded).
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
