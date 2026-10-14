//! Match classes for suggestions: one matcher ranks dictionary values
//! and key names alike, so every tier, every file and the WAL tail rank
//! a candidate the same way.
//!
//! Classes, best first: exact, prefix, contains, then typo-tolerant
//! (a prefix within one edit for terms of 4+ characters, two edits for
//! 8+). Exact, prefix and typo-tolerant compare ASCII case-folded;
//! contains folds Unicode simple case, so a non-ASCII case variant
//! (`É` for `é`) ranks as contains. The typo-tolerant class computes,
//! per candidate, the smallest edit distance between the term and a
//! prefix of the candidate: a small dynamic program over at most
//! [`FUZZY_MAX_CHARS`] term characters that stops as soon as no longer
//! prefix can come within reach. Its memory is one column per term,
//! with no automaton to build and no state limit to hit.

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
/// Longest term that gets typo-tolerant matches: a long term is rarely
/// a typo, and the per-candidate cost grows with it.
const FUZZY_MAX_CHARS: usize = 32;

/// Classifies candidates against one term; built once per request.
pub struct ValueMatcher {
    folded: Vec<u8>,
    contains: Option<regex::bytes::Regex>,
    fuzzy: Option<Fuzzy>,
}

impl ValueMatcher {
    pub fn new(term: &str) -> Self {
        let folded = term.to_ascii_lowercase();
        let contains = (!term.is_empty())
            .then(|| {
                regex::bytes::RegexBuilder::new(&regex::escape(term))
                    .case_insensitive(true)
                    .build()
                    .ok()
            })
            .flatten();
        let chars: Vec<char> = folded.chars().collect();
        let fuzzy = (FUZZY1_MIN_CHARS..=FUZZY_MAX_CHARS)
            .contains(&chars.len())
            .then(|| Fuzzy {
                max_distance: if chars.len() >= FUZZY2_MIN_CHARS {
                    2
                } else {
                    1
                },
                term: chars,
            });
        Self {
            folded: folded.into_bytes(),
            contains,
            fuzzy,
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
        let fuzzy = self.fuzzy.as_ref()?;
        let candidate = std::str::from_utf8(candidate).ok()?;
        match fuzzy.prefix_distance(candidate)? {
            0 | 1 => Some(MatchClass::Fuzzy1),
            _ => Some(MatchClass::Fuzzy2),
        }
    }
}

/// Typo-tolerant matching of one term (its ASCII-lowercased characters).
struct Fuzzy {
    term: Vec<char>,
    max_distance: u8,
}

impl Fuzzy {
    /// The smallest edit distance between the term and any prefix of
    /// `candidate` (ASCII case-folded), when it is at most
    /// `max_distance`.
    fn prefix_distance(&self, candidate: &str) -> Option<u8> {
        let n = self.term.len();
        let d = self.max_distance;
        // `col[i]`: the distance between the term's first `i` characters
        // and the candidate prefix read so far (the empty one at first).
        let mut col = [0u8; FUZZY_MAX_CHARS + 1];
        for (i, cell) in col.iter_mut().enumerate().take(n + 1) {
            *cell = i as u8;
        }
        let mut best = col[n];
        // A prefix longer than the term plus `d` characters is too far.
        for (j, c) in candidate.chars().take(n + usize::from(d)).enumerate() {
            let c = c.to_ascii_lowercase();
            let mut diagonal = col[0];
            col[0] = j as u8 + 1;
            let mut lowest = col[0];
            for i in 1..=n {
                let above = col[i];
                let substitute = diagonal + u8::from(self.term[i - 1] != c);
                col[i] = substitute.min(above + 1).min(col[i - 1] + 1);
                diagonal = above;
                lowest = lowest.min(col[i]);
            }
            best = best.min(col[n]);
            // The column minimum never decreases: no longer prefix can
            // come within `d`.
            if lowest > d {
                break;
            }
        }
        (best <= d).then_some(best)
    }
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
    fn long_terms_keep_their_typo_matches() {
        // Two edits (a transposition) in 14 and 16 characters.
        assert_eq!(class("frnotend-proxy", "frontend-proxy"), Some(Fuzzy2));
        assert_eq!(class("http.staus_code", "http.status_code"), Some(Fuzzy1));
        assert_eq!(
            class("htpt.stauts_code", "http.status_code"),
            None,
            "four edits"
        );
        let term = "abcdefghijklmnopqrstuvwxyz012345";
        assert_eq!(term.len(), FUZZY_MAX_CHARS);
        let typo = term.replace("mn", "nm");
        assert_eq!(class(term, &typo), Some(Fuzzy2));
    }

    #[test]
    fn typo_distance_is_over_a_prefix_of_the_candidate() {
        // `frnt` is one insertion from `front`, the start of `frontend`;
        // a transposition is two edits, too many for a 5-character term.
        assert_eq!(class("frnt", "frontend"), Some(Fuzzy1));
        assert_eq!(class("fornt", "frontend"), None);
        assert_eq!(
            class("frontx", "front"),
            Some(Fuzzy1),
            "a candidate shorter than the term"
        );
        assert_eq!(class("FRNT", "Frontend"), Some(Fuzzy1), "case-folded");
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
