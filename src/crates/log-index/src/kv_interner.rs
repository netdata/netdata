//! A key=value interner keyed by pre-computed xxhash64 values.
//!
//! # Role in the pre-computed hash optimization
//!
//! The indexer interns every `"key=value"` attribute string into a unique
//! `u32` ID. The same `key=value` pair appears on thousands of log records,
//! so interning avoids storing redundant copies and enables cheap ID-based
//! bitmap operations.
//!
//! The producer (`otel-ingestor`) pre-computes `xxhash64("key=value")` for
//! every attribute and ships the hashes alongside the data (see
//! `otel-ingestor/src/arrow_bridge.rs`). This interner is designed to
//! exploit those pre-computed hashes:
//!
//! - **[`lookup_hash`](KeyValueInterner::lookup_hash)**: Hash-only lookup.
//!   Returns the intern ID if this hash was already seen and has no
//!   collisions. This is the fast path — no string needed at all.
//!
//! - **[`intern_with_hash`](KeyValueInterner::intern_with_hash)**: Intern a
//!   string with a pre-computed hash. Used on the first encounter of a
//!   `key=value` pair (cache miss). The hash is stored so future
//!   `lookup_hash` calls will hit.
//!
//! - **[`intern`](KeyValueInterner::intern)**: Compute the hash from the
//!   string and intern it. Fallback for attributes without pre-computed
//!   hashes.
//!
//! # Identity hasher
//!
//! The internal `HashbrownMap<u64, u32>` uses an identity hasher — it passes
//! the `u64` key through as-is instead of re-hashing it. This is safe
//! because xxhash64 already provides excellent distribution, and avoids
//! the overhead of hashing a hash.
//!
//! # Hash collision handling
//!
//! The primary map stores one intern ID per hash (the common case). A
//! separate overflow map handles the astronomically rare case of genuine
//! xxhash64 collisions (different strings producing the same hash). When
//! collisions exist for a hash, `lookup_hash` returns `None` to force the
//! caller through the string-based slow path for disambiguation.

use std::collections::HashMap;
use std::hash::{BuildHasher, Hasher};

use hashbrown::HashMap as HashbrownMap;

use bumpalo::Bump;
use hashbrown::hash_map::RawEntryMut;
use twox_hash::XxHash64;

/// A unique ID assigned by [`KeyValueInterner`] to each distinct `key=value`
/// string. Used as the index into the per-value bitmap array and as the
/// elements in the per-log entries.
///
/// Not to be confused with `FileId` (tier-aligned IDs written to disk) or
/// raw log positions (array indices into the log list).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct KeyValueId(pub u32);

impl KeyValueId {
    /// Convert to `usize` for array indexing.
    #[inline]
    pub fn idx(self) -> usize {
        self.0 as usize
    }
}

impl From<usize> for KeyValueId {
    #[inline]
    fn from(id: usize) -> Self {
        KeyValueId(id as u32)
    }
}

#[derive(Default)]
struct IdentityHasher(u64);

impl Hasher for IdentityHasher {
    #[inline]
    fn finish(&self) -> u64 {
        self.0
    }

    #[inline]
    fn write_u64(&mut self, val: u64) {
        self.0 = val;
    }

    fn write(&mut self, _: &[u8]) {
        unreachable!("IdentityHasher only supports u64 keys");
    }
}

#[derive(Clone, Default)]
struct BuildIdentityHasher;

impl BuildHasher for BuildIdentityHasher {
    type Hasher = IdentityHasher;

    fn build_hasher(&self) -> Self::Hasher {
        IdentityHasher::default()
    }
}

/// Maps `key=value` strings to unique `u32` IDs, keyed by pre-computed xxhash64.
///
/// See the [module-level documentation](self) for how this fits into the
/// pre-computed hash optimization pipeline.
pub struct KeyValueInterner<'a> {
    arena: &'a Bump,
    /// xxhash64 → first intern ID with this hash
    map: HashbrownMap<u64, u32, BuildIdentityHasher>,
    /// xxhash64 → additional intern IDs (collision overflow, almost always empty)
    collisions: HashbrownMap<u64, Vec<u32>, BuildIdentityHasher>,
    /// intern ID → canonical string
    strings: Vec<&'a str>,
    /// field name → list of key=value IDs with that field.
    /// Built incrementally on each new interning (miss only).
    field_ids: HashMap<&'a str, Vec<KeyValueId>>,
    /// Fields with fewer unique values than this go into the primary FST.
    cardinality_threshold: u32,
}

impl<'a> KeyValueInterner<'a> {
    pub fn new(arena: &'a Bump, cardinality_threshold: u32) -> Self {
        Self {
            arena,
            map: HashbrownMap::with_hasher(BuildIdentityHasher),
            collisions: HashbrownMap::with_hasher(BuildIdentityHasher),
            strings: Vec::new(),
            field_ids: HashMap::new(),
            cardinality_threshold,
        }
    }

    /// Compute xxhash64 of `s` and intern it.
    pub fn intern(&mut self, s: &str) -> KeyValueId {
        let hash = xxhash64(s.as_bytes());
        self.intern_with_hash(hash, s)
    }

    /// Intern a string with a pre-computed xxhash64 value.
    pub fn intern_with_hash(&mut self, hash: u64, s: &str) -> KeyValueId {
        let kv_id = match self.map.raw_entry_mut().from_hash(hash, |&k| k == hash) {
            RawEntryMut::Occupied(entry) => {
                let &existing_id = entry.get();

                if self.strings[existing_id as usize] == s {
                    return KeyValueId(existing_id);
                }

                // Primary doesn't match — check collision overflow.
                if let Some(ids) = self.collisions.get(&hash) {
                    for &cid in ids {
                        if self.strings[cid as usize] == s {
                            return KeyValueId(cid);
                        }
                    }
                }

                // Genuine collision: different string, same hash.
                let id = self.strings.len() as u32;
                let interned = self.arena.alloc_str(s);
                self.strings.push(interned);
                self.collisions.entry(hash).or_default().push(id);
                KeyValueId(id)
            }
            RawEntryMut::Vacant(entry) => {
                let id = self.strings.len() as u32;
                let interned = self.arena.alloc_str(s);
                self.strings.push(interned);
                entry.insert_hashed_nocheck(hash, hash, id);
                KeyValueId(id)
            }
        };

        self.track_field(kv_id);
        kv_id
    }

    /// Fast path: look up by hash alone, without needing the string.
    ///
    /// Returns `Some(id)` only if exactly one string maps to this hash
    /// (no collision ambiguity). Returns `None` if the hash is unknown or
    /// if there are collisions requiring string disambiguation.
    #[inline]
    pub fn lookup_hash(&mut self, hash: u64) -> Option<KeyValueId> {
        let &id = self.map.get(&hash)?;
        if self.collisions.contains_key(&hash) {
            None
        } else {
            Some(KeyValueId(id))
        }
    }

    /// Register a newly interned string in the field_ids map.
    fn track_field(&mut self, id: KeyValueId) {
        let s = self.strings[id.idx()];
        let field = match s.find('=') {
            Some(pos) => &s[..pos],
            None => s,
        };
        self.field_ids.entry(field).or_default().push(id);
    }

    pub fn resolve(&self, id: KeyValueId) -> &str {
        self.strings[id.idx()]
    }

    pub fn len(&self) -> usize {
        self.strings.len()
    }

    pub fn strings(&self) -> &[&'a str] {
        &self.strings
    }

    pub fn cardinality_threshold(&self) -> u32 {
        self.cardinality_threshold
    }

    /// Low-cardinality fields (< threshold), sorted by field name.
    pub fn low_fields(&self) -> Vec<(&str, &[KeyValueId])> {
        self.fields_in_range(0, self.cardinality_threshold as usize)
    }

    /// Mid-cardinality fields ([threshold, 10*threshold)), sorted by field name.
    pub fn mid_fields(&self) -> Vec<(&str, &[KeyValueId])> {
        let t = self.cardinality_threshold as usize;
        self.fields_in_range(t, t * 10)
    }

    /// High-cardinality fields (>= 10*threshold), sorted by field name.
    pub fn high_fields(&self) -> Vec<(&str, &[KeyValueId])> {
        let t = self.cardinality_threshold as usize;
        self.fields_in_range(t * 10, usize::MAX)
    }

    /// Assign tier-aligned positions to all key=value IDs.
    ///
    /// Walks low → mid → high tiers. Within each tier, fields are sorted by
    /// name; within each field, values are sorted by their resolved string.
    /// Returns three vectors of [`KeyValueId`]s, one per tier.
    pub fn tier_assignment(&self) -> [Vec<KeyValueId>; 3] {
        let mut sorted: Vec<KeyValueId> = Vec::new();

        let mut collect_tier = |tier: &[(&str, &[KeyValueId])]| -> Vec<KeyValueId> {
            let mut order = Vec::new();
            for &(_, ids) in tier {
                sorted.clear();
                sorted.extend_from_slice(ids);
                sorted.sort_by(|a, b| self.strings[a.idx()].cmp(self.strings[b.idx()]));
                order.extend_from_slice(&sorted);
            }
            order
        };

        let low = collect_tier(&self.low_fields());
        let mid = collect_tier(&self.mid_fields());
        let high = collect_tier(&self.high_fields());

        [low, mid, high]
    }

    /// Collect fields whose value count is in [lo, hi), sorted by name.
    fn fields_in_range(&self, lo: usize, hi: usize) -> Vec<(&str, &[KeyValueId])> {
        let mut result: Vec<(&str, &[KeyValueId])> = self
            .field_ids
            .iter()
            .filter(|(_, ids)| ids.len() >= lo && ids.len() < hi)
            .map(|(&field, ids)| (field, ids.as_slice()))
            .collect();
        result.sort_unstable_by_key(|(field, _)| *field);
        result
    }
}

/// Compute xxhash64 (seed 0) of a byte slice.
#[inline]
pub fn xxhash64(bytes: &[u8]) -> u64 {
    let mut h = XxHash64::default();
    h.write(bytes);
    h.finish()
}
