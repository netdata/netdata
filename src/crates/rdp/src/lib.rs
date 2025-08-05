// String tokenizer and parser for field names with compact encoding
//
// Tokenizes strings into words (lowercase, UPPERCASE, Capitalized) and separators (. _ -)
// Parses tokens into fields: Lowercase, Uppercase, LowerCamel, UpperCamel, Empty
// Encodes token stream into compact lossless representation
//
// Example: "log.body.HostName" → encoded: "C3aao" (5 chars: 2-char checksum + 3-char structure)

fn is_uppercase_like(c: char) -> bool {
    c.is_uppercase() || c.is_ascii_digit()
}

fn is_lowercase(c: char) -> bool {
    c.is_lowercase()
}

fn is_dot(c: char) -> bool {
    c == '.'
}

fn is_underscore(c: char) -> bool {
    c == '_'
}

fn is_hyphen(c: char) -> bool {
    c == '-'
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum CharType {
    Lowercase,
    Uppercase,
    Dot,
    Underscore,
    Hyphen,
}

fn char_type(c: char) -> Option<CharType> {
    if is_lowercase(c) {
        Some(CharType::Lowercase)
    } else if is_uppercase_like(c) {
        Some(CharType::Uppercase)
    } else if is_dot(c) {
        Some(CharType::Dot)
    } else if is_underscore(c) {
        Some(CharType::Underscore)
    } else if is_hyphen(c) {
        Some(CharType::Hyphen)
    } else {
        None
    }
}

/// Checks if all characters in the string are valid for rdp encoding.
///
/// Valid characters are: letters (a-z, A-Z), dots (.), underscores (_), and hyphens (-).
fn has_only_valid_chars(s: &str) -> bool {
    s.chars().all(|c| char_type(c).is_some())
}

/// Prefix for field names that were remapped due to containing invalid characters.
const REMAPPED_PREFIX: &str = "ND_";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Word<'a> {
    Lowercase(&'a str),
    Uppercase(&'a str),
    Capitalized(&'a str),
    Dot(&'a str),
    Underscore(&'a str),
    Hyphen(&'a str),
}

impl<'a> Word<'a> {
    #[allow(dead_code)]
    fn as_str(&self) -> &'a str {
        match self {
            Word::Lowercase(s)
            | Word::Uppercase(s)
            | Word::Capitalized(s)
            | Word::Dot(s)
            | Word::Underscore(s)
            | Word::Hyphen(s) => s,
        }
    }
}

fn tokenize<'a>(s: &'a str) -> Vec<Word<'a>> {
    // Validate that all characters are valid
    debug_assert!(
        s.chars().all(|c| char_type(c).is_some()),
        "Input string contains invalid characters"
    );

    let mut result = Vec::new();

    if s.is_empty() {
        return result;
    }

    let mut start = 0;
    let mut chars = s.char_indices().peekable();
    let mut prev_type: Option<CharType> = None;

    // Track the current word characteristics
    let mut first_type: Option<CharType> = None;
    let mut has_lowercase = false;
    let mut has_uppercase = false;

    while let Some((i, ch)) = chars.next() {
        let curr_type = char_type(ch).unwrap();

        if let Some(prev) = prev_type {
            let should_split = match (prev, curr_type) {
                // Special characters are always single tokens
                (CharType::Dot, _) | (CharType::Underscore, _) | (CharType::Hyphen, _) => true,
                (_, CharType::Dot) | (_, CharType::Underscore) | (_, CharType::Hyphen) => true,

                // Same type - check for special cases
                (CharType::Uppercase, CharType::Uppercase) => {
                    // Check if next char is lowercase: "HTTPResponse" -> split between 'P' and 'R'
                    if let Some(&(_, next_ch)) = chars.peek() {
                        matches!(char_type(next_ch), Some(CharType::Lowercase))
                    } else {
                        false
                    }
                }
                (CharType::Lowercase, CharType::Lowercase) => false,

                // Uppercase to Lowercase can be Capitalized - don't split yet
                (CharType::Uppercase, CharType::Lowercase) => {
                    // Only continue if we're at the first transition (potential Capitalized word)
                    has_uppercase && has_lowercase
                }

                // Different types - split
                _ => true,
            };

            if should_split {
                // Create word based on what we've seen
                if let Some(word) = create_word(
                    &s[start..i],
                    first_type.unwrap(),
                    has_lowercase,
                    has_uppercase,
                ) {
                    result.push(word);
                }

                // Reset tracking for new word
                start = i;
                first_type = Some(curr_type);
                has_lowercase = false;
                has_uppercase = false;
            } else {
                // Continue current word, update tracking
                if curr_type == CharType::Lowercase {
                    has_lowercase = true;
                } else if curr_type == CharType::Uppercase {
                    has_uppercase = true;
                }
            }
        } else {
            // First character of the string
            first_type = Some(curr_type);
            has_lowercase = false;
            has_uppercase = false;
        }

        prev_type = Some(curr_type);
    }

    // Add the last word
    if start < s.len() {
        if let Some(word) = create_word(
            &s[start..],
            first_type.unwrap(),
            has_lowercase,
            has_uppercase,
        ) {
            result.push(word);
        }
    }

    result
}

fn create_word<'a>(
    s: &'a str,
    first: CharType,
    has_lowercase: bool,
    has_uppercase: bool,
) -> Option<Word<'a>> {
    match first {
        CharType::Lowercase => {
            // If first is lowercase, all must be lowercase
            if has_uppercase {
                None
            } else {
                Some(Word::Lowercase(s))
            }
        }
        CharType::Uppercase => {
            // First is uppercase
            if has_lowercase && has_uppercase {
                // Mixed case after first char - invalid
                None
            } else if has_lowercase {
                // Rest are lowercase - Capitalized
                Some(Word::Capitalized(s))
            } else {
                // All uppercase - Uppercase
                Some(Word::Uppercase(s))
            }
        }
        CharType::Dot => Some(Word::Dot(s)),
        CharType::Underscore => Some(Word::Underscore(s)),
        CharType::Hyphen => Some(Word::Hyphen(s)),
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Field<'a> {
    Lowercase(&'a str),
    Uppercase(&'a str),
    LowerCamel(&'a str),
    UpperCamel(&'a str),
    Empty,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Separator {
    Dot,
    Hyphen,
    Underscore,
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

    fn can_add(&self, word_type: WordType) -> bool {
        matches!(
            (self.field_type, word_type),
            (FieldType::Lowercase, WordType::Lowercase)
                | (FieldType::Uppercase, WordType::Uppercase)
                | (FieldType::LowerCamel, WordType::Capitalized)
                | (FieldType::UpperCamel, WordType::Capitalized)
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

    fn into_field<'a>(self, source: &'a str) -> Field<'a> {
        let slice = &source[self.start..self.end];
        match self.field_type {
            FieldType::Lowercase => Field::Lowercase(slice),
            FieldType::Uppercase => Field::Uppercase(slice),
            FieldType::LowerCamel => Field::LowerCamel(slice),
            FieldType::UpperCamel => Field::UpperCamel(slice),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum WordType {
    Lowercase,
    Uppercase,
    Capitalized,
}

fn word_type(word: &Word) -> WordType {
    match word {
        Word::Lowercase(_) => WordType::Lowercase,
        Word::Uppercase(_) => WordType::Uppercase,
        Word::Capitalized(_) => WordType::Capitalized,
        _ => unreachable!(),
    }
}

fn word_byte_range(source: &str, word: &Word) -> (usize, usize) {
    let word_str = match word {
        Word::Lowercase(s) | Word::Uppercase(s) | Word::Capitalized(s) => s,
        _ => unreachable!(),
    };
    let start = word_str.as_ptr() as usize - source.as_ptr() as usize;
    let end = start + word_str.len();
    (start, end)
}

fn parse<'a>(source: &'a str, tokens: &[Word<'a>]) -> Vec<Node<'a>> {
    let mut result = Vec::new();
    let mut i = 0;

    // Handle leading empty field
    if i < tokens.len() && is_separator(&tokens[i]) {
        result.push(Node::Field(Field::Empty));
    }

    let mut current_field: Option<FieldBuilder> = None;

    while i < tokens.len() {
        // Check if current position is a separator
        if let Some(sep) = token_to_separator(&tokens[i]) {
            // Finish current field if any
            if let Some(field) = current_field.take() {
                result.push(Node::Field(field.into_field(source)));
            }

            result.push(Node::Separator(sep));
            i += 1;

            // Check for empty field (consecutive separators or trailing separator)
            if i >= tokens.len() || is_separator(&tokens[i]) {
                result.push(Node::Field(Field::Empty));
            }
            continue;
        }

        // Process a word token
        let word = &tokens[i];
        let wtype = word_type(word);
        let (start, end) = word_byte_range(source, word);

        if let Some(ref mut field) = current_field {
            // Check if we can add this word to the current field
            if field.can_add(wtype) {
                field.extend_to(end);
            } else {
                // Can't add, so check if we can transition to LowerCamel
                let should_transition =
                    field.is_single_lowercase() && wtype == WordType::Capitalized;

                if should_transition {
                    // Transition from single lowercase to LowerCamel
                    field.transition_to_lower_camel();
                    field.extend_to(end);
                    i += 1;
                    continue;
                }

                // Otherwise, finish current field and start new one
                let finished_field = current_field.take().unwrap();
                result.push(Node::Field(finished_field.into_field(source)));

                let field_type = match wtype {
                    WordType::Lowercase => FieldType::Lowercase,
                    WordType::Uppercase => FieldType::Uppercase,
                    WordType::Capitalized => FieldType::UpperCamel,
                };
                current_field = Some(FieldBuilder::new(field_type, start, end));
            }
        } else {
            // No current field, start a new one
            let field_type = match wtype {
                WordType::Lowercase => FieldType::Lowercase,
                WordType::Uppercase => FieldType::Uppercase,
                WordType::Capitalized => FieldType::UpperCamel,
            };
            current_field = Some(FieldBuilder::new(field_type, start, end));
        }

        i += 1;
    }

    // Finish any remaining field
    if let Some(field) = current_field {
        result.push(Node::Field(field.into_field(source)));
    }

    result
}

fn is_separator(word: &Word) -> bool {
    matches!(word, Word::Dot(_) | Word::Hyphen(_) | Word::Underscore(_))
}

fn token_to_separator(word: &Word) -> Option<Separator> {
    match word {
        Word::Dot(_) => Some(Separator::Dot),
        Word::Hyphen(_) => Some(Separator::Hyphen),
        Word::Underscore(_) => Some(Separator::Underscore),
        _ => None,
    }
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
pub fn has_checksum(encoded: &str) -> bool {
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
pub fn encode(s: &str) -> String {
    let tokens = tokenize(s);
    let nodes = parse(s, &tokens);
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
pub fn compress_runs(s: &str) -> String {
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
    // Try to parse as UTF-8, fall back to MD5 if not valid
    let s = match std::str::from_utf8(field_name) {
        Ok(s) => s,
        Err(_) => {
            let digest = md5::compute(field_name);
            return format!("{}{:X}", REMAPPED_PREFIX, digest);
        }
    };

    // Check if the string contains only valid characters for rdp encoding
    // If not, fall back to MD5 hash
    if !has_only_valid_chars(s) {
        let digest = md5::compute(field_name);
        return format!("{}{:X}", REMAPPED_PREFIX, digest);
    }

    let encoded = encode(s);

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

/// Tests that the structure encoding is lossless by verifying node types are preserved.
///
/// Returns Ok(()) if the round-trip test passes, or Err with a description if it fails.
pub fn verify_structure_round_trip(s: &str) -> Result<(), String> {
    // Parse to get original node types
    let tokens = tokenize(s);
    let original_nodes = parse(s, &tokens);
    let original_types = node_types(&original_nodes);

    // Encode structure
    let encoded = encode_nodes(s, &original_nodes);

    // Decode structure back to nodes
    let decoded_nodes = decode_structure(&encoded);
    let decoded_types = node_types(&decoded_nodes);

    // Compare
    if original_types == decoded_types {
        Ok(())
    } else {
        Err(format!(
            "Structure round-trip failed for '{}'\nOriginal: {:?}\nDecoded: {:?}",
            s, original_types, decoded_types
        ))
    }
}

// Helper function to extract node types (ignoring content)
fn node_types(nodes: &[Node]) -> Vec<String> {
    nodes
        .iter()
        .map(|node| match node {
            Node::Field(Field::Lowercase(_)) => "Field::Lowercase".to_string(),
            Node::Field(Field::Uppercase(_)) => "Field::Uppercase".to_string(),
            Node::Field(Field::LowerCamel(_)) => "Field::LowerCamel".to_string(),
            Node::Field(Field::UpperCamel(_)) => "Field::UpperCamel".to_string(),
            Node::Field(Field::Empty) => "Field::Empty".to_string(),
            Node::Separator(Separator::Dot) => "Separator::Dot".to_string(),
            Node::Separator(Separator::Underscore) => "Separator::Underscore".to_string(),
            Node::Separator(Separator::Hyphen) => "Separator::Hyphen".to_string(),
        })
        .collect()
}

// Decode structure back to nodes (reconstructing from encoded structure)
fn decode_structure(structure: &str) -> Vec<Node<'_>> {
    let mut nodes = Vec::new();

    for ch in structure.chars() {
        let pair = FieldSeparatorPair::from_char(ch);
        if pair.is_none() {
            continue;
        }
        let pair = pair.unwrap();

        // Determine field type and separator
        let (field_type, separator) = match pair {
            FieldSeparatorPair::LowercaseDot => (Some(Field::Lowercase("")), Some(Separator::Dot)),
            FieldSeparatorPair::LowercaseUnderscore => {
                (Some(Field::Lowercase("")), Some(Separator::Underscore))
            }
            FieldSeparatorPair::LowercaseHyphen => {
                (Some(Field::Lowercase("")), Some(Separator::Hyphen))
            }
            FieldSeparatorPair::LowercaseNoSep => (Some(Field::Lowercase("")), None),
            FieldSeparatorPair::LowercaseEnd => (Some(Field::Lowercase("")), None),

            FieldSeparatorPair::LowerCamelDot => {
                (Some(Field::LowerCamel("")), Some(Separator::Dot))
            }
            FieldSeparatorPair::LowerCamelUnderscore => {
                (Some(Field::LowerCamel("")), Some(Separator::Underscore))
            }
            FieldSeparatorPair::LowerCamelHyphen => {
                (Some(Field::LowerCamel("")), Some(Separator::Hyphen))
            }
            FieldSeparatorPair::LowerCamelNoSep => (Some(Field::LowerCamel("")), None),
            FieldSeparatorPair::LowerCamelEnd => (Some(Field::LowerCamel("")), None),

            FieldSeparatorPair::UpperCamelDot => {
                (Some(Field::UpperCamel("")), Some(Separator::Dot))
            }
            FieldSeparatorPair::UpperCamelUnderscore => {
                (Some(Field::UpperCamel("")), Some(Separator::Underscore))
            }
            FieldSeparatorPair::UpperCamelHyphen => {
                (Some(Field::UpperCamel("")), Some(Separator::Hyphen))
            }
            FieldSeparatorPair::UpperCamelNoSep => (Some(Field::UpperCamel("")), None),
            FieldSeparatorPair::UpperCamelEnd => (Some(Field::UpperCamel("")), None),

            FieldSeparatorPair::UppercaseDot => (Some(Field::Uppercase("")), Some(Separator::Dot)),
            FieldSeparatorPair::UppercaseUnderscore => {
                (Some(Field::Uppercase("")), Some(Separator::Underscore))
            }
            FieldSeparatorPair::UppercaseHyphen => {
                (Some(Field::Uppercase("")), Some(Separator::Hyphen))
            }
            FieldSeparatorPair::UppercaseNoSep => (Some(Field::Uppercase("")), None),
            FieldSeparatorPair::UppercaseEnd => (Some(Field::Uppercase("")), None),

            FieldSeparatorPair::EmptyDot => (Some(Field::Empty), Some(Separator::Dot)),
            FieldSeparatorPair::EmptyUnderscore => {
                (Some(Field::Empty), Some(Separator::Underscore))
            }
            FieldSeparatorPair::EmptyHyphen => (Some(Field::Empty), Some(Separator::Hyphen)),
            FieldSeparatorPair::EmptyEnd => (Some(Field::Empty), None),
        };

        if let Some(field) = field_type {
            nodes.push(Node::Field(field));
        }
        if let Some(sep) = separator {
            nodes.push(Node::Separator(sep));
        }
    }

    nodes
}

#[cfg(test)]
mod tests {
    use super::*;
    use proptest::prelude::*;

    #[test]
    fn test_structure_round_trip() {
        let test_cases = vec![
            "hello",
            "HELLO",
            "Hello",
            "helloWorld",
            "HelloWorld",
            "hello.world",
            "hello_world",
            "hello-world",
            "HELLO.WORLD",
            "log.body.hostname",
            "LOG.BODY.HOSTNAME",
            "log.body.HostName",
            "parseHTMLString",
            ".hello",
            "hello.",
            "hello..world",
        ];

        for key in test_cases {
            verify_structure_round_trip(key).unwrap();
        }
    }

    proptest! {
        #[test]
        fn prop_structure_round_trip(s in "[a-zA-Z0-9._-]{1,3}") {
            prop_assert!(verify_structure_round_trip(&s).is_ok());
        }
    }
}
