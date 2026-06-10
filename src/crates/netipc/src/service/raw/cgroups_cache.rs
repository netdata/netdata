use super::client::{ClientConfig, ClientState, RawClient};
use super::common::next_power_of_2_u32;

/// Cached copy of a single cgroup item. Owns its strings.
/// Built from ephemeral L2 views during cache construction.
#[derive(Debug, Clone)]
pub struct CgroupsCacheItem {
    pub hash: u32,
    pub options: u32,
    pub enabled: u32,
    pub name: String,
    pub path: String,
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

fn cache_hash_name(name: &str) -> u32 {
    let mut h: u32 = 5381;
    for b in name.as_bytes() {
        h = ((h << 5).wrapping_add(h)).wrapping_add(*b as u32);
    }
    h
}

/// L3 client-side cgroups snapshot cache.
///
/// Wraps an L2 client and maintains a local owned copy of the most
/// recent successful snapshot. Lookup by hash+name is O(1) via HashMap.
///
/// On refresh failure, the previous cache is preserved. The cache
/// is empty only if no successful refresh has ever occurred.
pub struct CgroupsCache {
    pub(super) client: RawClient,
    items: Vec<CgroupsCacheItem>,
    /// Open-addressing hash table: (hash ^ djb2(name)) -> index into items vec
    buckets: Vec<CgroupsHashBucket>,
    systemd_enabled: u32,
    generation: u64,
    populated: bool,
    refresh_success_count: u32,
    refresh_failure_count: u32,
    /// Monotonic reference point for timestamp calculation
    epoch: std::time::Instant,
    /// Monotonic ms of last successful refresh (0 if never)
    pub last_refresh_ts: u64,
}

impl CgroupsCache {
    /// Create a new L3 cache. Creates the underlying L2 client context.
    /// Does NOT connect. Does NOT require the server to be running.
    /// Cache starts empty (populated == false).
    pub fn new(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        CgroupsCache {
            client: RawClient::new_snapshot(run_dir, service_name, config),
            items: Vec::new(),
            buckets: Vec::new(),
            systemd_enabled: 0,
            generation: 0,
            populated: false,
            refresh_success_count: 0,
            refresh_failure_count: 0,
            epoch: std::time::Instant::now(),
            last_refresh_ts: 0,
        }
    }

    /// Refresh the cache. Drives the L2 client (connect/reconnect as
    /// needed) and requests a fresh snapshot. On success, rebuilds the
    /// local cache. On failure, preserves the previous cache.
    ///
    /// Returns true if the cache was updated.
    pub fn refresh(&mut self) -> bool {
        self.client.refresh();

        match self.client.call_snapshot() {
            Ok(view) => {
                let mut new_items = Vec::with_capacity(view.item_count as usize);
                for i in 0..view.item_count {
                    match view.item(i) {
                        Ok(iv) => {
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
                        Err(_) => {
                            self.refresh_failure_count += 1;
                            return false;
                        }
                    }
                }

                let mut buckets = Vec::new();
                if !new_items.is_empty() {
                    let bcount = next_power_of_2_u32((new_items.len() as u32) * 2) as usize;
                    buckets.resize(bcount, CgroupsHashBucket::default());
                    let mask = (bcount - 1) as u32;
                    for (i, item) in new_items.iter().enumerate() {
                        let mut slot = (item.hash ^ cache_hash_name(&item.name)) & mask;
                        while buckets[slot as usize].used {
                            slot = (slot + 1) & mask;
                        }
                        buckets[slot as usize] = CgroupsHashBucket {
                            index: i as u32,
                            used: true,
                        };
                    }
                }

                self.items = new_items;
                self.buckets = buckets;
                self.systemd_enabled = view.systemd_enabled;
                self.generation = view.generation;
                self.populated = true;
                self.refresh_success_count += 1;
                self.last_refresh_ts = self.epoch.elapsed().as_millis() as u64;
                true
            }
            Err(_) => {
                self.refresh_failure_count += 1;
                false
            }
        }
    }

    /// Returns true if at least one successful refresh has occurred.
    /// Cheap cached boolean. No I/O, no syscalls.
    ///
    /// Note: ready means "has cached data", not "is connected."
    #[inline]
    pub fn ready(&self) -> bool {
        self.populated
    }

    /// Look up a cached item by hash + name. O(1) via open-addressing hash
    /// table. No I/O.
    pub fn lookup(&self, hash: u32, name: &str) -> Option<&CgroupsCacheItem> {
        if !self.populated {
            return None;
        }
        if !self.buckets.is_empty() {
            let mask = (self.buckets.len() - 1) as u32;
            let mut slot = (hash ^ cache_hash_name(name)) & mask;
            while self.buckets[slot as usize].used {
                let item = &self.items[self.buckets[slot as usize].index as usize];
                if item.hash == hash && item.name == name {
                    return Some(item);
                }
                slot = (slot + 1) & mask;
            }
            return None;
        }

        self.items
            .iter()
            .find(|item| item.hash == hash && item.name == name)
    }

    /// Fill a status snapshot for diagnostics.
    pub fn status(&self) -> CgroupsCacheStatus {
        CgroupsCacheStatus {
            populated: self.populated,
            item_count: self.items.len() as u32,
            systemd_enabled: self.systemd_enabled,
            generation: self.generation,
            refresh_success_count: self.refresh_success_count,
            refresh_failure_count: self.refresh_failure_count,
            connection_state: self.client.status().state,
            last_refresh_ts: self.last_refresh_ts,
        }
    }

    /// Close the cache: free all cached items, close the L2 client.
    pub fn close(&mut self) {
        self.items.clear();
        self.buckets.clear();
        self.populated = false;
        self.client.close();
    }
}

impl Drop for CgroupsCache {
    fn drop(&mut self) {
        self.close();
    }
}
