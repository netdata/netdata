//! Foundational utilities for Netdata plugins.
//!
//! This crate provides low-level primitives and utilities that other plugin
//! crates build upon, including async operation management and control flow.

// Timeout management
pub mod timeout;
pub use timeout::Timeout;
