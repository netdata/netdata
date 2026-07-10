use super::*;

#[test]
fn error_on_bad_magic() {
    let data = b"BADXxxxxxxxxxxxxxxxxxxxxxxxxxxxx";
    assert!(matches!(ChunkReader::open(data), Err(Error::InvalidMagic)));
}

#[test]
fn error_on_short_file() {
    let data = b"SFST";
    assert!(matches!(
        ChunkReader::open(data),
        Err(Error::FileTooShort(4, 12))
    ));
}

#[test]
fn error_on_other_format_version() {
    // A valid file whose version field is patched to any other version is
    // rejected on open — readers accept exactly the current version.
    let summary = crate::Summary {
        min_timestamp_s: 0,
        max_timestamp_s: 0,
        record_count: 1,
        content_meta: Vec::new(),
    };
    let mut buf = crate::writer::write_summary_only(std::io::Cursor::new(Vec::new()), &summary)
        .unwrap()
        .into_inner();
    // Header layout: magic(4) | version(4, LE) | num_chunks(4).
    buf[4..8].copy_from_slice(&2u32.to_le_bytes());
    assert!(matches!(
        ChunkReader::open(&buf),
        Err(Error::UnsupportedVersion(2))
    ));
}

#[test]
fn read_summary_serves_summary_only_files() {
    // A summary-only file (only the SUMR chunk) is exactly what the traces
    // seal produces; `IndexReader::open` refuses it, `read_summary` must not.
    let summary = Summary {
        min_timestamp_s: 10,
        max_timestamp_s: 20,
        record_count: 3,
        content_meta: vec![1, 2, 3],
    };
    let buf = crate::writer::write_summary_only(std::io::Cursor::new(Vec::new()), &summary)
        .unwrap()
        .into_inner();
    assert!(crate::IndexReader::open(&buf).is_err());
    assert_eq!(crate::read_summary(&buf).unwrap(), summary);
}
