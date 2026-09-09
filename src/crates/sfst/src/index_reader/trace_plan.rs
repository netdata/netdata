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
    /// Rows whose SPAN/PSPN column id is any of `ids` (`negated`: an id
    /// PRESENT and matching none — the all-zero UNSET sentinel counts
    /// as ABSENT, so a root span never satisfies a negated parent-id
    /// comparison). A fixed-width column read, excluded from the work
    /// units.
    IdColumn {
        column: IdColumnKind,
        ids: Vec<crate::SpanId>,
        negated: bool,
    },
    /// The EVENT subgroup (pinned spanset semantics C-3): rows where a
    /// SINGLE event satisfies every condition. Flat-token pre-filter
    /// (AND of each field condition's flat set; the events.name
    /// presence set when only computed conditions remain) → MANDATORY
    /// EVNB structural refine at KvId level; every event record visited
    /// by the refine feeds the counter and the budget. An absent EVNB
    /// chunk is a correct no-match; a corrupt one errors. POSITIVE
    /// conditions only — negated subgroup semantics are an open design
    /// question (see the engine crate's SOW).
    EventGroup { conditions: Vec<GroupCondition> },
    /// The LINK subgroup — as [`EventGroup`](Self::EventGroup) over
    /// LNKB. Id conditions have NO flat tokens (deliberate), so an
    /// id-only group scans every row in range (the pinned no-prefilter
    /// `link:*` unit).
    LinkGroup { conditions: Vec<GroupCondition> },
}

/// One condition of an event/link subgroup.
#[derive(Debug, Clone, PartialEq)]
pub enum GroupCondition {
    /// A dictionary condition on a FULL storage name
    /// (`events.attributes.X`, `events.name`, `links.attributes.X`) —
    /// refined by KvId membership against the tokens the matcher
    /// selects from the field's dictionary.
    Field { field: String, matcher: PlanMatcher },
    /// `event:timeSinceStart` in any of the inclusive ns intervals —
    /// computed in refine from the event time and the row start
    /// (event groups only).
    TimeSinceStart {
        intervals: Vec<(Option<i64>, Option<i64>)>,
    },
    /// Link span/trace ids (link groups only; read from the LinkRef).
    LinkSpanIds(Vec<crate::SpanId>),
    LinkTraceIds(Vec<crate::TraceId>),
}

/// Which per-row id column an [`PlanTerm::IdColumn`] term scans.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum IdColumnKind {
    SpanId,
    ParentSpanId,
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
    /// The pinned "visited rows" ceiling unit — despite the name it
    /// counts every budgeted VISIT, not only whole rows: stream-batch
    /// rows scanned, refine rows AND event/link records walked, and
    /// emitted extraction positions. One counter, one ceiling.
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

/// One event/link subgroup awaiting the post-pass structural refine.
struct GroupSlot {
    /// Which term slot the refined set fills.
    term_index: usize,
    is_event: bool,
    /// Flat pre-filter parts (AND-combined; empty = whole range).
    prefilter: Vec<Part>,
    conditions: Vec<ResolvedGroupCondition>,
    /// Statically proven empty for THIS file (its chunk is absent, or a
    /// condition's matching-KvId set is empty) — joins the pre-scan
    /// short-circuit so an unrelated budgeted scan is never charged for
    /// a file this group already ruled out.
    impossible: bool,
}

/// A subgroup condition resolved for KvId-level refine.
enum ResolvedGroupCondition {
    /// Matching KvIds of the condition's dictionary field (empty =
    /// unsatisfiable in this file).
    Kv(std::collections::HashSet<u32>),
    TimeSince(Vec<(Option<i64>, Option<i64>)>),
    LinkSpanIds(Vec<crate::SpanId>),
    LinkTraceIds(Vec<crate::TraceId>),
}

/// Iterate candidate rows through a per-row refine closure, counting
/// every visited row and (via the closure's counter) every event/link
/// record into the budgeted work counter — `None` = budget ran out
/// (record-count overshoot is bounded by one row's records).
fn refine_rows(
    positions: impl Iterator<Item = u32>,
    total: u32,
    ceiling: u64,
    work: &mut ScanWork,
    mut row_matches: impl FnMut(u32, &mut u64) -> bool,
) -> Option<PosSet> {
    let mut matched: Vec<u32> = Vec::new();
    for pos in positions {
        work.rows_visited += 1;
        if work.rows_visited > ceiling {
            return None;
        }
        let mut records = 0u64;
        let hit = row_matches(pos, &mut records);
        work.rows_visited += records;
        if work.rows_visited > ceiling {
            return None;
        }
        if hit {
            matched.push(pos);
        }
    }
    Some(PosSet::from_sorted(matched, total))
}

/// Whether a `field=value` key's VALUE bytes satisfy `matcher` (shared
/// by the per-tier set walks and the subgroup KvId collection).
fn matcher_hits(
    kv_bytes: &[u8],
    prefix_len: usize,
    matcher: &PlanMatcher,
    compiled_patterns: &[regex::bytes::Regex],
) -> bool {
    let value = &kv_bytes[prefix_len..];
    match matcher {
        PlanMatcher::Tokens { exact, .. } => {
            exact.iter().any(|e| e.as_bytes() == value)
                || compiled_patterns.iter().any(|r| r.is_match(value))
        }
        PlanMatcher::Number { cmp, values } => std::str::from_utf8(value)
            .is_ok_and(|v| values.iter().any(|&rhs| numeric_token_matches(v, *cmp, rhs))),
    }
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
        let mut groups: Vec<GroupSlot> = Vec::new(); // event/link subgroups (refined post-pass)
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
                PlanTerm::IdColumn {
                    column,
                    ids,
                    negated,
                } => {
                    // Fixed-width column read over the caller's range
                    // (excluded from the work units, like DURN).
                    let ids_match = |id: crate::SpanId| -> bool {
                        if id.is_unset() {
                            return false; // UNSET = absent, both polarities
                        }
                        ids.contains(&id) != *negated
                    };
                    // SPAN and PSPN are distinct arena types over the
                    // same SpanId values; collect through one closure.
                    let collect = |len: usize,
                                   get: &dyn Fn(usize) -> crate::SpanId|
                     -> Result<Vec<u32>, crate::Error> {
                        if (len as u32) < range_hi {
                            return Err(crate::Error::CorruptIndex(format!(
                                "trace plan: id column length {len} < range end {range_hi}",
                            )));
                        }
                        Ok((range_lo..range_hi)
                            .filter(|&pos| ids_match(get(pos as usize)))
                            .collect())
                    };
                    let positions: Vec<u32> = match column {
                        IdColumnKind::SpanId => {
                            let c = self.span_ids()?;
                            collect(c.len(), &|i| c.get(i))?
                        }
                        IdColumnKind::ParentSpanId => {
                            let c = self.parent_span_ids()?;
                            collect(c.len(), &|i| c.get(i))?
                        }
                    };
                    resolved.push(None);
                    durations.push(Some(PosSet::from_sorted(positions, total)));
                }
                PlanTerm::EventGroup { conditions } | PlanTerm::LinkGroup { conditions } => {
                    let is_event = matches!(term, PlanTerm::EventGroup { .. });
                    // Pre-filter: AND of each Field condition's flat
                    // set; a group with only computed conditions falls
                    // back to the events.name presence set (every event
                    // interns a name token) or — for link groups, whose
                    // id conditions deliberately have no flat tokens —
                    // to the whole range (the pinned no-prefilter scan).
                    let mut parts: Vec<(Part, Option<Part>)> = Vec::new();
                    let mut resolved_conditions: Vec<ResolvedGroupCondition> = Vec::new();
                    for condition in conditions {
                        match condition {
                            GroupCondition::Field { field, matcher } => {
                                let compiled_patterns = match matcher {
                                    PlanMatcher::Tokens { patterns, .. } => patterns
                                        .iter()
                                        .map(|pat| crate::query::compile_pattern(pat))
                                        .collect::<Result<Vec<_>, _>>()?,
                                    PlanMatcher::Number { .. } => Vec::new(),
                                };
                                let (m, _) = self.resolve_field(
                                    field,
                                    matcher,
                                    &compiled_patterns,
                                    false,
                                    total,
                                    &mut probes,
                                )?;
                                parts.push((m, None));
                                resolved_conditions.push(ResolvedGroupCondition::Kv(
                                    self.field_matching_kvids(field, matcher, &compiled_patterns)?,
                                ));
                            }
                            GroupCondition::TimeSinceStart { intervals } => {
                                resolved_conditions
                                    .push(ResolvedGroupCondition::TimeSince(intervals.clone()));
                            }
                            GroupCondition::LinkSpanIds(ids) => {
                                resolved_conditions
                                    .push(ResolvedGroupCondition::LinkSpanIds(ids.clone()));
                            }
                            GroupCondition::LinkTraceIds(ids) => {
                                resolved_conditions
                                    .push(ResolvedGroupCondition::LinkTraceIds(ids.clone()));
                            }
                        }
                    }
                    if parts.is_empty() && is_event {
                        // Presence pre-filter via events.name — every
                        // event interns a name token. Link groups have
                        // NO equivalent (links intern no universal
                        // token), which is exactly why id-only link
                        // groups are the pinned no-prefilter full scan.
                        let names = PlanMatcher::Tokens {
                            exact: Vec::new(),
                            patterns: vec![".*".to_string()],
                        };
                        let compiled = vec![crate::query::compile_pattern(".*")?];
                        let (m, _) = self.resolve_field(
                            "events.name",
                            &names,
                            &compiled,
                            false,
                            total,
                            &mut probes,
                        )?;
                        parts.push((m, None));
                    }
                    let chunk_absent = if is_event {
                        !self.has_event_index()
                    } else {
                        !self.has_link_index()
                    };
                    let impossible = chunk_absent
                        || resolved_conditions.iter().any(|c| {
                            matches!(c, ResolvedGroupCondition::Kv(kvids) if kvids.is_empty())
                        });
                    resolved.push(None);
                    durations.push(None);
                    groups.push(GroupSlot {
                        term_index: resolved.len() - 1,
                        is_event,
                        prefilter: parts.into_iter().map(|(m, _)| m).collect(),
                        conditions: resolved_conditions,
                        impossible,
                    });
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
        let group_ready_empty = |group: &GroupSlot| -> bool {
            if group.impossible {
                return true;
            }
            // An all-ready pre-filter that ANDs to empty leaves the
            // refine nothing to visit.
            let mut acc: Option<PosSet> = None;
            for part in &group.prefilter {
                let Part::Ready(set) = part else { return false };
                match &mut acc {
                    None => acc = Some(set.clone()),
                    Some(a) => a.and_assign(set),
                }
            }
            acc.is_some_and(|set| set.is_empty())
        };
        let ready_empty = resolved.iter().flatten().any(|term| {
            combine_ready(term).is_some_and(|set| set.is_empty())
        }) || durations.iter().flatten().any(PosSet::is_empty)
            || groups.iter().any(group_ready_empty);
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

        // ── Structural refine per subgroup (MANDATORY, decision 10):
        // a single event/link must satisfy the whole subgroup — every
        // event/link record visited counts and the budget aborts. ─────
        for group in &groups {
            if group.impossible {
                durations[group.term_index] = Some(PosSet::empty(total));
                continue;
            }
            let mut prefilter: Option<PosSet> = None;
            for part in &group.prefilter {
                let set = match part {
                    Part::Ready(set) => set.clone(),
                    Part::Probe(i) => probe_sets[*i].clone(),
                };
                match &mut prefilter {
                    None => prefilter = Some(set),
                    Some(acc) => acc.and_assign(&set),
                }
            }
            let mut range_set = PosSet::range(range_lo, range_hi, total);
            if let Some(pre) = prefilter {
                range_set.and_assign(&pre);
            }
            let set = if group.is_event {
                if self.has_event_index() {
                    let events = self.event_index()?;
                    let timestamps = self.load_timestamps()?;
                    let Some(set) = refine_rows(
                        range_set.iter(),
                        total,
                        ceiling,
                        work,
                        |pos, count| {
                            let start = timestamps.at(pos).unwrap_or(0);
                            let mut hit = false;
                            for e in events.events_for_row(pos) {
                                *count += 1;
                                let ok = group.conditions.iter().all(|c| match c {
                                    ResolvedGroupCondition::Kv(kvids) => {
                                        kvids.contains(&e.name.0)
                                            || e.attr_refs.iter().any(|id| kvids.contains(&id.0))
                                    }
                                    ResolvedGroupCondition::TimeSince(intervals) => {
                                        // Saturating, matching the recorded
                                        // ingest semantics — a wrapping cast
                                        // would flip far-future times negative.
                                        let t = i64::try_from(e.time_unix_nano)
                                            .unwrap_or(i64::MAX);
                                        let dt = t.saturating_sub(start);
                                        intervals.iter().any(|&(lo, hi)| {
                                            lo.is_none_or(|lo| dt >= lo)
                                                && hi.is_none_or(|hi| dt <= hi)
                                        })
                                    }
                                    _ => false, // link-only conditions never match events
                                });
                                if ok {
                                    hit = true;
                                    break;
                                }
                            }
                            hit
                        },
                    ) else {
                        return Ok(None);
                    };
                    set
                } else {
                    PosSet::empty(total) // absent chunk = correct no-match
                }
            } else if self.has_link_index() {
                let links = self.link_index()?;
                let Some(set) = refine_rows(range_set.iter(), total, ceiling, work, |pos, count| {
                    let mut hit = false;
                    for l in links.links_for_row(pos) {
                        *count += 1;
                        let ok = group.conditions.iter().all(|c| match c {
                            ResolvedGroupCondition::Kv(kvids) => {
                                l.attr_refs.iter().any(|id| kvids.contains(&id.0))
                            }
                            ResolvedGroupCondition::LinkSpanIds(ids) => ids.contains(&l.span_id),
                            ResolvedGroupCondition::LinkTraceIds(ids) => ids.contains(&l.trace_id),
                            ResolvedGroupCondition::TimeSince(_) => false, // event-only
                        });
                        if ok {
                            hit = true;
                            break;
                        }
                    }
                    hit
                }) else {
                    return Ok(None);
                };
                set
            } else {
                PosSet::empty(total)
            };
            durations[group.term_index] = Some(set);
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
        let value_matches =
            |kv_bytes: &[u8]| matcher_hits(kv_bytes, prefix_len, matcher, compiled_patterns);

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

    /// The KvIds of `field`'s dictionary tokens selected by `matcher` —
    /// the subgroup refine's membership sets. KvIds are assigned per
    /// field in field-table order with values in dictionary order (the
    /// tested `resolve_kv_strings` labeling), so the id of the `i`-th
    /// walked value is `field_start + i`. An absent field yields the
    /// empty set (unsatisfiable subgroup condition in this file).
    fn field_matching_kvids(
        &self,
        field: &str,
        matcher: &PlanMatcher,
        compiled_patterns: &[regex::bytes::Regex],
    ) -> Result<std::collections::HashSet<u32>, crate::Error> {
        let mut start = 0u32;
        let mut present = false;
        for entry in self.field_table().iter() {
            if entry.name == field {
                present = true;
                break;
            }
            start += entry.cardinality;
        }
        let mut out = std::collections::HashSet::new();
        if !present {
            return Ok(out);
        }
        let prefix_len = field.len() + 1;
        let hits = |kv: &[u8]| matcher_hits(kv, prefix_len, matcher, compiled_patterns);
        match self.locate_field(field) {
            None => {}
            Some(FieldLocation::Low) => {
                let prefix = format!("{field}=");
                let mut off = 0u32;
                self.primary.prefix_for_each(prefix.as_bytes(), |kv_bytes, _| {
                    if hits(kv_bytes) {
                        out.insert(start + off);
                    }
                    off += 1;
                });
            }
            Some(FieldLocation::Mid(idx)) => {
                let chunk = self.sfst.mid_field(idx)?;
                let mut off = 0u32;
                chunk.for_each(|kv_bytes, _| {
                    if hits(kv_bytes) {
                        out.insert(start + off);
                    }
                    off += 1;
                });
            }
            Some(FieldLocation::High(idx)) => {
                let hf = self.sfst.high_field(idx)?;
                for (local, key) in hf.keys().enumerate() {
                    if hits(key) {
                        out.insert(start + local as u32);
                    }
                }
            }
        }
        Ok(out)
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
