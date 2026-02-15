use crate::*;

// ---- Bitmap (high-level, De Morgan) tests ----

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

#[test]
fn test_bitmap_len() {
    let mut data = Vec::new();
    let bm = Bitmap::from_sorted_iter([10, 20, 30].into_iter(), 64, &mut data);
    assert_eq!(bm.len(&data), 3);

    let mut data2 = Vec::new();
    let bm = Bitmap::from_sorted_iter_complemented([10, 20, 30].into_iter(), 64, &mut data2);
    assert_eq!(bm.len(&data2), 61);
}

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

#[test]
fn test_bitmap_assign_variants() {
    let mut db = Vec::new();
    let b = Bitmap::from_sorted_iter([2, 3, 4].into_iter(), 64, &mut db);

    // AND
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([0, 1, 2].into_iter(), 64, &mut da);
    let mut out = Vec::new();
    let c = a.and(&da, &b, &db, &mut out);
    assert!(c.contains(&out, 2));
    assert!(!c.contains(&out, 0));

    // OR
    let mut da = Vec::new();
    let a = Bitmap::from_sorted_iter([0, 1, 2].into_iter(), 64, &mut da);
    let mut out = Vec::new();
    let c = a.or(&da, &b, &db, &mut out);
    for v in [0, 1, 2, 3, 4] {
        assert!(c.contains(&out, v), "missing {v}");
    }
}

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
