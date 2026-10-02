//! Materialization and value-enumeration correctness over whole files
//! this module builds with the crate's own writer ([`RowIndex`] +
//! [`IndexWriter::write_into`] — not the byte-level `fixture.rs`
//! helpers).
//!
//! The recurring hazard: one field name dot-extends another (`a` vs
//! `a.b`). The primary FST's global key sort then disagrees with the
//! file's KvId assignment (`a.b=y` sorts before `a=x` because '.' < '=',
//! while KvIds go tier by tier, fields name-sorted, values sorted within
//! each field) — every consumer that maps KvIds to strings must follow
//! the assignment, not the FST walk.
//!
//! Pins:
//!
//! - [`IndexReader::materialize_rows`] labels survive that divergence,
//!   in one tier and across all tiers;
//! - every string↔position consumer agrees with the same assignment:
//!   exact and regex selects (the high-card legs resolve through KvId
//!   ranges + the stream-batch scan), facets, projected columns via
//!   [`IndexReader::materialize_fields`], and field-less full text;
//! - [`IndexReader::field_values`]: per-tier enumeration, sorted, prefix
//!   stripped by LENGTH, `UnknownField` for an absent field;
//! - `field_values` reads dictionary chunks only — with every stream
//!   batch corrupted, enumeration still succeeds while row access fails.
//!
//! Not pinned here: projecting a field this file lacks (absent-field
//! columns), `materialize_rows`' corruption error paths, chunk-level
//! format round-trips (`round_trip.rs`), general query semantics
//! (`query.rs`), trace-plan evaluation (`trace_plan.rs`).

use bumpalo::Bump;

use crate::{IndexReader, IndexWriter, RowIndex};

/// Real-writer fixture: one row per kv list, timestamps in row order,
/// packed by [`IndexWriter::write_into`].
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

/// Two low-card fields where one name is a dot-extended prefix of the
/// other, each in its OWN row. In the primary FST `a.b=y` sorts BEFORE
/// `a=x` ('.' < '='), while KvId assignment orders fields by name (`a`
/// before `a.b`) — label resolution must follow the KvId assignment, not
/// the FST walk. Per-row separation makes a swap visible: one shared row
/// would swap the two labels within a single field set and still pass.
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
    fill(&mut ri, "a", 3, &mut expected); // low-tier pair
    fill(&mut ri, "a.b", 3, &mut expected);
    fill(&mut ri, "m", 20, &mut expected); // mid-tier pair
    fill(&mut ri, "m.n", 20, &mut expected);
    fill(&mut ri, "h", 120, &mut expected); // high-tier pair
    fill(&mut ri, "h.i", 120, &mut expected);
    let (buf, _s, _m) =
        IndexWriter::write_into(&ri, std::io::Cursor::new(Vec::new()), Vec::new()).unwrap();
    (buf.into_inner(), expected)
}

/// The same dot-extended-family divergence through
/// [`IndexReader::materialize_rows`], exercised in every tier at once.
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

/// Cross-path agreement on the KvId assignment, exercised per tier:
/// exact and regex selects (the high-card legs resolve through KvId
/// ranges + the stream-batch scan), facets, projected columns
/// ([`IndexReader::materialize_fields`]), and field-less full text.
/// Each `field=value` exists on exactly one known row, so any
/// writer/reader order disagreement returns the wrong positions.
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

    // Regex select on one value per field (exercises the pattern scans;
    // a pattern anchors to the whole value, so "v0*1" hits v001 only).
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

/// [`IndexReader::field_values`] per tier: every tier returns exactly its
/// stored values, sorted, with the `field=` prefix stripped by LENGTH
/// (values containing `=` survive whole); an absent field is
/// `UnknownField`. Reads only the field's dictionary chunk — the
/// access-pattern proof is the corruption test below.
#[test]
fn field_values_per_tier_prefix_stripped_by_length() {
    let (bytes, expected) = prefix_family_fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    for field in ["a", "a.b", "m", "m.n", "h", "h.i"] {
        let mut want: Vec<String> = expected
            .iter()
            .filter(|(f, _)| f == field)
            .map(|(_, v)| v.clone())
            .collect();
        want.sort();
        assert_eq!(
            idx.field_values(field).unwrap(),
            want,
            "values of {field}"
        );
    }
    assert!(matches!(
        idx.field_values("absent"),
        Err(crate::Error::UnknownField(f)) if f == "absent"
    ));

    // A value containing '=' round-trips whole: the prefix is stripped by
    // LENGTH, so later '='s stay in the value (a split-on-'=' would cut
    // it to "x").
    let bytes = file_of_rows(&[&["eq=x=y=z"], &["eq=plain"]]);
    let idx = IndexReader::open(&bytes).unwrap();
    assert_eq!(
        idx.field_values("eq").unwrap(),
        vec!["plain".to_string(), "x=y=z".to_string()]
    );
}

/// Access-pattern proof for [`IndexReader::field_values`]: with EVERY
/// stream-batch chunk payload corrupted (crc32 mismatch on access), value
/// enumeration across all tiers still returns exact results — it reads
/// only dictionary chunks — while any row access fails loudly.
#[test]
fn field_values_never_touch_stream_batches() {
    let (bytes, expected) = prefix_family_fixture();

    // Locate each stream-batch payload through the CLEAN file's raw
    // slices (crc-verified there), then flip a byte in a copy.
    let mut corrupted = bytes.clone();
    let n = {
        let cr = crate::reader::ChunkReader::open(&bytes).unwrap();
        let n = crate::num_stream_batches(cr.summary().unwrap().record_count);
        assert!(n > 0, "fixture must have stream batches");
        let base = bytes.as_ptr() as usize;
        for i in 0..n {
            let raw = cr.stream_batch_raw(i).unwrap();
            let off = raw.as_ptr() as usize - base;
            corrupted[off] ^= 0xFF;
        }
        n
    };

    let idx = IndexReader::open(&corrupted).unwrap();
    for i in 0..n {
        assert!(
            idx.load_stream_batch(i).is_err(),
            "batch {i} should be corrupt"
        );
    }
    for field in ["a", "a.b", "m", "m.n", "h", "h.i"] {
        let mut want: Vec<String> = expected
            .iter()
            .filter(|(f, _)| f == field)
            .map(|(_, v)| v.clone())
            .collect();
        want.sort();
        assert_eq!(idx.field_values(field).unwrap(), want, "values of {field}");
    }
}
