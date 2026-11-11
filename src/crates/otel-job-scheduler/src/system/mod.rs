//! What the scheduler asks the system it runs on.
//!
//! Two questions, for the memory budget: how much memory is free, and how
//! much a running child holds. The answers depend on the system, so each
//! system has its implementation in a module of its own and [`Host`] picks
//! the one this build is for. A system without one answers `None`, and the
//! scheduler then runs without a memory budget.
//!
//! And one question about a child that has ended: which signal ended it
//! ([`ending_signal`]). A system without signals answers `None`.

use std::process::ExitStatus;

#[cfg(target_os = "linux")]
mod linux;

/// The system a scheduler runs its jobs on.
pub(crate) trait System: Send + 'static {
    /// The memory that new work can take without the system having to swap
    /// or to kill, in bytes: what is free for this process, within the limits
    /// set on it. `None` when it cannot be read.
    fn free_memory(&self) -> Option<u64>;

    /// The memory that process `pid` holds now, in bytes. `None` when it
    /// cannot be read, or the process is gone.
    fn resident_memory(&self, pid: u32) -> Option<u64>;
}

/// The system this process runs on.
pub(crate) struct Host;

impl System for Host {
    fn free_memory(&self) -> Option<u64> {
        #[cfg(target_os = "linux")]
        return linux::free_memory();

        #[cfg(not(target_os = "linux"))]
        None
    }

    fn resident_memory(&self, pid: u32) -> Option<u64> {
        #[cfg(target_os = "linux")]
        return linux::resident_memory(pid);

        #[cfg(not(target_os = "linux"))]
        {
            let _ = pid;
            None
        }
    }
}

/// The signal that ended the child whose wait status is `status`. `None`
/// when no signal did, and on a system that has no signals.
pub(crate) fn ending_signal(status: &ExitStatus) -> Option<i32> {
    #[cfg(unix)]
    {
        use std::os::unix::process::ExitStatusExt;

        status.signal()
    }

    #[cfg(not(unix))]
    {
        let _ = status;
        None
    }
}
