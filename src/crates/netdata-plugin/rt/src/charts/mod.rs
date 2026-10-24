//! Declarative metrics publishing for Netdata plugins: pluginsd chart commands
//! (CHART/DIMENSION/BEGIN/SET/END) built from Rust structs.
//!
//! A chart is an ordinary struct. schemars `x-chart-*` / `x-dimension-*` extensions
//! carry its metadata, and `#[derive(JsonSchema, NetdataChart)]` (macro in the
//! `netdata-plugin-charts-derive` crate) generates a [`ChartDimensions`] impl that
//! writes every named field (except the `x-chart-instance` field) as an i64 `SET`
//! value through [`ChartWriter`], with no JSON serialization. The same annotations
//! are read back out of the generated JSON schema into [`ChartMetadata`]
//! (`NetdataChart::chart_metadata`).
//!
//! Charts are registered on the plugin runtime (`PluginRuntime::register_chart` /
//! `register_instanced_chart`, crate root), which returns a [`ChartHandle`] for
//! updating the values from any task or thread; updates become visible on the next
//! sample tick. `run()` starts the [`ChartRegistry`] background task and cancels it
//! on shutdown. The registry emits each chart's CHART/DIMENSION definition once and
//! re-emits BEGIN/SET/END on every tick even when values are unchanged, so each
//! sample interval carries a datapoint. Emission is batched: one lock acquisition
//! on the shared outbound `MessageWriter` per batch tick, and chart write errors
//! are discarded rather than failing the plugin. An [`InstancedChart`] registration
//! is a single concrete chart: the `{instance}` placeholder in the id is replaced
//! by the initial value's `instance_id()` at registration time.
//!
//! Submodules follow the data flow: `handle` holds the shared update state, `registry`
//! and `tracker` schedule and track emission, `writer` and `metadata` build the
//! pluginsd commands, and `chart_trait` defines the traits and annotation extraction.

mod chart_trait;
mod handle;
mod metadata;
mod registry;
mod tracker;
mod writer;

// Public API. lib.rs re-exports this list at the crate root (all but ChartWriter,
// plus the `NetdataChart` derive macro), so downstream code imports from `rt`.
pub use chart_trait::{ChartDimensions, InstancedChart, NetdataChart};
pub use handle::ChartHandle;
pub use metadata::{ChartMetadata, ChartType, DimensionAlgorithm, DimensionMetadata};
pub use registry::ChartRegistry;
pub use tracker::TrackedChart;
pub use writer::ChartWriter;
