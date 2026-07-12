//! Per-file search-plan evaluation — the traces search substrate
//! (phase-4c decision 22A).
//!
//! The cross-source engine (`sfsq::traces::search`) lowers its predicate
//! AST per candidate file into a neutral [`TracePlan`] — a conjunction of
//! terms in STORAGE field names — and this module executes it: one
//! compilation resolving every term to a position set (dictionary lookups
//! for low/mid tiers, ONE shared stream-batch scan for all high-card
//! terms — the `materialize_fields` precedent — and a DURN column pass
//! for duration bounds), then any number of RANK-BOUNDED extractions of
//! the newest-K matched positions. Position algebra ([`super::PosSet`])
//! stays private to the format crate; the seam is plan in,
//! positions + work out.
//!
//! # Work accounting (the search cost guard's units)
//!
//! [`ScanWork::rows_visited`] counts every row VISITED (not matched) by
//! work that scales with data, per the pinned ceiling units:
//!
//! - each row of a stream batch scanned for high-card terms — counted
//!   ONCE per compilation however many high-card terms share the pass;
//! - each EMITTED matched position (rank-bounded extraction makes
//!   emission itself the bounded unit).
//!
//! The caller's `ceiling` is enforced INSIDE the stream-batch scan (a
//! counter alone could overshoot by a whole file): a compilation whose
//! scan would exceed it stops and returns `Ok(None)` — a truncated
//! plan must never be used, it would under-approximate.
//!
//! Deliberately EXCLUDED (bounded by construction): dictionary walks
//! (FST/arena — dictionary-sized), the DURN column pass (clipped to the
//! caller's position range, so it is window-bounded) and other
//! fixed-width column reads, and the `range_cardinality` probes of the
//! extraction (tree walks, not row visits).
//!
//! # Rank-bounded extraction (pin R3-1)
//!
//! Positions are chronological within a file, so the newest-K matched
//! positions live in the matched set's TAIL band. `newest_in_range`
//! finds that band with `range_cardinality` probes — a partition-point
//! search for the largest band start still holding K matches, the exact
//! form of the pinned "descending sub-ranges, widen until ≥ K" probe —
//! and iterates ONLY the band: emission is exactly `min(K, matched)`
//! positions, never the full matched set. `TracePlan::default()` (match
//! all) compiles to the full-range set, so extraction costs O(K) per
//! file — the most common UI query stays bounded.

use crate::query::Matcher;

use super::{FieldLocation, IndexReader, KvIdSet, PosSet};

/// One term of a per-file plan (AND-combined by [`TracePlan`]).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PlanTerm {
    /// Rows carrying `field=v` for any `v` in `exact` or matching any of
    /// `patterns` (full-value-anchored regex sources, the `=~` op) — the
    /// OR-of-values within one field. A field absent from the file
    /// matches nothing (absent-matches-nothing, the `compile_filter`
    /// precedent).
    Tokens {
        /// STORAGE field name (the caller constructs it through its
        /// vocabulary mapping; this crate does not interpret names).
        field: String,
        exact: Vec<String>,
        patterns: Vec<String>,
    },
    /// Rows whose DURN duration lies in the INCLUSIVE `[min_ns, max_ns]`
    /// interval (either bound optional; the caller maps its `>`/`<`
    /// exclusive ops to inclusive bounds on integer nanoseconds).
    /// Errors if the file carries no DURN column (not a traces file).
    Duration {
        min_ns: Option<i64>,
        max_ns: Option<i64>,
    },
}

/// A per-file search plan: the conjunction of `terms` (empty = match
/// every row — the `{}` query).
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct TracePlan {
    pub terms: Vec<PlanTerm>,
}

/// Scan-work counters for the search cost guard — see the module docs
/// for exactly what counts.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct ScanWork {
    pub rows_visited: u64,
}

/// A [`TracePlan`] resolved against one file: the matched-position set,
/// ready for counting and rank-bounded extraction. Holds no borrow of
/// the reader — the engine keeps it alive across grow-K rounds without
/// re-paying the compilation (and its stream-batch work) per round.
#[derive(Debug)]
pub struct CompiledTracePlan {
    set: PosSet,
    universe: u32,
}

impl IndexReader<'_> {
    /// Resolve `plan` against this file for queries within the position
    /// range `[lo, hi)` (the caller's window; pass `(0, record_count)`
    /// for the whole file). Dictionary terms resolve per tier (low/mid
    /// lookups; high-card terms share ONE stream-batch scan, its visited
    /// rows counted once into `work` and bounded by `ceiling` —
    /// `Ok(None)` when the budget ran out mid-scan, deterministically);
    /// duration bounds scan the DURN column CLIPPED to the range (which
    /// is what keeps that pass out of the work units). An empty
    /// conjunction compiles to the full range. A term whose field this
    /// file lacks collapses the conjunction to empty — and
    /// short-circuits the remaining work, deterministically (term order
    /// is the plan's, high-card scan last).
    ///
    /// The compiled plan answers `count_in_range`/`newest_in_range`
    /// correctly only for sub-ranges of `[lo, hi)` (duration positions
    /// outside it were never collected).
    pub fn compile_trace_plan(
        &self,
        plan: &TracePlan,
        range: (u32, u32),
        ceiling: u64,
        work: &mut ScanWork,
    ) -> Result<Option<CompiledTracePlan>, crate::Error> {
        let total = self.summary().record_count;
        let (range_lo, range_hi) = (range.0.min(total), range.1.min(total));
        let mut acc: Option<PosSet> = None;
        let and_in = |set: PosSet, acc: &mut Option<PosSet>| match acc {
            None => *acc = Some(set),
            Some(a) => a.and_assign(&set),
        };
        let empty = |acc: &Option<PosSet>| acc.as_ref().is_some_and(PosSet::is_empty);

        // High-card terms are deferred and share one stream-batch pass.
        let mut high_terms: Vec<(KvIdSet, u8)> = Vec::new();

        for term in &plan.terms {
            if empty(&acc) {
                break;
            }
            match term {
                PlanTerm::Tokens {
                    field,
                    exact,
                    patterns,
                } => match self.locate_field(field) {
                    None => and_in(PosSet::empty(total), &mut acc),
                    Some(FieldLocation::High(idx)) => {
                        let exact_refs: Vec<&str> = exact.iter().map(String::as_str).collect();
                        let compiled: Vec<regex::bytes::Regex> = patterns
                            .iter()
                            .map(|p| crate::query::compile_pattern(p))
                            .collect::<Result<_, _>>()?;
                        let (targets, mask) =
                            self.high_targets(field, idx, &exact_refs, &compiled)?;
                        // A term matching no dictionary value collapses the
                        // conjunction NOW — deferring it would run the
                        // stream-batch scan for the other high terms only to
                        // AND everything to empty afterwards.
                        if targets.is_empty() {
                            and_in(PosSet::empty(total), &mut acc);
                        } else {
                            high_terms.push((targets, mask));
                        }
                    }
                    Some(FieldLocation::Low | FieldLocation::Mid(_)) => {
                        // The existing per-tier resolution (exact lookups +
                        // anchored-pattern dictionary walks) — dictionary
                        // work, excluded from the counters.
                        let matchers: Vec<Matcher> = exact
                            .iter()
                            .map(|v| Matcher::Exact(v.clone()))
                            .chain(patterns.iter().map(|p| Matcher::Pattern(p.clone())))
                            .collect();
                        and_in(self.field_values_or(field, &matchers)?, &mut acc);
                    }
                },
                PlanTerm::Duration { min_ns, max_ns } => {
                    // One DURN pass over the caller's range only (a
                    // fixed-width column read bounded by the window —
                    // the basis for excluding it from the counters);
                    // positions ascend by construction.
                    let durations = self.durations()?;
                    let lo = min_ns.unwrap_or(i64::MIN);
                    let hi = max_ns.unwrap_or(i64::MAX);
                    let column = durations
                        .0
                        .get(range_lo as usize..range_hi as usize)
                        .ok_or_else(|| {
                            crate::Error::CorruptIndex(format!(
                                "trace plan: DURN length {} < range end {range_hi}",
                                durations.0.len()
                            ))
                        })?;
                    let positions: Vec<u32> = column
                        .iter()
                        .enumerate()
                        .filter(|&(_, &d)| lo <= d && d <= hi)
                        .map(|(i, _)| range_lo + i as u32)
                        .collect();
                    and_in(PosSet::from_sorted(positions, total), &mut acc);
                }
            }
        }

        if !high_terms.is_empty() && !empty(&acc) {
            let refs: Vec<(&KvIdSet, u8)> =
                high_terms.iter().map(|(t, m)| (t, *m)).collect();
            let Some(sets) =
                self.scan_high_multi(&refs, total, ceiling, &mut work.rows_visited)?
            else {
                return Ok(None); // budget ran out mid-scan
            };
            for set in sets {
                and_in(set, &mut acc);
            }
        }

        Ok(Some(CompiledTracePlan {
            set: acc.unwrap_or_else(|| PosSet::full(total)),
            universe: total,
        }))
    }
}

impl CompiledTracePlan {
    /// Matched-position count within `[lo, hi)` — a bitmap-tree walk, no
    /// row visits.
    pub fn count_in_range(&self, lo: u32, hi: u32) -> u64 {
        let hi = hi.min(self.universe);
        if lo >= hi {
            return 0;
        }
        self.set.range_cardinality(lo, hi)
    }

    /// The newest `k` matched positions within `[lo, hi)`, ASCENDING
    /// (fewer if fewer match). Rank-bounded per the module docs: a
    /// partition-point search over `range_cardinality` locates the tail
    /// band holding exactly `min(k, matched)` positions, and only that
    /// band is iterated — every emitted position counts into `work`.
    pub fn newest_in_range(
        &self,
        lo: u32,
        hi: u32,
        k: usize,
        work: &mut ScanWork,
    ) -> Vec<u32> {
        let hi = hi.min(self.universe);
        if k == 0 || lo >= hi {
            return Vec::new();
        }
        let target = (k as u64).min(self.set.range_cardinality(lo, hi));
        if target == 0 {
            return Vec::new();
        }

        // Largest band start `a` with count(a..hi) >= target. Counts are
        // non-increasing in the start and positions are distinct, so the
        // boundary band holds EXACTLY `target` matches. Invariant:
        // count(a..hi) >= target > count(z..hi).
        let (mut a, mut z) = (lo, hi);
        while z - a > 1 {
            let m = a + (z - a) / 2;
            if self.set.range_cardinality(m, hi) >= target {
                a = m;
            } else {
                z = m;
            }
        }

        let mut band = PosSet::range(a, hi, self.universe);
        band.and_assign(&self.set);
        let out: Vec<u32> = band.iter().collect();
        debug_assert_eq!(out.len() as u64, target, "band holds exactly `target`");
        work.rows_visited += out.len() as u64;
        out
    }
}
