//! Neutral query input for the log engine.
//!
//! [`LogsQuery`] is what [`run`](super::run) consumes: a plain description
//! of *what to match, how to bucket, and which page to return* — no
//! transport or wire concerns. A consumer parses its own request format
//! (HTTP params, CLI flags, a UI payload) and assembles a [`LogsQuery`]
//! with [`LogsQueryBuilder`]; the engine never sees the wire shape.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};
use sfst::Filter;

use super::cursor::Cursor;

/// Default histogram dimension when the query doesn't specify one.
/// `severity_text` — the OTel canonical log-level field; what makes a
/// meaningful chart is the producer's responsibility, and the consumer
/// exposes `available_fields` for users to pick another.
const DEFAULT_HISTOGRAM_FIELD: &str = "severity_text";

/// Default facet field when the query doesn't specify any. Same
/// `severity_text` rationale as the histogram default: a first-load
/// request typically carries no facets and the engine can't infer the
/// user's intent (cardinality composes unpredictably across files), so it
/// surfaces just this one. Users add more via `facet_fields`.
const DEFAULT_FACET_FIELD: &str = "severity_text";

/// A multi-file log query in engine terms — fully resolved and ready to
/// run. Built with [`LogsQueryBuilder`], which applies the engine's
/// defaults (histogram / facet field) and converts the filter selections
/// once; the fields are read-only afterwards (via the accessor methods).
///
/// The caller supplies the histogram [`grid`](Self::grid) outright: the
/// engine buckets the histogram on exactly that grid and clips every count
/// (matched, facets, the page) to its span. Deciding the bucket geometry —
/// window bounds, bucket width, count, alignment — is a presentation choice
/// the consumer owns; the engine does not second-guess it.
#[derive(Debug, Clone)]
pub struct LogsQuery {
    pub(super) grid: sfst::Grid,
    pub(super) filter: Filter,
    pub(super) query: Option<String>,
    pub(super) histogram_field: String,
    pub(super) facet_fields: Vec<String>,
    pub(super) anchor: Option<Anchor>,
    pub(super) direction: Direction,
    pub(super) limit: usize,
}

impl LogsQuery {
    /// Histogram bucket geometry; its span is the query window.
    pub fn grid(&self) -> sfst::Grid {
        self.grid
    }
    /// The match filter (OR within a field, AND across fields).
    pub fn filter(&self) -> &Filter {
        &self.filter
    }
    /// Field-less full-text query — an unanchored regex matched against whole
    /// `key=value` pairs — or `None`. Resolved by the engine as a global AND
    /// constraint (it narrows matched / facets / histogram and the page
    /// alike).
    pub fn query(&self) -> Option<&str> {
        self.query.as_deref()
    }
    /// Histogram dimension field — always resolved (the default if the
    /// builder wasn't given one).
    pub fn histogram_field(&self) -> &str {
        &self.histogram_field
    }
    /// Facet fields to tabulate — always non-empty (the default if the
    /// builder wasn't given any).
    pub fn facet_fields(&self) -> &[String] {
        &self.facet_fields
    }
    /// Pagination anchor, or `None` to start at the edge.
    pub fn anchor(&self) -> Option<Anchor> {
        self.anchor
    }
    pub fn direction(&self) -> Direction {
        self.direction
    }
    pub fn limit(&self) -> usize {
        self.limit
    }
}

/// Builder for [`LogsQuery`]. Start from the histogram [`grid`](LogsQuery::grid)
/// (the one required input); every other field defaults — empty filter
/// (matches everything), the engine's default histogram and facet field, no
/// anchor, [`Direction::Backward`], and a zero `limit`.
/// [`build`](Self::build) resolves the defaults and converts the selections.
#[derive(Debug, Clone)]
pub struct LogsQueryBuilder {
    grid: sfst::Grid,
    filter: Filter,
    query: Option<String>,
    histogram_field: Option<String>,
    facet_fields: Vec<String>,
    anchor: Option<Anchor>,
    direction: Direction,
    limit: usize,
}

impl LogsQueryBuilder {
    /// Start a query for the given histogram grid.
    pub fn new(grid: sfst::Grid) -> Self {
        Self {
            grid,
            filter: Filter::new(),
            query: None,
            histogram_field: None,
            facet_fields: Vec::new(),
            anchor: None,
            direction: Direction::default(),
            limit: 0,
        }
    }

    /// Set the match filter from a `field -> values` selection map (OR
    /// within a field, AND across fields). Fields with an empty value list
    /// are dropped — see [`Filter::from`]. Exact matchers only; for regex
    /// patterns build a [`Filter`] and use [`filter`](Self::filter).
    ///
    /// Mutually exclusive with [`filter`](Self::filter) — both assign the
    /// filter, so the last call wins.
    pub fn selections(mut self, selections: HashMap<String, Vec<String>>) -> Self {
        self.filter = Filter::from(&selections);
        self
    }

    /// Set the match filter directly — the general form, supporting exact
    /// values and regex patterns ([`Filter::select_pattern`]). Mutually
    /// exclusive with [`selections`](Self::selections); the last call wins.
    pub fn filter(mut self, filter: Filter) -> Self {
        self.filter = filter;
        self
    }

    /// Set a field-less full-text query — an unanchored regex matched against
    /// whole `key=value` pairs (a "contains" search; the key part can scope
    /// it to a subset of fields). Resolved by the engine as a global AND
    /// term, so it narrows the aggregate counts as well as the page.
    pub fn query(mut self, query: impl Into<String>) -> Self {
        self.query = Some(query.into());
        self
    }

    pub fn histogram_field(mut self, field: impl Into<String>) -> Self {
        self.histogram_field = Some(field.into());
        self
    }

    pub fn facet_fields(mut self, fields: Vec<String>) -> Self {
        self.facet_fields = fields;
        self
    }

    pub fn anchor(mut self, anchor: Anchor) -> Self {
        self.anchor = Some(anchor);
        self
    }

    pub fn direction(mut self, direction: Direction) -> Self {
        self.direction = direction;
        self
    }

    pub fn limit(mut self, limit: usize) -> Self {
        self.limit = limit;
        self
    }

    /// Resolve defaults and produce the query: an unset histogram field
    /// becomes `severity_text`, and an empty facet list becomes that same
    /// single default facet.
    pub fn build(self) -> LogsQuery {
        let histogram_field = self
            .histogram_field
            .unwrap_or_else(|| DEFAULT_HISTOGRAM_FIELD.to_string());
        let facet_fields = if self.facet_fields.is_empty() {
            vec![DEFAULT_FACET_FIELD.to_string()]
        } else {
            self.facet_fields
        };
        LogsQuery {
            grid: self.grid,
            filter: self.filter,
            query: self.query,
            histogram_field,
            facet_fields,
            anchor: self.anchor,
            direction: self.direction,
            limit: self.limit,
        }
    }
}

/// A pagination anchor in engine terms — already parsed from whatever
/// form the consumer's wire protocol used.
#[derive(Debug, Clone, Copy)]
pub enum Anchor {
    /// A specific boundary row, from a prior page's [`Cursor`].
    Cursor(Cursor),
    /// A point in time (nanoseconds since epoch). Resolves to "the
    /// newest row at or before this instant".
    Timestamp(i64),
}

impl Anchor {
    /// Resolve to a [`Cursor`] in the global total order. A row cursor is
    /// used as-is; a timestamp resolves to the largest cursor at that
    /// instant (see [`Cursor::synthetic_max`]), so a backward page shows the
    /// newest rows up to that time.
    pub(super) fn to_cursor(self) -> Cursor {
        match self {
            Anchor::Cursor(c) => c,
            Anchor::Timestamp(ns) => Cursor::synthetic_max(ns),
        }
    }
}

/// Page direction relative to the anchor. `Backward` walks toward older
/// rows (the default), `Forward` toward newer ones.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Direction {
    Forward,
    #[default]
    Backward,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn grid() -> sfst::Grid {
        sfst::Grid::new(0, 1_000_000_000, 1)
    }

    #[test]
    fn build_resolves_histogram_and_facet_defaults() {
        // No histogram field / no facets → the engine's `severity_text`
        // defaults, applied once here rather than lazily downstream.
        let q = LogsQueryBuilder::new(grid()).build();
        assert_eq!(q.histogram_field(), "severity_text");
        assert_eq!(q.facet_fields().to_vec(), vec!["severity_text".to_string()]);
    }

    #[test]
    fn build_keeps_explicit_histogram_and_facets() {
        let q = LogsQueryBuilder::new(grid())
            .histogram_field("service.name")
            .facet_fields(vec!["level".to_string(), "host".to_string()])
            .build();
        assert_eq!(q.histogram_field(), "service.name");
        assert_eq!(
            q.facet_fields().to_vec(),
            vec!["level".to_string(), "host".to_string()]
        );
    }

    #[test]
    fn build_converts_selections_to_filter() {
        let mut selections = HashMap::new();
        selections.insert("level".to_string(), vec!["error".to_string()]);
        let q = LogsQueryBuilder::new(grid()).selections(selections).build();
        assert!(q.filter().has_field("level"));

        // No selections → an empty filter (matches everything).
        assert!(LogsQueryBuilder::new(grid()).build().filter().is_empty());
    }

    #[test]
    fn filter_carries_patterns_and_overrides_selections() {
        // `filter()` carries a regex-pattern filter the `selections()`
        // convenience can't express.
        let q = LogsQueryBuilder::new(grid())
            .filter(Filter::new().select_pattern("trace", "abc.*"))
            .build();
        assert!(q.filter().has_field("trace"));

        // `filter()` and `selections()` both assign the filter — last wins.
        let mut selections = HashMap::new();
        selections.insert("level".to_string(), vec!["info".to_string()]);
        let q = LogsQueryBuilder::new(grid())
            .selections(selections)
            .filter(Filter::new().select_pattern("trace", "x.*"))
            .build();
        assert!(q.filter().has_field("trace"));
        assert!(
            !q.filter().has_field("level"),
            "filter() replaced the earlier selections()"
        );
    }
}
