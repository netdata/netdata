//! The logs module's hub: `query` runs journal queries, `table` renders
//! their results.
//!
//! - query — [`LogQuery`], the builder callers drive: `execute` returns one
//!   result vector, `execute_page` paginates and resumes from a
//!   [`query::PaginationState`]. Inside, `retrieve_log_entries`
//!   (query.rs:249) picks and merges the matching entry IDs, then the
//!   private `extract_entry_data` (query.rs:546) re-opens each journal file
//!   once and returns `Vec<LogEntryData>` (query.rs:526) — raw field=value
//!   pairs, untransformed; the crate's only producer of `LogEntryData`.
//!
//! - table — the render types [`Table`] (text layout via its `Display`
//!   impl, table.rs:115), [`CellValue`], [`ColumnInfo`], plus
//!   `entry_data_to_table` (table.rs:173), which turns a vector of
//!   `LogEntryData` into a `Table` of the requested columns — `timestamp`
//!   first, cells verbatim — returning journal-core's `Result`, not the
//!   crate's, although its body never constructs an error. Display
//!   transformations are journal-function's job
//!   (journal-function/src/netdata/builder.rs:14).
//!
//! Exports: the six names re-exported below are flattened again at the
//! crate root (lib.rs:38) and by journal-function
//! (journal-function/src/lib.rs:12-17), where production callers go
//! (otel-legacy-logs/src/handler.rs:214,243); `query::PaginationState` is
//! the one item nothing re-exports. transformations.rs in this directory
//! is not compiled — no `mod` declares it; the live registry is
//! journal-function's (journal-function/src/netdata/transformations.rs:12).

pub mod query;
pub mod table;

pub use query::{LogEntryData, LogQuery};
pub use table::{CellValue, ColumnInfo, Table, entry_data_to_table};
