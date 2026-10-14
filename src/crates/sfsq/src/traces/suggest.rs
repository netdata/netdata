//! Suggestions for the search bar: the keys, or one key's values,
//! matched against a typed term.
//!
//! Ranking: match class (exact, prefix, contains, then typo-tolerant;
//! [`sfst::ValueMatcher`]), then bytes. One matcher classifies every
//! tier and the WAL tail, so a candidate ranks the same wherever it
//! lives.
//!
//! Keys come back as the caller renders them ([`KeyRenderer`]: the
//! engine owns no wire words), and a key matches by the better of its
//! rendered form (`resource.service.name`) and its bare name
//! (`service.name`; a builtin's bare name is its rendered word): users
//! type attribute names, not owners. Every builtin field is a key
//! whatever the data holds (builtins are engine capabilities); a key's
//! tier is the highest any visited source gives it, and a key no source
//! carries (a virtual builtin such as the duration) has none.
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
//!   is always visited, and the first source that carries the key is
//!   scanned whole. An early stop reports `complete_after`: every span
//!   from that second on lives in a visited source; it is absent when an
//!   unvisited source is a WAL tail, which may hold any recent span.
//!
//! A sealed file contributes from its metadata (keys) or its
//! dictionaries ([`sfst::MetaReader::for_each_field_value`]: no position
//! bitmaps, no rows), a tail from its frame-decoded pair table, whose
//! value counts give its keys' tiers. Values are matched
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
use super::vocab::{AttributeKey, AttributeOwner, BuiltinField, storage_to_attribute};
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

/// Renders a key as the caller names it (the ledger's selection grammar,
/// the CLI's words).
pub type KeyRenderer = fn(AttributeOwner, &AttributeKey) -> String;

#[derive(Debug, Clone)]
enum Target {
    Keys(KeyRenderer),
    Values(AttributeOwner, AttributeKey),
}

/// One suggestions request: the keys, or a key's values, matching `term`.
#[derive(Debug, Clone)]
pub struct SuggestQuery {
    target: Target,
    term: String,
    window: Option<TimeWindow>,
    max_items: usize,
    max_files: usize,
    stale_files: usize,
    deadline: Option<Instant>,
    deadline_after_values: Option<usize>,
}

impl SuggestQuery {
    /// Suggest the values of `key` (an attribute under a concrete owner,
    /// or a builtin field with a dictionary under
    /// [`AttributeOwner::Builtin`]).
    pub fn values(owner: AttributeOwner, key: AttributeKey) -> Self {
        Self::new(Target::Values(owner, key))
    }

    /// Suggest keys, each rendered by `render`.
    pub fn keys(render: KeyRenderer) -> Self {
        Self::new(Target::Keys(render))
    }

    fn new(target: Target) -> Self {
        Self {
            target,
            term: String::new(),
            window: None,
            max_items: DEFAULT_SUGGEST_ITEMS,
            max_files: DEFAULT_SUGGEST_FILES,
            stale_files: DEFAULT_STALE_FILES,
            deadline: None,
            deadline_after_values: None,
        }
    }

    /// The typed text; empty matches every candidate (as a prefix).
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

    /// Zero is rejected ([`AttributeRequestError::ZeroLimit`]).
    pub fn stale_files(mut self, n: usize) -> Self {
        self.stale_files = n;
        self
    }

    /// Stop visiting sources at `at`. The newest source is always
    /// visited, and the first that carries the key is scanned whole.
    pub fn deadline(mut self, at: Instant) -> Self {
        self.deadline = Some(at);
        self
    }

    /// Tests: a deadline that passes once `n` values were scanned, so
    /// a stop inside a source is reproducible.
    #[doc(hidden)]
    pub fn deadline_after_values(mut self, n: usize) -> Self {
        self.deadline_after_values = Some(n);
        self
    }
}

/// One suggestion.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Suggestion {
    /// The key or value, cut to [`DISPLAY_MAX_BYTES`] when `truncated`.
    pub text: String,
    /// The text is a prefix of at least one longer value.
    pub truncated: bool,
    pub class: MatchClass,
    /// Keys only: the highest tier a visited source gives the key;
    /// `None` when no source carries it.
    pub tier: Option<sfst::FieldTier>,
    /// Keys: `false` for a key without stored values (a virtual builtin
    /// such as the duration), whose values cannot be suggested. Values:
    /// always `true`.
    pub enumerable: bool,
}

/// The suggestions, best first.
#[derive(Debug)]
pub struct SuggestData {
    pub items: Vec<Suggestion>,
    /// A further match existed among the visited sources.
    pub truncated: bool,
    pub status: QueryStatus,
    /// Set after an early stop: every span from this second (unix) on
    /// lives in a visited source. Absent when nothing was left unvisited,
    /// or when an unvisited source is a tail, which could hold any recent
    /// span.
    pub complete_after: Option<u32>,
}

/// Suggest keys or values per `query` over `sources`. See the module docs for
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
    if query.max_items == 0 || query.max_files == 0 || query.stale_files == 0 {
        return Err(AttributeRequestError::ZeroLimit);
    }
    let target = match &query.target {
        Target::Keys(render) => Resolved::Keys(*render),
        Target::Values(owner, key) => Resolved::Values(storage_field_of(*owner, key)?),
    };
    validate_sources(&sources)?;
    newest_first(&mut sources);

    let matcher = ValueMatcher::new(&query.term);
    let mut set = BoundedSuggestions::new(query.max_items);
    if let Resolved::Keys(render) = target {
        for field in BuiltinField::ALL {
            let key = AttributeKey::Builtin(field);
            offer_key(
                &mut set,
                &matcher,
                render,
                AttributeOwner::Builtin,
                &key,
                None,
            );
        }
    }
    let mut status = StatusBuilder::new();
    let mut clock = Clock::new(query.deadline, query.deadline_after_values);
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
            stop = Some((i, PartialReason::Deadline, i));
            break;
        }
        if visited_files == query.max_files {
            stop = Some((i, PartialReason::FileBudget, i));
            break;
        }
        if stale == query.stale_files {
            stop = Some((i, PartialReason::NoNewMatches, i));
            break;
        }
        // The first source that carries the key is scanned whole, so the
        // answer is not empty for lack of time once one is reached.
        clock.armed = visited_files > 0;
        let visit = match source {
            TraceSource::Sfst(c) if pruned(query.window, &c.summary) => Visit::Absent,
            TraceSource::Sfst(c) => visit_sfst(c, &target, &matcher, &mut set, &mut clock),
            TraceSource::Tail(t) => visit_tail(t, &target, &matcher, &mut set, &mut clock),
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
                // The source was cut short: it counts as not visited, but
                // its progress tick is already in.
                stop = Some((i, PartialReason::Deadline, i + 1));
                break;
            }
        }
    }

    let mut complete = None;
    if let Some((first_unvisited, reason, first_unticked)) = stop {
        status.add(reason);
        progress.fetch_add(sources.len() - first_unticked, Ordering::Relaxed);
        complete = complete_after(&sources[first_unvisited..]);
    }
    let truncated = set.truncated;
    Ok(SuggestData {
        items: set.into_items(),
        truncated,
        status: status.finish(),
        complete_after: complete,
    })
}

/// The query's target, validated.
enum Resolved {
    Keys(KeyRenderer),
    /// The storage field holding the key's values.
    Values(String),
}

/// What one source contributed.
enum Visit {
    /// Pruned by the window, unavailable, or (values) without the key:
    /// no work.
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
    target: &Resolved,
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
    let storage = match target {
        Resolved::Values(storage) => storage,
        Resolved::Keys(render) => {
            let fields = match meta.field_table() {
                Ok(fields) => fields,
                Err(e) => return Visit::Failed(format!("field table: {e}")),
            };
            let mut changed = false;
            for field in fields.iter() {
                if let Some((owner, key)) = storage_to_attribute(&field.name) {
                    changed |= offer_key(set, matcher, *render, owner, &key, Some(field.tier));
                }
            }
            return Visit::Scanned { changed };
        }
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
    target: &Resolved,
    matcher: &ValueMatcher,
    set: &mut BoundedSuggestions,
    clock: &mut Clock,
) -> Visit {
    let scan = match TraceWalScan::scan_range(&t.path, t.coverage.range) {
        Ok(scan) => scan,
        Err(e) => return Visit::Failed(format!("scan: {e}")),
    };
    let storage = match target {
        Resolved::Values(storage) => storage,
        Resolved::Keys(render) => {
            let mut changed = false;
            for (name, values) in scan.pair_table() {
                if let Some((owner, key)) = storage_to_attribute(name) {
                    let tier = sfst::FieldTier::from_cardinality(
                        values.len(),
                        sfst::DEFAULT_CARDINALITY_THRESHOLD,
                    );
                    changed |= offer_key(set, matcher, *render, owner, &key, Some(tier));
                }
            }
            return Visit::Scanned { changed };
        }
    };
    let Some(values) = scan.pair_table().get(storage.as_str()) else {
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
    set.offer(text, class, None)
}

/// What a key suggestion carries besides its text.
#[derive(Debug, Clone, Copy)]
struct KeyInfo {
    tier: Option<sfst::FieldTier>,
    enumerable: bool,
}

/// Classify a key by the better of its rendered form and its bare name,
/// and offer it; `true` when the set changed.
fn offer_key(
    set: &mut BoundedSuggestions,
    matcher: &ValueMatcher,
    render: KeyRenderer,
    owner: AttributeOwner,
    key: &AttributeKey,
    tier: Option<sfst::FieldTier>,
) -> bool {
    let text = render(owner, key);
    let (bare, enumerable) = match key {
        AttributeKey::Attribute(name) => (matcher.classify(name.as_bytes()), true),
        AttributeKey::Builtin(b) => (None, b.dictionary_field().is_some()),
    };
    match matcher
        .classify(text.as_bytes())
        .into_iter()
        .chain(bare)
        .min()
    {
        Some(class) => set.offer(&text, class, Some(KeyInfo { tier, enumerable })),
        None => false,
    }
}

/// The deadline, checked cheaply inside scans.
struct Clock {
    deadline: Option<Instant>,
    /// Tests: expire once this many values were scanned.
    after_values: Option<usize>,
    ticks: usize,
    /// Whether a scan may stop mid-source.
    armed: bool,
}

impl Clock {
    fn new(deadline: Option<Instant>, after_values: Option<usize>) -> Self {
        Self {
            deadline,
            after_values,
            ticks: 0,
            armed: false,
        }
    }

    fn expired(&self) -> bool {
        self.deadline.is_some_and(|d| Instant::now() >= d)
            || self.after_values.is_some_and(|n| self.ticks >= n)
    }

    /// Counts one scanned value; checks the clock every
    /// [`DEADLINE_CHECK_EVERY`] values when armed.
    fn tick_expired(&mut self) -> bool {
        self.ticks += 1;
        self.armed && self.ticks.is_multiple_of(DEADLINE_CHECK_EVERY) && self.expired()
    }
}

/// What the set knows about one display text.
struct Entry {
    class: MatchClass,
    truncated: bool,
    key: Option<KeyInfo>,
}

/// The best `k` suggestions seen so far, by (class, text). Memory is
/// O(k): a candidate that cannot enter is dropped at once.
struct BoundedSuggestions {
    k: usize,
    best: HashMap<Box<str>, Entry>,
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

    /// Offer a matching candidate (`key`: a key's details; `None` for a
    /// value); `true` when the set changed. A key seen again keeps the
    /// highest tier; that alone is no change.
    fn offer(&mut self, value: &str, class: MatchClass, key: Option<KeyInfo>) -> bool {
        let (display, cut) = display_text(value);
        if let Some(entry) = self.best.get_mut(display) {
            // Another candidate with the same display text: keep the
            // better class and the higher tier, and flag the text if
            // either was cut.
            if let (Some(seen), Some(key)) = (entry.key.as_mut(), key) {
                seen.tier = seen.tier.max(key.tier);
            }
            let mut changed = false;
            if cut && !entry.truncated {
                entry.truncated = true;
                changed = true;
            }
            if class < entry.class {
                let text: Box<str> = display.into();
                self.order.remove(&(entry.class, text.clone()));
                self.order.insert((class, text));
                entry.class = class;
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
        let text: Box<str> = display.into();
        self.best.insert(
            text.clone(),
            Entry {
                class,
                truncated: cut,
                key,
            },
        );
        self.order.insert((class, text));
        true
    }

    fn into_items(mut self) -> Vec<Suggestion> {
        self.order
            .into_iter()
            .map(|(class, text)| {
                let entry = self
                    .best
                    .remove(&text)
                    .expect("every ordered text has an entry");
                Suggestion {
                    text: text.into(),
                    truncated: entry.truncated,
                    class,
                    tier: entry.key.and_then(|k| k.tier),
                    enumerable: entry.key.is_none_or(|k| k.enumerable),
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
        assert!(set.offer("b", Prefix, None));
        assert!(set.offer("a", Contains, None));
        // Full: a worse candidate is dropped, a better one evicts the worst.
        assert!(!set.offer("c", Contains, None));
        assert!(set.offer("z", Exact, None));
        assert!(set.truncated);
        assert_eq!(
            texts(set),
            [("z".into(), Exact, false), ("b".into(), Prefix, false)]
        );
    }

    #[test]
    fn a_repeated_text_keeps_its_better_class() {
        let mut set = BoundedSuggestions::new(3);
        assert!(set.offer("a", Fuzzy1, None));
        assert!(!set.offer("a", Fuzzy2, None));
        assert!(set.offer("a", Prefix, None));
        assert!(!set.truncated);
        assert_eq!(texts(set), [("a".into(), Prefix, false)]);
    }

    #[test]
    fn an_evicted_text_never_returns_as_a_worse_match() {
        // The worst only improves, so an evicted value cannot re-enter
        // with the class it was evicted at.
        let mut set = BoundedSuggestions::new(1);
        assert!(set.offer("b", Contains, None));
        assert!(set.offer("a", Prefix, None));
        assert!(!set.offer("b", Contains, None));
        assert_eq!(texts(set), [("a".into(), Prefix, false)]);
    }

    #[test]
    fn long_values_share_one_flagged_prefix() {
        // A 2-byte character straddles the cut: the cut moves before it.
        let head = "x".repeat(DISPLAY_MAX_BYTES - 1);
        let long_a = format!("{head}éa");
        let long_b = format!("{head}éb");
        let mut set = BoundedSuggestions::new(4);
        assert!(set.offer(&long_a, Prefix, None));
        assert!(!set.offer(&long_b, Prefix, None));
        assert_eq!(texts(set), [(head, Prefix, true)]);
    }
}
