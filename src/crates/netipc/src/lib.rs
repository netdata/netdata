// netipc - Netdata Plugin IPC Library (Rust)

pub mod protocol;
pub mod service;
pub mod transport;

#[cfg(all(test, unix))]
mod test_locks;
