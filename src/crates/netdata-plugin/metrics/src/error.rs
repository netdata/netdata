//! The crate's error: a mistake in declaring a metric set.

use crate::metric_set::{MAX_ALLOWED_LATENESS_WINDOWS, MAX_INTERVAL_SECS};
use std::time::Duration;

/// A mistake in declaring a metric set, returned by the
/// [`MetricSetBuilder`](crate::MetricSetBuilder) step that makes it.
#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
pub enum Error {
    /// The collection interval is not a whole number of seconds from 1 to
    /// `i32::MAX`.
    #[error("the collection interval must be whole seconds, from 1 to {max}: {0:?}", max = MAX_INTERVAL_SECS)]
    CollectionInterval(Duration),
    /// The allowed lateness is more than 60 collection intervals.
    #[error("the allowed lateness must be at most {max} collection intervals: {0:?}", max = MAX_ALLOWED_LATENESS_WINDOWS)]
    AllowedLateness(Duration),
    /// `expiry` is 0.
    #[error("expiry must be positive")]
    ZeroExpiry,
    /// The cardinality limit's `max_entities` is 0.
    #[error("the cardinality limit must be positive")]
    ZeroCardinalityLimit,
    /// The cardinality limit's overflow labels are empty.
    #[error("overflow labels are required")]
    NoOverflowLabels,
    /// A [`Column`](crate::Column) type has no names.
    #[error("an instrument needs at least one column")]
    NoColumns,
    /// A name appears more than once in a [`Column`](crate::Column) type's
    /// names.
    #[error("duplicate column name {0:?}")]
    DuplicateColumn(&'static str),
    /// Histogram bounds that are not finite and strictly increasing.
    #[error("histogram bounds must be finite and increasing")]
    HistogramBounds,
    /// The set has no instrument.
    #[error("metric set has no instruments")]
    NoInstruments,
    /// A column name is not a valid dimension id.
    #[error("invalid dimension id {0:?}")]
    InvalidDimensionId(String),
    /// A context breaks the rules at
    /// [`ChartDescriptor::context`](crate::ChartDescriptor::context) on its
    /// bytes and dots.
    #[error("invalid context {0:?}")]
    InvalidContext(String),
    /// A context ends like an entity's chart-id suffix (`_overflow`, or `_`
    /// and 16 hex digits).
    #[error("context {0:?} ends like an entity suffix")]
    ContextLikeEntitySuffix(String),
    /// A context is already used in the metric set.
    #[error("duplicate context {0:?}")]
    DuplicateContext(String),
    /// A histogram's priority is above `i32::MAX - 2`: its `max` and heatmap
    /// charts take the next two priorities.
    #[error("histogram priority {0} must be at most {max}", max = i32::MAX - 2)]
    HistogramPriority(i32),
}
