//! The logs module's hub: `query` runs journal queries, `table` renders
//! their results.
//!
//! - query — [`LogQuery`], the builder callers drive: `execute` returns one
//!   result vector, `execute_page` paginates and resumes from a
//!   [`query::PaginationState`]. Inside, `retrieve_log_entries`
//!   (`logs/query.rs`, private) picks and merges the matching entry IDs, then the
//!   private `extract_entry_data` re-opens each journal file
//!   once and returns [`LogEntryData`] values — field=value
//!   pairs, values verbatim up to lossy UTF-8 decoding
//!   (invalid sequences replaced with U+FFFD) and names
//!   reverse-mapped to their OTEL forms; the crate's only
//!   producer of `LogEntryData`.
//!
//! - table — the render types [`Table`] (text layout via its `Display`
//!   impl), [`CellValue`], [`ColumnInfo`], plus
//!   [`entry_data_to_table`], which turns a slice of
//!   `LogEntryData` into a `Table` of the requested columns — `timestamp`
//!   first, cells verbatim — returning journal-core's `Result`, not the
//!   crate's, although its body never constructs an error. Display
//!   transformations are journal-function's job
//!   (`journal-function/src/netdata/builder.rs` `build_ui_response`).
//!
//! Exports: the six names re-exported below are flattened again at the
//! crate root and by journal-function
//! (`journal-function/src/lib.rs`), where production callers go
//! (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler`); `query::PaginationState` is
//! the one item nothing re-exports.

pub mod query;
pub mod table;

pub use query::{LogEntryData, LogQuery};
pub use table::{CellValue, ColumnInfo, Table, entry_data_to_table};
