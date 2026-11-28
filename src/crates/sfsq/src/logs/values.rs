//! Value aggregation: top values and quantiles of one field, optionally
//! grouped by a second field, over a query's matching logs.
//!
//! The per-source accumulator ([`ValueAgg`]) rides in the step-1 shard
//! ([`LogsShard`](super::aggregate::LogsShard)) and merges with it, so it
//! inherits the shard's monoid property and the same fan-out argument.
//!
//! Both kinds keep **exact** per-group value counts. Merging exact counts is
//! exact, so the answer does not depend on how sources are split or merged.
//! The number of distinct `(group, value)` entries is capped at
//! [`MAX_EXACT_ENTRIES`] per aggregation:
//!
//! - top values: past the cap only the most frequent entries are kept and
//!   the result is flagged `incomplete` (counts of kept values may then be
//!   undercounts, and a value dropped in one source may be missing);
//! - quantiles: past the cap every group switches to a log-linear histogram
//!   (`crate::duration_hist`; within 1/32 of the exact value for magnitudes
//!   from about 0.016 up, coarser below as values keep three decimals) and the result
//!   is flagged `approximate`.
//!
//! Quantiles parse each distinct value once as a number; a value that is
//! not a finite number is counted in `ignored`. Exact quantiles use the
//! nearest-rank definition (rank `ceil(phi * n)`, 1-based).

use std::collections::HashMap;

use crate::duration_hist::DurationHistogram;

/// Distinct `(group, value)` entries an aggregation keeps exactly.
pub const MAX_EXACT_ENTRIES: usize = 250_000;

/// Approximate mode records `round(value * APPROX_SCALE)` into the integer
/// histogram, so values keep three decimals before the histogram's own
/// 1/32 relative error applies.
const APPROX_SCALE: f64 = 1000.0;

/// What to aggregate: the values of `field`, per value of `by` when given.
#[derive(Debug, Clone, PartialEq)]
pub struct AggregateSpec {
    pub field: String,
    pub by: Option<String>,
    pub kind: AggregateKind,
}

/// The aggregation's answer per group.
#[derive(Debug, Clone, PartialEq)]
pub enum AggregateKind {
    /// The `n` most frequent values, with their counts.
    Top(usize),
    /// These quantiles (each in `[0, 1]`) of the values parsed as numbers.
    Quantiles(Vec<f64>),
}

/// Per-group exact or approximate numeric distribution.
#[derive(Debug, Clone)]
enum Numbers {
    /// Count per distinct value, keyed by the value's `f64` bits.
    Exact(HashMap<u64, u64>),
    /// Non-negative values, and the magnitudes of negative values.
    Approx {
        pos: Box<DurationHistogram>,
        neg: Box<DurationHistogram>,
    },
}

impl Numbers {
    fn entries(&self) -> usize {
        match self {
            Numbers::Exact(map) => map.len(),
            Numbers::Approx { .. } => 0,
        }
    }

    fn add(&mut self, value: f64, count: u64) {
        match self {
            Numbers::Exact(map) => *map.entry(value.to_bits()).or_insert(0) += count,
            Numbers::Approx { pos, neg } => {
                let scaled = (value.abs() * APPROX_SCALE).round().min(i64::MAX as f64) as i64;
                if value < 0.0 {
                    neg.record_n(scaled, count);
                } else {
                    pos.record_n(scaled, count);
                }
            }
        }
    }

    fn make_approx(&mut self) {
        if let Numbers::Exact(map) = self {
            let mut approx = Numbers::Approx {
                pos: Box::new(DurationHistogram::new()),
                neg: Box::new(DurationHistogram::new()),
            };
            for (&bits, &count) in map.iter() {
                approx.add(f64::from_bits(bits), count);
            }
            *self = approx;
        }
    }

    fn merge(&mut self, other: Numbers) {
        match (&mut *self, other) {
            (Numbers::Exact(a), Numbers::Exact(b)) => {
                for (bits, count) in b {
                    *a.entry(bits).or_insert(0) += count;
                }
            }
            (Numbers::Approx { pos, neg }, Numbers::Approx { pos: p2, neg: n2 }) => {
                pos.merge(&p2);
                neg.merge(&n2);
            }
            (Numbers::Approx { .. }, mut other @ Numbers::Exact(_)) => {
                other.make_approx();
                self.merge(other);
            }
            (Numbers::Exact(_), other @ Numbers::Approx { .. }) => {
                self.make_approx();
                self.merge(other);
            }
        }
    }

    fn count(&self) -> u64 {
        match self {
            Numbers::Exact(map) => map.values().sum(),
            Numbers::Approx { pos, neg } => pos.count() + neg.count(),
        }
    }

    /// Nearest-rank quantile `phi`; `None` when nothing was recorded.
    fn quantile(&self, phi: f64, sorted: &[(f64, u64)]) -> Option<f64> {
        let n = self.count();
        if n == 0 {
            return None;
        }
        // Shave rounding error first: 0.07 * 100 is 7.000000000000001,
        // which must still be rank 7.
        let rank = ((phi * n as f64 * (1.0 - 4.0 * f64::EPSILON)).ceil() as u64).clamp(1, n);
        match self {
            Numbers::Exact(_) => {
                let mut seen = 0u64;
                for &(value, count) in sorted {
                    seen += count;
                    if seen >= rank {
                        return Some(value);
                    }
                }
                sorted.last().map(|&(value, _)| value)
            }
            Numbers::Approx { pos, neg } => {
                // Negative values come first, largest magnitude first.
                let negs = neg.count();
                if rank <= negs {
                    let magnitude = neg.at_rank(negs - rank + 1)?;
                    Some(-(magnitude as f64) / APPROX_SCALE)
                } else {
                    pos.at_rank(rank - negs).map(|v| v as f64 / APPROX_SCALE)
                }
            }
        }
    }
}

/// Per-group accumulators of one kind.
#[derive(Debug, Clone)]
enum Groups {
    Top(HashMap<Option<String>, HashMap<String, u64>>),
    Quantiles(HashMap<Option<String>, Numbers>),
}

/// One source's (or a merge of sources') aggregation state. Built from an
/// [`AggregateSpec`]; merged with [`ValueAgg::merge`]; answered with
/// [`ValueAgg::finish`].
#[derive(Debug, Clone)]
pub struct ValueAgg {
    groups: Groups,
    /// Rows whose value could not be used (quantiles: not a finite number).
    ignored: u64,
    /// Top values lost entries to the cap.
    incomplete: bool,
}

impl ValueAgg {
    pub fn new(spec: &AggregateSpec) -> Self {
        let groups = match spec.kind {
            AggregateKind::Top(_) => Groups::Top(HashMap::new()),
            AggregateKind::Quantiles(_) => Groups::Quantiles(HashMap::new()),
        };
        Self {
            groups,
            ignored: 0,
            incomplete: false,
        }
    }

    /// Add `count` logs whose `field` value is `value` and whose `by` value
    /// is `group` (`None`: no `by` field requested, or the log lacks it).
    pub fn add(&mut self, group: Option<&str>, value: &str, count: u64) {
        self.add_group(group, std::iter::once((value, count)));
    }

    /// [`add`](Self::add) for many values of one group, with a single
    /// group lookup.
    pub fn add_group<'v>(
        &mut self,
        group: Option<&str>,
        values: impl IntoIterator<Item = (&'v str, u64)>,
    ) {
        match &mut self.groups {
            Groups::Top(groups) => {
                let into = group_entry(groups, group, HashMap::new);
                for (value, count) in values {
                    if count == 0 {
                        continue;
                    }
                    match into.get_mut(value) {
                        Some(c) => *c += count,
                        None => {
                            into.insert(value.to_owned(), count);
                        }
                    }
                }
            }
            Groups::Quantiles(groups) => {
                // Groups share one mode: a new group starts approximate once
                // the others have switched.
                let approx = groups
                    .values()
                    .next()
                    .is_some_and(|n| matches!(n, Numbers::Approx { .. }));
                let into = group_entry(groups, group, || {
                    let mut n = Numbers::Exact(HashMap::new());
                    if approx {
                        n.make_approx();
                    }
                    n
                });
                for (value, count) in values {
                    if count == 0 {
                        continue;
                    }
                    match parse_number(value) {
                        Some(number) => into.add(number, count),
                        None => self.ignored += count,
                    }
                }
            }
        }
    }

    /// Distinct exact `(group, value)` entries held.
    fn entries(&self) -> usize {
        match &self.groups {
            Groups::Top(groups) => groups.values().map(HashMap::len).sum(),
            Groups::Quantiles(groups) => groups.values().map(Numbers::entries).sum(),
        }
    }

    /// Bring the exact entries back under [`MAX_EXACT_ENTRIES`]: top values
    /// keep the most frequent entries and become `incomplete`; quantiles
    /// switch every group to the approximate histogram.
    pub fn enforce_cap(&mut self) {
        if self.entries() <= MAX_EXACT_ENTRIES {
            return;
        }
        match &mut self.groups {
            Groups::Top(groups) => {
                let mut all: Vec<(Option<String>, String, u64)> = groups
                    .drain()
                    .flat_map(|(g, values)| values.into_iter().map(move |(v, c)| (g.clone(), v, c)))
                    .collect();
                all.sort_unstable_by(|a, b| {
                    b.2.cmp(&a.2)
                        .then_with(|| a.1.cmp(&b.1))
                        .then_with(|| a.0.cmp(&b.0))
                });
                all.truncate(MAX_EXACT_ENTRIES);
                for (g, v, c) in all {
                    groups.entry(g).or_default().insert(v, c);
                }
                self.incomplete = true;
            }
            Groups::Quantiles(groups) => {
                for numbers in groups.values_mut() {
                    numbers.make_approx();
                }
            }
        }
    }

    /// Fold `other` into `self` (same spec), then re-apply the cap.
    pub fn merge(&mut self, other: ValueAgg) {
        self.ignored += other.ignored;
        self.incomplete |= other.incomplete;
        match (&mut self.groups, other.groups) {
            (Groups::Top(a), Groups::Top(b)) => {
                for (g, values) in b {
                    let into = a.entry(g).or_default();
                    for (v, c) in values {
                        *into.entry(v).or_insert(0) += c;
                    }
                }
            }
            (Groups::Quantiles(a), Groups::Quantiles(b)) => {
                let approx = a
                    .values()
                    .chain(b.values())
                    .any(|n| matches!(n, Numbers::Approx { .. }));
                for (g, numbers) in b {
                    match a.get_mut(&g) {
                        Some(into) => into.merge(numbers),
                        None => {
                            a.insert(g, numbers);
                        }
                    }
                }
                // Groups share one mode, so the result's flag is uniform.
                if approx {
                    for numbers in a.values_mut() {
                        numbers.make_approx();
                    }
                }
            }
            // Shards of one query share its spec, so their kinds match.
            _ => debug_assert!(false, "value aggregation kinds differ across shards"),
        }
        self.enforce_cap();
    }

    /// The final answer for `spec`: groups ordered by total count
    /// (descending), then by group value.
    pub fn finish(self, spec: &AggregateSpec) -> AggregateResult {
        let mut approximate = false;
        let mut groups: Vec<GroupResult> = match (self.groups, &spec.kind) {
            (Groups::Top(groups), AggregateKind::Top(n)) => groups
                .into_iter()
                .map(|(group, values)| {
                    let count = values.values().sum();
                    let mut values: Vec<(String, u64)> = values.into_iter().collect();
                    values.sort_unstable_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
                    let distinct = values.len();
                    values.truncate(*n);
                    GroupResult {
                        group,
                        count,
                        answer: GroupAnswer::Top { values, distinct },
                    }
                })
                .collect(),
            (Groups::Quantiles(groups), AggregateKind::Quantiles(phis)) => groups
                .into_iter()
                .map(|(group, numbers)| {
                    approximate |= matches!(numbers, Numbers::Approx { .. });
                    let sorted: Vec<(f64, u64)> = match &numbers {
                        Numbers::Exact(map) => {
                            let mut v: Vec<(f64, u64)> =
                                map.iter().map(|(&b, &c)| (f64::from_bits(b), c)).collect();
                            v.sort_unstable_by(|a, b| a.0.total_cmp(&b.0));
                            v
                        }
                        Numbers::Approx { .. } => Vec::new(),
                    };
                    let quantiles = phis
                        .iter()
                        .map(|&phi| (phi, numbers.quantile(phi, &sorted)))
                        .collect();
                    GroupResult {
                        group,
                        count: numbers.count(),
                        answer: GroupAnswer::Quantiles(quantiles),
                    }
                })
                .collect(),
            _ => Vec::new(),
        };
        groups.sort_unstable_by(|a, b| b.count.cmp(&a.count).then_with(|| a.group.cmp(&b.group)));
        AggregateResult {
            field: spec.field.clone(),
            by: spec.by.clone(),
            groups,
            ignored: self.ignored,
            incomplete: self.incomplete,
            approximate,
        }
    }
}

/// The aggregation's answer.
#[derive(Debug, Clone, PartialEq)]
pub struct AggregateResult {
    /// The aggregated field, from the spec.
    pub field: String,
    /// The grouping field, from the spec.
    pub by: Option<String>,
    pub groups: Vec<GroupResult>,
    /// Logs whose value could not be used (quantiles: not a number).
    pub ignored: u64,
    /// Top values lost entries to [`MAX_EXACT_ENTRIES`].
    pub incomplete: bool,
    /// Quantiles came from the approximate histogram.
    pub approximate: bool,
}

/// One group's answer.
#[derive(Debug, Clone, PartialEq)]
pub struct GroupResult {
    /// The `by` value; `None` without `by`, or for logs lacking it.
    pub group: Option<String>,
    /// Values counted in this group: one per log and distinct value, so a
    /// log with several values of the field counts once per value
    /// (quantiles: numeric values only).
    pub count: u64,
    pub answer: GroupAnswer,
}

#[derive(Debug, Clone, PartialEq)]
pub enum GroupAnswer {
    /// The most frequent values (count descending, then value), and how
    /// many distinct values the group had.
    Top {
        values: Vec<(String, u64)>,
        distinct: usize,
    },
    /// Each requested quantile and its value (`None`: no numeric values).
    Quantiles(Vec<(f64, Option<f64>)>),
}

/// The entry for `group`, created with `make` when absent. Allocates the
/// key once per call, so callers batch a group's values
/// ([`ValueAgg::add_group`]).
fn group_entry<'a, T>(
    groups: &'a mut HashMap<Option<String>, T>,
    group: Option<&str>,
    make: impl FnOnce() -> T,
) -> &'a mut T {
    let key: Option<String> = group.map(str::to_owned);
    groups.entry(key).or_insert_with(make)
}

/// A finite number, or `None`; `-0` reads as `0`.
fn parse_number(value: &str) -> Option<f64> {
    let v: f64 = value.trim().parse().ok()?;
    if !v.is_finite() {
        return None;
    }
    Some(if v == 0.0 { 0.0 } else { v })
}

#[cfg(test)]
mod tests;
