//! Charts a plugin computes itself from the events it sees (requests, log
//! records, its own counters), sent to the agent as chart-protocol text.
//!
//! ```
//! use netdata_plugin_metrics::{
//!     Aggregator, Batch, Chart, ChartType, Labels, MetricSetBuilder, Settings, dimensions,
//! };
//!
//! dimensions! {
//!     enum Outcome {
//!         Success = "success",
//!         Error = "error",
//!     }
//! }
//!
//! // Once, at startup: declare the instruments and their chart templates, and
//! // create the aggregator.
//! let mut builder = MetricSetBuilder::new(Settings::default());
//! let requests = builder.counter::<Outcome>(Chart {
//!     title: "Requests",
//!     units: "requests/s",
//!     family: "requests",
//!     context: "svc.requests",
//!     chart_type: ChartType::Stacked,
//!     priority: 1000,
//! });
//! let set = builder.build();
//! let mut aggregator = Aggregator::new(&set);
//!
//! // Per unit of work, for example one export request: record its events at
//! // their own time in one batch, then merge it once the work is durable.
//! let mut batch = Batch::new(&set);
//!
//! for (service, outcomes) in [
//!     ("checkout", &[Outcome::Success, Outcome::Error][..]),
//!     ("cart", &[Outcome::Success]),
//! ] {
//!     let mut labels = Labels::new();
//!     labels.insert("service.name", service);
//!     let mut entity = batch.entity(labels);
//!     for &outcome in outcomes {
//!         entity.record(100).add(requests, outcome, 1.0);
//!     }
//! }
//!
//! aggregator.merge(batch, 100);
//!
//! // Once per second: write the windows whose allowed lateness has passed
//! // to the agent.
//! let text = String::from_utf8(aggregator.emit(101, 1 << 20).concat()).unwrap();
//! assert!(text.contains("SET success = 1\nSET error = 1\n"));
//! ```
//!
//! Terms:
//! - an *instrument* is a [`Counter`], [`CumulativeCounter`], [`Gauge`] or
//!   [`Histogram`]; each brings its chart template, and the first three are
//!   split into dimensions, one per value of their [`Dimension`] type;
//! - an *entity* is the thing being monitored, for example one service,
//!   identified by its [`Labels`]; it gets one chart instance of every chart
//!   template of the set, so instruments that need different labels (per
//!   service, process-wide) belong in different metric sets. There is no
//!   explicit create: recording with a new label set is a new entity;
//! - a *window* is one collection interval `I` (see [`Settings`]): window `n`
//!   covers `[n × I, (n + 1) × I)` and holds the events whose time falls in
//!   it; it becomes one point per dimension, stored at its end,
//!   `(n + 1) × I`;
//! - an *event* is one occurrence of an entity at its own time, recorded
//!   with [`BatchEntity::record`] and its measurements (`add`, `set_value`,
//!   `set_total`, `observe`); the events of one entity in one window combine
//!   into that window's point.

mod aggregator;
mod batch;
mod metric_set;

pub use aggregator::{Aggregator, Stats};
pub use batch::{Batch, BatchEntity, Measurements};
pub use metric_set::{
    CardinalityLimit, Chart, Counter, CumulativeCounter, Dimension, Gauge, Histogram, Labels,
    MetricSet, MetricSetBuilder, Settings,
};
pub use netdata_plugin_protocol::ChartType;
