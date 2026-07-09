//! The trace-id bloom filter (`TBLM` chunk): a per-file membership pre-check
//! that lets a reader skip a whole file without touching `TIDX`/`TRCE`.
//!
//! `TIDX` answers "where are this trace's spans" in O(log) *within* a file;
//! what it cannot do is make "not in this file" cheap across MANY files — the
//! cross-file trace-by-id (the query engine's fan-out) would otherwise open
//! every candidate file's index. The bloom is the industry-standard answer
//! (Tempo's only auxiliary index is exactly this): one small chunk read gives
//! "definitely absent" for ~95% of non-member files at the configured 5%
//! false-positive rate. False positives cost only a wasted `TIDX` lookup;
//! false negatives cannot happen.
//!
//! The filter itself is [`fastbloom::BloomFilter`] — an audited, widely-used
//! crate (no `unsafe` in its source as of the pinned version; see the
//! workspace `Cargo.toml` pin note) — serialized verbatim inside the chunk via
//! serde, so the on-disk payload is self-describing (bit length, hash count,
//! and seeded hasher state all travel with it). Build-time policy lives here:
//! 5% target FP over the file's DISTINCT non-unset trace ids, one filter per
//! file, and a constant seed so identical inputs seal to identical bytes.
//!
//! Same additive TOC-indexed contract as `TIDX`/`EVNB`/`LNKB`: optional,
//! detected via the TOC, no format version bump; absent when the file has no
//! set trace ids.
//!
//! Build-time sizing is ~6.25 bits per distinct id (≈0.8 MB per million
//! distinct traces): bounded in practice by the producer's file-rotation
//! limits, not here — bounding distinct-id cardinality is the seal's
//! responsibility (see the production-cutover ingest caps).

use fastbloom::BloomFilter;
use serde::{Deserialize, Serialize};

use crate::{Error, TraceId, TraceIdIndex, TraceIds};

/// Build-time false-positive target over the distinct ids (≈6.25 bits/id).
const FALSE_POSITIVE_RATE: f64 = 0.05;

/// Constant hashing seed: identical inputs seal to byte-identical chunks
/// (reproducible files), and the seed state serializes with the filter so
/// readers never depend on this constant.
const SEED: u128 = 0x6e64_5442_4c4d_2031; // arbitrary, stable ("ndTBLM 1")

/// Body of the `TBLM` chunk. `distinct_ids` is carried for validation and
/// diagnostics (the filter itself does not expose its build-time item count).
///
/// Equality note: the derived `PartialEq` delegates to fastbloom's, which
/// compares bits + hash count but NOT the hasher seed — `==` is structural,
/// not behavioral, equality (tests pair it with membership re-checks).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct TraceIdBloom {
    distinct_ids: u32,
    filter: BloomFilter,
}

impl TraceIdBloom {
    /// Build the per-file filter from the file's sorted trace-id index —
    /// adjacent-dedup over the permutation yields each distinct id exactly
    /// once, collected up front because the filter must be sized before
    /// insertion. The transient `Vec` costs 16 B per distinct id, bounded by
    /// the `TRCE` column the seal already holds in memory twice
    /// (insertion-order + chronological) — never the peak. Returns `None`
    /// when the file has no set (non-zero) trace ids: no chunk is written,
    /// and a reader treats absence as "cannot skip".
    ///
    /// `trace_ids` MUST be the chronological `TRCE` column `index` was built
    /// from (the same coupling `TraceIdIndex::positions` documents).
    pub(crate) fn build(index: &TraceIdIndex, trace_ids: &TraceIds) -> Option<Self> {
        // One pass: the filter needs its item count BEFORE insertion (sizing),
        // so collect the distinct ids rather than walking the permutation twice.
        let distinct: Vec<TraceId> = index.distinct_ids(trace_ids).collect();
        if distinct.is_empty() {
            return None;
        }
        let mut filter = BloomFilter::with_false_pos(FALSE_POSITIVE_RATE)
            .seed(&SEED)
            .expected_items(distinct.len());
        for id in &distinct {
            filter.insert(id.as_bytes());
        }
        Some(Self {
            distinct_ids: distinct.len() as u32,
            filter,
        })
    }

    /// Whether the file MIGHT contain `id`. `false` is definitive (no false
    /// negatives); `true` is probabilistic (~5% of absent ids). The unset
    /// (all-zero) id forms no trace and is never contained.
    pub fn might_contain(&self, id: TraceId) -> bool {
        !id.is_unset() && self.filter.contains(id.as_bytes())
    }

    /// Distinct set trace ids at build time.
    pub fn distinct_ids(&self) -> u32 {
        self.distinct_ids
    }

    /// Test-only constructor for deliberately malformed filters (validate /
    /// degradation tests) — production filters only come from [`build`](Self::build).
    #[cfg(test)]
    pub(crate) fn raw_for_tests(distinct_ids: u32, filter: BloomFilter) -> Self {
        Self {
            distinct_ids,
            filter,
        }
    }

    /// Panic-safety validation at the trust boundary (the `TIDX` contract):
    /// a decoded chunk claiming an impossible shape surfaces as
    /// [`Error::CorruptIndex`] so the query layer skips the file.
    ///
    /// This deliberately reaches INTO the decoded filter, not just our
    /// envelope: serde bypasses the crate's constructor asserts, so a
    /// CRC-valid crafted chunk could deserialize a filter whose bit vector is
    /// empty (`contains` would index it — panic) or whose hash count is
    /// absurd (`contains` would run up to ~4B probes per lookup — a stall).
    /// Neither state is constructible through the crate's API; both are
    /// constructible through its serde layout.
    pub(crate) fn validate(&self, record_count: usize) -> Result<(), Error> {
        if self.distinct_ids == 0 {
            return Err(Error::CorruptIndex(
                "trace_id bloom with zero distinct ids (an empty filter is never written)".into(),
            ));
        }
        if self.distinct_ids as usize > record_count {
            return Err(Error::CorruptIndex(format!(
                "trace_id bloom claims {} distinct ids, exceeds record_count {record_count}",
                self.distinct_ids
            )));
        }
        if self.filter.num_bits() == 0 {
            return Err(Error::CorruptIndex(
                "trace_id bloom filter has an empty bit vector".into(),
            ));
        }
        // The optimal hash count at our 5% target is 4 for normal filters —
        // but fastbloom floors the filter at 64 bits, which inflates the
        // optimal k for tiny files (a legitimate 1-distinct-id filter at 5%
        // computes k = 44). The cap must therefore stay >= 44; do NOT tighten
        // it toward the "normal" optimum or freshly-rolled low-volume files
        // become unreadable.
        //
        // fastbloom's `num_hashes()` computes `num_hashes_minus_one + 1`
        // UNCHECKED on a serde-visible private field: a crafted chunk carrying
        // u32::MAX wraps the accessor to 0 where overflow checks are off (our
        // release profiles) and panics where they are on (dev/test). Catch the
        // dev panic and fold it into the same rejection; 0 is unreachable for
        // a genuinely built filter (minus_one + 1 >= 1), so it doubles as the
        // wrap marker in release. (Not directly constructible in a test: the
        // builder clamps, and bincode's varint layout makes byte-patching the
        // field infeasible — the guard is defense-in-depth at the boundary.)
        const MAX_HASHES: u32 = 64;
        let num_hashes =
            std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| self.filter.num_hashes()))
                .unwrap_or(0);
        if num_hashes == 0 || num_hashes > MAX_HASHES {
            return Err(Error::CorruptIndex(format!(
                "trace_id bloom filter claims {num_hashes} hashes per id \
                 (valid range 1..={MAX_HASHES}; 0 marks an overflowed count)"
            )));
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn id(seed: u8, tail: u8) -> TraceId {
        let mut a = [0u8; 16];
        a[0] = seed;
        a[15] = tail;
        TraceId::from(a)
    }

    /// A column + index over `ids` (in the given chronological order).
    fn indexed(ids: &[TraceId]) -> (TraceIds, TraceIdIndex) {
        let mut t = TraceIds::with_capacity(ids.len());
        for &i in ids {
            t.push(i);
        }
        let idx = TraceIdIndex::build(&t);
        (t, idx)
    }

    #[test]
    fn no_false_negatives_exhaustive() {
        // Every id written to the file MUST be reported as possibly present —
        // including duplicates (one trace, many spans) and ids sharing fanout
        // buckets.
        let ids: Vec<TraceId> = (0..500u16)
            .map(|i| id((i % 13) as u8 + 1, (i / 13) as u8))
            .collect();
        let mut with_dups = ids.clone();
        with_dups.extend_from_slice(&ids[..100]); // resends/multi-span traces
        let (col, idx) = indexed(&with_dups);
        let bloom = TraceIdBloom::build(&idx, &col).expect("set ids present");
        assert_eq!(bloom.distinct_ids(), 500);
        for &i in &ids {
            assert!(bloom.might_contain(i), "false negative for {i}");
        }
        bloom.validate(with_dups.len()).unwrap();
    }

    #[test]
    fn false_positive_rate_near_target() {
        // 10k distinct member ids; probe 100k absent ids. Expect ~5%,
        // asserted loosely (< 2x target) to stay robust across crate versions.
        let members: Vec<TraceId> = (0..10_000u32)
            .map(|i| {
                let mut a = [0u8; 16];
                a[..4].copy_from_slice(&i.to_be_bytes());
                a[15] = 1; // never unset
                TraceId::from(a)
            })
            .collect();
        let (col, idx) = indexed(&members);
        let bloom = TraceIdBloom::build(&idx, &col).unwrap();

        let mut hits = 0u32;
        const PROBES: u32 = 100_000;
        for i in 0..PROBES {
            let mut a = [0u8; 16];
            a[..4].copy_from_slice(&i.to_be_bytes());
            a[8] = 0xAB; // disjoint from the member universe
            if bloom.might_contain(TraceId::from(a)) {
                hits += 1;
            }
        }
        let rate = hits as f64 / PROBES as f64;
        assert!(
            rate < 0.10,
            "measured FP rate {rate} exceeds 2x the 5% target"
        );
        assert!(rate > 0.005, "suspiciously low FP rate {rate} — wrong n?");
    }

    #[test]
    fn unset_ids_never_build_or_match() {
        // A file of only unset ids builds no filter (no chunk written)...
        let (col, idx) = indexed(&[TraceId::UNSET, TraceId::UNSET]);
        assert!(TraceIdBloom::build(&idx, &col).is_none());

        // ...and the unset id never matches an existing filter.
        let (col, idx) = indexed(&[id(1, 1)]);
        let bloom = TraceIdBloom::build(&idx, &col).unwrap();
        assert!(!bloom.might_contain(TraceId::UNSET));
    }

    #[test]
    fn bincode_round_trip_through_the_chunk_codec() {
        // The crate tests its serde support with serde_cbor; OUR chunk codec
        // is bincode+zstd (`writer::pack` / `reader::unpack`). Round-trip
        // through the real codec — seed/hasher state must survive, proven by
        // identical membership answers.
        let ids: Vec<TraceId> = (0..1_000u16)
            .map(|i| id((i % 7) as u8 + 1, i as u8))
            .collect();
        let (col, idx) = indexed(&ids);
        let bloom = TraceIdBloom::build(&idx, &col).unwrap();

        let packed = crate::writer::pack(&bloom, crate::ZSTD_LEVEL_DEFAULT).unwrap();
        let back: TraceIdBloom = crate::reader::unpack(&packed).unwrap();
        assert_eq!(back, bloom);
        for &i in &ids {
            assert!(back.might_contain(i));
        }
        // Deterministic build: same input → same packed bytes (constant seed).
        let again = TraceIdBloom::build(&idx, &col).unwrap();
        assert_eq!(
            crate::writer::pack(&again, crate::ZSTD_LEVEL_DEFAULT).unwrap(),
            packed
        );
    }

    #[test]
    fn validate_rejects_corrupt_envelopes() {
        let (col, idx) = indexed(&[id(1, 1), id(2, 2)]);
        let mut bloom = TraceIdBloom::build(&idx, &col).unwrap();
        assert!(bloom.validate(2).is_ok());
        // Claims more distinct ids than the file has rows.
        assert!(bloom.validate(1).is_err());
        // Zero-distinct envelope (never written).
        bloom.distinct_ids = 0;
        assert!(bloom.validate(2).is_err());
    }

    #[test]
    fn validate_rejects_hostile_filter_states() {
        // An absurd hash count is legal through the builder (`.hashes(k)`),
        // and reachable through serde regardless — a probe against it would
        // stall for up to ~4B iterations. The validate cap rejects it.
        let filter = BloomFilter::from_vec(vec![0u64; 4])
            .seed(&SEED)
            .hashes(1_000);
        let bloom = TraceIdBloom {
            distinct_ids: 1,
            filter,
        };
        assert!(matches!(
            bloom.validate(10),
            Err(Error::CorruptIndex(msg)) if msg.contains("hashes per id")
        ));

        // Truncated/garbage payloads must error out of the codec, never panic
        // (the empty-bit-vector guard in validate() backs the same boundary
        // for a payload crafted to decode "successfully").
        let (col, idx) = indexed(&[id(3, 3)]);
        let good = TraceIdBloom::build(&idx, &col).unwrap();
        let packed = crate::writer::pack(&good, crate::ZSTD_LEVEL_DEFAULT).unwrap();
        for cut in [1usize, packed.len() / 2, packed.len() - 1] {
            assert!(
                crate::reader::unpack::<TraceIdBloom>(&packed[..cut]).is_err(),
                "truncated payload at {cut} must fail decode"
            );
        }
    }

    /// Read-side contract: a file carrying TBLM without the TIDX it derives
    /// from is invalid — the writer refuses to produce one, so this crafts the
    /// container by hand and asserts the reader rejects it (instead of letting
    /// a definite bloom miss silently answer "trace absent").
    #[test]
    fn reader_rejects_bloom_without_index() {
        use crate::{ColumnsTable, Histogram, IdRanges, Metadata, SchemaTree, Summary};

        let (col, idx) = indexed(&[id(1, 1)]);
        let bloom = TraceIdBloom::build(&idx, &col).unwrap();

        let lvl = crate::ZSTD_LEVEL_DEFAULT;
        let summary = Summary {
            min_timestamp_s: 1,
            max_timestamp_s: 1,
            record_count: 1,
            content_meta: Vec::new(),
        };
        let metadata = Metadata {
            histogram: Histogram {
                timestamps: vec![1],
                counts: vec![1],
            },
            id_ranges: IdRanges {
                low_end: crate::KvId(0),
                mid_end: crate::KvId(0),
                high_end: crate::KvId(0),
            },
            tree: SchemaTree::flat(&Vec::new().into()),
            columns: ColumnsTable::default(),
        };
        let prim =
            crate::PrefixMap::<crate::BitmapValue>::build(Vec::<(&str, crate::BitmapValue)>::new())
                .unwrap();

        // Bypass ChunkWriter (which enforces TBLM ⟹ TIDX) and write the raw
        // container: SUMR, META, TIMS, PRIM, TBLM, SB00 — no TIDX.
        let mut w = chunk_file::container::StreamingWriter::new(
            std::io::Cursor::new(Vec::new()),
            *crate::MAGIC,
            crate::VERSION,
            6,
        )
        .unwrap();
        w.write_chunk(
            crate::CHUNK_SUMMARY,
            &crate::writer::pack(&summary, lvl).unwrap(),
        )
        .unwrap();
        w.write_chunk(
            crate::CHUNK_META,
            &crate::writer::pack(&metadata, lvl).unwrap(),
        )
        .unwrap();
        w.write_chunk(
            crate::CHUNK_TIMS,
            &crate::writer::pack(&[1i64][..], lvl).unwrap(),
        )
        .unwrap();
        w.write_chunk(
            crate::CHUNK_PRIMARY,
            &crate::writer::pack(&prim, lvl).unwrap(),
        )
        .unwrap();
        w.write_chunk(
            crate::CHUNK_TRACE_BLOOM,
            &crate::writer::pack(&bloom, lvl).unwrap(),
        )
        .unwrap();
        w.write_chunk(
            crate::stream_batch_id(0),
            &crate::writer::pack(&crate::StreamBatch::for_write(&[]), lvl).unwrap(),
        )
        .unwrap();
        let buf = w.finish().unwrap().into_inner();

        let reader = crate::reader::ChunkReader::open(&buf).unwrap();
        assert!(reader.has_trace_id_bloom());
        assert!(!reader.has_trace_id_index());
        assert!(matches!(
            reader.trace_id_bloom(),
            Err(Error::CorruptIndex(msg)) if msg.contains("without the trace_id index")
        ));
    }

    /// The full dependency chain: TBLM + TIDX chunks present but no declared
    /// TRCE column — the reader must reject the bloom (require_column), not
    /// let a miss silently answer "trace absent".
    #[test]
    fn reader_rejects_bloom_without_trace_column() {
        use crate::{ColumnsTable, Histogram, IdRanges, Metadata, SchemaTree, Summary};

        let (col, idx) = indexed(&[id(1, 1)]);
        let bloom = TraceIdBloom::build(&idx, &col).unwrap();

        let lvl = crate::ZSTD_LEVEL_DEFAULT;
        let summary = Summary {
            min_timestamp_s: 1,
            max_timestamp_s: 1,
            record_count: 1,
            content_meta: Vec::new(),
        };
        let metadata = Metadata {
            histogram: Histogram {
                timestamps: vec![1],
                counts: vec![1],
            },
            id_ranges: IdRanges {
                low_end: crate::KvId(0),
                mid_end: crate::KvId(0),
                high_end: crate::KvId(0),
            },
            tree: SchemaTree::flat(&Vec::new().into()),
            columns: ColumnsTable::default(), // no TRCE in the manifest
        };
        let prim = crate::PrefixMap::<crate::BitmapValue>::build(
            Vec::<(&str, crate::BitmapValue)>::new(),
        )
        .unwrap();

        let mut w = chunk_file::container::StreamingWriter::new(
            std::io::Cursor::new(Vec::new()),
            *crate::MAGIC,
            crate::VERSION,
            7,
        )
        .unwrap();
        w.write_chunk(crate::CHUNK_SUMMARY, &crate::writer::pack(&summary, lvl).unwrap())
            .unwrap();
        w.write_chunk(crate::CHUNK_META, &crate::writer::pack(&metadata, lvl).unwrap())
            .unwrap();
        w.write_chunk(crate::CHUNK_TIMS, &crate::writer::pack(&[1i64][..], lvl).unwrap())
            .unwrap();
        w.write_chunk(crate::CHUNK_PRIMARY, &crate::writer::pack(&prim, lvl).unwrap())
            .unwrap();
        w.write_chunk(crate::CHUNK_TRACE_INDEX, &crate::writer::pack(&idx, lvl).unwrap())
            .unwrap();
        w.write_chunk(
            crate::CHUNK_TRACE_BLOOM,
            &crate::writer::pack(&bloom, lvl).unwrap(),
        )
        .unwrap();
        w.write_chunk(
            crate::stream_batch_id(0),
            &crate::writer::pack(&crate::StreamBatch::for_write(&[]), lvl).unwrap(),
        )
        .unwrap();
        let buf = w.finish().unwrap().into_inner();

        let reader = crate::reader::ChunkReader::open(&buf).unwrap();
        assert!(reader.has_trace_id_bloom() && reader.has_trace_id_index());
        assert!(reader.trace_id_bloom().is_err(), "missing TRCE column rejected");
    }
}
