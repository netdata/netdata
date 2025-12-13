//! Log entry formatting and display.
//!
//! This module provides generic types for converting log entries
//! into formatted tables.

pub mod query;
pub mod table;

pub use query::{LogEntryData, LogQuery};
pub use table::{CellValue, ColumnInfo, Table, entry_data_to_table};
