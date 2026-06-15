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
}
