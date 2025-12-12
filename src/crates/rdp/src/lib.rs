// String tokenizer and parser for field names with compact encoding
//
// Tokenizes strings into words (lowercase, UPPERCASE, Capitalized) and separators (. _ -)
// Parses tokens into fields: Lowercase, Uppercase, LowerCamel, UpperCamel, Empty
// Encodes token stream into compact lossless representation
//
// Example: "log.body.HostName" → encoded: "C3aao" (5 chars: 2-char checksum + 3-char structure)

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum CharType {
    Lowercase,
    Uppercase,
    Dot,
    Underscore,
    Hyphen,
}

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

/// Prefix for field names that were remapped due to containing invalid characters.
const REMAPPED_PREFIX: &str = "ND_";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum TokenType {
    Lowercase,
    Uppercase,
    Capitalized,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Separator {
    Dot,
    Hyphen,
    Underscore,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Token {
    Word {
        kind: TokenType,
        start: usize,
        end: usize,
    },
    Separator(Separator),
}

fn create_token(
    first: CharType,
    has_lowercase: bool,
    has_uppercase: bool,
    start: usize,
    end: usize,
) -> Token {
    match first {
        CharType::Lowercase => {
            assert!(!has_uppercase);
            Token::Word {
                kind: TokenType::Lowercase,
                start,
                end,
            }
        }
        CharType::Uppercase => {
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
                // only continue if we're at the first transition (potential Capitalized word)
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
        // make sure the rest of the word contains valid chars
        if !s[start..].iter().all(|ch| char_type(*ch).is_some()) {
            return None;
        };

        let token = create_token(first_type?, has_lowercase, has_uppercase, start, s.len());
        tokens.push(token);
    }

    Some(tokens)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Field<'a> {
    Lowercase(&'a [u8]),
    Uppercase(&'a [u8]),
    LowerCamel(&'a [u8]),
    UpperCamel(&'a [u8]),
    Empty,
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum Node<'a> {
    Field(Field<'a>),
    Separator(Separator),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum FieldType {
    Lowercase,
    Uppercase,
    LowerCamel,
    UpperCamel,
}

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

fn token_type(word: &Token) -> TokenType {
    match word {
        Token::Word { kind, .. } => *kind,
        _ => unreachable!(),
    }
}

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

/// Returns true if the encoded string contains a checksum prefix.
/// Checksums are added for strings containing camel case fields.
fn has_checksum(encoded: &str) -> bool {
    if let Some(first_char) = encoded.chars().next() {
        // Checksum uses A-Z and 0-9, structure encoding uses a-x
        first_char.is_ascii_uppercase() || first_char.is_ascii_digit()
    } else {
        false
    }
}

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

/// Encodes a key string into a compact representation.
///
/// The encoding is lossless for all practical naming conventions and includes:
/// - Structure encoding: captures field types (lowercase, UPPERCASE, camelCase) and separators (. _ -)
/// - Checksum: 2-character prefix (A-Z, 0-9) added for strings with camel case fields
///
/// # Examples
///
/// ```
/// use rdp::encode;
///
/// // Simple lowercase field - no checksum
/// assert_eq!(encode("hello"), "e");
///
/// // Camel case - includes 2-char checksum
/// let encoded = encode("helloWorld");
/// assert_eq!(encoded.len(), 3); // 2-char checksum + 1-char structure
///
/// // Complex field with camel case
/// let encoded = encode("log.body.HostName");
/// assert_eq!(encoded.len(), 5); // 2-char checksum + 3-char structure
/// ```
fn encode(b: &[u8]) -> String {
    let Some(tokens) = tokenize(b) else {
        let digest = md5::compute(b);
        return format!("{}{:X}", REMAPPED_PREFIX, digest);
    };

    // SAFETY: We'll only get tokens when we have valid ASCII characters in
    // the input byte slice.
    let s = unsafe { str::from_utf8_unchecked(b) };

    let nodes = parse(b, &tokens);
    encode_nodes(s, &nodes)
}

/// Compresses runs of 3 or more consecutive identical characters using run-length encoding.
///
/// Runs are encoded as count + character, with a maximum count of 9 per segment.
/// For runs longer than 9, multiple segments are created.
///
/// # Examples
///
/// ```
/// # use rdp::compress_runs;
/// assert_eq!(compress_runs("aaa"), "3a");
/// assert_eq!(compress_runs("aaaaaaaaaa"), "9aa");  // 10 a's → 9a + a
/// assert_eq!(compress_runs("aaaaaaaaaaaa"), "9a3a");  // 12 a's → 9a + 3a
/// assert_eq!(compress_runs("aabbbcc"), "aa3bcc");
/// ```
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

/// Returns the fully encoded key: ND prefix + compressed uppercase structure encoding + normalized key.
///
/// The result combines:
/// - "ND" prefix (Netdata namespace identifier)
/// - The compact structure encoding with run-length compression and uppercased
/// - An underscore separator
/// - The original key converted to uppercase with dots and hyphens replaced by underscores,
///   and common prefixes shortened:
///   - "RESOURCE_ATTRIBUTES_" → "RA_"
///   - "LOG_ATTRIBUTES_" → "LA_"
///   - "LOG_BODY_" → "LB_"
///
/// Run-length compression replaces 3+ consecutive identical characters with count + character.
/// The checksum (first 2 characters if present) is never compressed.
///
/// # MD5 Fallback
///
/// Falls back to `ND_<32-hex-chars>` format when:
/// - Input is not valid UTF-8
/// - Input contains invalid characters (anything except a-z, A-Z, '.', '-', '_')
/// - Encoded result would exceed 64 bytes (systemd's field name limit)
///
/// The result is guaranteed to be systemd-compatible and ≤ 64 bytes.
///
/// # Examples
///
/// ```
/// use rdp::encode_full;
///
/// // Simple lowercase field
/// assert_eq!(encode_full(b"hello"), "HELLO_NDE");
///
/// // With dot separators - no compression (only 2 consecutive a's)
/// assert_eq!(encode_full(b"log.body.hostname"), "LB_HOSTNAME_NDAAE");
///
/// // Many nested levels - structure compression (10 a's → 9a + a)
/// assert_eq!(encode_full(b"my.very.deeply.nested.field.that.ends.in.the.abyss"), "MY_VERY_DEEPLY_NESTED_FIELD_THAT_ENDS_IN_THE_ABYSS_ND9AE");
///
/// // With camel case (includes checksum - not compressed)
/// let full = encode_full(b"log.body.HostName");
/// assert!(full.starts_with("LB_HOSTNAME_ND")); // prefix + normalized + ND + checksum
/// assert!(full.ends_with("83AAO")); // 2-char checksum + structure (2 a's not compressed)
///
/// // With hyphens
/// assert_eq!(encode_full(b"hello-world"), "HELLO_WORLD_NDCE");
///
/// // With resource.attributes prefix - compression (3 a's → 3a)
/// assert_eq!(encode_full(b"resource.attributes.host.name"), "RA_HOST_NAME_ND3AE");
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
