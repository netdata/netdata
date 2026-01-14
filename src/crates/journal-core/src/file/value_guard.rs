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
    offset: NonZeroU64,
    value: T,
    in_use_flag: &'a RefCell<bool>,
}

impl<'a, T> ValueGuard<'a, T> {
    pub fn new(offset: NonZeroU64, value: T, in_use_flag: &'a RefCell<bool>) -> Self {
        Self {
            offset,
            value,
            in_use_flag,
        }
    }

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

impl<T> Drop for ValueGuard<'_, T> {
    fn drop(&mut self) {
        *self.in_use_flag.borrow_mut() = false;
    }
}

use crate::file::{HashableObject, HashableObjectMut};
use std::num::NonZeroU64;

impl<T: HashableObject> HashableObject for ValueGuard<'_, T> {
    fn hash(&self) -> u64 {
        self.value.hash()
    }

    fn get_payload(&self) -> &[u8] {
        self.value.get_payload()
    }

    fn next_hash_offset(&self) -> Option<NonZeroU64> {
        self.value.next_hash_offset()
    }

    fn object_type() -> super::object::ObjectType {
        T::object_type()
    }
}

impl<T: HashableObjectMut> HashableObjectMut for ValueGuard<'_, T> {
    fn set_next_hash_offset(&mut self, offset: NonZeroU64) {
        self.value.set_next_hash_offset(offset);
    }

    fn set_payload(&mut self, data: &[u8]) {
        self.value.set_payload(data);
    }
}
