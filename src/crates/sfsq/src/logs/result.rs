//! Neutral query result from the log engine.
//!
//! [`LogsData`] is what [`run`](super::run) produces: the merged counts,
//! facets, histogram, materialized page, and column schema — all in
//! engine / `sfst` terms, *before* any wire shaping. A consumer turns it
//! into whatever its frontend expects (a JSON envelope, a CLI table, …).

use std::collections::BTreeSet;

use super::cursor::Cursor;

/// The result of a multi-file log query.
///
/// Counts and facets describe exactly the logs matching the filter
/// within the query window; the histogram is bucketed on the query's
/// grid; the page is materialized newest-first.
pub struct LogsData {
    /// Filter-matching logs within the window, summed across files. A
    /// bitmap-cardinality count (`u64`, like the histogram buckets);
    /// callers narrow to a UI integer at their wire boundary.
    pub matched: u64,
    /// One entry per requested facet field, values summed across files
    /// (FST iteration order: lexicographic by value).
    pub facets: Vec<sfst::FacetResult>,
    /// The dimension field the histogram was bucketed by.
    pub histogram_field: String,
    /// The merged per-bucket histogram on the query's grid. Logs that
    /// match the filter but lack `histogram_field` land in its `unset`.
    pub histogram: sfst::Timeline,
    /// Low/mid-cardinality fields across all candidate files — the
    /// fields usable both as histogram dimensions and as facets (any
    /// field that is high-cardinality in *any* file is excluded).
    pub available_fields: sfst::FieldTable,
    /// The row-table column schema: the union of every candidate file's
    /// field names, all tiers (so high-card attributes still get a
    /// column), sorted.
    pub columns: Vec<String>,
    /// The materialized page, newest-first, each row tagged with its
    /// [`Cursor`] in the global total order.
    pub rows: Vec<(Cursor, sfst::MaterializedRow)>,
    /// A newer row exists beyond the page (consumer "scroll up").
    pub has_newer: bool,
    /// An older row exists beyond the page (consumer "scroll down").
    pub has_older: bool,
}

impl LogsData {
    /// The set of facetable field names — the [`available_fields`]
    /// (low/mid-card) by name. A convenience for consumers that tag
    /// table columns as filterable.
    ///
    /// [`available_fields`]: LogsData::available_fields
    pub fn facetable(&self) -> BTreeSet<&str> {
        self.available_fields.names().collect()
    }

    /// The empty result for `grid` — what [`run`](super::run) produces for
    /// an empty source set: zero counts, no facets or fields, and a
    /// grid-aligned all-zero histogram ([`sfst::Timeline::empty`]). For
    /// consumers that must emit a well-formed envelope without running a
    /// query (a cancelled or failed call): the chart contract still gets
    /// its full grid of zero counts rather than a shapeless blank.
    pub fn empty(histogram_field: impl Into<String>, grid: sfst::Grid) -> Self {
        Self {
            matched: 0,
            facets: Vec::new(),
            histogram_field: histogram_field.into(),
            histogram: sfst::Timeline::empty(grid),
            available_fields: sfst::FieldTable::default(),
            columns: Vec::new(),
            rows: Vec::new(),
            has_newer: false,
            has_older: false,
        }
    }
}
