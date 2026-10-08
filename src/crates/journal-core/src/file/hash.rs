//! The hash algorithms the journal file format pins, plus
//! [`journal_hash_data`], the dispatcher behind `JournalFile::hash`
//! (file/file.rs).
//!
//! Format contract, matching systemd's `journal_file_hash_data`
//! (src/libsystemd/sd-journal/journal-file.c in the systemd tree): a file
//! whose header carries `HEADER_INCOMPATIBLE_KEYED_HASH` hashes payloads with
//! SipHash-2-4 keyed by the header's `file_id`; legacy files use Jenkins
//! lookup3. Both sides default to keyed for newly created files (systemd:
//! `$SYSTEMD_JOURNAL_KEYED_HASH` unset means on; `enable_keyed_hash`,
//! file/file.rs), so the implementations must agree bit-for-bit for files
//! written by one side to stay readable by the other.
//!
//! The choice is per file and permanent: `file_id` is a fresh UUIDv4 at
//! create and never changes; `create_successor` inherits the keyed/legacy
//! mode flag for the next file but gives it a new id (file/file.rs). Stored
//! object `hash` fields and hash-table buckets (`hash % n_buckets`,
//! file/object.rs) were computed with the file's own hash mode (keyed by
//! `file_id`, or unkeyed for legacy files), and lookups re-check the
//! recomputed hash against the stored one (file/file.rs, `PayloadMatcher`)
//! — changing the key or either algorithm strands every existing object.
//!
//! Consumers (grep-verified): [`journal_hash_data`] has one caller,
//! `JournalFile::hash` (file/file.rs), which reads the header flag and
//! `file_id`; its callers store hashes in data/field object headers and
//! hash-table tails for dedup-on-write (file/writer.rs), build filter
//! matches via `find_data_offset` (file/filter.rs), and look up the
//! `ND_REMAPPING=1` marker and field names (file/file.rs `load_fields`,
//! `field_data_objects`). [`jenkins_hash64`] is also called directly by
//! file/writer.rs for entry `xor_hash`. Near-twin:
//! src/crates/jf/journal_file/src/hash.rs duplicates all three functions;
//! keep the copies in step.
use siphasher::sip::SipHasher24;
use std::hash::Hasher;

/// Jenkins lookup3 (`hashlittle2`) over `data`, packed as `pc << 32 | pb`,
/// the same packing systemd's `jenkins_hash64` uses
/// (src/libsystemd/sd-journal/lookup3.h). Consumed by [`journal_hash_data`]
/// for legacy (unkeyed) object hashes and directly by file/writer.rs for
/// entry `xor_hash`, which systemd computes key-independently so cursors and
/// entry identity stay identical across keyed and legacy files.
///
/// Divergence from systemd: empty input hashes to 0 (the hasher skips empty
/// writes), while systemd's `hashlittle2` returns its un-mixed
/// 0xdeadbeef/0xdeadbeef state; the jf twin mirrors systemd here, this copy
/// does not.
pub fn jenkins_hash64(data: &[u8]) -> u64 {
    use hashers::jenkins::Lookup3Hasher;

    let mut hasher = Lookup3Hasher::default();
    hasher.write(data);
    let hash = hasher.finish();

    // The `hashers` crate packs the halves opposite to systemd
    // (finish() = pb << 32 | pc); swap so the result matches the format.
    let low = (hash & 0xFFFFFFFF) as u32;
    let high = (hash >> 32) as u32;
    ((low as u64) << 32) | (high as u64)
}

/// SipHash-2-4 over `data` keyed by `key` — the header `file_id` in
/// practice — split little-endian into the two 64-bit key words, matching
/// systemd's `siphash24_init` (src/basic/siphash24.c). Single-shot; only
/// [`journal_hash_data`] calls it.
pub fn siphash24(data: &[u8], key: &[u8; 16]) -> u64 {
    let k0 = u64::from_le_bytes(key[0..8].try_into().unwrap());
    let k1 = u64::from_le_bytes(key[8..16].try_into().unwrap());

    let mut hasher = SipHasher24::new_with_keys(k0, k1);
    hasher.write(data);
    hasher.finish()
}

/// Dispatches to the file's hash mode: keyed files hash with [`siphash24`]
/// under `file_id`, legacy files with [`jenkins_hash64`] — the same split
/// systemd makes in `journal_file_hash_data`. `JournalFile::hash`
/// (file/file.rs) is the only caller; call that instead so readers and
/// writers follow one per-file decision taken from the header.
pub fn journal_hash_data(data: &[u8], is_keyed_hash: bool, file_id: Option<&[u8; 16]>) -> u64 {
    if is_keyed_hash {
        if let Some(file_id) = file_id {
            siphash24(data, file_id)
        } else {
            // Unreachable via the only caller: a keyed file always carries a
            // header file_id, and systemd has no equivalent branch. Kept so
            // the function stays total.
            jenkins_hash64(data)
        }
    } else {
        jenkins_hash64(data)
    }
}
