//! SIGBUS recovery for the journal mmap layer. Read-only maps are created
//! without checking their range against the file length
//! ([`MemoryMap::create`](crate::file::mmap::MemoryMap::create)
//! for [`Mmap`](crate::file::mmap::Mmap); only the
//! [`MmapMut`](crate::file::mmap::MmapMut) impl pre-extends the file
//! first), so a reader touching a page past EOF faults on an
//! unbacked page and, by default, SIGBUS kills the process. This handler
//! turns that fault into reading zeros so the reader continues.
//!
//! Mechanism: `sigbus_handler` page-aligns the faulting address reported in
//! siginfo and maps a fresh anonymous read-only page over it, then returns;
//! the kernel re-executes the interrupted instruction against the new
//! zero-filled page and the access succeeds - the mmap layer neither retries
//! nor reports an error, and the handler never longjmps. Any SIGBUS gets
//! this treatment: the handler does not check where the fault came from.
//!
//! Signal-handler contract: the handler only reads the siginfo, performs one
//! mmap(2) syscall and one Relaxed atomic store - no allocation, locks,
//! stdio or logging. Relaxed ordering is fine: nothing synchronizes on this
//! flag. mmap(2) is not on POSIX's async-signal-safe list
//! (signal-safety(7)); the direct syscall path is what makes this work, not
//! a POSIX guarantee. The 4 KiB page size is hardcoded, matching `PAGE_SIZE`
//! in file/mmap.rs.
//!
//! Deployment (grep-verified): nothing in this repo installs this copy or
//! calls [`signalled`]; lib.rs re-exports [`install_handler`] as
//! [`install_sigbus_handler`](crate::install_sigbus_handler). The live
//! installers are the twins - netflow-plugin
//! installs the published systemd-journal-sdk-core 0.8.1 copy
//! (src/crates/Cargo.toml; fatal on error,
//! netflow-plugin/src/main.rs), and jf/journal_reader_ffi installs the
//! jf twin src/crates/jf/sigbus (same handler; non-fatal,
//! jf/journal_reader_ffi/src/lib.rs). The published twin adds
//! #[cfg(unix)] guards and a MAP_ANON fallback.
#![allow(dead_code)]

use crate::error::{JournalError, Result};
use std::sync::OnceLock;
use std::sync::atomic::{AtomicBool, Ordering};

// Handler state: the sticky SIGBUS_OCCURRED flag (read via signalled()) and
// the cached sigaction(2) return code that makes installation at-most-once.
static SIGBUS_OCCURRED: AtomicBool = AtomicBool::new(false);
static HANDLER_INSTALLED: OnceLock<i32> = OnceLock::new();

// Signal dispositions are process-wide: installing this handler affects
// every thread and replaces any prior SIGBUS handler without saving it.
extern "C" fn sigbus_handler(
    _sig: libc::c_int,
    info: *mut libc::siginfo_t,
    _ucontext: *mut libc::c_void,
) {
    unsafe {
        let si = &*info;
        let fault_addr = si.si_addr();

        // MAP_FIXED over the faulting page; the return value is ignored, so
        // a failed remap leaves the page unbacked and the retried access
        // faults straight back into this handler.
        let page_addr = (fault_addr as usize & !(4096 - 1)) as *mut libc::c_void;
        libc::mmap(
            page_addr,
            4096,
            libc::PROT_READ,
            libc::MAP_PRIVATE | libc::MAP_ANONYMOUS | libc::MAP_FIXED,
            -1,
            0,
        );

        SIGBUS_OCCURRED.store(true, Ordering::Relaxed);
    }
}

// Sticky, never cleared: true once any SIGBUS has been recovered in this
// process.
pub fn signalled() -> bool {
    SIGBUS_OCCURRED.load(Ordering::Relaxed)
}

// First call runs the sigaction(2) and caches its return code; later calls
// replay the cache without touching the kernel. -1 surfaces as
// JournalError::SigbusHandlerError (error.rs).
pub fn install_handler() -> Result<()> {
    let rc = HANDLER_INSTALLED.get_or_init(|| unsafe {
        let mut sa: libc::sigaction = std::mem::zeroed();

        sa.sa_flags = libc::SA_SIGINFO;
        sa.sa_sigaction = sigbus_handler as *const () as usize;

        libc::sigaction(libc::SIGBUS, &sa, std::ptr::null_mut())
    });

    match rc {
        -1 => Err(JournalError::SigbusHandlerError),
        _ => Ok(()),
    }
}
