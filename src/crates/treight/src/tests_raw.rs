use crate::*;

#[test]
fn test_ceil_log8() {
    assert_eq!(ceil_log8(0), 0);
    assert_eq!(ceil_log8(1), 1);
    assert_eq!(ceil_log8(8), 1);
    assert_eq!(ceil_log8(9), 2);
    assert_eq!(ceil_log8(64), 2);
    assert_eq!(ceil_log8(65), 3);
    assert_eq!(ceil_log8(512), 3);
    assert_eq!(ceil_log8(513), 4);
}

#[test]
fn test_empty_bitmap() {
    let bm = RawBitmap::empty(0);
    assert_eq!(bm.universe_size(), 0);
    assert_eq!(bm.levels(), 0);

    let bm = RawBitmap::empty(1);
    assert_eq!(bm.universe_size(), 1);
    assert_eq!(bm.levels(), 1);

    let bm = RawBitmap::empty(64);
    assert_eq!(bm.universe_size(), 64);
    assert_eq!(bm.levels(), 2);

    let bm = RawBitmap::empty(512);
    assert_eq!(bm.universe_size(), 512);
    assert_eq!(bm.levels(), 3);
}

#[test]
fn test_build_universe8_bit0() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter([0].into_iter(), 8, &mut data);
    assert_eq!(&data, &[0x01]);
    let _ = bm;
}

#[test]
fn test_build_universe8_bit7() {
    let mut data = Vec::new();
    RawBitmap::from_sorted_iter([7].into_iter(), 8, &mut data);
    assert_eq!(&data, &[0x80]);
}

#[test]
fn test_build_universe8_all() {
    let mut data = Vec::new();
    RawBitmap::from_sorted_iter(0..8, 8, &mut data);
    assert_eq!(&data, &[0xFF]);
}

#[test]
fn test_build_universe16_insert_0_8() {
    let mut data = Vec::new();
    RawBitmap::from_sorted_iter([0, 8].into_iter(), 16, &mut data);
    assert_eq!(&data, &[0x03, 0x01, 0x01]);
}

#[test]
fn test_build_universe64_insert_63() {
    let mut data = Vec::new();
    RawBitmap::from_sorted_iter([63].into_iter(), 64, &mut data);
    assert_eq!(&data, &[0x80, 0x80]);
}

#[test]
fn test_build_universe9_insert_8() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter([8].into_iter(), 9, &mut data);
    assert_eq!(bm.levels(), 2);
    assert_eq!(&data, &[0x02, 0x01]);
}

#[test]
fn test_contains_universe8() {
    let (bm, data) = make_bitmap(8, &[0, 3, 7]);
    for i in 0..8 {
        assert_eq!(
            bm.contains(&data, i),
            i == 0 || i == 3 || i == 7,
            "value={i}"
        );
    }
}

#[test]
fn test_contains_universe16() {
    let (bm, data) = make_bitmap(16, &[0, 8]);
    for i in 0..16 {
        assert_eq!(bm.contains(&data, i), i == 0 || i == 8, "value={i}");
    }
}

#[test]
fn test_contains_universe64_sparse() {
    let (bm, data) = make_bitmap(64, &[0, 31, 63]);
    for i in 0..64 {
        assert_eq!(
            bm.contains(&data, i),
            i == 0 || i == 31 || i == 63,
            "value={i}"
        );
    }
}

#[test]
fn test_contains_universe64_dense() {
    let values: Vec<u32> = (0..64).collect();
    let (bm, data) = make_bitmap(64, &values);
    for i in 0..64 {
        assert!(bm.contains(&data, i), "value={i}");
    }
    assert!(!bm.contains(&data, 64));
}

#[test]
fn test_contains_empty() {
    let bm = RawBitmap::empty(64);
    let data: Vec<u8> = Vec::new();
    for i in 0..64 {
        assert!(!bm.contains(&data, i));
    }
}

#[test]
fn test_contains_universe9_insert_8() {
    let (bm, data) = make_bitmap(9, &[8]);
    for i in 0..9 {
        assert_eq!(bm.contains(&data, i), i == 8, "value={i}");
    }
}

#[test]
fn test_contains_universe512() {
    let values = [0, 1, 7, 8, 63, 64, 255, 256, 511];
    let (bm, data) = make_bitmap(512, &values);
    assert_eq!(bm.levels(), 3);
    for i in 0..512 {
        assert_eq!(bm.contains(&data, i), values.contains(&i), "value={i}");
    }
}

#[test]
fn test_iter_ascending_order() {
    let (bm, data) = make_bitmap(512, &[511, 0, 255, 1, 63, 64, 8, 7, 256]);
    let result: Vec<u32> = bm.iter(&data).collect();
    assert_eq!(result, vec![0, 1, 7, 8, 63, 64, 255, 256, 511]);
}

#[test]
fn test_iter_empty() {
    let bm = RawBitmap::empty(64);
    let data: Vec<u8> = Vec::new();
    let result: Vec<u32> = bm.iter(&data).collect();
    assert!(result.is_empty());
}

#[test]
fn test_iter_single_element() {
    let (bm, data) = make_bitmap(64, &[42]);
    let result: Vec<u32> = bm.iter(&data).collect();
    assert_eq!(result, vec![42]);
}

#[test]
fn test_iter_full() {
    let (bm, data) = make_bitmap(8, &[0, 1, 2, 3, 4, 5, 6, 7]);
    let result: Vec<u32> = bm.iter(&data).collect();
    assert_eq!(result, vec![0, 1, 2, 3, 4, 5, 6, 7]);
}

#[test]
fn test_len() {
    let bm = RawBitmap::empty(64);
    assert_eq!(bm.len(&[]), 0);

    let (bm, data) = make_bitmap(512, &[0, 1, 7, 8, 63, 64, 255, 256, 511]);
    assert_eq!(bm.len(&data), 9);
}

#[test]
fn test_is_empty() {
    assert!(RawBitmap::empty(64).is_empty(&[]));
    let (bm, data) = make_bitmap(8, &[0]);
    assert!(!bm.is_empty(&data));
}

#[test]
fn test_min_max() {
    let bm = RawBitmap::empty(64);
    assert_eq!(bm.min(&[]), None);
    assert_eq!(bm.max(&[]), None);

    let (bm, data) = make_bitmap(512, &[42]);
    assert_eq!(bm.min(&data), Some(42));
    assert_eq!(bm.max(&data), Some(42));

    let (bm, data) = make_bitmap(512, &[0, 255, 511]);
    assert_eq!(bm.min(&data), Some(0));
    assert_eq!(bm.max(&data), Some(511));
}

#[test]
fn test_min_max_universe8() {
    let (bm, data) = make_bitmap(8, &[3, 5]);
    assert_eq!(bm.min(&data), Some(3));
    assert_eq!(bm.max(&data), Some(5));
}

fn make_bitmap(universe_size: u32, values: &[u32]) -> (RawBitmap, Vec<u8>) {
    let mut sorted = values.to_vec();
    sorted.sort_unstable();
    sorted.dedup();
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter(sorted.into_iter(), universe_size, &mut data);
    (bm, data)
}

/// Helper to compare two bitmaps by iterating their values.
fn bitmaps_equal(a: &RawBitmap, ad: &[u8], b: &RawBitmap, bd: &[u8]) -> bool {
    a.iter(ad).collect::<Vec<_>>() == b.iter(bd).collect::<Vec<_>>()
}

#[test]
fn test_union_disjoint() {
    let (a, da) = make_bitmap(64, &[0, 1, 2]);
    let (b, db) = make_bitmap(64, &[60, 61, 62]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    let result: Vec<u32> = c.iter(&out).collect();
    assert_eq!(result, vec![0, 1, 2, 60, 61, 62]);
}

#[test]
fn test_union_overlapping() {
    let (a, da) = make_bitmap(64, &[0, 1, 2, 3]);
    let (b, db) = make_bitmap(64, &[2, 3, 4, 5]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    let result: Vec<u32> = c.iter(&out).collect();
    assert_eq!(result, vec![0, 1, 2, 3, 4, 5]);
}

#[test]
fn test_union_empty() {
    let (a, da) = make_bitmap(64, &[1, 2, 3]);
    let b = RawBitmap::empty(64);
    let db: Vec<u8> = Vec::new();
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    assert!(bitmaps_equal(&c, &out, &a, &da));
}

#[test]
fn test_union_single_level() {
    let (a, da) = make_bitmap(8, &[0, 2, 4]);
    let (b, db) = make_bitmap(8, &[1, 3, 5]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0, 1, 2, 3, 4, 5]);
}

#[test]
fn test_union_single_level_overlapping() {
    let (a, da) = make_bitmap(8, &[0, 1, 2]);
    let (b, db) = make_bitmap(8, &[1, 2, 3]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0, 1, 2, 3]);
}

#[test]
fn test_union_one_empty_both_directions() {
    let (a, da) = make_bitmap(512, &[0, 100, 511]);
    let b = RawBitmap::empty(512);
    let db: Vec<u8> = Vec::new();
    let mut out1 = Vec::new();
    let c1 = a.union(&da, &b, &db, &mut out1);
    assert!(bitmaps_equal(&c1, &out1, &a, &da));
    let mut out2 = Vec::new();
    let c2 = b.union(&db, &a, &da, &mut out2);
    assert!(bitmaps_equal(&c2, &out2, &a, &da));
}

#[test]
fn test_union_both_empty() {
    let a = RawBitmap::empty(64);
    let b = RawBitmap::empty(64);
    let mut out = Vec::new();
    let c = a.union(&[], &b, &[], &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_union_with_self() {
    let (a, da) = make_bitmap(512, &[0, 7, 42, 100, 255, 511]);
    let mut out = Vec::new();
    let c = a.union(&da, &a, &da, &mut out);
    assert!(bitmaps_equal(&c, &out, &a, &da));
}

#[test]
fn test_union_3level_disjoint_octants() {
    let (a, da) = make_bitmap(512, &[0, 1, 2]);
    let (b, db) = make_bitmap(512, &[500, 510, 511]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    assert_eq!(
        c.iter(&out).collect::<Vec<_>>(),
        vec![0, 1, 2, 500, 510, 511]
    );
}

#[test]
fn test_union_3level_partial_overlap() {
    let (a, da) = make_bitmap(512, &[0, 8, 64, 256]);
    let (b, db) = make_bitmap(512, &[0, 9, 65, 300]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    let mut expected = vec![0, 8, 9, 64, 65, 256, 300];
    expected.sort();
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), expected);
}

#[test]
fn test_union_4level() {
    let (a, da) = make_bitmap(4096, &[0, 511, 1000, 2000, 4095]);
    let (b, db) = make_bitmap(4096, &[0, 512, 1000, 3000, 4095]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    let mut expected = vec![0, 511, 512, 1000, 2000, 3000, 4095];
    expected.sort();
    expected.dedup();
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), expected);
}

#[test]
fn test_union_result_valid_contains() {
    let (a, da) = make_bitmap(512, &[0, 1, 7, 8, 63, 64, 255, 256, 511]);
    let (b, db) = make_bitmap(512, &[1, 8, 32, 64, 128, 255, 400, 511]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    for v in c.iter(&out) {
        assert!(
            a.contains(&da, v) || b.contains(&db, v),
            "result {v} not in either operand"
        );
    }
    for v in 0..512 {
        if a.contains(&da, v) || b.contains(&db, v) {
            assert!(c.contains(&out, v), "value {v} missing from union");
        }
    }
}

#[test]
fn test_intersect_overlapping() {
    let (a, da) = make_bitmap(64, &[0, 1, 2, 3]);
    let (b, db) = make_bitmap(64, &[2, 3, 4, 5]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    let result: Vec<u32> = c.iter(&out).collect();
    assert_eq!(result, vec![2, 3]);
}

#[test]
fn test_intersect_disjoint() {
    let (a, da) = make_bitmap(64, &[0, 1]);
    let (b, db) = make_bitmap(64, &[2, 3]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_intersect_single_level() {
    let (a, da) = make_bitmap(8, &[0, 1, 3, 5, 7]);
    let (b, db) = make_bitmap(8, &[1, 2, 3, 6, 7]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![1, 3, 7]);
}

#[test]
fn test_intersect_single_level_disjoint() {
    let (a, da) = make_bitmap(8, &[0, 2, 4]);
    let (b, db) = make_bitmap(8, &[1, 3, 5]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_intersect_one_empty() {
    let (a, da) = make_bitmap(512, &[0, 100, 255, 511]);
    let b = RawBitmap::empty(512);
    let mut out = Vec::new();
    assert!(a.intersect(&da, &b, &[], &mut out).is_empty(&out));
    out.clear();
    assert!(b.intersect(&[], &a, &da, &mut out).is_empty(&out));
}

#[test]
fn test_intersect_both_empty() {
    let a = RawBitmap::empty(64);
    let b = RawBitmap::empty(64);
    let mut out = Vec::new();
    assert!(a.intersect(&[], &b, &[], &mut out).is_empty(&out));
}

#[test]
fn test_intersect_with_self() {
    let (a, da) = make_bitmap(512, &[0, 7, 42, 100, 255, 511]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &a, &da, &mut out);
    assert_eq!(
        c.iter(&out).collect::<Vec<_>>(),
        vec![0, 7, 42, 100, 255, 511]
    );
    assert!(bitmaps_equal(&c, &out, &a, &da));
}

#[test]
fn test_intersect_3level_sparse() {
    let (a, da) = make_bitmap(512, &[0, 1, 2]);
    let (b, db) = make_bitmap(512, &[500, 510, 511]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_intersect_3level_partial_overlap() {
    let (a, da) = make_bitmap(512, &[0, 8, 64, 256]);
    let (b, db) = make_bitmap(512, &[0, 9, 65, 300]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0]);
}

#[test]
fn test_intersect_inner_overlap_leaf_disjoint() {
    let (a, da) = make_bitmap(64, &[0]);
    let (b, db) = make_bitmap(64, &[7]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_intersect_4level() {
    let (a, da) = make_bitmap(4096, &[0, 511, 1000, 2000, 4095]);
    let (b, db) = make_bitmap(4096, &[0, 512, 1000, 3000, 4095]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0, 1000, 4095]);
}

#[test]
fn test_intersect_dense() {
    let (a, da) = make_bitmap(8, &[0, 1, 2, 3, 4, 5, 6, 7]);
    let (b, db) = make_bitmap(8, &[0, 1, 2, 3, 4, 5, 6, 7]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert_eq!(c.len(&out), 8);
}

#[test]
fn test_intersect_subset() {
    let (a, da) = make_bitmap(512, &[10, 20, 30]);
    let (b, db) = make_bitmap(512, &[10, 15, 20, 25, 30, 35]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    assert!(bitmaps_equal(&c, &out, &a, &da));
}

#[test]
fn test_intersect_result_valid_contains() {
    let (a, da) = make_bitmap(512, &[0, 1, 7, 8, 63, 64, 255, 256, 511]);
    let (b, db) = make_bitmap(512, &[1, 8, 32, 64, 128, 255, 400, 511]);
    let mut out = Vec::new();
    let c = a.intersect(&da, &b, &db, &mut out);
    for v in c.iter(&out) {
        assert!(a.contains(&da, v), "result {v} not in a");
        assert!(b.contains(&db, v), "result {v} not in b");
    }
    for v in 0..512 {
        if a.contains(&da, v) && b.contains(&db, v) {
            assert!(c.contains(&out, v), "common value {v} missing from result");
        }
    }
}

#[test]
fn test_difference() {
    let (a, da) = make_bitmap(64, &[0, 1, 2, 3]);
    let (b, db) = make_bitmap(64, &[2, 3, 4, 5]);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &db, &mut out);
    let result: Vec<u32> = c.iter(&out).collect();
    assert_eq!(result, vec![0, 1]);
}

#[test]
fn test_difference_single_level() {
    let (a, da) = make_bitmap(8, &[0, 1, 2, 3, 4]);
    let (b, db) = make_bitmap(8, &[2, 3, 5]);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0, 1, 4]);
}

#[test]
fn test_difference_one_empty() {
    let (a, da) = make_bitmap(512, &[0, 100, 511]);
    let b = RawBitmap::empty(512);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &[], &mut out);
    assert!(bitmaps_equal(&c, &out, &a, &da));

    out.clear();
    let c = b.difference(&[], &a, &da, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_difference_both_empty() {
    let a = RawBitmap::empty(64);
    let b = RawBitmap::empty(64);
    let mut out = Vec::new();
    assert!(a.difference(&[], &b, &[], &mut out).is_empty(&out));
}

#[test]
fn test_difference_with_self() {
    let (a, da) = make_bitmap(512, &[0, 42, 255, 511]);
    let mut out = Vec::new();
    let c = a.difference(&da, &a, &da, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_difference_disjoint() {
    let (a, da) = make_bitmap(512, &[0, 1, 2]);
    let (b, db) = make_bitmap(512, &[500, 510, 511]);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &db, &mut out);
    assert!(bitmaps_equal(&c, &out, &a, &da));
}

#[test]
fn test_difference_3level_partial() {
    let (a, da) = make_bitmap(512, &[0, 8, 64, 256]);
    let (b, db) = make_bitmap(512, &[0, 9, 65, 300]);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![8, 64, 256]);
}

#[test]
fn test_difference_superset() {
    let (a, da) = make_bitmap(512, &[10, 20, 30]);
    let (b, db) = make_bitmap(512, &[10, 15, 20, 25, 30, 35]);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &db, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_difference_4level() {
    let (a, da) = make_bitmap(4096, &[0, 511, 1000, 2000, 4095]);
    let (b, db) = make_bitmap(4096, &[0, 512, 1000, 3000, 4095]);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![511, 2000]);
}

#[test]
fn test_difference_result_valid() {
    let (a, da) = make_bitmap(512, &[0, 1, 7, 8, 63, 64, 255, 256, 511]);
    let (b, db) = make_bitmap(512, &[1, 8, 32, 64, 128, 255, 400, 511]);
    let mut out = Vec::new();
    let c = a.difference(&da, &b, &db, &mut out);
    for v in c.iter(&out) {
        assert!(a.contains(&da, v), "result {v} not in a");
        assert!(!b.contains(&db, v), "result {v} should not be in b");
    }
    for v in 0..512 {
        if a.contains(&da, v) && !b.contains(&db, v) {
            assert!(c.contains(&out, v), "value {v} missing from difference");
        }
    }
}

#[test]
fn test_symmetric_difference() {
    let (a, da) = make_bitmap(64, &[0, 1, 2, 3]);
    let (b, db) = make_bitmap(64, &[2, 3, 4, 5]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da, &b, &db, &mut out);
    let result: Vec<u32> = c.iter(&out).collect();
    assert_eq!(result, vec![0, 1, 4, 5]);
}

#[test]
fn test_symmetric_difference_single_level() {
    let (a, da) = make_bitmap(8, &[0, 1, 2, 3]);
    let (b, db) = make_bitmap(8, &[2, 3, 4, 5]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0, 1, 4, 5]);
}

#[test]
fn test_symmetric_difference_one_empty() {
    let (a, da) = make_bitmap(512, &[0, 100, 511]);
    let b = RawBitmap::empty(512);
    let mut out1 = Vec::new();
    let c1 = a.symmetric_difference(&da, &b, &[], &mut out1);
    assert!(bitmaps_equal(&c1, &out1, &a, &da));
    let mut out2 = Vec::new();
    let c2 = b.symmetric_difference(&[], &a, &da, &mut out2);
    assert!(bitmaps_equal(&c2, &out2, &a, &da));
}

#[test]
fn test_symmetric_difference_both_empty() {
    let a = RawBitmap::empty(64);
    let b = RawBitmap::empty(64);
    let mut out = Vec::new();
    assert!(a
        .symmetric_difference(&[], &b, &[], &mut out)
        .is_empty(&out));
}

#[test]
fn test_symmetric_difference_with_self() {
    let (a, da) = make_bitmap(512, &[0, 42, 255, 511]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da, &a, &da, &mut out);
    assert!(c.is_empty(&out));
}

#[test]
fn test_symmetric_difference_disjoint() {
    let (a, da) = make_bitmap(512, &[0, 1, 2]);
    let (b, db) = make_bitmap(512, &[500, 510, 511]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da, &b, &db, &mut out);
    assert_eq!(
        c.iter(&out).collect::<Vec<_>>(),
        vec![0, 1, 2, 500, 510, 511]
    );
}

#[test]
fn test_symmetric_difference_3level_partial() {
    let (a, da) = make_bitmap(512, &[0, 8, 64, 256]);
    let (b, db) = make_bitmap(512, &[0, 9, 65, 300]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da, &b, &db, &mut out);
    let mut expected = vec![8, 9, 64, 65, 256, 300];
    expected.sort();
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), expected);
}

#[test]
fn test_symmetric_difference_4level() {
    let (a, da) = make_bitmap(4096, &[0, 511, 1000, 2000, 4095]);
    let (b, db) = make_bitmap(4096, &[0, 512, 1000, 3000, 4095]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da, &b, &db, &mut out);
    let mut expected = vec![511, 512, 2000, 3000];
    expected.sort();
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), expected);
}

#[test]
fn test_symmetric_difference_result_valid() {
    let (a, da) = make_bitmap(512, &[0, 1, 7, 8, 63, 64, 255, 256, 511]);
    let (b, db) = make_bitmap(512, &[1, 8, 32, 64, 128, 255, 400, 511]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da, &b, &db, &mut out);
    for v in c.iter(&out) {
        assert!(
            a.contains(&da, v) ^ b.contains(&db, v),
            "result {v} should be in exactly one operand"
        );
    }
    for v in 0..512 {
        if a.contains(&da, v) ^ b.contains(&db, v) {
            assert!(c.contains(&out, v), "value {v} missing from xor");
        }
    }
}

#[test]
fn test_union_commutativity() {
    let (a, da) = make_bitmap(64, &[0, 10, 20]);
    let (b, db) = make_bitmap(64, &[5, 15, 25]);
    let mut out1 = Vec::new();
    let c1 = a.union(&da, &b, &db, &mut out1);
    let mut out2 = Vec::new();
    let c2 = b.union(&db, &a, &da, &mut out2);
    assert!(bitmaps_equal(&c1, &out1, &c2, &out2));
}

#[test]
fn test_intersect_commutativity() {
    let (a, da) = make_bitmap(64, &[0, 10, 20, 30]);
    let (b, db) = make_bitmap(64, &[10, 20, 40, 50]);
    let mut out1 = Vec::new();
    let c1 = a.intersect(&da, &b, &db, &mut out1);
    let mut out2 = Vec::new();
    let c2 = b.intersect(&db, &a, &da, &mut out2);
    assert!(bitmaps_equal(&c1, &out1, &c2, &out2));
}

#[test]
fn test_symmetric_difference_commutativity() {
    let (a, da) = make_bitmap(64, &[0, 10, 20]);
    let (b, db) = make_bitmap(64, &[10, 20, 30]);
    let mut out1 = Vec::new();
    let c1 = a.symmetric_difference(&da, &b, &db, &mut out1);
    let mut out2 = Vec::new();
    let c2 = b.symmetric_difference(&db, &a, &da, &mut out2);
    assert!(bitmaps_equal(&c1, &out1, &c2, &out2));
}

#[test]
fn test_intersection_subset_of_union() {
    let (a, da) = make_bitmap(512, &[0, 100, 200, 300, 400, 511]);
    let (b, db) = make_bitmap(512, &[50, 100, 250, 300, 450, 511]);
    let mut inter_out = Vec::new();
    let intersection = a.intersect(&da, &b, &db, &mut inter_out);
    let mut union_out = Vec::new();
    let union = a.union(&da, &b, &db, &mut union_out);
    for val in intersection.iter(&inter_out) {
        assert!(union.contains(&union_out, val));
    }
}

#[test]
fn test_union_full() {
    let (a, da) = make_bitmap(8, &[0, 1, 2, 3]);
    let (b, db) = make_bitmap(8, &[4, 5, 6, 7]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    assert_eq!(c.len(&out), 8);
}

#[test]
fn test_assign_variants() {
    // Union assign
    let (a, mut da) = make_bitmap(64, &[0, 1, 2]);
    let (b, db) = make_bitmap(64, &[2, 3, 4]);
    let mut out = Vec::new();
    let c = a.union(&da, &b, &db, &mut out);
    da = out;
    assert_eq!(c.iter(&da).collect::<Vec<_>>(), vec![0, 1, 2, 3, 4]);

    // Intersect assign
    let (a, da2) = make_bitmap(64, &[0, 1, 2, 3]);
    let mut out = Vec::new();
    let c = a.intersect(&da2, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![2, 3]);

    // Difference assign
    let (a, da3) = make_bitmap(64, &[0, 1, 2, 3]);
    let mut out = Vec::new();
    let c = a.difference(&da3, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0, 1]);

    // Symmetric difference assign
    let (a, da4) = make_bitmap(64, &[0, 1, 2, 3]);
    let mut out = Vec::new();
    let c = a.symmetric_difference(&da4, &b, &db, &mut out);
    assert_eq!(c.iter(&out).collect::<Vec<_>>(), vec![0, 1, 4]);
}

#[test]
#[should_panic(expected = "universe_size mismatch")]
fn test_set_op_mismatched_universe() {
    let (a, da) = make_bitmap(64, &[0]);
    let (b, db) = make_bitmap(128, &[0]);
    let mut out = Vec::new();
    let _ = a.union(&da, &b, &db, &mut out);
}

#[test]
fn test_serialize_deserialize_roundtrip() {
    let (bm, data) = make_bitmap(512, &[0, 1, 7, 8, 63, 64, 255, 256, 511]);
    let mut buf = Vec::new();
    bm.serialize_into(&data, &mut buf).unwrap();
    let (bm2, data2) = RawBitmap::deserialize_from(&buf[..]).unwrap();
    assert!(bitmaps_equal(&bm, &data, &bm2, &data2));
}

#[test]
fn test_serialize_deserialize_empty() {
    let bm = RawBitmap::empty(64);
    let data: Vec<u8> = Vec::new();
    let mut buf = Vec::new();
    bm.serialize_into(&data, &mut buf).unwrap();
    let (bm2, data2) = RawBitmap::deserialize_from(&buf[..]).unwrap();
    assert!(bitmaps_equal(&bm, &data, &bm2, &data2));
}

#[test]
fn test_serialized_size_matches() {
    let (bm, data) = make_bitmap(512, &[0, 100, 200, 300, 511]);
    let mut buf = Vec::new();
    bm.serialize_into(&data, &mut buf).unwrap();
    assert_eq!(buf.len(), bm.serialized_size(&data));
}

#[test]
fn test_deserialize_truncated() {
    let buf = [1u8, 0, 0, 0];
    let result = RawBitmap::deserialize_from(&buf[..]);
    assert!(result.is_err());
}

#[test]
fn test_heap_bytes() {
    let data: Vec<u8> = Vec::new();
    assert_eq!(data.len(), 0);

    let (_bm, data) = make_bitmap(8, &[0]);
    assert!(data.len() > 0);
}

#[test]
fn test_insert_into_empty() {
    let bm = RawBitmap::empty(64);
    let mut data = Vec::new();
    assert!(bm.is_empty(&data));
    bm.insert(&mut data, 42);
    assert!(!bm.is_empty(&data));
    assert!(bm.contains(&data, 42));
    assert_eq!(bm.len(&data), 1);
}

#[test]
fn test_insert_duplicate_noop() {
    let (bm, mut data) = make_bitmap(64, &[10, 20]);
    let before: Vec<u32> = bm.iter(&data).collect();
    bm.insert(&mut data, 10);
    let after: Vec<u32> = bm.iter(&data).collect();
    assert_eq!(before, after);
}

#[test]
fn test_remove_from_populated() {
    let (bm, mut data) = make_bitmap(64, &[10, 20, 30]);
    bm.remove(&mut data, 20);
    assert!(!bm.contains(&data, 20));
    assert!(bm.contains(&data, 10));
    assert!(bm.contains(&data, 30));
    assert_eq!(bm.len(&data), 2);
}

#[test]
fn test_remove_absent_noop() {
    let (bm, mut data) = make_bitmap(64, &[10, 20]);
    let before: Vec<u32> = bm.iter(&data).collect();
    bm.remove(&mut data, 30);
    let after: Vec<u32> = bm.iter(&data).collect();
    assert_eq!(before, after);
}

#[test]
fn test_clear() {
    let (bm, mut data) = make_bitmap(64, &[10, 20, 30]);
    data.clear();
    assert!(bm.is_empty(&data));
    assert_eq!(bm.len(&data), 0);
    for i in 0..64 {
        assert!(!bm.contains(&data, i));
    }
}

#[test]
fn test_mutation_sequence() {
    let bm = RawBitmap::empty(512);
    let mut data = Vec::new();
    bm.insert(&mut data, 0);
    bm.insert(&mut data, 100);
    bm.insert(&mut data, 511);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![0, 100, 511]);

    bm.remove(&mut data, 100);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![0, 511]);

    bm.insert(&mut data, 200);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![0, 200, 511]);

    data.clear();
    assert!(bm.is_empty(&data));

    bm.insert(&mut data, 42);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![42]);
}

#[test]
fn test_remove_last_element() {
    let (bm, mut data) = make_bitmap(8, &[3]);
    bm.remove(&mut data, 3);
    assert!(bm.is_empty(&data));
}

#[test]
#[should_panic(expected = "out of bounds")]
fn test_insert_out_of_bounds_mutation() {
    let bm = RawBitmap::empty(8);
    let mut data = Vec::new();
    bm.insert(&mut data, 8);
}

#[test]
#[should_panic(expected = "out of bounds")]
fn test_remove_out_of_bounds() {
    let bm = RawBitmap::empty(8);
    let mut data = Vec::new();
    bm.remove(&mut data, 8);
}

#[test]
fn test_from_sorted_iter_correctness() {
    let cases: Vec<(u32, Vec<u32>)> = vec![
        (8, vec![0]),
        (8, vec![7]),
        (8, vec![0, 1, 2, 3, 4, 5, 6, 7]),
        (16, vec![0, 8]),
        (64, vec![63]),
        (64, vec![0, 31, 63]),
        (512, vec![0, 1, 7, 8, 63, 64, 255, 256, 511]),
        (9, vec![8]),
        (1_000_000, vec![950_000]),
        (1_000_000, vec![0, 500_000, 999_999]),
    ];
    for (universe_size, values) in &cases {
        let mut data = Vec::new();
        let bm = RawBitmap::from_sorted_iter(values.iter().copied(), *universe_size, &mut data);
        let result: Vec<u32> = bm.iter(&data).collect();
        assert_eq!(
            result, *values,
            "universe={universe_size}, values={values:?}"
        );
        for &v in values {
            assert!(
                bm.contains(&data, v),
                "universe={universe_size}, missing {v}"
            );
        }
        assert_eq!(bm.len(&data), values.len() as u64);
    }
}

#[test]
fn test_from_sorted_iter_empty() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter(std::iter::empty(), 64, &mut data);
    assert!(bm.is_empty(&data));
    assert_eq!(bm.universe_size(), 64);

    let mut data2 = Vec::new();
    let bm = RawBitmap::from_sorted_iter(std::iter::empty(), 0, &mut data2);
    assert!(bm.is_empty(&data2));
    assert_eq!(bm.levels(), 0);
}

#[test]
fn test_from_sorted_iter_single_level() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter([3, 5].into_iter(), 8, &mut data);
    assert_eq!(&data, &[0x28]); // bits 3 and 5
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![3, 5]);
}

#[test]
fn test_from_sorted_iter_duplicates_tolerated() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter([10, 10, 20, 20, 20].into_iter(), 64, &mut data);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![10, 20]);
}

#[test]
fn test_from_sorted_iter_dense() {
    let values: Vec<u32> = (0..64).collect();
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter(values.iter().copied(), 64, &mut data);
    assert_eq!(bm.len(&data), 64);
    for v in 0..64 {
        assert!(bm.contains(&data, v));
    }
}

#[test]
fn test_from_sorted_iter_large_universe() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_sorted_iter([0, 500_000, 999_999].into_iter(), 1_000_000, &mut data);
    assert_eq!(bm.len(&data), 3);
    assert!(bm.contains(&data, 0));
    assert!(bm.contains(&data, 500_000));
    assert!(bm.contains(&data, 999_999));
    assert!(!bm.contains(&data, 1));
}

#[test]
fn test_from_range_full() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_range(0..64, 64, &mut data);
    assert_eq!(bm.len(&data), 64);
    for v in 0..64 {
        assert!(bm.contains(&data, v));
    }
}

#[test]
fn test_from_range_partial() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_range(10..20, 64, &mut data);
    assert_eq!(bm.len(&data), 10);
    for v in 0..64 {
        assert_eq!(bm.contains(&data, v), (10..20).contains(&v));
    }
}

#[test]
fn test_from_range_inclusive() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_range(5..=10, 64, &mut data);
    assert_eq!(bm.len(&data), 6);
    assert!(bm.contains(&data, 5));
    assert!(bm.contains(&data, 10));
    assert!(!bm.contains(&data, 4));
    assert!(!bm.contains(&data, 11));
}

#[test]
fn test_from_range_unbounded() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_range(.., 64, &mut data);
    assert_eq!(bm.len(&data), 64);

    let mut data = Vec::new();
    let bm = RawBitmap::from_range(..32, 64, &mut data);
    assert_eq!(bm.len(&data), 32);
    assert!(bm.contains(&data, 0));
    assert!(!bm.contains(&data, 32));

    let mut data = Vec::new();
    let bm = RawBitmap::from_range(32.., 64, &mut data);
    assert_eq!(bm.len(&data), 32);
    assert!(!bm.contains(&data, 31));
    assert!(bm.contains(&data, 32));
    assert!(bm.contains(&data, 63));
}

#[test]
fn test_from_range_empty() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_range(10..10, 64, &mut data);
    assert!(bm.is_empty(&data));

    let mut data = Vec::new();
    let bm = RawBitmap::from_range(20..10, 64, &mut data);
    assert!(bm.is_empty(&data));
}

#[test]
fn test_from_range_clamped_to_universe() {
    let mut data = Vec::new();
    let bm = RawBitmap::from_range(0..1000, 64, &mut data);
    assert_eq!(bm.len(&data), 64);

    let mut data = Vec::new();
    let bm = RawBitmap::from_range(60..1000, 64, &mut data);
    assert_eq!(bm.len(&data), 4);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![60, 61, 62, 63]);
}

#[test]
fn test_from_range_matches_from_sorted_iter() {
    for &universe in &[8, 64, 512, 4096] {
        let mut data1 = Vec::new();
        let from_range = RawBitmap::from_range(0..universe, universe, &mut data1);
        let mut data2 = Vec::new();
        let from_iter = RawBitmap::from_sorted_iter(0..universe, universe, &mut data2);
        assert!(
            bitmaps_equal(&from_range, &data1, &from_iter, &data2),
            "universe={universe}"
        );

        let start = universe / 4;
        let end = universe * 3 / 4;
        let mut data3 = Vec::new();
        let from_range = RawBitmap::from_range(start..end, universe, &mut data3);
        let mut data4 = Vec::new();
        let from_iter = RawBitmap::from_sorted_iter(start..end, universe, &mut data4);
        assert!(
            bitmaps_equal(&from_range, &data3, &from_iter, &data4),
            "universe={universe} range={start}..{end}"
        );
    }
}

#[test]
fn test_range_cardinality_full() {
    let (bm, data) = make_bitmap(64, &(0..64).collect::<Vec<_>>());
    assert_eq!(bm.range_cardinality(&data, ..), 64);
    assert_eq!(bm.range_cardinality(&data, 0..64), 64);
    assert_eq!(bm.range_cardinality(&data, 0..=63), 64);
}

#[test]
fn test_range_cardinality_equals_len() {
    let (bm, data) = make_bitmap(512, &[0, 7, 8, 63, 64, 255, 256, 511]);
    assert_eq!(bm.range_cardinality(&data, ..), bm.len(&data));
    assert_eq!(bm.range_cardinality(&data, 0..512), bm.len(&data));
}

#[test]
fn test_range_cardinality_partial() {
    let (bm, data) = make_bitmap(64, &[0, 1, 2, 10, 20, 30, 40, 50, 60, 63]);
    assert_eq!(bm.range_cardinality(&data, 0..3), 3);
    assert_eq!(bm.range_cardinality(&data, 10..11), 1);
    assert_eq!(bm.range_cardinality(&data, 3..10), 0);
    assert_eq!(bm.range_cardinality(&data, 10..=20), 2);
    assert_eq!(bm.range_cardinality(&data, ..10), 3);
    assert_eq!(bm.range_cardinality(&data, 60..), 2);
}

#[test]
fn test_range_cardinality_empty_bitmap() {
    let bm = RawBitmap::empty(64);
    assert_eq!(bm.range_cardinality(&[], ..), 0);
    assert_eq!(bm.range_cardinality(&[], 0..64), 0);
}

#[test]
fn test_range_cardinality_empty_range() {
    let (bm, data) = make_bitmap(64, &[0, 1, 2, 3]);
    assert_eq!(bm.range_cardinality(&data, 10..10), 0);
    assert_eq!(bm.range_cardinality(&data, 10..5), 0);
}

#[test]
fn test_range_cardinality_multi_level() {
    let (bm, data) = make_bitmap(4096, &[0, 100, 500, 1000, 2000, 3000, 4095]);
    assert_eq!(bm.range_cardinality(&data, 0..101), 2);
    assert_eq!(bm.range_cardinality(&data, 100..1001), 3);
    assert_eq!(bm.range_cardinality(&data, 1000..4096), 4);
    assert_eq!(bm.range_cardinality(&data, 3000..=4095), 2);
}

#[test]
fn test_range_cardinality_single_level() {
    let (bm, data) = make_bitmap(8, &[1, 3, 5, 7]);
    assert_eq!(bm.range_cardinality(&data, 0..4), 2);
    assert_eq!(bm.range_cardinality(&data, 4..8), 2);
    assert_eq!(bm.range_cardinality(&data, 0..8), 4);
    assert_eq!(bm.range_cardinality(&data, 1..=5), 3);
}

#[test]
fn test_remove_range_all() {
    let (bm, mut data) = make_bitmap(64, &(0..64).collect::<Vec<_>>());
    bm.remove_range(&mut data, ..);
    assert!(bm.is_empty(&data));
}

#[test]
fn test_remove_range_prefix() {
    let (bm, mut data) = make_bitmap(64, &(0..64).collect::<Vec<_>>());
    bm.remove_range(&mut data, ..32);
    assert_eq!(bm.len(&data), 32);
    assert!(!bm.contains(&data, 31));
    assert!(bm.contains(&data, 32));
    assert!(bm.contains(&data, 63));
}

#[test]
fn test_remove_range_suffix() {
    let (bm, mut data) = make_bitmap(64, &(0..64).collect::<Vec<_>>());
    bm.remove_range(&mut data, 32..);
    assert_eq!(bm.len(&data), 32);
    assert!(bm.contains(&data, 0));
    assert!(bm.contains(&data, 31));
    assert!(!bm.contains(&data, 32));
}

#[test]
fn test_remove_range_middle() {
    let (bm, mut data) = make_bitmap(64, &(0..64).collect::<Vec<_>>());
    bm.remove_range(&mut data, 10..20);
    assert_eq!(bm.len(&data), 54);
    assert!(bm.contains(&data, 9));
    assert!(!bm.contains(&data, 10));
    assert!(!bm.contains(&data, 19));
    assert!(bm.contains(&data, 20));
}

#[test]
fn test_remove_range_empty_range() {
    let vals: Vec<u32> = (0..64).collect();
    let (bm, mut data) = make_bitmap(64, &vals);
    let orig: Vec<u32> = bm.iter(&data).collect();
    bm.remove_range(&mut data, 10..10);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), orig);
}

#[test]
fn test_remove_range_no_overlap() {
    let (bm, mut data) = make_bitmap(64, &[0, 1, 2, 60, 61, 62]);
    let orig: Vec<u32> = bm.iter(&data).collect();
    bm.remove_range(&mut data, 30..40);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), orig);
}

#[test]
fn test_remove_range_single_level() {
    let (bm, mut data) = make_bitmap(8, &[0, 1, 2, 3, 4, 5, 6, 7]);
    bm.remove_range(&mut data, 2..6);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![0, 1, 6, 7]);
}

#[test]
fn test_remove_range_multi_level() {
    let (bm, mut data) = make_bitmap(4096, &[0, 100, 500, 1000, 2000, 3000, 4095]);
    bm.remove_range(&mut data, 100..3000);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![0, 3000, 4095]);
}

#[test]
fn test_remove_range_inclusive() {
    let (bm, mut data) = make_bitmap(64, &[5, 10, 15, 20, 25]);
    bm.remove_range(&mut data, 10..=20);
    assert_eq!(bm.iter(&data).collect::<Vec<_>>(), vec![5, 25]);
}

#[test]
fn test_remove_range_empty_bitmap() {
    let bm = RawBitmap::empty(64);
    let mut data = Vec::new();
    bm.remove_range(&mut data, 0..64);
    assert!(bm.is_empty(&data));
}

#[test]
fn test_estimate_data_size_matches_actual() {
    let cases: Vec<(u32, Vec<u32>)> = vec![
        (8, vec![0]),
        (8, vec![7]),
        (8, vec![0, 1, 2, 3, 4, 5, 6, 7]),
        (16, vec![0, 8]),
        (64, vec![63]),
        (64, vec![0, 31, 63]),
        (512, vec![0, 1, 7, 8, 63, 64, 255, 256, 511]),
        (9, vec![8]),
        (1_000_000, vec![950_000]),
        (1_000_000, vec![0, 500_000, 999_999]),
    ];
    for (universe_size, values) in &cases {
        let (_, data) = make_bitmap(*universe_size, values);
        let estimated = estimate_data_size(*universe_size, values.iter().copied());
        assert_eq!(
            estimated,
            data.len(),
            "universe={universe_size}, values={values:?}"
        );
    }
}

#[test]
fn test_estimate_data_size_empty() {
    assert_eq!(estimate_data_size(64, std::iter::empty()), 0);
    assert_eq!(estimate_data_size(0, std::iter::empty()), 0);
}

#[test]
fn test_estimate_data_size_single_element_is_levels() {
    for &universe in &[8, 64, 512, 4096, 1_000_000] {
        let levels = ceil_log8(universe) as usize;
        let size = estimate_data_size(universe, std::iter::once(0));
        assert_eq!(size, levels, "universe={universe}");
    }
}

#[test]
fn test_estimate_data_size_dense() {
    let values: Vec<u32> = (0..64).collect();
    let (_, data) = make_bitmap(64, &values);
    let estimated = estimate_data_size(64, values.iter().copied());
    assert_eq!(estimated, data.len());
}
