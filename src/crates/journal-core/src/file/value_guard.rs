//! RAII half of the journal mmap layer's one-borrow model.
//!
//! Journal objects are zerocopy views into the window manager's mmap windows
//! (file/mmap.rs), which keeps a bounded set of windows (`max_windows`) and
//! remaps or evicts them on demand, invalidating any view handed
//! out earlier. `JournalFile` therefore allows at most one window-backed object
//! view at a time via the in-use flag of its `GuardedCell`
//! (file/guarded_cell.rs, wrapping the `window_manager` field), and this
//! guard is the RAII piece that releases it:
//! `GuardedCell::with_guarded` raises the flag and returns a
//! `ValueGuard`, whose `Drop` clears the flag again. A
//! conflicting access returns `Err(JournalError::ValueGuardInUse)`
//! instead of racing or panicking.
//!
//! The flag is the entire protection; the guard adds no unsafe code. `Drop` runs
//! during unwinding too, so a panic while a guard is live still frees the slot.
//! Threading: `!Send` and `!Sync` for every `T` (the `&RefCell<bool>` field is
//! neither), so a guard never leaves its creating thread.
//!
//! Consumers (grep-verified): built only by `GuardedCell::with_guarded`
//! via `JournalFile::journal_object_ref` and
//! `journal_object_mut` plus their `*_ref`/`*_mut`
//! wrappers (file/file.rs); held across reader steps by
//! [`JournalReader`](crate::file::JournalReader) (file/reader.rs); mutated
//! through by `JournalWriter::add_data` and `object_added`
//! (file/writer.rs). Crate-internal: `mod value_guard;` is private
//! (file/mod.rs) and
//! lib.rs re-exports neither this type nor `GuardedCell`. Near-twin with different
//! trait plumbing: src/crates/jf/journal_file/src/value_guard.rs.
use std::cell::RefCell;
use std::ops::{Deref, DerefMut};

/// One live journal object plus the flag slot that keeps it exclusive.
///
/// `value` is the object — a view into an mmap window — whose validity is covered
/// by the raised in-use flag; `in_use_flag` is a shared reference to the
/// `GuardedCell`'s flag (file/guarded_cell.rs), cleared by `Drop`; `offset` is
/// caller metadata (the object offset, passed by `journal_object_ref`/
/// `journal_object_mut`) kept for retrieval
/// via `ValueGuard::offset()`.
///
/// `Deref`/`DerefMut` and the trait impls below expose the wrapped value directly,
/// so callers read and mutate through the guard — e.g. `data_guard.header.hash =
/// hash` and `set_payload` in `JournalWriter::add_data`.
#[derive(Debug)]
pub struct ValueGuard<'a, T> {
    offset: NonZeroU64,
    value: T,
    in_use_flag: &'a RefCell<bool>,
}

impl<'a, T> ValueGuard<'a, T> {
    /// Stores the three fields as given and does not touch the flag: the caller
    /// must already have raised it (`GuardedCell::with_guarded` does).
    pub fn new(offset: NonZeroU64, value: T, in_use_flag: &'a RefCell<bool>) -> Self {
        Self {
            offset,
            value,
            in_use_flag,
        }
    }

    /// The object offset stored at creation; the hash-bucket matcher returns it to
    /// identify the object that matched (`PayloadMatcher::visit` in
    /// `file/file.rs`).
    pub fn offset(&self) -> NonZeroU64 {
        self.offset
    }
}

impl<T> Deref for ValueGuard<'_, T> {
    type Target = T;

    fn deref(&self) -> &Self::Target {
        &self.value
    }
}

impl<T> DerefMut for ValueGuard<'_, T> {
    fn deref_mut(&mut self) -> &mut Self::Target {
        &mut self.value
    }
}

/// Releases the slot: clears the in-use flag unconditionally, including during
/// unwinding, so the cell is immediately reusable no matter how the guard ends.
/// A leaked (forgotten) guard instead keeps the flag raised, so every later
/// access fails with `ValueGuardInUse` until the guard is dropped.
impl<T> Drop for ValueGuard<'_, T> {
    fn drop(&mut self) {
        *self.in_use_flag.borrow_mut() = false;
    }
}

// Load-bearing import: `HashableObjectMut` is not among file/mod.rs's explicit
// re-exports and `mod object;` is private, so it
// resolves only through the crate-flat glob `pub(crate) use object::*`
// (file/mod.rs). Removing the glob breaks this file's compilation.
use crate::file::{HashableObject, HashableObjectMut};
use std::num::NonZeroU64;

/// Forwards all methods to the wrapped object so bucket visitors match through the
/// guard unchanged (the
/// [`BucketVisitor::visit`](crate::file::file::BucketVisitor::visit)
/// contract in `file/file.rs`).
impl<T: HashableObject> HashableObject for ValueGuard<'_, T> {
    fn hash(&self) -> u64 {
        self.value.hash()
    }

    fn raw_payload(&self) -> &[u8] {
        self.value.raw_payload()
    }

    fn is_compressed(&self) -> bool {
        self.value.is_compressed()
    }

    fn decompress(&self, buf: &mut Vec<u8>) -> crate::error::Result<usize> {
        self.value.decompress(buf)
    }

    fn next_hash_offset(&self) -> Option<NonZeroU64> {
        self.value.next_hash_offset()
    }

    fn object_type() -> super::object::ObjectType {
        T::object_type()
    }
}

/// Same forwarding for the mutation methods (`set_next_hash_offset`,
/// `set_payload`), letting writers update the object through the guard.
impl<T: HashableObjectMut> HashableObjectMut for ValueGuard<'_, T> {
    fn set_next_hash_offset(&mut self, offset: NonZeroU64) {
        self.value.set_next_hash_offset(offset);
    }

    fn set_payload(&mut self, data: &[u8]) {
        self.value.set_payload(data);
    }
}
