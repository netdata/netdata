use super::*;

#[test]
fn error_on_bad_magic() {
    let data = b"BADXxxxxxxxxxxxxxxxxxxxxxxxxxxxx";
    assert!(matches!(Reader::open(data), Err(Error::InvalidMagic)));
}

#[test]
fn error_on_short_file() {
    let data = b"SFST";
    assert!(matches!(
        Reader::open(data),
        Err(Error::FileTooShort(4, 12))
    ));
}

#[test]
fn error_on_older_format_version() {
    // A valid current-version file whose version field is rewound to v7 is
    // rejected on open — the v8 META/columns layout is incompatible with v7.
    let summary = crate::Summary {
        min_timestamp_s: 0,
        max_timestamp_s: 0,
        record_count: 1,
        content_meta: Vec::new(),
    };
    let mut buf = crate::write_summary_only(std::io::Cursor::new(Vec::new()), &summary)
        .unwrap()
        .into_inner();
    // Header layout: magic(4) | version(4, LE) | num_chunks(4). Overwrite v8 -> v7.
    buf[4..8].copy_from_slice(&7u32.to_le_bytes());
    assert!(matches!(Reader::open(&buf), Err(Error::UnsupportedVersion(7))));
}
