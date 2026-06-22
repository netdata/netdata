#![no_main]

use libfuzzer_sys::arbitrary::{self, Arbitrary, Unstructured};
use libfuzzer_sys::fuzz_target;
use std::mem;

// Interesting universe sizes that exercise different tree depths (1-8 levels).
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

#[derive(Debug, Copy, Clone)]
struct Num(u32);

impl<'a> Arbitrary<'a> for Num {
    fn arbitrary(u: &mut Unstructured<'a>) -> arbitrary::Result<Self> {
        Ok(Self(u.arbitrary()?))
    }
}

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

#[derive(Arbitrary, Debug)]
struct FuzzInput {
    universe_idx: u8,
    initial_lhs: Vec<Num>,
    initial_rhs: Vec<Num>,
    ops: Vec<Operation>,
}

/// Assert that a treight RawBitmap and a RoaringBitmap contain the same elements.
fn check_equal(t: &treight::RawBitmap, td: &[u8], r: &roaring::RoaringBitmap) {
    assert_eq!(t.len(td), r.len(), "len mismatch: treight={} roaring={}", t.len(td), r.len());
    assert_eq!(t.min(td), r.min(), "min mismatch");
    assert_eq!(t.max(td), r.max(), "max mismatch");
    assert_eq!(t.is_empty(td), r.is_empty(), "is_empty mismatch");

    let t_vals: Vec<u32> = t.iter(td).collect();
    let r_vals: Vec<u32> = r.iter().collect();
    assert_eq!(t_vals, r_vals, "iter mismatch");
}

/// Build a (treight, data, roaring) triple from sorted-deduped values.
fn make_pair(vals: &[u32], universe: u32) -> (treight::RawBitmap, Vec<u8>, roaring::RoaringBitmap) {
    let mut data = Vec::new();
    let t = treight::RawBitmap::from_sorted_iter(vals.iter().copied(), universe, &mut data);
    let r = roaring::RoaringBitmap::from_sorted_iter(vals.iter().copied()).unwrap();
    (t, data, r)
}

fuzz_target!(|input: FuzzInput| {
    let universe = UNIVERSES[input.universe_idx as usize % UNIVERSES.len()];

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

    let lhs_final_vals: Vec<u32> = lhs_t.iter(&lhs_d).collect();
    let est = treight::estimate_data_size(universe, lhs_final_vals.iter().copied());
    assert_eq!(
        est,
        lhs_d.len(),
        "estimate_data_size mismatch for final LHS"
    );
});
