//! Declarative chart creation and management for Netdata plugins.
//!
//! This module provides declarative chart definitions using Rust structs
//! annotated with `schemars` attributes. A derive macro generates efficient dimension
//! writing code, and the registry handles automatic sampling and batched emission.

mod chart_trait;
mod handle;
mod metadata;
mod registry;
mod tracker;
mod writer;

// Re-export public API
pub use chart_trait::{ChartDimensions, InstancedChart, NetdataChart};
pub use handle::ChartHandle;
pub use metadata::{ChartMetadata, ChartType, DimensionAlgorithm, DimensionMetadata};
pub use registry::ChartRegistry;
pub use tracker::TrackedChart;
pub use writer::ChartWriter;
