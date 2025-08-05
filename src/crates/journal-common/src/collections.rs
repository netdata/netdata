//! Collection type aliases.
//!
//! This module provides convenient aliases for the hash-based collections
//! used throughout the journal crates. We use `rustc_hash::FxHashMap` and
//! `FxHashSet` for their performance characteristics.

pub type HashMap<K, V> = rustc_hash::FxHashMap<K, V>;
pub type HashSet<T> = rustc_hash::FxHashSet<T>;
pub type VecDeque<T> = std::collections::VecDeque<T>;
