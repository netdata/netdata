use arrayvec::ArrayString;

use crate::parser::{Field, Node, parse};
use crate::tokenizer::{Separator, tokenize};

/// Converts a hash index (mod 36) to an alphanumeric character (A-Z, 0-9).
fn base36_char(idx: u64) -> char {
    const TABLE: &[u8; 36] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
    TABLE[(idx % 36) as usize] as char
}

/// Computes a 2-character checksum (A-Z, 0-9) for camel case disambiguation.
/// Provides 36^2 = 1,296 possible values.
fn compute_checksum(s: &[u8]) -> [char; 2] {
    use std::hash::{DefaultHasher, Hash, Hasher};

    let mut hasher = DefaultHasher::new();
    s.hash(&mut hasher);
    let hash = hasher.finish();

    [base36_char(hash / 36), base36_char(hash)]
}

/// Flushes a run of identical characters using RLE compression.
/// Runs of 1-2 are kept as-is; runs of 3+ become `<count><char>`, splitting at 9.
fn flush_run(result: &mut ArrayString<64>, ch: u8, count: usize) {
    let mut remaining = count;

    while remaining > 9 {
        result.push('9');
        result.push(ch as char);
        remaining -= 9;
    }

    if remaining > 2 {
        result.push((b'0' + remaining as u8) as char);
        result.push(ch as char);
    } else {
        for _ in 0..remaining {
            result.push(ch as char);
        }
    }
}

/// Encodes parser nodes into a compact, RLE-compressed string.
///
/// Each field is encoded as a single char from the contiguous range A–X
/// (24 values), computed as `'A' + field_base + following_offset`:
///
/// ```text
///              .Dot  _Underscore  -Hyphen  NoSep  End
/// Lowercase      A       B           C       D     E
/// LowerCamel     F       G           H       I     J
/// UpperCamel     K       L           M       N     O
/// Uppercase      P       Q           R       S     T
/// Empty          U       V           W       —     X
/// ```
///
/// Empty fields cannot have NoSep (an empty field is always adjacent to a
/// separator), so End sits at offset 3 instead of 4.
///
/// Consecutive identical characters are RLE-compressed inline (e.g. `"AAAE"` → `"3AE"`).
/// If a checksum is provided, it is prepended verbatim (never compressed).
fn encode_nodes(nodes: &[Node], checksum: Option<[char; 2]>, result: &mut ArrayString<64>) {
    // Prepend the checksum for camel case disambiguation.
    // Checksum chars (A-Z, 0-9) don't interfere with the RLE compression
    // below because they are pushed before the run tracker starts.
    if let Some(cs) = checksum {
        result.push(cs[0]);
        result.push(cs[1]);
    }

    // Encode fields and RLE-compress inline. Each field produces a single
    // char; we track the current run and flush it when the char changes.
    // Separators between fields are consumed as part of the "following"
    // context (offset + skip), so they don't appear in the output.
    let mut prev: u8 = 0;
    let mut run_count: usize = 0;

    let mut i = 0;
    while i < nodes.len() {
        if let Node::Field(field) = nodes[i] {
            // Row in the encoding table
            let base: u8 = match field {
                Field::Lowercase => 0,
                Field::LowerCamel => 5,
                Field::UpperCamel => 10,
                Field::Uppercase => 15,
                Field::Empty => 20,
            };

            // Column: what follows this field? If a separator follows,
            // consume it (skip=2). Otherwise just advance past the field (skip=1).
            let (offset, skip): (u8, usize) = match nodes.get(i + 1) {
                Some(Node::Separator(Separator::Dot)) => (0, 2),
                Some(Node::Separator(Separator::Underscore)) => (1, 2),
                Some(Node::Separator(Separator::Hyphen)) => (2, 2),
                Some(Node::Field(_)) => (3, 1),
                None => (if field == Field::Empty { 3 } else { 4 }, 1),
            };

            let ch = b'A' + base + offset;
            if ch == prev {
                run_count += 1;
            } else {
                flush_run(result, prev, run_count);
                prev = ch;
                run_count = 1;
            }

            i += skip;
        } else {
            // Bare separator (shouldn't happen — separators are consumed
            // by the field that precedes them). Skip defensively.
            i += 1;
        }
    }

    flush_run(result, prev, run_count);
}

/// Encodes a key into a compact, RLE-compressed structure representation,
/// appending the result to `result`.
///
/// Each character represents a (field-type, following) pair, with runs of 3+
/// identical characters compressed (e.g. `"AAAE"` → `"3AE"`).
/// If the input contains camel case fields, a 2-character checksum is prepended.
/// Returns `false` if the input contains invalid characters.
pub(crate) fn encode(source: &[u8], result: &mut ArrayString<64>) -> bool {
    let Some(tokens) = tokenize(source) else {
        return false;
    };
    let nodes = parse(&tokens);

    let checksum = if nodes.iter().any(|node| {
        matches!(
            node,
            Node::Field(Field::LowerCamel) | Node::Field(Field::UpperCamel)
        )
    }) {
        Some(compute_checksum(source))
    } else {
        None
    };

    encode_nodes(&nodes, checksum, result);
    true
}

#[cfg(test)]
mod tests {
    use super::*;

    fn encode_str(source: &[u8]) -> Option<String> {
        let mut result = ArrayString::<64>::new();
        encode(source, &mut result).then_some(result.as_str().into())
    }

    // --- encode_nodes boundary values ---

    #[test]
    fn encode_nodes_boundary_chars() {
        // Verify first and last char of each field type row in the encoding table
        // Lowercase: 'A' (dot) through 'E' (end)
        assert_eq!(encode_str(b"foo.bar").unwrap(), "AE"); // A=Low.Dot, E=Low.End
        assert_eq!(encode_str(b"foo_bar").unwrap(), "BE"); // B=Low.Underscore
        assert_eq!(encode_str(b"foo-bar").unwrap(), "CE"); // C=Low.Hyphen

        // Uppercase: 'P' (dot) through 'T' (end)
        assert_eq!(encode_str(b"HTTP").unwrap(), "T"); // T=Up.End

        // Empty: 'U' (dot) through 'X' (end)
        assert_eq!(encode_str(b".foo").unwrap(), "UE"); // U=Empty.Dot
        assert_eq!(encode_str(b"foo.").unwrap(), "AX"); // X=Empty.End
    }

    // --- checksum ---

    #[test]
    fn checksum_is_deterministic() {
        let a = compute_checksum(b"log.body.HostName");
        let b = compute_checksum(b"log.body.HostName");
        assert_eq!(a, b);
    }

    #[test]
    fn checksum_differs_for_different_inputs() {
        let a = compute_checksum(b"fooBar");
        let b = compute_checksum(b"fooBaz");
        assert_ne!(a, b);
    }

    #[test]
    fn checksum_is_two_chars() {
        let cs = compute_checksum(b"anything");
        assert!(
            cs.iter()
                .all(|c| c.is_ascii_uppercase() || c.is_ascii_digit())
        );
    }

    #[test]
    fn checksum_no_collision_for_camel_case_variants() {
        // These all share the same structure encoding ("aao") but differ in
        // camel case boundaries. The checksum must distinguish them.
        let inputs: &[&[u8]] = &[
            b"log.body.Hostname",
            b"log.body.HoStname",
            b"log.body.HoStnAme",
            b"log.body.HosTnaMe",
            b"log.body.HoStNaMe",
            b"log.body.HosTname",
            b"log.body.HostName",
            b"log.body.HosTnAme",
            b"log.body.HoStnaMe",
            b"log.body.HostnAme",
            b"log.body.HostnaMe",
            b"log.body.HoStName",
            b"log.body.HostNaMe",
        ];

        let encoded: Vec<_> = inputs.iter().map(|b| encode_str(b).unwrap()).collect();

        // All should have the same structure but different checksums
        for e in &encoded {
            assert!(e.ends_with("AAO"), "{} doesn't end with 'AAO'", e);
        }

        // No two should be identical
        for i in 0..encoded.len() {
            for j in (i + 1)..encoded.len() {
                assert_ne!(
                    encoded[i],
                    encoded[j],
                    "collision: {:?} and {:?} both encode to {:?}",
                    std::str::from_utf8(inputs[i]).unwrap(),
                    std::str::from_utf8(inputs[j]).unwrap(),
                    encoded[i],
                );
            }
        }
    }

    // --- encode ---

    #[test]
    fn encode_single_lowercase() {
        assert_eq!(encode_str(b"hello").unwrap(), "E");
    }

    #[test]
    fn encode_single_uppercase() {
        assert_eq!(encode_str(b"HTTP").unwrap(), "T");
    }

    #[test]
    fn encode_dot_separated_lowercase() {
        assert_eq!(encode_str(b"foo.bar").unwrap(), "AE");
    }

    #[test]
    fn encode_underscore_separated() {
        assert_eq!(encode_str(b"foo_bar").unwrap(), "BE");
    }

    #[test]
    fn encode_hyphen_separated() {
        assert_eq!(encode_str(b"foo-bar").unwrap(), "CE");
    }

    #[test]
    fn encode_three_dot_separated() {
        assert_eq!(encode_str(b"a.b.c").unwrap(), "AAE");
    }

    #[test]
    fn encode_upper_camel_has_checksum() {
        let result = encode_str(b"HostName").unwrap();
        assert_eq!(result.len(), 3); // 2-char checksum + "O"
        assert!(result.ends_with('O')); // UpperCamelEnd
    }

    #[test]
    fn encode_lower_camel_has_checksum() {
        let result = encode_str(b"fooBar").unwrap();
        assert_eq!(result.len(), 3); // 2-char checksum + "J"
        assert!(result.ends_with('J')); // LowerCamelEnd
    }

    #[test]
    fn encode_log_body_hostname() {
        let result = encode_str(b"log.body.HostName").unwrap();
        assert_eq!(result.len(), 5); // 2-char checksum + "AAO"
        assert!(result.ends_with("AAO"));
    }

    #[test]
    fn encode_mixed_no_sep() {
        // serviceURL.Host → [Low.NoSep, Up.Dot, UpperCamel.End] → checksum + "DPO"
        let result = encode_str(b"serviceURL.Host").unwrap();
        assert!(result.ends_with("DPO"));
    }

    #[test]
    fn encode_no_camel_no_checksum() {
        // No camel case → no checksum prefix → all chars are A-X
        let result = encode_str(b"foo.bar").unwrap();
        assert!(result.chars().all(|c| c.is_ascii_uppercase()));
    }

    #[test]
    fn encode_invalid_chars_returns_none() {
        assert!(encode_str(b"hello world").is_none());
        assert!(encode_str(b"foo@bar").is_none());
        assert!(encode_str(b"\xff").is_none());
        // Also verify encode() directly returns false
        let mut buf = ArrayString::<64>::new();
        assert!(!encode(b"hello world", &mut buf));
    }

    #[test]
    fn encode_leading_separator() {
        assert_eq!(encode_str(b".foo").unwrap(), "UE");
    }

    #[test]
    fn encode_trailing_separator() {
        assert_eq!(encode_str(b"foo.").unwrap(), "AX");
    }

    #[test]
    fn encode_consecutive_separators() {
        // nodes: [Field(Low), Sep(Dot), Field(Empty), Sep(Dot), Field(Low)]
        // Low+Dot='A', Empty+Dot='U', Low+End='E' → "AUE"
        assert_eq!(encode_str(b"foo..bar").unwrap(), "AUE");
    }

    #[test]
    fn encode_empty_input() {
        assert_eq!(encode_str(b"").unwrap(), "");
    }

    // --- RLE compression ---

    #[test]
    fn rle_no_compression_for_two_identical() {
        // "a.b.c" → 2 A's + end = "AAE" (no compression, run of 2)
        assert_eq!(encode_str(b"a.b.c").unwrap(), "AAE");
    }

    #[test]
    fn rle_compresses_three_identical() {
        // "a.b.c.d" → 3 A's + end = "3AE"
        assert_eq!(encode_str(b"a.b.c.d").unwrap(), "3AE");
    }

    #[test]
    fn rle_compresses_nine_identical() {
        // 9 dot-separated + end = "9AE"
        assert_eq!(encode_str(b"a.b.c.d.e.f.g.h.i.j").unwrap(), "9AE");
    }

    #[test]
    fn rle_wraps_at_nine() {
        // 10 dot-separated + end = "9A" + "AE"
        assert_eq!(encode_str(b"a.b.c.d.e.f.g.h.i.j.k").unwrap(), "9AAE");
    }

    #[test]
    fn rle_twelve_wraps() {
        // 12 dot-separated + end = "9A" + "3AE"
        assert_eq!(encode_str(b"a.b.c.d.e.f.g.h.i.j.k.l.m").unwrap(), "9A3AE");
    }
}
