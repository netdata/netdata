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
//! let mut store = Store::new(&set);
//!
//! // Per unit of work, for example one request: record its events at their
//! // own time, then merge once the work is durable.
//! let mut labels = Labels::new();
//! labels.push("service.name", "checkout");
//! let mut batch = Batch::new(&set);
//! batch
//!     .entity(labels)
//!     .record(100)
//!     .add(requests, Outcome::Error, 1.0);
//! store.merge(batch, 100);
//!
//! // Once per second: write the finished slots to the agent.
//! let text: Vec<u8> = store.emit(101, 1 << 20).concat();
//! assert!(String::from_utf8(text).unwrap().contains("SET error = 1\n"));
//! ```
//!
//! Terms:
//! - an *instrument* is a [`Counter`], [`CumulativeCounter`], [`Gauge`] or
//!   [`Histogram`]; each brings its chart, and the first three are split by
//!   the values of their [`Dimensions`];
//! - an *entity* is the thing being monitored, for example one service,
//!   identified by its [`Labels`]; it gets its own instance (in Netdata's
//!   sense: one chart) of every chart of the set, so instruments that need
//!   different labels (per service, process-wide) belong in different metric
//!   sets. There is no explicit create: recording with a new label set is a
//!   new entity;
//! - a *slot* is one collection interval `I` (see [`Settings`]): slot `n`
//!   covers `[n × I, (n + 1) × I)` and holds the records whose time falls in
//!   it; it becomes one point per dimension, stored at its end,
//!   `(n + 1) × I`;
//! - a *record* is one event of an entity at its own time, with its
//!   measurements (`add`, `set_value`, `set_total`, `observe`); the records of one
//!   entity in one slot combine into that slot's point.

mod batch;
mod metric_set;
mod store;

pub use batch::{Batch, BatchEntity, Measurements};
pub use metric_set::{
    Cap, Chart, Counter, CumulativeCounter, Dimensions, Gauge, Histogram, Labels, MetricSet,
    MetricSetBuilder, Settings,
};
pub use netdata_plugin_protocol::ChartType;
pub use store::{Stats, Store};
