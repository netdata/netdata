//! Differential tests for the roaring bridges ([`RawBitmap::from_roaring`]
//! /[`RawBitmap::to_roaring`], roaring.rs): every operation treight offers
//! is replayed on a `RoaringBitmap` built from the same values, and the
//! answers must match — the proptest complement of the fuzz target
//! (fuzz/fuzz_targets/against_roaring.rs), which runs the same oracle over
//! long random op sequences and tree depths up to 8 levels. These tests
//! are also the bridges' only callers in the tree. Compiled only under
//! `all(test, feature = "roaring")` (the module gate in `lib.rs`). By area:
//!
//! - Queries (`contains`/`iter`/`len`/`min`/`max`/`range_cardinality`),
//!   in-place `insert`/`remove`/`remove_range`, the serialize roundtrip
//!   and the four blob set ops equal roaring case for case, as proptests
//!   over generated sets.
//! - `estimate_data_size` equals the real appended blob length — the
//!   treight-vs-roaring size comparison input.
//! - Set-algebra identities hold on treight blobs directly:
//!   commutativity, empty identities, `a − a` = `a ⊕ a` = ∅,
//!   intersection ⊆ union.
//! - The inverted-flag `Bitmap` wrapper's `and`/`or` match a membership
//!   oracle across all flag combinations (the per-row unit pins live in
//!   tests_bitmap.rs).
//! - The bridges themselves (the `test_rawbitmap_from_roaring*` /
//!   `test_roaring_*` unit tests and the `roaring_roundtrip`
//!   proptest): explicit universe adoption,
//!   empty in both directions, exact treight↔roaring roundtrip.
//!
//! Universes stay ≤ `MAX_UNIVERSE` so each generated case is checked
//! exhaustively over all values. Not pinned here: out-of-range values in
//! a `from_roaring` input — there is no bounds check (see roaring.rs),
//! keeping `rb` within `0..universe_size` is on the caller.
use crate::*;
use ::roaring::RoaringBitmap;
use proptest::prelude::*;

/// Maximum universe size for property tests. Kept small enough that
/// exhaustive membership checks (0..universe) are fast.
const MAX_UNIVERSE: u32 = 4096;

/// Oracle harness strategy: a (universe_size, values) pair, the universe
/// in 1..=MAX_UNIVERSE and up to 256 values drawn from 0..universe,
/// sorted and deduped so `from_sorted_iter`'s ascending contract holds.
fn arb_bitmap() -> impl Strategy<Value = (u32, Vec<u32>)> {
    (1u32..=MAX_UNIVERSE).prop_flat_map(|universe| {
        proptest::collection::vec(0..universe, 0..=(universe.min(256) as usize)).prop_map(
            move |mut vals| {
                vals.sort_unstable();
                vals.dedup();
                (universe, vals)
            },
        )
    })
}

/// Build the comparison pair for `vals`: a treight (descriptor, blob) and
/// a `RoaringBitmap`, same values under the same caller-chosen universe.
fn make_pair(universe: u32, vals: &[u32]) -> (RawBitmap, Vec<u8>, RoaringBitmap) {
    let mut data = Vec::new();
    let raw = RawBitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data);
    let roaring = RoaringBitmap::from_sorted_iter(vals.iter().copied()).unwrap();
    (raw, data, roaring)
}

/// Build a treight (descriptor, blob) from arbitrary values: sorted and
/// deduped first, so the input is always ascending.
fn make_bitmap(universe_size: u32, values: &[u32]) -> (RawBitmap, Vec<u8>) {
    let mut sorted = values.to_vec();
    sorted.sort_unstable();
    sorted.dedup();
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter(sorted.into_iter(), universe_size, &mut data);
    (bm, data)
}

// ===== Construction & queries =====

proptest! {
    /// Point membership matches roaring for every value in the
    /// universe, not only the stored ones.
    #[test]
    fn contains_matches_roaring((universe, vals) in arb_bitmap()) {
        let (raw, data, roaring) = make_pair(universe, &vals);
        for v in 0..universe {
            prop_assert_eq!(
                raw.contains(&data, v),
                roaring.contains(v),
                "contains({}) mismatch, universe={}", v, universe
            );
        }
    }

    /// Full iteration yields roaring's exact element sequence — the
    /// strictly-ascending order the `to_roaring` bridge relies on.
    #[test]
    fn iter_matches_roaring((universe, vals) in arb_bitmap()) {
        let (raw, data, roaring) = make_pair(universe, &vals);
        let raw_vals: Vec<u32> = raw.iter(&data).collect();
        let roaring_vals: Vec<u32> = roaring.iter().collect();
        prop_assert_eq!(raw_vals, roaring_vals);
    }

    /// Cardinality matches roaring, empty sets included.
    #[test]
    fn len_matches_roaring((universe, vals) in arb_bitmap()) {
        let (raw, data, roaring) = make_pair(universe, &vals);
        prop_assert_eq!(raw.len(&data), roaring.len());
    }

    /// min/max match roaring, including the empty case (both `None`).
    #[test]
    fn min_max_match_roaring((universe, vals) in arb_bitmap()) {
        let (raw, data, roaring) = make_pair(universe, &vals);
        prop_assert_eq!(raw.min(&data), roaring.min());
        prop_assert_eq!(raw.max(&data), roaring.max());
    }

    /// `estimate_data_size` predicts the exact blob length a real build
    /// appends (its documented contract in lib.rs); the fuzz target
    /// re-asserts it after op sequences
    /// (`fuzz/fuzz_targets/against_roaring.rs`).
    #[test]
    fn estimate_data_size_is_exact((universe, vals) in arb_bitmap()) {
        let est = estimate_data_size(universe, vals.iter().copied());
        let mut data = Vec::new();
        let _raw = RawBitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data);
        prop_assert_eq!(est, data.len());
    }
}

// ===== Insert / remove =====

proptest! {
    /// In-place `insert` matches roaring across a round of extra values
    /// (folded into 0..universe, possibly already present), compared by
    /// full iteration.
    #[test]
    fn insert_matches_roaring(
        (universe, vals) in arb_bitmap(),
        extra in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..32),
    ) {
        let (raw, mut data, mut roaring) = make_pair(universe, &vals);
        for v in extra {
            let v = v % universe;
            raw.insert(&mut data, v);
            roaring.insert(v);
        }

        let raw_vals: Vec<u32> = raw.iter(&data).collect();
        let roaring_vals: Vec<u32> = roaring.iter().collect();
        prop_assert_eq!(raw_vals, roaring_vals);
    }

    /// In-place `remove` matches roaring, including removals of absent
    /// values, compared by full iteration.
    #[test]
    fn remove_matches_roaring(
        (universe, vals) in arb_bitmap(),
        to_remove in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..32),
    ) {
        let (raw, mut data, mut roaring) = make_pair(universe, &vals);
        for v in to_remove {
            let v = v % universe;
            raw.remove(&mut data, v);
            roaring.remove(v);
        }

        let raw_vals: Vec<u32> = raw.iter(&data).collect();
        let roaring_vals: Vec<u32> = roaring.iter().collect();
        prop_assert_eq!(raw_vals, roaring_vals);
    }
}

// ===== Set operations =====

proptest! {
    /// `union` equals roaring's `|`: two generated operands sharing the
    /// universe, results compared by iterating the output blob.
    #[test]
    fn union_matches_roaring(
        (universe, a_vals) in arb_bitmap(),
        b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=256usize),
    ) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };

        let (raw_a, da, roaring_a) = make_pair(universe, &a_vals);
        let (raw_b, db, roaring_b) = make_pair(universe, &b_vals);

        let mut out = Vec::new();
        let raw_result = raw_a.union(&da, &raw_b, &db, &mut out);
        let roaring_result = &roaring_a | &roaring_b;

        let raw_out: Vec<u32> = raw_result.iter(&out).collect();
        let roaring_out: Vec<u32> = roaring_result.iter().collect();
        prop_assert_eq!(raw_out, roaring_out);
    }

    /// `intersect` equals roaring's `&`, same setup as the union test.
    #[test]
    fn intersection_matches_roaring(
        (universe, a_vals) in arb_bitmap(),
        b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=256usize),
    ) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };

        let (raw_a, da, roaring_a) = make_pair(universe, &a_vals);
        let (raw_b, db, roaring_b) = make_pair(universe, &b_vals);

        let mut out = Vec::new();
        let raw_result = raw_a.intersect(&da, &raw_b, &db, &mut out);
        let roaring_result = &roaring_a & &roaring_b;

        let raw_out: Vec<u32> = raw_result.iter(&out).collect();
        let roaring_out: Vec<u32> = roaring_result.iter().collect();
        prop_assert_eq!(raw_out, roaring_out);
    }

    /// `difference` equals roaring's `-`, same setup as the union test.
    #[test]
    fn difference_matches_roaring(
        (universe, a_vals) in arb_bitmap(),
        b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=256usize),
    ) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };

        let (raw_a, da, roaring_a) = make_pair(universe, &a_vals);
        let (raw_b, db, roaring_b) = make_pair(universe, &b_vals);

        let mut out = Vec::new();
        let raw_result = raw_a.difference(&da, &raw_b, &db, &mut out);
        let roaring_result = &roaring_a - &roaring_b;

        let raw_out: Vec<u32> = raw_result.iter(&out).collect();
        let roaring_out: Vec<u32> = roaring_result.iter().collect();
        prop_assert_eq!(raw_out, roaring_out);
    }

    /// `symmetric_difference` equals roaring's `^`, same setup as the
    /// union test.
    #[test]
    fn symmetric_difference_matches_roaring(
        (universe, a_vals) in arb_bitmap(),
        b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=256usize),
    ) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };

        let (raw_a, da, roaring_a) = make_pair(universe, &a_vals);
        let (raw_b, db, roaring_b) = make_pair(universe, &b_vals);

        let mut out = Vec::new();
        let raw_result = raw_a.symmetric_difference(&da, &raw_b, &db, &mut out);
        let roaring_result = &roaring_a ^ &roaring_b;

        let raw_out: Vec<u32> = raw_result.iter(&out).collect();
        let roaring_out: Vec<u32> = roaring_result.iter().collect();
        prop_assert_eq!(raw_out, roaring_out);
    }
}

// ===== Algebraic set properties =====

proptest! {
    /// a ∪ b and b ∪ a produce identical blobs — treight-internal
    /// algebra, no oracle involved.
    #[test]
    fn union_is_commutative((universe, a_vals) in arb_bitmap(), b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=128usize)) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };
        let (a, da, _) = make_pair(universe, &a_vals);
        let (b, db, _) = make_pair(universe, &b_vals);
        let mut out1 = Vec::new();
        let c1 = a.union(&da, &b, &db, &mut out1);
        let mut out2 = Vec::new();
        let c2 = b.union(&db, &a, &da, &mut out2);
        prop_assert_eq!(c1.iter(&out1).collect::<Vec<_>>(), c2.iter(&out2).collect::<Vec<_>>());
    }

    /// a ∩ b and b ∩ a produce identical blobs.
    #[test]
    fn intersection_is_commutative((universe, a_vals) in arb_bitmap(), b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=128usize)) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };
        let (a, da, _) = make_pair(universe, &a_vals);
        let (b, db, _) = make_pair(universe, &b_vals);
        let mut out1 = Vec::new();
        let c1 = a.intersect(&da, &b, &db, &mut out1);
        let mut out2 = Vec::new();
        let c2 = b.intersect(&db, &a, &da, &mut out2);
        prop_assert_eq!(c1.iter(&out1).collect::<Vec<_>>(), c2.iter(&out2).collect::<Vec<_>>());
    }

    /// a ⊕ b and b ⊕ a produce identical blobs.
    #[test]
    fn xor_is_commutative((universe, a_vals) in arb_bitmap(), b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=128usize)) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };
        let (a, da, _) = make_pair(universe, &a_vals);
        let (b, db, _) = make_pair(universe, &b_vals);
        let mut out1 = Vec::new();
        let c1 = a.symmetric_difference(&da, &b, &db, &mut out1);
        let mut out2 = Vec::new();
        let c2 = b.symmetric_difference(&db, &a, &da, &mut out2);
        prop_assert_eq!(c1.iter(&out1).collect::<Vec<_>>(), c2.iter(&out2).collect::<Vec<_>>());
    }

    /// Union with `empty(universe)` over a zero-byte blob is the
    /// identity in both operand orders.
    #[test]
    fn union_with_empty_is_identity((universe, vals) in arb_bitmap()) {
        let (a, da, _) = make_pair(universe, &vals);
        let empty = RawBitmap::empty(universe);
        let mut out1 = Vec::new();
        let c1 = a.union(&da, &empty, &[], &mut out1);
        prop_assert_eq!(c1.iter(&out1).collect::<Vec<_>>(), a.iter(&da).collect::<Vec<_>>());
        let mut out2 = Vec::new();
        let c2 = empty.union(&[], &a, &da, &mut out2);
        prop_assert_eq!(c2.iter(&out2).collect::<Vec<_>>(), a.iter(&da).collect::<Vec<_>>());
    }

    /// Intersecting with the empty bitmap yields an empty result, in
    /// both operand orders.
    #[test]
    fn intersection_with_empty_is_empty((universe, vals) in arb_bitmap()) {
        let (a, da, _) = make_pair(universe, &vals);
        let empty = RawBitmap::empty(universe);
        let mut out = Vec::new();
        prop_assert!(a.intersect(&da, &empty, &[], &mut out).is_empty(&out));
        out.clear();
        prop_assert!(empty.intersect(&[], &a, &da, &mut out).is_empty(&out));
    }

    /// a − a is empty.
    #[test]
    fn difference_with_self_is_empty((universe, vals) in arb_bitmap()) {
        let (a, da, _) = make_pair(universe, &vals);
        let mut out = Vec::new();
        prop_assert!(a.difference(&da, &a, &da, &mut out).is_empty(&out));
    }

    /// a ⊕ a is empty.
    #[test]
    fn xor_with_self_is_empty((universe, vals) in arb_bitmap()) {
        let (a, da, _) = make_pair(universe, &vals);
        let mut out = Vec::new();
        prop_assert!(a.symmetric_difference(&da, &a, &da, &mut out).is_empty(&out));
    }

    /// Every intersection member lies in the union, and
    /// inter.len() <= union.len().
    #[test]
    fn intersection_is_subset_of_union(
        (universe, a_vals) in arb_bitmap(),
        b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=128usize),
    ) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };
        let (a, da, _) = make_pair(universe, &a_vals);
        let (b, db, _) = make_pair(universe, &b_vals);

        let mut inter_out = Vec::new();
        let inter = a.intersect(&da, &b, &db, &mut inter_out);
        let mut union_out = Vec::new();
        let union = a.union(&da, &b, &db, &mut union_out);

        for v in inter.iter(&inter_out) {
            prop_assert!(union.contains(&union_out, v), "intersection element {} not in union", v);
        }
        prop_assert!(inter.len(&inter_out) <= union.len(&union_out));
    }
}

// ===== Bitmap (De Morgan wrapper) property tests =====

/// Extends `arb_bitmap` with a random inverted flag, driving the
/// complement-storage dimension of the `Bitmap` wrapper tests.
fn arb_bitmap_wrapper() -> impl Strategy<Value = (u32, Vec<u32>, bool)> {
    arb_bitmap().prop_flat_map(|(universe, vals)| {
        proptest::bool::ANY.prop_map(move |inv| (universe, vals.clone(), inv))
    })
}

/// Build a `Bitmap` from vals, stored as the complement when `inverted`.
fn make_bitmap_wrapper(universe: u32, vals: &[u32], inverted: bool) -> (Bitmap, Vec<u8>) {
    let mut data = Vec::new();
    let bm = if inverted {
        Bitmap::from_sorted_iter_complemented(vals.iter().copied(), universe, &mut data)
    } else {
        Bitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data)
    };
    (bm, data)
}

proptest! {
    /// Wrapper membership is the oracle's, complemented when inverted:
    /// checked for every value in the universe.
    #[test]
    fn bitmap_contains_matches_roaring(
        (universe, vals, inverted) in arb_bitmap_wrapper(),
    ) {
        let (bm, data) = make_bitmap_wrapper(universe, &vals, inverted);

        let stored = RoaringBitmap::from_sorted_iter(vals.iter().copied()).unwrap();

        for v in 0..universe {
            let in_stored = stored.contains(v);
            let expected = if inverted { !in_stored } else { in_stored };
            prop_assert_eq!(
                bm.contains(&data, v), expected,
                "Bitmap.contains({}) wrong, inverted={}", v, inverted
            );
        }
    }

    /// Wrapper len is stored.len(), or universe − stored.len() when
    /// complement-stored.
    #[test]
    fn bitmap_len_matches_roaring(
        (universe, vals, inverted) in arb_bitmap_wrapper(),
    ) {
        let (bm, data) = make_bitmap_wrapper(universe, &vals, inverted);
        let stored = RoaringBitmap::from_sorted_iter(vals.iter().copied()).unwrap();
        let expected = if inverted {
            universe as u64 - stored.len()
        } else {
            stored.len()
        };
        prop_assert_eq!(bm.len(&data), expected);
    }

    /// `and` across all four inverted-flag combinations equals the
    /// per-value logical AND of the two oracle memberships.
    #[test]
    fn bitmap_and_matches_oracle(
        (universe, a_vals) in arb_bitmap(),
        b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=128usize),
        a_inv in proptest::bool::ANY,
        b_inv in proptest::bool::ANY,
    ) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };

        let (bm_a, da) = make_bitmap_wrapper(universe, &a_vals, a_inv);
        let (bm_b, db) = make_bitmap_wrapper(universe, &b_vals, b_inv);
        let mut out = Vec::new();
        let result = bm_a.and(&da, &bm_b, &db, &mut out);

        let stored_a = RoaringBitmap::from_sorted_iter(a_vals.iter().copied()).unwrap();
        let stored_b = RoaringBitmap::from_sorted_iter(b_vals.iter().copied()).unwrap();

        for v in 0..universe {
            let a_has = if a_inv { !stored_a.contains(v) } else { stored_a.contains(v) };
            let b_has = if b_inv { !stored_b.contains(v) } else { stored_b.contains(v) };
            let expected = a_has && b_has;
            prop_assert_eq!(
                result.contains(&out, v), expected,
                "AND({}): a_inv={} b_inv={}", v, a_inv, b_inv
            );
        }
    }

    /// `or` across all four inverted-flag combinations equals the
    /// per-value logical OR of the two oracle memberships.
    #[test]
    fn bitmap_or_matches_oracle(
        (universe, a_vals) in arb_bitmap(),
        b_frac in proptest::collection::vec(0u32..MAX_UNIVERSE, 0..=128usize),
        a_inv in proptest::bool::ANY,
        b_inv in proptest::bool::ANY,
    ) {
        let b_vals = {
            let mut v: Vec<u32> = b_frac.into_iter().map(|x| x % universe).collect();
            v.sort_unstable();
            v.dedup();
            v
        };

        let (bm_a, da) = make_bitmap_wrapper(universe, &a_vals, a_inv);
        let (bm_b, db) = make_bitmap_wrapper(universe, &b_vals, b_inv);
        let mut out = Vec::new();
        let result = bm_a.or(&da, &bm_b, &db, &mut out);

        let stored_a = RoaringBitmap::from_sorted_iter(a_vals.iter().copied()).unwrap();
        let stored_b = RoaringBitmap::from_sorted_iter(b_vals.iter().copied()).unwrap();

        for v in 0..universe {
            let a_has = if a_inv { !stored_a.contains(v) } else { stored_a.contains(v) };
            let b_has = if b_inv { !stored_b.contains(v) } else { stored_b.contains(v) };
            let expected = a_has || b_has;
            prop_assert_eq!(
                result.contains(&out, v), expected,
                "OR({}): a_inv={} b_inv={}", v, a_inv, b_inv
            );
        }
    }
}

// ===== Serialization, roaring roundtrip & range ops =====

proptest! {
    /// The serialize wire format round-trips: `deserialize_from` of a
    /// `serialize_into` buffer reproduces the exact value sequence
    /// (same wire-format pin as tests_raw.rs, no roaring involved).
    #[test]
    fn serialize_roundtrip((universe, vals) in arb_bitmap()) {
        let mut data = Vec::new();
        let raw = RawBitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data);
        let mut buf = Vec::new();
        raw.serialize_into(&data, &mut buf).unwrap();
        let (raw2, data2) = RawBitmap::deserialize_from(&buf[..]).unwrap();
        prop_assert_eq!(raw.iter(&data).collect::<Vec<_>>(), raw2.iter(&data2).collect::<Vec<_>>());
    }

    /// treight → roaring → treight preserves the value sequence: the
    /// `to_roaring`/`from_roaring` pair under the same universe.
    #[test]
    fn roaring_roundtrip((universe, vals) in arb_bitmap()) {
        let mut data = Vec::new();
        let raw = RawBitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data);
        let rb = raw.to_roaring(&data);
        let mut data2 = Vec::new();
        let raw2 = RawBitmap::from_roaring(&rb, universe, &mut data2);
        prop_assert_eq!(raw.iter(&data).collect::<Vec<_>>(), raw2.iter(&data2).collect::<Vec<_>>());
    }

    /// range_cardinality matches roaring over a random in-universe
    /// half-open range (ends folded into range, then ordered).
    #[test]
    fn range_cardinality_matches_roaring(
        (universe, vals) in arb_bitmap(),
        range_start in 0u32..MAX_UNIVERSE,
        range_end in 0u32..MAX_UNIVERSE,
    ) {
        let lo = range_start.min(range_end) % universe;
        let hi = (range_start.max(range_end) % universe).saturating_add(1);
        let range = lo..hi;

        let (raw, data, roaring) = make_pair(universe, &vals);
        prop_assert_eq!(
            raw.range_cardinality(&data, range.clone()),
            roaring.range_cardinality(range)
        );
    }

    /// The unbounded range counts everything:
    /// range_cardinality(..) equals len.
    #[test]
    fn range_cardinality_full_equals_len((universe, vals) in arb_bitmap()) {
        let mut data = Vec::new();
        let raw = RawBitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data);
        prop_assert_eq!(raw.range_cardinality(&data, ..), raw.len(&data));
    }

    /// In-place `remove_range` matches roaring over the same folded,
    /// ordered range construction as above.
    #[test]
    fn remove_range_matches_roaring(
        (universe, vals) in arb_bitmap(),
        range_start in 0u32..MAX_UNIVERSE,
        range_end in 0u32..MAX_UNIVERSE,
    ) {
        let lo = range_start.min(range_end) % universe;
        let hi = (range_start.max(range_end) % universe).saturating_add(1);

        let (raw, mut data, mut roaring) = make_pair(universe, &vals);
        raw.remove_range(&mut data, lo..hi);
        roaring.remove_range(lo..hi);

        let raw_vals: Vec<u32> = raw.iter(&data).collect();
        let roaring_vals: Vec<u32> = roaring.iter().collect();
        prop_assert_eq!(raw_vals, roaring_vals);
    }
}

// ===== Roaring conversion unit tests =====

/// `from_roaring` adopts the caller-chosen universe (512) and reproduces
/// the rb's membership: {0, 42, 511} present, len 3.
#[test]
fn test_rawbitmap_from_roaring() {
    let mut rb = RoaringBitmap::new();
    rb.insert(0);
    rb.insert(42);
    rb.insert(511);

    let mut data = Vec::new();
    let bm = RawBitmap::from_roaring(&rb, 512, &mut data);
    assert_eq!(bm.universe_size(), 512);
    assert!(bm.contains(&data, 0));
    assert!(bm.contains(&data, 42));
    assert!(bm.contains(&data, 511));
    assert!(!bm.contains(&data, 1));
    assert_eq!(bm.len(&data), 3);
}

/// An empty rb converts to the empty descriptor while still adopting the
/// requested universe — nothing is appended to `data`.
#[test]
fn test_rawbitmap_from_roaring_empty() {
    let rb = RoaringBitmap::new();

    let mut data = Vec::new();
    let bm = RawBitmap::from_roaring(&rb, 64, &mut data);
    assert!(bm.is_empty(&data));
    assert_eq!(bm.universe_size(), 64);
}

/// `to_roaring` carries the full member set ({0, 42, 255, 511}) across
/// to a `RoaringBitmap`.
#[test]
fn test_roaring_from_rawbitmap() {
    let (bm, data) = make_bitmap(512, &[0, 42, 255, 511]);
    let rb = bm.to_roaring(&data);
    assert_eq!(rb.len(), 4);
    assert!(rb.contains(0));
    assert!(rb.contains(42));
    assert!(rb.contains(255));
    assert!(rb.contains(511));
}

/// An empty treight blob converts to an empty `RoaringBitmap`.
#[test]
fn test_roaring_from_rawbitmap_empty() {
    let bm = RawBitmap::empty(64);
    let rb = bm.to_roaring(&[]);
    assert!(rb.is_empty());
}

/// The full bridge roundtrip is exact on 8-bit boundary values
/// (0/1/7/8/63/64/255/256/511): `from_roaring` then `to_roaring` yields
/// an equal `RoaringBitmap`.
#[test]
fn test_roaring_roundtrip() {
    let values = [0, 1, 7, 8, 63, 64, 255, 256, 511];
    let mut rb = RoaringBitmap::new();
    for &v in &values {
        rb.insert(v);
    }

    let mut data = Vec::new();
    let bm = RawBitmap::from_roaring(&rb, 512, &mut data);
    let rb2 = bm.to_roaring(&data);
    assert_eq!(rb, rb2);
}
