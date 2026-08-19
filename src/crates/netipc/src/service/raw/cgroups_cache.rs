use super::client::{ClientAbortHandle, ClientConfig, ClientState, RawClient};
use super::common::next_power_of_2_u32;
use std::sync::{Mutex, MutexGuard, RwLock, RwLockReadGuard};

/// Owned copy of a single cgroup item.
#[derive(Debug, Clone)]
pub struct CgroupsCacheItem {
    pub hash: u32,
    pub options: u32,
    pub enabled: u32,
    pub name: String,
    pub path: String,
}

/// Borrowed immutable cgroup item view.
///
/// The view is valid only while the read guard that produced it is alive.
#[derive(Debug, Clone, Copy)]
pub struct CgroupsCacheItemView<'a> {
    pub hash: u32,
    pub options: u32,
    pub enabled: u32,
    pub name: &'a str,
    pub path: &'a str,
}

impl<'a> CgroupsCacheItemView<'a> {
    /// Duplicate this borrowed view into an owned item that survives unlock.
    pub fn dup(self) -> CgroupsCacheItem {
        CgroupsCacheItem {
            hash: self.hash,
            options: self.options,
            enabled: self.enabled,
            name: self.name.to_string(),
            path: self.path.to_string(),
        }
    }
}

/// L3 cache status snapshot (for diagnostics, not hot path).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CgroupsCacheStatus {
    pub populated: bool,
    pub item_count: u32,
    pub systemd_enabled: u32,
    pub generation: u64,
    pub refresh_success_count: u32,
    pub refresh_failure_count: u32,
    pub connection_state: ClientState,
    /// Monotonic milliseconds of last successful refresh (0 if never).
    pub last_refresh_ts: u64,
}

#[derive(Debug, Clone, Copy, Default)]
struct CgroupsHashBucket {
    index: u32,
    used: bool,
}

#[derive(Debug, Default)]
struct CgroupsCacheSnapshot {
    items: Vec<CgroupsCacheItem>,
    buckets: Vec<CgroupsHashBucket>,
    systemd_enabled: u32,
    generation: u64,
    populated: bool,
}

struct CgroupsCacheState {
    snapshot: CgroupsCacheSnapshot,
    refresh_success_count: u32,
    refresh_failure_count: u32,
    epoch: std::time::Instant,
    last_refresh_ts: u64,
}

impl CgroupsCacheState {
    fn new() -> Self {
        Self {
            snapshot: CgroupsCacheSnapshot::default(),
            refresh_success_count: 0,
            refresh_failure_count: 0,
            epoch: std::time::Instant::now(),
            last_refresh_ts: 0,
        }
    }
}

fn lock_mutex<T>(mutex: &Mutex<T>) -> MutexGuard<'_, T> {
    mutex
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

fn read_lock<T>(lock: &RwLock<T>) -> RwLockReadGuard<'_, T> {
    lock.read().unwrap_or_else(|poisoned| poisoned.into_inner())
}

fn cache_hash_name(name: &str) -> u32 {
    let mut h: u32 = 5381;
    for b in name.as_bytes() {
        h = ((h << 5).wrapping_add(h)).wrapping_add(*b as u32);
    }
    h
}

fn cache_build_buckets(items: &[CgroupsCacheItem]) -> Vec<CgroupsHashBucket> {
    if items.is_empty() || items.len() > (u32::MAX as usize / 2) {
        return Vec::new();
    }

    let bcount = next_power_of_2_u32((items.len() as u32) * 2) as usize;
    if bcount == 0 {
        return Vec::new();
    }

    let mut buckets = vec![CgroupsHashBucket::default(); bcount];
    let mask = (bcount - 1) as u32;
    for (i, item) in items.iter().enumerate() {
        let mut slot = (item.hash ^ cache_hash_name(&item.name)) & mask;
        while buckets[slot as usize].used {
            slot = (slot + 1) & mask;
        }
        buckets[slot as usize] = CgroupsHashBucket {
            index: i as u32,
            used: true,
        };
    }
    buckets
}

impl CgroupsCacheSnapshot {
    fn from_items(items: Vec<CgroupsCacheItem>, systemd_enabled: u32, generation: u64) -> Self {
        let buckets = cache_build_buckets(&items);
        Self {
            items,
            buckets,
            systemd_enabled,
            generation,
            populated: true,
        }
    }

    fn get(&self, hash: u32, name: &str) -> Option<CgroupsCacheItemView<'_>> {
        if !self.populated {
            return None;
        }

        if !self.buckets.is_empty() {
            let mask = (self.buckets.len() - 1) as u32;
            let mut slot = (hash ^ cache_hash_name(name)) & mask;
            while self.buckets[slot as usize].used {
                let item = &self.items[self.buckets[slot as usize].index as usize];
                if item.hash == hash && item.name == name {
                    return Some(item.as_view());
                }
                slot = (slot + 1) & mask;
            }
            return None;
        }

        self.items
            .iter()
            .find(|item| item.hash == hash && item.name == name)
            .map(CgroupsCacheItem::as_view)
    }
}

impl CgroupsCacheItem {
    fn as_view(&self) -> CgroupsCacheItemView<'_> {
        CgroupsCacheItemView {
            hash: self.hash,
            options: self.options,
            enabled: self.enabled,
            name: &self.name,
            path: &self.path,
        }
    }
}

/// Read guard for borrowed L3 cache access.
pub struct CgroupsCacheReadGuard<'a> {
    state: RwLockReadGuard<'a, CgroupsCacheState>,
}

impl<'a> CgroupsCacheReadGuard<'a> {
    /// Look up a cached item by hash + name. No I/O.
    pub fn get(&self, hash: u32, name: &str) -> Option<CgroupsCacheItemView<'_>> {
        self.state.snapshot.get(hash, name)
    }

    /// Duplicate a borrowed view into an owned item that survives unlock.
    pub fn dup(&self, view: CgroupsCacheItemView<'_>) -> CgroupsCacheItem {
        view.dup()
    }
}

/// L3 client-side cgroups snapshot cache.
///
/// Refresh serializes L2 client use, builds a new immutable snapshot privately,
/// then swaps it under a write lock. Readers hold read guards while using
/// borrowed views and do not block snapshot construction.
pub struct CgroupsCache {
    client: Mutex<RawClient>,
    abort_handle: ClientAbortHandle,
    state: RwLock<CgroupsCacheState>,
}

impl CgroupsCache {
    /// Create a new L3 cache. Creates the underlying L2 client context.
    /// Does NOT connect. Does NOT require the server to be running.
    /// Cache starts empty (populated == false).
    pub fn new(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        let client = RawClient::new_snapshot(run_dir, service_name, config);
        let abort_handle = client.abort_handle();
        CgroupsCache {
            client: Mutex::new(client),
            abort_handle,
            state: RwLock::new(CgroupsCacheState::new()),
        }
    }

    /// Refresh the cache. Drives the L2 client and requests a fresh snapshot.
    /// On success, swaps in a new immutable snapshot. On failure, preserves the
    /// previous snapshot.
    pub fn refresh(&self) -> bool {
        let mut client = lock_mutex(&self.client);
        client.refresh();

        let view = match client.call_snapshot() {
            Ok(view) => view,
            Err(_) => {
                let mut state = self
                    .state
                    .write()
                    .unwrap_or_else(|poisoned| poisoned.into_inner());
                state.refresh_failure_count += 1;
                return false;
            }
        };

        let mut new_items = Vec::with_capacity(view.item_count as usize);
        for i in 0..view.item_count {
            let iv = match view.item(i) {
                Ok(iv) => iv,
                Err(_) => {
                    let mut state = self
                        .state
                        .write()
                        .unwrap_or_else(|poisoned| poisoned.into_inner());
                    state.refresh_failure_count += 1;
                    return false;
                }
            };
            let name = match iv.name.as_str() {
                Ok(s) => s.to_string(),
                Err(_) => String::from_utf8_lossy(iv.name.as_bytes()).into_owned(),
            };
            let path = match iv.path.as_str() {
                Ok(s) => s.to_string(),
                Err(_) => String::from_utf8_lossy(iv.path.as_bytes()).into_owned(),
            };
            new_items.push(CgroupsCacheItem {
                hash: iv.hash,
                options: iv.options,
                enabled: iv.enabled,
                name,
                path,
            });
        }

        let snapshot =
            CgroupsCacheSnapshot::from_items(new_items, view.systemd_enabled, view.generation);
        let mut state = self
            .state
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        state.snapshot = snapshot;
        state.refresh_success_count += 1;
        state.last_refresh_ts = state.epoch.elapsed().as_millis() as u64;
        true
    }

    /// Returns true if at least one successful refresh has occurred.
    #[inline]
    pub fn ready(&self) -> bool {
        read_lock(&self.state).snapshot.populated
    }

    /// Acquire a read guard for borrowed cache access.
    pub fn read_lock(&self) -> CgroupsCacheReadGuard<'_> {
        CgroupsCacheReadGuard {
            state: read_lock(&self.state),
        }
    }

    /// Duplicate a borrowed view into an owned item.
    pub fn item_dup(&self, view: CgroupsCacheItemView<'_>) -> CgroupsCacheItem {
        view.dup()
    }

    #[doc(hidden)]
    pub fn seed_for_tests(
        &self,
        items: Vec<CgroupsCacheItem>,
        systemd_enabled: u32,
        generation: u64,
    ) {
        let snapshot = CgroupsCacheSnapshot::from_items(items, systemd_enabled, generation);
        let mut state = self
            .state
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        state.snapshot = snapshot;
        state.refresh_success_count += 1;
        state.last_refresh_ts = state.epoch.elapsed().as_millis() as u64;
    }

    /// Fill a status snapshot for diagnostics.
    pub fn status(&self) -> CgroupsCacheStatus {
        let connection_state = lock_mutex(&self.client).status().state;
        let state = read_lock(&self.state);
        CgroupsCacheStatus {
            populated: state.snapshot.populated,
            item_count: state.snapshot.items.len() as u32,
            systemd_enabled: state.snapshot.systemd_enabled,
            generation: state.snapshot.generation,
            refresh_success_count: state.refresh_success_count,
            refresh_failure_count: state.refresh_failure_count,
            connection_state,
            last_refresh_ts: state.last_refresh_ts,
        }
    }

    /// Set the context-level default timeout for blocking refresh calls.
    pub fn set_call_timeout(&self, timeout_ms: u32) {
        lock_mutex(&self.client).set_call_timeout(timeout_ms);
    }

    /// Return a cloneable handle that can abort refresh calls from another thread.
    pub fn abort_handle(&self) -> ClientAbortHandle {
        self.abort_handle.clone()
    }

    /// Request abort of an in-flight or future refresh call.
    pub fn abort(&self) {
        self.abort_handle.abort();
    }

    /// Clear a previous abort request so the cache client can be reused.
    pub fn clear_abort(&self) {
        self.abort_handle.clear();
    }

    #[cfg(test)]
    pub fn max_receive_message_bytes(&self) -> usize {
        lock_mutex(&self.client).max_receive_message_bytes()
    }

    /// Close the cache and underlying L2 client.
    pub fn close(&self) {
        {
            let mut state = self
                .state
                .write()
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            state.snapshot = CgroupsCacheSnapshot::default();
        }
        lock_mutex(&self.client).close();
    }
}

impl Drop for CgroupsCache {
    fn drop(&mut self) {
        self.close();
    }
}
