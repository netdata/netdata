//! Charts a plugin computes itself from the events it sees (requests, log
//! entries, its own counters), sent to the agent as chart-protocol text.
//!
//! ```
//! use netdata_plugin_metrics::{
//!     Aggregator, Batch, ChartDescriptor, ChartType, Labels, MetricSetBuilder, Settings,
//!     dimensions,
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
//! let requests = builder.counter::<Outcome>(ChartDescriptor {
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
//! // their event time in one batch, then merge it once the work is durable.
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
//!   [`Histogram`]; each brings its *chart template* (a [`ChartDescriptor`]
//!   and its dimensions; a histogram brings three, derived from its
//!   descriptor), and the other instruments are split into dimensions, one
//!   per value of their [`Dimension`] type;
//! - an *entity* is the thing being monitored, for example one service,
//!   identified by its [`Labels`]; it gets one *chart instance* (in Netdata's
//!   sense: one chart with its own chart id) of every chart template of the
//!   set, so instruments that need different labels (per service,
//!   process-wide), or that do not apply to every entity, belong in
//!   different metric sets. There is no explicit create: recording with a new
//!   label set is a new entity;
//! - a *window* is one collection interval `I` (see [`Settings`]): window `n`
//!   covers `[n × I, (n + 1) × I)` and holds the events whose time falls in
//!   it; it becomes one point per dimension, stored at its end,
//!   `(n + 1) × I`. A window is *open*, taking events, until
//!   [`Aggregator::emit`] emits it, once;
//! - an *event* is one thing that happened to an entity (a request, a log
//!   entry, a poll) at a time of its own, its *event time*;
//!   [`BatchEntity::record`] starts one and returns its measurements (`add`,
//!   `set_value`, `set_total`, `observe`). The events of one entity in one
//!   window combine into that window's point.
//!
//! Where OpenTelemetry splits a metric by attributes, Netdata and this crate
//! split a chart template two ways: by entity (one chart instance per label
//! set) and by dimension (one line per value of a [`Dimension`] type).

mod aggregator;
mod batch;
mod chart;
mod emit;
mod layout;
mod metric_set;

pub use aggregator::{Aggregator, Stats};
pub use batch::{Batch, BatchEntity, Measurements};
pub use chart::ChartDescriptor;
pub use metric_set::{
    CardinalityLimit, Counter, CumulativeCounter, Dimension, Gauge, Histogram, Labels, MetricSet,
    MetricSetBuilder, Settings,
};
pub use netdata_plugin_protocol::ChartType;
