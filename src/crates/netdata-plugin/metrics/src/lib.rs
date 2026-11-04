//! Metrics a plugin derives itself, turned into Netdata charts.
//!
//! A plugin declares a [`MetricSet`]: instruments, each with its chart, that
//! every instance of something (a service, a pipeline stage, the plugin
//! itself) gets a copy of. Code that sees events records them into a
//! [`Batch`], keyed by the instance's [`Labels`] and the event's own time; a
//! [`Store`] merges batches into one accumulator per slot (`update_every`
//! seconds) and, each time the caller calls [`Store::emit`], emits every
//! finished slot as chart-protocol text.
//!
//! Declaring a metric set:
//! - each instrument has a fixed set of [`Dimensions`], declared as an enum
//!   with [`dimensions!`]: one chart dimension per value;
//! - a [`Counter`] (rates per second), a [`Cumulative`] (running totals,
//!   charted as increases per second) and a [`Gauge`] get one [`Chart`]
//!   each, a [`Histogram`] its mean/maximum chart and a heatmap;
//! - [`Settings`] hold the slot length, lateness, expiry and instance cap.
//!
//! What the agent gets:
//! - every chart is defined (with its labels) before its first update, and
//!   defined again when an instance returns after it was marked obsolete;
//! - each slot is emitted once, in order, and its point is stored at the
//!   slot's end time, not at the time it was emitted;
//! - every dimension is a float, so no value is truncated;
//! - output comes in chunks under a byte budget, split only between whole
//!   chart blocks.

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
