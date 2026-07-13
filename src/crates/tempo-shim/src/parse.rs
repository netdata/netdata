//! Recursive-descent parser for the form-generated filter grammar
//! (verified against grafana-tempo-datasource @ 95f2697; the pinned
//! generated strings live in the golden tests below):
//!
//! ```text
//! query    := '{' [ section (' && ' section)* ] '}'
//! section  := term
//!           | '(' term (' || ' term)+ ')'        // multi-value = / =~
//!           | '(' term (' && ' term)+ ')'        // multi-value != / !~
//! term     := field op value
//! field    := (resource|span|event|instrumentation|link) '.' tag
//!           | '.' tag                             // unscoped attribute
//!           | tag                                 // bare ad-hoc field (unscoped)
//!           | intrinsic                           // incl. colon forms
//! op       := = | != | > | < | >= | <= | =~ | !~
//! value    := "string" | number | duration | bare keyword/identifier
//! ```
//!
//! A parenthesized section is the plugin's multi-value rendering of ONE
//! filter: every term shares the field and operator, and the values
//! fold into one engine [`Condition`] (whose multi-value semantics —
//! `=`/`=~` OR, `!=`/`!~` AND — are exactly the generated shapes).
//!
//! Everything outside this grammar — pipelines, structural operators,
//! aggregates, the `parent.` scope, non-double-quoted strings — is a
//! typed [`ParseError`] (→ HTTP 400), never a silent misparse.

use sfsq::traces::{CompareOp, Condition, Predicate, PredicateTarget, PredicateValue, TraceIntrinsic};

use crate::duration::parse_duration_ns;
use crate::error::ParseError;
use crate::keywords::{
    KIND_ALLOWED, STATUS_ALLOWED, kind_keyword_to_storage, resolve_field,
    status_keyword_to_storage,
};

/// Parse a Tempo `q` filter string into the engine predicate. An absent
/// or blank `q` and the empty form `{}` are both [`Predicate::all`] (the
/// plugin omits `q` entirely when the raw editor is empty).
pub fn parse_query(q: &str) -> Result<Predicate, ParseError> {
    let tokens = lex(q)?;
    if tokens.is_empty() {
        return Ok(Predicate::all());
    }
    let mut parser = Parser { tokens, at: 0 };
    let predicate = parser.query()?;
    Ok(predicate)
}

// ── Lexer ───────────────────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq)]
enum Tok {
    LBrace,
    RBrace,
    LParen,
    RParen,
    And,
    Or,
    Op(CompareOp),
    /// A double-quoted string, unescaped per the wire rule: `\"` → `"`,
    /// `\\` → `\`, any other `\x` preserved verbatim (the form emits
    /// regex-escaped values like `\\(`; hand-written patterns carry
    /// bare `\s` — neither is JSON).
    Str(String),
    /// A bare token: field names, keywords, numbers, durations, hex ids.
    Word(String),
}

#[derive(Debug, Clone)]
struct Spanned {
    tok: Tok,
    pos: usize,
}

fn lex(input: &str) -> Result<Vec<Spanned>, ParseError> {
    let mut out = Vec::new();
    let mut chars = input.char_indices().peekable();
    while let Some((pos, c)) = chars.next() {
        let tok = match c {
            c if c.is_whitespace() => continue,
            '{' => Tok::LBrace,
            '}' => Tok::RBrace,
            '(' => Tok::LParen,
            ')' => Tok::RParen,
            '&' => match chars.peek() {
                Some((_, '&')) => {
                    chars.next();
                    Tok::And
                }
                _ => {
                    return Err(ParseError::Malformed {
                        pos,
                        what: "expected '&&'".to_string(),
                    });
                }
            },
            '|' => match chars.peek() {
                Some((_, '|')) => {
                    chars.next();
                    Tok::Or
                }
                _ => {
                    return Err(ParseError::Unsupported {
                        pos,
                        what: "the pipeline operator '|'".to_string(),
                    });
                }
            },
            '=' => match chars.peek() {
                Some((_, '~')) => {
                    chars.next();
                    Tok::Op(CompareOp::Regex)
                }
                _ => Tok::Op(CompareOp::Eq),
            },
            '!' => match chars.peek() {
                Some((_, '~')) => {
                    chars.next();
                    Tok::Op(CompareOp::NotRegex)
                }
                Some((_, '=')) => {
                    chars.next();
                    Tok::Op(CompareOp::NotEq)
                }
                _ => {
                    return Err(ParseError::Malformed {
                        pos,
                        what: "expected '!=' or '!~'".to_string(),
                    });
                }
            },
            '>' => match chars.peek() {
                Some((_, '=')) => {
                    chars.next();
                    Tok::Op(CompareOp::Gte)
                }
                Some((_, '>')) => {
                    return Err(ParseError::Unsupported {
                        pos,
                        what: "the structural operator '>>'".to_string(),
                    });
                }
                _ => Tok::Op(CompareOp::Gt),
            },
            '<' => match chars.peek() {
                Some((_, '=')) => {
                    chars.next();
                    Tok::Op(CompareOp::Lte)
                }
                Some((_, '<')) => {
                    return Err(ParseError::Unsupported {
                        pos,
                        what: "the structural operator '<<'".to_string(),
                    });
                }
                _ => Tok::Op(CompareOp::Lt),
            },
            '~' => {
                return Err(ParseError::Unsupported {
                    pos,
                    what: "the structural operator '~'".to_string(),
                });
            }
            '"' => {
                let mut s = String::new();
                loop {
                    match chars.next() {
                        None => {
                            return Err(ParseError::Malformed {
                                pos,
                                what: "unterminated string".to_string(),
                            });
                        }
                        Some((_, '"')) => break,
                        Some((esc_pos, '\\')) => match chars.next() {
                            None => {
                                return Err(ParseError::Malformed {
                                    pos: esc_pos,
                                    what: "unterminated string escape".to_string(),
                                });
                            }
                            Some((_, '"')) => s.push('"'),
                            Some((_, '\\')) => s.push('\\'),
                            Some((_, other)) => {
                                s.push('\\');
                                s.push(other);
                            }
                        },
                        Some((_, c)) => s.push(c),
                    }
                }
                Tok::Str(s)
            }
            '\'' | '`' => {
                return Err(ParseError::Unsupported {
                    pos,
                    what: format!("{c}-quoted strings; use double quotes"),
                });
            }
            c if is_word_char(c) => {
                let mut w = String::new();
                w.push(c);
                while let Some((_, next)) = chars.peek() {
                    if is_word_char(*next) {
                        w.push(*next);
                        chars.next();
                    } else {
                        break;
                    }
                }
                Tok::Word(w)
            }
            other => {
                return Err(ParseError::Malformed {
                    pos,
                    what: format!("unexpected character {other:?}"),
                });
            }
        };
        out.push(Spanned { tok, pos });
    }
    Ok(out)
}

/// Bare-token characters: attribute keys (dots, slashes, dashes,
/// underscores), colon-form intrinsics, numbers, durations (µ included
/// via `is_alphanumeric`).
fn is_word_char(c: char) -> bool {
    c.is_alphanumeric() || matches!(c, '_' | '.' | ':' | '/' | '-')
}

// ── Parser ──────────────────────────────────────────────────────────

struct Parser {
    tokens: Vec<Spanned>,
    at: usize,
}

impl Parser {
    fn peek(&self) -> Option<&Spanned> {
        self.tokens.get(self.at)
    }

    fn next(&mut self) -> Option<Spanned> {
        let t = self.tokens.get(self.at).cloned();
        if t.is_some() {
            self.at += 1;
        }
        t
    }

    fn end_pos(&self) -> usize {
        self.tokens.last().map(|t| t.pos + 1).unwrap_or(0)
    }

    fn query(&mut self) -> Result<Predicate, ParseError> {
        match self.next() {
            Some(Spanned { tok: Tok::LBrace, .. }) => {}
            Some(Spanned { pos, .. }) => {
                return Err(ParseError::Malformed {
                    pos,
                    what: "expected '{' opening the filter".to_string(),
                });
            }
            None => unreachable!("parse_query returns early on empty token streams"),
        }
        let mut conditions = Vec::new();
        if !matches!(self.peek().map(|t| &t.tok), Some(Tok::RBrace)) {
            loop {
                self.section(&mut conditions)?;
                match self.next() {
                    Some(Spanned { tok: Tok::And, .. }) => continue,
                    Some(Spanned { tok: Tok::RBrace, .. }) => break,
                    Some(Spanned { tok: Tok::Or, pos }) => {
                        return Err(ParseError::Unsupported {
                            pos,
                            what: "'||' between top-level filters (the form only \
                                   generates it inside a multi-value group)"
                                .to_string(),
                        });
                    }
                    Some(Spanned { pos, .. }) => {
                        return Err(ParseError::Malformed {
                            pos,
                            what: "expected '&&' or '}'".to_string(),
                        });
                    }
                    None => {
                        return Err(ParseError::Malformed {
                            pos: self.end_pos(),
                            what: "missing closing '}'".to_string(),
                        });
                    }
                }
            }
        } else {
            self.next(); // the RBrace of the empty form `{}`
        }
        if let Some(t) = self.peek() {
            return Err(ParseError::Unsupported {
                pos: t.pos,
                what: "content after the closing '}' (pipelines, structural \
                       operators, and aggregates are not implemented)"
                    .to_string(),
            });
        }
        Ok(Predicate { conditions })
    }

    /// One section: a bare term, or a parenthesized multi-value group
    /// folding into one condition.
    fn section(&mut self, conditions: &mut Vec<Condition>) -> Result<(), ParseError> {
        if !matches!(self.peek().map(|t| &t.tok), Some(Tok::LParen)) {
            conditions.push(self.term()?);
            return Ok(());
        }
        let open = self.next().expect("peeked LParen");
        let mut merged = self.term()?;
        let sep = match self.next() {
            Some(Spanned { tok: Tok::Or, .. }) => Tok::Or,
            Some(Spanned { tok: Tok::And, .. }) => Tok::And,
            other => {
                return Err(ParseError::Unsupported {
                    pos: other.map(|t| t.pos).unwrap_or(open.pos),
                    what: "a parenthesized group that is not a multi-value \
                           filter ('(a=x || a=y)' / '(a!=x && a!=y)')"
                        .to_string(),
                });
            }
        };
        let compatible = match sep {
            Tok::Or => matches!(merged.op, CompareOp::Eq | CompareOp::Regex),
            Tok::And => matches!(merged.op, CompareOp::NotEq | CompareOp::NotRegex),
            _ => unreachable!("sep is Or or And"),
        };
        if !compatible {
            return Err(ParseError::Unsupported {
                pos: open.pos,
                what: format!(
                    "a multi-value group mixing {:?} with this separator (the \
                     form generates '||' only for =/=~ and '&&' only for !=/!~)",
                    merged.op
                ),
            });
        }
        loop {
            let term = self.term()?;
            if term.target != merged.target || term.op != merged.op {
                return Err(ParseError::Unsupported {
                    pos: open.pos,
                    what: "a parenthesized group whose terms differ in field or \
                           operator (the form's multi-value groups repeat one \
                           filter)"
                        .to_string(),
                });
            }
            merged.values.extend(term.values);
            match self.next() {
                Some(Spanned { tok: Tok::RParen, .. }) => break,
                Some(Spanned { ref tok, .. }) if *tok == sep => continue,
                Some(Spanned { pos, .. }) => {
                    return Err(ParseError::Malformed {
                        pos,
                        what: "expected the group separator or ')'".to_string(),
                    });
                }
                None => {
                    return Err(ParseError::Malformed {
                        pos: self.end_pos(),
                        what: "missing closing ')'".to_string(),
                    });
                }
            }
        }
        conditions.push(merged);
        Ok(())
    }

    /// One `field op value` term.
    fn term(&mut self) -> Result<Condition, ParseError> {
        let (field, field_pos) = match self.next() {
            Some(Spanned { tok: Tok::Word(w), pos }) => (w, pos),
            Some(Spanned { tok: Tok::Str(_), pos }) => {
                return Err(ParseError::Unsupported {
                    pos,
                    what: "quoted attribute keys (the form never generates them)"
                        .to_string(),
                });
            }
            Some(Spanned { pos, .. }) => {
                return Err(ParseError::Malformed {
                    pos,
                    what: "expected a field name".to_string(),
                });
            }
            None => {
                return Err(ParseError::Malformed {
                    pos: self.end_pos(),
                    what: "expected a field name".to_string(),
                });
            }
        };
        let target = resolve_field(&field).map_err(|why| ParseError::Unsupported {
            pos: field_pos,
            what: why,
        })?;
        let op = match self.next() {
            Some(Spanned { tok: Tok::Op(op), .. }) => op,
            Some(Spanned { pos, .. }) => {
                return Err(ParseError::Malformed {
                    pos,
                    what: "expected a comparison operator".to_string(),
                });
            }
            None => {
                return Err(ParseError::Malformed {
                    pos: self.end_pos(),
                    what: "expected a comparison operator".to_string(),
                });
            }
        };
        let value = match self.next() {
            Some(Spanned { tok: Tok::Str(s), pos }) => {
                classify_value(&target, op, ValueTok::Str(s), pos)?
            }
            Some(Spanned { tok: Tok::Word(w), pos }) => {
                classify_value(&target, op, ValueTok::Word(w), pos)?
            }
            Some(Spanned { pos, .. }) => {
                return Err(ParseError::Malformed {
                    pos,
                    what: "expected a comparison value".to_string(),
                });
            }
            None => {
                return Err(ParseError::Malformed {
                    pos: self.end_pos(),
                    what: "expected a comparison value".to_string(),
                });
            }
        };
        Ok(Condition {
            target,
            op,
            values: vec![value],
        })
    }
}

enum ValueTok {
    Str(String),
    Word(String),
}

impl ValueTok {
    fn raw(&self) -> &str {
        match self {
            ValueTok::Str(s) | ValueTok::Word(s) => s,
        }
    }
}

/// Map one wire value onto the engine value class its target expects.
/// The kind/status keyword translation and the duration-literal
/// conversion happen HERE (16A); everything the engine can validate
/// itself (regex compilability, id widths, numeric-op requirements)
/// passes through untouched.
fn classify_value(
    target: &PredicateTarget,
    op: CompareOp,
    tok: ValueTok,
    pos: usize,
) -> Result<PredicateValue, ParseError> {
    use TraceIntrinsic::*;
    let enum_field = match target {
        PredicateTarget::Intrinsic(Kind) => Some(("kind", KIND_ALLOWED)),
        PredicateTarget::Intrinsic(Status) => Some(("status", STATUS_ALLOWED)),
        _ => None,
    };
    if let Some((field, allowed)) = enum_field {
        // Regex over the closed keyword sets is raw-editor-only, and the
        // lowercase wire keywords vs uppercase storage labels make a
        // pattern pass-through silently wrong — reject instead.
        if matches!(op, CompareOp::Regex | CompareOp::NotRegex) {
            return Err(ParseError::Unsupported {
                pos,
                what: format!("regex matching on {field} (use = or != with its keywords)"),
            });
        }
        let keyword = tok.raw().to_ascii_lowercase();
        let storage = match field {
            "kind" => kind_keyword_to_storage(&keyword),
            _ => status_keyword_to_storage(&keyword),
        };
        return match storage {
            Some(label) => Ok(PredicateValue::Text(label.to_string())),
            None => Err(ParseError::UnknownKeyword {
                pos,
                field: if field == "kind" { "kind" } else { "status" },
                value: tok.raw().to_string(),
                allowed,
            }),
        };
    }
    if let PredicateTarget::Intrinsic(Duration | TraceDuration | EventTimeSinceStart) = target {
        let ValueTok::Word(lit) = tok else {
            return Err(ParseError::BadDuration {
                pos,
                lit: tok.raw().to_string(),
                why: "durations are bare literals like 100ms, not quoted strings".to_string(),
            });
        };
        let ns = parse_duration_ns(&lit).map_err(|why| ParseError::BadDuration {
            pos,
            lit: lit.clone(),
            why,
        })?;
        return Ok(PredicateValue::Integer(ns));
    }
    match tok {
        ValueTok::Str(s) => Ok(PredicateValue::Text(s)),
        ValueTok::Word(w) => Ok(classify_bare_word(target, w)),
    }
}

/// A bare (unquoted) value on a non-enum, non-duration target: numbers
/// become the dictionary-numeric classes; everything else — identifiers,
/// hex ids, `true`/`false` — is text, matched verbatim against the
/// stored tokens. Numeric parsing is gated on a leading numeric shape so
/// words like `nan`/`inf` stay text.
fn classify_bare_word(target: &PredicateTarget, w: String) -> PredicateValue {
    // Id intrinsics take hex text; a run of digits is still an id, not
    // a number (`predicate.rs` validates width and hexness).
    if let PredicateTarget::Intrinsic(
        TraceIntrinsic::SpanId
        | TraceIntrinsic::ParentSpanId
        | TraceIntrinsic::TraceId
        | TraceIntrinsic::LinkSpanId
        | TraceIntrinsic::LinkTraceId,
    ) = target
    {
        return PredicateValue::Text(w);
    }
    let mut chars = w.chars();
    let first = chars.next().expect("lexer never emits empty words");
    let looks_numeric = first.is_ascii_digit()
        || (matches!(first, '-' | '+' | '.')
            && chars.next().is_some_and(|c| c.is_ascii_digit()));
    if looks_numeric {
        if let Ok(i) = w.parse::<i64>() {
            return PredicateValue::Integer(i);
        }
        if let Ok(f) = w.parse::<f64>() {
            return PredicateValue::Float(f);
        }
    }
    PredicateValue::Text(w)
}

#[cfg(test)]
#[path = "parse_tests.rs"]
mod tests;
