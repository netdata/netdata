//! Field-name normalization for the journal stack: turns arbitrary field
//! names into names journald accepts as field names — uppercase ASCII
//! letters, digits and underscores, at most 64 bytes.
//!
//! The pipeline behind the single public entry point `encode_full`:
//! `tokenize` splits the name into words (lowercase / UPPERCASE /
//! Capitalized; digits count as uppercase) and separators (`. _ -`), `parse`
//! merges words into typed fields, `encode_nodes` emits one structure
//! character per field+separator pair (alphabet a-x) plus a 2-character
//! checksum when camelCase fields are present, and `encode_full`
//! run-length-compresses the structure characters (never the checksum),
//! uppercases, and appends the original name normalized to uppercase with
//! `.`/`-` mapped to `_` and common attribute prefixes shortened
//! (RA_/LA_/LB_).
//!
//! Output shapes (the contract the journal stack keys on):
//! - Normal: `ND<checksum?><compressed-structure>_<NORMALIZED-NAME>` — e.g.
//!   `log.body.hostname` → `NDAAE_LB_HOSTNAME`.
//! - MD5 fallback: `ND_` + 32 uppercase hex = 35 bytes — for inputs that are
//!   not tokenizable (non-UTF-8 or any byte outside [A-Za-z0-9._-]) or whose
//!   normal shape would exceed 64 bytes. The shapes are distinguishable
//!   because structure characters are lowercase a-x (A-X after uppercasing)
//!   and checksum characters A-Z/0-9; neither alphabet contains `_`.
//!
//! Consumers: journal-log-writer remaps non-journald-compatible field names
//! at write time (`journal-log-writer/src/log/mod.rs`
//! `write_entry_with_timestamps`) and persists the otel→systemd mapping as
//! `ND_REMAPPING=1` entries (same file, `write_remapping_entry`);
//! journal-core's reader rebuilds the reverse mapping by accepting only
//! fields that start with `ND_` and are exactly 35 bytes
//! (`journal-core/src/file/reader.rs` `parse_remapping_entry`), so
//! normal-shape mappings are written but silently dropped on load — only
//! MD5-fallback mappings round-trip; journal-core's FieldMap tests drive
//! `encode_full` directly (`journal-core/src/field_map.rs` test
//! `test_remapping_registry`). The
//! published systemd-journal-sdk-* crates carry no twin (grep-verified
//! 0.8.1; their engine's cache notes v3 dropped ND_REMAPPING-specific
//! indexing). The `rdp` bin (main.rs) prints the encodings of a fixed key
//! list with a checksum of the whole output — a dev tool. Dependency: `md5`
//! only (Cargo.toml). Nothing in the repo expands the crate name.

// The character classes `tokenize` recognizes; digits classify as uppercase.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum CharType {
    Lowercase,
    Uppercase,
    Dot,
    Underscore,
    Hyphen,
}

// Classifies one byte; `None` marks everything outside [A-Za-z0-9._-] — the
// condition that makes `tokenize` (and with it `encode`) fail and take the
// MD5 fallback.
fn char_type(c: u8) -> Option<CharType> {
    if c.is_ascii_lowercase() {
        Some(CharType::Lowercase)
    } else if c.is_ascii_uppercase() || c.is_ascii_digit() {
        Some(CharType::Uppercase)
    } else if c == b'.' {
        Some(CharType::Dot)
    } else if c == b'_' {
        Some(CharType::Underscore)
    } else if c == b'-' {
        Some(CharType::Hyphen)
    } else {
        None
    }
}

/// Prefix of the MD5 fallback shape: `encode` emits `ND_` + 32 uppercase hex
/// (35 bytes) for names it cannot tokenize, and `encode_full` re-checks this
/// prefix to tell fallback outputs from structure encodings. A structure
/// encoding can never start with `ND_` — structure characters are lowercase
/// a-x and checksum characters A-Z/0-9, and neither alphabet contains `_`.
/// journal-core's reader gate keys on this shape to recognize remapping
/// fields (`journal-core/src/file/reader.rs` `parse_remapping_entry`).
const REMAPPED_PREFIX: &str = "ND_";

// Word shapes as `tokenize` sees them: all-lowercase, all-uppercase-class
// (letters and digits), or Capitalized (uppercase-class first character,
// lowercase tail).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum TokenType {
    Lowercase,
    Uppercase,
    Capitalized,
}

// The separators the encoder distinguishes; each kind maps to its own
// structure character, so the separator kind survives encoding.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Separator {
    Dot,
    Hyphen,
    Underscore,
}

// One `tokenize` output element: a word (carrying its byte range in the
// input) or a single separator character.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Token {
    Word {
        kind: TokenType,
        start: usize,
        end: usize,
    },
    Separator(Separator),
}

// Builds the token for one scanned word from its first character's class and
// the flags accumulated while scanning it.
fn create_token(
    first: CharType,
    has_lowercase: bool,
    has_uppercase: bool,
    start: usize,
    end: usize,
) -> Token {
    match first {
        CharType::Lowercase => {
            // A lowercase-started word cannot contain an uppercase-class
            // character: (Lower, Upper) transitions always split the word.
            assert!(!has_uppercase);
            Token::Word {
                kind: TokenType::Lowercase,
                start,
                end,
            }
        }
        CharType::Uppercase => {
            // Both flags can never be set on one word: the character that
            // would set the second flag splits the word first.
            assert!(!(has_lowercase && has_uppercase));

            if has_lowercase {
                // Rest are lowercase
                Token::Word {
                    kind: TokenType::Capitalized,
                    start,
                    end,
                }
            } else {
                // All uppercase
                Token::Word {
                    kind: TokenType::Uppercase,
                    start,
                    end,
                }
            }
        }
        CharType::Dot => Token::Separator(Separator::Dot),
        CharType::Hyphen => Token::Separator(Separator::Hyphen),
        CharType::Underscore => Token::Separator(Separator::Underscore),
    }
}

// Splits the input into words and separators. A new word starts at each
// separator and at class transitions: lower→upper always splits ("fooBar" →
// foo|Bar), upper→upper splits only when a lowercase run follows
// ("HTTPResponse" → HTTP|Response), and upper→lower keeps the word going
// (the tail of a Capitalized word). Returns `None` when any byte has no
// class — the trigger for `encode`'s MD5 fallback.
fn tokenize(s: &[u8]) -> Option<Vec<Token>> {
    let mut tokens = Vec::new();

    if s.is_empty() {
        return Some(tokens);
    }

    let mut start = 0;
    let mut chars = s.iter().enumerate().peekable();
    let mut prev_type: Option<CharType> = None;

    // track the current word characteristics
    let mut first_type: Option<CharType> = None;
    let mut has_lowercase = false;
    let mut has_uppercase = false;

    while let Some((i, &ch)) = chars.next() {
        let curr_type = char_type(ch)?;

        let Some(prev) = prev_type else {
            // first character of the string
            first_type = Some(curr_type);
            prev_type = Some(curr_type);
            continue;
        };

        let should_split = match (prev, curr_type) {
            // special characters are always single tokens
            (CharType::Dot, _) | (CharType::Underscore, _) | (CharType::Hyphen, _) => true,
            (_, CharType::Dot) | (_, CharType::Underscore) | (_, CharType::Hyphen) => true,

            // same type - check for special cases
            (CharType::Uppercase, CharType::Uppercase) => {
                // Check if next char is lowercase: "HTTPResponse" -> split between 'P' and 'R'
                if let Some(&(_, &next_ch)) = chars.peek() {
                    matches!(char_type(next_ch)?, CharType::Lowercase)
                } else {
                    false
                }
            }
            (CharType::Lowercase, CharType::Lowercase) => false,

            // Uppercase to Lowercase can be Capitalized - don't split yet
            (CharType::Uppercase, CharType::Lowercase) => {
                // has_uppercase/has_lowercase are always both false at this
                // transition: the uppercase run before the lowercase tail
                // already split the word and reset the flags, so this never
                // splits — a Capitalized word just keeps absorbing.
                has_uppercase && has_lowercase
            }

            // different types - split
            _ => true,
        };

        if should_split {
            // create word based on what we've seen
            let token = create_token(first_type?, has_lowercase, has_uppercase, start, i);
            tokens.push(token);

            // reset tracking for new word
            start = i;
            first_type = Some(curr_type);
            has_lowercase = false;
            has_uppercase = false;
        } else {
            // continue current word, update tracking
            if curr_type == CharType::Lowercase {
                has_lowercase = true;
            } else if curr_type == CharType::Uppercase {
                has_uppercase = true;
            }
        }

        prev_type = Some(curr_type);
    }

    // add the last word
    if start < s.len() {
        // Belt-and-braces guard: the loop above already validated every byte
        // via `char_type(ch)?`, so this always passes.
        if !s[start..].iter().all(|ch| char_type(*ch).is_some()) {
            return None;
        };

        let token = create_token(first_type?, has_lowercase, has_uppercase, start, s.len());
        tokens.push(token);
    }

    Some(tokens)
}

// The field shapes `parse` assembles from words: single-case runs and the
// two camel shapes. `Empty` marks a leading, doubled or trailing separator.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Field<'a> {
    Lowercase(&'a [u8]),
    Uppercase(&'a [u8]),
    LowerCamel(&'a [u8]),
    UpperCamel(&'a [u8]),
    Empty,
}

// `parse` output: one field or one separator, in input order.
#[derive(Debug, Clone, PartialEq, Eq)]
enum Node<'a> {
    Field(Field<'a>),
    Separator(Separator),
}

// The builder's view of a field's shape (`Empty` needs no builder).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum FieldType {
    Lowercase,
    Uppercase,
    LowerCamel,
    UpperCamel,
}

// Accumulates consecutive words into one field. A word joins the current
// field only when the (field type, word type) pair composes (`can_add`); the
// one special case turns a lone lowercase word followed by Capitalized words
// into a LowerCamel field ("helloWorld"); anything else finishes the current
// field and starts a new one.
#[derive(Debug, Clone, Copy)]
struct FieldBuilder {
    field_type: FieldType,
    start: usize,
    end: usize,
    extended: bool,
}

impl FieldBuilder {
    fn new(field_type: FieldType, start: usize, end: usize) -> Self {
        Self {
            field_type,
            start,
            end,
            extended: false,
        }
    }

    fn can_add(&self, word_type: TokenType) -> bool {
        matches!(
            (self.field_type, word_type),
            (FieldType::Lowercase, TokenType::Lowercase)
                | (FieldType::Uppercase, TokenType::Uppercase)
                | (FieldType::LowerCamel, TokenType::Capitalized)
                | (FieldType::UpperCamel, TokenType::Capitalized)
        )
    }

    fn extend_to(&mut self, end: usize) {
        self.end = end;
        self.extended = true;
    }

    fn transition_to_lower_camel(&mut self) {
        self.field_type = FieldType::LowerCamel;
    }

    fn is_single_lowercase(&self) -> bool {
        self.field_type == FieldType::Lowercase && !self.extended
    }

    fn into_field<'a>(self, source: &'a [u8]) -> Field<'a> {
        let slice = &source[self.start..self.end];

        match self.field_type {
            FieldType::Lowercase => Field::Lowercase(slice),
            FieldType::Uppercase => Field::Uppercase(slice),
            FieldType::LowerCamel => Field::LowerCamel(slice),
            FieldType::UpperCamel => Field::UpperCamel(slice),
        }
    }
}

// Unused helper (dead code kept with an allow): unwraps a word token's kind.
#[allow(dead_code)]
fn token_type(word: &Token) -> TokenType {
    match word {
        Token::Word { kind, .. } => *kind,
        _ => unreachable!(),
    }
}

// Groups tokens into fields: same-shape words merge (see `FieldBuilder`),
// separators finish the current field, and a leading separator, two
// consecutive separators, or a trailing separator each contribute an `Empty`
// field so the surrounding shape is preserved. Output order matches input.
fn parse<'a>(source: &'a [u8], tokens: &[Token]) -> Vec<Node<'a>> {
    let mut nodes = Vec::new();

    // handle leading empty field
    if matches!(tokens.first(), Some(Token::Separator(_))) {
        nodes.push(Node::Field(Field::Empty));
    }

    let mut field_builder: Option<FieldBuilder> = None;

    for i in 0..tokens.len() {
        match tokens[i] {
            Token::Separator(sep) => {
                // finish current field if any
                if let Some(field) = field_builder.take() {
                    nodes.push(Node::Field(field.into_field(source)));
                }

                nodes.push(Node::Separator(sep));

                // check for empty field (consecutive separators or trailing separator)
                if i + 1 >= tokens.len() || matches!(tokens[i + 1], Token::Separator(_)) {
                    nodes.push(Node::Field(Field::Empty));
                }
            }
            Token::Word {
                kind: wtype,
                start,
                end,
            } => {
                if let Some(ref mut field) = field_builder {
                    if field.can_add(wtype) {
                        // we can extend the field with this word type
                        field.extend_to(end);
                    } else if field.is_single_lowercase() && wtype == TokenType::Capitalized {
                        // transition from single lowercase to LowerCamel
                        field.transition_to_lower_camel();
                        field.extend_to(end);
                    } else {
                        // finish current field and start new one
                        let finished_field = field_builder.take().unwrap();
                        nodes.push(Node::Field(finished_field.into_field(source)));

                        let field_type = match wtype {
                            TokenType::Lowercase => FieldType::Lowercase,
                            TokenType::Uppercase => FieldType::Uppercase,
                            TokenType::Capitalized => FieldType::UpperCamel,
                        };
                        field_builder = Some(FieldBuilder::new(field_type, start, end));
                    }
                } else {
                    // no current field builder, start a new one
                    let field_type = match wtype {
                        TokenType::Lowercase => FieldType::Lowercase,
                        TokenType::Uppercase => FieldType::Uppercase,
                        TokenType::Capitalized => FieldType::UpperCamel,
                    };
                    field_builder = Some(FieldBuilder::new(field_type, start, end));
                }
            }
        }
    }

    // finish any remaining field
    if let Some(field) = field_builder {
        nodes.push(Node::Field(field.into_field(source)));
    }

    nodes
}

// The structure alphabet: one character per (field shape, what follows) —
// the following separator kind, another field with no separator in between,
// or end of input. 24 codes (a-x): five per field shape plus Empty's four
// (u-x), since an `Empty` field only ever faces a separator. `encode_full`
// uppercases these to A-X.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum FieldSeparatorPair {
    // Lowercase field followed by...
    LowercaseDot,
    LowercaseUnderscore,
    LowercaseHyphen,
    LowercaseNoSep,
    LowercaseEnd,

    // LowerCamel field followed by...
    LowerCamelDot,
    LowerCamelUnderscore,
    LowerCamelHyphen,
    LowerCamelNoSep,
    LowerCamelEnd,

    // UpperCamel field followed by...
    UpperCamelDot,
    UpperCamelUnderscore,
    UpperCamelHyphen,
    UpperCamelNoSep,
    UpperCamelEnd,

    // Uppercase field followed by...
    UppercaseDot,
    UppercaseUnderscore,
    UppercaseHyphen,
    UppercaseNoSep,
    UppercaseEnd,

    // Empty field followed by...
    EmptyDot,
    EmptyUnderscore,
    EmptyHyphen,
    EmptyEnd,
}

// The a-x code table; `from_char` below is its (currently unused) inverse.
impl FieldSeparatorPair {
    fn to_char(self) -> char {
        match self {
            // Lowercase (a-e)
            FieldSeparatorPair::LowercaseDot => 'a',
            FieldSeparatorPair::LowercaseUnderscore => 'b',
            FieldSeparatorPair::LowercaseHyphen => 'c',
            FieldSeparatorPair::LowercaseNoSep => 'd',
            FieldSeparatorPair::LowercaseEnd => 'e',

            // LowerCamel (f-j)
            FieldSeparatorPair::LowerCamelDot => 'f',
            FieldSeparatorPair::LowerCamelUnderscore => 'g',
            FieldSeparatorPair::LowerCamelHyphen => 'h',
            FieldSeparatorPair::LowerCamelNoSep => 'i',
            FieldSeparatorPair::LowerCamelEnd => 'j',

            // UpperCamel (k-o)
            FieldSeparatorPair::UpperCamelDot => 'k',
            FieldSeparatorPair::UpperCamelUnderscore => 'l',
            FieldSeparatorPair::UpperCamelHyphen => 'm',
            FieldSeparatorPair::UpperCamelNoSep => 'n',
            FieldSeparatorPair::UpperCamelEnd => 'o',

            // Uppercase (p-t)
            FieldSeparatorPair::UppercaseDot => 'p',
            FieldSeparatorPair::UppercaseUnderscore => 'q',
            FieldSeparatorPair::UppercaseHyphen => 'r',
            FieldSeparatorPair::UppercaseNoSep => 's',
            FieldSeparatorPair::UppercaseEnd => 't',

            // Empty (u-x)
            FieldSeparatorPair::EmptyDot => 'u',
            FieldSeparatorPair::EmptyUnderscore => 'v',
            FieldSeparatorPair::EmptyHyphen => 'w',
            FieldSeparatorPair::EmptyEnd => 'x',
        }
    }

    // Inverse of `to_char`; never called (dead code kept with an allow).
    #[allow(dead_code)]
    fn from_char(c: char) -> Option<Self> {
        match c {
            'a' => Some(FieldSeparatorPair::LowercaseDot),
            'b' => Some(FieldSeparatorPair::LowercaseUnderscore),
            'c' => Some(FieldSeparatorPair::LowercaseHyphen),
            'd' => Some(FieldSeparatorPair::LowercaseNoSep),
            'e' => Some(FieldSeparatorPair::LowercaseEnd),

            'f' => Some(FieldSeparatorPair::LowerCamelDot),
            'g' => Some(FieldSeparatorPair::LowerCamelUnderscore),
            'h' => Some(FieldSeparatorPair::LowerCamelHyphen),
            'i' => Some(FieldSeparatorPair::LowerCamelNoSep),
            'j' => Some(FieldSeparatorPair::LowerCamelEnd),

            'k' => Some(FieldSeparatorPair::UpperCamelDot),
            'l' => Some(FieldSeparatorPair::UpperCamelUnderscore),
            'm' => Some(FieldSeparatorPair::UpperCamelHyphen),
            'n' => Some(FieldSeparatorPair::UpperCamelNoSep),
            'o' => Some(FieldSeparatorPair::UpperCamelEnd),

            'p' => Some(FieldSeparatorPair::UppercaseDot),
            'q' => Some(FieldSeparatorPair::UppercaseUnderscore),
            'r' => Some(FieldSeparatorPair::UppercaseHyphen),
            's' => Some(FieldSeparatorPair::UppercaseNoSep),
            't' => Some(FieldSeparatorPair::UppercaseEnd),

            'u' => Some(FieldSeparatorPair::EmptyDot),
            'v' => Some(FieldSeparatorPair::EmptyUnderscore),
            'w' => Some(FieldSeparatorPair::EmptyHyphen),
            'x' => Some(FieldSeparatorPair::EmptyEnd),

            _ => None,
        }
    }
}

// 2-character checksum over the whole input string, via std's DefaultHasher:
// index 0-25 maps to A-Z, 26-35 to 0-9. Distinct inputs can collide, and std
// gives no cross-release stability guarantee for DefaultHasher — a toolchain
// update can change the checksums of the names persisted in journals.
fn compute_checksum(s: &str) -> String {
    use std::hash::{DefaultHasher, Hash, Hasher};

    let mut hasher = DefaultHasher::new();
    s.hash(&mut hasher);
    let hash = hasher.finish();

    // Two character checksum for 36^2 = 1,296 possible values
    let first_idx = ((hash / 36) % 36) as usize;
    let second_idx = (hash % 36) as usize;

    let first_char = if first_idx < 26 {
        (b'A' + first_idx as u8) as char
    } else {
        (b'0' + (first_idx - 26) as u8) as char
    };

    let second_char = if second_idx < 26 {
        (b'A' + second_idx as u8) as char
    } else {
        (b'0' + (second_idx - 26) as u8) as char
    };

    format!("{}{}", first_char, second_char)
}

/// Detects the checksum prefix by first character. Only called on `encode`
/// output that already passed the `ND_` fallback check in `encode_full`, so
/// an uppercase/digit first character can only be a checksum: structure
/// characters are lowercase a-x.
fn has_checksum(encoded: &str) -> bool {
    if let Some(first_char) = encoded.chars().next() {
        // Checksum uses A-Z and 0-9, structure encoding uses a-x
        first_char.is_ascii_uppercase() || first_char.is_ascii_digit()
    } else {
        false
    }
}

// Renders the structure encoding: one lowercase a-x character per
// field(+separator) pair. A 2-character checksum prefixes the output when
// any field is camelCase — `has_checksum` and `encode_full` rely on the
// resulting first-character split. Consecutive separators survive as
// Empty+separator pairs, so separator kinds and counts are preserved.
fn encode_nodes(source: &str, nodes: &[Node]) -> String {
    // Check if any field is camel case
    let has_camel_case = nodes.iter().any(|node| {
        matches!(
            node,
            Node::Field(Field::LowerCamel(_)) | Node::Field(Field::UpperCamel(_))
        )
    });

    let mut result = String::new();

    // Add checksum at the beginning if there's any camel case
    if has_camel_case {
        result.push_str(&compute_checksum(source));
    }

    let mut i = 0;
    while i < nodes.len() {
        if let Node::Field(field) = &nodes[i] {
            // Look ahead to see what follows this field
            let next_is_separator =
                i + 1 < nodes.len() && matches!(nodes[i + 1], Node::Separator(_));
            let next_is_field = i + 1 < nodes.len() && matches!(nodes[i + 1], Node::Field(_));

            let pair = match field {
                Field::Lowercase(_) => {
                    if next_is_separator {
                        match nodes[i + 1] {
                            Node::Separator(Separator::Dot) => FieldSeparatorPair::LowercaseDot,
                            Node::Separator(Separator::Underscore) => {
                                FieldSeparatorPair::LowercaseUnderscore
                            }
                            Node::Separator(Separator::Hyphen) => {
                                FieldSeparatorPair::LowercaseHyphen
                            }
                            _ => unreachable!(),
                        }
                    } else if next_is_field {
                        FieldSeparatorPair::LowercaseNoSep
                    } else {
                        FieldSeparatorPair::LowercaseEnd
                    }
                }
                Field::LowerCamel(_) => {
                    if next_is_separator {
                        match nodes[i + 1] {
                            Node::Separator(Separator::Dot) => FieldSeparatorPair::LowerCamelDot,
                            Node::Separator(Separator::Underscore) => {
                                FieldSeparatorPair::LowerCamelUnderscore
                            }
                            Node::Separator(Separator::Hyphen) => {
                                FieldSeparatorPair::LowerCamelHyphen
                            }
                            _ => unreachable!(),
                        }
                    } else if next_is_field {
                        FieldSeparatorPair::LowerCamelNoSep
                    } else {
                        FieldSeparatorPair::LowerCamelEnd
                    }
                }
                Field::UpperCamel(_) => {
                    if next_is_separator {
                        match nodes[i + 1] {
                            Node::Separator(Separator::Dot) => FieldSeparatorPair::UpperCamelDot,
                            Node::Separator(Separator::Underscore) => {
                                FieldSeparatorPair::UpperCamelUnderscore
                            }
                            Node::Separator(Separator::Hyphen) => {
                                FieldSeparatorPair::UpperCamelHyphen
                            }
                            _ => unreachable!(),
                        }
                    } else if next_is_field {
                        FieldSeparatorPair::UpperCamelNoSep
                    } else {
                        FieldSeparatorPair::UpperCamelEnd
                    }
                }
                Field::Uppercase(_) => {
                    if next_is_separator {
                        match nodes[i + 1] {
                            Node::Separator(Separator::Dot) => FieldSeparatorPair::UppercaseDot,
                            Node::Separator(Separator::Underscore) => {
                                FieldSeparatorPair::UppercaseUnderscore
                            }
                            Node::Separator(Separator::Hyphen) => {
                                FieldSeparatorPair::UppercaseHyphen
                            }
                            _ => unreachable!(),
                        }
                    } else if next_is_field {
                        FieldSeparatorPair::UppercaseNoSep
                    } else {
                        FieldSeparatorPair::UppercaseEnd
                    }
                }
                Field::Empty => {
                    if next_is_separator {
                        match nodes[i + 1] {
                            Node::Separator(Separator::Dot) => FieldSeparatorPair::EmptyDot,
                            Node::Separator(Separator::Underscore) => {
                                FieldSeparatorPair::EmptyUnderscore
                            }
                            Node::Separator(Separator::Hyphen) => FieldSeparatorPair::EmptyHyphen,
                            _ => unreachable!(),
                        }
                    } else {
                        FieldSeparatorPair::EmptyEnd
                    }
                }
            };

            result.push(pair.to_char());

            // Skip the separator if we just encoded it
            if next_is_separator {
                i += 2; // Skip field + separator
            } else {
                i += 1; // Skip just the field
            }
        } else {
            // This shouldn't happen if nodes are well-formed
            // (fields and separators should alternate)
            i += 1;
        }
    }

    result
}

/// The private core of `encode_full`: encodes a tokenizable field name into
/// its compact structure representation — lowercase a-x characters capturing
/// field shapes and separators, with a 2-character checksum prefix (A-Z,
/// 0-9) when the name has camelCase fields. Camel-case word boundaries
/// collapse within a field ("HelloWorld" and "Helloworld" share a structure
/// character); the checksum over the raw input keeps distinct names apart
/// except for 1-in-1296 collisions. Inputs that cannot be tokenized (any
/// byte outside [A-Za-z0-9._-], including non-UTF-8) take the MD5 fallback
/// instead: `ND_` + 32 uppercase hex characters (35 bytes). The structure
/// encoding of an empty input is the empty string.
fn encode(b: &[u8]) -> String {
    let Some(tokens) = tokenize(b) else {
        let digest = md5::compute(b);
        return format!("{}{:X}", REMAPPED_PREFIX, digest);
    };

    // SAFETY: `tokenize` returning `Some` proves every input byte passed
    // `char_type`, i.e. the input is ASCII over [A-Za-z0-9._-] — valid UTF-8.
    let s = unsafe { str::from_utf8_unchecked(b) };

    let nodes = parse(b, &tokens);
    encode_nodes(s, &nodes)
}

/// Run-length-compresses runs of 3 or more identical characters as
/// `count` + character, splitting runs longer than 9 into 9-character chunks
/// with a literal remainder; runs of 1-2 stay literal. Only the structure
/// encoding goes through this (the checksum is exempt — see `encode_full`),
/// and its characters are a-x, so the digits in the output are unambiguously
/// run counts. Pinned by `tests::compress_runs_examples_match_private_contract`.
fn compress_runs(s: &str) -> String {
    if s.is_empty() {
        return String::new();
    }

    let mut result = String::new();
    let chars: Vec<char> = s.chars().collect();
    let mut i = 0;

    while i < chars.len() {
        let ch = chars[i];
        let mut count = 1;

        // Count consecutive identical characters
        while i + count < chars.len() && chars[i + count] == ch {
            count += 1;
        }

        // Process the run
        if count <= 2 {
            // Output as-is for runs of 1 or 2
            for _ in 0..count {
                result.push(ch);
            }
        } else {
            // Compress runs of 3+
            let mut remaining = count;
            while remaining > 0 {
                if remaining > 9 {
                    result.push('9');
                    result.push(ch);
                    remaining -= 9;
                } else if remaining > 2 {
                    result.push(char::from_digit(remaining as u32, 10).unwrap());
                    result.push(ch);
                    remaining = 0;
                } else {
                    // Output remaining 1 or 2 characters as-is
                    for _ in 0..remaining {
                        result.push(ch);
                    }
                    remaining = 0;
                }
            }
        }

        i += count;
    }

    result
}

/// Encodes a field name into a systemd-journal-compatible field name: the
/// single public entry point of this crate. journal-log-writer calls it for
/// every field name that is not already journald-compatible
/// (`journal-log-writer/src/log/mod.rs` `write_entry_with_timestamps`).
///
/// The result has one of two shapes:
///
/// - Normal: `ND<checksum?><compressed-structure>_<NORMALIZED>` — the `ND`
///   (Netdata) prefix, the optional 2-character checksum (present only for
///   names with camelCase fields), the run-length-compressed structure
///   encoding uppercased (A-X letters with 0-9 run counts), an underscore,
///   and the original name uppercased with `.` and `-` mapped to `_` plus
///   the common prefixes shortened: `RESOURCE_ATTRIBUTES_` → `RA_`,
///   `LOG_ATTRIBUTES_` → `LA_`, `LOG_BODY_` → `LB_` (matched on the
///   uppercased name, so dotted and underscored spellings both hit).
/// - MD5 fallback: `ND_` + 32 uppercase hex characters = 35 bytes — used
///   when the input is not valid UTF-8, contains any byte outside
///   [A-Za-z0-9._-], or the normal shape would exceed 64 bytes (journald's
///   field-name limit). The result is always ≤ 64 bytes and
///   journald-compatible.
///
/// The checksum is computed over the raw input and is never compressed
/// (see `compute_checksum` for its stability caveats). The mapping is
/// deterministic for a given toolchain but not idempotent: re-encoding an
/// output produces a different name (the normalized part keeps growing, e.g.
/// `NDE_HELLO` → `NDQT_NDE_HELLO`). Edge case: an empty input yields the
/// 3-byte `ND_` (empty structure and empty normalized name), which resembles
/// but is not the MD5 fallback shape.
///
/// # Examples
///
/// ```
/// use rdp::encode_full;
///
/// // Simple lowercase field
/// assert_eq!(encode_full(b"hello"), "NDE_HELLO");
///
/// // With dot separators - no compression (only 2 consecutive a's)
/// assert_eq!(encode_full(b"log.body.hostname"), "NDAAE_LB_HOSTNAME");
///
/// // Many nested levels - structure compression (10 a's → 9a + a)
/// assert_eq!(encode_full(b"my.very.deeply.nested.field.that.ends.in.the.abyss"), "ND9AE_MY_VERY_DEEPLY_NESTED_FIELD_THAT_ENDS_IN_THE_ABYSS");
///
/// // With camel case (includes checksum - not compressed)
/// let full = encode_full(b"log.body.HostName");
/// assert!(full.starts_with("ND83AAO_")); // ND + 2-char checksum + structure
/// assert!(full.ends_with("LB_HOSTNAME")); // normalized field name
///
/// // With hyphens
/// assert_eq!(encode_full(b"hello-world"), "NDCE_HELLO_WORLD");
///
/// // With resource.attributes prefix - compression (3 a's → 3a)
/// assert_eq!(encode_full(b"resource.attributes.host.name"), "ND3AE_RA_HOST_NAME");
///
/// // With invalid characters (space) - falls back to MD5
/// let md5_result = encode_full(b"field name");
/// assert!(md5_result.starts_with("ND_"));
/// assert_eq!(md5_result.len(), 35); // ND_ + 32 hex chars
///
/// // Non-UTF8 - falls back to MD5
/// let non_utf8 = b"\xFF\xFE invalid";
/// let result = encode_full(non_utf8);
/// assert!(result.starts_with("ND_"));
/// assert_eq!(result.len(), 35);
///
/// // Long names that would exceed 64 bytes - falls back to MD5
/// let long_name = b"very.long.deeply.nested.field.name.that.would.definitely.exceed.the.systemd.limit";
/// let result = encode_full(long_name);
/// assert!(result.starts_with("ND_"));
/// assert!(result.len() <= 64);
/// ```
pub fn encode_full(field_name: &[u8]) -> String {
    let encoded = encode(field_name);
    if encoded.starts_with(REMAPPED_PREFIX) {
        return encoded;
    }

    // Compress runs in the structure encoding (but not the checksum)
    let compressed = if has_checksum(&encoded) {
        // Keep checksum as-is, compress the rest
        let checksum = &encoded[..2];
        let structure = &encoded[2..];
        format!("{}{}", checksum, compress_runs(structure))
    } else {
        // No checksum, compress everything
        compress_runs(&encoded)
    };

    // SAFETY: reaching this line means `encode` took the structure path, so
    // every input byte passed `char_type` — the input is ASCII, hence valid
    // UTF-8.
    let s = unsafe { String::from_utf8_unchecked(field_name.to_vec()) };
    let mut normalized = s.to_uppercase().replace(['.', '-'], "_");

    // Replace common prefixes with shorter versions
    if let Some(suffix) = normalized.strip_prefix("RESOURCE_ATTRIBUTES_") {
        normalized = format!("RA_{}", suffix);
    } else if let Some(suffix) = normalized.strip_prefix("LOG_ATTRIBUTES_") {
        normalized = format!("LA_{}", suffix);
    } else if let Some(suffix) = normalized.strip_prefix("LOG_BODY_") {
        normalized = format!("LB_{}", suffix);
    }

    let result = format!("ND{}_{}", compressed.to_uppercase(), normalized);

    // If the result exceeds systemd's 64-byte limit, fall back to MD5
    if result.len() > 64 {
        let digest = md5::compute(field_name);
        return format!("{}{:X}", REMAPPED_PREFIX, digest);
    }

    result
}

// Pinned-output contract tests: these values are the documented behavior of
// `encode_full`, `encode` and `compress_runs` — keep them in sync with the
// doc comments (the `encode_full` doctest duplicates the third test here).
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn encode_examples_match_private_contract() {
        assert_eq!(encode(b"hello"), "e");

        let encoded = encode(b"helloWorld");
        assert_eq!(encoded.len(), 3);

        let encoded = encode(b"log.body.HostName");
        assert_eq!(encoded.len(), 5);
    }

    #[test]
    fn compress_runs_examples_match_private_contract() {
        assert_eq!(compress_runs("aaa"), "3a");
        assert_eq!(compress_runs("aaaaaaaaaa"), "9aa");
        assert_eq!(compress_runs("aaaaaaaaaaaa"), "9a3a");
        assert_eq!(compress_runs("aabbbcc"), "aa3bcc");
    }

    #[test]
    fn encode_full_examples_match_public_contract() {
        assert_eq!(encode_full(b"hello"), "NDE_HELLO");
        assert_eq!(encode_full(b"log.body.hostname"), "NDAAE_LB_HOSTNAME");
        assert_eq!(
            encode_full(b"my.very.deeply.nested.field.that.ends.in.the.abyss"),
            "ND9AE_MY_VERY_DEEPLY_NESTED_FIELD_THAT_ENDS_IN_THE_ABYSS"
        );

        let full = encode_full(b"log.body.HostName");
        assert!(full.starts_with("ND83AAO_"));
        assert!(full.ends_with("LB_HOSTNAME"));

        assert_eq!(encode_full(b"hello-world"), "NDCE_HELLO_WORLD");
        assert_eq!(
            encode_full(b"resource.attributes.host.name"),
            "ND3AE_RA_HOST_NAME"
        );

        let md5_result = encode_full(b"field name");
        assert!(md5_result.starts_with("ND_"));
        assert_eq!(md5_result.len(), 35);

        let non_utf8 = b"\xFF\xFE invalid";
        let result = encode_full(non_utf8);
        assert!(result.starts_with("ND_"));
        assert_eq!(result.len(), 35);

        let long_name =
            b"very.long.deeply.nested.field.name.that.would.definitely.exceed.the.systemd.limit";
        let result = encode_full(long_name);
        assert!(result.starts_with("ND_"));
        assert!(result.len() <= 64);
    }
}
