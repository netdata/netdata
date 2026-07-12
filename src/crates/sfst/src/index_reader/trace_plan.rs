//! Per-file search-plan evaluation — the traces search substrate
//! (phase-4c decision 22A).
//!
//! The cross-source engine (`sfsq::traces::search`) lowers its predicate
//! AST per candidate file into a neutral [`TracePlan`] — a conjunction of
//! terms in STORAGE field names — and this module executes it: one
//! compilation resolving every term to a position set, then any number
//! of RANK-BOUNDED extractions of the newest-K matched positions.
//! Position algebra ([`super::PosSet`]) stays private to the format
//! crate; the seam is plan in, positions + work out.
//!
//! # The term algebra
//!
//! Every dictionary term is `fields × matcher × negated`:
//!
//! - `fields` holds ONE storage field, or several OR-combined (the
//!   grammar's unscoped attribute = resource ∪ span disjunction);
//! - the matcher selects tokens — exact values and anchored regexes, or
//!   a NUMERIC comparison evaluated at the dictionary (each distinct
//!   stored value parsed once; unparseable values never match);
//! - `negated` applies the pinned negation rule
//!   `presence(fields) ∩ complement(match)`: an absent attribute never
//!   satisfies a negated comparison.
//!
//! Terms resolve per tier: low/mid dictionary lookups and walks; ALL
//! high-card probes (matches AND presence sets alike — a presence probe
//! is just a full-field target set) share ONE stream-batch pass — the
//! `materialize_fields` precedent. Duration terms scan the DURN column
//! slice clipped to the caller's range, testing an OR of inclusive
//! intervals (optionally negated).
//!
//! # Work accounting (the search cost guard's units)
//!
//! [`ScanWork::rows_visited`] counts every row VISITED (not matched) by
//! work that scales with data, per the pinned ceiling units:
//!
//! - each row of a stream batch scanned for high-card probes — counted
//!   ONCE per compilation however many probes share the pass;
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

use super::{FieldLocation, IndexReader, KvId, KvIdSet, PosSet};

/// How a [`PlanTerm::Fields`] term selects dictionary tokens.
#[derive(Debug, Clone, PartialEq)]
pub enum PlanMatcher {
    /// Any `v` in `exact`, or any full-value-anchored regex in
    /// `patterns` matching `v` — the OR-of-values within a term.
    Tokens {
        exact: Vec<String>,
        patterns: Vec<String>,
    },
    /// Distinct stored values parsed as numbers and compared to ANY of
    /// `values` (the pinned dictionary-numeric rule; several values =
    /// the grammar's multi-value numeric equality); stored values that
    /// do not parse never match. See [`numeric_token_matches`].
    Number { cmp: NumberCmp, values: Vec<f64> },
}

/// The numeric comparison of [`PlanMatcher::Number`]. Negated
/// comparisons (`!=`) are the term's `negated` flag, not an operator.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NumberCmp {
    Eq,
    Gt,
    Lt,
    Gte,
    Lte,
}

/// THE numeric token comparator — shared verbatim by the dictionary
/// walks here and by the span-side evaluator in the engine crate, so
/// the raw index path and the canonical span path can never disagree on
/// what a numeric comparison means. A value that does not parse as a
/// number never matches; a NaN `rhs` matches nothing.
pub fn numeric_token_matches(value: &str, cmp: NumberCmp, rhs: f64) -> bool {
    let Ok(v) = value.parse::<f64>() else {
        return false;
    };
    match cmp {
        NumberCmp::Eq => v == rhs,
        NumberCmp::Gt => v > rhs,
        NumberCmp::Lt => v < rhs,
        NumberCmp::Gte => v >= rhs,
        NumberCmp::Lte => v <= rhs,
    }
}

/// One term of a per-file plan (AND-combined by [`TracePlan`]).
#[derive(Debug, Clone, PartialEq)]
pub enum PlanTerm {
    /// Rows selected by `matcher` over the union of `fields` (one
    /// STORAGE field name, or several for the unscoped resource ∪ span
    /// disjunction — the caller constructs names through its vocabulary
    /// mapping; this crate does not interpret them). `negated` applies
    /// `presence(fields) ∩ complement(match)` — a field absent from the
    /// file contributes nothing to presence, so an absent attribute
    /// never satisfies a negated comparison; a positive term on an
    /// absent field matches nothing (the `compile_filter` precedent).
    Fields {
        fields: Vec<String>,
        matcher: PlanMatcher,
        negated: bool,
    },
    /// Rows whose DURN duration lies in ANY of the INCLUSIVE
    /// `[min_ns, max_ns]` intervals (bounds optional; the caller maps
    /// exclusive ops to inclusive integer bounds), the whole test
    /// inverted by `negated` (`!=` = "in no interval"). Errors if the
    /// file carries no DURN column (not a traces file).
    Duration {
        intervals: Vec<(Option<i64>, Option<i64>)>,
        negated: bool,
    },
}

impl PlanTerm {
    /// Convenience for the common single-field positive token term.
    pub fn tokens(field: impl Into<String>, exact: Vec<String>, patterns: Vec<String>) -> Self {
        PlanTerm::Fields {
            fields: vec![field.into()],
            matcher: PlanMatcher::Tokens { exact, patterns },
            negated: false,
        }
    }

    /// Convenience for a single positive duration interval.
    pub fn duration(min_ns: Option<i64>, max_ns: Option<i64>) -> Self {
        PlanTerm::Duration {
            intervals: vec![(min_ns, max_ns)],
            negated: false,
        }
    }
}

/// A per-file search plan: the conjunction of `terms` (empty = match
/// every row — the `{}` query).
#[derive(Debug, Clone, Default, PartialEq)]
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

/// One per-field resolution: a set already known (low/mid tiers, absent
/// fields) or the index of a high-card probe the shared stream-batch
/// pass will fill.
enum Part {
    Ready(PosSet),
    Probe(usize),
}

/// One term's field parts awaiting combination: `matches` OR-combine;
/// `presence` (negated terms only) OR-combines before the difference.
struct ResolvedTerm {
    matches: Vec<Part>,
    presence: Option<Vec<Part>>,
}

impl IndexReader<'_> {
    /// Resolve `plan` against this file for queries within the position
    /// range `[lo, hi)` (the caller's window; pass `(0, record_count)`
    /// for the whole file). See the module docs for the term algebra,
    /// tier resolution, and work accounting. `Ok(None)` = the visited
    /// budget ran out inside the shared stream-batch scan; a truncated
    /// plan is never returned (it would under-approximate).
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

        // ── Resolve every term: dictionary work only (uncounted) ─────
        let mut probes: Vec<(KvIdSet, u8)> = Vec::new();
        let mut resolved: Vec<Option<ResolvedTerm>> = Vec::new(); // None = Duration slot
        let mut durations: Vec<Option<PosSet>> = Vec::new(); // parallel, ugly-free zip below
        for term in &plan.terms {
            match term {
                PlanTerm::Fields {
                    fields,
                    matcher,
                    negated,
                } => {
                    let compiled_patterns = match matcher {
                        PlanMatcher::Tokens { patterns, .. } => patterns
                            .iter()
                            .map(|p| crate::query::compile_pattern(p))
                            .collect::<Result<Vec<_>, _>>()?,
                        PlanMatcher::Number { .. } => Vec::new(),
                    };
                    let mut matches = Vec::with_capacity(fields.len());
                    let mut presence = negated.then(|| Vec::with_capacity(fields.len()));
                    for field in fields {
                        let (m, p) = self.resolve_field(
                            field,
                            matcher,
                            &compiled_patterns,
                            *negated,
                            total,
                            &mut probes,
                        )?;
                        matches.push(m);
                        if let (Some(acc), Some(part)) = (presence.as_mut(), p) {
                            acc.push(part);
                        }
                    }
                    resolved.push(Some(ResolvedTerm { matches, presence }));
                    durations.push(None);
                }
                PlanTerm::Duration { intervals, negated } => {
                    // One DURN pass over the caller's range only (a
                    // fixed-width column read bounded by the window —
                    // the basis for excluding it from the counters);
                    // positions ascend by construction.
                    let column = self.durations()?;
                    let column = column
                        .0
                        .get(range_lo as usize..range_hi as usize)
                        .ok_or_else(|| {
                            crate::Error::CorruptIndex(format!(
                                "trace plan: DURN length {} < range end {range_hi}",
                                column.0.len()
                            ))
                        })?;
                    let in_any = |d: i64| {
                        intervals.iter().any(|&(lo, hi)| {
                            lo.is_none_or(|lo| d >= lo) && hi.is_none_or(|hi| d <= hi)
                        })
                    };
                    let positions: Vec<u32> = column
                        .iter()
                        .enumerate()
                        .filter(|&(_, &d)| in_any(d) != *negated)
                        .map(|(i, _)| range_lo + i as u32)
                        .collect();
                    resolved.push(None);
                    durations.push(Some(PosSet::from_sorted(positions, total)));
                }
            }
        }

        // ── Probe-free short-circuit: if the terms already resolvable
        // collapse the conjunction, the stream-batch scan is wasted
        // work — skip it, deterministically. ──────────────────────────
        let combine_ready = |term: &ResolvedTerm| -> Option<PosSet> {
            // A term combines WITHOUT the scan only if all parts are ready.
            let or_parts = |parts: &[Part]| -> Option<PosSet> {
                let mut acc = PosSet::empty(total);
                for part in parts {
                    match part {
                        Part::Ready(set) => acc.or_assign(set),
                        Part::Probe(_) => return None,
                    }
                }
                Some(acc)
            };
            let matched = or_parts(&term.matches)?;
            match &term.presence {
                None => Some(matched),
                Some(parts) => {
                    let mut presence = or_parts(parts)?;
                    presence.and_not_assign(&matched);
                    Some(presence)
                }
            }
        };
        let ready_empty = resolved.iter().flatten().any(|term| {
            combine_ready(term).is_some_and(|set| set.is_empty())
        }) || durations.iter().flatten().any(PosSet::is_empty);
        let probe_sets: Vec<PosSet> = if probes.is_empty() || ready_empty {
            probes.iter().map(|_| PosSet::empty(total)).collect()
        } else {
            let refs: Vec<(&KvIdSet, u8)> = probes.iter().map(|(t, m)| (t, *m)).collect();
            match self.scan_high_multi(&refs, total, ceiling, &mut work.rows_visited)? {
                Some(sets) => sets,
                None => return Ok(None), // budget ran out mid-scan
            }
        };
        if ready_empty {
            return Ok(Some(CompiledTracePlan {
                set: PosSet::empty(total),
                universe: total,
            }));
        }

        // ── Combine in plan order ─────────────────────────────────────
        let materialize = |parts: &[Part]| -> PosSet {
            let mut acc = PosSet::empty(total);
            for part in parts {
                match part {
                    Part::Ready(set) => acc.or_assign(set),
                    Part::Probe(i) => acc.or_assign(&probe_sets[*i]),
                }
            }
            acc
        };
        let mut acc: Option<PosSet> = None;
        for (term, duration) in resolved.iter().zip(&durations) {
            let set = match (term, duration) {
                (Some(term), _) => {
                    let matched = materialize(&term.matches);
                    match &term.presence {
                        None => matched,
                        Some(parts) => {
                            let mut presence = materialize(parts);
                            presence.and_not_assign(&matched);
                            presence
                        }
                    }
                }
                (None, Some(set)) => set.clone(),
                (None, None) => unreachable!("every slot is a term or a duration"),
            };
            match &mut acc {
                None => acc = Some(set),
                Some(a) => a.and_assign(&set),
            }
            if acc.as_ref().is_some_and(PosSet::is_empty) {
                break;
            }
        }

        Ok(Some(CompiledTracePlan {
            set: acc.unwrap_or_else(|| PosSet::full(total)),
            universe: total,
        }))
    }

    /// Resolve one field of a term: the MATCH part and (for negated
    /// terms) the PRESENCE part, each a ready set (low/mid, absent) or
    /// a registered high-card probe. Dictionary work only.
    fn resolve_field(
        &self,
        field: &str,
        matcher: &PlanMatcher,
        compiled_patterns: &[regex::bytes::Regex],
        want_presence: bool,
        total: u32,
        probes: &mut Vec<(KvIdSet, u8)>,
    ) -> Result<(Part, Option<Part>), crate::Error> {
        let prefix_len = field.len() + 1;
        // Whether a `field=value` key's VALUE bytes match this matcher
        // (patterns are anchored; numeric values parse-and-compare).
        let value_matches = |kv_bytes: &[u8]| -> bool {
            let value = &kv_bytes[prefix_len..];
            match matcher {
                PlanMatcher::Tokens { exact, .. } => {
                    exact.iter().any(|e| e.as_bytes() == value)
                        || compiled_patterns.iter().any(|r| r.is_match(value))
                }
                PlanMatcher::Number { cmp, values } => std::str::from_utf8(value)
                    .is_ok_and(|v| values.iter().any(|&rhs| numeric_token_matches(v, *cmp, rhs))),
            }
        };

        let location = match self.locate_field(field) {
            Some(loc) => loc,
            None => {
                // Absent field: no matches, no presence.
                let presence = want_presence.then(|| Part::Ready(PosSet::empty(total)));
                return Ok((Part::Ready(PosSet::empty(total)), presence));
            }
        };

        match location {
            FieldLocation::Low => {
                let prefix = format!("{field}=");
                let mut matched = PosSet::empty(total);
                let mut presence = want_presence.then(|| PosSet::empty(total));
                self.primary
                    .prefix_for_each(prefix.as_bytes(), |kv_bytes, bv| {
                        if value_matches(kv_bytes) {
                            matched.or_assign(&PosSet::from_value(bv));
                        }
                        if let Some(p) = presence.as_mut() {
                            p.or_assign(&PosSet::from_value(bv));
                        }
                    });
                Ok((Part::Ready(matched), presence.map(Part::Ready)))
            }
            FieldLocation::Mid(idx) => {
                let chunk = self.sfst.mid_field(idx)?;
                let mut matched = PosSet::empty(total);
                let mut presence = want_presence.then(|| PosSet::empty(total));
                chunk.for_each(|kv_bytes, bv| {
                    if value_matches(kv_bytes) {
                        matched.or_assign(&PosSet::from_value(bv));
                    }
                    if let Some(p) = presence.as_mut() {
                        p.or_assign(&PosSet::from_value(bv));
                    }
                });
                Ok((Part::Ready(matched), presence.map(Part::Ready)))
            }
            FieldLocation::High(idx) => {
                let hf = self.sfst.high_field(idx)?;
                let base = self.high_kv_id(idx, 0).0;
                let mut targets = KvIdSet::new(base, hf.len() as u32);
                let mut mask: u8 = 0;
                match matcher {
                    // Exact-only terms keep the pinned KvId fast path
                    // (binary search per value); everything else walks
                    // the dictionary once.
                    PlanMatcher::Tokens { exact, patterns } if patterns.is_empty() => {
                        for value in exact {
                            let kv = format!("{field}={value}");
                            if let Ok(local) = hf.binary_search(kv.as_bytes()) {
                                targets.insert(KvId(base + local as u32));
                                mask |= hf.masks[local];
                            }
                        }
                    }
                    _ => {
                        for (local, key) in hf.keys().enumerate() {
                            if value_matches(key) {
                                targets.insert(KvId(base + local as u32));
                                mask |= hf.masks[local];
                            }
                        }
                    }
                }
                let matched = if targets.is_empty() {
                    // Nothing to find: skip the scan for this part.
                    Part::Ready(PosSet::empty(total))
                } else {
                    probes.push((targets, mask));
                    Part::Probe(probes.len() - 1)
                };
                let presence = if want_presence {
                    // Presence = any KvId of the field: a full-range
                    // target set probed in the same shared pass.
                    let mut all = KvIdSet::new(base, hf.len() as u32);
                    let mut all_mask: u8 = 0;
                    for (local, m) in hf.masks.iter().enumerate() {
                        all.insert(KvId(base + local as u32));
                        all_mask |= m;
                    }
                    probes.push((all, all_mask));
                    Some(Part::Probe(probes.len() - 1))
                } else {
                    None
                };
                Ok((matched, presence))
            }
        }
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
