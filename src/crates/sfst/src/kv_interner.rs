//! The build-side `key=value` interner: assigns every distinct attribute
//! string a unique [`KvSlot`] ID, so rows reference attributes by cheap
//! `u32`s, duplicates share one stored string, and the indexer keeps one
//! bitmap per distinct pair instead of one per row.
//!
//! # Pre-computed hash fast path
//!
//! The producer (`ng-flatten`) pre-computes `xxhash64("key=value")` for
//! every attribute and carries the hash alongside the data (`hash_kv` in
//! `src/crates/ng-flatten/src/common.rs`). The builder feed
//! (`src/crates/ng-index/src/sfst_build.rs`) hands it to this interner:
//!
//! - [`lookup_hash`](KeyValueInterner::lookup_hash): resolve a hash to a
//!   slot with no string at all — the fast path.
//! - [`intern_with_hash`](KeyValueInterner::intern_with_hash): first
//!   encounter of a pair; stores the hash so later lookups hit.
//! - [`intern`](KeyValueInterner::intern): hashes the string itself —
//!   fallback when no pre-computed hash is available.
//!
//! # Identity hasher
//!
//! The maps hash their `u64` keys with a pass-through identity hasher —
//! xxhash64 already spread the keys, so re-hashing a hash is pure
//! overhead. With it, the standard `entry(hash)` matches the removed
//! hashbrown raw-entry API (`from_hash(hash, |&k| k == hash)`).
//!
//! # Collision handling
//!
//! The primary map stores one slot per hash (the common case). A separate
//! overflow map handles genuine xxhash64 collisions — different strings
//! with the same hash, astronomically rare. For an ambiguous hash,
//! `lookup_hash` returns `None` to force the caller through the
//! string-comparing slow path.
//!
//! # Cardinality tiers and canonical order
//!
//! Besides interning, the interner groups slots by field (the part before
//! the first `=`) and classifies each field by slot count against
//! `cardinality_threshold` (`T`; default 100 — `schema.rs`):
//! low `< T`, mid `[T, 10·T)`, high `≥ 10·T` — the same taxonomy as
//! [`FieldTier`](crate::FieldTier). [`tier_assignment`](KeyValueInterner::tier_assignment)
//! orders all slots canonically — low → mid → high, fields by name,
//! values sorted — and that ordering is the contract the on-disk ID
//! translation depends on: `build_id_translation` in
//! `src/crates/sfst/src/build.rs` numbers the concatenated tiers 0.. and
//! assigns `table[slot.idx()] = KvId(i)`, recording the cumulative tier
//! boundaries as `IdRanges`. The tier also routes the on-disk home: low
//! fields into the primary FST (`PRIM`), mid fields into per-field FST
//! chunks (`MF{hi}{lo}`), high fields into columnar chunks (`HF{hi}{lo}`).
//!
//! [`KeyValueInterner`] is crate-internal, owned by `RowIndex`
//! (`src/crates/sfst/src/row_index.rs`); [`KvSlot`] is the module's only
//! public export. Interned strings are arena-allocated and live as long
//! as the build.

use std::collections::HashMap;
use std::hash::{BuildHasher, Hasher};

use hashbrown::HashMap as HashbrownMap;

use bumpalo::Bump;
use hashbrown::hash_map::Entry;
use twox_hash::XxHash64;

/// A unique ID the build-side interner assigns to each distinct
/// `key=value` string. Indexes the per-pair bitmap array, appears in the
/// per-row slot lists, and is held by the span event/link rows and trace
/// rollup; the build translates every slot to its file `KvId` before
/// writing.
///
/// Not to be confused with raw log positions (array indices into the
/// row list), which are also `u32`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct KvSlot(pub u32);

impl KvSlot {
    /// Convert to `usize` for array indexing.
    #[inline]
    pub fn idx(self) -> usize {
        self.0 as usize
    }
}

impl From<usize> for KvSlot {
    #[inline]
    fn from(id: usize) -> Self {
        KvSlot(id as u32)
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

/// Maps `key=value` strings to unique [`KvSlot`] IDs, keyed by
/// pre-computed xxhash64. The module docs cover the hash fast path,
/// collision handling, and the tier/canonical-order contract.
pub struct KeyValueInterner<'a> {
    arena: &'a Bump,
    /// xxhash64 → first intern ID with this hash
    map: HashbrownMap<u64, u32, BuildIdentityHasher>,
    /// xxhash64 → additional intern IDs (collision overflow, almost always empty)
    collisions: HashbrownMap<u64, Vec<u32>, BuildIdentityHasher>,
    /// intern ID → canonical string
    strings: Vec<&'a str>,
    /// Field name → the key=value slots carrying it. The field is the
    /// part before the first `=`. Slots are appended only when a new one
    /// is created (string-match hits skip `track_field`).
    field_slots: HashMap<&'a str, Vec<KvSlot>>,
    /// Tier boundary `T`: a field with `< T` slots is low (primary FST),
    /// `[T, 10·T)` mid, `≥ 10·T` high.
    cardinality_threshold: u32,
}

impl<'a> KeyValueInterner<'a> {
    pub fn new(arena: &'a Bump, cardinality_threshold: u32) -> Self {
        Self {
            arena,
            map: HashbrownMap::with_hasher(BuildIdentityHasher),
            collisions: HashbrownMap::with_hasher(BuildIdentityHasher),
            strings: Vec::new(),
            field_slots: HashMap::new(),
            cardinality_threshold,
        }
    }

    /// Compute xxhash64 of `s` and intern it — the fallback when the
    /// caller has no pre-computed hash.
    pub fn intern(&mut self, s: &str) -> KvSlot {
        let hash = xxhash64(s.as_bytes());
        self.intern_with_hash(hash, s)
    }

    /// Intern a string with a pre-computed xxhash64 value.
    ///
    /// The string-match early returns below deliberately skip `track_field`:
    /// a slot is registered in `field_slots` exactly once, when first interned.
    /// Re-tracking a returned slot would double-count it under its field.
    pub fn intern_with_hash(&mut self, hash: u64, s: &str) -> KvSlot {
        let kv_slot = match self.map.entry(hash) {
            Entry::Occupied(entry) => {
                let &existing_id = entry.get();

                if self.strings[existing_id as usize] == s {
                    return KvSlot(existing_id);
                }

                // Primary doesn't match — check collision overflow.
                if let Some(ids) = self.collisions.get(&hash) {
                    for &cid in ids {
                        if self.strings[cid as usize] == s {
                            return KvSlot(cid);
                        }
                    }
                }

                // Genuine collision: different string, same hash.
                let id = self.strings.len() as u32;
                let interned = self.arena.alloc_str(s);
                self.strings.push(interned);
                self.collisions.entry(hash).or_default().push(id);
                KvSlot(id)
            }
            Entry::Vacant(entry) => {
                let id = self.strings.len() as u32;
                let interned = self.arena.alloc_str(s);
                self.strings.push(interned);
                entry.insert(id);
                KvSlot(id)
            }
        };

        self.track_field(kv_slot);
        kv_slot
    }

    /// Fast path: look up by hash alone, without needing the string.
    ///
    /// Returns `Some(slot)` only if exactly one string maps to this hash
    /// (no collision ambiguity). Returns `None` if the hash is unknown or
    /// if there are collisions requiring string disambiguation.
    #[inline]
    pub fn lookup_hash(&mut self, hash: u64) -> Option<KvSlot> {
        let &id = self.map.get(&hash)?;
        if self.collisions.contains_key(&hash) {
            None
        } else {
            Some(KvSlot(id))
        }
    }

    /// Register a newly created slot under its field — the part before
    /// the first `=`, or the whole string when there is no `=`.
    fn track_field(&mut self, slot: KvSlot) {
        let s = self.strings[slot.idx()];
        let field = match s.find('=') {
            Some(pos) => &s[..pos],
            None => s,
        };
        self.field_slots.entry(field).or_default().push(slot);
    }

    /// Return the canonical string for a slot.
    pub fn resolve(&self, slot: KvSlot) -> &str {
        self.strings[slot.idx()]
    }

    /// Low-cardinality fields (< threshold), sorted by field name.
    pub fn low_fields(&self) -> Vec<(&str, &[KvSlot])> {
        self.fields_in_range(0, self.cardinality_threshold as usize)
    }

    /// Mid-cardinality fields ([threshold, 10*threshold)), sorted by field name.
    pub fn mid_fields(&self) -> Vec<(&str, &[KvSlot])> {
        let t = self.cardinality_threshold as usize;
        self.fields_in_range(t, t * 10)
    }

    /// High-cardinality fields (>= 10*threshold), sorted by field name.
    pub fn high_fields(&self) -> Vec<(&str, &[KvSlot])> {
        let t = self.cardinality_threshold as usize;
        self.fields_in_range(t * 10, usize::MAX)
    }

    /// Assign every key=value slot its canonical position.
    ///
    /// Walks low → mid → high tiers. Within each tier, fields are sorted by
    /// name; within each field, values are sorted by their resolved string.
    /// Returns three vectors of [`KvSlot`]s, one per tier.
    ///
    /// This ordering is the on-disk `KvId` order: `build_id_translation`
    /// in `src/crates/sfst/src/build.rs` numbers the concatenated tiers.
    pub fn tier_assignment(&self) -> [Vec<KvSlot>; 3] {
        // Reusable decorate buffer of (value suffix, slot). Cheaper than
        // sorting the slots directly by `self.strings[idx]`:
        //   - every value in the field shares the same `field=` prefix,
        //     so sorting the remaining suffix gives the same order as
        //     sorting full strings;
        //   - the `&str` sits inline in the tuple, so comparisons don't
        //     re-index `self.strings`;
        //   - `sort_unstable_by` skips the stable sort's scratch allocation
        //     and is safe here: every value in a field is a distinct string
        //     (no ties).
        let mut scratch: Vec<(&str, KvSlot)> = Vec::new();

        let mut collect_tier = |tier: &[(&str, &[KvSlot])]| -> Vec<KvSlot> {
            let mut order = Vec::new();
            for &(field, ids) in tier {
                let prefix = field.len();
                scratch.clear();
                scratch.extend(
                    ids.iter()
                        .map(|&id| (&self.strings[id.idx()][prefix..], id)),
                );
                scratch.sort_unstable_by(|a, b| a.0.cmp(b.0));
                order.extend(scratch.iter().map(|&(_, id)| id));
            }
            order
        };

        let low = collect_tier(&self.low_fields());
        let mid = collect_tier(&self.mid_fields());
        let high = collect_tier(&self.high_fields());

        [low, mid, high]
    }

    /// Collect fields whose value count is in [lo, hi), sorted by name.
    fn fields_in_range(&self, lo: usize, hi: usize) -> Vec<(&str, &[KvSlot])> {
        let mut result: Vec<(&str, &[KvSlot])> = self
            .field_slots
            .iter()
            .filter(|(_, ids)| ids.len() >= lo && ids.len() < hi)
            .map(|(&field, ids)| (field, ids.as_slice()))
            .collect();
        result.sort_unstable_by_key(|(field, _)| *field);
        result
    }
}

/// Compute xxhash64 (seed 0) of a byte slice — the same hash `ng-flatten`'s
/// `hash_kv` pre-computes for a pair. A mismatch here silently disables the
/// `lookup_hash` fast path and lets one pair intern twice (once per hash).
#[inline]
fn xxhash64(bytes: &[u8]) -> u64 {
    let mut h = XxHash64::default();
    h.write(bytes);
    h.finish()
}
