//! A bounded, read-through cache of remote objects on the local filesystem.
//!
//! The cache exists so a consumer can fetch large immutable objects (e.g. index
//! files in object storage) back to local disk and then read them as ordinary
//! files — crucially via `mmap`, which no value/heap cache (`foyer`, `cacache`,
//! …) can offer for objects of arbitrary size. It is deliberately generic and
//! knows nothing about its consumer: an entry is keyed by its local **filename**
//! (the identity of immutable content); the supplied **size** is used for budget
//! accounting and a fetch-time integrity check, not as part of the key; and the
//! bytes are materialized by a caller-supplied async **fetch closure**.
//!
//! Design (clean-room, informed by Quickwit's `SplitCache` and SlateDB's
//! `CachedObjectStore`):
//!
//! - **Hard byte cap.** Total bytes on disk never exceed the configured capacity.
//!   A query atomically *reserves* the footprint it needs before fetching, so
//!   concurrent queries share one budget rather than each blowing past it.
//! - **Deadlock-free admission.** A query commits to *all* of its files at once or
//!   waits holding nothing. Because a consumer typically needs every file of a
//!   query available together, partial holds would deadlock; atomic all-or-nothing
//!   admission rules that out.
//! - **Single-flight.** Concurrent demand for the same object fetches it once;
//!   other callers await the in-flight fetch.
//! - **In-use pinning.** A returned [`CachedFile`] pins its entry; pinned entries
//!   are never evicted, so a reader's file cannot vanish mid-use.
//! - **LRU eviction** of unpinned entries to make room.
//! - **Torn-write safe across restart.** Writes go through
//!   [`file_registry::durable`] (temp-file + fsync + atomic rename + parent-dir
//!   fsync) — the workspace's single source of truth for crash-safe local writes,
//!   so the uniform `.tmp` naming/sweep contract is shared with every other
//!   producer. On [`open`] stray temp files (interrupted writes) are swept,
//!   surviving files are re-registered, and the cache is evicted back under
//!   capacity if it shrank.
//!
//! Scope / preconditions:
//!
//! - **Filenames are single path components** — exactly one normal component
//!   (rejecting path separators, `.`, `..`, a root, or a platform prefix such as
//!   a Windows drive), with no NUL and not the reserved `.tmp` suffix; enforced
//!   both on caller input and on recovered names. The cache treats the filename
//!   as the identity of immutable content: a
//!   cache hit returns the on-disk file as-is and does NOT re-verify [`Want::size`],
//!   so a consumer whose objects can change must key them by content identity
//!   (hash) — there is no TTL / etag / invalidation.
//! - **Whole-object in memory.** A fetch yields the entire object as a `Vec<u8>`
//!   before it is written, so peak memory during a fetch is bounded by the object
//!   size (plus the backend's own read buffer). Sized for objects that fit in RAM.
//! - **Local, owner-private directory.** The cache directory MUST be on
//!   locally-attached storage owned privately by this process (it is created
//!   `0700` on Unix). Eviction unlinks happen synchronously under the state lock,
//!   which assumes fast local unlinks; and recovery/writes are not symlink-hardened,
//!   so a shared/writable cache dir is out of scope.
//!
//! [`open`]: FileCache::open

use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::SystemTime;

use file_registry::durable::{self, TMP_SUFFIX};
use tokio::sync::Notify;
use tokio_util::sync::CancellationToken;

/// An object the caller wants present locally: its cache **filename** (the key)
/// and its known **size** in bytes.
///
/// - `filename` MUST be a single normal path component (no separators, `.`/`..`,
///   root, platform prefix, NUL, or the `.tmp` suffix); otherwise the object is
///   rejected and omitted from the result.
/// - `size` is the object's byte length. It is enforced as an integrity check on
///   freshly-fetched bytes (a mismatch degrades the object), but is NOT
///   re-verified on a cache hit — the cache assumes the filename identifies
///   immutable content.
#[derive(Clone, Debug)]
pub struct Want {
    pub filename: String,
    pub size: u64,
}

/// Whether `name` is a safe cache filename: a single normal path component (no
/// separators, `.`/`..`, root, or platform prefix such as a Windows drive), no
/// NUL, and not the reserved temp suffix (which recovery would sweep). Using
/// [`Path::components`] keeps this correct across platforms rather than
/// blacklisting separator characters.
fn is_valid_filename(name: &str) -> bool {
    if name.is_empty() || name.contains('\0') || name.ends_with(TMP_SUFFIX) {
        return false;
    }
    let mut comps = Path::new(name).components();
    match (comps.next(), comps.next()) {
        (Some(std::path::Component::Normal(c)), None) => c == std::ffi::OsStr::new(name),
        _ => false,
    }
}

/// Failure modes of [`FileCache::acquire`]. Per-object fetch failures are NOT
/// errors here — they degrade (the object is simply omitted from the result);
/// only a query-wide condition surfaces as an `Err`.
#[derive(Debug, thiserror::Error)]
pub enum CacheError {
    /// The query's total footprint exceeds the whole cache capacity, so it can
    /// never be satisfied. The caller is expected to shrink the request.
    #[error("requested footprint {footprint} bytes exceeds cache capacity {capacity} bytes")]
    TooLarge { footprint: u64, capacity: u64 },
    /// The cancellation token fired before the query was satisfied.
    #[error("cancelled")]
    Cancelled,
    /// Eviction could not free the space the query needs because unlinking a
    /// victim failed (e.g. the cache directory became unwritable). The
    /// feasibility check guarantees enough *evictable* bytes exist, so this is a
    /// real filesystem failure, not contention — surfaced rather than waited on so
    /// a permanently-broken cache dir cannot hang the caller.
    #[error("cache eviction failed (could not free space on a broken cache directory)")]
    EvictionFailed,
}

/// A pinned handle to a cached file. While it is alive the underlying entry is
/// not evictable; dropping it releases the pin (the file stays cached and
/// becomes eligible for eviction).
pub struct CachedFile {
    shared: Arc<Shared>,
    filename: String,
    path: PathBuf,
}

impl CachedFile {
    /// Absolute path to the cached file — open/`mmap` it directly.
    pub fn path(&self) -> &Path {
        &self.path
    }

    /// The cache key (filename) this handle pins.
    pub fn filename(&self) -> &str {
        &self.filename
    }
}

impl std::fmt::Debug for CachedFile {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("CachedFile")
            .field("filename", &self.filename)
            .field("path", &self.path)
            .finish()
    }
}

impl Drop for CachedFile {
    fn drop(&mut self) {
        {
            let mut inner = self.shared.lock();
            if let Some(e) = inner.entries.get_mut(&self.filename) {
                debug_assert!(e.pins > 0, "file-cache: pin underflow for {}", self.filename);
                e.pins = e.pins.saturating_sub(1);
            }
        }
        // A freed pin may let a waiting reservation make room — wake re-checks.
        self.shared.notify.notify_waiters();
    }
}

/// One on-disk cached object.
struct Entry {
    size: u64,
    /// Monotonic access tick for LRU ordering (larger = more recently used).
    last_access: u64,
    /// Number of live [`CachedFile`] handles; `> 0` ⇒ not evictable.
    pins: usize,
}

/// Mutable cache state, guarded by a single mutex. The mutex is held only for
/// short, panic-free critical sections and never across `.await`; the one
/// synchronous I/O under the lock is the bounded per-victim `remove_file` in
/// `make_room` (acceptable because the cache directory is local — see crate docs).
struct State {
    capacity: u64,
    /// Bytes physically on disk (= sum of `entries` sizes). Invariant: `≤ capacity`
    /// in steady state. Recovery is best-effort: if an unlink fails while evicting
    /// a shrunk cache back under capacity, `open` returns with `total > capacity`
    /// and self-heals as entries are later evicted (and any room-needing `acquire`
    /// surfaces the broken dir as [`CacheError::EvictionFailed`]).
    total: u64,
    /// Bytes reserved by in-flight queries whose fetches have not landed yet.
    /// Invariant: `total + reserved ≤ capacity` once admission has committed (see
    /// the `total` note for the recovery exception).
    reserved: u64,
    /// Monotonic tick source for LRU.
    tick: u64,
    entries: HashMap<String, Entry>,
    /// Filenames currently being fetched (single-flight markers).
    inflight: HashSet<String>,
}

/// Outcome of an atomic admission attempt.
enum Admit {
    /// Admitted: hits pinned, misses reserved + marked in-flight.
    Ready(Plan),
    /// Not satisfiable right now (room held by pins / other reservations); wait.
    Wait,
    /// Footprint exceeds capacity; can never be satisfied.
    TooLarge { footprint: u64, capacity: u64 },
    /// Enough evictable space existed but an unlink failed, so room could not be
    /// freed — a filesystem failure, surfaced rather than waited on.
    EvictionFailed,
}

/// What an admitted query must do: hits are already pinned; `mine` are reserved
/// and must be fetched by this query; `await_others` are being fetched by a
/// concurrent query and must be awaited.
struct Plan {
    hits: Vec<String>,
    mine: Vec<Want>,
    await_others: Vec<Want>,
}

impl State {
    fn next_tick(&mut self) -> u64 {
        let t = self.tick;
        self.tick += 1;
        t
    }

    /// Evict unpinned, unprotected entries (LRU first) until `total ≤ target`.
    /// Returns whether the target was reached: it can fall short if every
    /// remaining over-budget byte is pinned/protected, or if an unlink fails (the
    /// file is then kept accounted, see below). Callers MUST treat `false` as
    /// "could not make room" and not commit beyond capacity.
    #[must_use]
    fn make_room(&mut self, dir: &Path, target: u64, protect: &HashSet<String>) -> bool {
        if self.total <= target {
            return true;
        }
        let mut victims: Vec<(String, u64)> = self
            .entries
            .iter()
            .filter(|(name, e)| e.pins == 0 && !protect.contains(*name))
            .map(|(name, e)| (name.clone(), e.last_access))
            .collect();
        victims.sort_by_key(|(_, last_access)| *last_access);

        for (name, _) in victims {
            if self.total <= target {
                break;
            }
            let size = self.entries[&name].size;
            let path = dir.join(&name);
            match std::fs::remove_file(&path) {
                // Gone from disk (unlinked now, or already absent) ⇒ bytes freed.
                Ok(()) => {}
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                Err(e) => {
                    // The file is still on disk; dropping it from accounting would
                    // understate real usage and break the hard cap. Keep it
                    // accounted and skip it as a victim.
                    tracing::warn!(
                        "file-cache: failed to evict {} ({e}); keeping it accounted",
                        path.display()
                    );
                    continue;
                }
            }
            self.entries.remove(&name);
            self.total -= size;
        }
        self.total <= target
    }

    /// Try to admit `items` atomically. On success the state is mutated to
    /// reflect the commitment (pins + reservations + in-flight markers).
    fn try_admit(&mut self, items: &[Want], dir: &Path) -> Admit {
        // Saturating so a pathological/overflowing footprint reads as huge and is
        // rejected, rather than wrapping near zero and falsely "fitting".
        let footprint: u64 = items
            .iter()
            .map(|w| w.size)
            .fold(0u64, u64::saturating_add);
        if footprint > self.capacity {
            return Admit::TooLarge {
                footprint,
                capacity: self.capacity,
            };
        }

        let mut hits = Vec::new();
        let mut hit_set = HashSet::new();
        let mut await_others = Vec::new();
        let mut mine = Vec::new();
        let mut need = 0u64;
        for w in items {
            if let Some(e) = self.entries.get(&w.filename) {
                // The cache keys by filename (immutable-content contract); a hit is
                // served without size re-verification. A size mismatch means the
                // caller violated that contract — catch it in dev, ignore in release.
                debug_assert_eq!(
                    e.size, w.size,
                    "file-cache: size mismatch on hit for {} (cached {}, requested {}); \
                     filenames must identify immutable content",
                    w.filename, e.size, w.size
                );
                hit_set.insert(w.filename.clone());
                hits.push(w.filename.clone());
            } else if self.inflight.contains(&w.filename) {
                await_others.push(w.clone());
            } else {
                need = need.saturating_add(w.size);
                mine.push(w.clone());
            }
        }

        // Bytes that cannot be evicted to make room: already-pinned entries plus
        // this query's own hits (which we are about to pin). In-flight objects
        // being fetched by other queries are NOT in `entries` (they live in
        // `inflight`), so they are excluded here and accounted via `reserved`
        // below. If even after dropping everything else we still cannot fit
        // `reserved + need`, the room is held by others — wait, holding nothing.
        let protected_total: u64 = self
            .entries
            .iter()
            .filter(|(name, e)| e.pins > 0 || hit_set.contains(*name))
            .map(|(_, e)| e.size)
            .fold(0u64, u64::saturating_add);
        if protected_total.saturating_add(self.reserved).saturating_add(need) > self.capacity {
            return Admit::Wait;
        }

        // Feasible in principle; actually free the headroom. If eviction falls
        // short (e.g. an unlink failed and the bytes stay accounted), do NOT
        // commit — that would push `total + reserved` over capacity. Wait and let
        // the caller retry on the next notify.
        let target = self.capacity - self.reserved - need;
        if !self.make_room(dir, target, &hit_set) {
            // The feasibility check above proved enough evictable bytes exist, so
            // a shortfall here is an unlink failure (broken cache dir), not
            // contention. Surface it rather than waiting forever for room that
            // can never be freed.
            tracing::error!(
                "file-cache: eviction failed to free room (total={}, target={})",
                self.total,
                target
            );
            return Admit::EvictionFailed;
        }

        // Commit.
        for name in &hits {
            let tick = self.next_tick();
            let e = self.entries.get_mut(name).expect("hit present");
            e.pins += 1;
            e.last_access = tick;
        }
        self.reserved += need;
        for w in &mine {
            self.inflight.insert(w.filename.clone());
        }

        Admit::Ready(Plan {
            hits,
            mine,
            await_others,
        })
    }

    /// Register a freshly-fetched file: move its bytes from `reserved` to on-disk
    /// `total`, clear the in-flight marker, and pin it once. Defensive against a
    /// double-register: it never clobbers an existing entry's pin count, and only
    /// gives back a reservation it actually holds (`inflight` gates the decrement,
    /// mirroring [`Self::release_unfetched`]).
    fn register(&mut self, filename: &str, size: u64) {
        if self.entries.contains_key(filename) {
            // Should be unreachable (admission dedups + classifies hits). Pin the
            // existing entry to stay balanced with the `CachedFile` the caller is
            // about to create — otherwise its drop would underflow the pin count
            // and the file could be evicted out from under a live reader. Never
            // double-count `total`; reconcile any stray reservation.
            tracing::error!("file-cache: register of already-cached {filename}; pinning existing");
            if self.inflight.remove(filename) {
                self.reserved = self.reserved.saturating_sub(size);
            }
            let tick = self.next_tick();
            let e = self.entries.get_mut(filename).expect("present");
            e.pins += 1;
            e.last_access = tick;
            return;
        }
        if self.inflight.remove(filename) {
            self.reserved = self.reserved.saturating_sub(size);
        } else {
            // Unreachable: register is only called for a `mine` item that
            // admission marked in-flight. Surface the regression, and still
            // reconcile `reserved` (best-effort, saturating) so a sibling bug
            // cannot permanently leak budget past the cap.
            tracing::error!("file-cache: register of {filename} without in-flight marker");
            self.reserved = self.reserved.saturating_sub(size);
        }
        self.total += size;
        let tick = self.next_tick();
        self.entries.insert(
            filename.to_owned(),
            Entry {
                size,
                last_access: tick,
                pins: 1,
            },
        );
    }

    /// Release a reservation whose fetch did not land (failure / size mismatch /
    /// cancellation): give the bytes back and clear the in-flight marker.
    fn release_unfetched(&mut self, filename: &str, size: u64) {
        if self.inflight.remove(filename) {
            self.reserved = self.reserved.saturating_sub(size);
        }
    }
}

/// Shared, reference-counted cache core.
struct Shared {
    dir: PathBuf,
    state: Mutex<State>,
    /// Pulsed whenever room frees or an in-flight fetch settles, so waiters
    /// (reservation admission and single-flight awaiters) re-check.
    notify: Notify,
}

impl Shared {
    fn lock(&self) -> std::sync::MutexGuard<'_, State> {
        self.state.lock().unwrap_or_else(|e| {
            tracing::error!("file-cache: mutex poisoned, recovering — state may be inconsistent");
            e.into_inner()
        })
    }
}

/// A bounded read-through local-file cache. Cheap to clone (it is an `Arc`).
#[derive(Clone)]
pub struct FileCache {
    shared: Arc<Shared>,
}

impl FileCache {
    /// Open (or create) a cache rooted at `dir` with a hard byte `capacity`.
    ///
    /// The directory is created `0700` on Unix (it MUST be owner-private — see the
    /// crate docs). Recovery: stray `*.tmp` files (torn writes) are swept via
    /// [`file_registry::durable::sweep_tmp`]; every surviving file with a safe name
    /// is re-registered (size from disk, recency from mtime); unsafe recovered
    /// names are ignored; if the configured capacity is now smaller than what is on
    /// disk, the cache is evicted back under it.
    pub fn open(dir: impl Into<PathBuf>, capacity: u64) -> std::io::Result<Self> {
        let dir = dir.into();
        std::fs::create_dir_all(&dir)?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            if let Err(e) = std::fs::set_permissions(&dir, std::fs::Permissions::from_mode(0o700)) {
                tracing::warn!(
                    "file-cache: could not set 0700 on {}: {e}",
                    dir.display()
                );
            }
        }

        // Sweep interrupted writes using the shared temp contract, then walk the
        // survivors.
        durable::sweep_tmp(&dir);

        let mut survivors: Vec<(String, u64, SystemTime)> = Vec::new();
        for entry in std::fs::read_dir(&dir)? {
            let entry = entry?;
            let meta = match entry.metadata() {
                Ok(m) => m,
                // The file vanished between read_dir and metadata (e.g. a
                // concurrent sweep); skip rather than fail the whole open.
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => continue,
                Err(e) => return Err(e),
            };
            if !meta.is_file() {
                continue;
            }
            // A non-UTF-8 name is never one we wrote; `to_string_lossy` would
            // mangle it so eviction later unlinks the wrong path. Skip it.
            let name = match entry.file_name().to_str() {
                Some(s) => s.to_owned(),
                None => {
                    tracing::warn!("file-cache: ignoring non-UTF-8 filename in cache dir");
                    continue;
                }
            };
            if name.ends_with(TMP_SUFFIX) {
                // A temp that survived the sweep (e.g. created between sweep and
                // walk); ignore it — it is not a cached object.
                continue;
            }
            if !is_valid_filename(&name) {
                // A stray file with an unsafe name (co-resident process, prior
                // bug). Registering it would let eviction `dir.join` outside the
                // cache; ignore it.
                tracing::warn!("file-cache: ignoring unsafe recovered filename {name:?}");
                continue;
            }
            let mtime = meta.modified().unwrap_or(SystemTime::UNIX_EPOCH);
            survivors.push((name, meta.len(), mtime));
        }
        // Oldest mtime first → smallest LRU tick, so it is evicted first. Break
        // mtime ties by name so recovery order is deterministic even when the
        // filesystem reports coarse or equal mtimes.
        survivors.sort_by(|a, b| a.2.cmp(&b.2).then_with(|| a.0.cmp(&b.0)));

        let mut entries = HashMap::new();
        let mut total = 0u64;
        let mut tick = 0u64;
        for (name, size, _) in survivors {
            entries.insert(
                name,
                Entry {
                    size,
                    last_access: tick,
                    pins: 0,
                },
            );
            total = total.saturating_add(size);
            tick += 1;
        }

        let mut state = State {
            capacity,
            total,
            reserved: 0,
            tick,
            entries,
            inflight: HashSet::new(),
        };
        // Capacity may have shrunk since last run; evict back under it. Best
        // effort at startup — if some file cannot be unlinked the cache simply
        // starts slightly over capacity and recovers as entries are evicted.
        if !state.make_room(&dir, capacity, &HashSet::new()) {
            tracing::warn!(
                "file-cache: recovery could not evict back under capacity (total={}, capacity={})",
                state.total,
                capacity
            );
        }

        Ok(Self {
            shared: Arc::new(Shared {
                dir,
                state: Mutex::new(state),
                notify: Notify::new(),
            }),
        })
    }

    /// Ensure every object in `items` is present locally and return a pinned
    /// [`CachedFile`] for each that is available.
    ///
    /// `fetch(filename)` is invoked at most once per object **per attempt** (a
    /// coalesced object that vanishes before this call pins it — owner failed, or
    /// it was evicted in the lost-race window — is re-fetched on a later attempt,
    /// bounded by `MAX_ATTEMPTS`). Single-flight coalesces concurrent demand. A
    /// fetch that errors, or returns the wrong number of bytes, **degrades**:
    /// that object is
    /// omitted from the result (the caller treats it as a missing source) rather
    /// than failing the whole call. The only `Err`s are [`CacheError::TooLarge`]
    /// (footprint exceeds capacity), [`CacheError::EvictionFailed`] (the cache
    /// directory could not be evicted to make room — a broken dir), and
    /// [`CacheError::Cancelled`].
    ///
    /// The returned handles pin their files; hold them for as long as the files
    /// are in use (e.g. across the whole query) and drop them to release. They are
    /// **not** in the same order as `items` (and a degraded object has none), so
    /// correlate by [`CachedFile::filename`] rather than by index.
    ///
    /// `fetch` MUST NOT re-enter this cache: the reservation for `items` is held
    /// across `fetch.await`, so a re-entrant `acquire` whose footprint collides
    /// with this call's reservation would self-deadlock. Unsafe filenames (path
    /// traversal) and duplicate filenames are dropped/collapsed with a warning
    /// rather than failing the call.
    ///
    /// Cancellation is honored at fetch boundaries (before/after each object),
    /// not mid-write: the durable write of an already-fetched object runs to
    /// completion on a blocking thread before the next cancellation check. This
    /// is bounded by local disk speed and never leaks budget, but a cancelled
    /// call may briefly finish one in-flight write.
    pub async fn acquire<F, Fut>(
        &self,
        items: &[Want],
        fetch: F,
        cancel: &CancellationToken,
    ) -> Result<Vec<CachedFile>, CacheError>
    where
        F: Fn(&str) -> Fut + Send,
        Fut: std::future::Future<Output = anyhow::Result<Vec<u8>>> + Send,
    {
        // Reject unsafe filenames (degrade) and collapse duplicates up front, so
        // admission/accounting only ever sees unique, single-component names.
        let mut seen = HashSet::new();
        let mut items_owned: Vec<Want> = Vec::with_capacity(items.len());
        for w in items {
            if !is_valid_filename(&w.filename) {
                tracing::warn!("file-cache: rejecting unsafe filename {:?}", w.filename);
                continue;
            }
            if seen.insert(w.filename.clone()) {
                items_owned.push(w.clone());
            } else {
                tracing::warn!("file-cache: duplicate filename {:?} in acquire", w.filename);
            }
        }
        if items_owned.is_empty() {
            return Ok(Vec::new());
        }
        // A coalesced object that vanishes before we pin it (owner's fetch failed,
        // or a third query evicted it after the owner dropped its handle) is retried
        // as a fresh miss — immutable content makes a re-fetch safe and correct,
        // and a genuinely-unfetchable object degrades via the miss path. Bounded so
        // a pathological evict/refetch storm cannot loop forever.
        const MAX_ATTEMPTS: usize = 3;
        let mut result: Vec<CachedFile> = Vec::with_capacity(items.len());
        let mut pending: Vec<Want> = items_owned;
        let mut attempt = 0;

        while !pending.is_empty() {
            attempt += 1;

            // Phase 1 — atomic admission. Register the wakeup BEFORE re-checking so
            // a notify between unlock and await is never lost.
            let plan = loop {
                if cancel.is_cancelled() {
                    return Err(CacheError::Cancelled);
                }
                let notified = self.shared.notify.notified();
                tokio::pin!(notified);
                notified.as_mut().enable();

                match self.shared.lock().try_admit(&pending, &self.shared.dir) {
                    Admit::Ready(plan) => break plan,
                    Admit::TooLarge { footprint, capacity } => {
                        return Err(CacheError::TooLarge { footprint, capacity });
                    }
                    Admit::EvictionFailed => return Err(CacheError::EvictionFailed),
                    Admit::Wait => {}
                }

                tokio::select! {
                    _ = &mut notified => {}
                    _ = cancel.cancelled() => return Err(CacheError::Cancelled),
                }
            };

            for name in plan.hits {
                let path = self.shared.dir.join(&name);
                result.push(self.handle(name, path));
            }

            // Phase 2a — fetch the objects this query reserved. The guard releases
            // the reservation of any object that is neither registered nor
            // explicitly released, on every exit path (the release is idempotent —
            // see [`State::release_unfetched`] — so success and degrade are safe).
            let guard = ReserveGuard::new(self.shared.clone(), plan.mine);
            for want in guard.items() {
                if cancel.is_cancelled() {
                    return Err(CacheError::Cancelled);
                }
                match self.fetch_one(&fetch, want, cancel).await {
                    FetchOutcome::Ready(file) => result.push(file),
                    FetchOutcome::Degrade => guard.release(want),
                    FetchOutcome::Cancelled => return Err(CacheError::Cancelled),
                }
            }
            drop(guard);

            if cancel.is_cancelled() {
                return Err(CacheError::Cancelled);
            }

            // Phase 2b — await objects a concurrent query is fetching; the ones that
            // vanish become the next attempt's retry set.
            let mut gone: Vec<Want> = Vec::new();
            for want in plan.await_others {
                match self.await_inflight(&want.filename, cancel).await? {
                    Some(file) => result.push(file),
                    None => gone.push(want),
                }
            }

            if !gone.is_empty() && attempt >= MAX_ATTEMPTS {
                for w in &gone {
                    tracing::warn!(
                        "file-cache: giving up on {} after {attempt} attempts",
                        w.filename
                    );
                }
                break;
            }
            pending = gone;
        }

        Ok(result)
    }

    /// Run one fetch, validate, write atomically, and register.
    async fn fetch_one<F, Fut>(
        &self,
        fetch: &F,
        want: &Want,
        cancel: &CancellationToken,
    ) -> FetchOutcome
    where
        F: Fn(&str) -> Fut + Send,
        Fut: std::future::Future<Output = anyhow::Result<Vec<u8>>> + Send,
    {
        let bytes = tokio::select! {
            r = fetch(&want.filename) => r,
            _ = cancel.cancelled() => return FetchOutcome::Cancelled,
        };
        let bytes = match bytes {
            Ok(b) => b,
            Err(e) => {
                tracing::warn!("file-cache: fetch failed for {}: {e:#}", want.filename);
                return FetchOutcome::Degrade;
            }
        };
        if bytes.len() as u64 != want.size {
            tracing::warn!(
                "file-cache: size mismatch for {} (expected {}, got {})",
                want.filename,
                want.size,
                bytes.len()
            );
            return FetchOutcome::Degrade;
        }
        if let Err(e) = self.write_atomic(&want.filename, bytes).await {
            tracing::warn!("file-cache: write failed for {}: {e}", want.filename);
            return FetchOutcome::Degrade;
        }
        self.shared.lock().register(&want.filename, want.size);
        self.shared.notify.notify_waiters();
        let path = self.shared.dir.join(&want.filename);
        FetchOutcome::Ready(self.handle(want.filename.clone(), path))
    }

    /// Await an object being fetched by another query: pin it once present, or
    /// degrade (return `None`) if that fetch failed.
    async fn await_inflight(
        &self,
        name: &str,
        cancel: &CancellationToken,
    ) -> Result<Option<CachedFile>, CacheError> {
        loop {
            if cancel.is_cancelled() {
                return Err(CacheError::Cancelled);
            }
            let notified = self.shared.notify.notified();
            tokio::pin!(notified);
            notified.as_mut().enable();

            {
                let mut state = self.shared.lock();
                if state.entries.contains_key(name) {
                    let tick = state.next_tick();
                    let e = state.entries.get_mut(name).expect("present");
                    e.pins += 1;
                    e.last_access = tick;
                    let path = self.shared.dir.join(name);
                    return Ok(Some(self.handle(name.to_owned(), path)));
                }
                if !state.inflight.contains(name) {
                    // Not on disk and no longer in flight — the owner's fetch
                    // failed, or it was evicted before we pinned it. The caller
                    // retries it as a fresh miss (bounded).
                    return Ok(None);
                }
            }

            tokio::select! {
                _ = &mut notified => {}
                _ = cancel.cancelled() => return Err(CacheError::Cancelled),
            }
        }
    }

    /// Write `bytes` to `filename` durably (temp + fsync + atomic rename +
    /// parent-dir fsync) via the workspace's shared [`file_registry::durable`]
    /// helper, off the async runtime via `spawn_blocking`.
    async fn write_atomic(&self, filename: &str, bytes: Vec<u8>) -> std::io::Result<()> {
        let final_path = self.shared.dir.join(filename);
        let task = tokio::task::spawn_blocking(move || {
            let r = durable::write_atomic(&final_path, &bytes);
            if r.is_err() {
                // `durable::write_atomic` fsyncs the parent dir AFTER the rename,
                // so an error can still leave the final file in place. Since the
                // caller degrades (never registers it), remove it so an untracked
                // file can't linger and break the byte cap. Best-effort: if the
                // remove ALSO fails the cache dir is failing every op (the
                // documented broken-dir case), and the next room-needing acquire
                // surfaces it as EvictionFailed.
                let _ = std::fs::remove_file(&final_path);
            }
            r
        });
        match task.await {
            Ok(io_result) => io_result,
            Err(e) => Err(std::io::Error::other(e)),
        }
    }

    fn handle(&self, filename: String, path: PathBuf) -> CachedFile {
        CachedFile {
            shared: self.shared.clone(),
            filename,
            path,
        }
    }

    /// Configured byte capacity.
    pub fn capacity(&self) -> u64 {
        self.shared.lock().capacity
    }

    /// Bytes currently on disk.
    pub fn total_bytes(&self) -> u64 {
        self.shared.lock().total
    }

    /// Number of cached files.
    pub fn file_count(&self) -> usize {
        self.shared.lock().entries.len()
    }

    /// Whether `filename` is currently cached on disk.
    pub fn is_cached(&self, filename: &str) -> bool {
        self.shared.lock().entries.contains_key(filename)
    }
}

/// Result of a single fetch attempt.
enum FetchOutcome {
    Ready(CachedFile),
    Degrade,
    Cancelled,
}

/// Releases the reservation + in-flight marker of any object not yet fetched,
/// on every exit path (error, cancellation, drop), so a query can never leak
/// reserved budget.
struct ReserveGuard {
    shared: Arc<Shared>,
    items: Vec<Want>,
}

impl ReserveGuard {
    fn new(shared: Arc<Shared>, items: Vec<Want>) -> Self {
        Self { shared, items }
    }

    fn items(&self) -> &[Want] {
        &self.items
    }

    /// Hand a degraded object's reservation back promptly. Idempotent with the
    /// final drop-time sweep and with [`State::register`].
    fn release(&self, want: &Want) {
        self.shared
            .lock()
            .release_unfetched(&want.filename, want.size);
        self.shared.notify.notify_waiters();
    }
}

impl Drop for ReserveGuard {
    fn drop(&mut self) {
        {
            let mut state = self.shared.lock();
            for want in &self.items {
                // No-op for objects already registered or released (their
                // in-flight marker is gone); releases only what is still pending.
                state.release_unfetched(&want.filename, want.size);
            }
        }
        self.shared.notify.notify_waiters();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tempfile::tempdir;

    /// A fetcher returning `size` deterministic bytes, counting invocations.
    fn counting_fetch(size: u64) -> (Arc<AtomicUsize>, impl Fn(&str) -> futures_ready::Ready) {
        let calls = Arc::new(AtomicUsize::new(0));
        let c = calls.clone();
        let f = move |_name: &str| {
            c.fetch_add(1, Ordering::SeqCst);
            futures_ready::ready(Ok(vec![b'x'; size as usize]))
        };
        (calls, f)
    }

    /// Minimal ready-future helper so tests don't pull an async dep.
    mod futures_ready {
        use std::future::Future;
        use std::pin::Pin;
        use std::task::{Context, Poll};

        pub struct Ready(pub Option<anyhow::Result<Vec<u8>>>);
        impl Future for Ready {
            type Output = anyhow::Result<Vec<u8>>;
            fn poll(mut self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<Self::Output> {
                Poll::Ready(self.0.take().expect("polled Ready twice"))
            }
        }
        pub fn ready(v: anyhow::Result<Vec<u8>>) -> Ready {
            Ready(Some(v))
        }
    }

    fn want(name: &str, size: u64) -> Want {
        Want {
            filename: name.to_owned(),
            size,
        }
    }

    #[tokio::test]
    async fn miss_then_hit() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 1024).unwrap();
        let (calls, fetch) = counting_fetch(10);
        let cancel = CancellationToken::new();

        let files = cache.acquire(&[want("a", 10)], &fetch, &cancel).await.unwrap();
        assert_eq!(files.len(), 1);
        assert_eq!(std::fs::read(files[0].path()).unwrap(), vec![b'x'; 10]);
        assert_eq!(calls.load(Ordering::SeqCst), 1);
        drop(files);

        // Second acquire is a hit — fetch not called again.
        let files = cache.acquire(&[want("a", 10)], &fetch, &cancel).await.unwrap();
        assert_eq!(files.len(), 1);
        assert_eq!(calls.load(Ordering::SeqCst), 1);
        assert_eq!(cache.total_bytes(), 10);
    }

    #[tokio::test]
    async fn evicts_lru_and_respects_pins() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 20).unwrap();
        let (_c, fetch) = counting_fetch(10);
        let cancel = CancellationToken::new();

        // Pin A (hold its handle), then cache B (drop handle → unpinned).
        let a = cache.acquire(&[want("a", 10)], &fetch, &cancel).await.unwrap();
        let b = cache.acquire(&[want("b", 10)], &fetch, &cancel).await.unwrap();
        drop(b);
        assert_eq!(cache.total_bytes(), 20);

        // C needs 10 bytes; A is pinned, B is not → B is evicted, A survives.
        let _c = cache.acquire(&[want("c", 10)], &fetch, &cancel).await.unwrap();
        assert!(cache.is_cached("a"), "pinned A must survive");
        assert!(!cache.is_cached("b"), "unpinned LRU B must be evicted");
        assert!(cache.is_cached("c"));
        assert_eq!(cache.total_bytes(), 20);
        drop(a);
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn single_flight_fetches_once() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 1024).unwrap();
        let calls = Arc::new(AtomicUsize::new(0));
        let cancel = CancellationToken::new();

        let mk = || {
            let cache = cache.clone();
            let calls = calls.clone();
            let cancel = cancel.clone();
            async move {
                let calls2 = calls.clone();
                let fetch = move |_n: &str| {
                    calls2.fetch_add(1, Ordering::SeqCst);
                    // Yield so the sibling task can coalesce onto the in-flight fetch.
                    async move {
                        tokio::task::yield_now().await;
                        anyhow::Ok(vec![b'x'; 10])
                    }
                };
                cache
                    .acquire(&[want("shared", 10)], &fetch, &cancel)
                    .await
                    .unwrap()
            }
        };

        let (f1, f2) = tokio::join!(tokio::spawn(mk()), tokio::spawn(mk()));
        assert_eq!(f1.unwrap().len(), 1);
        assert_eq!(f2.unwrap().len(), 1);
        assert_eq!(calls.load(Ordering::SeqCst), 1, "exactly one fetch for two callers");
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn reservation_waits_then_proceeds() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 20).unwrap();
        let (_c, fetch) = counting_fetch(10);
        let cancel = CancellationToken::new();

        // Fill and pin the whole budget with A and B.
        let a = cache.acquire(&[want("a", 10)], &fetch, &cancel).await.unwrap();
        let b = cache.acquire(&[want("b", 10)], &fetch, &cancel).await.unwrap();

        // C cannot be admitted until a pin releases; run it concurrently.
        let task = {
            let cache = cache.clone();
            let cancel = cancel.clone();
            let (_c2, fetch2) = counting_fetch(10);
            tokio::spawn(async move {
                cache.acquire(&[want("c", 10)], &fetch2, &cancel).await.unwrap()
            })
        };

        // Give C a chance to block, then free B.
        for _ in 0..8 {
            tokio::task::yield_now().await;
        }
        assert!(!task.is_finished(), "C must wait while budget is fully pinned");
        drop(b);

        let c = task.await.unwrap();
        assert_eq!(c.len(), 1);
        assert!(cache.is_cached("a"));
        assert!(!cache.is_cached("b"));
        assert!(cache.is_cached("c"));
        drop(a);
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn no_deadlock_two_full_budget_queries() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 20).unwrap();
        let cancel = CancellationToken::new();

        // Each query needs the whole budget (footprint == capacity). They must
        // serialize, but neither may deadlock holding a partial set.
        let run = |a: &'static str, b: &'static str| {
            let cache = cache.clone();
            let cancel = cancel.clone();
            tokio::spawn(async move {
                let fetch = |_n: &str| async move { anyhow::Ok(vec![b'x'; 10]) };
                let files = cache
                    .acquire(&[want(a, 10), want(b, 10)], &fetch, &cancel)
                    .await
                    .unwrap();
                files.len()
            })
        };

        let (q1, q2) = tokio::join!(run("a", "b"), run("c", "d"));
        assert_eq!(q1.unwrap(), 2);
        assert_eq!(q2.unwrap(), 2);
        assert!(cache.total_bytes() <= 20);
    }

    #[tokio::test]
    async fn fetch_failure_and_size_mismatch_degrade() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 1024).unwrap();
        let cancel = CancellationToken::new();

        // "bad" errors, "wrong" returns the wrong length, "good" succeeds.
        let fetch = |name: &str| {
            let name = name.to_owned();
            async move {
                match name.as_str() {
                    "bad" => anyhow::bail!("boom"),
                    "wrong" => anyhow::Ok(vec![b'x'; 3]), // expected 10
                    _ => anyhow::Ok(vec![b'x'; 10]),
                }
            }
        };

        let files = cache
            .acquire(
                &[want("bad", 10), want("wrong", 10), want("good", 10)],
                &fetch,
                &cancel,
            )
            .await
            .unwrap();
        assert_eq!(files.len(), 1, "only the good object survives");
        assert_eq!(files[0].filename(), "good");
        assert!(!cache.is_cached("bad"));
        assert!(!cache.is_cached("wrong"));
        assert_eq!(cache.total_bytes(), 10, "no reserved bytes leaked");
    }

    #[tokio::test]
    async fn footprint_over_capacity_is_too_large() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 15).unwrap();
        let (_c, fetch) = counting_fetch(10);
        let cancel = CancellationToken::new();

        let err = cache
            .acquire(&[want("a", 10), want("b", 10)], &fetch, &cancel)
            .await
            .unwrap_err();
        assert!(matches!(err, CacheError::TooLarge { footprint: 20, capacity: 15 }));
    }

    #[tokio::test]
    async fn cancelled_token_returns_cancelled() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 10).unwrap();
        let (_c, fetch) = counting_fetch(10);
        // A token cancelled before the call returns at Phase 1's first check.
        let cancel = CancellationToken::new();
        cancel.cancel();

        let err = cache
            .acquire(&[want("a", 10)], &fetch, &cancel)
            .await
            .unwrap_err();
        assert!(matches!(err, CacheError::Cancelled));
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn cancel_during_admission_wait_unblocks() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 10).unwrap();
        let (_c, fetch) = counting_fetch(10);

        // Pin the whole budget so a second query must wait in Phase 1 admission.
        let a = cache.acquire(&[want("a", 10)], &fetch, &CancellationToken::new()).await.unwrap();

        let cancel = CancellationToken::new();
        let task = {
            let cache = cache.clone();
            let cancel = cancel.clone();
            let (_c2, fetch2) = counting_fetch(10);
            tokio::spawn(async move { cache.acquire(&[want("b", 10)], &fetch2, &cancel).await })
        };
        for _ in 0..8 {
            tokio::task::yield_now().await;
        }
        assert!(!task.is_finished(), "B must be waiting for room");
        cancel.cancel();
        assert!(matches!(task.await.unwrap().unwrap_err(), CacheError::Cancelled));
        drop(a);
    }

    #[tokio::test]
    async fn duplicate_filenames_collapse_without_double_count() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 1024).unwrap();
        let (calls, fetch) = counting_fetch(10);
        let cancel = CancellationToken::new();

        let files = cache
            .acquire(&[want("a", 10), want("a", 10)], &fetch, &cancel)
            .await
            .unwrap();
        assert_eq!(files.len(), 1, "duplicate collapses to one handle");
        assert_eq!(calls.load(Ordering::SeqCst), 1, "fetched once");
        assert_eq!(cache.total_bytes(), 10, "total not double-counted");
    }

    #[tokio::test]
    async fn rejects_unsafe_filenames() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 1024).unwrap();
        let (_c, fetch) = counting_fetch(10);
        let cancel = CancellationToken::new();

        // Path-traversal / multi-component names degrade; only the safe one lands.
        let files = cache
            .acquire(
                &[
                    want("../escape", 10),
                    want("sub/dir", 10),
                    want("ok", 10),
                ],
                &fetch,
                &cancel,
            )
            .await
            .unwrap();
        assert_eq!(files.len(), 1);
        assert_eq!(files[0].filename(), "ok");
        // Nothing was written outside the cache dir.
        assert!(!dir.path().parent().unwrap().join("escape").exists());
        assert_eq!(cache.total_bytes(), 10);
    }

    /// Drive a deterministic single-flight scenario: an owner takes "shared"
    /// in-flight and then fails; a second query coalesces onto it (proven by its
    /// own "marker" file becoming cached) and is awaiting "shared" when the owner
    /// fails. Returns the awaiter's result and the cache for assertions.
    ///
    /// `awaiter_shared_ok` controls whether the awaiter's own fetch of "shared"
    /// (used on the retry-after-gone) succeeds.
    async fn run_coalesce_then_owner_fail(
        awaiter_shared_ok: bool,
    ) -> (Vec<CachedFile>, FileCache, tempfile::TempDir) {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 1024).unwrap();

        let started = Arc::new(tokio::sync::Notify::new());
        let proceed = Arc::new(tokio::sync::Notify::new());

        let owner = {
            let cache = cache.clone();
            let started = started.clone();
            let proceed = proceed.clone();
            tokio::spawn(async move {
                let fetch = move |_n: &str| {
                    let started = started.clone();
                    let proceed = proceed.clone();
                    async move {
                        started.notify_one();
                        proceed.notified().await;
                        anyhow::Result::<Vec<u8>>::Err(anyhow::anyhow!("owner fetch failed"))
                    }
                };
                cache
                    .acquire(&[want("shared", 10)], &fetch, &CancellationToken::new())
                    .await
                    .unwrap()
            })
        };

        started.notified().await; // owner is in-flight on "shared"

        let awaiter = {
            let cache = cache.clone();
            tokio::spawn(async move {
                let fetch = move |n: &str| {
                    let n = n.to_owned();
                    async move {
                        if n == "shared" && !awaiter_shared_ok {
                            anyhow::bail!("awaiter fetch also failed");
                        }
                        anyhow::Ok(vec![b'x'; 10])
                    }
                };
                cache
                    .acquire(
                        &[want("marker", 10), want("shared", 10)],
                        &fetch,
                        &CancellationToken::new(),
                    )
                    .await
                    .unwrap()
            })
        };
        // Once "marker" is cached the awaiter has admitted and is awaiting "shared".
        while !cache.is_cached("marker") {
            tokio::task::yield_now().await;
        }
        proceed.notify_one(); // owner fails now

        assert_eq!(owner.await.unwrap().len(), 0, "owner degrades on its own failure");
        let awaited = awaiter.await.unwrap();
        (awaited, cache, dir)
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 3)]
    async fn coalesced_waiter_refetches_when_owner_failed_but_self_succeeds() {
        // The owner's fetch failed, but the waiter's own fetch can produce
        // "shared" — it must re-fetch (not silently degrade) and get the file.
        let (awaited, cache, _dir) = run_coalesce_then_owner_fail(true).await;
        let names: Vec<&str> = awaited.iter().map(|f| f.filename()).collect();
        assert_eq!(awaited.len(), 2, "marker + re-fetched shared");
        assert!(names.contains(&"marker") && names.contains(&"shared"));
        assert!(cache.is_cached("shared"), "shared re-fetched and cached");
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 3)]
    async fn coalesced_waiter_degrades_when_refetch_also_fails() {
        // The owner failed and the waiter's own fetch of "shared" also fails →
        // after the bounded retry it degrades, keeping only "marker".
        let (awaited, cache, _dir) = run_coalesce_then_owner_fail(false).await;
        assert_eq!(awaited.len(), 1, "only marker survives");
        assert_eq!(awaited[0].filename(), "marker");
        assert!(!cache.is_cached("shared"), "unfetchable shared not cached");
        assert!(cache.is_cached("marker"));
    }

    #[tokio::test]
    async fn recovers_files_and_drops_temps_on_open() {
        let dir = tempdir().unwrap();
        {
            let cache = FileCache::open(dir.path(), 1024).unwrap();
            let (_c, fetch) = counting_fetch(10);
            let cancel = CancellationToken::new();
            let files = cache.acquire(&[want("a", 10)], &fetch, &cancel).await.unwrap();
            drop(files); // unpin so it survives as plain cached data
        }
        // Stray temp file from a hypothetical torn write.
        std::fs::write(dir.path().join("b.tmp"), b"partial").unwrap();

        let cache = FileCache::open(dir.path(), 1024).unwrap();
        assert!(cache.is_cached("a"), "surviving file re-registered");
        assert_eq!(cache.total_bytes(), 10);
        assert!(!dir.path().join("b.tmp").exists(), "stale temp removed");
    }

    #[tokio::test]
    async fn open_evicts_when_capacity_shrank() {
        let dir = tempdir().unwrap();
        {
            let cache = FileCache::open(dir.path(), 1024).unwrap();
            let (_c, fetch) = counting_fetch(10);
            let cancel = CancellationToken::new();
            for n in ["a", "b", "c"] {
                drop(cache.acquire(&[want(n, 10)], &fetch, &cancel).await.unwrap());
            }
            assert_eq!(cache.total_bytes(), 30);
        }
        // Reopen with a smaller budget — must evict back under it.
        let cache = FileCache::open(dir.path(), 15).unwrap();
        assert!(cache.total_bytes() <= 15);
    }

    #[tokio::test]
    async fn zero_capacity_allows_only_zero_byte_objects() {
        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 0).unwrap();
        let cancel = CancellationToken::new();
        let fetch = |_: &str| async { anyhow::Ok(Vec::new()) };

        let files = cache.acquire(&[want("a", 0)], &fetch, &cancel).await.unwrap();
        assert_eq!(files.len(), 1);
        assert_eq!(std::fs::read(files[0].path()).unwrap(), b"");
        assert_eq!(cache.total_bytes(), 0);

        let err = cache.acquire(&[want("b", 1)], &fetch, &cancel).await.unwrap_err();
        assert!(matches!(err, CacheError::TooLarge { .. }));
    }

    // Unix-only: making the cache dir read-only forces eviction unlinks to fail,
    // so `make_room` cannot reach its target. The hard cap MUST hold — the query
    // fails with EvictionFailed (a broken cache dir, not contention) rather than
    // over-committing or hanging.
    #[cfg(unix)]
    #[tokio::test]
    async fn eviction_failure_errors_without_breaching_cap() {
        use std::os::unix::fs::PermissionsExt;

        let dir = tempdir().unwrap();
        let cache = FileCache::open(dir.path(), 20).unwrap();
        let (_c, fetch) = counting_fetch(10);
        let cancel = CancellationToken::new();

        // Fill the budget with two unpinned files.
        for n in ["a", "b"] {
            drop(cache.acquire(&[want(n, 10)], &fetch, &cancel).await.unwrap());
        }
        assert_eq!(cache.total_bytes(), 20);

        // Read-only dir ⇒ unlink inside it fails ⇒ eviction cannot free room.
        std::fs::set_permissions(dir.path(), std::fs::Permissions::from_mode(0o500)).unwrap();

        let err = cache.acquire(&[want("c", 10)], &fetch, &cancel).await.unwrap_err();
        assert!(matches!(err, CacheError::EvictionFailed), "got {err:?}");
        assert_eq!(cache.total_bytes(), 20, "cap not breached");

        // Restore perms so the tempdir can be cleaned up.
        std::fs::set_permissions(dir.path(), std::fs::Permissions::from_mode(0o700)).unwrap();
    }
}
