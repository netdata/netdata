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
//! - `sort_perm[N]` — row positions sorted by their 16-byte `trace_id` value
//!   (a **stable** sort, so within one trace the spans stay chronological).
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

use serde::{Deserialize, Serialize};

use crate::{Error, TraceIds};

/// Whether every byte of `id` is zero — the OTLP/W3C "unset/invalid" trace id,
/// which belongs to no trace and is excluded from the index.
fn is_unset(id: &[u8]) -> bool {
    id.iter().all(|&b| b == 0)
}

/// The `trace_id` index: a first-byte fanout over row positions sorted by their
/// 16-byte `trace_id`. Built at seal from the `TRCE` column, read on lookup.
///
/// Self-contained for the bucket step (`fanout`); the comparison step reads the
/// `TRCE` column the index was built from (positions are indices into it).
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TraceIdIndex {
    /// 256 cumulative first-byte counts. `fanout[b]` = number of indexed
    /// positions whose `trace_id[0] <= b`; `fanout[255]` == `sort_perm.len()`.
    /// Stored as a `Vec` (serde has no `[u32; 256]` impl) with the length
    /// invariant enforced on decode by [`TraceIdIndex::validate`].
    fanout: Vec<u32>,
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
        let mut sort_perm: Vec<u32> = (0..trace_ids.len() as u32)
            .filter(|&i| !is_unset(trace_ids.get(i as usize)))
            .collect();
        // Stable sort: equal ids keep chronological (ascending-position) order,
        // so a trace's spans come back in time order.
        sort_perm.sort_by(|&a, &b| trace_ids.get(a as usize).cmp(trace_ids.get(b as usize)));

        let mut fanout = vec![0u32; 256];
        for &p in &sort_perm {
            fanout[trace_ids.get(p as usize)[0] as usize] += 1;
        }
        let mut acc = 0u32;
        for slot in &mut fanout {
            acc += *slot;
            *slot = acc;
        }

        Self { fanout, sort_perm }
    }

    /// Positions (chronological) of every span whose `trace_id == needle`, or an
    /// empty slice if the trace is absent. `trace_ids` MUST be the `TRCE` column
    /// this index was built from.
    ///
    /// `fanout` narrows to the first-byte bucket, then two indirect binary
    /// searches bound the equal run: `fanout + 2·log2(N/256)` comparisons.
    pub fn positions<'s>(&'s self, needle: &[u8], trace_ids: &TraceIds) -> &'s [u32] {
        if needle.len() != TraceIds::WIDTH || is_unset(needle) {
            return &[];
        }
        let b = needle[0] as usize;
        let lo = if b == 0 { 0 } else { self.fanout[b - 1] as usize };
        let hi = self.fanout[b] as usize;
        let bucket = &self.sort_perm[lo..hi];
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
    pub(crate) fn validate(&self, record_count: usize) -> Result<(), Error> {
        if self.fanout.len() != 256 {
            return Err(Error::CorruptIndex(format!(
                "trace_id index fanout has {} entries, expected 256",
                self.fanout.len()
            )));
        }
        // Monotonic non-decreasing.
        if self.fanout.windows(2).any(|w| w[0] > w[1]) {
            return Err(Error::CorruptIndex(
                "trace_id index fanout is not monotonic".into(),
            ));
        }
        // The last bucket must account for every permuted position.
        if self.fanout[255] as usize != self.sort_perm.len() {
            return Err(Error::CorruptIndex(format!(
                "trace_id index fanout[255]={} != sort_perm len {}",
                self.fanout[255],
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

    /// Build a `TraceIds` arena from 16-byte ids.
    fn arena(ids: &[[u8; 16]]) -> TraceIds {
        let mut t = TraceIds::with_capacity(ids.len());
        for id in ids {
            t.push(id);
        }
        t
    }

    /// A 16-byte id whose bytes are a simple pattern around `seed`.
    fn id(seed: u8) -> [u8; 16] {
        let mut a = [0u8; 16];
        a[0] = seed;
        a[15] = seed.wrapping_mul(7).wrapping_add(1);
        a
    }

    /// Oracle: positions of `needle` by a plain linear scan of the arena.
    fn linear(trace_ids: &TraceIds, needle: &[u8; 16]) -> Vec<u32> {
        (0..trace_ids.len() as u32)
            .filter(|&i| trace_ids.get(i as usize) == &needle[..])
            .collect()
    }

    #[test]
    fn empty_arena() {
        let t = arena(&[]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.indexed_rows(), 0);
        assert!(idx.positions(&id(1), &t).is_empty());
    }

    #[test]
    fn single_trace_many_spans_stays_chronological() {
        let a = id(0xAB);
        let t = arena(&[a, a, a, a]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.positions(&a, &t), &[0, 1, 2, 3]);
    }

    #[test]
    fn interleaved_traces() {
        // pos:  0  1  2  3  4  5
        // id:   A  B  A  A  C  B
        let (a, b, c) = (id(b'A'), id(b'B'), id(b'C'));
        let t = arena(&[a, b, a, a, c, b]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.positions(&a, &t), &[0, 2, 3]);
        assert_eq!(idx.positions(&b, &t), &[1, 5]);
        assert_eq!(idx.positions(&c, &t), &[4]);
    }

    #[test]
    fn absent_trace_is_empty() {
        let t = arena(&[id(1), id(2)]);
        let idx = TraceIdIndex::build(&t);
        assert!(idx.positions(&id(99), &t).is_empty());
    }

    #[test]
    fn unset_ids_are_skipped() {
        let zero = [0u8; 16];
        let a = id(5);
        let t = arena(&[zero, a, zero, a]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.indexed_rows(), 2, "the two zero rows are not indexed");
        assert_eq!(idx.positions(&a, &t), &[1, 3]);
        assert!(idx.positions(&zero, &t).is_empty(), "unset id has no trace");
    }

    #[test]
    fn first_byte_zero_is_valid() {
        // A non-unset id whose first byte is 0 must land in bucket 0, not be
        // skipped (only the all-zero id is the unset sentinel).
        let mut x = [0u8; 16];
        x[7] = 9;
        let t = arena(&[id(2), x, id(2)]);
        let idx = TraceIdIndex::build(&t);
        assert_eq!(idx.positions(&x, &t), &[1]);
    }

    #[test]
    fn malformed_needle_length_is_empty() {
        let t = arena(&[id(1)]);
        let idx = TraceIdIndex::build(&t);
        assert!(idx.positions(&[1, 2, 3], &t).is_empty());
    }

    #[test]
    fn validate_accepts_a_built_index() {
        let t = arena(&[id(1), id(2), id(1)]);
        let idx = TraceIdIndex::build(&t);
        assert!(idx.validate(t.len()).is_ok());
    }

    #[test]
    fn validate_rejects_short_fanout() {
        let idx = TraceIdIndex { fanout: vec![0u32; 10], sort_perm: vec![] };
        assert!(idx.validate(0).is_err());
    }

    #[test]
    fn validate_rejects_out_of_range_position() {
        let mut fanout = vec![0u32; 256];
        fanout.fill(1);
        let idx = TraceIdIndex { fanout, sort_perm: vec![5] };
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
            state = state.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407);
            // Only ~12 distinct first bytes so traces share buckets and repeat.
            let mut a = [0u8; 16];
            a[0] = (state >> 56) as u8 % 12 + 1;
            a[1] = (state >> 48) as u8 % 5;
            a[15] = (state >> 8) as u8;
            ids.push(a);
        }
        let t = arena(&ids);
        let idx = TraceIdIndex::build(&t);
        assert!(idx.validate(t.len()).is_ok());

        let mut distinct: Vec<[u8; 16]> = ids.clone();
        distinct.sort();
        distinct.dedup();
        for needle in &distinct {
            assert_eq!(idx.positions(needle, &t), linear(&t, needle).as_slice());
        }
    }
}
