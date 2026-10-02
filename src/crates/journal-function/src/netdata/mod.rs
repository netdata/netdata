//! Netdata's Functions-protocol and logs-UI response layer for the
//! systemd-journal function: the request/response types plus the builders
//! that turn a query histogram and its matched entries into the logs
//! table's `columns`/`data`, facet filters, and histograms.
//!
//! Module map - every submodule is `pub`; the re-exports below are the
//! flat surface (nothing here is `doc(hidden)`):
//! - builder - `build_ui_response`: query histogram + matched entries ->
//!   the `(columns, data)` JSON pair ([`build_ui_response`])
//! - columns - logs-table column schema: [`FilterType`],
//!   [`columns::ColumnSchema`], [`columns::generate_column_schema`]
//! - facets - facet filter options from the histogram ([`facets()`])
//! - histogram - histogram chart + available-histogram lists
//!   ([`available_histograms`], [`histogram()`])
//! - response - table -> row arrays
//!   ([`response::table_to_netdata_response`])
//! - severity - syslog PRIORITY -> [`Severity`]
//! - transformations - field-value transforms + the systemd registry
//!   ([`systemd_transformations`])
//! - types - Functions-protocol types: [`JournalRequest`],
//!   [`JournalResponse`]
//! - ui_types - the flat serde types the builders emit: [`Facet`],
//!   [`Histogram`], [`Chart`]
//!
//! Sole external consumer (grep-verified; journal-function has no other
//! dependent, see the `journal-function` dep in
//! `otel-legacy-logs/Cargo.toml`):
//! `otel-legacy-logs/src/handler.rs` imports `netdata` and
//! takes everything through the re-exports below
//! to assemble one `JournalResponse` - its `facets`/`histogram`/
//! `columns`/`data` fields are filled from these builders
//! (the `JournalResponse` fields, filled in `LegacyLogsHandler::on_call`).
//!
//! Not re-exported: the column-schema builders
//! ([`columns::generate_column_schema`], [`columns::columns_to_sorted_json`]) and
//! [`response::table_to_netdata_response`] are reachable only via
//! their module paths (`netdata::columns::`, `netdata::response::`). And
//! the `Histogram` here is the UI chart type; the crate-root `Histogram`
//! is journal-engine's bucket histogram ([`crate::Histogram`]).

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
