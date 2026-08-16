# Progress

## Phase 1 — Planning

- [x] Status repo created, linked at `.local/status`
- [x] Investigation (done in originating session; evidence in plan.md)
- [x] plan.md draft round 0 written
- [x] linfra round 1 + 3 targeted consults complete and consolidated (round1-consolidation.md)
- [x] plan v2 written (3 real UAFs added; T4 redesigned; broker accessor; transport adapter + provider lifecycle in scope)
- [x] linfra round 2 complete (job_d195fd8d…, 4/4 models) — NOT converged:
  unanimous material findings against D7 ("both paths" wrong for streaming),
  the UAF-B defer handling (destroy-mid-defer breaks G3), the transport
  lifetime model (needs the two-refcount collector shape), and the C3
  flag/set protocol (lost-DEL race). Consolidated with judgments in
  round2-consolidation.md; k3's C4-filter finding verified against code
  (exporters.c:12,:36) and accepted.
- [x] plan v3 written (UAF-D added; D7 rescoped; transport two-refcount
  model; C3 protocol mandate + drain-in-renderer + gates-passed-in; C4
  availability in all filters; C5b corrections; C6 scope += pluginsd_dyncfg)
- [x] linfra round 3 complete (job_097fb646…, 4/4) — NOT converged but
  sharply narrowed: C3/C5b/D7/destroy-mid-defer/two-refcount core declared
  closed by all four; new material findings confined to C6's ownership spec
  (conflict-SWAP source tag 4/4; DYNCFG-node pin 3/4; result_end
  release-then-del; delivery-outside-locks MUST/ABBA; equal-pointer conflict
  netting). Judgments in round3-consolidation.md.
- [x] plan v4 written (C6 ownership amendments + text corrections only)
- [x] linfra round 4 complete (job_0ed8f6dd…, 4/4) — NOT converged; all
  findings are C6 spec-text amendments with agreed fixes (equal-pointer
  underflow 3/4; result_end sweep; DYNCFG pin discriminator; delivery
  mechanism obligations; lookup-time pin). Two model factual disputes
  resolved by me against dictionary code (round4-consolidation.md).
- [x] plan v5 written (collector-pattern conflict accounting; release→del→
  sweep; DYNCFG transport field; delivery obligations; lookup pin;
  forced-503 narrowed to 2xx; round-3 canceller note withdrawn)
- [x] linfra round 5 complete (job_c26fbd7e…, 4/4) — CAP REACHED, loop
  closed. k3: full convergence, zero new findings. glm/deepseek/qwen: four
  narrow, mutually consistent spec amendments (sweep variant + enabling
  invariant; leaf-lock pin mechanism + capture-at-find; key-copy in
  per-victim del; pinned fan-out copy) — no inter-model disagreement.
  Judgments in round5-consolidation.md.
- [x] plan v6 written (round-5 amendments encoded; consultation loop CLOSED)
- [x] Clean-context subagent plan review — verdict SOUND-WITH-AMENDMENTS:
  architecture confirmed against code; 3 material findings, all with
  contained fixes, all accepted (detach-then-deliver dictionary primitive
  replaces dup-based pending sweep; leaf lock extended over swapped STRINGs
  for C4 copies; DYNCFG pin moved into insert/transfer callbacks); C5b
  "latent dangling read" note softened.
- [x] plan v7 written (subagent amendments encoded)
- [ ] Human approval gate (D1, D3-D7 + cap trajectory + subagent verdict)
  — PENDING USER; no implementation before explicit approval.
- [x] Reference searches (pre-verification of plan claims):
  - `host->functions` outside the module: only comments (sqlite_aclk.c:1406-1408,
    sqlite_aclk_node.c:182, pluginsd_parser.c:1532, pluginsd_functions.c:359,371) —
    no code touches the raw dict outside the module. Comments updated in step 7.
  - `functions_view` outside the module: rrdset-index-id.c:149 (destroy),
    rrdcontext-instance.c:59 (`rrdinstance_acquired_functions()` returns the raw
    view when `ri->rrdset` exists — the step 3 migration target).
  - `stop_monotonic_ut` daemon-side beyond broker/parser: dyncfg-inline.c:17 chains
    `rfe->stop_monotonic_ut` into dyncfg callbacks synchronously (valid-during-call,
    matches step 6 contract); health_dyncfg + inicfg/dyncfg receive it as call args.
    Plugin-side hits are in plugin processes (functions_evloop world), unaffected.

## Phase 2 — Implementation

Not started (blocked on plan approval).
