//! Spike: de-risk running a regex over a per-column *value* FST via a
//! `regex-automata` DFA driven byte-by-byte as an `fst::Automaton`, and feeding a
//! constant `field=` prefix so a full-text `field=value` regex works WITHOUT
//! storing or reconstructing `field=value`.
//!
//! Validates the three gotchas from the design discussion:
//!   - EOI/`$` semantics (via `fst::Automaton::accept_eof` + `next_eoi_state`),
//!   - unanchored vs anchored start states,
//!   - prefix-feeding the automaton to recover `field=value` matching.
//!
//! If this passes, the on-disk regex story for the typed per-column index holds.

use fst::{Automaton, IntoStreamer, Map, Streamer};
use regex_automata::dfa::{Automaton as DfaAutomaton, dense};
use regex_automata::util::primitives::StateID;
use regex_automata::{Anchored, Input};

/// A `regex-automata` dense DFA exposed as an `fst::Automaton`.
///
/// State is `(StateID, matched)` — `matched` LATCHES: once any prefix of the key
/// has matched (substring patterns complete mid-key; `$`-anchored ones complete at
/// EOI via `accept_eof`), the key is accepted regardless of trailing bytes. `start`
/// may be pre-advanced through a constant `field=` prefix (prefix-feeding).
struct RegexDfa {
    dfa: dense::DFA<Vec<u32>>,
    start: (StateID, bool),
}

impl RegexDfa {
    fn new(pattern: &str, anchored: Anchored, prefix: &[u8]) -> Self {
        let dfa = dense::DFA::new(pattern).expect("valid pattern");
        let s0 = dfa
            .start_state_forward(&Input::new("").anchored(anchored))
            .expect("start state");
        let mut state = (s0, dfa.is_match_state(s0));
        for &b in prefix {
            state = Self::step(&dfa, state, b);
        }
        Self { dfa, start: state }
    }

    /// One byte transition with match-latching.
    fn step(dfa: &dense::DFA<Vec<u32>>, (s, m): (StateID, bool), b: u8) -> (StateID, bool) {
        if m {
            return (s, true);
        }
        let ns = dfa.next_state(s, b);
        (ns, dfa.is_match_state(ns))
    }
}

impl Automaton for RegexDfa {
    type State = (StateID, bool);

    fn start(&self) -> Self::State {
        self.start
    }

    fn is_match(&self, &(_, matched): &Self::State) -> bool {
        matched
    }

    fn can_match(&self, &(s, matched): &Self::State) -> bool {
        matched || !self.dfa.is_dead_state(s)
    }

    fn accept(&self, &state: &Self::State, byte: u8) -> Self::State {
        Self::step(&self.dfa, state, byte)
    }

    /// End-of-key: take the DFA's EOI transition so `$`-anchored patterns resolve.
    fn accept_eof(&self, &(s, matched): &Self::State) -> Option<Self::State> {
        if matched {
            return Some((s, true));
        }
        let es = self.dfa.next_eoi_state(s);
        Some((es, self.dfa.is_match_state(es)))
    }
}

/// The values a per-column FST might hold (sorted, unique — fst requirement).
fn value_fst() -> Map<Vec<u8>> {
    let mut values = [
        "GET",
        "GETX",
        "POST",
        "r2.cloudflarestorage.com",
        "static.example.com",
    ];
    values.sort_unstable();
    Map::from_iter(values.iter().enumerate().map(|(i, v)| (*v, i as u64))).expect("build fst")
}

/// Keys matched by `re`, sorted.
fn matches(map: &Map<Vec<u8>>, re: &RegexDfa) -> Vec<String> {
    let mut out = Vec::new();
    let mut stream = map.search(re).into_stream();
    while let Some((key, _)) = stream.next() {
        out.push(String::from_utf8(key.to_vec()).unwrap());
    }
    out.sort();
    out
}

#[test]
fn unanchored_substring_matches_inside_values() {
    let map = value_fst();
    // Substring "cloud" anywhere in the value.
    let re = RegexDfa::new("cloud", Anchored::No, b"");
    assert_eq!(matches(&map, &re), ["r2.cloudflarestorage.com"]);
}

#[test]
fn anchored_full_value_does_not_match_prefixes() {
    let map = value_fst();
    // Full-value match: "GET"/"POST" exactly — NOT "GETX" (proves `$` via EOI).
    let re = RegexDfa::new("^(?:GET|POST)$", Anchored::No, b"");
    assert_eq!(matches(&map, &re), ["GET", "POST"]);
}

#[test]
fn prefix_fed_automaton_recovers_field_equals_value_matching() {
    let map = value_fst();
    // A full-text regex written against `field=value`, run over a value-only FST by
    // feeding the constant "domain=" prefix into the automaton first. No `field=`
    // is stored or reconstructed per value.
    let re = RegexDfa::new("domain=r2", Anchored::No, b"domain=");
    // Conceptually matches "domain=r2.cloudflarestorage.com"; nothing else.
    assert_eq!(matches(&map, &re), ["r2.cloudflarestorage.com"]);

    // A prefix that no value continues into matches nothing.
    let none = RegexDfa::new("domain=nope", Anchored::No, b"domain=");
    assert!(matches(&map, &none).is_empty());
}

#[test]
fn can_match_prunes_dead_branches() {
    let map = value_fst();
    // Anchored literal: only keys starting with "r2" can match; the automaton goes
    // dead on any other first byte, so can_match() prunes those FST branches.
    let re = RegexDfa::new("^r2", Anchored::No, b"");
    assert_eq!(matches(&map, &re), ["r2.cloudflarestorage.com"]);
}
