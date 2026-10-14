//! The trace-level pre-assembly gate: the rollup-backed superset
//! filter that spares [`search`](super::search::search) an assembly
//! per provably non-matching candidate.
//!
//! Trace-level predicates (`trace:root_service_name` / `trace:root_name`
//! selections, `trace:duration` lower bounds) are decided
//! post-assembly; without a gate every candidate in the window is
//! assembled just to be discarded, and rare predicates burn the
//! assembled ceiling on discards. The per-file `TRSU` rollup already
//! stores each trace's envelope and recorded root, so the gate sits at
//! the examine loop's `pool.pop()` and asks: could this candidate
//! possibly match?
//!
//! # The superset contract
//!
//! The gate only ever SKIPS an assembly when the rollup evidence
//! PROVES a non-match; failing any precondition means UNPRUNABLE —
//! the candidate assembles and the post-assembly evaluator stays the
//! truth. The known exception is the recorded-vs-canonical root
//! divergence (see [`search`](super::search)'s module docs), accepted
//! by explicit ruling as recall-miss only.
//!
//! The caller engages the gate only when every completion source
//! opened at setup (its metadata view): a source that failed or was
//! unavailable holds spans the gate cannot see, so every assembly is
//! already degraded and nothing may be pruned toward. A file that opens
//! but later fails to open fully (a corrupt dictionary found at a plan
//! compile or a merge) degrades every assembly from that point on, the
//! same point-of-discovery rule as a corrupt rollup below.
//!
//! # Prune rules
//!
//! The first two rules are preconditions; the last three each prove a
//! non-match on their own:
//!
//! - **Tail presence**: the candidate is in any tail's provenance →
//!   unprunable (tails have no rollup; a tail span could change the
//!   root or extend the envelope).
//! - **Coverage-complete**: every surviving completion file is either
//!   screened out for the candidate (its bloom, or the session's tight
//!   screen) or has a decoded rollup
//!   (a present row is evidence; a missing row in a validly decoded
//!   rollup proves the trace absent from that file). A file with no
//!   rollup chunk (pre-rollup retention) is Uncovered and blocks
//!   pruning for every candidate it might contain.
//! - **No root** (any root condition, either polarity — decision 1D):
//!   every covering row proves LOCAL root absence
//!   (`ROOT_CLAIM_NONE`), so the assembled trace has no unset-parent
//!   span and no root condition can be satisfied
//!   (absent-never-satisfies). A `ROOT_CLAIM_WITHHELD` row (ambiguous
//!   tie abstention) blocks BOTH root rules — real roots exist there,
//!   values unknown.
//! - **All claims fail** (positive conditions only): at least one row
//!   claims a true root AND every claiming row's resolved value fails
//!   the condition. No cross-file root selection is ever attempted —
//!   the rollup has no root start, so "the" root is not identifiable
//!   across files; all-claims-fail needs no ordering.
//! - **Duration**: the MERGED cross-file envelope
//!   (`max(max_end) - min(min_start)`, saturating) falls below the
//!   compiled lower bound. The envelope over-estimates the canonical
//!   duration, so only lower-bound violations are provable.
//!
//! # Corrupt files: skip and surface
//!
//! A file that proves itself corrupt in ANY way — an undecodable TBLM
//! or TRSU chunk, or a rollup ref the resolver cannot map to a proven
//! value in its field (the closed rule; see
//! [`sfst::RollupRootResolver`]) — becomes a FAILED SOURCE from the
//! point of discovery:
//!
//! - the gate stops consulting it and queues the discovery for the
//!   caller ([`TraceGate::take_new_failures`]), which surfaces the
//!   skip (`SourceFailure` + degraded assembly) — never a silent
//!   downgrade;
//! - the candidate that exposed the corruption is excluded either
//!   way: a resolver-corrupt ref returns `Assemble` directly, while a
//!   TBLM/TRSU decode failure only drops the file from the evidence
//!   set — the candidate MAY then prune on the remaining files,
//!   result-neutral ONLY because the caller's `degraded_assembly`
//!   flag makes the truth path exclude every post-discovery candidate
//!   as indeterminate. If that tri-state exclusion ever weakens, this
//!   prune becomes a false prune — the two contracts are coupled;
//! - an ABSENT chunk is a version fact, not corruption: absent TBLM =
//!   might-contain for every candidate; absent TRSU = Uncovered.
//!
//! Recorded status delta: a pruned candidate's assembly never runs,
//! so corruption living only in chunks that assembly alone reads
//! (columns, stream batches) goes UNDISCOVERED on gate-engaged
//! queries that prune the affected candidates — gate-off would have
//! surfaced `SourceFailure`. Same class as the pinned SizeCap delta
//! (and gate-off queries never read TRSU, so laziness of discovery
//! cuts both ways).
//!
//! # The single-source proof
//!
//! Almost every trace lives in exactly one file (99.4% of a day's
//! traces). When a gate engages, setup records every completion
//! source's trace ids once per source in a fixed-size
//! [`sfst::RepeatFilter`] ([`single_source_proof`]): every file's
//! rollup ids and every tail's distinct ids. A candidate the filter
//! proves single-source, emitted by a window file, lives in that file
//! alone: the trace-level rules read that one rollup row (coverage is
//! complete by the proof), and the token gate ([`tokens_prune`]) reads
//! that one file's rows. Rollup discovery applies the same row rules
//! while it reads a window file's rollup ([`TraceGate::check_row`]), so
//! such a trace never enters the candidate pool. Other candidates (straddling traces and the
//! filter's false positives) take the all-file path above. A file
//! without a rollup, or one that fails to decode it, disables the proof
//! for the query: its ids are unknown. So does passing
//! [`PROOF_IDS_MAX`]: the proof is an accelerator with its own budget,
//! never a cut-short filter (an unrecorded id would look single-source).
//!
//! # The token gate
//!
//! With two or more positive span-local tokens, discovery follows one
//! (the driver) and the others are checked at assembly. For a
//! single-source candidate, every other positive token's compiled set
//! for its file is probed at the candidate's rows: a token matching
//! none of them prunes. Raw rows over-approximate the canonical spans
//! and the sets hold in-window rows only, so no raw match proves no
//! canonical in-window match. Negated tokens never prune (a raw match
//! may be a losing resend).
//!
//! # Cost accounting
//!
//! Pruned pops deliberately do NOT charge the assembled ceiling (that
//! is the point). The proof's recorded ids count against its own
//! [`PROOF_IDS_MAX`], so the setup never eats the discovery budget.
//! Every other read is charged to the visited ceiling, one unit each: a
//! tail span collected for the gates, a screen, a rollup row, a
//! root-ref resolution, a candidate's rows and each membership probe. Once the ceiling is breached the caller stops
//! consulting the gates (assembling is always sound), so their work
//! overshoots by at most one check. Probes and decodes are also counted
//! in [`GateStats`] and logged once per query via `tracing::debug!` by
//! the caller.
//!
//! # Memory
//!
//! The gate keeps no decoded file state of its own: a file's rollup and
//! root-value dictionaries live in the caller's session (its loaded
//! layer), every read reports to the query's
//! [`LoadedSources`](sfst::LoadedSources) — so a candidate held by
//! hundreds of files still keeps few loaded — and a file is screened
//! before its rollup is read, so only files that hold the candidate
//! decode one.

use std::collections::HashSet;

use sfst::ScanWork;

use super::predicate::{EvalMatcher, GateRootField, TraceLevelEval};
use super::vocab::{BuiltinField, resource_service_field};

/// The single-source proof's budget in recorded ids (one per trace per
/// completion source): about three days of the measured demo load (a
/// day's completion range held ~2.3 M). Past it the proof is off.
pub(crate) const PROOF_IDS_MAX: u64 = 8_000_000;

/// The `name` dictionary spelling, from the shared vocabulary — the
/// same source the truth path reads, never hand-written (a divergent
/// spelling would resolve as `Corrupt` and spuriously fail files).
fn name_field() -> &'static str {
    BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed")
}

/// The gate's verdict for one popped candidate.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum GateDecision {
    /// Proven non-match: skip the assembly.
    Prune,
    /// Not provable: assemble; post-assembly evaluation decides.
    Assemble,
}

/// Advisory probe/decode counters, logged once per query.
#[derive(Debug, Default)]
pub(crate) struct GateStats {
    pub checks: u64,
    pub single_source: u64,
    pub rows_decided: u64,
    pub prunes: u64,
    pub no_root_prunes: u64,
    pub tail_unprunable: u64,
    pub uncovered_unprunable: u64,
    pub screens: u64,
    pub rollup_rows: u64,
    pub corrupt_files: u64,
}

/// One rollup row of evidence for the candidate under test.
#[derive(Clone, Copy)]
struct RowEvidence {
    file: usize,
    row: sfst::RollupRow,
}

/// One claiming row's verdict against one root condition.
enum RowVerdict {
    Matches,
    Fails,
    /// Unresolvable ref (the closed corruption rule): the caller marks
    /// the file failed; this candidate assembles.
    Corrupt,
}

/// The trace-level pre-assembly gate over one query's completion
/// sessions: the compiled prune rules, consulted per popped candidate.
pub(crate) struct TraceGate<'e> {
    /// Per completion session: proven corrupt, so out of the evidence
    /// set (the caller also removes it from the truth by marking the
    /// source failed).
    failed: Vec<bool>,
    /// Trace ids with ANY tail presence — unprunable wholesale.
    tail_ids: HashSet<sfst::TraceId>,
    /// Positive root conditions; matchers borrow the compiled eval.
    root_conditions: Vec<(GateRootField, &'e EvalMatcher)>,
    /// ANY root condition exists (negated included) — the no-root
    /// prune's trigger under true-root filter semantics (decision 1D).
    has_root_conditions: bool,
    /// The resource `service.name` storage spelling, computed once.
    service_field: String,
    duration_lower_bound: Option<i64>,
    /// Reader indices newly proven corrupt, for the caller to surface.
    new_failures: Vec<usize>,
    pub(crate) stats: GateStats,
}

impl<'e> TraceGate<'e> {
    /// Build the gate over `files` completion sessions (indices align
    /// with the caller's session/origin order; each check lends them)
    /// and the eagerly collected tail provenance ids.
    pub(crate) fn new(
        files: usize,
        tail_ids: HashSet<sfst::TraceId>,
        eval: &'e TraceLevelEval,
    ) -> Self {
        Self {
            failed: vec![false; files],
            tail_ids,
            root_conditions: eval.prunable_root_conditions().collect(),
            has_root_conditions: eval.has_root_conditions(),
            service_field: resource_service_field(),
            duration_lower_bound: eval.duration_lower_bound(),
            new_failures: Vec::new(),
            stats: GateStats::default(),
        }
    }

    /// Reader indices proven corrupt since the last call. The caller
    /// MUST surface each (warn + `SourceFailure` + degraded assembly)
    /// — the skip-and-surface half of the corrupt-file principle.
    pub(crate) fn take_new_failures(&mut self) -> Vec<usize> {
        std::mem::take(&mut self.new_failures)
    }

    /// Decide one popped candidate. `single_file` names the candidate's
    /// only source when the single-source proof holds (module docs);
    /// every read is charged to `work`. Always sound to ignore
    /// (assembling a prunable candidate is never wrong — only slower).
    pub(crate) fn check(
        &mut self,
        trace_id: sfst::TraceId,
        single_file: Option<usize>,
        sessions: &mut [sfst::TraceFileSession<'_, '_>],
        loaded: &mut sfst::LoadedSources,
        work: &mut ScanWork,
    ) -> GateDecision {
        self.stats.checks += 1;
        let mut rows: Vec<RowEvidence> = Vec::new();
        if let Some(idx) = single_file {
            // ── The proof: this file alone holds the candidate ────────
            self.stats.single_source += 1;
            if self.failed[idx] {
                return GateDecision::Assemble;
            }
            work.rows_visited += 1;
            self.stats.rollup_rows += 1;
            let evidence = sessions[idx].rollup_row(trace_id);
            loaded.touch(idx, sessions);
            match evidence {
                Ok(sfst::RollupEvidence::Row(row)) => rows.push(RowEvidence { file: idx, row }),
                // The proof recorded this file's rollup ids, so a row
                // must exist; without one there is nothing to decide on.
                Ok(_) => return GateDecision::Assemble,
                Err(e) => {
                    tracing::warn!("sfsq traces gate: trace rollup failed to decode: {e}");
                    self.mark_failed(idx);
                    return GateDecision::Assemble;
                }
            }
        } else {
            if self.tail_ids.contains(&trace_id) {
                self.stats.tail_unprunable += 1;
                return GateDecision::Assemble;
            }

            // ── Evidence collection: screen, then rollup row ─────────
            for idx in 0..self.failed.len() {
                if self.failed[idx] {
                    continue;
                }
                work.rows_visited += 1;
                if !self.screen(idx, trace_id, sessions) {
                    continue; // proven absent from this file, or file failed
                }
                work.rows_visited += 1;
                self.stats.rollup_rows += 1;
                let evidence = sessions[idx].rollup_row(trace_id);
                loaded.touch(idx, sessions);
                match evidence {
                    Ok(sfst::RollupEvidence::NoRollup) => {
                        // Short-circuit: one uncovered screened-in file
                        // already forces UNPRUNABLE.
                        self.stats.uncovered_unprunable += 1;
                        return GateDecision::Assemble;
                    }
                    // A missing row in a valid rollup proves the trace
                    // absent from this file — nothing to collect.
                    Ok(sfst::RollupEvidence::Absent) => {}
                    Ok(sfst::RollupEvidence::Row(row)) => {
                        rows.push(RowEvidence { file: idx, row });
                    }
                    Err(e) => {
                        tracing::warn!("sfsq traces gate: trace rollup failed to decode: {e}");
                        self.mark_failed(idx);
                    }
                }
            }
        }
        // Coverage-complete now holds over the surviving files.
        self.decide(&rows, sessions, loaded, work)
    }

    /// Decide a candidate from ONE rollup row of a file the single-source
    /// proof says holds it alone (rollup discovery's local verdict) —
    /// the same rules as [`check`](Self::check), on that row.
    pub(crate) fn check_row(
        &mut self,
        file: usize,
        row: sfst::RollupRow,
        sessions: &mut [sfst::TraceFileSession<'_, '_>],
        loaded: &mut sfst::LoadedSources,
        work: &mut ScanWork,
    ) -> GateDecision {
        if self.failed[file] {
            return GateDecision::Assemble;
        }
        self.stats.rows_decided += 1;
        self.decide(&[RowEvidence { file, row }], sessions, loaded, work)
    }

    /// The prune rules over the candidate's covering rows (every file
    /// that may hold it, coverage complete).
    fn decide(
        &mut self,
        rows: &[RowEvidence],
        sessions: &mut [sfst::TraceFileSession<'_, '_>],
        loaded: &mut sfst::LoadedSources,
        work: &mut ScanWork,
    ) -> GateDecision {
        // ── Root claim census (decision 1D) ──────────────────────────
        let mut any_claim = false;
        let mut any_withheld = false;
        for r in rows {
            if self.failed[r.file] {
                continue;
            }
            match r.row.root_claim {
                sfst::ROOT_CLAIM_TRUE => any_claim = true,
                sfst::ROOT_CLAIM_WITHHELD => any_withheld = true,
                _ => {}
            }
        }
        // No-root prune: every covering row proves LOCAL root absence,
        // so the assembled trace has no unset-parent span — no root
        // condition, negated included, can be satisfied (decision 1D).
        if self.has_root_conditions && !rows.is_empty() && !any_claim && !any_withheld {
            self.stats.no_root_prunes += 1;
            self.stats.prunes += 1;
            return GateDecision::Prune;
        }

        // ── Root rule: prune iff ≥1 true-root claim and ALL fail ─────
        // Index-iterated, copying each (Copy) pair out per round — the
        // matcher borrows the eval, not the gate, so per-row file state
        // stays freely borrowable without cloning the list per pop.
        // Withheld claims hide real unset-parent spans of unknown
        // value — all-claims-fail is unprovable, so the rule is skipped.
        let positive_conditions = if any_withheld {
            0
        } else {
            self.root_conditions.len()
        };
        'condition: for ci in 0..positive_conditions {
            let (field, matcher) = self.root_conditions[ci];
            let mut claiming = 0usize;
            for &RowEvidence { file, row } in rows {
                if self.failed[file] {
                    continue; // defensive: never read evidence from a failed file
                }
                if row.root_claim != sfst::ROOT_CLAIM_TRUE {
                    continue;
                }
                claiming += 1;
                let kv_ref = match field {
                    GateRootField::Service => row.root_service_ref,
                    GateRootField::Name => row.root_name_ref,
                };
                if kv_ref == sfst::ROLLUP_NO_REF {
                    continue; // absent never satisfies: this row fails
                }
                work.rows_visited += 1;
                let verdict = self.resolve_row(file, field, kv_ref, matcher, sessions);
                loaded.touch(file, sessions);
                match verdict {
                    RowVerdict::Matches => continue 'condition,
                    RowVerdict::Fails => {}
                    RowVerdict::Corrupt => {
                        // Closed rule: unresolvable ref = corrupt file.
                        // Conservative for THIS candidate; the file is
                        // out of the evidence set for later pops.
                        self.mark_failed(file);
                        return GateDecision::Assemble;
                    }
                }
            }
            if claiming > 0 {
                // Every claiming row failed the condition. (No claim at
                // all is an evidence gap, never a proven non-match: zero
                // evidence proves nothing, and the corruption-onset
                // candidate reaches exactly this state.)
                self.stats.prunes += 1;
                return GateDecision::Prune;
            }
        }

        // ── Duration rule: merged envelope below the lower bound ─────
        if let Some(bound) = self.duration_lower_bound {
            let mut envelope: Option<(i64, i64)> = None;
            for &RowEvidence { file, row } in rows {
                if self.failed[file] {
                    continue;
                }
                let (min_start, max_end) = (row.min_start_ns, row.max_end_ns);
                envelope = Some(match envelope {
                    None => (min_start, max_end),
                    Some((lo, hi)) => (lo.min(min_start), hi.max(max_end)),
                });
            }
            // Zero covering rows → survive (never fold an empty set).
            if let Some((lo, hi)) = envelope {
                if hi.saturating_sub(lo) < bound {
                    self.stats.prunes += 1;
                    return GateDecision::Prune;
                }
            }
        }

        GateDecision::Assemble
    }

    /// Resolve one claiming row's ref and apply the condition matcher.
    fn resolve_row(
        &self,
        file: usize,
        field: GateRootField,
        kv_ref: u32,
        matcher: &EvalMatcher,
        sessions: &mut [sfst::TraceFileSession<'_, '_>],
    ) -> RowVerdict {
        let field_name: &str = match field {
            GateRootField::Service => &self.service_field,
            GateRootField::Name => name_field(),
        };
        // Resolving a value needs the full reader (the dictionaries);
        // a file that cannot open fully cannot prove its claim.
        match sessions[file].resolve_rollup_ref(field_name, kv_ref) {
            Ok(sfst::RollupRefOutcome::Value(value)) => {
                if matcher.value_matches(value) {
                    RowVerdict::Matches
                } else {
                    RowVerdict::Fails
                }
            }
            Ok(sfst::RollupRefOutcome::Corrupt) | Err(_) => RowVerdict::Corrupt,
        }
    }

    /// Whether the candidate MIGHT be in file `idx` (the session's
    /// bloom, then its screen). An unreadable bloom fails the file
    /// (corrupt) and returns `false` — the file is no longer evidence OR
    /// truth.
    fn screen(
        &mut self,
        idx: usize,
        trace_id: sfst::TraceId,
        sessions: &mut [sfst::TraceFileSession<'_, '_>],
    ) -> bool {
        if sessions[idx].bloom_unreadable() {
            self.mark_failed(idx);
            return false;
        }
        self.stats.screens += 1;
        sessions[idx].screen(trace_id)
    }

    /// Record the file as proven corrupt: out of the evidence set from
    /// now on, queued for the caller to surface.
    fn mark_failed(&mut self, idx: usize) {
        if !self.failed[idx] {
            self.failed[idx] = true;
            self.new_failures.push(idx);
            self.stats.corrupt_files += 1;
        }
    }
}

/// The single-source proof (module docs): record every completion
/// source's trace ids once per source — each file's rollup ids, then
/// each tail's distinct ids — stopping once more than `max_ids` are
/// recorded. `Ok(None)` when a file has no rollup (its ids are unknown,
/// so no id is provably single-source), the budget ran out, or
/// `cancelled` fired. `Err((file, error))` when a file's rollup fails
/// to decode: a corrupt source the caller surfaces (module docs).
pub(crate) fn single_source_proof(
    sessions: &mut [sfst::TraceFileSession<'_, '_>],
    tail_ids: &[HashSet<sfst::TraceId>],
    loaded: &mut sfst::LoadedSources,
    max_ids: u64,
    cancelled: &dyn Fn() -> bool,
) -> Result<Option<sfst::RepeatFilter>, (usize, sfst::Error)> {
    let mut filter = sfst::RepeatFilter::new();
    let mut recorded = 0u64;
    for idx in 0..sessions.len() {
        if cancelled() {
            return Ok(None);
        }
        let visited = sessions[idx].for_each_rollup_row(|id, _| filter.record(id));
        loaded.touch(idx, sessions);
        match visited {
            Ok(Some(n)) => recorded += n as u64,
            Ok(None) => return Ok(None),
            Err(e) => return Err((idx, e)),
        }
        if recorded > max_ids {
            return Ok(None);
        }
    }
    for ids in tail_ids {
        for &id in ids {
            filter.record(id);
        }
        recorded += ids.len() as u64;
        if recorded > max_ids {
            return Ok(None);
        }
    }
    Ok(Some(filter))
}

/// The token gate (module docs) for a candidate held by `session`'s
/// file alone: `true` (prune) when one of `others` — the file's compiled
/// sets of the positive span-local tokens other than the driver —
/// matches none of the candidate's rows. Charged: one unit for the row
/// lookup, then one per row and per probe. A lookup the file cannot
/// answer prunes nothing (the assembly reads the same rows and surfaces
/// the failure).
pub(crate) fn tokens_prune(
    trace_id: sfst::TraceId,
    session: &mut sfst::TraceFileSession<'_, '_>,
    others: &[sfst::CompiledTracePlan],
    work: &mut ScanWork,
) -> bool {
    work.rows_visited += 1;
    let Ok(positions) = session.positions(trace_id) else {
        return false;
    };
    work.rows_visited += positions.len() as u64;
    for plan in others {
        work.rows_visited += positions.len() as u64;
        if !positions.iter().any(|&pos| plan.contains(pos)) {
            return true;
        }
    }
    false
}
