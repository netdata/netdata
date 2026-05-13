// SPDX-License-Identifier: GPL-3.0-or-later
//
// Lookback rule: given a stream of samples in time order, pick the most
// recent one at or before `target_ms` whose timestamp is within
// `lookback_ms` of the target. Returns `None` if no qualifying sample
// exists; the caller drops the series (Prometheus staleness rule).

use super::types::Sample;

/// Find the latest sample at or before `target_ms` whose timestamp is
/// no older than `target_ms - lookback_ms`. Returns the picked sample
/// or `None` if none qualifies.
///
/// The input is expected to be in non-decreasing timestamp order (the
/// shim's storage iterator guarantees this).
pub fn pick_latest_within_window(
    samples: &[Sample],
    target_ms: i64,
    lookback_ms: i64,
) -> Option<Sample> {
    let earliest = target_ms.saturating_sub(lookback_ms);
    samples
        .iter()
        .rev()
        .find(|s| s.timestamp_ms <= target_ms && s.timestamp_ms >= earliest)
        .copied()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn s(ts: i64, v: f64) -> Sample {
        Sample {
            timestamp_ms: ts,
            value: v,
        }
    }

    #[test]
    fn picks_latest_within_window() {
        let samples = vec![s(100, 1.0), s(200, 2.0), s(300, 3.0), s(400, 4.0)];
        let r = pick_latest_within_window(&samples, 350, 200).unwrap();
        assert_eq!(r.timestamp_ms, 300);
    }

    #[test]
    fn drops_when_window_empty() {
        let samples = vec![s(100, 1.0)];
        assert!(pick_latest_within_window(&samples, 1000, 200).is_none());
    }

    #[test]
    fn returns_none_for_empty_input() {
        assert!(pick_latest_within_window(&[], 100, 200).is_none());
    }

    #[test]
    fn includes_sample_at_target() {
        let samples = vec![s(100, 1.0), s(200, 2.0)];
        let r = pick_latest_within_window(&samples, 200, 100).unwrap();
        assert_eq!(r.timestamp_ms, 200);
    }
}
