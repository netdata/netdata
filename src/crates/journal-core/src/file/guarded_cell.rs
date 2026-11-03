//! Guarded interior mutability for one-at-a-time window access.
//!
//! [`GuardedCell`] pairs a value with a `RefCell<bool>` in-use flag. It hands
//! out `&mut T` from `&self` only while the flag is clear; the caller raises
//! the flag while the returned reference (or anything derived from it) is
//! live and clears it when done - normally through the RAII `ValueGuard`
//! (file/value_guard.rs), whose `Drop` clears it. A conflicting borrow fails
//! with [`JournalError::ValueGuardInUse`] (produced by the borrow checks in
//! [`GuardedCell::borrow_mut_checked`] and [`GuardedCell::with_guarded`])
//! instead of racing or panicking.
//!
//! The only consumer is `JournalFile`'s `window_manager` field
//! (`file/file.rs`): the window manager (file/mmap.rs) remaps its windows on
//! demand and thereby invalidates slices handed out earlier, so at most one
//! window-backed object view may be live at a time. The module is private
//! (`file/mod.rs` declares it without `pub`) and neither `GuardedCell` nor
//! `ValueGuard` is re-exported from lib.rs.
//!
//! Threading: `!Sync` for every `T` (`UnsafeCell` and `RefCell` are both
//! `!Sync`); `Send` follows `T: Send`. Since `JournalFile` holds this cell,
//! `&JournalFile` cannot cross threads, so `ValueGuardInUse` only ever
//! reports same-thread conflicts (e.g. a nested object access).
use crate::error::{JournalError, Result};
use crate::file::value_guard::ValueGuard;
use std::cell::{RefCell, UnsafeCell};
use std::num::NonZeroU64;

/// Interior mutability with a built-in one-borrow gate: returns `&mut T` from
/// `&self`, but at most one borrow at a time.
///
/// `RefCell` alone cannot express this: its `RefMut` must be dropped before
/// the borrowing call returns, while here the reference - or data derived
/// from it, such as a slice into an mmap window - must outlive the call.
/// Instead the cell carries an in-use flag that the caller manages: check
/// that it is clear, take the borrow, raise the flag while the reference is
/// live, clear it when done. `with_guarded` and the `ValueGuard` it returns
/// automate the last two steps.
///
/// The flag is a protocol, not type-level enforcement:
/// [`GuardedCell::borrow_mut_checked`] only checks it and never raises it,
/// so a caller that returns references without raising the flag (the
/// object-header helpers
/// [`JournalFile::object_header_ref`](crate::file::JournalFile::object_header_ref)
/// and `object_header_mut` in `file/file.rs`) bypasses the gate. In-crate
/// callers should prefer [`GuardedCell::with_guarded`], which runs the full
/// protocol and returns a `ValueGuard`.
pub struct GuardedCell<T> {
    value: UnsafeCell<T>,
    guard: RefCell<bool>,
}

impl<T> GuardedCell<T> {
    /// Creates a cell holding `value`, with the flag clear (not in use).
    pub fn new(value: T) -> Self {
        Self {
            value: UnsafeCell::new(value),
            guard: RefCell::new(false),
        }
    }

    /// The in-use flag, exposed so an external RAII guard can clear it:
    /// `ValueGuard` stores exactly such a `&RefCell<bool>` and clears it on
    /// drop. `with_guarded` passes `&self.guard` directly, so today only the
    /// tests use this accessor.
    #[allow(dead_code)]
    pub fn guard(&self) -> &RefCell<bool> {
        &self.guard
    }

    /// Returns `&mut T` from `&self` after verifying the flag is clear.
    ///
    /// The cell does not raise the flag. The caller must raise it before the
    /// returned reference (or anything derived from it) is used, keep it
    /// raised while that reference is live, and clear it afterwards - the
    /// contract `ValueGuard`'s `Drop` implements. While the flag is clear,
    /// no later borrow is blocked, so this escape hatch fits only immediate,
    /// scope-bound uses like the header read in
    /// [`JournalFile::object_header_ref`](crate::file::JournalFile::object_header_ref)
    /// and header writes in `object_header_mut`.
    ///
    /// Returns `Err(JournalError::ValueGuardInUse)` when the flag is raised.
    #[allow(clippy::mut_from_ref)]
    pub fn borrow_mut_checked(&self) -> Result<&mut T> {
        let is_in_use = self.guard.borrow();
        if *is_in_use {
            return Err(JournalError::ValueGuardInUse);
        }
        drop(is_in_use);

        // SAFETY: the flag was just verified clear, so no guarded borrow is
        // live. The caller must raise the flag before the reference escapes
        // and keep it raised while it (or anything derived from it) is used;
        // `RefCell` guards cannot express a reference that outlives this
        // call, hence the manual protocol.
        unsafe { Ok(&mut *self.value.get()) }
    }

    /// Mutable access through `&mut self`: the borrow checker already
    /// guarantees uniqueness, so the flag is irrelevant here.
    pub fn get_mut(&mut self) -> &mut T {
        self.value.get_mut()
    }

    /// Consumes the cell and returns the value; the flag dies with it.
    #[allow(dead_code)]
    pub fn into_inner(self) -> T {
        self.value.into_inner()
    }

    /// Runs `f` with `&mut T` and wraps its result in a `ValueGuard` - the
    /// intended entry point, automating the full protocol.
    ///
    /// Steps: verify the flag is clear, run `f` with `&mut T`, raise the
    /// flag, return `ValueGuard::new(offset, result, &self.guard)` whose
    /// `Drop` clears the flag. On any error the flag stays clear, so the
    /// cell is immediately reusable. A conflicting call returns
    /// `Err(JournalError::ValueGuardInUse)`; re-entering the cell from
    /// inside `f` panics instead (the flag's `RefMut` is still held).
    ///
    /// `offset` is caller-defined metadata stored in the guard for later
    /// retrieval via `ValueGuard::offset`; `JournalFile` passes the object
    /// offset (from `journal_object_ref`/`journal_object_mut`) so readers
    /// can identify the guarded object.
    ///
    /// ```ignore
    /// let object = self.window_manager.with_guarded(offset, |wm| {
    ///     let slice = wm.get_slice(offset.get(), size)?;
    ///     DataObject::ref_from_bytes(slice).map_err(|_| JournalError::ZerocopyFailure)
    /// })?;
    /// // `object` derefs to the parsed value; dropping it clears the flag.
    /// ```
    pub fn with_guarded<'a, R, F>(&'a self, offset: NonZeroU64, f: F) -> Result<ValueGuard<'a, R>>
    where
        F: FnOnce(&'a mut T) -> Result<R>,
    {
        let mut is_in_use = self.guard.borrow_mut();
        if *is_in_use {
            return Err(JournalError::ValueGuardInUse);
        }

        // SAFETY: the flag is clear, so no guarded borrow is live; `f` gets
        // temporary `&mut T` and the flag is raised before the `ValueGuard`
        // (which clears it on drop) escapes.
        let value_ref = unsafe { &mut *self.value.get() };

        let result = f(value_ref)?;

        *is_in_use = true;

        Ok(ValueGuard::new(offset, result, &self.guard))
    }
}


#[cfg(test)]
mod tests {
    use super::*;

    struct TestData {
        value: Vec<u8>,
    }

    impl TestData {
        fn get_slice(&mut self, start: usize, len: usize) -> &[u8] {
            &self.value[start..start + len]
        }
    }

    #[test]
    fn test_basic_borrow() {
        let cell = GuardedCell::new(TestData {
            value: vec![1, 2, 3, 4, 5],
        });

        // First cycle: check, borrow, use the slice, raise the flag - the
        // caller side of borrow_mut_checked's contract.
        {
            // Scoped so the flag borrow is released before the block's later
            // `borrow_mut()`: that call panics while any borrow is held.
            {
                let is_in_use = cell.guard().borrow();
                assert!(!*is_in_use);
            }

            let data = cell.borrow_mut_checked().unwrap();
            let slice = data.get_slice(0, 3);
            assert_eq!(slice, &[1, 2, 3]);

            *cell.guard().borrow_mut() = true;
        }

        // Clear the flag (as ValueGuard's Drop would), making the cell
        // reusable.
        *cell.guard().borrow_mut() = false;

        // Second cycle succeeds once the flag is clear.
        {
            {
                let is_in_use = cell.guard().borrow();
                assert!(!*is_in_use);
            }

            let data = cell.borrow_mut_checked().unwrap();
            let slice = data.get_slice(2, 3);
            assert_eq!(slice, &[3, 4, 5]);

            *cell.guard().borrow_mut() = true;
        }
    }

    #[test]
    fn test_guard_prevents_double_borrow() {
        let cell = GuardedCell::new(TestData {
            value: vec![1, 2, 3],
        });

        // Simulate an active borrow by raising the flag directly.
        *cell.guard().borrow_mut() = true;

        // The conflicting borrow fails with ValueGuardInUse.
        let result = cell.borrow_mut_checked();
        assert!(matches!(result, Err(JournalError::ValueGuardInUse)));
    }

    #[test]
    fn test_get_mut() {
        let mut cell = GuardedCell::new(TestData {
            value: vec![1, 2, 3],
        });

        let data = cell.get_mut();
        data.value.push(4);
        assert_eq!(data.value, vec![1, 2, 3, 4]);
    }

    #[test]
    fn test_into_inner() {
        let cell = GuardedCell::new(TestData {
            value: vec![1, 2, 3],
        });

        let data = cell.into_inner();
        assert_eq!(data.value, vec![1, 2, 3]);
    }
}
