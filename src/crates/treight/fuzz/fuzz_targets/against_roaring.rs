// Differential fuzz target: replays random op sequences on a
// `treight::RawBitmap` and a `roaring::RoaringBitmap` (the oracle) built
// from the same values, asserting the two implementations always agree on
// len, min/max, emptiness, membership, ranges, ascending iteration, set-op
// results and the serialize roundtrip.
//
// A treight bitmap is a `Copy` descriptor whose tree bytes live in an
// external blob (src/crates/treight/src/raw.rs `RawBitmap`), so each side
// is a (descriptor, data) pair and the blob is passed into every call.
//
// Run with `cargo fuzz run against_roaring` from src/crates/treight/fuzz
// (nightly pinned by src/crates/treight/fuzz/rust-toolchain.toml
// `channel`). The fuzz workspace pins the same netdata/roaring-rs fork as
// the main workspace so both sides test one roaring implementation
// (src/crates/treight/fuzz/Cargo.toml, the `roaring` dependency); the
// in-crate proptest twin of this file is
// src/crates/treight/src/tests_roaring.rs.
#![no_main]

use libfuzzer_sys::arbitrary::{self, Arbitrary, Unstructured};
use libfuzzer_sys::fuzz_target;
use std::mem;

// Universe sizes hitting the minimum and maximum value at every tree depth
// 1-8, per treight::ceil_log8 (src/crates/treight/src/lib.rs).
const UNIVERSES: [u32; 16] = [
    1,           // 1 level
    8,           // 1 level (max)
    9,           // 2 levels (min)
    64,          // 2 levels (max)
    65,          // 3 levels (min)
    512,         // 3 levels (max)
    513,         // 4 levels (min)
    4_096,       // 4 levels (max)
    4_097,       // 5 levels (min)
    32_768,      // 5 levels (max)
    32_769,      // 6 levels (min)
    262_144,     // 6 levels (max)
    262_145,     // 7 levels (min)
    2_097_152,   // 7 levels (max)
    2_097_153,   // 8 levels (min)
    16_777_216,  // 8 levels (max)
];

/// Value payload for the value-carrying ops, read straight from the fuzz
/// input bytes (the manual `Arbitrary` impl below is what a derive would
/// emit for this newtype).
#[derive(Debug, Copy, Clone)]
struct Num(u32);

impl<'a> Arbitrary<'a> for Num {
    fn arbitrary(u: &mut Unstructured<'a>) -> arbitrary::Result<Self> {
        Ok(Self(u.arbitrary()?))
    }
}

/// One op in the replayed sequence; see the `fuzz_target!` body for how
/// each maps onto treight and roaring calls.
#[derive(Arbitrary, Debug)]
enum Operation {
    Insert(Num),
    Remove(Num),
    Clear,
    Contains(Num),
    CheckLen,
    CheckMinMax,
    CheckIter,
    RangeCardinality(Num, Num),
    RemoveRange(Num, Num),
    And,
    Or,
    Sub,
    Xor,
    SwapSides,
    SerializeRoundtrip,
}

/// Generated input: which universe to use, the two starting value lists and
/// the op sequence.
#[derive(Arbitrary, Debug)]
struct FuzzInput {
    universe_idx: u8,
    initial_lhs: Vec<Num>,
    initial_rhs: Vec<Num>,
    ops: Vec<Operation>,
}

/// Assert the treight and roaring sides hold the same set: len, min, max,
/// emptiness and full ascending iteration must all agree.
fn check_equal(t: &treight::RawBitmap, td: &[u8], r: &roaring::RoaringBitmap) {
    assert_eq!(t.len(td), r.len(), "len mismatch: treight={} roaring={}", t.len(td), r.len());
    assert_eq!(t.min(td), r.min(), "min mismatch");
    assert_eq!(t.max(td), r.max(), "max mismatch");
    assert_eq!(t.is_empty(td), r.is_empty(), "is_empty mismatch");

    let t_vals: Vec<u32> = t.iter(td).collect();
    let r_vals: Vec<u32> = r.iter().collect();
    assert_eq!(t_vals, r_vals, "iter mismatch");
}

/// Build one side's (treight descriptor, tree data, roaring bitmap) triple
/// from the same values; callers must pass values in ascending order
/// (src/crates/treight/src/raw.rs `from_sorted_iter` corrupts the pre-order
/// layout on descending input and merely tolerates duplicates), deduped so
/// both sides hold identical sets.
fn make_pair(vals: &[u32], universe: u32) -> (treight::RawBitmap, Vec<u8>, roaring::RoaringBitmap) {
    let mut data = Vec::new();
    let t = treight::RawBitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data);
    let r = roaring::RoaringBitmap::from_sorted_iter(vals.iter().copied()).unwrap();
    (t, data, r)
}

fuzz_target!(|input: FuzzInput| {
    let universe = UNIVERSES[input.universe_idx as usize % UNIVERSES.len()];

    // Mask the seeded values into the universe, then sort and dedup:
    // `from_sorted_iter` needs ascending values on both sides
    // (src/crates/treight/src/raw.rs `from_sorted_iter`), and treight
    // insert/remove panic on out-of-universe values (same file, `insert`
    // and `remove`).
    let mut lhs_vals: Vec<u32> = input.initial_lhs.iter().map(|n| n.0 % universe).collect();
    lhs_vals.sort_unstable();
    lhs_vals.dedup();
    let (mut lhs_t, mut lhs_d, mut lhs_r) = make_pair(&lhs_vals, universe);

    let mut rhs_vals: Vec<u32> = input.initial_rhs.iter().map(|n| n.0 % universe).collect();
    rhs_vals.sort_unstable();
    rhs_vals.dedup();
    let (mut rhs_t, mut rhs_d, mut rhs_r) = make_pair(&rhs_vals, universe);

    check_equal(&lhs_t, &lhs_d, &lhs_r);
    check_equal(&rhs_t, &rhs_d, &rhs_r);

    // Replay the ops in lockstep: LHS treight and LHS roaring see the same
    // mutations, and every query op asserts the two answers agree. RHS is
    // the untouched operand for the boolean set ops; `SwapSides` exchanges
    // the two sides.
    for op in &input.ops {
        match *op {
            Operation::Insert(Num(n)) => {
                let v = n % universe;
                lhs_t.insert(&mut lhs_d, v);
                lhs_r.insert(v);
            }
            Operation::Remove(Num(n)) => {
                let v = n % universe;
                lhs_t.remove(&mut lhs_d, v);
                lhs_r.remove(v);
            }
            Operation::Clear => {
                // An empty blob IS an empty treight bitmap — the emptiness
                // check is just a blob check (src/crates/treight/src/raw.rs
                // `is_empty`).
                lhs_d.clear();
                lhs_r.clear();
            }
            Operation::Contains(Num(n)) => {
                let v = n % universe;
                assert_eq!(
                    lhs_t.contains(&lhs_d, v),
                    lhs_r.contains(v),
                    "contains({}) mismatch",
                    v
                );
            }
            Operation::CheckLen => {
                assert_eq!(lhs_t.len(&lhs_d), lhs_r.len(), "len mismatch");
            }
            Operation::CheckMinMax => {
                assert_eq!(lhs_t.min(&lhs_d), lhs_r.min(), "min mismatch");
                assert_eq!(lhs_t.max(&lhs_d), lhs_r.max(), "max mismatch");
            }
            Operation::CheckIter => {
                let t_vals: Vec<u32> = lhs_t.iter(&lhs_d).collect();
                let r_vals: Vec<u32> = lhs_r.iter().collect();
                assert_eq!(t_vals, r_vals, "iter mismatch");
            }
            Operation::RangeCardinality(Num(a), Num(b)) => {
                // Order the two random values into a half-open lo..hi range
                // inside the universe (saturating add keeps hi <= universe);
                // RemoveRange below derives the same bounds.
                let lo = a.min(b) % universe;
                let hi = (a.max(b) % universe).saturating_add(1);
                assert_eq!(
                    lhs_t.range_cardinality(&lhs_d, lo..hi),
                    lhs_r.range_cardinality(lo..hi),
                    "range_cardinality({}..{}) mismatch",
                    lo,
                    hi
                );
            }
            Operation::RemoveRange(Num(a), Num(b)) => {
                let lo = a.min(b) % universe;
                let hi = (a.max(b) % universe).saturating_add(1);
                lhs_t.remove_range(&mut lhs_d, lo..hi);
                lhs_r.remove_range(lo..hi);
            }
            Operation::And => {
                let mut out = Vec::new();
                lhs_t = lhs_t.intersect(&lhs_d, &rhs_t, &rhs_d, &mut out);
                lhs_d = out;
                lhs_r &= &rhs_r;
            }
            Operation::Or => {
                let mut out = Vec::new();
                lhs_t = lhs_t.union(&lhs_d, &rhs_t, &rhs_d, &mut out);
                lhs_d = out;
                lhs_r |= &rhs_r;
            }
            Operation::Sub => {
                let mut out = Vec::new();
                lhs_t = lhs_t.difference(&lhs_d, &rhs_t, &rhs_d, &mut out);
                lhs_d = out;
                lhs_r -= &rhs_r;
            }
            Operation::Xor => {
                let mut out = Vec::new();
                lhs_t = lhs_t.symmetric_difference(&lhs_d, &rhs_t, &rhs_d, &mut out);
                lhs_d = out;
                lhs_r ^= &rhs_r;
            }
            Operation::SwapSides => {
                mem::swap(&mut lhs_t, &mut rhs_t);
                mem::swap(&mut lhs_d, &mut rhs_d);
                mem::swap(&mut lhs_r, &mut rhs_r);
            }
            Operation::SerializeRoundtrip => {
                // Wire format is [universe_size: u32 LE][data_len: u32 LE]
                // [data bytes] (src/crates/treight/src/raw.rs
                // `serialize_into`); the restored pair must iterate
                // identically to the original.
                let mut buf = Vec::new();
                lhs_t.serialize_into(&lhs_d, &mut buf).unwrap();
                let (restored, restored_data) = treight::RawBitmap::deserialize_from(&buf[..]).unwrap();
                assert_eq!(
                    lhs_t.iter(&lhs_d).collect::<Vec<_>>(),
                    restored.iter(&restored_data).collect::<Vec<_>>(),
                    "serialize roundtrip mismatch"
                );
            }
        }
    }

    check_equal(&lhs_t, &lhs_d, &lhs_r);
    check_equal(&rhs_t, &rhs_d, &rhs_r);

    // The final LHS blob must still be exactly the size `estimate_data_size`
    // predicts for its contents: canonical storage, no dead bytes, whichever
    // mutation path built it (src/crates/treight/src/lib.rs
    // `estimate_data_size`).
    let lhs_final_vals: Vec<u32> = lhs_t.iter(&lhs_d).collect();
    let est = treight::estimate_data_size(universe, lhs_final_vals.iter().copied());
    assert_eq!(
        est,
        lhs_d.len(),
        "estimate_data_size mismatch for final LHS"
    );
});
