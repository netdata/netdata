pub(super) const SERVER_POLL_TIMEOUT_MS: u32 = 100;
/// How long a stopping server waits for session workers to finish their
/// current message before forcing them out of blocking I/O.
pub(super) const SERVER_STOP_GRACE_MS: u64 = 1_000;

/// Server waits. Production uses the defaults; tests shorten them per
/// instance so timeout paths run in milliseconds.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) struct ServerTimings {
    pub(super) poll_ms: u32,
    pub(super) stop_grace: std::time::Duration,
    pub(super) handshake_ms: u32,
}

impl Default for ServerTimings {
    fn default() -> Self {
        Self {
            poll_ms: SERVER_POLL_TIMEOUT_MS,
            stop_grace: std::time::Duration::from_millis(SERVER_STOP_GRACE_MS),
            handshake_ms: crate::protocol::SERVER_HANDSHAKE_TIMEOUT_MS,
        }
    }
}

impl ServerTimings {
    /// How long a connection waits for a worker slot before it is refused.
    /// Two poll slices: a worker whose client left exits within one.
    pub(super) fn slot_wait(&self) -> std::time::Duration {
        std::time::Duration::from_millis(2 * self.poll_ms as u64)
    }
}
#[cfg(any(target_os = "linux", target_os = "freebsd", windows))]
pub(super) const CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS: u64 = 5;
/// A call deadline that ends an SHM attach wait is not the region's fault
/// once; the second time in a row on a live session, SHM is dropped like on
/// any attach failure, so a region that never becomes attachable cannot make
/// every call end at its deadline.
#[cfg(any(target_os = "linux", target_os = "freebsd", windows))]
pub(super) const CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT: u32 = 2;
#[cfg(any(target_os = "linux", target_os = "freebsd", windows))]
pub(super) const CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS: u64 = 5_000;
pub(super) const CACHE_RESPONSE_BUF_SIZE: usize = 65536;
pub const LOOKUP_LOGICAL_ITEMS_DEFAULT: u32 = 65_536;
pub const LOOKUP_LOGICAL_SUBCALLS_DEFAULT: u32 = 4_096;
pub const LOOKUP_LOGICAL_RESPONSE_BYTES_DEFAULT: u32 = 64 * 1024 * 1024;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LookupLogicalConfig {
    pub max_items: u32,
    pub max_subcalls: u32,
    pub max_response_bytes: u32,
}

impl Default for LookupLogicalConfig {
    fn default() -> Self {
        Self {
            max_items: LOOKUP_LOGICAL_ITEMS_DEFAULT,
            max_subcalls: LOOKUP_LOGICAL_SUBCALLS_DEFAULT,
            max_response_bytes: LOOKUP_LOGICAL_RESPONSE_BYTES_DEFAULT,
        }
    }
}

impl LookupLogicalConfig {
    pub(super) fn normalize(self) -> Self {
        let defaults = Self::default();
        Self {
            max_items: if self.max_items == 0 {
                defaults.max_items
            } else {
                self.max_items
            },
            max_subcalls: if self.max_subcalls == 0 {
                defaults.max_subcalls
            } else {
                self.max_subcalls
            },
            max_response_bytes: if self.max_response_bytes == 0 {
                defaults.max_response_bytes
            } else {
                self.max_response_bytes
            },
        }
    }
}

/// When all workers are busy, wait up to `slot_wait` for one to
/// finish, so a client that reconnects right after closing is not refused
/// while its old worker exits. Clients queued behind this one wait as well,
/// but while every worker stays busy they would be refused too. Returns
/// whether a slot is free.
pub(super) fn wait_for_worker_slot(
    running: &std::sync::atomic::AtomicBool,
    slot_wait: std::time::Duration,
    mut has_slot: impl FnMut() -> bool,
) -> bool {
    let deadline = std::time::Instant::now() + slot_wait;
    loop {
        if has_slot() {
            return true;
        }
        if !running.load(std::sync::atomic::Ordering::Acquire)
            || std::time::Instant::now() >= deadline
        {
            return false;
        }
        std::thread::sleep(std::time::Duration::from_millis(1));
    }
}

pub(super) fn next_power_of_2_u32(n: u32) -> u32 {
    if n < 16 {
        return 16;
    }

    if n > (1u32 << 31) {
        return 1u32 << 31;
    }

    let mut value = n - 1;
    value |= value >> 1;
    value |= value >> 2;
    value |= value >> 4;
    value |= value >> 8;
    value |= value >> 16;
    value + 1
}

pub(super) fn ensure_client_scratch(buf: &mut Vec<u8>, needed: usize) -> &mut [u8] {
    if buf.len() < needed {
        buf.resize(needed, 0);
    }
    &mut buf[..needed]
}

pub(super) fn lookup_raw_response_size(
    hdr_size: usize,
    item_count: usize,
    items: &[Vec<u8>],
) -> Result<usize, crate::protocol::NipcError> {
    let dir_size = item_count
        .checked_mul(crate::protocol::LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(crate::protocol::NipcError::Overflow)?;
    let mut data = hdr_size
        .checked_add(dir_size)
        .ok_or(crate::protocol::NipcError::Overflow)?;
    for item in items {
        data = crate::protocol::align8(data);
        data = data
            .checked_add(item.len())
            .ok_or(crate::protocol::NipcError::Overflow)?;
    }
    Ok(data)
}
