use std::cell::RefCell;
use std::ops::{Deref, DerefMut};

/// A guard that ensures exclusive access to objects obtained from a shared memory window.
///
/// # Purpose
///
/// `ValueGuard` enforces that only one journal object can be accessed at a time.
/// This is necessary because:
///
/// 1. The underlying window manager uses a limited number of memory-mapped windows
///    that are shared between all objects.
/// 2. Creating a new object could invalidate memory references held by previously
///    created objects, even though the objects themselves are immutable.
///
/// # Usage
///
/// When a `ValueGuard<T>` is returned from methods like `data_object()`, it provides
/// read-only access to the underlying object via the `Deref` trait. When the guard
/// is dropped (goes out of scope), it automatically releases the lock, allowing new
/// objects to be created.
///
/// # Safety and Interior Mutability
///
/// Although the returned objects are immutable, the window manager that provides
/// the memory they reference uses interior mutability (via `UnsafeCell`) to reuse
/// memory-mapped regions. This guard ensures that objects are not accessed after
/// their underlying memory might have been repurposed.
#[derive(Debug)]
pub struct ValueGuard<'a, T> {
    value: T,
    in_use_flag: &'a RefCell<bool>,
}

impl<'a, T> ValueGuard<'a, T> {
    pub fn new(value: T, in_use_flag: &'a RefCell<bool>) -> Self {
        Self { value, in_use_flag }
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

impl<T> Drop for ValueGuard<'_, T> {
    fn drop(&mut self) {
        *self.in_use_flag.borrow_mut() = false;
    }
}
