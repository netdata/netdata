//! Newest-first source order. An operation that may stop early visits
//! the newest data first, so everything it skipped is older than what
//! it saw, and it can say from when on its answer is complete.

use std::cmp::Reverse;

use super::sources::TraceSource;

/// The newest second a source may hold; `None` for a tail, which has no
/// summary and holds the active WAL's newest data.
fn newest_s(source: &TraceSource) -> Option<u32> {
    match source {
        TraceSource::Tail(_) => None,
        TraceSource::Sfst(c) => Some(c.summary.max_timestamp_s),
        TraceSource::Unavailable(u) => Some(u.summary.max_timestamp_s),
    }
}

/// Sort `sources` newest first: tails, then by summary max timestamp,
/// descending. Ties go by source id, so the caller's order never
/// changes the result.
pub(crate) fn newest_first(sources: &mut [TraceSource]) {
    let key = |s: &TraceSource| match newest_s(s) {
        None => (0u8, Reverse(0u32)),
        Some(t) => (1, Reverse(t)),
    };
    sources.sort_by(|a, b| {
        key(a)
            .cmp(&key(b))
            .then_with(|| a.source_id().as_str().cmp(b.source_id().as_str()))
    });
}

/// After a newest-first walk stopped before `unvisited`: the first second
/// from which every span lives in a visited source — one past the newest
/// unvisited summary (summaries are inclusive seconds). An unvisited
/// tail could hold any recent span, so it claims nothing (`u32::MAX`).
/// `None` when nothing was left unvisited.
pub(crate) fn complete_after(unvisited: &[TraceSource]) -> Option<u32> {
    unvisited
        .iter()
        .map(|s| newest_s(s).map_or(u32::MAX, |t| t.saturating_add(1)))
        .max()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::source::Source;
    use crate::traces::sources::{
        SourceId, TraceSfstCandidate, TraceUnavailable, TraceWalTail, WalCoverage,
    };

    fn summary(max_s: u32) -> sfst::Summary {
        sfst::Summary {
            min_timestamp_s: max_s.saturating_sub(10),
            max_timestamp_s: max_s,
            record_count: 1,
            content_meta: Vec::new(),
        }
    }

    fn sealed(id: &str, max_s: u32) -> TraceSource {
        TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(id),
            summary: summary(max_s),
            source: Source::File(id.into()),
            coverage: None,
        })
    }

    fn remote(id: &str, max_s: u32) -> TraceSource {
        TraceSource::Unavailable(TraceUnavailable {
            source_id: SourceId::new(id),
            summary: summary(max_s),
        })
    }

    fn tail(id: &str) -> TraceSource {
        TraceSource::Tail(TraceWalTail {
            source_id: SourceId::new(id),
            path: id.into(),
            coverage: WalCoverage {
                wal_id: id.into(),
                range: wal::FrameRange::new(0, 1),
            },
        })
    }

    fn ids(sources: &[TraceSource]) -> Vec<&str> {
        sources.iter().map(|s| s.source_id().as_str()).collect()
    }

    #[test]
    fn tails_first_then_newest_summaries_with_id_ties() {
        let mut sources = vec![
            sealed("b", 100),
            remote("r", 300),
            tail("t"),
            sealed("a", 100),
            sealed("c", 200),
        ];
        newest_first(&mut sources);
        assert_eq!(ids(&sources), ["t", "r", "c", "a", "b"]);
        // The caller's order never matters.
        sources.reverse();
        newest_first(&mut sources);
        assert_eq!(ids(&sources), ["t", "r", "c", "a", "b"]);
    }

    #[test]
    fn completeness_starts_past_the_newest_unvisited_second() {
        assert_eq!(complete_after(&[]), None);
        assert_eq!(
            complete_after(&[sealed("a", 100), sealed("b", 50)]),
            Some(101)
        );
        assert_eq!(complete_after(&[tail("t")]), Some(u32::MAX));
    }
}
