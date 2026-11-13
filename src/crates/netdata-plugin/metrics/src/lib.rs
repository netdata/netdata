//! Metrics a plugin computes itself from the events it sees (requests, log
//! entries, its own counters): a table of one row per entity and window,
//! sent to the agent as charts.
//!
//! ```
//! use netdata_plugin_metrics::{
//!     Aggregator, Batch, ChartDescriptor, ChartType, Labels, MetricSetBuilder, Settings, columns,
//! };
//!
//! columns! {
//!     enum Outcome {
//!         Success = "success",
//!         Error = "error",
//!     }
//! }
//!
//! // Once, at startup: declare the metric set, each instrument with its
//! // chart, and create the aggregator.
//! let mut builder = MetricSetBuilder::new(Settings::default())?;
//! let requests = builder.counter::<Outcome>(ChartDescriptor {
//!     title: "Requests".into(),
//!     units: "requests/s".into(),
//!     family: "requests".into(),
//!     context: "svc.requests".into(),
//!     chart_type: ChartType::Stacked,
//!     priority: 1000,
//! })?;
//! let set = builder.build()?;
//!
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
//! // Each value is exact, as `@` and its IEEE 754 bits in base64: 1 here.
//! assert!(text.contains("SET success = @D/wAAAAAAAA\nSET error = @D/wAAAAAAAA\n"));
//! # Ok::<(), netdata_plugin_metrics::Error>(())
//! ```
//!
//! The table:
//! - an *instrument* is a [`Counter`], [`CumulativeCounter`], [`Gauge`] or
//!   [`Histogram`]: a group of the table's columns (a histogram has three
//!   groups); the others have one column per value of their [`Column`]
//!   type;
//! - an *entity* is the thing being monitored, for example one service,
//!   identified by its [`Labels`], the key of its rows. Every entity has
//!   every instrument of the set, so instruments that need different labels
//!   (per service, process-wide), or that do not apply to every entity,
//!   belong in different metric sets. There is no explicit create:
//!   recording with a new label set is a new entity;
//! - a *window* is one collection interval `I` (see [`Settings`]): window `n`
//!   covers `[n × I, (n + 1) × I)` and holds the events whose time falls in
//!   it; each entity has one row per window, which yields one value per
//!   column (a mean's sum and count yield one), stored at the window's end,
//!   `(n + 1) × I`. A window is *open*, taking events, until
//!   [`Aggregator::emit`] emits it, once;
//! - an *event* is one thing that happened to an entity (a request, a log
//!   entry, a poll) at a time of its own, its *event time*;
//!   [`BatchEntity::record`] starts one and returns its measurements (`add`,
//!   `set_value`, `set_total`, `observe`). The events of one entity in one
//!   window combine into that window's row.
//!
//! The charts are how the agent shows the table, declared with each
//! instrument by a [`ChartDescriptor`]; recording never sees them. Each
//! column group becomes a *chart template* whose *dimensions* are the group's
//! values, and each entity gets one *chart instance* (in Netdata's sense: one
//! chart with its own chart id) of every template. Where OpenTelemetry splits
//! a metric by attributes, the table splits an instrument by entity and by
//! column; the charts show them as chart instances and dimensions.
//!
//! A mistake in declaring a metric set makes the [`MetricSetBuilder`] step
//! that makes it return an [`Error`], and that step adds nothing. Other
//! mistakes of the plugin's own code are checked by debug assertions: an
//! empty, too long or repeated label key, and, when recording or merging, an
//! instrument or batch of another metric set or a column index out of range
//! (a broken manual [`Column`] implementation). Tests and debug builds panic
//! on them, release builds skip the checks. [`Labels::insert`] and
//! [`Aggregator::merge`] list them under `# Panics`, [`Measurements`] in its
//! own documentation.

mod aggregator;
mod batch;
mod chart;
mod emit;
mod error;
mod layout;
mod metric_set;

pub use aggregator::{Aggregator, Stats};
pub use batch::{Batch, BatchEntity, Measurements};
pub use chart::ChartDescriptor;
pub use error::Error;
pub use metric_set::{
    CardinalityLimit, Column, Counter, CumulativeCounter, Gauge, Histogram, Labels, MetricSet,
    MetricSetBuilder, Settings,
};
pub use netdata_plugin_protocol::ChartType;
