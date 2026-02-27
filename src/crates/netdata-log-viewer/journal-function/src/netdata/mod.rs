//! Netdata-specific formatting and protocol types.
//!
//! This module contains all Netdata-specific logic for the systemd-journal function plugin,
//! including protocol types, UI formatting for logs and histograms, and field transformations.

pub mod builder;
pub mod columns;
pub mod facets;
pub mod histogram;
pub mod response;
pub mod severity;
pub mod transformations;
pub mod types;
pub mod ui_types;

// High-level builders
pub use builder::build_ui_response;

// Protocol types
pub use types::{
    Items, JournalRequest, JournalResponse, MultiSelection, MultiSelectionOption, Pagination,
    RequestParam, RequiredParam, Version,
};

// Log formatting
pub use columns::FilterType;
pub use severity::Severity;
pub use transformations::{FieldTransformation, TransformationRegistry, systemd_transformations};

// Histogram/facet formatting
pub use facets::facets;
pub use histogram::{available_histograms, histogram};

// UI types
pub use ui_types::{
    AvailableHistogram, Chart, ChartDimensions, ChartPoint, ChartResult, ChartView, DataPoint,
    Facet, FacetOption, Histogram, Response,
};
