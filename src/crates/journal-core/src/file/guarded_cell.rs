use crate::error::{JournalError, Result};
use crate::file::value_guard::ValueGuard;
use std::cell::{RefCell, UnsafeCell};
use std::num::NonZeroU64;

/// A cell type that provides interior mutability with integrated guard-based exclusion.
///
/// # Purpose
///
/// `GuardedCell` is designed for situations where:
/// - You need interior mutability (get `&mut T` from `&self`)
/// - References to the inner data must outlive the borrowing function scope
/// - You need to ensure only one "logical borrow" is active at a time
/// - `RefCell` cannot be used because `RefMut` guards must be dropped before returning
///
/// # Design
///
/// `GuardedCell` owns both the value and a `RefCell<bool>` guard flag. When borrowing,
/// it checks that the guard is clear (no active borrow), then returns a mutable reference.
/// The caller is responsible for managing the guard's lifecycle, typically via a separate
/// RAII type (like `ValueGuard`) that sets the flag and clears it on drop.
///
/// # Example Use Case
///
/// This type is used for the `WindowManager` in journal files:
/// - The window manager maps/unmaps memory windows dynamically
/// - Methods return slices into these windows that must outlive the borrow scope
/// - Only one object slice can be active at a time (enforced by the guard)
/// - A `ValueGuard` RAII type manages the guard lifecycle
///
/// # Safety Model
///
/// The safety relies on two cooperating parts:
/// 1. `GuardedCell` - Owns the guard and checks it before borrowing
/// 2. External RAII guard (like `ValueGuard`) - Manages the guard flag lifecycle
///
/// The contract is:
/// - Before calling `borrow_mut_checked()`, the guard must be `false`
/// - After getting a reference, the caller must set the guard to `true`
/// - The guard must stay `true` while any reference derived from the borrow is live
/// - When all references are done, the guard must be set back to `false`
///
/// # Example
///
/// ```ignore
/// struct Container {
///     window_manager: GuardedCell<WindowManager>,
/// }
///
/// impl Container {
///     fn get_slice(&self, offset: u64, size: u64) -> Result<ValueGuard<&[u8]>> {
///         // Check if guard is already held
///         let mut is_in_use = self.window_manager.guard().borrow_mut();
///         if *is_in_use {
///             return Err(Error::AlreadyBorrowed);
///         }
///
///         // Borrow the window manager
///         let wm = self.window_manager.borrow_mut_checked()?;
///         let slice = wm.get_slice(offset, size)?;
///
///         // Set guard and return RAII guard that clears it on drop
///         *is_in_use = true;
///         Ok(ValueGuard::new(slice, self.window_manager.guard()))
///     }
/// }
/// ```
pub struct GuardedCell<T> {
    value: UnsafeCell<T>,
    guard: RefCell<bool>,
}

impl<T> GuardedCell<T> {
    /// Creates a new `GuardedCell` containing the given value.
    ///
    /// The guard is initialized to `false` (not in use).
    pub fn new(value: T) -> Self {
        Self {
            value: UnsafeCell::new(value),
            guard: RefCell::new(false),
        }
    }

    /// Returns a reference to the guard flag.
    ///
    /// This allows external RAII types (like `ValueGuard`) to manage the guard's
    /// lifecycle by setting it to `true` when borrowing and `false` when done.
    #[allow(dead_code)]
    pub fn guard(&self) -> &RefCell<bool> {
        &self.guard
    }

    /// Attempts to borrow the inner value mutably.
    ///
    /// # Returns
    ///
    /// - `Ok(&mut T)` if the guard is not currently held
    /// - `Err(JournalError::ValueGuardInUse)` if the guard is held (another borrow is active)
    ///
    /// # Safety Contract
    ///
    /// After calling this method successfully, the caller MUST:
    /// 1. Set the guard to `true` (via `guard().borrow_mut()`) before using the returned reference
    /// 2. Keep the guard `true` for the entire lifetime of the returned reference
    /// 3. Set the guard back to `false` when the reference is no longer needed
    ///
    /// This is typically enforced via a RAII guard type that manages the flag automatically.
    ///
    /// # Example
    ///
    /// ```ignore
    /// let cell = GuardedCell::new(WindowManager::new(...));
    ///
    /// // Check and borrow
    /// let mut is_in_use = cell.guard().borrow_mut();
    /// if *is_in_use {
    ///     return Err(JournalError::ValueGuardInUse);
    /// }
    ///
    /// let wm = cell.borrow_mut_checked()?;
    /// let slice = wm.get_slice(offset, size)?;
    ///
    /// // Set guard before using the reference
    /// *is_in_use = true;
    ///
    /// // Use slice...
    /// // (guard will be cleared by RAII when done)
    /// ```
    #[allow(clippy::mut_from_ref)]
    pub fn borrow_mut_checked(&self) -> Result<&mut T> {
        // Check the guard to ensure no other borrow is active
        let is_in_use = self.guard.borrow();
        if *is_in_use {
            return Err(JournalError::ValueGuardInUse);
        }
        drop(is_in_use);

        // SAFETY: We've verified via the guard that no other mutable reference exists.
        // The caller is responsible for:
        // 1. Setting the guard to true before using the returned reference
        // 2. Keeping the guard true while the reference (or data derived from it) is live
        // 3. Clearing the guard when done (typically via RAII Drop)
        //
        // This manual lifetime management is necessary because:
        // - We need references that outlive this function's scope
        // - RefCell's RefMut cannot express this pattern (it must drop before returning)
        // - The guard flag provides the runtime safety check for exclusive access
        unsafe { Ok(&mut *self.value.get()) }
    }

    /// Gets a mutable reference to the inner value.
    ///
    /// This is safe because it requires `&mut self`, guaranteeing unique access.
    /// No guard checking is needed.
    pub fn get_mut(&mut self) -> &mut T {
        self.value.get_mut()
    }

    /// Consumes the cell and returns the inner value.
    #[allow(dead_code)]
    pub fn into_inner(self) -> T {
        self.value.into_inner()
    }

    /// Executes a closure with mutable access to the inner value and wraps the result in a `ValueGuard`.
    ///
    /// This is the primary method for working with `GuardedCell` in a safe, ergonomic way.
    /// It handles all guard management automatically:
    /// 1. Checks that the guard is not currently held
    /// 2. Provides mutable access to the inner value via the closure
    /// 3. Sets the guard flag
    /// 4. Wraps the closure's result in a `ValueGuard` that will clear the flag on drop
    ///
    /// # Parameters
    ///
    /// - `offset`: Domain-specific metadata (e.g., file offset for journal objects) that will be
    ///   stored in the `ValueGuard` for later retrieval
    /// - `f`: A closure that takes mutable access to `T` and returns a `Result<R>` where `R` is
    ///   the value to be wrapped in the guard
    ///
    /// # Returns
    ///
    /// - `Ok(ValueGuard<R>)` on success, which automatically manages the guard lifecycle
    /// - `Err(JournalError::ValueGuardInUse)` if the guard is already held
    /// - `Err(...)` if the closure returns an error
    ///
    /// # Example
    ///
    /// ```ignore
    /// let window_manager = GuardedCell::new(WindowManager::new(...));
    ///
    /// let object_guard = window_manager.with_guarded(object_offset, |wm| {
    ///     // Access window manager mutably
    ///     let slice = wm.get_slice(offset, size)?;
    ///     let object = parse_object(slice)?;
    ///     Ok(object)
    /// })?;
    ///
    /// // object_guard automatically clears the guard when dropped
    /// ```
    ///
    /// # Safety
    ///
    /// This method encapsulates all the safety requirements:
    /// - Guard checking is done automatically
    /// - The guard is set before the `ValueGuard` is returned
    /// - The guard is cleared automatically when `ValueGuard` is dropped
    /// - No `unsafe` code is exposed to the caller
    pub fn with_guarded<'a, R, F>(&'a self, offset: NonZeroU64, f: F) -> Result<ValueGuard<'a, R>>
    where
        F: FnOnce(&'a mut T) -> Result<R>,
    {
        // Check if the guard is already held
        let mut is_in_use = self.guard.borrow_mut();
        if *is_in_use {
            return Err(JournalError::ValueGuardInUse);
        }

        // SAFETY: We've verified via the guard that no other mutable reference exists.
        // The closure gets temporary mutable access, but we ensure the guard is set
        // before returning the ValueGuard.
        let value_ref = unsafe { &mut *self.value.get() };

        // Execute the user's closure to extract/create the result value
        let result = f(value_ref)?;

        // Mark the guard as in use
        *is_in_use = true;

        // Return a ValueGuard that will automatically clear the guard on drop
        Ok(ValueGuard::new(offset, result, &self.guard))
    }
}

// GuardedCell is NOT Send or Sync by default (inherited from UnsafeCell).
// This is correct for single-threaded use in journal file reading.

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

        // First borrow
        {
            // Check guard is not in use
            {
                let is_in_use = cell.guard().borrow();
                assert!(!*is_in_use);
            }

            // Borrow the data
            let data = cell.borrow_mut_checked().unwrap();
            let slice = data.get_slice(0, 3);
            assert_eq!(slice, &[1, 2, 3]);

            // Mark as in use
            *cell.guard().borrow_mut() = true;
        }

        // Clear guard
        *cell.guard().borrow_mut() = false;

        // Second borrow after clearing
        {
            // Check guard is not in use
            {
                let is_in_use = cell.guard().borrow();
                assert!(!*is_in_use);
            }

            // Borrow the data
            let data = cell.borrow_mut_checked().unwrap();
            let slice = data.get_slice(2, 3);
            assert_eq!(slice, &[3, 4, 5]);

            // Mark as in use
            *cell.guard().borrow_mut() = true;
        }
    }

    #[test]
    fn test_guard_prevents_double_borrow() {
        let cell = GuardedCell::new(TestData {
            value: vec![1, 2, 3],
        });

        // Set guard to true (simulating active borrow)
        *cell.guard().borrow_mut() = true;

        // Attempt to borrow while guard is held should fail
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
