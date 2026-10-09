//! Collection type aliases.
//!
//! Single import point for the collections shared by the journal crates, so
//! the hasher behind every hash map and set in the stack is decided in one
//! file. `HashMap` and `HashSet` wrap `rustc_hash::FxHasher` (workspace
//! dependency `rustc-hash`): a fast, non-cryptographic, unseeded polynomial
//! hash with no HashDoS hardening, unlike std's randomly-seeded SipHash, so
//! crafted keys can degrade these maps. Because centralizing that choice is
//! the point of this module, code in this stack should import
//! `collections::{HashMap, HashSet}` rather than `std::collections` hash
//! types ad hoc, so a future hasher swap lands in one file. `VecDeque` is the
//! standard-library type, aliased only so the three share one import path.
//!
//! Adoption is selective, not crate-wide: parts of journal-function and
//! journal-engine import `std::collections` hash types directly.
//!
//! Consumers (grep-verified). Via the `journal_core::collections` re-export
//! shim (`journal-core/src/collections.rs`): journal-core `field_map.rs`
//! (HashMap, HashSet), `file/file.rs` (HashMap); journal-index
//! `file_index.rs`, `file_indexer.rs`; journal-function `netdata/facets.rs`,
//! `netdata/histogram.rs`; journal-engine `histogram.rs`; journal-log-writer
//! `log/chain.rs`. Direct `journal_common::collections` imports:
//! journal-registry `registry/mod.rs`, `repository/collection.rs`,
//! `repository/mod.rs`. The crate root also re-exports the three aliases flat
//! ([`crate::HashMap`], [`crate::HashSet`], [`crate::VecDeque`]); no external
//! importer of that flat path was found.

/// Hash map backed by `rustc_hash::FxHasher`; same API as `std::collections::HashMap`.
pub type HashMap<K, V> = rustc_hash::FxHashMap<K, V>;
/// Hash set backed by `rustc_hash::FxHasher`; same API as `std::collections::HashSet`.
pub type HashSet<T> = rustc_hash::FxHashSet<T>;
/// Standard-library `VecDeque`, aliased for a uniform import path.
pub type VecDeque<T> = std::collections::VecDeque<T>;
