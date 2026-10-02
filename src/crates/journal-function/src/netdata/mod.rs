//! Netdata's Functions-protocol and logs-UI response layer for the
//! systemd-journal function: the request/response types plus the builders
//! that turn a query histogram and its matched entries into the logs
//! table's `columns`/`data`, facet filters, and histograms.
//!
//! Module map - every submodule is `pub`; the re-exports below are the
//! flat surface (nothing here is `doc(hidden)`):
//! - builder - `build_ui_response`: query histogram + matched entries ->
//!   the `(columns, data)` JSON pair (builder.rs:79)
//! - columns - logs-table column schema: `FilterType`, `ColumnSchema`,
//!   `generate_column_schema` (columns.rs:13,45,238)
//! - facets - facet filter options from the histogram (facets.rs:16)
//! - histogram - histogram chart + available-histogram lists
//!   (histogram.rs:18,50)
//! - response - table -> row arrays (`table_to_netdata_response`,
//!   response.rs:50)
//! - severity - syslog PRIORITY -> `Severity` (severity.rs:14)
//! - transformations - field-value transforms + the systemd registry
//!   (`systemd_transformations`, transformations.rs:609)
//! - types - Functions-protocol types: `JournalRequest`,
//!   `JournalResponse` (types.rs:13,168)
//! - ui_types - the flat serde types the builders emit: `Facet`,
//!   `Histogram`, `Chart` (ui_types.rs:24,44,59)
//!
//! Sole external consumer (grep-verified; journal-function has no other
//! dependent, otel-legacy-logs/Cargo.toml:19):
//! otel-legacy-logs/src/handler.rs imports `netdata` (handler.rs:23) and
//! takes everything through the re-exports below (handler.rs:29-32,
//! 469-517) to assemble one `JournalResponse` - its `facets`/`histogram`/
//! `columns`/`data` fields are filled from these builders
//! (types.rs:177-182).
//!
//! Not re-exported: the column-schema builders (`generate_column_schema`,
//! `columns_to_sorted_json`, columns.rs:238,280) and
//! `table_to_netdata_response` (response.rs:50) are reachable only via
//! their module paths (`netdata::columns::`, `netdata::response::`). And
//! the `Histogram` here is the UI chart type; the crate-root `Histogram`
//! is journal-engine's bucket histogram (lib.rs:14).

pub mod builder;
pub mod columns;
pub mod facets;
pub mod histogram;
pub mod response;
pub mod severity;
pub mod transformations;
pub mod types;
pub mod ui_types;

// One-call logs-table builder: the `(columns, data)` pair for `JournalResponse`
pub use builder::build_ui_response;

// Functions-protocol types: the request and the response envelope
pub use types::{
    Items, JournalRequest, JournalResponse, MultiSelection, MultiSelectionOption, Pagination,
    RequestParam, RequiredParam, Version,
};

// Field presentation: column filter kinds, severity levels, value transforms
pub use columns::FilterType;
pub use severity::Severity;
pub use transformations::{FieldTransformation, TransformationRegistry, systemd_transformations};

// Facet options + histogram charts built from the query histogram
pub use facets::facets;
pub use histogram::{available_histograms, histogram};

// UI serde types: what the facet/histogram builders emit and `JournalResponse` carries
pub use ui_types::{
    AvailableHistogram, Chart, ChartDimensions, ChartPoint, ChartResult, ChartView, DataPoint,
    Facet, FacetOption, Histogram,
};
