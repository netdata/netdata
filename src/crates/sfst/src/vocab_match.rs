//! Match classes for suggestions: one matcher ranks dictionary values
//! and key names alike, so every tier, every file and the WAL tail rank
//! a candidate the same way.
//!
//! Classes, best first: exact, prefix, contains, then typo-tolerant
//! (a prefix within one edit for terms of 4+ characters, two edits for
//! 8+). Exact, prefix and typo-tolerant compare ASCII case-folded;
//! contains folds Unicode simple case, so a non-ASCII case variant
//! (`É` for `é`) ranks as contains. The typo-tolerant class runs the
//! fst crate's Levenshtein automaton over the candidate's bytes.

use fst::Automaton;
use fst::automaton::{Levenshtein, StartsWith};

/// How a candidate matches the term. The declaration order is the rank
/// (best first).
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum MatchClass {
    /// Equal to the term, ASCII case-folded.
    Exact,
    /// Starts with the term, ASCII case-folded. With an empty term,
    /// every non-empty candidate.
    Prefix,
    /// Contains the term, Unicode simple case-folded.
    Contains,
    /// Starts within one edit of the term (terms of 4+ characters).
    Fuzzy1,
    /// Starts within two edits of the term (terms of 8+ characters).
    Fuzzy2,
}

/// Shortest term (in characters) that gets one-edit matches.
const FUZZY1_MIN_CHARS: usize = 4;
/// Shortest term (in characters) that gets two-edit matches.
const FUZZY2_MIN_CHARS: usize = 8;
/// Longest term that gets typo-tolerant matches: the automaton's build
/// cost grows with the term, and a long term is rarely a typo.
const FUZZY_MAX_CHARS: usize = 32;
/// DFA state limit for one automaton; a term past it gets no
/// typo-tolerant class (never an error).
const FUZZY_STATE_LIMIT: usize = 2_000;

/// Classifies candidates against one term; built once per request.
pub struct ValueMatcher {
    folded: Vec<u8>,
    contains: Option<regex::bytes::Regex>,
    fuzzy1: Option<StartsWith<Levenshtein>>,
    fuzzy2: Option<StartsWith<Levenshtein>>,
}

impl ValueMatcher {
    pub fn new(term: &str) -> Self {
        let folded = term.to_ascii_lowercase();
        let chars = term.chars().count();
        let contains = (!term.is_empty())
            .then(|| {
                regex::bytes::RegexBuilder::new(&regex::escape(term))
                    .case_insensitive(true)
                    .build()
                    .ok()
            })
            .flatten();
        let fuzzy = |min: usize, distance: u32| {
            (min..=FUZZY_MAX_CHARS)
                .contains(&chars)
                .then(|| Levenshtein::new_with_limit(&folded, distance, FUZZY_STATE_LIMIT).ok())
                .flatten()
                .map(Automaton::starts_with)
        };
        Self {
            fuzzy1: fuzzy(FUZZY1_MIN_CHARS, 1),
            fuzzy2: fuzzy(FUZZY2_MIN_CHARS, 2),
            contains,
            folded: folded.into_bytes(),
        }
    }

    /// The candidate's best class, or `None` when it does not match.
    pub fn classify(&self, candidate: &[u8]) -> Option<MatchClass> {
        let n = self.folded.len();
        if candidate.len() >= n && candidate[..n].eq_ignore_ascii_case(&self.folded) {
            return Some(if candidate.len() == n {
                MatchClass::Exact
            } else {
                MatchClass::Prefix
            });
        }
        if self
            .contains
            .as_ref()
            .is_some_and(|re| re.is_match(candidate))
        {
            return Some(MatchClass::Contains);
        }
        if self
            .fuzzy1
            .as_ref()
            .is_some_and(|a| run_folded(a, candidate))
        {
            return Some(MatchClass::Fuzzy1);
        }
        if self
            .fuzzy2
            .as_ref()
            .is_some_and(|a| run_folded(a, candidate))
        {
            return Some(MatchClass::Fuzzy2);
        }
        None
    }
}

/// Run `automaton` over `bytes` ASCII-lowercased (the automaton is built
/// from the lowercased term).
fn run_folded<A: Automaton>(automaton: &A, bytes: &[u8]) -> bool {
    let mut state = automaton.start();
    for &b in bytes {
        if automaton.will_always_match(&state) {
            return true;
        }
        if !automaton.can_match(&state) {
            return false;
        }
        state = automaton.accept(&state, b.to_ascii_lowercase());
    }
    automaton.is_match(&state)
}

#[cfg(test)]
mod tests {
    use super::MatchClass::*;
    use super::*;

    fn class(term: &str, candidate: &str) -> Option<MatchClass> {
        ValueMatcher::new(term).classify(candidate.as_bytes())
    }

    #[test]
    fn classes_rank_exact_prefix_contains_then_typos() {
        assert_eq!(class("front", "front"), Some(Exact));
        assert_eq!(class("front", "FRONT"), Some(Exact));
        assert_eq!(class("front", "Frontend"), Some(Prefix));
        assert_eq!(class("front", "my-frontend"), Some(Contains));
        // One deletion away from a prefix of the candidate.
        assert_eq!(class("front", "frnt-end"), Some(Fuzzy1));
        // A transposition is two edits: only terms of 8+ characters.
        assert_eq!(class("checkout", "chekcout-svc"), Some(Fuzzy2));
        assert_eq!(class("front", "xyz"), None);
        assert!(Exact < Prefix && Prefix < Contains && Contains < Fuzzy1 && Fuzzy1 < Fuzzy2);
    }

    #[test]
    fn short_and_long_terms_get_no_typo_matches() {
        assert_eq!(class("abc", "abd"), None);
        assert_eq!(class("abcd", "abxd"), Some(Fuzzy1));
        assert_eq!(class("abcdefg", "abdcefg"), None);
        let long = "a".repeat(FUZZY_MAX_CHARS + 1);
        let typo = format!("b{}", &long[1..]);
        assert_eq!(class(&long, &typo), None);
    }

    #[test]
    fn non_ascii_case_variants_only_contain() {
        assert_eq!(class("é", "é"), Some(Exact));
        assert_eq!(class("é", "É"), Some(Contains));
        assert_eq!(class("café", "CAFÉ au lait"), Some(Contains));
    }

    #[test]
    fn an_empty_term_matches_everything_as_a_prefix() {
        assert_eq!(class("", "anything"), Some(Prefix));
        assert_eq!(class("", ""), Some(Exact));
    }

    #[test]
    fn regex_metacharacters_are_literal() {
        assert_eq!(class("a.b", "xa.by"), Some(Contains));
        assert_eq!(class("a.b", "xacby"), None);
    }
}
