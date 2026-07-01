//! The `trace_id` index (`TIDX` chunk): O(log) trace-by-id lookup over the
//! time-ordered `TRCE` column.
//!
//! Logs never needed it; traces do — a span carries a near-unique 16-byte
//! `trace_id` and the core tracing query is "give me every span of this trace".
//! The `TRCE` per-row column stays **chronological** (position-aligned to `TIMS`
//! and every other column); this index adds the *sorted order* in a small
//! auxiliary chunk so a lookup never scans.
//!
//! Structure (design note `~/mo/traces-sfst.md` §5):
//! - `sort_perm[N]` — row positions sorted by their 16-byte `trace_id` value,
//!   with the row position as a tiebreaker, so within one trace the spans stay
//!   chronological (structural, not reliant on sort stability — see `build`).
//!   Only rows with a set (non-zero) id are indexed; the all-zero W3C "unset"
//!   sentinel forms no trace and is skipped.
//! - `fanout[256]` — cumulative count of indexed positions whose `trace_id`
//!   first byte is `<= b` (git-packfile fanout). Narrows a lookup to the
//!   first-byte bucket before the binary search.
//!
//! All spans of one trace are a **contiguous run** in `sort_perm`, so the
//! "posting list" for a trace is that slice — the permutation IS the index, with
//! no per-id dictionary or offsets stored (the `TRCE` column is the dictionary,
//! compared indirectly).

use std::ops::Range;

use serde::de::Error as _;
use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::{Error, TraceId, TraceIds};

/// First-byte fanout over the indexed `trace_id`s (git-packfile style): 256
/// cumulative counts where `self.0[b]` = number of indexed positions whose
/// `trace_id` first byte is `<= b`. The bucket for byte `b` is the half-open
/// run `[self.0[b-1], self.0[b])` ([`Fanout::bucket`]).
///
/// A fixed `[u32; 256]`, so the length-256 invariant is a compile-time
/// guarantee rather than a runtime check. Every `Fanout` — whether built
/// ([`Fanout::build`]) or decoded ([`Deserialize`]) — is **monotonic
/// non-decreasing**: `build` produces it by cumulative sum, and `deserialize`
/// rejects a non-monotonic encoding at the parse boundary, so [`Fanout::bucket`]
/// can never yield an inverted (`lo > hi`) range. Serde has no `[T; 256]` impl
/// (arrays only to `[T; 32]`), so the codec is a hand-written
/// `&[u32]`-slice round-trip — wire-identical to a `Vec<u32>`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Fanout([u32; 256]);

impl Fanout {
    /// Build the cumulative fanout from the first byte of every indexed id.
    /// Order-independent (it is a histogram + prefix sum). The `u32` accumulator
    /// cannot overflow: the total is `sort_perm.len() <= record_count`, itself a
    /// `u32` (the format's row cap).
    fn build(first_bytes: impl Iterator<Item = u8>) -> Self {
        let mut counts = [0u32; 256];
        for b in first_bytes {
            counts[b as usize] += 1;
        }
        let mut acc = 0u32;
        for slot in &mut counts {
            acc += *slot;
            *slot = acc;
        }
        Fanout(counts)
    }

    /// The half-open `sort_perm` range holding the positions whose `trace_id`
    /// first byte is `first_byte`. Never inverts (`Fanout` is always monotonic),
    /// and the `u32 → usize` casts never lose bits: `TraceIdIndex::validate`
    /// guarantees `total() == sort_perm.len()`, so every `hi` is a valid
    /// `sort_perm` endpoint.
    fn bucket(&self, first_byte: u8) -> Range<usize> {
        let b = first_byte as usize;
        let lo = if b == 0 { 0 } else { self.0[b - 1] as usize };
        let hi = self.0[b] as usize;
        lo..hi
    }

    /// Total indexed positions (`self.0[255]`); must equal `sort_perm.len()`.
    fn total(&self) -> u32 {
        self.0[255]
    }
}

impl Serialize for Fanout {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        // Wire-identical to `Vec<u32>` under bincode (len prefix + elements).
        self.0.as_slice().serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for Fanout {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let v = Vec::<u32>::deserialize(deserializer)?;
        let arr: [u32; 256] = v
            .try_into()
            .map_err(|v: Vec<u32>| D::Error::invalid_length(v.len(), &"256 fanout entries"))?;
        // Enforce monotonicity at the parse boundary so `bucket()` cannot invert
        // (panic-safety for the `sort_perm[lo..hi]` slice). Cheap O(256).
        if arr.windows(2).any(|w| w[0] > w[1]) {
            return Err(D::Error::custom("fanout is not monotonic non-decreasing"));
        }
        Ok(Fanout(arr))
    }
}

/// The `trace_id` index: a first-byte fanout over row positions sorted by their
/// 16-byte `trace_id`. Built at seal from the `TRCE` column, read on lookup.
///
/// Self-contained for the bucket step (`fanout`); the comparison step reads the
/// `TRCE` column the index was built from (positions are indices into it).
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TraceIdIndex {
    /// First-byte fanout over the indexed ids (see [`Fanout`]).
    fanout: Fanout,
    /// Row positions sorted ascending by 16-byte `trace_id`. Length is the
    /// number of indexed (set-id) rows, `<= record_count`.
    sort_perm: Vec<u32>,
}

impl TraceIdIndex {
    /// Build the index from the (chronological) `TRCE` column. Rows with an
    /// unset (all-zero) id are excluded. `O(N log N)` 16-byte comparisons.
    ///
    /// The arena passed here MUST be the same one stored in the file's `TRCE`
    /// chunk (same row order), since `sort_perm` indexes into it.
    pub fn build(trace_ids: &TraceIds) -> Self {
        // Pair each set id with its position, then sort by `(id, position)`: the
        // 16-byte key is read exactly once per row (not on every comparison, as
        // `sort_by_key` would), and the position tiebreaker makes a trace's spans
        // chronological *structurally* — not by relying on sort stability.
        // (`record_count` is `u32`, so `trace_ids.len()` fits `u32` by the format
        // row cap.)
        let mut keyed: Vec<(TraceId, u32)> = (0..trace_ids.len() as u32)
            .filter_map(|i| {
                let id = trace_ids.get(i as usize);
                (!id.is_unset()).then_some((id, i))
            })
            .collect();
        keyed.sort_unstable();

        let fanout = Fanout::build(keyed.iter().map(|(id, _)| id.as_bytes()[0]));
        let sort_perm: Vec<u32> = keyed.into_iter().map(|(_, p)| p).collect();

        // Enforce the index invariants at write time (debug builds + tests),
        // where a producer bug is cheap to catch at its source — rather than
        // paying to re-prove them on every read. `sort_perm` is a permutation of
        // a filtered subset of `0..len`, sorted by id, so all three hold by
        // construction; the asserts guard against a future refactor regressing
        // build(). Sortedness is the invariant `positions()` actually relies on
        // (its two binary searches assume a sorted bucket).
        debug_assert!(
            sort_perm.iter().all(|&p| (p as usize) < trace_ids.len()),
            "build produced an out-of-range position"
        );
        debug_assert!(
            sort_perm
                .windows(2)
                .all(|w| trace_ids.get(w[0] as usize) <= trace_ids.get(w[1] as usize)),
            "build produced an unsorted permutation"
        );
        debug_assert!(
            {
                let mut seen = sort_perm.clone();
                seen.sort_unstable();
                seen.windows(2).all(|w| w[0] != w[1])
            },
            "build produced duplicate positions"
        );

        Self { fanout, sort_perm }
    }

    /// Positions (chronological) of every span whose `trace_id == needle`, or an
    /// empty slice if the trace is absent. `trace_ids` MUST be the `TRCE` column
    /// this index was built from.
    ///
    /// `fanout` narrows to the first-byte bucket, then two indirect binary
    /// searches bound the equal run: `fanout + 2·log2(N/256)` comparisons.
    pub fn positions<'s>(&'s self, needle: TraceId, trace_ids: &TraceIds) -> &'s [u32] {
        if needle.is_unset() {
            return &[];
        }
        let bucket = &self.sort_perm[self.fanout.bucket(needle.as_bytes()[0])];
        // `bucket` is sorted ascending by id, so both predicates are monotonic.
        let start = bucket.partition_point(|&p| trace_ids.get(p as usize) < needle);
        let end = bucket.partition_point(|&p| trace_ids.get(p as usize) <= needle);
        &bucket[start..end]
    }

    /// Number of indexed (set-id) rows.
    pub fn indexed_rows(&self) -> usize {
        self.sort_perm.len()
    }

    /// Validate a decoded index against the file's `record_count` at the trust
    /// boundary: a malformed (but CRC-valid) chunk must surface as
    /// [`Error::CorruptIndex`] so the query layer skips the file rather than
    /// indexing out of bounds or trusting a broken fanout.
    ///
    /// Scope is **panic-safety**: it guarantees `positions()` cannot index out
    /// of bounds. [`Fanout`] already self-guarantees length-256 + monotonicity
    /// (so a bucket slice never inverts); this adds the fanout↔`sort_perm`
    /// relation (`total == sort_perm.len()`, so a bucket's `hi` never exceeds
    /// `sort_perm`) and the position↔`record_count` range (so the `TRCE` access
    /// is in bounds). It deliberately does NOT re-verify the *semantic*
    /// invariants (`sort_perm` actually sorted by id, buckets matching
    /// first-byte, positions unique) — those would cost an extra O(N) pass on
    /// every read and are instead enforced at write time (`build()`'s
    /// `debug_assert`s) under the CRC integrity guarantee, since the only
    /// producer is `build()` and the bytes on disk are CRC-checked. A future
    /// producer regressing them is caught in debug/test, not in release reads.
    pub(crate) fn validate(&self, record_count: usize) -> Result<(), Error> {
        // The last bucket must account for every permuted position.
        if self.fanout.total() as usize != self.sort_perm.len() {
            return Err(Error::CorruptIndex(format!(
                "trace_id index fanout total {} != sort_perm len {}",
                self.fanout.total(),
                self.sort_perm.len()
            )));
        }
        if self.sort_perm.len() > record_count {
            return Err(Error::CorruptIndex(format!(
                "trace_id index has {} positions, exceeds record_count {record_count}",
                self.sort_perm.len()
            )));
        }
        if self.sort_perm.iter().any(|&p| p as usize >= record_count) {
            return Err(Error::CorruptIndex(
                "trace_id index position out of range".into(),
            ));
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::SpanId;

    /// Build a `TraceIds` arena from typed ids.
    fn arena(ids: &[TraceId]) -> TraceIds {
        let mut t = TraceIds::with_capacity(ids.len());
        for &id in ids {
            t.push(id);
        }
        t
    }

    /// A trace id whose bytes are a simple pattern around `seed`.
    fn id(seed: u8) -> TraceId {
        let mut a = [0u8; 16];
        a[0] = seed;
        a[15] = seed.wrapping_mul(7).wrapping_add(1);
        TraceId::from(a)
    }

    /// Oracle: positions of `needle` by a plain linear scan of the arena.
    fn linear(trace_ids: &TraceIds, needle: TraceId) -> Vec<u32> {
        (0..trace_ids.len() as u32)
            .filter(|&i| trace_ids.get(i as usize) == needle)
            .collect()
    }

    #[test]
    fn empty_arena() {
        let t = arena(&[]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.indexed_rows(), 0);
        assert!(idx.positions(id(1), &t).is_empty());
    }

    #[test]
    fn single_trace_many_spans_stays_chronological() {
        let a = id(0xAB);
        let t = arena(&[a, a, a, a]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.positions(a, &t), &[0, 1, 2, 3]);
    }

    #[test]
    fn interleaved_traces() {
        // pos:  0  1  2  3  4  5
        // id:   A  B  A  A  C  B
        let (a, b, c) = (id(b'A'), id(b'B'), id(b'C'));
        let t = arena(&[a, b, a, a, c, b]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.positions(a, &t), &[0, 2, 3]);
        assert_eq!(idx.positions(b, &t), &[1, 5]);
        assert_eq!(idx.positions(c, &t), &[4]);
    }

    #[test]
    fn absent_trace_is_empty() {
        let t = arena(&[id(1), id(2)]);
        let idx = TraceIdIndex::build(&t);
        assert!(idx.positions(id(99), &t).is_empty());
    }

    #[test]
    fn unset_ids_are_skipped() {
        let a = id(5);
        let t = arena(&[TraceId::UNSET, a, TraceId::UNSET, a]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.indexed_rows(), 2, "the two zero rows are not indexed");
        assert_eq!(idx.positions(a, &t), &[1, 3]);
        assert!(
            idx.positions(TraceId::UNSET, &t).is_empty(),
            "unset id has no trace"
        );
    }

    #[test]
    fn first_byte_zero_is_valid() {
        // A non-unset id whose first byte is 0 must land in bucket 0, not be
        // skipped (only the all-zero id is the unset sentinel).
        let mut x = [0u8; 16];
        x[7] = 9;
        let x = TraceId::from(x);
        let t = arena(&[id(2), x, id(2)]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.positions(x, &t), &[1]);
    }

    #[test]
    fn id_hex_display_and_debug_format() {
        // Pin the W3C lowercase-hex text contract (consumers display/parse it).
        let mut t = [0u8; 16];
        t[0] = 0x0a;
        t[1] = 0xbc;
        let t = TraceId::from(t);
        assert_eq!(t.to_string(), "0abc0000000000000000000000000000");
        assert_eq!(
            format!("{t:?}"),
            "TraceId(0abc0000000000000000000000000000)"
        );
        assert_eq!(
            SpanId::from([0xff, 0, 0, 0, 0, 0, 0, 1]).to_string(),
            "ff00000000000001"
        );
    }

    #[test]
    fn from_bytes_is_strict_about_length() {
        // The typed needle makes a wrong-length lookup unrepresentable; the
        // strictness now lives at the parse boundary.
        assert!(TraceId::from_bytes(&[1, 2, 3]).is_none());
        assert!(TraceId::from_bytes(&[7u8; 16]).is_some());
        assert!(SpanId::from_bytes(&[7u8; 16]).is_none());
        assert!(SpanId::from_bytes(&[7u8; 8]).is_some());
    }

    #[test]
    fn validate_accepts_a_built_index() {
        let t = arena(&[id(1), id(2), id(1)]);
        let idx = TraceIdIndex::build(&t);
        assert!(idx.validate(t.len()).is_ok());
    }

    /// Round-trip a `Vec<u32>` through the chunk codec and decode it as a
    /// `Fanout` — the path a malformed on-disk fanout would take.
    fn decode_fanout(raw: &[u32]) -> Result<Fanout, bincode::error::DecodeError> {
        let bytes = bincode::serde::encode_to_vec(raw, bincode::config::standard()).unwrap();
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).map(|(f, _)| f)
    }

    #[test]
    fn fanout_build_and_bucket() {
        // first bytes: A(0x41)×2, B(0x42)×1 → cumulative buckets.
        let f = Fanout::build([0x41, 0x42, 0x41].into_iter());
        assert_eq!(f.total(), 3);
        assert_eq!(f.bucket(0x41), 0..2);
        assert_eq!(f.bucket(0x42), 2..3);
        assert_eq!(f.bucket(0x00), 0..0, "empty bucket below the first id");
        assert_eq!(f.bucket(0xFF), 3..3, "empty bucket above the last id");
    }

    #[test]
    fn fanout_serde_round_trips() {
        let f = Fanout::build([1u8, 1, 2, 255].into_iter());
        assert_eq!(decode_fanout_value(&f), f);
    }

    fn decode_fanout_value(f: &Fanout) -> Fanout {
        let bytes = bincode::serde::encode_to_vec(f, bincode::config::standard()).unwrap();
        bincode::serde::decode_from_slice(&bytes, bincode::config::standard())
            .map(|(f, _)| f)
            .unwrap()
    }

    #[test]
    fn fanout_deserialize_rejects_wrong_length() {
        // A length-256 invariant enforced at the parse boundary (the type is
        // `[u32; 256]`, so a short/long encoding cannot decode into it).
        assert!(decode_fanout(&[0u32; 10]).is_err());
        assert!(decode_fanout(&[0u32; 256]).is_ok());
    }

    #[test]
    fn fanout_deserialize_rejects_non_monotonic() {
        let mut raw = [0u32; 256];
        raw[5] = 10;
        raw[6] = 5; // a drop violates the cumulative (non-decreasing) invariant.
        assert!(
            decode_fanout(&raw).is_err(),
            "non-monotonic fanout must be rejected on decode"
        );
    }

    #[test]
    fn validate_rejects_fanout_tail_mismatch() {
        // total() == 0 (empty fanout) but a non-empty permutation.
        let idx = TraceIdIndex {
            fanout: Fanout::build(std::iter::empty()),
            sort_perm: vec![0],
        };
        assert!(
            idx.validate(1).is_err(),
            "fanout total != sort_perm.len() must be rejected"
        );
    }

    #[test]
    fn validate_rejects_out_of_range_position() {
        // total() == 1 matches sort_perm.len() == 1 (passes the tail check), but
        // position 5 is out of range for record_count 3.
        let idx = TraceIdIndex {
            fanout: Fanout::build(std::iter::once(0u8)),
            sort_perm: vec![5],
        };
        assert!(idx.validate(3).is_err(), "position 5 >= record_count 3");
    }

    /// Property-ish: a pseudo-random arena, every distinct id resolves to exactly
    /// the linear-scan positions, in chronological order.
    #[test]
    fn matches_linear_scan_over_many_ids() {
        // Deterministic pseudo-random ids (no Math.random / rand needed).
        let mut ids = Vec::new();
        let mut state: u64 = 0x9E37_79B9_7F4A_7C15;
        for _ in 0..2000 {
            state = state
                .wrapping_mul(6364136223846793005)
                .wrapping_add(1442695040888963407);
            // Only ~12 distinct first bytes so traces share buckets and repeat.
            let mut a = [0u8; 16];
            a[0] = (state >> 56) as u8 % 12 + 1;
            a[1] = (state >> 48) as u8 % 5;
            a[15] = (state >> 8) as u8;
            ids.push(TraceId::from(a));
        }
        let t = arena(&ids);
        let idx = TraceIdIndex::build(&t);
        assert!(idx.validate(t.len()).is_ok());

        let mut distinct = ids.clone();
        distinct.sort();
        distinct.dedup();
        for &needle in &distinct {
            assert_eq!(idx.positions(needle, &t), linear(&t, needle).as_slice());
        }
    }
}
