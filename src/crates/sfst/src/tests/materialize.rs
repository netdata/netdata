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

/// All-tier fixture with a dot-extended field family per tier
/// (threshold 10 → mid = [10, 100), high ≥ 100): one `field=value` per row,
/// ascending timestamps (position == insertion index), so any misassignment
/// is visible per row. Returns the file bytes plus each row's expected pair.
fn prefix_family_fixture() -> (Vec<u8>, Vec<(String, String)>) {
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
    (buf.into_inner(), expected)
}

/// The same dot-extended-family divergence, exercised in every tier at once.
#[test]
fn labels_survive_prefix_field_families_all_tiers() {
    let (bytes, expected) = prefix_family_fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let positions: Vec<u32> = (0..expected.len() as u32).collect();
    let rows = idx.materialize_rows(&positions).unwrap();
    for (pos, exp) in expected.iter().enumerate() {
        assert_eq!(rows[pos].fields, vec![exp.clone()], "row {pos} mislabeled");
    }
}

/// KvId-assignment agreement audit for the query paths that do KvId
/// arithmetic (rather than string-keyed lookups): the high-card filter path
/// (exact + regex → SB scan), projected columns for every tier, facets, and
/// field-less full text. Each `field=value` exists on exactly one known row,
/// so any writer/reader order disagreement returns the wrong positions.
#[test]
fn kv_id_paths_agree_for_prefix_field_families() {
    let (bytes, expected) = prefix_family_fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let span = i64::MIN..i64::MAX;
    let all: Vec<u32> = (0..expected.len() as u32).collect();

    // Exact select per (field, value) — every tier: exactly its own row.
    for (pos, (field, value)) in expected.iter().enumerate() {
        let f = crate::Filter::new().select(field, value);
        let bf = idx.compile_filter(&f, None).unwrap();
        assert_eq!(
            idx.matched_positions(&bf, span.clone()).unwrap(),
            vec![pos as u32],
            "exact select {field}={value}"
        );
    }

    // Regex select on one value per field (exercises the pattern scans).
    for field in ["a", "a.b", "m", "m.n", "h", "h.i"] {
        let f = crate::Filter::new().select_pattern(field, "v0*1"); // matches v001 only
        let bf = idx.compile_filter(&f, None).unwrap();
        let pos = expected
            .iter()
            .position(|(fname, v)| fname == field && v == "v001")
            .unwrap() as u32;
        assert_eq!(
            idx.matched_positions(&bf, span.clone()).unwrap(),
            vec![pos],
            "regex select on {field}"
        );
    }

    // Facets (low/mid only — high is rejected by design): per field, every
    // value counts exactly 1 and the value set is exact.
    let empty = idx.compile_filter(&crate::Filter::new(), None).unwrap();
    let facets = idx
        .facets(&["a", "a.b", "m", "m.n"], &empty, span.clone())
        .unwrap();
    for fr in &facets {
        let want: Vec<(String, u32)> = expected
            .iter()
            .filter(|(f, _)| *f == fr.field)
            .map(|(_, v)| (v.clone(), 1u32))
            .collect();
        let mut got = fr.values.clone();
        got.sort();
        assert_eq!(got, want, "facet {}", fr.field);
    }

    // Projected columns for every field: the value appears exactly at its row.
    let cols = idx
        .materialize_fields(&["a", "a.b", "m", "m.n", "h", "h.i"], &all)
        .unwrap();
    for (fi, field) in ["a", "a.b", "m", "m.n", "h", "h.i"].iter().enumerate() {
        for (pos, exp) in expected.iter().enumerate() {
            let want: Vec<String> = if exp.0 == *field {
                vec![exp.1.clone()]
            } else {
                vec![]
            };
            assert_eq!(cols[fi][pos], want, "column {field} at row {pos}");
        }
    }

    // Field-less full text hitting one high-card pair.
    let q = idx
        .compile_filter(&crate::Filter::new(), Some("h.i=v005"))
        .unwrap();
    let pos = expected
        .iter()
        .position(|(f, v)| f == "h.i" && v == "v005")
        .unwrap() as u32;
    assert_eq!(
        idx.matched_positions(&q, span.clone()).unwrap(),
        vec![pos],
        "full-text high-card"
    );
}
