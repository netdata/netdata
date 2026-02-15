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

fn is_lowercase(s: &[u8], i: usize) -> bool {
    char_type(s[i]) == Some(CharType::Lowercase)
}

fn is_uppercase(s: &[u8], i: usize) -> bool {
    char_type(s[i]) == Some(CharType::Uppercase)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum TokenType {
    Lowercase,
    Uppercase,
    Capitalized,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Separator {
    Dot,
    Hyphen,
    Underscore,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Token {
    Word {
        kind: TokenType,
        start: usize,
        end: usize,
    },
    Separator(Separator),
}

/// Tokenizes a byte string into words and separators.
///
/// Words are classified as `Lowercase`, `Uppercase`, or `Capitalized`.
/// Separators are `.`, `_`, or `-`. CamelCase boundaries are detected
/// (e.g. `"HTTPResponse"` splits into `["HTTP", "Response"]`).
///
/// Returns `None` if the input contains invalid characters.
pub(crate) fn tokenize(s: &[u8]) -> Option<arrayvec::ArrayVec<Token, 64>> {
    let mut tokens = arrayvec::ArrayVec::new();
    let mut i = 0;

    while i < s.len() {
        match char_type(s[i])? {
            CharType::Dot => {
                tokens.push(Token::Separator(Separator::Dot));
                i += 1;
            }
            CharType::Underscore => {
                tokens.push(Token::Separator(Separator::Underscore));
                i += 1;
            }
            CharType::Hyphen => {
                tokens.push(Token::Separator(Separator::Hyphen));
                i += 1;
            }
            CharType::Lowercase => {
                let start = i;
                i += 1;
                while i < s.len() && is_lowercase(s, i) {
                    i += 1;
                }
                tokens.push(Token::Word {
                    kind: TokenType::Lowercase,
                    start,
                    end: i,
                });
            }
            CharType::Uppercase => {
                let start = i;
                i += 1;

                // Consume uppercase, but stop before one followed by lowercase
                // ("HTTPResponse" → "HTTP" | "Response")
                while i < s.len() && is_uppercase(s, i) {
                    if i + 1 < s.len() && is_lowercase(s, i + 1) {
                        break;
                    }
                    i += 1;
                }

                if i == start + 1 && i < s.len() && is_lowercase(s, i) {
                    // Single uppercase + lowercase → Capitalized ("Hello", "Bar")
                    while i < s.len() && is_lowercase(s, i) {
                        i += 1;
                    }
                    tokens.push(Token::Word {
                        kind: TokenType::Capitalized,
                        start,
                        end: i,
                    });
                } else {
                    // Pure uppercase ("HTTP", "HELLO", "A")
                    tokens.push(Token::Word {
                        kind: TokenType::Uppercase,
                        start,
                        end: i,
                    });
                }
            }
        }
    }

    Some(tokens)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty() {
        assert_eq!(tokenize(b"").unwrap().as_slice(), &[]);
    }

    #[test]
    fn single_lowercase_word() {
        assert_eq!(
            tokenize(b"hello").unwrap().as_slice(),
            &[Token::Word {
                kind: TokenType::Lowercase,
                start: 0,
                end: 5,
            }]
        );
    }

    #[test]
    fn single_uppercase_word() {
        assert_eq!(
            tokenize(b"HTTP").unwrap().as_slice(),
            &[Token::Word {
                kind: TokenType::Uppercase,
                start: 0,
                end: 4,
            }]
        );
    }

    #[test]
    fn single_capitalized_word() {
        assert_eq!(
            tokenize(b"Hello").unwrap().as_slice(),
            &[Token::Word {
                kind: TokenType::Capitalized,
                start: 0,
                end: 5,
            }]
        );
    }

    #[test]
    fn dot_separated() {
        assert_eq!(
            tokenize(b"foo.bar").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 0,
                    end: 3,
                },
                Token::Separator(Separator::Dot),
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 4,
                    end: 7,
                },
            ]
        );
    }

    #[test]
    fn underscore_separated() {
        assert_eq!(
            tokenize(b"foo_bar").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 0,
                    end: 3,
                },
                Token::Separator(Separator::Underscore),
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 4,
                    end: 7,
                },
            ]
        );
    }

    #[test]
    fn hyphen_separated() {
        assert_eq!(
            tokenize(b"foo-bar").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 0,
                    end: 3,
                },
                Token::Separator(Separator::Hyphen),
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 4,
                    end: 7,
                },
            ]
        );
    }

    #[test]
    fn lower_camel_case() {
        // "fooBar" → lowercase "foo" + capitalized "Bar"
        assert_eq!(
            tokenize(b"fooBar").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 0,
                    end: 3,
                },
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 3,
                    end: 6,
                },
            ]
        );
    }

    #[test]
    fn upper_camel_case() {
        // "FooBar" → capitalized "Foo" + capitalized "Bar"
        assert_eq!(
            tokenize(b"FooBar").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 0,
                    end: 3,
                },
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 3,
                    end: 6,
                },
            ]
        );
    }

    #[test]
    fn http_response_split() {
        // "HTTPResponse" → uppercase "HTTP" + capitalized "Response"
        assert_eq!(
            tokenize(b"HTTPResponse").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Uppercase,
                    start: 0,
                    end: 4,
                },
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 4,
                    end: 12,
                },
            ]
        );
    }

    #[test]
    fn invalid_character_returns_none() {
        assert!(tokenize(b"hello world").is_none());
        assert!(tokenize(b"foo@bar").is_none());
        assert!(tokenize(b"\xff").is_none());
    }

    #[test]
    fn mixed_real_world() {
        // "log.body.HostName" → low "log" . low "body" . cap "Host" cap "Name"
        assert_eq!(
            tokenize(b"log.body.HostName").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 0,
                    end: 3,
                },
                Token::Separator(Separator::Dot),
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 4,
                    end: 8,
                },
                Token::Separator(Separator::Dot),
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 9,
                    end: 13,
                },
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 13,
                    end: 17,
                },
            ]
        );
    }

    #[test]
    fn digits_treated_as_uppercase() {
        // "OAuth2Token" → "O" (up) + "Auth" (cap) + "2" (up) + "Token" (cap)
        // O→A splits because next char (u) is lowercase
        // 2 is uppercase-class, so it splits from "Auth"
        assert_eq!(
            tokenize(b"OAuth2Token").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Uppercase,
                    start: 0,
                    end: 1,
                },
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 1,
                    end: 5,
                },
                Token::Word {
                    kind: TokenType::Uppercase,
                    start: 5,
                    end: 6,
                },
                Token::Word {
                    kind: TokenType::Capitalized,
                    start: 6,
                    end: 11,
                },
            ]
        );
    }

    #[test]
    fn single_separator() {
        assert_eq!(
            tokenize(b".").unwrap().as_slice(),
            &[Token::Separator(Separator::Dot)]
        );
    }

    #[test]
    fn consecutive_separators() {
        assert_eq!(
            tokenize(b"..").unwrap().as_slice(),
            &[
                Token::Separator(Separator::Dot),
                Token::Separator(Separator::Dot),
            ]
        );
    }

    #[test]
    fn single_char_words() {
        assert_eq!(
            tokenize(b"a.B").unwrap().as_slice(),
            &[
                Token::Word {
                    kind: TokenType::Lowercase,
                    start: 0,
                    end: 1,
                },
                Token::Separator(Separator::Dot),
                Token::Word {
                    kind: TokenType::Uppercase,
                    start: 2,
                    end: 3,
                },
            ]
        );
    }
}
