// Modules - keep some public for advanced usage
pub mod cursor;
pub mod file;
pub mod filter;
mod guarded_cell;
pub mod hash;
pub mod mmap;
mod object;
pub mod offset_array;
pub mod reader;
pub(crate) mod sigbus;
mod value_guard;
pub mod writer;

// Core functionality
pub use file::{BucketUtilization, JournalFile, JournalFileOptions};
pub use reader::JournalReader;
pub use writer::JournalWriter;

// Essential types for working with readers
pub use cursor::Location;
pub use offset_array::Direction;

// Advanced filtering (for users who need it)
pub use cursor::JournalCursor;
pub use filter::{FilterExpr, JournalFilter, LogicalOp};

// For FFI compatibility and advanced object manipulation
pub use object::{EntryItemsType, HashableObject, JournalState, HeaderIncompatibleFlags};

// Re-export commonly needed external types
pub use mmap::{Mmap, MmapMut};

// Internal utilities that might be needed
pub use crate::file::hash::journal_hash_data;

// Internal re-exports needed by the crate itself (not part of public API)
pub(crate) use object::*;

// Re-export DataObject for journal-index
pub use object::DataObject;

pub type JournalFileMap = JournalFile<Mmap>;
