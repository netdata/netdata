// SPDX-License-Identifier: GPL-3.0-or-later
//
// Sample iterator wrapping `nd_pds_samples`.

use std::marker::PhantomData;
use std::ptr::NonNull;

use super::raw;

/// Storage-side staleness marker. The shim surfaces `SN_FLAG_*` bits via
/// the `flags_out` parameter; bits we care about are exposed here.
pub(crate) mod flags {
    // Storage-side flags surfaced through the shim's flags_out parameter.
    // These are Rust-internal constants; not exported through the C ABI.
    #[allow(dead_code)]
    pub(crate) const STALE: u32 = 0x01;
    #[allow(dead_code)]
    pub(crate) const RESET: u32 = 0x02;
}

/// One stored sample as projected by the shim.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Sample {
    pub timestamp_ms: i64,
    pub value: f64,
    pub flags: u32,
}

/// Sample iterator. Bound to its parent query's lifetime; the shim
/// guarantees the underlying handle stays valid until `close`.
pub struct NdSamples<'q> {
    handle: NonNull<raw::nd_pds_samples>,
    _marker: PhantomData<&'q ()>,
}

impl<'q> NdSamples<'q> {
    /// Construct from a raw handle. Caller is responsible for ensuring the
    /// handle was just returned by `nd_pds_open_samples` and not yet closed.
    pub(crate) unsafe fn from_raw(handle: NonNull<raw::nd_pds_samples>) -> Self {
        Self {
            handle,
            _marker: PhantomData,
        }
    }
}

impl Iterator for NdSamples<'_> {
    type Item = Sample;

    fn next(&mut self) -> Option<Self::Item> {
        let mut ts: i64 = 0;
        let mut value: f64 = 0.0;
        let mut flags: u32 = 0;
        let r = unsafe {
            raw::nd_pds_samples_next(self.handle.as_ptr(), &mut ts, &mut value, &mut flags)
        };
        if r > 0 {
            Some(Sample {
                timestamp_ms: ts,
                value,
                flags,
            })
        } else {
            // r == 0 -> EOF; r < 0 -> error. We treat both as end-of-stream;
            // error surfacing happens in the storage adapter layer if
            // distinguishing them matters.
            None
        }
    }
}

impl Drop for NdSamples<'_> {
    fn drop(&mut self) {
        unsafe { raw::nd_pds_samples_close(self.handle.as_ptr()) }
    }
}

// Safety: the handle is movable across threads. Concurrent calls into the
// shim against the same handle are not safe; the borrow checker enforces
// `&mut self` access for `next`.
unsafe impl Send for NdSamples<'_> {}
