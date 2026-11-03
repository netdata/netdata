//! Bitmap-based filter over a prebuilt file index. [`IndexFilter`]
//! resolves caller-supplied `FIELD=VALUE` matches to roaring bitmaps of
//! entry indices and accumulates them into one [`IndexFilterExpr`], which
//! combines the bitmaps with set operations - the eager bitmap analog of
//! file/filter.rs's cursor-based `JournalFilter`/`FilterExpr` (same
//! builder shape and `LogicalOp`; pre-resolved bitmaps instead of lazily
//! stepped entry chains; infallible instead of `Result`).
//!
//! Verified status: this file is an uncompiled orphan. No `mod
//! index_filter` declaration exists anywhere (file/mod.rs declares
//! cursor..writer without it), and the file could not compile even if
//! declared: `super::index` and `crate::index` resolve to nothing in
//! journal-core (lib.rs declares only error, collections, file,
//! field_map, repository), `roaring` is not a dependency of this crate
//! (Cargo.toml), and the tests' own imports fail too (`crate::index`
//! unresolved, `FxHashMap` unimported). Grep across src/crates finds
//! `IndexFilterExpr`/`IndexFilter` referenced only in this file.
//!
//! The index shape it targets matches nothing shipped: no crate defines
//! a `FileIndex` with an `entries_index` field keyed by raw
//! `"FIELD=VALUE"` strings plus a `file_histogram.get_entry_range()`.
//! The live bitmap-filter line is src/crates/journal-index: its
//! `FileIndex` (file_index.rs) keys bitmaps by typed `FieldValuePair`
//! under a `Histogram`, and its `Filter` (filter.rs) is the consumed
//! evaluator.
//!
//! Implied contract: bitmap bits are entry indices - positions in one
//! index's entry list, not file offsets - so a bitmap is only meaningful
//! against the index it was built from; only
//! `matching_indices_in_bucket` couples an expression back to an index.
//! A pair the index lacks resolves to [`IndexFilterExpr::None`], which
//! matches nothing instead of failing the build.
use super::index::FileIndex;
use roaring::RoaringBitmap;

/// A filter expression over a prebuilt index's entry-index bitmaps,
/// evaluated by [`IndexFilterExpr::matching_indices`] to the set of
/// matching indices.
#[derive(Clone, Debug)]
pub enum IndexFilterExpr {
    /// Matches nothing. Produced for a matched pair the index does not
    /// contain; evaluates to an empty bitmap.
    None,
    /// One pair's entry-index bitmap, taken as-is at build time and
    /// cloned on evaluation.
    Match(RoaringBitmap),
    /// Intersection of sub-expressions: the AND across matched field
    /// names. An empty vec yields an empty bitmap.
    Conjunction(Vec<IndexFilterExpr>),
    /// Union of sub-expressions: the OR across a field's value set. An
    /// empty vec yields an empty bitmap.
    Disjunction(Vec<IndexFilterExpr>),
}

impl IndexFilterExpr {
    /// The set of entry indices matching this expression: the `Match`
    /// bitmap, the intersection across a conjunction, or the union across
    /// a disjunction; `None` yields an empty set.
    ///
    /// Fully materialized on every call, with a conjunction early-exiting
    /// once its intersection is empty. Independent of any `FileIndex`:
    /// the bitmaps were resolved at build time.
    pub fn matching_indices(&self) -> RoaringBitmap {
        match self {
            IndexFilterExpr::None => RoaringBitmap::new(),
            IndexFilterExpr::Match(bitmap) => bitmap.clone(),
            IndexFilterExpr::Conjunction(filter_exprs) => {
                if filter_exprs.is_empty() {
                    return RoaringBitmap::new();
                }

                let mut result = filter_exprs[0].matching_indices();
                for expr in filter_exprs.iter().skip(1) {
                    result &= expr.matching_indices();
                    if result.is_empty() {
                        break;
                    }
                }
                result
            }
            IndexFilterExpr::Disjunction(filter_exprs) => {
                let mut result = RoaringBitmap::new();
                for expr in filter_exprs.iter() {
                    result |= expr.matching_indices();
                }
                result
            }
        }
    }

    /// Number of matching entry indices. Evaluates the full set; there
    /// is no cheaper path than `matching_indices().len()`.
    pub fn count(&self) -> u64 {
        self.matching_indices().len()
    }

    /// Whether the expression has matches, without intersecting
    /// anything. Exact for `None`/`Match`/`Disjunction`; for a
    /// `Conjunction` only necessary, not sufficient - every arm must be
    /// non-empty, but the arms may still intersect to nothing (a
    /// conjunction of the two `_SYSTEMD_UNIT` values used in the tests
    /// below would report true over an empty intersection).
    pub fn has_matches(&self) -> bool {
        match self {
            IndexFilterExpr::None => false,
            IndexFilterExpr::Match(bitmap) => !bitmap.is_empty(),
            IndexFilterExpr::Conjunction(filter_exprs) => {
                filter_exprs.iter().all(|expr| expr.has_matches())
            }
            IndexFilterExpr::Disjunction(filter_exprs) => {
                filter_exprs.iter().any(|expr| expr.has_matches())
            }
        }
    }

    /// Matching entry indices restricted to the inclusive index range
    /// `start..=end`: the full set materialized first, then indices
    /// below `start` and above `end` removed. `end + 1` overflows at
    /// `end == u32::MAX` (panics in debug; wraps and empties the result
    /// in release).
    pub fn matching_indices_in_range(&self, start: u32, end: u32) -> RoaringBitmap {
        let mut result = self.matching_indices();
        result.remove_range(..start);
        result.remove_range((end + 1)..);
        result
    }

    /// Matching entry indices inside one histogram bucket: the bucket's
    /// entry-index range from `file_index.file_histogram`, then
    /// `matching_indices_in_range` on it. `None` when the bucket holds
    /// no entries.
    pub fn matching_indices_in_bucket(
        &self,
        file_index: &FileIndex,
        bucket_index: usize,
    ) -> Option<RoaringBitmap> {
        let (start, end) = file_index.file_histogram.get_entry_range(bucket_index)?;
        Some(self.matching_indices_in_range(start, end))
    }
}

/// The operator recorded between match groups while `IndexFilter`
/// accumulates; see `IndexFilter::set_operation`. Identical to the
/// `LogicalOp` of file/filter.rs.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogicalOp {
    Conjunction,
    Disjunction,
}

/// Accumulates `FIELD=VALUE` matches into an [`IndexFilterExpr`],
/// resolving each pair against a prebuilt `FileIndex` when its group
/// is closed.
///
/// Grouping mirrors systemd-journal match semantics, as in file/filter.rs's
/// `JournalFilter`: matches accumulate in groups; within a group the
/// values of a shared field name OR together and distinct field names AND
/// together. Each group-closing call also records the operator joining
/// the NEXT group, `Conjunction` until then - so `A=1; add_disjunction;
/// B=2; build` yields `A=1 OR B=2`.
#[derive(Debug)]
pub struct IndexFilter {
    /// The expression accumulated so far, or `None` before the first
    /// group-closing call that had matches to flush.
    filter_expr: Option<IndexFilterExpr>,
    /// Pending `FIELD=VALUE` strings of the open group, kept sorted by
    /// field name (see `add_match`).
    current_matches: Vec<String>,
    /// The operator the next flushed group will be attached with; set by
    /// the previous group-closing call.
    current_op: LogicalOp,
}

impl Default for IndexFilter {
    fn default() -> Self {
        Self {
            filter_expr: None,
            current_matches: Vec::new(),
            current_op: LogicalOp::Conjunction,
        }
    }
}

impl IndexFilter {
    /// An empty filter: no accumulated expression, no pending matches.
    pub fn new() -> Self {
        Self::default()
    }

    /// The field name of a `FIELD=VALUE` pair: everything before the
    /// first `=`. A string without `=` yields the whole string (`split`
    /// always yields a first part, so this never returns `None`);
    /// `add_match` filters those out anyway. The sort and grouping key
    /// for the pending matches.
    fn extract_key(field_value: &str) -> Option<&str> {
        field_value.split('=').next()
    }

    /// Converts the pending matches of the open group into one expression
    /// and clears them.
    ///
    /// The key-sorted list is walked in runs of equal field names: a
    /// single-value run becomes one [`IndexFilterExpr::Match`], a
    /// multi-value run a [`IndexFilterExpr::Disjunction`] over the value
    /// set, and the per-key results combine in an
    /// [`IndexFilterExpr::Conjunction`].
    ///
    /// Every pair is resolved here by lookup in the index's
    /// `entries_index`; a pair the index does not contain becomes
    /// [`IndexFilterExpr::None`] - it matches nothing instead of failing.
    /// Infallible, unlike the file/filter.rs twin, which propagates
    /// resolution errors. Returns `None` with no pending matches; the
    /// 0-element arm below is unreachable, since each pending pair
    /// contributes exactly one element.
    fn convert_current_matches(&mut self, file_index: &FileIndex) -> Option<IndexFilterExpr> {
        if self.current_matches.is_empty() {
            return None;
        }

        let mut elements = Vec::new();
        let mut i = 0;

        // Sort by field name for the grouping walk; add_match already
        // inserts key-sorted, so this is normally a no-op.
        self.current_matches.sort_by(|a, b| {
            let key_a = Self::extract_key(a).unwrap_or("");
            let key_b = Self::extract_key(b).unwrap_or("");
            key_a.cmp(key_b)
        });

        while i < self.current_matches.len() {
            let current_key = Self::extract_key(&self.current_matches[i]).unwrap_or("");
            let start = i;

            // Extend the run of pending matches sharing this field name.
            while i < self.current_matches.len()
                && Self::extract_key(&self.current_matches[i]).unwrap_or("") == current_key
            {
                i += 1;
            }

            // Multiple values for one field name OR together; a single
            // value stays a plain Match.
            if i - start > 1 {
                let mut matches = Vec::with_capacity(i - start);
                for idx in start..i {
                    let field_value = &self.current_matches[idx];
                    if let Some(bitmap) = file_index.entries_index.get(field_value) {
                        matches.push(IndexFilterExpr::Match(bitmap.clone()));
                    } else {
                        matches.push(IndexFilterExpr::None);
                    }
                }
                elements.push(IndexFilterExpr::Disjunction(matches));
            } else {
                let field_value = &self.current_matches[start];
                if let Some(bitmap) = file_index.entries_index.get(field_value) {
                    elements.push(IndexFilterExpr::Match(bitmap.clone()));
                } else {
                    elements.push(IndexFilterExpr::None);
                }
            }
        }

        self.current_matches.clear();

        match elements.len() {
            0 => None,
            1 => Some(elements.into_iter().next().unwrap()),
            _ => Some(IndexFilterExpr::Conjunction(elements)),
        }
    }

    /// Adds one `FIELD=VALUE` pair to the open match group. Strings
    /// without `=` are ignored silently.
    ///
    /// The pair is inserted key-sorted: the binary search compares field
    /// names only, so equal-key values stay unordered but adjacent, and
    /// duplicates are accepted as-is. The ordering is what lets
    /// `convert_current_matches` group same-key values into one
    /// disjunction.
    ///
    /// # Examples
    /// ```
    /// filter.add_match("_SYSTEMD_UNIT=ssh.service");
    /// filter.add_match("PRIORITY=6");
    /// ```
    pub fn add_match(&mut self, field_value: &str) {
        if field_value.contains('=') {
            let key = Self::extract_key(field_value).unwrap_or("");
            let pos = self
                .current_matches
                .binary_search_by(|item| {
                    let item_key = Self::extract_key(item).unwrap_or("");
                    item_key.cmp(key)
                })
                .unwrap_or_else(|e| e);

            self.current_matches.insert(pos, field_value.to_string());
        }
    }

    /// Flushes the open match group and records `op` for the next one.
    ///
    /// The pending matches are converted and attached to the accumulated
    /// expression with the PREVIOUSLY recorded operator (`Conjunction`
    /// when nothing is recorded yet); `op` then becomes the operator for
    /// the next flush. A group joined by the same operator is pushed
    /// into the existing vec; a different operator wraps the accumulated
    /// expression in a fresh vec - sound only because `filter_expr` is
    /// built exclusively here, so every vec element was joined by that
    /// vec's operator.
    ///
    /// With no pending matches the call only records `op`. Infallible:
    /// resolution failures degrade to `IndexFilterExpr::None` arms,
    /// never errors (the file/filter.rs twin returns `Result`).
    pub fn set_operation(&mut self, file_index: &FileIndex, op: LogicalOp) {
        let new_expr = self.convert_current_matches(file_index);
        if new_expr.is_none() {
            self.current_op = op;
            return;
        }

        if self.filter_expr.is_none() {
            self.filter_expr = new_expr;
            self.current_op = op;
            return;
        }

        let new_expr = new_expr.unwrap();
        let current_expr = self.filter_expr.take().unwrap();

        self.filter_expr = Some(match (current_expr, self.current_op) {
            (IndexFilterExpr::Disjunction(mut exprs), LogicalOp::Disjunction) => {
                exprs.push(new_expr);
                IndexFilterExpr::Disjunction(exprs)
            }
            (IndexFilterExpr::Conjunction(mut exprs), LogicalOp::Conjunction) => {
                exprs.push(new_expr);
                IndexFilterExpr::Conjunction(exprs)
            }
            (current_expr, LogicalOp::Disjunction) => {
                IndexFilterExpr::Disjunction(vec![current_expr, new_expr])
            }
            (current_expr, LogicalOp::Conjunction) => {
                IndexFilterExpr::Conjunction(vec![current_expr, new_expr])
            }
        });

        self.current_op = op;
    }

    /// Shorthand for `set_operation(&file_index, LogicalOp::Conjunction)`:
    /// closes the pending group, AND-ing it into the filter.
    pub fn add_conjunction(&mut self, file_index: &FileIndex) {
        self.set_operation(file_index, LogicalOp::Conjunction);
    }

    /// Shorthand for `set_operation(&file_index, LogicalOp::Disjunction)`:
    /// closes the pending group, OR-ing it into the filter.
    pub fn add_disjunction(&mut self, file_index: &FileIndex) {
        self.set_operation(file_index, LogicalOp::Disjunction);
    }

    /// Closes the final match group and returns the accumulated
    /// expression.
    ///
    /// The flush attaches pending matches with the recorded operator;
    /// afterwards the filter is reset (matches cleared, operator back to
    /// `Conjunction`), so it can accumulate a fresh run. Returns
    /// [`IndexFilterExpr::None`] when nothing was ever added - unlike the
    /// file/filter.rs twin, which fails with `MalformedFilter`.
    pub fn build(&mut self, file_index: &FileIndex) -> IndexFilterExpr {
        self.set_operation(file_index, self.current_op);

        self.current_matches.clear();
        self.current_op = LogicalOp::Conjunction;
        self.filter_expr.take().unwrap_or(IndexFilterExpr::None)
    }

    /// One pair's expression, resolved straight from the index: its
    /// [`IndexFilterExpr::Match`] bitmap, or [`IndexFilterExpr::None`]
    /// when the index does not contain the pair.
    pub fn simple_match(file_index: &FileIndex, field_value: &str) -> IndexFilterExpr {
        if let Some(bitmap) = file_index.entries_index.get(field_value) {
            IndexFilterExpr::Match(bitmap.clone())
        } else {
            IndexFilterExpr::None
        }
    }

    /// AND of several pairs, built through the same grouping as the
    /// accumulator: distinct field names AND together, repeated values
    /// of one field name OR together. Unknown pairs contribute
    /// [`IndexFilterExpr::None`] arms.
    pub fn conjunction(file_index: &FileIndex, field_values: &[&str]) -> IndexFilterExpr {
        let mut filter = IndexFilter::new();
        for field_value in field_values {
            filter.add_match(field_value);
        }
        filter.build(file_index)
    }

    /// OR of several pair lookups. An empty list yields
    /// [`IndexFilterExpr::None`]; a single pair collapses to its own
    /// arm; unknown pairs become `None` arms, so a disjunction over only
    /// unknown pairs matches nothing.
    pub fn disjunction(file_index: &FileIndex, field_values: &[&str]) -> IndexFilterExpr {
        let matches: Vec<_> = field_values
            .iter()
            .map(|fv| Self::simple_match(file_index, fv))
            .collect();

        if matches.is_empty() {
            IndexFilterExpr::None
        } else if matches.len() == 1 {
            matches.into_iter().next().unwrap()
        } else {
            IndexFilterExpr::Disjunction(matches)
        }
    }
}

#[cfg(test)]
mod tests {
    // Draft-shape fixture, built by struct literal: the fields below
    // (`file_histogram`, `entries_index`) exist on no shipped `FileIndex`.
    use super::*;
    use crate::index::{FileHistogram, FileIndex};

    fn create_test_file_index() -> FileIndex {
        let mut entry_indices = FxHashMap::default();

        // Fixture pairs: raw "FIELD=VALUE" strings -> entry-index bitmaps.
        entry_indices.insert(
            "_SYSTEMD_UNIT=ssh.service".to_string(),
            RoaringBitmap::from_sorted_iter([1, 3, 5, 7]).unwrap(),
        );
        entry_indices.insert(
            "_SYSTEMD_UNIT=nginx.service".to_string(),
            RoaringBitmap::from_sorted_iter([2, 4, 6, 8]).unwrap(),
        );
        entry_indices.insert(
            "PRIORITY=6".to_string(),
            RoaringBitmap::from_sorted_iter([1, 2, 9, 10]).unwrap(),
        );
        entry_indices.insert(
            "PRIORITY=3".to_string(),
            RoaringBitmap::from_sorted_iter([3, 4, 5]).unwrap(),
        );

        FileIndex {
            file_histogram: FileHistogram::default(),
            entries_index: entry_indices,
        }
    }

    #[test]
    fn test_simple_match() {
        let file_index = create_test_file_index();
        let filter = IndexFilter::simple_match(&file_index, "_SYSTEMD_UNIT=ssh.service");

        let matches = filter.matching_indices();
        assert_eq!(matches.iter().collect::<Vec<_>>(), vec![1, 3, 5, 7]);
    }

    #[test]
    fn test_conjunction() {
        let file_index = create_test_file_index();
        let filter =
            IndexFilter::conjunction(&file_index, &["_SYSTEMD_UNIT=ssh.service", "PRIORITY=6"]);

        let matches = filter.matching_indices();
        assert_eq!(matches.iter().collect::<Vec<_>>(), vec![1]); // Only entry 1 matches both
    }

    #[test]
    fn test_disjunction() {
        let file_index = create_test_file_index();
        let filter = IndexFilter::disjunction(
            &file_index,
            &["_SYSTEMD_UNIT=ssh.service", "_SYSTEMD_UNIT=nginx.service"],
        );

        let matches = filter.matching_indices();
        assert_eq!(
            matches.iter().collect::<Vec<_>>(),
            vec![1, 2, 3, 4, 5, 6, 7, 8]
        );
    }

    #[test]
    fn test_complex_filter() {
        let file_index = create_test_file_index();
        let mut filter = IndexFilter::new();

        // Two values of one field name: they OR together inside the group.
        filter.add_match("_SYSTEMD_UNIT=ssh.service");
        filter.add_match("_SYSTEMD_UNIT=nginx.service");
        filter.add_conjunction(&file_index);

        // Next group, attached with the recorded Conjunction.
        filter.add_match("PRIORITY=6");

        let result = filter.build(&file_index);
        let matches = result.matching_indices();
        assert_eq!(matches.iter().collect::<Vec<_>>(), vec![1, 2]); // Entries that match PRIORITY=6 AND (ssh OR nginx)
    }
}
