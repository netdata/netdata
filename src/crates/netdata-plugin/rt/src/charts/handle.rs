//! Chart handle for updating metric values.

use parking_lot::RwLock;
use std::sync::Arc;

/// A handle to a chart that allows updating its values.
///
/// Multiple handles can exist for the same chart (they share the underlying data via Arc).
/// This allows different parts of your code to update the same chart independently.
#[derive(Clone)]
pub struct ChartHandle<T> {
    pub(crate) data: Arc<RwLock<T>>,
}

impl<T> ChartHandle<T> {
    /// Create a new chart handle with the given initial value
    pub(crate) fn new(initial: T) -> Self {
        Self {
            data: Arc::new(RwLock::new(initial)),
        }
    }

    /// Update the chart data using a closure
    ///
    /// # Example
    ///
    /// ```ignore
    /// handle.update(|metrics| {
    ///     metrics.user = 42;
    ///     metrics.system = 13;
    /// });
    /// ```
    pub fn update<F>(&self, f: F)
    where
        F: FnOnce(&mut T),
    {
        let mut guard = self.data.write();
        f(&mut *guard);
    }

    /// Get a write lock to the chart data
    ///
    /// This allows direct mutable access to the data. Remember to drop
    /// the guard when you're done to release the lock.
    ///
    /// # Example
    ///
    /// ```ignore
    /// {
    ///     let mut guard = handle.write();
    ///     guard.user = 42;
    ///     guard.system = 13;
    /// } // Lock released here
    /// ```
    pub fn write(&self) -> parking_lot::RwLockWriteGuard<'_, T> {
        self.data.write()
    }

    /// Get a read lock to the chart data
    pub fn read(&self) -> parking_lot::RwLockReadGuard<'_, T> {
        self.data.read()
    }
}

impl<T: std::fmt::Debug> std::fmt::Debug for ChartHandle<T> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ChartHandle")
            .field("data", &*self.data.read())
            .finish()
    }
}
