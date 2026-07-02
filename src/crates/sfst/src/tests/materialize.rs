//! Materialized-row label correctness.

use bumpalo::Bump;

use crate::{IndexReader, IndexWriter, RowIndex};

/// One row per kv list, timestamps in row order.
fn file_of_rows(rows: &[&[&str]]) -> Vec<u8> {
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 100);
    for (i, kvs) in rows.iter().enumerate() {
        let tokens: Vec<_> = kvs.iter().map(|kv| ri.intern(None, kv)).collect();
        ri.row(1_000 + i as i64, &tokens);
    }
    let (buf, _summary, _metadata) =
        IndexWriter::write_into(&ri, std::io::Cursor::new(Vec::new()), Vec::new()).unwrap();
    buf.into_inner()
}

/// Two low-card fields where one name is a dot-extended prefix of the other,
/// each in its OWN row. In the primary FST `a.b=y` sorts BEFORE `a=x`
/// ('.' < '='), while KvId assignment orders fields by name (`a` before
/// `a.b`) — the two orders disagree, and label resolution must follow the
/// KvId assignment, not the FST walk. Per-row separation makes a swap
/// visible (a shared row would swap labels into the same set).
#[test]
fn labels_survive_prefix_field_families() {
    let bytes = file_of_rows(&[&["a=x"], &["a.b=y"]]);
    let idx = IndexReader::open(&bytes).unwrap();
    let rows = idx.materialize_rows(&[0, 1]).unwrap();
    assert_eq!(
        rows[0].fields,
        vec![("a".to_string(), "x".to_string())],
        "row 0 mislabeled"
    );
    assert_eq!(
        rows[1].fields,
        vec![("a.b".to_string(), "y".to_string())],
        "row 1 mislabeled"
    );
    // The full reverse table follows the KvId assignment order too.
    let table = idx.build_string_table(idx.field_table()).unwrap();
    assert_eq!(table, vec!["a=x".to_string(), "a.b=y".to_string()]);
}

/// The same dot-extended-family divergence, exercised in every tier at once
/// (threshold 10 → mid = [10, 100), high ≥ 100). One value per row, so any
/// label swap is visible per row.
#[test]
fn labels_survive_prefix_field_families_all_tiers() {
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 10);
    let mut expected: Vec<(String, String)> = Vec::new();
    let fill = |ri: &mut RowIndex, field: &str, n: usize, exp: &mut Vec<(String, String)>| {
        for i in 0..n {
            let v = format!("v{i:03}");
            let t = ri.intern(None, &format!("{field}={v}"));
            ri.row(exp.len() as i64, &[t]);
            exp.push((field.to_string(), v));
        }
    };
    fill(&mut ri, "a", 3, &mut expected); // low pair
    fill(&mut ri, "a.b", 3, &mut expected);
    fill(&mut ri, "m", 20, &mut expected); // mid pair
    fill(&mut ri, "m.n", 20, &mut expected);
    fill(&mut ri, "h", 120, &mut expected); // high pair
    fill(&mut ri, "h.i", 120, &mut expected);

    let (buf, _s, _m) =
        IndexWriter::write_into(&ri, std::io::Cursor::new(Vec::new()), Vec::new()).unwrap();
    let idx = IndexReader::open(buf.get_ref()).unwrap();
    let positions: Vec<u32> = (0..expected.len() as u32).collect();
    let rows = idx.materialize_rows(&positions).unwrap();
    for (pos, exp) in expected.iter().enumerate() {
        assert_eq!(rows[pos].fields, vec![exp.clone()], "row {pos} mislabeled");
    }
}
