//! L1 transport backends.
//!
//! Each transport provides connection lifecycle, handshake, and
//! send/receive with transparent chunking.

#[cfg(unix)]
pub mod posix;

#[cfg(target_os = "linux")]
pub mod shm;

#[cfg(windows)]
pub mod windows;

#[cfg(windows)]
pub mod win_shm;
