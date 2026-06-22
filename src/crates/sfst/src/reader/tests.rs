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
