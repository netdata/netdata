//! Shared update state for a registered chart.
//!
//! [`ChartHandle`] is the cell holding a chart's current value between sample
//! ticks: plugin code writes it from any task or thread, and the chart sampler
//! in `registry` reads and clones it on every tick for emission.

use parking_lot::RwLock;
use std::sync::Arc;

/// Shared, clonable cell holding the current value of one registered chart.
///
/// All clones wrap the same `Arc<RwLock<T>>` (parking_lot), so the value can be
/// updated from any task or thread. `T: Send + Sync + 'static` is required by
/// chart registration, which is also the only way to obtain a handle
/// (`PluginRuntime::register_chart` / `register_instanced_chart`, delegating to
/// `ChartRegistry`).
///
/// The registry keeps its own clone, so dropping the last plugin handle does
/// not stop emission: the chart keeps publishing the last written value until
/// shutdown. Locking a handle while holding one of its guards can deadlock:
/// write locks are not reentrant, and a queued writer makes even a recursive
/// read block (parking_lot's task-fair policy).
#[derive(Clone)]
pub struct ChartHandle<T> {
    pub(crate) data: Arc<RwLock<T>>,
}

impl<T> ChartHandle<T> {
    /// Create a handle with the given initial value.
    ///
    /// Called only by `ChartRegistry::register_chart` /
    /// `register_instanced_chart`; plugins receive handles from registration.
    pub(crate) fn new(initial: T) -> Self {
        Self {
            data: Arc::new(RwLock::new(initial)),
        }
    }

    /// Update the chart value by running `f` with exclusive access.
    ///
    /// Runs the closure while holding the write lock. The new value is emitted
    /// on the chart's next sample tick.
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

    /// Get a write lock to the chart data for direct mutable access.
    ///
    /// Unlike `update`, the guard outlives the call. The sampler blocks on its
    /// per-tick read until the guard is dropped, so holding it across an await
    /// point or past a tick stalls emission for every chart in the same
    /// registry batch (`ChartRegistry::run` groups samplers into batches of up
    /// to 1000 and samples them serially).
    pub fn write(&self) -> parking_lot::RwLockWriteGuard<'_, T> {
        self.data.write()
    }

    /// Get a read lock to the chart data for inspection.
    pub fn read(&self) -> parking_lot::RwLockReadGuard<'_, T> {
        self.data.read()
    }
}

/// Formats the chart value under a read lock; calling this while the calling
/// thread holds a write guard on the same handle deadlocks.
impl<T: std::fmt::Debug> std::fmt::Debug for ChartHandle<T> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ChartHandle")
            .field("data", &*self.data.read())
            .finish()
    }
}
