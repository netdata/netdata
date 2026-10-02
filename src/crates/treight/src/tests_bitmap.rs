//! Tests for the high-level `Bitmap` layer (bitmap.rs): the `RawBitmap`
//! descriptor plus `inverted` flag that lets dense sets store their
//! complement. Pinned here:
//!
//! - Query results are representation-independent: `contains`, `len` and
//!   `is_empty` answer the same whether the set is stored directly or as
//!   its complement, and `is_inverted` reports which form is in use.
//! - `contains` rejects out-of-range values before the flag flips the
//!   answer — even `full`, which contains every in-range value, excludes
//!   everything at or past the universe.
//! - `and`/`or` dispatch on the two inverted flags (the De Morgan tables
//!   in bitmap.rs), pinned per combination and exhaustively against a
//!   plain set oracle over the same logical operands.
//! - `and_not` = `and` with the complemented rhs, across flag
//!   combinations and the full/empty identities.
//! - `empty`/`full` carry their state in the flag alone over an empty raw
//!   tree; boolean short-circuits against them return identity results,
//!   `and` with a full operand copying the other side's tree bytes into
//!   `out`.
//! - A double `complement()` round-trips to the original set.
//!
//! The raw tree layer itself is pinned in tests_raw.rs; the roaring
//! bridge in tests_roaring.rs (feature `roaring`).
use crate::*;

// ── single-bitmap queries ─────────────────────────────────────────

/// A directly stored bitmap contains exactly its stored values: present
/// values hit, absent and out-of-range values miss.
#[test]
fn test_bitmap_contains_normal() {
    let mut data = Vec::new();
    let bm = Bitmap::from_sorted_iter([10, 20, 30].into_iter(), 64, &mut data);
    assert!(bm.contains(&data, 10));
    assert!(bm.contains(&data, 20));
    assert!(bm.contains(&data, 30));
    assert!(!bm.contains(&data, 0));
    assert!(!bm.contains(&data, 15));
    assert!(!bm.is_inverted());
}

/// The complement-stored form of the same set answers the opposite way:
/// stored (clear) values miss, every other value in the universe hits.
#[test]
fn test_bitmap_contains_inverted() {
    let mut data = Vec::new();
    let bm = Bitmap::from_sorted_iter_complemented([10, 20, 30].into_iter(), 64, &mut data);
    assert!(!bm.contains(&data, 10));
    assert!(!bm.contains(&data, 20));
    assert!(!bm.contains(&data, 30));
    assert!(bm.contains(&data, 0));
    assert!(bm.contains(&data, 15));
    assert!(bm.contains(&data, 63));
    assert!(bm.is_inverted());
}

/// Out-of-range values are never contained, whatever the representation:
/// the universe bound is checked before the inverted flag flips the
/// answer — so `full`, which contains every in-range value, still rejects
/// everything at or past the universe.
#[test]
fn test_bitmap_contains_out_of_bounds() {
    let mut data = Vec::new();
    let normal = Bitmap::from_sorted_iter([0, 1, 2].into_iter(), 5, &mut data);
    assert!(!normal.contains(&data, 5));
    assert!(!normal.contains(&data, 100));

    let mut data2 = Vec::new();
    let inverted = Bitmap::from_sorted_iter_complemented([1].into_iter(), 5, &mut data2);
    assert!(inverted.contains(&data2, 0));
    assert!(!inverted.contains(&data2, 1));
    assert!(inverted.contains(&data2, 4));
    assert!(!inverted.contains(&data2, 5));
    assert!(!inverted.contains(&data2, 100));

    let full = Bitmap::full(5);
    let empty_data: Vec<u8> = Vec::new();
    assert!(full.contains(&empty_data, 0));
    assert!(full.contains(&empty_data, 4));
    assert!(!full.contains(&empty_data, 5));
    assert!(!full.contains(&empty_data, u32::MAX));
}

/// `len` counts logical set bits: the stored count when directly stored,
/// universe minus stored count when complement-stored (3 clear of 64
/// means 61 set).
#[test]
fn test_bitmap_len() {
    let mut data = Vec::new();
    let bm = Bitmap::from_sorted_iter([10, 20, 30].into_iter(), 64, &mut data);
    assert_eq!(bm.len(&data), 3);

    let mut data2 = Vec::new();
    let bm = Bitmap::from_sorted_iter_complemented([10, 20, 30].into_iter(), 64, &mut data2);
    assert_eq!(bm.len(&data2), 61);
}

/// `empty` and `full` are pure flag states over an empty raw tree: with a
/// zero-byte data buffer, empty reports no bits and full behaves as if
/// every value in the universe were set.
#[test]
fn test_bitmap_empty_full() {
    let bm = Bitmap::empty(64);
    let empty_data: Vec<u8> = Vec::new();
    assert!(bm.is_empty(&empty_data));
    assert_eq!(bm.len(&empty_data), 0);
    assert!(!bm.contains(&empty_data, 0));

    let bm = Bitmap::full(64);
    assert!(!bm.is_empty(&empty_data));
    assert_eq!(bm.len(&empty_data), 64);
    for i in 0..64 {
        assert!(bm.contains(&empty_data, i));
    }
}

// ── boolean ops: the De Morgan and/or tables ──────────────────────
/// De Morgan row N&N: a raw intersection, result normal.
/// {0,1,2,3} ∩ {2,3,4,5} = {2,3}.
#[test]
fn test_bitmap_and_nn() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([0, 1, 2, 3].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter([2, 3, 4, 5].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.and(&da, &b, &db, &mut out);
    assert!(!c.is_inverted());
    assert_eq!(c.len(&out), 2);
    assert!(c.contains(&out, 2));
    assert!(c.contains(&out, 3));
    assert!(!c.contains(&out, 0));
    assert!(!c.contains(&out, 4));
}

/// De Morgan row N&I: lowered to a raw difference, result normal.
/// {0,1,2,3} ∩ ¬{2,3} = {0,1}.
#[test]
fn test_bitmap_and_ni() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([0, 1, 2, 3].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter_complemented([2, 3].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.and(&da, &b, &db, &mut out);
    assert!(!c.is_inverted());
    assert_eq!(c.len(&out), 2);
    assert!(c.contains(&out, 0));
    assert!(c.contains(&out, 1));
    assert!(!c.contains(&out, 2));
    assert!(!c.contains(&out, 3));
}

/// De Morgan row I&N: the inverted lhs's stored tree is subtracted from
/// the rhs's, result normal. ¬{2,3} ∩ {0,1,2,3} = {0,1}.
#[test]
fn test_bitmap_and_in() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter_complemented([2, 3].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter([0, 1, 2, 3].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.and(&da, &b, &db, &mut out);
    assert!(!c.is_inverted());
    assert_eq!(c.len(&out), 2);
    assert!(c.contains(&out, 0));
    assert!(c.contains(&out, 1));
    assert!(!c.contains(&out, 2));
}

/// De Morgan row I&I: lowered to a raw union, result inverted.
/// ¬{0,1} ∩ ¬{2,3} = ¬{0,1,2,3} = {4..63}.
#[test]
fn test_bitmap_and_ii() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter_complemented([0, 1].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter_complemented([2, 3].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.and(&da, &b, &db, &mut out);
    assert!(c.is_inverted());
    assert!(!c.contains(&out, 0));
    assert!(!c.contains(&out, 1));
    assert!(!c.contains(&out, 2));
    assert!(!c.contains(&out, 3));
    assert!(c.contains(&out, 4));
    assert!(c.contains(&out, 63));
}

/// De Morgan row N|N: a raw union, result normal.
/// {0,1} ∪ {2,3} = {0,1,2,3}.
#[test]
fn test_bitmap_or_nn() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([0, 1].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter([2, 3].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.or(&da, &b, &db, &mut out);
    assert!(!c.is_inverted());
    assert_eq!(c.len(&out), 4);
    assert!(c.contains(&out, 0));
    assert!(c.contains(&out, 3));
    assert!(!c.contains(&out, 4));
}

/// De Morgan row N|I: lowered to an inverted difference.
/// {10,20} ∪ ¬{5,10,15} = ¬{5,15}, result inverted.
#[test]
fn test_bitmap_or_ni() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([10, 20].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter_complemented([5, 10, 15].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.or(&da, &b, &db, &mut out);
    assert!(c.is_inverted());
    assert!(!c.contains(&out, 5));
    assert!(c.contains(&out, 10));
    assert!(!c.contains(&out, 15));
    assert!(c.contains(&out, 20));
    assert!(c.contains(&out, 0));
    assert!(c.contains(&out, 63));
}

/// De Morgan row I|N, mirror of N|I: ¬{5,10,15} ∪ {10,20} = ¬{5,15},
/// result inverted.
#[test]
fn test_bitmap_or_in() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter_complemented([5, 10, 15].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter([10, 20].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.or(&da, &b, &db, &mut out);
    assert!(c.is_inverted());
    assert!(!c.contains(&out, 5));
    assert!(c.contains(&out, 10));
    assert!(!c.contains(&out, 15));
    assert!(c.contains(&out, 20));
}

/// De Morgan row I|I: lowered to a raw intersection, result inverted.
/// ¬{0,1,2} ∪ ¬{2,3,4} = ¬{2} — 63 of 64 values set.
#[test]
fn test_bitmap_or_ii() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter_complemented([0, 1, 2].into_iter(), 64, &mut da);
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter_complemented([2, 3, 4].into_iter(), 64, &mut db);
    let mut out = Vec::new();
    let c = a.or(&da, &b, &db, &mut out);
    assert!(c.is_inverted());
    assert_eq!(c.len(&out), 63);
    assert!(c.contains(&out, 0));
    assert!(c.contains(&out, 1));
    assert!(!c.contains(&out, 2));
    assert!(c.contains(&out, 3));
    assert!(c.contains(&out, 4));
}

/// `and` short-circuits on degenerate operands: an empty operand yields an
/// empty result without writing `out`; a full operand returns the other
/// side unchanged, its tree bytes copied into `out` (a ∩ full = a).
#[test]
fn test_bitmap_and_with_empty() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([10, 20].into_iter(), 64, &mut da);
    let b = Bitmap::empty(64);
    let db: Vec<u8> = Vec::new();
    let mut out = Vec::new();
    let c = a.and(&da, &b, &db, &mut out);
    assert!(c.is_empty(&out));

    let full = Bitmap::full(64);
    let full_data: Vec<u8> = Vec::new();
    let mut out2 = Vec::new();
    let c = a.and(&da, &full, &full_data, &mut out2);
    assert_eq!(c.len(&out2), 2);
    assert!(c.contains(&out2, 10));
    assert!(c.contains(&out2, 20));
}

/// `or` against a full operand short-circuits to `full` itself: `out`
/// stays empty, yet `len` reports the full 64 because an inverted
/// bitmap's len needs no stored bytes.
#[test]
fn test_bitmap_or_with_full() {
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([10, 20].into_iter(), 64, &mut da);
    let b = Bitmap::full(64);
    let db: Vec<u8> = Vec::new();
    let mut out = Vec::new();
    let c = a.or(&da, &b, &db, &mut out);
    assert_eq!(c.len(&out), 64);
}

/// One stored operand serves both operations: the same `b` tree bytes are
/// read as rhs of `and` and of `or` — operands are immutable inputs, each
/// call appends its result's tree bytes to a fresh caller-owned `out`.
#[test]
fn test_bitmap_assign_variants() {
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter([2, 3, 4].into_iter(), 64, &mut db);

    // a ∩ b = {2}
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([0, 1, 2].into_iter(), 64, &mut da);
    let mut out = Vec::new();
    let c = a.and(&da, &b, &db, &mut out);
    assert!(c.contains(&out, 2));
    assert!(!c.contains(&out, 0));

    // a ∪ b = {0,1,2,3,4}
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([0, 1, 2].into_iter(), 64, &mut da);
    let mut out = Vec::new();
    let c = a.or(&da, &b, &db, &mut out);
    for v in [0, 1, 2, 3, 4] {
        assert!(c.contains(&out, v), "missing {v}");
    }
}

/// All four `and` inverted-flag combinations on the same logical sets:
/// each row re-encodes `a_vals`/`b_vals` (directly, or as
/// complement-of-complement through the complement value list), and every
/// value in the universe must land in `a_vals ∩ b_vals`.
#[test]
fn test_bitmap_and_exhaustive() {
    let universe = 64u32;
    let a_vals: Vec<u32> = vec![0, 1, 7, 8, 32, 63];
    let b_vals: Vec<u32> = vec![1, 8, 32, 40, 63];
    let a_complement: Vec<u32> = (0..universe).filter(|v| !a_vals.contains(v)).collect();
    let b_complement: Vec<u32> = (0..universe).filter(|v| !b_vals.contains(v)).collect();

    let mut da_nn = Vec::new();
    let a_nn = Bitmap::from_sorted_iter(a_vals.iter().copied(), universe, &mut da_nn);
    let mut db_nn = Vec::new();
    let b_nn = Bitmap::from_sorted_iter(b_vals.iter().copied(), universe, &mut db_nn);

    let mut da_ni = Vec::new();
    let a_ni = Bitmap::from_sorted_iter(a_vals.iter().copied(), universe, &mut da_ni);
    let mut db_ni = Vec::new();
    let b_ni =
        Bitmap::from_sorted_iter_complemented(b_complement.iter().copied(), universe, &mut db_ni);

    let mut da_in = Vec::new();
    let a_in =
        Bitmap::from_sorted_iter_complemented(a_complement.iter().copied(), universe, &mut da_in);
    let mut db_in = Vec::new();
    let b_in = Bitmap::from_sorted_iter(b_vals.iter().copied(), universe, &mut db_in);

    let mut da_ii = Vec::new();
    let a_ii =
        Bitmap::from_sorted_iter_complemented(a_complement.iter().copied(), universe, &mut da_ii);
    let mut db_ii = Vec::new();
    let b_ii =
        Bitmap::from_sorted_iter_complemented(b_complement.iter().copied(), universe, &mut db_ii);

    let representations = [
        (&a_nn, &da_nn, &b_nn, &db_nn, "N&N"),
        (&a_ni, &da_ni, &b_ni, &db_ni, "N&I"),
        (&a_in, &da_in, &b_in, &db_in, "I&N"),
        (&a_ii, &da_ii, &b_ii, &db_ii, "I&I"),
    ];

    for (a, da, b, db, label) in &representations {
        let mut out = Vec::new();
        let c = a.and(da, b, db, &mut out);
        for v in 0..universe {
            let expected = a_vals.contains(&v) && b_vals.contains(&v);
            assert_eq!(c.contains(&out, v), expected, "{label}: value {v}");
        }
    }
}

/// The same four-representation matrix for `or`: every value in the
/// universe must land in `a_vals ∪ b_vals`, whichever way each operand is
/// stored.
#[test]
fn test_bitmap_or_exhaustive() {
    let universe = 64u32;
    let a_vals: Vec<u32> = vec![0, 1, 7, 8, 32, 63];
    let b_vals: Vec<u32> = vec![1, 8, 32, 40, 63];
    let a_complement: Vec<u32> = (0..universe).filter(|v| !a_vals.contains(v)).collect();
    let b_complement: Vec<u32> = (0..universe).filter(|v| !b_vals.contains(v)).collect();

    let mut da_nn = Vec::new();
    let a_nn = Bitmap::from_sorted_iter(a_vals.iter().copied(), universe, &mut da_nn);
    let mut db_nn = Vec::new();
    let b_nn = Bitmap::from_sorted_iter(b_vals.iter().copied(), universe, &mut db_nn);

    let mut da_ni = Vec::new();
    let a_ni = Bitmap::from_sorted_iter(a_vals.iter().copied(), universe, &mut da_ni);
    let mut db_ni = Vec::new();
    let b_ni =
        Bitmap::from_sorted_iter_complemented(b_complement.iter().copied(), universe, &mut db_ni);

    let mut da_in = Vec::new();
    let a_in =
        Bitmap::from_sorted_iter_complemented(a_complement.iter().copied(), universe, &mut da_in);
    let mut db_in = Vec::new();
    let b_in = Bitmap::from_sorted_iter(b_vals.iter().copied(), universe, &mut db_in);

    let mut da_ii = Vec::new();
    let a_ii =
        Bitmap::from_sorted_iter_complemented(a_complement.iter().copied(), universe, &mut da_ii);
    let mut db_ii = Vec::new();
    let b_ii =
        Bitmap::from_sorted_iter_complemented(b_complement.iter().copied(), universe, &mut db_ii);

    let representations = [
        (&a_nn, &da_nn, &b_nn, &db_nn, "N|N"),
        (&a_ni, &da_ni, &b_ni, &db_ni, "N|I"),
        (&a_in, &da_in, &b_in, &db_in, "I|N"),
        (&a_ii, &da_ii, &b_ii, &db_ii, "I|I"),
    ];

    for (a, da, b, db, label) in &representations {
        let mut out = Vec::new();
        let c = a.or(da, b, db, &mut out);
        for v in 0..universe {
            let expected = a_vals.contains(&v) || b_vals.contains(&v);
            assert_eq!(c.contains(&out, v), expected, "{label}: value {v}");
        }
    }
}

/// `and_not` (self ∩ ¬other) through the `and` dispatch: directly stored
/// rhs, complement-stored rhs (subtracting it keeps its cleared bits),
/// full \ N = ¬N, the identities a \ empty = a and a \ full = empty, and
/// a double `complement()` round-tripping to the original set.
#[test]
fn test_bitmap_and_not_all_flag_combinations() {
    let mut da = Vec::new();
    let mut db = Vec::new();
    let a = Bitmap::from_sorted_iter([1, 5, 10, 20].into_iter(), 64, &mut da);
    let b = Bitmap::from_sorted_iter([5, 20, 40].into_iter(), 64, &mut db);

    // N \ N
    let mut out = Vec::new();
    let c = a.and_not(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![1, 10]);

    // N \ I (b complemented: {5,20,40} are the CLEAR bits)
    let mut dbi = Vec::new();
    let bi = Bitmap::from_sorted_iter_complemented([5, 20, 40].into_iter(), 64, &mut dbi);
    let mut out = Vec::new();
    let c = a.and_not(&da, &bi, &dbi, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![5, 20]);

    // full \ N = complement of N
    let full = Bitmap::full(64);
    let mut out = Vec::new();
    let c = full.and_not(&[], &b, &db, &mut out);
    assert_eq!(c.len(&out), 61);
    assert!(!c.contains(&out, 5) && !c.contains(&out, 20) && !c.contains(&out, 40));
    assert!(c.contains(&out, 0) && c.contains(&out, 63));

    // A \ empty = A; A \ full = empty
    let empty = Bitmap::empty(64);
    let mut out = Vec::new();
    let c = a.and_not(&da, &empty, &[], &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![1, 5, 10, 20]);
    let mut out = Vec::new();
    let c = a.and_not(&da, &full, &[], &mut out);
    assert!(c.is_empty(&out));

    // complement round-trips
    assert_eq!(a.complement().complement().iter(&da).collect::<Vec<_>>(),
               a.iter(&da).collect::<Vec<_>>());
}
