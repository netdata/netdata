//! The trace-level pre-assembly gate: the rollup-backed superset
//! filter that spares [`search`](super::search::search) an assembly per
//! provably non-matching candidate.
//!
//! Trace-level predicates (`trace:root_service_name` / `trace:root_name`
//! selections, `trace:duration` lower bounds) are decided post-assembly;
//! without a gate every candidate in the window is assembled just to be
//! discarded, and rare predicates burn the assembled ceiling on
//! discards. The per-file `TRSU` rollup already stores each trace's
//! envelope and recorded root per file, so the gate sits at the examine
//! loop's `pool.pop()` and asks: could this candidate possibly match?
//!
//! # The superset contract
//!
//! The gate only ever SKIPS an assembly when the rollup evidence PROVES
//! a non-match; failing any precondition means UNPRUNABLE — the
//! candidate assembles and the post-assembly evaluator stays the truth.
//! The known exception is the documented recorded-vs-canonical root
//! divergence mechanisms (see [`search`](super::search)'s module docs):
//! accepted, recall-miss only, by explicit ruling.
//!
//! Prune preconditions, per candidate:
//!
//! - NOT in any tail's provenance (tails have no rollup; a tail span
//!   could change the root or extend the envelope).
//! - Coverage-complete: every surviving completion file is either
//!   bloom-proven absent for the candidate or has a decoded rollup
//!   (a present row is evidence; a missing row in a validly decoded
//!   rollup proves the trace absent from that file). A file with no
//!   rollup chunk (pre-rollup retention) is Uncovered and blocks
//!   pruning for every candidate it might contain.
//! - No-root prune (any root condition, either polarity — decision
//!   1D): when every covering row PROVES local root absence
//!   (`ROOT_CLAIM_NONE`), the assembled trace has no unset-parent span
//!   and no root condition can be satisfied (absent-never-satisfies).
//!   A `ROOT_CLAIM_WITHHELD` row (ambiguous tie abstention) blocks
//!   BOTH root rules — real roots exist there, values unknown.
//! - All-claims-fail (positive conditions only): at least one row
//!   claims a true root AND every claiming row's resolved value fails
//!   the condition. No cross-file root selection is ever attempted —
//!   the rollup has no root start, so "the" root is not identifiable
//!   across files; all-claims-fail needs no ordering.
//! - Duration rule: the MERGED cross-file envelope
//!   (`max(max_end) - min(min_start)`, saturating) falls below the
//!   compiled lower bound. The envelope over-estimates the canonical
//!   duration, so only lower-bound violations are provable.
//!
//! # Corrupt files: skip and surface
//!
//! Per the project principle, a file that proves itself corrupt in ANY
//! way — an undecodable TBLM or TRSU chunk, or a rollup ref the
//! resolver cannot map to a proven value in its field (the closed rule;
//! see [`sfst::RollupRootResolver`]) — becomes a FAILED SOURCE from the
//! point of discovery: the gate stops consulting it, and the caller
//! surfaces the skip (`SourceFailure` + degraded assembly), never a
//! silent downgrade. The candidate that exposed the corruption is
//! excluded either way: a resolver-corrupt ref returns `Assemble`
//! directly, while a TBLM/TRSU decode failure only drops the file from
//! the evidence set — the candidate MAY then prune on the remaining
//! files, which is result-neutral ONLY because the caller's
//! `degraded_assembly` flag makes the truth path exclude every
//! post-discovery candidate as indeterminate. If that tri-state
//! exclusion ever weakens, this prune becomes a false prune — the two
//! contracts are coupled. An ABSENT chunk is a version fact, not
//! corruption: absent TBLM = might-contain for every candidate; absent
//! TRSU = Uncovered.
//!
//! One consequence is RECORDED as a deliberate status delta: a pruned
//! candidate's assembly never runs, so corruption living only in chunks
//! that assembly alone reads (columns, stream batches) goes
//! UNDISCOVERED on gate-engaged queries that prune the affected
//! candidates — gate-off would have surfaced `SourceFailure`. Same
//! class as the pinned SizeCap delta: the would-have-been partials of a
//! pruned candidate are unobserved by design (and gate-off queries
//! never read TRSU, so laziness of discovery cuts both ways).
//!
//! # Cost accounting
//!
//! Pruned pops deliberately do NOT charge the assembled ceiling (that
//! is the point) and the gate's probes are uncharged — the trade for
//! the assemblies they replace. Termination is by pool drain; refills
//! stay bounded by the visited-rows ceiling. Decodes and probes are
//! counted in [`GateStats`] and surfaced via `tracing::debug!` by the
//! caller.

use std::collections::HashSet;

use super::predicate::{EvalMatcher, GateRootField, TraceLevelEval};
use super::vocab::{BuiltinField, resource_service_field};

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
    pub prunes: u64,
    pub no_root_prunes: u64,
    pub tail_unprunable: u64,
    pub uncovered_unprunable: u64,
    pub bloom_probes: u64,
    pub bloom_decodes: u64,
    pub rollup_decodes: u64,
    pub corrupt_files: u64,
}

/// Per-file bloom state, resolved once per file.
enum BloomState {
    Unprobed,
    /// No TBLM chunk: might-contain for every candidate (absence is a
    /// version fact — mirrors the session's `BloomGate::Pass`).
    PassAll,
    Ready(sfst::TraceIdBloom),
}

/// Per-file rollup state, resolved once per file.
enum RollupState {
    Undecoded,
    /// No TRSU chunk (pre-rollup file): blocks pruning for every
    /// candidate the file might contain.
    Uncovered,
    Ready(Box<sfst::TraceRollup>),
}

struct GateFile<'r, 'a> {
    reader: &'r sfst::IndexReader<'a>,
    bloom: BloomState,
    rollup: RollupState,
    resolver: Option<sfst::RollupRootResolver<'r, 'a>>,
    /// Proven corrupt: out of the evidence set (the caller also removes
    /// it from the truth by marking the source failed).
    failed: bool,
}

/// One rollup row of evidence for the candidate under test.
struct RowEvidence {
    file: usize,
    row: usize,
    /// The row's root-claim tri-state (`sfst::ROOT_CLAIM_*`).
    claim: u8,
}

/// One claiming row's verdict against one root condition.
enum RowVerdict {
    Matches,
    Fails,
    Corrupt,
}

pub(crate) struct TraceGate<'r, 'a, 'e> {
    files: Vec<GateFile<'r, 'a>>,
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

impl<'r, 'a, 'e> TraceGate<'r, 'a, 'e> {
    /// Build the gate over the completion readers (indices align with
    /// the caller's reader/session/origin order) and the eagerly
    /// collected tail provenance ids.
    pub(crate) fn new(
        readers: Vec<&'r sfst::IndexReader<'a>>,
        tail_ids: HashSet<sfst::TraceId>,
        eval: &'e TraceLevelEval,
    ) -> Self {
        Self {
            files: readers
                .into_iter()
                .map(|reader| GateFile {
                    reader,
                    bloom: BloomState::Unprobed,
                    rollup: RollupState::Undecoded,
                    resolver: None,
                    failed: false,
                })
                .collect(),
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

    /// Decide one popped candidate. Always sound to ignore (assembling
    /// a prunable candidate is never wrong — only slower).
    pub(crate) fn check(&mut self, trace_id: sfst::TraceId) -> GateDecision {
        self.stats.checks += 1;
        if self.tail_ids.contains(&trace_id) {
            self.stats.tail_unprunable += 1;
            return GateDecision::Assemble;
        }

        // ── Evidence collection: bloom probe, then rollup row ────────
        let mut rows: Vec<RowEvidence> = Vec::new();
        for idx in 0..self.files.len() {
            if self.files[idx].failed {
                continue;
            }
            if !self.probe_bloom(idx, trace_id) {
                continue; // proven absent from this file, or file failed
            }
            match self.decode_rollup(idx) {
                RollupProbe::Uncovered => {
                    // Short-circuit: one uncovered bloom-present file
                    // already forces UNPRUNABLE.
                    self.stats.uncovered_unprunable += 1;
                    return GateDecision::Assemble;
                }
                RollupProbe::Failed => continue,
                RollupProbe::Ready => {
                    let RollupState::Ready(rollup) = &self.files[idx].rollup else {
                        unreachable!("probe said Ready")
                    };
                    if let Some(row) = rollup.find(trace_id) {
                        rows.push(RowEvidence {
                            file: idx,
                            row,
                            claim: rollup.root_is_true_root[row],
                        });
                    }
                    // A missing row in a valid rollup proves the trace
                    // absent from this file — nothing to collect.
                }
            }
        }
        // Coverage-complete now holds over the surviving files.

        // ── Root claim census (decision 1D) ──────────────────────────
        let mut any_claim = false;
        let mut any_withheld = false;
        for r in &rows {
            if self.files[r.file].failed {
                continue;
            }
            match r.claim {
                sfst::ROOT_CLAIM_TRUE => any_claim = true,
                sfst::ROOT_CLAIM_WITHHELD => any_withheld = true,
                _ => {}
            }
        }
        // No-root prune: every covering row proves LOCAL root absence,
        // so the assembled trace (tails already excluded) has no
        // unset-parent span — under true-root filter semantics no root
        // condition, negated included, can be satisfied.
        if self.has_root_conditions && !rows.is_empty() && !any_claim && !any_withheld {
            self.stats.no_root_prunes += 1;
            self.stats.prunes += 1;
            return GateDecision::Prune;
        }

        // ── Root rule: prune iff ≥1 true-root claim and ALL fail ─────
        // Index-iterated, copying each (Copy) pair out per round — the
        // matcher borrows the eval, not the gate, so per-row file state
        // stays freely borrowable without cloning the list per pop. A
        // withheld claim hides real unset-parent spans with unknown
        // values, so all-claims-fail cannot be proven — rule skipped.
        let positive_conditions = if any_withheld { 0 } else { self.root_conditions.len() };
        'condition: for ci in 0..positive_conditions {
            let (field, matcher) = self.root_conditions[ci];
            let mut claiming = 0usize;
            for &RowEvidence { file, row, claim } in &rows {
                if self.files[file].failed {
                    continue; // failed mid-check while testing this candidate
                }
                let RollupState::Ready(rollup) = &self.files[file].rollup else {
                    unreachable!("evidence rows come from Ready rollups")
                };
                if claim != sfst::ROOT_CLAIM_TRUE {
                    continue;
                }
                claiming += 1;
                let kv_ref = match field {
                    GateRootField::Service => rollup.root_service_refs[row],
                    GateRootField::Name => rollup.root_name_refs[row],
                };
                if kv_ref == sfst::ROLLUP_NO_REF {
                    continue; // absent never satisfies: this row fails
                }
                match self.resolve_row(file, field, kv_ref, matcher) {
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
                // all would mean the assembled root may be an orphan
                // promotion the rollup never records — unprunable.)
                self.stats.prunes += 1;
                return GateDecision::Prune;
            }
        }

        // ── Duration rule: merged envelope below the lower bound ─────
        if let Some(bound) = self.duration_lower_bound {
            let mut envelope: Option<(i64, i64)> = None;
            for &RowEvidence { file, row, .. } in &rows {
                if self.files[file].failed {
                    continue;
                }
                let RollupState::Ready(rollup) = &self.files[file].rollup else {
                    unreachable!("evidence rows come from Ready rollups")
                };
                let (min_start, max_end) =
                    (rollup.min_start_ns[row], rollup.max_end_ns[row]);
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
        &mut self,
        file: usize,
        field: GateRootField,
        kv_ref: u32,
        matcher: &EvalMatcher,
    ) -> RowVerdict {
        let field_name: &str = match field {
            GateRootField::Service => &self.service_field,
            GateRootField::Name => name_field(),
        };
        let entry = &mut self.files[file];
        let reader = entry.reader;
        let resolver = entry
            .resolver
            .get_or_insert_with(|| sfst::RollupRootResolver::new(reader));
        match resolver.resolve(field_name, kv_ref) {
            sfst::RollupRefOutcome::Value(value) => {
                if matcher.value_matches(value) {
                    RowVerdict::Matches
                } else {
                    RowVerdict::Fails
                }
            }
            sfst::RollupRefOutcome::Corrupt => RowVerdict::Corrupt,
        }
    }

    /// Whether the candidate MIGHT be in file `idx` (bloom semantics).
    /// A bloom decode failure fails the file (corrupt) and returns
    /// `false` — the file is no longer evidence OR truth.
    fn probe_bloom(&mut self, idx: usize, trace_id: sfst::TraceId) -> bool {
        if matches!(self.files[idx].bloom, BloomState::Unprobed) {
            let reader = self.files[idx].reader;
            self.files[idx].bloom = if !reader.has_trace_id_bloom() {
                BloomState::PassAll
            } else {
                self.stats.bloom_decodes += 1;
                match reader.trace_id_bloom() {
                    Ok(bloom) => BloomState::Ready(bloom),
                    Err(e) => {
                        tracing::warn!(
                            "sfsq traces gate: trace-id bloom failed to decode: {e}"
                        );
                        self.mark_failed(idx);
                        return false;
                    }
                }
            };
        }
        self.stats.bloom_probes += 1;
        match &self.files[idx].bloom {
            BloomState::PassAll => true,
            BloomState::Ready(bloom) => bloom.might_contain(trace_id),
            BloomState::Unprobed => unreachable!("resolved above"),
        }
    }

    fn decode_rollup(&mut self, idx: usize) -> RollupProbe {
        if matches!(self.files[idx].rollup, RollupState::Undecoded) {
            let reader = self.files[idx].reader;
            self.files[idx].rollup = if !reader.has_trace_rollup() {
                RollupState::Uncovered
            } else {
                self.stats.rollup_decodes += 1;
                match reader.trace_rollup() {
                    Ok(rollup) => RollupState::Ready(Box::new(rollup)),
                    Err(e) => {
                        tracing::warn!(
                            "sfsq traces gate: trace rollup failed to decode: {e}"
                        );
                        self.mark_failed(idx);
                        return RollupProbe::Failed;
                    }
                }
            };
        }
        match &self.files[idx].rollup {
            RollupState::Uncovered => RollupProbe::Uncovered,
            RollupState::Ready(_) => RollupProbe::Ready,
            RollupState::Undecoded => unreachable!("resolved above"),
        }
    }

    fn mark_failed(&mut self, idx: usize) {
        if !self.files[idx].failed {
            self.files[idx].failed = true;
            self.new_failures.push(idx);
            self.stats.corrupt_files += 1;
        }
    }
}

enum RollupProbe {
    Uncovered,
    Failed,
    Ready,
}
