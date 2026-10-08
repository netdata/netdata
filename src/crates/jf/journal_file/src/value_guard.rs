use std::cell::RefCell;
use std::ops::{Deref, DerefMut};

/// A guard that ensures exclusive access to objects obtained from a shared memory window.
///
/// # Purpose
///
/// `ValueGuard` enforces that only one journal object can be accessed at a time.
/// This is necessary because:
///
/// 1. The underlying window manager keeps a limited number of memory-mapped windows
///    that are shared between all objects.
/// 2. Accessing a new object can remap or evict a shared window, invalidating the
///    references held by previously created objects.
///
/// # Usage
///
/// Reader methods such as `JournalFile::data_ref()` and writer methods such as
/// `JournalFile::data_mut()` return a `ValueGuard<T>` that derefs to the object via the
/// `Deref`/`DerefMut` traits. While a guard is alive, any other object access fails with
/// `JournalError::ValueGuardInUse`; dropping the guard clears the in-use flag.
///
/// # Safety and Interior Mutability
///
/// `JournalFile` wraps the window manager in an `UnsafeCell` so its `&self` methods can
/// remap windows. This guard makes guarded object access safe: those accessors check the
/// in-use flag before touching any window, so a live object view cannot be invalidated
/// through them. Internal helpers such as `object_header_ref` bypass that check and can
/// still remap or evict windows while a view is live.
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

use crate::{HashableObject, HashableObjectMut};
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

    fn object_type() -> crate::ObjectType {
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
