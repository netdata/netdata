//! Stateful filter expressions for the journal file's filtered iteration
//! path. [`FilterExpr`] resolves `FIELD=VALUE` matches to data objects and
//! steps through each matched object's entry chain - the list of entries
//! carrying that payload - returning entry-object offsets.
//! [`JournalFilter`] accumulates caller-supplied matches into one
//! expression.
//!
//! Data flow: `JournalReader::add_match` feeds a pending [`JournalFilter`]
//! (field-name remapping happens there); group-closing calls
//! (`add_conjunction`/`add_disjunction`) resolve what they flush right
//! away, and the first `step` resolves what is left against the file,
//! installing the result on the cursor, while `build_filter` resolves and
//! returns it without installing. From then on
//! the cursor's filtered path drives the expression one entry at a time
//! (`JournalCursor::resolve_filter_location`), rewinding with `head`/`tail`
//! before resolving a location from scratch, because `next`/`previous`
//! continue from the scan position instead of seeking (the resolver's
//! fresh-resolution arms rewind first).
//!
//! A match is an exact, case-sensitive byte match of one full `FIELD=VALUE`
//! payload: hash lookup in the data hash table plus raw-payload comparison
//! ([`JournalFile::find_data_offset`]). A payload the file does not contain - or whose
//! data object has no entry chain - resolves to [`FilterExpr::None`], which
//! matches nothing: every step then ends the iteration (`Ok(None)` here,
//! `Ok(false)` from
//! [`JournalCursor::step`](crate::file::cursor::JournalCursor::step)).
//!
//! Entry chains list their entries in write order, and entry offsets share
//! one file-global, write-ordered space (every object is appended at
//! `JournalWriter`'s single growing `append_offset`), so offsets
//! from different chains are comparable - what the disjunction min/max and
//! the conjunction fixed-point iteration below rely on.
//!
//! The expression is bound to the file it was built against: data-object
//! offsets and entry-array positions are followed as-is, with no
//! re-resolution mid-scan, so it must be used with that file only.
//!
//! Consumers (grep-verified): `file/cursor.rs` and `file/reader.rs` only.
//! Publicly reachable as `journal_core::file::{FilterExpr, JournalFilter,
//! LogicalOp}` (the `pub use` in `file/mod.rs`) but absent from lib.rs's
//! flat re-exports. Same-shape relatives: the near twin
//! src/crates/jf/journal_file/src/filter.rs (stateless `lookup` instead of
//! this file's stepping API) and this crate's index_filter.rs (bitmap-based
//! `JournalFilter` analog).
use super::mmap::MemoryMap;
use crate::error::{JournalError, Result};
use crate::file::{file::JournalFile, offset_array::InlinedCursor};
use std::num::NonZeroU64;

/// A stateful filter over one journal file's entries: `next`/`previous`
/// return entry-object offsets of entries carrying the matched payloads.
///
/// Every arm except `None` carries scan state between calls.
/// `next`/`previous` continue from it and clamp to the needle - the first
/// chain position at/after (forward) or at/before (backward) the needle,
/// which may be the position the scan already holds. A caller resolving a
/// location from scratch must rewind with `head`/`tail` first, or the step
/// re-reports the current entry; the filtered resolution path that lives by
/// this contract is `JournalCursor::resolve_filter_location`.
#[derive(Clone, Debug)]
pub enum FilterExpr {
    /// Matches nothing. Produced when a matched payload is absent from the
    /// file or its data object has no entry chain; `next`/`previous` always
    /// return `Ok(None)`, so a filter built to `None` ends every iteration.
    None,
    /// One payload's data object plus the scan cursor over that object's
    /// entry chain: the state carried between `next`/`previous` calls.
    /// Built by `JournalFilter::convert_current_matches`.
    Match(NonZeroU64, InlinedCursor),
    /// AND of sub-expressions: one per distinct matched field name within a
    /// group, or per group joined by a repeated AND
    /// (`JournalFilter::set_operation`).
    Conjunction(Vec<FilterExpr>),
    /// OR of sub-expressions: the value set of one matched field name, or
    /// groups joined by a repeated OR (`JournalFilter::set_operation`).
    Disjunction(Vec<FilterExpr>),
}

impl FilterExpr {
    // Dead prototype, kept verbatim: the stateless seek variant of the
    // stepping API below, active in the near twin
    // src/crates/jf/journal_file/src/filter.rs. It does not compile as-is
    // here: it names `Direction` (not imported by this file) and
    // dereferences the old `Match(u64, _)` payload, not this enum's
    // `Match(NonZeroU64, InlinedCursor)`.
    // pub fn lookup<M: MemoryMap>(
    //     &self,
    //     journal_file: &JournalFile<M>,
    //     needle_offset: u64,
    //     direction: Direction,
    // ) -> Result<Option<u64>> {
    //     let Some(needle_offset) = NonZeroU64::new(needle_offset) else {
    //         return Err(JournalError::InvalidOffset);
    //     };

    //     let predicate =
    //         move |entry_offset: NonZeroU64| -> Result<bool> { Ok(entry_offset < needle_offset) };

    //     match self {
    //         FilterExpr::Match(data_offset, _) => {
    //             let Some(data_offset) = NonZeroU64::new(*data_offset) else {
    //                 return Err(JournalError::InvalidOffset);
    //             };
    //             let entry_offset = journal_file.data_object_directed_partition_point(
    //                 data_offset,
    //                 predicate,
    //                 direction,
    //             )?;
    //             Ok(entry_offset.map(|x| x.get()))
    //         }
    //         FilterExpr::Conjunction(filter_exprs) => {
    //             let mut current_offset = needle_offset;

    //             loop {
    //                 let previous_offset = current_offset;

    //                 for filter_expr in filter_exprs {
    //                     if direction == Direction::Backward {
    //                         current_offset = current_offset.saturating_add(1);
    //                     }

    //                     match filter_expr.lookup(journal_file, current_offset.get(), direction)? {
    //                         Some(new_offset) => {
    //                             if new_offset == 0 {
    //                                 panic!("Wtf");
    //                             }
    //                             current_offset = NonZeroU64::new(new_offset).unwrap();
    //                         }
    //                         None => return Ok(None),
    //                     }
    //                 }

    //                 if current_offset == previous_offset {
    //                     return Ok(Some(current_offset.get()));
    //                 }
    //             }
    //         }
    //         FilterExpr::Disjunction(filter_exprs) => {
    //             let cmp = match direction {
    //                 Direction::Forward => std::cmp::min,
    //                 Direction::Backward => std::cmp::max,
    //             };

    //             filter_exprs.iter().try_fold(None, |acc, expr| {
    //                 let result = expr.lookup(journal_file, needle_offset.get(), direction)?;

    //                 Ok(match (acc, result) {
    //                     (None, Some(offset)) => Some(offset),
    //                     (Some(best), Some(offset)) => Some(cmp(best, offset)),
    //                     (acc, None) => acc,
    //                 })
    //             })
    //         }
    //         FilterExpr::None => Ok(None),
    //     }
    // }

    /// Rewinds every scan cursor to its chain head: the first entry carrying
    /// the payload. Infallible (no file access); returns `self` so a rewind
    /// and the first `next` chain.
    pub fn head(&mut self) -> &mut Self {
        match self {
            FilterExpr::None => (),
            FilterExpr::Match(_, ic) => {
                *ic = ic.head();
            }
            FilterExpr::Conjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.head();
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.head();
                }
            }
        }

        self
    }

    /// Positions every scan cursor at the last entry of its chain, ready for
    /// a backward scan. Unlike `head` this walks the entry arrays, so
    /// object/array failures propagate as `Err`. Single-entry chains have no
    /// array to park past; the cursor stays on the entry, which `previous`'s
    /// clamp resolves identically.
    pub fn tail<M: MemoryMap>(&mut self, journal_file: &JournalFile<M>) -> Result<&mut Self> {
        match self {
            FilterExpr::None => {}
            FilterExpr::Match(_, ic) => {
                *ic = ic.tail(journal_file)?;
            }
            FilterExpr::Conjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.tail(journal_file)?;
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.tail(journal_file)?;
                }
            }
        }

        Ok(self)
    }

    /// Returns the entry offset of the next matching entry at or after
    /// `needle_offset`, advancing the scan state to it.
    ///
    /// Continuation, not seeking: the scan resumes from the state left by
    /// the previous call and reports the first chain position >= the needle,
    /// which may be the position already held - returned without moving.
    /// Callers resolving a location from scratch must rewind with `head`
    /// first, or `next` re-reports the current entry; see
    /// `JournalCursor::resolve_filter_location` for the consumer side. Per arm:
    /// - `Match`: walks the payload's entry chain forward
    ///   ([`InlinedCursor::next_until`]).
    /// - `Conjunction`: iterates to a fixed point - sub-expressions advance
    ///   in order, ratcheting the needle upward, until a full pass leaves it
    ///   unchanged; the first sub-expression without a later match ends the
    ///   scan with `Ok(None)`. At convergence every cursor rests on the
    ///   resolved entry.
    /// - `Disjunction`: every branch advances to its own next match and the
    ///   smallest offset wins. Unselected branches stay parked at their
    ///   match, so their intermediate entries are consumed by the scan.
    /// - `None`: always `Ok(None)`.
    ///
    /// On `Ok(None)` the exhausted cursors rest at their chains' ends;
    /// `head` rewinds. `Err` propagates object/mmap failures raised while
    /// walking the chains.
    pub fn next<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        needle_offset: NonZeroU64,
    ) -> Result<Option<NonZeroU64>> {
        match self {
            FilterExpr::None => Ok(None),
            FilterExpr::Match(_, ic) => ic.next_until(journal_file, needle_offset),
            FilterExpr::Conjunction(filter_exprs) => {
                let mut needle_offset = needle_offset;

                loop {
                    let previous_offset = needle_offset;

                    for fe in filter_exprs.iter_mut() {
                        if let Some(new_offset) = fe.next(journal_file, needle_offset)? {
                            needle_offset = new_offset;
                        } else {
                            return Ok(None);
                        }
                    }

                    if needle_offset == previous_offset {
                        return Ok(Some(needle_offset));
                    }
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                let mut best_offset: Option<NonZeroU64> = None;

                for fe in filter_exprs.iter_mut() {
                    if let Some(fe_offset) = fe.next(journal_file, needle_offset)? {
                        best_offset = match best_offset {
                            Some(offset) => Some(fe_offset.min(offset)),
                            None => Some(fe_offset),
                        };
                    }
                }

                Ok(best_offset)
            }
        }
    }

    /// `next` in reverse: the entry offset of the previous matching entry at
    /// or before `needle_offset`, with the same continuation-and-clamp
    /// contract - an already-satisfying position is returned without moving,
    /// so fresh resolutions rewind with `tail` first. `Conjunction`
    /// sub-expressions are walked in reverse order and iterate to a fixed
    /// point; `Disjunction` advances every branch and takes the largest
    /// offset; `None` always yields `Ok(None)`. On `Ok(None)` the exhausted
    /// cursors rest at their chains' starts.
    pub fn previous<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        needle_offset: NonZeroU64,
    ) -> Result<Option<NonZeroU64>> {
        match self {
            FilterExpr::None => Ok(None),
            FilterExpr::Match(_, ic) => ic.previous_until(journal_file, needle_offset),
            FilterExpr::Conjunction(filter_exprs) => {
                let mut needle_offset = needle_offset;

                loop {
                    let previous_offset = needle_offset;

                    for fe in filter_exprs.iter_mut().rev() {
                        if let Some(new_offset) = fe.previous(journal_file, needle_offset)? {
                            needle_offset = new_offset;
                        } else {
                            return Ok(None);
                        }
                    }

                    if needle_offset == previous_offset {
                        return Ok(Some(needle_offset));
                    }
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                let mut best_offset: Option<NonZeroU64> = None;

                for fe in filter_exprs.iter_mut() {
                    if let Some(fe_offset) = fe.previous(journal_file, needle_offset)? {
                        best_offset = match best_offset {
                            Some(offset) => Some(fe_offset.max(offset)),
                            None => Some(fe_offset),
                        };
                    }
                }

                Ok(best_offset)
            }
        }
    }
}

/// The operator recorded between match groups while `JournalFilter`
/// accumulates; see `JournalFilter::set_operation`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogicalOp {
    Conjunction,
    Disjunction,
}

/// Accumulates `KEY=VALUE` matches into a [`FilterExpr`], the pending stage
/// between `JournalReader::add_match`/`add_conjunction`/`add_disjunction`
/// and filter installation (built per step by `JournalReader::step`, which
/// installs it on the cursor).
///
/// Grouping mirrors systemd-journal's match semantics: matches accumulate
/// in groups; within a group the values of a shared field name OR together
/// and distinct field names AND together. Each group-splitting call also
/// records the operator joining the next group, `Conjunction` initially - so
/// `A=1; add_disjunction; B=2; build` yields `A=1 OR B=2`.
#[derive(Debug)]
pub struct JournalFilter {
    /// The expression accumulated so far, or `None` before the first
    /// group-closing call that had matches to flush.
    filter_expr: Option<FilterExpr>,
    /// Pending `KEY=VALUE` payloads of the open group, sorted by field name
    /// (see `add_match`).
    current_matches: Vec<Vec<u8>>,
    /// The operator the next flushed group will be attached with; set by the
    /// previous group-closing call, `Conjunction` until then.
    current_op: LogicalOp,
}

impl Default for JournalFilter {
    fn default() -> Self {
        Self {
            filter_expr: None,
            current_matches: Vec::new(),
            current_op: LogicalOp::Conjunction,
        }
    }
}

impl JournalFilter {
    /// The bytes before the first `=` of a `KEY=VALUE` pair; `None` without
    /// an `=`. Same rule as `field_map::extract_field_name`; `add_match`
    /// sorts on this key.
    fn extract_key(kv_pair: &[u8]) -> Option<&[u8]> {
        if let Some(equal_pos) = kv_pair.iter().position(|&b| b == b'=') {
            Some(&kv_pair[..equal_pos])
        } else {
            None
        }
    }

    /// Converts the pending matches of the open group into one expression
    /// and clears them.
    ///
    /// The key-sorted list is walked in runs of equal field names: a
    /// single-value run becomes a [`FilterExpr::Match`], a multi-value run a
    /// [`FilterExpr::Disjunction`] over the value set, and the per-key
    /// results combine in a [`FilterExpr::Conjunction`] (a lone result is
    /// returned unwrapped).
    ///
    /// Every payload is resolved against `journal_file` here: hash it
    /// ([`JournalFile::hash`] reads the header's keyed-hash flag), look
    /// it up in the data hash table ([`JournalFile::find_data_offset`]), and
    /// take the matched data object's entry-chain cursor
    /// ([`DataObject::inlined_cursor`](crate::file::DataObject::inlined_cursor)).
    /// A payload the
    /// file does not contain, or a data object without an entry chain
    /// (missing first entry, count or entry-array offset), becomes
    /// [`FilterExpr::None`] - it matches nothing instead of failing the
    /// build. Resolution errors (missing hash table, object layer)
    /// propagate. The 0-element panic below is unreachable: the empty
    /// early-return covers it.
    fn convert_current_matches<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
    ) -> Result<Option<FilterExpr>> {
        if self.current_matches.is_empty() {
            return Ok(None);
        }

        let mut elements = Vec::new();
        let mut i = 0;

        while i < self.current_matches.len() {
            let current_key = Self::extract_key(&self.current_matches[i]).unwrap_or(&[]);
            let start = i;

            // Extend the run of matches sharing this field name.
            while i < self.current_matches.len()
                && Self::extract_key(&self.current_matches[i]).unwrap_or(&[]) == current_key
            {
                i += 1;
            }

            if i - start > 1 {
                let mut matches = Vec::with_capacity(i - start);
                for idx in start..i {
                    let data = self.current_matches[idx].as_slice();
                    let hash = journal_file.hash(data);

                    let match_expr = match journal_file.find_data_offset(hash, data)? {
                        Some(offset) => match journal_file.data_ref(offset)?.inlined_cursor() {
                            Some(ic) => FilterExpr::Match(offset, ic),
                            None => FilterExpr::None,
                        },
                        None => FilterExpr::None,
                    };
                    matches.push(match_expr);
                }
                elements.push(FilterExpr::Disjunction(matches));
            } else {
                let data = self.current_matches[start].as_slice();
                let hash = journal_file.hash(data);

                let match_expr = match journal_file.find_data_offset(hash, data)? {
                    Some(offset) => match journal_file.data_ref(offset)?.inlined_cursor() {
                        Some(ic) => FilterExpr::Match(offset, ic),
                        None => FilterExpr::None,
                    },
                    None => FilterExpr::None,
                };
                elements.push(match_expr);
            }
        }

        self.current_matches.clear();

        match elements.len() {
            0 => panic!("Could not create filter elements from current matches"),
            1 => Ok(Some(elements.remove(0))),
            _ => Ok(Some(FilterExpr::Conjunction(elements))),
        }
    }

    /// Adds one `KEY=VALUE` payload to the open match group. Pairs without
    /// `=` are ignored silently.
    ///
    /// The pair is inserted key-sorted: the binary search compares key bytes
    /// only, so equal-key values stay unordered but adjacent. Identical
    /// payloads are accepted again, producing duplicate arms in the same
    /// value set. The ordering is what lets `convert_current_matches` group
    /// same-key values into one disjunction.
    pub fn add_match(&mut self, kv_pair: &[u8]) {
        if kv_pair.contains(&b'=') {
            let new_item = kv_pair.to_vec();
            let new_key = Self::extract_key(&new_item).unwrap_or(&[]);

            // Key-sorted insert position; the comparison never looks at the
            // value bytes.
            let pos = self
                .current_matches
                .binary_search_by(|item| {
                    let key = Self::extract_key(item).unwrap_or(&[]);
                    key.cmp(new_key)
                })
                .unwrap_or_else(|e| e);

            self.current_matches.insert(pos, new_item);
        }
    }

    /// Flushes the open match group and records `op` for the next one.
    ///
    /// The pending matches are converted and attached to the accumulated
    /// expression with the PREVIOUSLY recorded operator (`Conjunction` when
    /// nothing is recorded yet); `op` then becomes the operator for the next
    /// flush. A group joined by the same operator is pushed into the
    /// existing vec; a different operator wraps the accumulated expression
    /// in a fresh vec. Flattening is sound only because `filter_expr` is
    /// built exclusively here, so every element of a vec was joined by that
    /// vec's operator.
    ///
    /// With no pending matches the call only records `op`. `Err` propagates
    /// from match resolution.
    pub fn set_operation<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        op: LogicalOp,
    ) -> Result<()> {
        let new_expr = self.convert_current_matches(journal_file)?;
        if new_expr.is_none() {
            self.current_op = op;
            return Ok(());
        }

        if self.filter_expr.is_none() {
            self.filter_expr = new_expr;
            self.current_op = op;
            return Ok(());
        }

        let new_expr = new_expr.unwrap();
        let current_expr = self.filter_expr.take().unwrap();

        self.filter_expr = Some(match (current_expr, self.current_op) {
            (FilterExpr::Disjunction(mut exprs), LogicalOp::Disjunction) => {
                exprs.push(new_expr);
                FilterExpr::Disjunction(exprs)
            }
            (FilterExpr::Conjunction(mut exprs), LogicalOp::Conjunction) => {
                exprs.push(new_expr);
                FilterExpr::Conjunction(exprs)
            }
            (current_expr, LogicalOp::Disjunction) => {
                FilterExpr::Disjunction(vec![current_expr, new_expr])
            }
            (current_expr, LogicalOp::Conjunction) => {
                FilterExpr::Conjunction(vec![current_expr, new_expr])
            }
        });

        self.current_op = op;
        Ok(())
    }

    /// Closes the final match group and returns the accumulated expression.
    ///
    /// The flush uses the recorded operator; afterwards the filter is reset
    /// (matches cleared, operator back to `Conjunction`), so it can
    /// accumulate a fresh run. `Err(MalformedFilter)` when nothing was ever
    /// added: no pending matches and no accumulated expression.
    pub fn build<M: MemoryMap>(&mut self, journal_file: &JournalFile<M>) -> Result<FilterExpr> {
        self.set_operation(journal_file, self.current_op)?;

        self.current_matches.clear();
        self.current_op = LogicalOp::Conjunction;
        self.filter_expr.take().ok_or(JournalError::MalformedFilter)
    }
}
