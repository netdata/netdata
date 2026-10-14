//! Suggestions for the search bar: one key's values matched against a
//! typed term.
//!
//! Ranking: match class (exact, prefix, contains, then typo-tolerant;
//! [`sfst::ValueMatcher`]), then bytes. One matcher classifies every
//! tier and the WAL tail, so a value ranks the same wherever it lives.
//!
//! Bounds, independent of how many files the window holds:
//! - items: at most `max_items` suggestions, and only that many are kept
//!   while merging (memory follows the limit, not the key's distinct
//!   values); `truncated` says a further match existed among the
//!   visited sources;
//! - work: sources are visited newest first ([`super::order`]) and the
//!   walk stops at `max_files` sources that carry the key
//!   ([`FileBudget`](PartialReason::FileBudget)), after `stale_files`
//!   consecutive such sources changed nothing
//!   ([`NoNewMatches`](PartialReason::NoNewMatches)), or at the deadline
//!   ([`Deadline`](PartialReason::Deadline)), checked between sources and
//!   every [`DEADLINE_CHECK_EVERY`] values inside one; the newest source
//!   is always visited. An early stop reports `complete_after`: every
//!   span from that second on lives in a visited source.
//!
//! A sealed file contributes from its dictionaries alone
//! ([`sfst::MetaReader::for_each_field_value`]: no position bitmaps, no
//! rows), a tail from its frame-decoded pair table. Values are matched
//! on their full bytes and shown cut to [`DISPLAY_MAX_BYTES`] at a
//! character boundary, flagged: values sharing that prefix are one
//! suggestion. Values that are not UTF-8 are skipped, since a selection
//! could not name them.
//!
//! Time bounding is file-granular, as for the other enumerations: a
//! window prunes sources whose summary does not overlap it.
//! Cancellation is all-or-empty. Failure honesty: a source that fails to
//! map, open or read is a [`SourceFailure`](PartialReason::SourceFailure),
//! an in-window unavailable one a
//! [`RemoteUnavailable`](PartialReason::RemoteUnavailable).

use std::collections::{BTreeSet, HashMap};
use std::ops::ControlFlow;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Instant;

use sfst::{MatchClass, ValueMatcher};
use tokio_util::sync::CancellationToken;

use super::attributes::{AttributeRequestError, pruned, storage_field_of};
use super::order::{complete_after, newest_first};
use super::sources::{TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::{AttributeKey, AttributeOwner};
use super::wal_scan::TraceWalScan;
use super::window::TimeWindow;
use crate::source::map_source;

/// Longest suggestion text, in bytes; longer values are cut at a
/// character boundary and flagged.
pub const DISPLAY_MAX_BYTES: usize = 256;
/// Suggestions returned when the query does not say.
pub const DEFAULT_SUGGEST_ITEMS: usize = 50;
/// Sources carrying the key visited at most, when the query does not say.
pub const DEFAULT_SUGGEST_FILES: usize = 256;
/// Consecutive sources carrying the key that may change nothing before
/// the walk stops, when the query does not say.
pub const DEFAULT_STALE_FILES: usize = 8;
/// Values scanned between two deadline checks inside one source.
pub const DEADLINE_CHECK_EVERY: usize = 1024;

/// One suggestions request: a key's values matching `term`.
#[derive(Debug, Clone)]
pub struct SuggestQuery {
    owner: AttributeOwner,
    key: AttributeKey,
    term: String,
    window: Option<TimeWindow>,
    max_items: usize,
    max_files: usize,
    stale_files: usize,
    deadline: Option<Instant>,
}

impl SuggestQuery {
    /// Suggest the values of `key` (an attribute under a concrete owner,
    /// or a builtin field with a dictionary under
    /// [`AttributeOwner::Builtin`]).
    pub fn values(owner: AttributeOwner, key: AttributeKey) -> Self {
        Self {
            owner,
            key,
            term: String::new(),
            window: None,
            max_items: DEFAULT_SUGGEST_ITEMS,
            max_files: DEFAULT_SUGGEST_FILES,
            stale_files: DEFAULT_STALE_FILES,
            deadline: None,
        }
    }

    /// The typed text; empty matches every value (as a prefix).
    pub fn term(mut self, term: impl Into<String>) -> Self {
        self.term = term.into();
        self
    }

    pub fn window(mut self, window: TimeWindow) -> Self {
        self.window = Some(window);
        self
    }

    /// Zero is rejected ([`AttributeRequestError::ZeroLimit`]).
    pub fn max_items(mut self, max: usize) -> Self {
        self.max_items = max;
        self
    }

    /// Zero is rejected ([`AttributeRequestError::ZeroLimit`]).
    pub fn max_files(mut self, max: usize) -> Self {
        self.max_files = max;
        self
    }

    pub fn stale_files(mut self, n: usize) -> Self {
        self.stale_files = n;
        self
    }

    /// Stop visiting sources at `at` (the newest one is always visited).
    pub fn deadline(mut self, at: Instant) -> Self {
        self.deadline = Some(at);
        self
    }
}

/// One suggestion.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Suggestion {
    /// The value, cut to [`DISPLAY_MAX_BYTES`] when `truncated`.
    pub text: String,
    /// The text is a prefix of at least one longer value.
    pub truncated: bool,
    pub class: MatchClass,
}

/// The suggestions, best first.
#[derive(Debug)]
pub struct SuggestData {
    pub items: Vec<Suggestion>,
    /// A further match existed among the visited sources.
    pub truncated: bool,
    pub status: QueryStatus,
    /// Set after an early stop: every span from this second (unix) on
    /// lives in a visited source (see [`complete_after`]).
    pub complete_after: Option<u32>,
}

/// Suggest values per `query` over `sources`. See the module docs for
/// ranking, bounds, cancellation and failure contracts. `progress` ticks
/// once per source; an early stop ticks the rest.
///
/// Pure sync: reads and decompresses files; run it off any async
/// runtime thread.
pub fn suggest(
    mut sources: Vec<TraceSource>,
    query: SuggestQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<SuggestData, AttributeRequestError> {
    if query.max_items == 0 || query.max_files == 0 {
        return Err(AttributeRequestError::ZeroLimit);
    }
    let storage = storage_field_of(query.owner, &query.key)?;
    validate_sources(&sources)?;
    newest_first(&mut sources);

    let matcher = ValueMatcher::new(&query.term);
    let mut set = BoundedSuggestions::new(query.max_items);
    let mut status = StatusBuilder::new();
    let mut clock = Clock::new(query.deadline);
    let (mut visited_files, mut stale) = (0usize, 0usize);
    let mut stop = None;
    for (i, source) in sources.iter().enumerate() {
        if cancel.is_cancelled() {
            status.add(PartialReason::Cancelled);
            return Ok(SuggestData {
                items: Vec::new(),
                truncated: false,
                status: status.finish(),
                complete_after: None,
            });
        }
        if i > 0 && clock.expired() {
            stop = Some((i, PartialReason::Deadline));
            break;
        }
        if visited_files == query.max_files {
            stop = Some((i, PartialReason::FileBudget));
            break;
        }
        if stale == query.stale_files {
            stop = Some((i, PartialReason::NoNewMatches));
            break;
        }
        let visit = match source {
            TraceSource::Sfst(c) if pruned(query.window, &c.summary) => Visit::Absent,
            TraceSource::Sfst(c) => visit_sfst(c, &storage, &matcher, &mut set, &mut clock),
            TraceSource::Tail(t) => visit_tail(t, &storage, &matcher, &mut set, &mut clock),
            TraceSource::Unavailable(u) => {
                if !pruned(query.window, &u.summary) {
                    status.add(PartialReason::RemoteUnavailable);
                }
                Visit::Absent
            }
        };
        progress.fetch_add(1, Ordering::Relaxed);
        match visit {
            Visit::Absent => {}
            Visit::Failed(e) => {
                tracing::warn!("sfsq traces: suggest: source {}: {e}", source.source_id());
                status.add(PartialReason::SourceFailure);
            }
            Visit::Scanned { changed } => {
                visited_files += 1;
                stale = if changed { 0 } else { stale + 1 };
            }
            Visit::Interrupted => {
                // The source was cut short: count it as not visited.
                stop = Some((i, PartialReason::Deadline));
                break;
            }
        }
    }

    let mut complete = None;
    if let Some((first_unvisited, reason)) = stop {
        status.add(reason);
        let rest = &sources[first_unvisited..];
        progress.fetch_add(rest.len(), Ordering::Relaxed);
        complete = complete_after(rest);
    }
    let truncated = set.truncated;
    Ok(SuggestData {
        items: set.into_items(),
        truncated,
        status: status.finish(),
        complete_after: complete,
    })
}

/// What one source contributed.
enum Visit {
    /// Pruned by the window, unavailable, or without the key: no work.
    Absent,
    Failed(String),
    /// Scanned to the end; `changed` when the set took a new suggestion.
    Scanned {
        changed: bool,
    },
    /// The deadline passed mid-scan.
    Interrupted,
}

fn visit_sfst(
    c: &super::sources::TraceSfstCandidate,
    storage: &str,
    matcher: &ValueMatcher,
    set: &mut BoundedSuggestions,
    clock: &mut Clock,
) -> Visit {
    let mapped = match map_source(&c.source) {
        Ok(m) => m,
        Err(e) => return Visit::Failed(format!("map: {e}")),
    };
    let meta = match sfst::MetaReader::open(mapped.bytes()) {
        Ok(m) => m,
        Err(e) => return Visit::Failed(format!("open: {e}")),
    };
    let (mut changed, mut interrupted) = (false, false);
    let present = meta.for_each_field_value(storage, |value| {
        if clock.tick_expired() {
            interrupted = true;
            return ControlFlow::Break(());
        }
        changed |= offer(set, matcher, value);
        ControlFlow::Continue(())
    });
    match present {
        Err(e) => Visit::Failed(format!("read {storage}: {e}")),
        Ok(false) => Visit::Absent,
        Ok(true) if interrupted => Visit::Interrupted,
        Ok(true) => Visit::Scanned { changed },
    }
}

fn visit_tail(
    t: &super::sources::TraceWalTail,
    storage: &str,
    matcher: &ValueMatcher,
    set: &mut BoundedSuggestions,
    clock: &mut Clock,
) -> Visit {
    let scan = match TraceWalScan::scan_range(&t.path, t.coverage.range) {
        Ok(scan) => scan,
        Err(e) => return Visit::Failed(format!("scan: {e}")),
    };
    let Some(values) = scan.pair_table().get(storage) else {
        return Visit::Absent;
    };
    let mut changed = false;
    for value in values {
        if clock.tick_expired() {
            return Visit::Interrupted;
        }
        changed |= offer(set, matcher, value.as_bytes());
    }
    Visit::Scanned { changed }
}

/// Classify `value` and offer it; `true` when the set changed.
fn offer(set: &mut BoundedSuggestions, matcher: &ValueMatcher, value: &[u8]) -> bool {
    let Some(class) = matcher.classify(value) else {
        return false;
    };
    let Ok(text) = std::str::from_utf8(value) else {
        return false;
    };
    set.offer(text, class)
}

/// The deadline, checked cheaply inside scans.
struct Clock {
    deadline: Option<Instant>,
    ticks: usize,
}

impl Clock {
    fn new(deadline: Option<Instant>) -> Self {
        Self { deadline, ticks: 0 }
    }

    fn expired(&self) -> bool {
        self.deadline.is_some_and(|d| Instant::now() >= d)
    }

    /// Checks the clock every [`DEADLINE_CHECK_EVERY`] calls.
    fn tick_expired(&mut self) -> bool {
        self.ticks += 1;
        self.ticks.is_multiple_of(DEADLINE_CHECK_EVERY) && self.expired()
    }
}

/// The best `k` suggestions seen so far, by (class, text). Memory is
/// O(k): a candidate that cannot enter is dropped at once.
struct BoundedSuggestions {
    k: usize,
    /// Display text → (class, truncated).
    best: HashMap<Box<str>, (MatchClass, bool)>,
    order: BTreeSet<(MatchClass, Box<str>)>,
    /// A match was rejected or evicted.
    truncated: bool,
}

impl BoundedSuggestions {
    fn new(k: usize) -> Self {
        Self {
            k,
            best: HashMap::new(),
            order: BTreeSet::new(),
            truncated: false,
        }
    }

    /// Offer a matching value; `true` when the set changed.
    fn offer(&mut self, value: &str, class: MatchClass) -> bool {
        let (display, cut) = display_text(value);
        if let Some((old_class, old_cut)) = self.best.get_mut(display) {
            // Another value with the same display text: keep the better
            // class, and flag the text if either was cut.
            let mut changed = false;
            if cut && !*old_cut {
                *old_cut = true;
                changed = true;
            }
            if class < *old_class {
                let key: Box<str> = display.into();
                self.order.remove(&(*old_class, key.clone()));
                self.order.insert((class, key));
                *old_class = class;
                changed = true;
            }
            return changed;
        }
        if self.order.len() == self.k {
            let worst = self.order.last().expect("k > 0");
            if (class, display) >= (worst.0, &*worst.1) {
                self.truncated = true;
                return false;
            }
            let (_, evicted) = self.order.pop_last().expect("k > 0");
            self.best.remove(&evicted);
            self.truncated = true;
        }
        let key: Box<str> = display.into();
        self.best.insert(key.clone(), (class, cut));
        self.order.insert((class, key));
        true
    }

    fn into_items(mut self) -> Vec<Suggestion> {
        self.order
            .into_iter()
            .map(|(class, text)| {
                let truncated = self.best.remove(&text).is_some_and(|(_, cut)| cut);
                Suggestion {
                    text: text.into(),
                    truncated,
                    class,
                }
            })
            .collect()
    }
}

/// `value` cut to [`DISPLAY_MAX_BYTES`] at a character boundary, and
/// whether it was cut.
fn display_text(value: &str) -> (&str, bool) {
    if value.len() <= DISPLAY_MAX_BYTES {
        return (value, false);
    }
    let mut cut = DISPLAY_MAX_BYTES;
    while !value.is_char_boundary(cut) {
        cut -= 1;
    }
    (&value[..cut], true)
}

#[cfg(test)]
mod tests {
    use super::*;
    use MatchClass::*;

    fn texts(set: BoundedSuggestions) -> Vec<(String, MatchClass, bool)> {
        set.into_items()
            .into_iter()
            .map(|s| (s.text, s.class, s.truncated))
            .collect()
    }

    #[test]
    fn the_set_keeps_the_best_k_by_class_then_text() {
        let mut set = BoundedSuggestions::new(2);
        assert!(set.offer("b", Prefix));
        assert!(set.offer("a", Contains));
        // Full: a worse candidate is dropped, a better one evicts the worst.
        assert!(!set.offer("c", Contains));
        assert!(set.offer("z", Exact));
        assert!(set.truncated);
        assert_eq!(
            texts(set),
            [("z".into(), Exact, false), ("b".into(), Prefix, false)]
        );
    }

    #[test]
    fn a_repeated_text_keeps_its_better_class() {
        let mut set = BoundedSuggestions::new(3);
        assert!(set.offer("a", Fuzzy1));
        assert!(!set.offer("a", Fuzzy2));
        assert!(set.offer("a", Prefix));
        assert!(!set.truncated);
        assert_eq!(texts(set), [("a".into(), Prefix, false)]);
    }

    #[test]
    fn an_evicted_text_never_returns_as_a_worse_match() {
        // The worst only improves, so an evicted value cannot re-enter
        // with the class it was evicted at.
        let mut set = BoundedSuggestions::new(1);
        assert!(set.offer("b", Contains));
        assert!(set.offer("a", Prefix));
        assert!(!set.offer("b", Contains));
        assert_eq!(texts(set), [("a".into(), Prefix, false)]);
    }

    #[test]
    fn long_values_share_one_flagged_prefix() {
        // A 2-byte character straddles the cut: the cut moves before it.
        let head = "x".repeat(DISPLAY_MAX_BYTES - 1);
        let long_a = format!("{head}éa");
        let long_b = format!("{head}éb");
        let mut set = BoundedSuggestions::new(4);
        assert!(set.offer(&long_a, Prefix));
        assert!(!set.offer(&long_b, Prefix));
        assert_eq!(texts(set), [(head, Prefix, true)]);
    }
}
