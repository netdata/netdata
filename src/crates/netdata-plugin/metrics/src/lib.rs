//! Metrics a plugin derives itself, turned into Netdata charts.
//!
//! A plugin declares a [`Family`]: a set of charts that every instance of
//! something (a service, a pipeline stage, the plugin itself) gets, plus the
//! cells those charts read. Code that sees events records them into a
//! [`Batch`], keyed by the instance's [`Labels`] and the event's own time; a
//! [`Store`] merges batches into one-slot-per-interval accumulators and, once
//! per tick, renders every finished slot as chart-protocol text
//! (`netdata-plugin-protocol` encodes the lines). The crate reads no clock and
//! does no I/O: callers pass the current time and deliver the bytes.
//!
//! Declaring a family:
//! - each instrument has a fixed set of [`Dimensions`], declared as an enum
//!   with [`dimensions!`]: one cell and one chart dimension per value;
//! - each instrument brings its [`Chart`]: a [`Counter`] (rates per second),
//!   a [`Cumulative`] (running totals, charted as increases per second) and
//!   a [`Gauge`] get one chart each, a [`Histogram`] its mean/maximum chart
//!   and a heatmap;
//! - [`Settings`] hold the slot length, lateness, expiry and instance cap.
//!
//! Layers, bottom-up:
//! - `family`: cells (the numbers a slot accumulates and how two values
//!   merge), the instruments that allocate them and their charts;
//! - `batch`: request-local recording, so a caller can count a request only
//!   after it is durable and then merge it under one short lock;
//! - `store`: instances, the slot window, lateness, the instance cap with its
//!   overflow instance, idle expiry, and rendering.
//!
//! Contracts the store keeps for the agent:
//! - every chart is defined (with its labels) before its first update, and
//!   re-defined when an instance returns after it was marked obsolete;
//! - slots are rendered in order, each once, after the family's lateness,
//!   and each update is placed at its slot's end (`END <t> 0 1` on charts
//!   defined with `store_first`), so points sit at the time of their records
//!   rather than the time they were rendered;
//! - an instance idle for the expiry time gets its charts marked obsolete;
//! - every dimension is a float, so no value is truncated;
//! - output comes in chunks under a byte budget, split only between whole
//!   chart blocks.

mod batch;
mod family;
mod store;

pub use batch::{Batch, InstanceRef, Record};
pub use family::{
    Cap, Chart, Counter, Cumulative, Dimensions, Family, FamilyBuilder, Gauge, Histogram, Labels,
    Settings,
};
pub use netdata_plugin_protocol::ChartType;
pub use store::{MAX_BACKLOG_SLOTS, Stats, Store};
