//! Deterministic synthetic OTLP log generation, for verifying the otel-logs
//! subsystem with known-exact corpora.
//!
//! Unlike the live sources (certstream/jetstream/github), this generates a
//! fixed, reproducible batch of `LogRecord`s from parameters — so a test can
//! push exactly N records with controlled timestamps, severities, and field
//! cardinalities, then assert the query results across forced rotation /
//! eviction boundaries. Generation is pure (no RNG, no clock): record `i` is a
//! deterministic function of `i` + params, so the same params always produce
//! the same corpus.

use opentelemetry_proto::tonic::logs::v1::LogRecord;

use crate::otel::{kv, str_val};

/// Severity cycle (text, OTel severity number) applied by record index — a
/// representative low-cardinality field for facet/histogram tests.
const SEVERITIES: &[(&str, i32)] = &[("ERROR", 17), ("WARN", 13), ("INFO", 9), ("DEBUG", 5)];

#[derive(Debug, Clone)]
pub struct SynthParams {
    /// Number of records to generate.
    pub count: usize,
    /// Timestamp of the first record (unix nanos); record `i` is
    /// `start_time_nanos + i * spacing_nanos`.
    pub start_time_nanos: u64,
    /// Nanoseconds between consecutive records (monotonic, ascending).
    pub spacing_nanos: u64,
    /// Distinct values per mid-cardinality attribute (`host`, `code`): a knob
    /// to push fields across the low/mid/high tier boundaries.
    pub field_cardinality: usize,
    /// Offset added to the record index before the `% field_cardinality` that
    /// picks `host`/`code`. Seeds differing by less than `field_cardinality`
    /// give distinct corpora; seeds differing by a multiple of it collide. Use
    /// seeds in `[0, field_cardinality)` for guaranteed-distinct corpora.
    pub seed: u64,
}

/// Build the deterministic batch. Each record carries: a monotonic timestamp;
/// a cycled severity (low-card `level`); `host`/`code` over `field_cardinality`
/// distinct values (mid-card); and a unique `seq` (high-card). The body is a
/// stable per-index message.
pub fn generate(p: &SynthParams) -> Vec<LogRecord> {
    let card = p.field_cardinality.max(1) as u64;
    (0..p.count)
        .map(|i| {
            let n = i as u64 + p.seed;
            let (sev_text, sev_num) = SEVERITIES[i % SEVERITIES.len()];
            let ts = p
                .start_time_nanos
                .saturating_add((i as u64).saturating_mul(p.spacing_nanos));
            LogRecord {
                time_unix_nano: ts,
                observed_time_unix_nano: ts,
                severity_number: sev_num,
                severity_text: sev_text.to_string(),
                body: str_val(&format!("synthetic log message {i}")),
                attributes: vec![
                    kv("level", str_val(&sev_text.to_lowercase())),
                    kv("host", str_val(&format!("host-{}", n % card))),
                    kv("code", str_val(&format!("c{:03}", n % card))),
                    kv("seq", str_val(&i.to_string())),
                ],
                event_name: "synthetic".to_string(),
                ..Default::default()
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn params(count: usize, card: usize) -> SynthParams {
        SynthParams {
            count,
            start_time_nanos: 1_000_000_000_000,
            spacing_nanos: 1_000_000_000,
            field_cardinality: card,
            seed: 0,
        }
    }

    #[test]
    fn count_and_monotonic_ascending_timestamps() {
        let recs = generate(&params(5, 10));
        assert_eq!(recs.len(), 5);
        for w in recs.windows(2) {
            assert!(w[1].time_unix_nano > w[0].time_unix_nano, "timestamps ascend");
        }
        assert_eq!(recs[0].time_unix_nano, 1_000_000_000_000);
        assert_eq!(recs[1].time_unix_nano, 1_000_000_000_000 + 1_000_000_000);
    }

    #[test]
    fn deterministic_for_same_params() {
        let a = generate(&params(8, 3));
        let b = generate(&params(8, 3));
        assert_eq!(a, b);
    }

    #[test]
    fn field_cardinality_bounds_distinct_host_values() {
        // 50 records, cardinality 4 → at most 4 distinct host values.
        let recs = generate(&params(50, 4));
        let hosts: std::collections::BTreeSet<_> = recs
            .iter()
            .flat_map(|r| &r.attributes)
            .filter(|a| a.key == "host")
            .map(|a| format!("{:?}", a.value))
            .collect();
        assert_eq!(hosts.len(), 4);
    }

    #[test]
    fn seq_is_unique_per_record() {
        let recs = generate(&params(20, 4));
        let seqs: std::collections::BTreeSet<_> = recs
            .iter()
            .flat_map(|r| &r.attributes)
            .filter(|a| a.key == "seq")
            .map(|a| format!("{:?}", a.value))
            .collect();
        assert_eq!(seqs.len(), 20, "seq is high-cardinality (unique per record)");
    }
}
