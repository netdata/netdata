//! Hub of the journal file layer: declares its submodules and defines the
//! re-export surface other crates consume; the public items all land under
//! `journal_core::file::...`. lib.rs re-exports only a flat subset of this
//! (lib.rs:31-34) and renames `sigbus::install_handler` to
//! `install_sigbus_handler` (lib.rs:37); the layout mirrors the published
//! twin's (src/crates/jf/journal_file/src/lib.rs:2-32).
//!
//! Public submodules - cursor: JournalCursor, the seek-and-step iterator over
//! the entry chain; file: JournalFile, its options, utilization stats and
//! field/entry iterators; filter: FilterExpr/JournalFilter/LogicalOp; hash:
//! journald hash functions; mmap: MemoryMap/MemoryMapMut traits,
//! WindowManager and memmap2 re-exports; offset_array: the entry-array chain
//! (List, Cursor, InlinedCursor, Direction); reader: JournalReader; sigbus:
//! SIGBUS handler; writer: JournalWriter. Private - object: on-disk object
//! layout, exposed only through the `pub use` re-exports below; guarded_cell
//! and value_guard: the one-borrow machinery behind JournalFile's
//! window-backed views.
//!
//! index_filter.rs sits in this directory but is not declared here: it
//! imports a `super::index` that does not exist in this crate (FileIndex
//! lives in journal-index/src/file_index.rs:25), so `mod index_filter;` would
//! not compile until an `index` module is provided.
//!
//! JournalFileMap, the alias at the bottom, pins JournalFile to a real
//! memmap2::Mmap instead of the MemoryMap trait.
pub mod cursor;
pub mod file;
pub mod filter;
mod guarded_cell;
pub mod hash;
pub mod mmap;
mod object;
pub mod offset_array;
pub mod reader;
pub mod sigbus;
mod value_guard;
pub mod writer;

// Core file types: JournalFile, its reader and its writer.
pub use file::{BucketUtilization, JournalFile, JournalFileOptions};
pub use reader::JournalReader;
pub use writer::JournalWriter;

// Reader stepping vocabulary: seek keys (Location) and direction (Direction).
pub use cursor::Location;
pub use offset_array::Direction;

// Cursor and filter types; the filter types are not re-exported by lib.rs.
pub use cursor::JournalCursor;
pub use filter::{FilterExpr, JournalFilter, LogicalOp};

// Object-layout types other crates import (e.g. JournalState: journal-index,
// journal-log-writer).
pub use object::{EntryItemsType, HashableObject, HeaderIncompatibleFlags, JournalState};

// memmap2 mapping types, surfaced via file/mmap.rs (mmap.rs:8).
pub use mmap::{Mmap, MmapMut};

// journal_hash_data (hash.rs:28); writer.rs flat-imports it from this path.
pub use crate::file::hash::journal_hash_data;

// Flat, crate-internal access to object types (writer.rs:6-12,
// value_guard.rs:70); not public API.
pub(crate) use object::*;

// DataObject, consumed by journal-index (field_types.rs:231).
pub use object::DataObject;

pub type JournalFileMap = JournalFile<Mmap>;
