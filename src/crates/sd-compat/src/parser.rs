use crate::tokenizer::{Token, TokenType};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Field {
    Lowercase,
    Uppercase,
    LowerCamel,
    UpperCamel,
    Empty,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Node {
    Field(Field),
    Separator(crate::tokenizer::Separator),
}

#[derive(Debug, Clone, Copy)]
struct FieldBuilder {
    field: Field,
    extended: bool,
}

impl FieldBuilder {
    fn new(field: Field) -> Self {
        Self {
            field,
            extended: false,
        }
    }

    fn can_add(&self, word_type: TokenType) -> bool {
        matches!(
            (self.field, word_type),
            (Field::Lowercase, TokenType::Lowercase)
                | (Field::Uppercase, TokenType::Uppercase)
                | (Field::LowerCamel, TokenType::Capitalized)
                | (Field::UpperCamel, TokenType::Capitalized)
        )
    }

    fn extend(&mut self) {
        self.extended = true;
    }

    fn is_single_lowercase(&self) -> bool {
        self.field == Field::Lowercase && !self.extended
    }

    fn transition_to_lower_camel(&mut self) {
        self.field = Field::LowerCamel;
    }
}

fn field_from_token_type(wtype: TokenType) -> Field {
    match wtype {
        TokenType::Lowercase => Field::Lowercase,
        TokenType::Uppercase => Field::Uppercase,
        TokenType::Capitalized => Field::UpperCamel,
    }
}

/// Groups tokens into fields and separators.
///
/// Consecutive words of compatible types are merged into a single field:
/// - Lowercase words → `Lowercase` field
/// - Uppercase words → `Uppercase` field
/// - Capitalized words → `UpperCamel` field
/// - Single lowercase + Capitalized → `LowerCamel` field
///
/// Empty fields are inserted for leading, trailing, or consecutive separators.
pub(crate) fn parse(tokens: &[Token]) -> arrayvec::ArrayVec<Node, 64> {
    let mut nodes = arrayvec::ArrayVec::new();

    // handle leading empty field
    if matches!(tokens.first(), Some(Token::Separator(_))) {
        nodes.push(Node::Field(Field::Empty));
    }

    let mut builder: Option<FieldBuilder> = None;

    for i in 0..tokens.len() {
        match tokens[i] {
            Token::Separator(sep) => {
                // finish current field if any
                if let Some(b) = builder.take() {
                    nodes.push(Node::Field(b.field));
                }

                nodes.push(Node::Separator(sep));

                // check for empty field (consecutive separators or trailing separator)
                if i + 1 >= tokens.len() || matches!(tokens[i + 1], Token::Separator(_)) {
                    nodes.push(Node::Field(Field::Empty));
                }
            }
            Token::Word { kind: wtype, .. } => {
                if let Some(ref mut b) = builder {
                    if b.can_add(wtype) {
                        b.extend();
                    } else if b.is_single_lowercase() && wtype == TokenType::Capitalized {
                        b.transition_to_lower_camel();
                        b.extend();
                    } else {
                        // finish current field and start new one
                        nodes.push(Node::Field(b.field));
                        builder = Some(FieldBuilder::new(field_from_token_type(wtype)));
                    }
                } else {
                    builder = Some(FieldBuilder::new(field_from_token_type(wtype)));
                }
            }
        }
    }

    // finish any remaining field
    if let Some(b) = builder {
        nodes.push(Node::Field(b.field));
    }

    nodes
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tokenizer::{Separator, tokenize};

    fn parse_str(s: &str) -> arrayvec::ArrayVec<Node, 64> {
        let tokens = tokenize(s.as_bytes()).unwrap();
        parse(&tokens)
    }

    #[test]
    fn empty() {
        assert_eq!(parse_str("").as_slice(), &[]);
    }

    #[test]
    fn single_lowercase() {
        assert_eq!(
            parse_str("hello").as_slice(),
            &[Node::Field(Field::Lowercase)]
        );
    }

    #[test]
    fn single_uppercase() {
        assert_eq!(
            parse_str("HTTP").as_slice(),
            &[Node::Field(Field::Uppercase)]
        );
    }

    #[test]
    fn single_capitalized_becomes_upper_camel() {
        assert_eq!(
            parse_str("Hello").as_slice(),
            &[Node::Field(Field::UpperCamel)]
        );
    }

    #[test]
    fn dot_separated_lowercase() {
        assert_eq!(
            parse_str("foo.bar").as_slice(),
            &[
                Node::Field(Field::Lowercase),
                Node::Separator(Separator::Dot),
                Node::Field(Field::Lowercase),
            ]
        );
    }

    #[test]
    fn upper_camel_merges_capitalized_words() {
        // "HostName" → tokenizer gives [Cap "Host", Cap "Name"]
        // parser merges them into a single UpperCamel field
        assert_eq!(
            parse_str("HostName").as_slice(),
            &[Node::Field(Field::UpperCamel)]
        );
    }

    #[test]
    fn lower_camel_from_lowercase_plus_capitalized() {
        // "fooBar" → tokenizer gives [Low "foo", Cap "Bar"]
        // parser: single lowercase + capitalized → LowerCamel
        assert_eq!(
            parse_str("fooBar").as_slice(),
            &[Node::Field(Field::LowerCamel)]
        );
    }

    #[test]
    fn upper_then_capitalized_stays_separate() {
        // "HTTPResponse" → tokenizer gives [Up "HTTP", Cap "Response"]
        // parser: uppercase can't merge with capitalized → two fields
        assert_eq!(
            parse_str("HTTPResponse").as_slice(),
            &[
                Node::Field(Field::Uppercase),
                Node::Field(Field::UpperCamel),
            ]
        );
    }

    #[test]
    fn real_world_log_body_hostname() {
        assert_eq!(
            parse_str("log.body.HostName").as_slice(),
            &[
                Node::Field(Field::Lowercase),
                Node::Separator(Separator::Dot),
                Node::Field(Field::Lowercase),
                Node::Separator(Separator::Dot),
                Node::Field(Field::UpperCamel),
            ]
        );
    }

    #[test]
    fn leading_separator_inserts_empty() {
        assert_eq!(
            parse_str(".foo").as_slice(),
            &[
                Node::Field(Field::Empty),
                Node::Separator(Separator::Dot),
                Node::Field(Field::Lowercase),
            ]
        );
    }

    #[test]
    fn trailing_separator_inserts_empty() {
        assert_eq!(
            parse_str("foo.").as_slice(),
            &[
                Node::Field(Field::Lowercase),
                Node::Separator(Separator::Dot),
                Node::Field(Field::Empty),
            ]
        );
    }

    #[test]
    fn consecutive_separators_insert_empty() {
        assert_eq!(
            parse_str("foo..bar").as_slice(),
            &[
                Node::Field(Field::Lowercase),
                Node::Separator(Separator::Dot),
                Node::Field(Field::Empty),
                Node::Separator(Separator::Dot),
                Node::Field(Field::Lowercase),
            ]
        );
    }

    #[test]
    fn hyphen_and_underscore() {
        assert_eq!(
            parse_str("foo-bar_baz").as_slice(),
            &[
                Node::Field(Field::Lowercase),
                Node::Separator(Separator::Hyphen),
                Node::Field(Field::Lowercase),
                Node::Separator(Separator::Underscore),
                Node::Field(Field::Lowercase),
            ]
        );
    }

    #[test]
    fn mixed_camel_with_separators() {
        assert_eq!(
            parse_str("serviceURL.Host").as_slice(),
            &[
                Node::Field(Field::Lowercase),
                Node::Field(Field::Uppercase),
                Node::Separator(Separator::Dot),
                Node::Field(Field::UpperCamel),
            ]
        );
    }
}
