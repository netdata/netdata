//! Common types and utilities shared across journal crates.
//!
//! This crate provides foundational types and utilities used by multiple
//! journal-related crates, avoiding code duplication and circular dependencies.

pub mod collections;
pub mod compat;
pub mod system;
pub mod time;
pub mod time_range;

pub use time::{Microseconds, RealtimeClock, Seconds, monotonic_now};
pub use time_range::TimeRange;

// Re-export collection types for convenience
pub use collections::{HashMap, HashSet, VecDeque};

// Re-export system utilities for convenience
pub use system::{load_boot_id, load_machine_id};
