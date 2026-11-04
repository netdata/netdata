//! Charts a plugin computes itself from the events it sees (requests, log
//! records, its own counters), sent to the agent as chart-protocol text.
//!
//! ```
//! use netdata_plugin_metrics::{
//!     Batch, Chart, ChartType, Labels, MetricSetBuilder, Settings, Store, dimensions,
//! };
//!
//! dimensions! {
//!     enum Outcome {
//!         Success = "success",
//!         Error = "error",
//!     }
//! }
//!
//! // Once, at startup: declare the instruments and their charts.
//! let mut builder = MetricSetBuilder::new("svc", Settings::default());
//! let requests = builder.counter::<Outcome>(Chart {
//!     id: "requests",
//!     title: "Requests",
//!     units: "requests/s",
//!     family: "requests",
//!     context: "svc.requests",
//!     chart_type: ChartType::Stacked,
//!     priority: 1000,
//! });
//! let set = builder.build();
//! let mut store = Store::new(set.clone());
//!
//! // Per unit of work, for example one request: record its events at their
//! // own time, then merge once the work is durable.
//! let mut labels = Labels::new();
//! labels.push("service.name", "checkout");
//! let mut batch = Batch::new(&set);
//! let checkout = batch.instance(labels);
//! batch.record(checkout, 100).add(requests, Outcome::Error, 1.0);
//! store.merge(batch, 100);
//!
//! // Once per second: write the finished slots to the agent.
//! let text: Vec<u8> = store.emit(101, 1 << 20).concat();
//! assert!(String::from_utf8(text).unwrap().contains("SET error = 1\n"));
//! ```
//!
//! Terms:
//! - an *instrument* is a [`Counter`], [`Cumulative`], [`Gauge`] or
//!   [`Histogram`]; each brings its chart, and the first three are split by
//!   the values of their [`Dimensions`];
//! - an *instance* is one label set, for example one service; it gets its
//!   own copy of every chart of the set, so instruments that need different
//!   labels (per service, process-wide) belong in different metric sets;
//! - a *slot* is one `update_every`-second step (see [`Settings`]); it
//!   becomes one point per dimension, stored at the slot's end time.

mod batch;
mod metric_set;
mod store;

pub use batch::{Batch, InstanceRef, Record};
pub use metric_set::{
    Cap, Chart, Counter, Cumulative, Dimensions, Gauge, Histogram, Labels, MetricSet,
    MetricSetBuilder, Settings,
};
pub use netdata_plugin_protocol::ChartType;
pub use store::{Stats, Store};
