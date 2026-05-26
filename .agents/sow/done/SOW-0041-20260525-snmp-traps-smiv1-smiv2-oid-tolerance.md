# SOW-0041 - SNMP Trap Receiver: SMIv1 / SMIv2 OID Encoding Tolerance

## Status

Status: closed

Sub-state: consolidated into `.agents/sow/current/SOW-0035-20260525-snmp-traps-foundation-mvp.md` on 2026-05-26 by user direction. This SOW is not independently implemented; SOW-0035 owns the receiver-side `ProfileIndex.Lookup` change, tests, and matching documentation.

## Requirements

### Purpose

Make the snmp-traps receiver match real-world traps whose `snmpTrapOID.0` differs from the shipped profile YAML OID by a single `.0.` segment between the enterprise prefix and the specific-trap number.

The shipped OOB trap profile pack is derived mechanically from a large MIB corpus via `tools/snmp-traps-profile-gen/`. The pysmi-based extractor is observed to produce inconsistent OID encoding for the same MIB module name depending on the symbol-table state at compile time. As a result, the pack contains some entries with the SMIv1 RFC 3584 canonical form (`enterprise.0.specific`) and some with the bare SMIv2 form (`enterprise.specific`). Real devices send only one form per trap, but which form they send depends on the MIB definition (TRAP-TYPE vs NOTIFICATION-TYPE) and the agent's SNMP version. The current `ProfileIndex.Lookup` does exact-match map lookup with no tolerance, so any mismatch results in a silent "unknown trap" classification.

This SOW makes the receiver tolerant to the `.0.` ambiguity so a mismatch in either direction is automatically resolved at lookup time. The fix is durable — it absorbs the entire class of MIB-tool inconsistency rather than auditing individual entries.

### User Request

User direction:

> "yes, fix the code. but there may be more than 280. 280 happened to be flipped. many more may be wrong"

Context: a comparison experiment (see Analysis) found 280 trap OIDs that "flipped" between two pysmi runs (158 baseline-with-`.0.` → expanded-without; 4 the other way; 7 OID parent moves; 111 actual MIB-version differences). User correctly observed that the 280 flips are only the cases we caught by comparing two runs — many additional entries in the current shipped pack may have the wrong encoding and we have no way to identify them without a comprehensive audit.

User explicitly asked for the receiver-side defensive fix and a self-contained SOW that can be reviewed with other assistants.

2026-05-26 update:

User confirmed this SOW should be incorporated into ingestion. SOW-0035 now owns the implementation, validation, reviewer loop, artifact updates, and close-out evidence for this scope.

### Assistant Understanding

Facts:

- The receiver decodes SNMPv1 traps and synthesizes a canonical `snmpTrapOID.0` per RFC 3584 §3.1 by computing `enterprise + ".0." + specificTrap` when `genericTrap == 6` (enterprise-specific). Code: `src/go/plugin/go.d/collector/snmp_traps/decode.go:344-351` and `decode.go:144-191`.
- The receiver decodes SNMPv2c/v3 traps by reading the `snmpTrapOID.0` varbind value as-is. Code: `decode.go:262`.
- Profile lookup is a single exact-match map access with no fallback. Code: `src/go/plugin/go.d/collector/snmp_traps/profile.go:161-167`:

  ```go
  func (idx *ProfileIndex) Lookup(oid string) *TrapDef {
      if idx == nil {
          return nil
      }
      return idx.trapsByOID[oid]
  }
  ```

- `Lookup` is invoked from `collector.go:172` (the hot path) and from tests at 21 call sites in `profile_test.go`.
- The shipped profile pack lives under `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/` and contains 351 vendor YAMLs (catalogue at `catalogue.json`).
- RFC 3584 §3.1 ("Mapping SNMPv1 Notification Parameters onto SNMPv2 Notification Parameters") prescribes the SMIv1 → SMIv2 OID translation: when an SMIv1 TRAP-TYPE has `ENTERPRISE bar` and specific-trap-number `N`, the equivalent SMIv2 notification OID is `bar.0.N`. The `.0.` is inserted as part of the canonical conversion.
- SMIv1 TRAP-TYPE definitions resolve naturally to `enterprise.0.specific` per the RFC. SMIv2 NOTIFICATION-TYPE definitions resolve to `parent.N` where `parent` is the OBJECT IDENTIFIER node assigned in the MIB and `N` is the child number — no `.0.` is involved.
- Real agents send the OID intrinsic to the MIB's declared form. A device whose firmware speaks SNMPv1 sends a v1 trap PDU containing the enterprise OID and specific-trap-number; the receiver synthesizes the canonical `.0.` OID for lookup. A device whose firmware speaks SNMPv2c/v3 but whose MIB defines the trap as SMIv1 TRAP-TYPE is required by RFC 3584 to encode the v2/v3 `snmpTrapOID.0` as `enterprise.0.specific` — i.e., the `.0.` is preserved on the wire. Therefore: **the `.0.` form is intrinsic to the MIB's declared trap form, not to the SNMP version used on the wire**.
- An empirical comparison of two pysmi extraction runs on overlapping MIB corpora (baseline vs baseline-plus-27-new-repos) produced these stats:
  - baseline: 51,168 records, 50,206 unique OIDs
  - expanded: 95,761 records, 95,761 unique OIDs
  - common: 49,926 OIDs
  - lost from baseline: 280 OIDs, decomposed as:
    - 111 truly gone (different MIB version picked from a new repo lacking these traps)
    - 158 baseline had `.0.`, expanded dropped it
    - 4 baseline lacked `.0.`, expanded added it
    - 7 OID parent changed entirely (mostly experimental→IANA-final corrections)
  - Same source file produces different OID encodings across runs (e.g., `AIRESPACE-WIRELESS-MIB::bsnDot11StationAuthenticateFail` at `pysnmp/mibs @ f410b42ba1f8afb4f077adf0eff363e81c31eb00`, `src/vendor/cisco/AIRESPACE-WIRELESS-MIB`, resolved to `...14179.2.6.3.0.3` in baseline and `...14179.2.6.3.3` in expanded). The shift is driven by pysmi symbol-table state at compile time, not by file content changes.
- The 162 `.0.` shifts in the empirical sample only represent OIDs that flipped between two runs. The total number of pack entries with the wrong form is unknown; pysmi's behavior is also internally inconsistent within a single run.

Inferences:

- The same defect that caused 162 OIDs to flip between runs must have also affected entries in the shipped pack that did NOT happen to flip between our two specific runs but were nonetheless generated with the wrong form on the run that produced the shipped pack. We cannot enumerate these without a comprehensive MIB-form audit cross-referenced against `TRAP-TYPE` vs `NOTIFICATION-TYPE` declarations in the source MIBs.
- A receiver-side fallback that tries the alternate `.0.` form on lookup miss absorbs this entire class of defect — both the entries we know are wrong and the entries we cannot identify — at the cost of one additional map lookup on every miss.
- The risk of a false-positive match (an unrelated trap accidentally matching) is low because: (a) the index only contains NOTIFICATION-TYPE / TRAP-TYPE entries, not arbitrary OBJECT IDENTIFIERs; (b) two different traps that differ only by `.0.` at the second-to-last position would require two MIBs to make conflicting OID claims, which is rare; (c) exact-match always wins, so the alternate form is only attempted after the primary lookup fails.

Unknowns:

- Exact number of incorrectly-encoded entries in the current shipped pack. A full audit would require comparing each entry against its source MIB declaration. Not in scope for this SOW; the fix renders the question moot.
- Whether any vendor MIB legitimately defines two traps that differ only by the `.0.` segment (i.e., `enterprise.X.0.Y` and `enterprise.X.Y` are both real notifications). Risk surveyed below.

### Acceptance Criteria

- AC1: a trap whose decoder-synthesized `snmpTrapOID.0` is `1.3.6.1.4.1.14179.2.6.3.0.24` correctly identifies a profile entry whose `oid:` is `1.3.6.1.4.1.14179.2.6.3.24`. Verified by unit test exercising `ProfileIndex.Lookup`.
- AC2: a trap whose decoder-synthesized `snmpTrapOID.0` is `1.3.6.1.2.1.33.2.1` (SMIv2 form for UPS-MIB::upsTrapOnBattery) correctly identifies a profile entry whose `oid:` is `1.3.6.1.2.1.33.2.0.1`. Verified by unit test.
- AC3: when both forms exist as separate profile entries, exact-match takes precedence (the alternate-form lookup is only consulted on primary miss). Verified by unit test seeding both forms with distinct names and asserting the exact form wins.
- AC4: OIDs that cannot reasonably have the `.0.` ambiguity (too short, leading/trailing edge) do not produce spurious lookups. Verified by unit tests on edge cases.
- AC5: a trap whose decoder-synthesized `snmpTrapOID.0` matches no profile entry in either form returns `nil` (existing behavior preserved on true miss). Verified by unit test.
- AC6: the lookup-cost overhead on a primary-hit path is zero additional map operations; on a primary-miss path it is exactly one additional map lookup against a deterministically-derived alternate key. Verified by reading the code.
- AC7: the profile-format documentation and the trap subsystem spec (`.agents/sow/specs/snmp-traps/netdata.md`) describe the `.0.` tolerance behavior so operators authoring custom YAMLs understand which form is canonical and that both will match.
- AC8: existing tests in `profile_test.go`, `pipeline_test.go`, `collector.go`-callers continue to pass without modification.

## Analysis

Sources checked:

- `src/go/plugin/go.d/collector/snmp_traps/decode.go` (v1 → canonical OID synthesis at line 344; v2/v3 passthrough at line 262)
- `src/go/plugin/go.d/collector/snmp_traps/profile.go` (Lookup at line 161; ProfileIndex struct at line 156)
- `src/go/plugin/go.d/collector/snmp_traps/collector.go` (hot-path call site at line 172)
- `src/go/plugin/go.d/collector/snmp_traps/profile_test.go` (21 Lookup call sites)
- `src/go/plugin/go.d/collector/snmp_traps/load.go` (index population at line 110-121)
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` (operator-facing profile schema)
- `.agents/sow/specs/snmp-traps/netdata.md` (subsystem spec)
- Experiment outputs under `/tmp/snmptraps-experiment/output-pysmi-expanded/` (baseline vs expanded comparison, 280-OID regression breakdown)
- RFC 3584 §3.1 "Mapping SNMPv1 Notification Parameters onto SNMPv2 Notification Parameters"
- RFC 2576 §3.1 (predecessor with equivalent rule)

Current state:

- `Lookup(oid string) *TrapDef` does a single exact-match map access. No fallback, no normalization.
- The receiver path correctly synthesizes the RFC 3584 canonical form for SMIv1 traps; the gap is purely on the profile-side encoding.
- 351 vendor profile YAMLs are shipped. An empirical sample of 280 lookup-key mismatches shows two distinct classes: (a) format ambiguity (`.0.` flips, 162 / 280), and (b) MIB-version differences (truly gone, 111 / 280). This SOW addresses class (a) only; class (b) is a separate concern about OOB pack content quality, tracked as follow-up.

Risks:

- False-positive matches: if a vendor MIB defines two distinct notification OIDs that differ only by a `.0.` segment at the next-to-last position (e.g., both `enterprise.X.0.Y` and `enterprise.X.Y` are valid trap OIDs in the same or different MIBs), the alternate-form lookup could return a wrong but plausible match. The receiver picks the FIRST exact match before trying alternates, so this only manifests when one form is in the pack and the other is on the wire. Risk impact: the trap is identified as a related-but-wrong notification, leading to incorrect category/severity classification. Risk likelihood: low — across the shipped 351 vendor pack the probability of two MIBs colliding on this exact structural relationship is small, but not zero.
- Operator confusion: operators authoring custom YAMLs may be surprised that "wrong" OIDs still match. Mitigation: profile-format documentation explicitly describes the tolerance.
- Performance: an extra map lookup on miss path. Negligible (Go map is O(1); the receiver is not lookup-bound).
- The fix masks the underlying pysmi inconsistency rather than fixing it. Follow-up SOW (already tracked) for OOB pack regeneration with correct per-MIB form would be a complementary improvement, not a substitute.

## Pre-Implementation Gate

Status: closed as consolidated into SOW-0035 before standalone activation.

Problem / root-cause model:

- SMIv1 TRAP-TYPE definitions encode their canonical OID as `enterprise.0.specific` per RFC 3584 §3.1. SMIv2 NOTIFICATION-TYPE definitions encode as `parent.N` (no `.0.`).
- pysmi sometimes loses or gains the `.0.` segment depending on the symbol-table state when a given MIB is compiled. This is not a content-of-MIB issue but a pysmi behavior.
- Real-world devices send the form intrinsic to their MIB's declaration. Our profile pack is generated by pysmi, so some entries have the wrong form.
- The receiver does exact-match lookup, so wrong-form pack entries fail to match real traps.

Evidence reviewed:

- Receiver source: `decode.go:344-351`, `decode.go:262`, `profile.go:161-167`, `collector.go:172`, `load.go:110-121`.
- Existing profile-format doc: `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`.
- Empirical comparison: `/tmp/snmptraps-experiment/output-pysmi-expanded/comparison.txt` (280-OID regression breakdown, decomposed by failure mode).
- RFC 3584 §3.1, RFC 2576 §3.1, RFC 1908 (SMIv1/v2 coexistence general rules).

Affected contracts and surfaces:

- `ProfileIndex.Lookup` signature: unchanged.
- `ProfileIndex.Lookup` semantics: extended to fall back to alternate `.0.` form on primary miss. Pre-existing exact-match callers behave identically; the only behavior change is on the previously-returns-`nil` path.
- Profile YAML schema: unchanged. Both forms remain valid as entries; operators can author either.
- `profile-format.md` and the trap subsystem spec: must describe the tolerance behavior so operators understand both forms match.
- No change to: decoder, listener, journal writer, dyncfg, dedup, retention, config schema, metric set, OTLP exporter contract.

Existing patterns to reuse:

- Helper functions in `decode.go` (`normalizeOID`, `v1TrapOID`): the alternate-OID computation can live alongside these as a peer of `normalizeOID`, or in `profile.go` adjacent to `Lookup`. Decision below.
- Table-driven unit tests using `map[string]struct{}` keys, per `AGENTS.md` Go test style. Follow this for the new tests added to `profile_test.go`.

Risk and blast radius:

- Blast radius: one helper function plus a single line addition in `Lookup`. Test coverage adds a small table to `profile_test.go`.
- Regression risk: false-positive matches as described in Risks above. Mitigated by exact-match-wins precedence and the low real-world likelihood of MIB OID-shape collisions.
- Compatibility: no schema change; no behavior change for hits; only the previously-nil miss path may now return a value when an alternate-form entry exists.
- Security: zero. Map lookups on read-only profile data. No external input parsing change.
- Operational: deploying the fix on top of the existing pack only changes behavior for previously-unknown traps that happen to have an alternate-form entry; those traps now classify correctly instead of as "unknown".

Sensitive data handling plan:

- This SOW touches receiver code, profile-format documentation, and the subsystem spec. None of these durable artifacts will contain raw secrets, SNMP communities, customer identifiers, private endpoints, or proprietary incident details. Empirical evidence cited above is from a public MIB corpus and refers only to OID numerical values and MIB symbolic names (both publicly documented).

Implementation plan:

1. Add `alternateTrapOID(oid string) string` helper in `profile.go` (or `decode.go` if reviewers prefer co-location with `v1TrapOID`). Algorithm: locate the last dot in `oid`; if the segment immediately preceding the last segment is exactly `"0"`, return `oid` with that `.0` removed; otherwise return `oid` with `.0` inserted before the last segment. Reject (return unchanged) on degenerate inputs (no dot, leading dot, fewer than two dots).
2. Modify `ProfileIndex.Lookup` to attempt the alternate form on primary miss. Exact match retains precedence:

   ```go
   func (idx *ProfileIndex) Lookup(oid string) *TrapDef {
       if idx == nil {
           return nil
       }
       if td := idx.trapsByOID[oid]; td != nil {
           return td
       }
       if alt := alternateTrapOID(oid); alt != oid {
           if td := idx.trapsByOID[alt]; td != nil {
               return td
           }
       }
       return nil
   }
   ```

3. Add a table-driven `TestProfileIndex_Lookup_SMIv1SMIv2Tolerance` in `profile_test.go` covering: AC1 (with→without), AC2 (without→with), AC3 (both forms present, exact wins), AC4 (degenerate inputs), AC5 (genuine miss returns nil). Use `map[string]struct{}` keyed by case name per AGENTS.md style.
4. Add a unit test for `alternateTrapOID` covering: normal `.0.` insertion, normal `.0.` removal, leading-dot stripping (defer to existing `normalizeOID` if reused), too-short OIDs, OIDs with `.0.` at non-trap positions (e.g., `1.0.X.Y` where the leading `.0` is part of the prefix), OIDs with multiple `.0.` segments where only the last-position one should flip.
5. Update `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` with a section explaining the canonical RFC 3584 form, the encoding ambiguity, and the receiver tolerance.
6. Update `.agents/sow/specs/snmp-traps/netdata.md` §7 (Profile loading) or a new sub-section noting that receiver lookup is tolerant to the `.0.` ambiguity.
7. Update `tools/snmp-traps-profile-gen/README.md` with a note that the extractor's output may have inconsistent `.0.` encoding and that the receiver tolerates this — operators can author either form.

Validation plan:

- Run `go test ./src/go/plugin/go.d/collector/snmp_traps/...` and verify all existing tests pass plus the new tests for AC1-AC5.
- Manual verification using a synthetic trap (gosnmp test helper or existing pcap fixture) sending a v1 trap with `enterprise=1.3.6.1.4.1.14179.2.6.3, specific=24` against a pack containing only `1.3.6.1.4.1.14179.2.6.3.24` — confirm the trap classifies correctly.
- Reverse case: send a v2c trap with `snmpTrapOID.0 = 1.3.6.1.2.1.33.2.1` against a pack containing `1.3.6.1.2.1.33.2.0.1` — confirm classification.
- Cross-check `decode.go:344-351` v1 synthesis path is unaffected (no code change there; only `Lookup` consumes the synthesized OID).
- Same-failure scan: search the rest of the codebase for similar exact-match-only lookups on OIDs that may arrive in either form (`grep -rn "trapsByOID\|varbindsByOID\|byOID" src/go/plugin/go.d/collector/snmp_traps/`). Verify varbind OID lookups are not affected by this issue (varbinds do not have the `.0.` ambiguity).

Artifact impact plan:

- `AGENTS.md`: no update (workflow/responsibility unchanged).
- Runtime project skills: `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` — add a brief note that profile authors may use either form and the receiver tolerates it. Verify.
- Specs: `.agents/sow/specs/snmp-traps/netdata.md` — add a paragraph explaining the receiver tolerance and citing RFC 3584 §3.1.
- End-user/operator docs: `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` — add the tolerance explanation.
- End-user/operator skills: none affected.
- SOW lifecycle: new SOW (this one) in `pending/`, moves to `current/` on activation, then `done/` after completion.

Open-source reference evidence:

- This SOW does not depend on cross-repository implementations beyond RFC text. `mirrored-repos` not consulted (the gosmi / pysmi behavior is observed empirically from our extraction runs; the receiver path is in-repo).

Open decisions:

- D1: helper placement. Should `alternateTrapOID` live in `decode.go` (co-located with `v1TrapOID`) or `profile.go` (co-located with `Lookup`)? **Recommendation**: `profile.go`, because it operates on profile-index keys, not on packet decoding. The decoder does not need to know about index tolerance.
- D2: whether the tolerance should be configurable (i.e., a per-job or global flag to disable). **Recommendation**: NOT configurable. The tolerance is intrinsic to absorbing MIB-tool inconsistency and should be unconditional. Reviewers may dispute.
- D3: whether the alternate-form lookup should also apply to varbind OID resolution (`varbindByOID` at `profile.go:111-117`). **Recommendation**: NO. Varbind OIDs come from real PDU data and are not subject to the SMIv1/v2 trap encoding ambiguity. Their canonical form is unambiguous.
- D4: whether to ship a one-time audit script that scans the existing pack against source MIBs and flags suspected wrong-form entries. **Recommendation**: defer to a separate follow-up SOW about pack content quality. The receiver tolerance fix renders this non-blocking.

## Implications And Decisions

User decisions required before implementation:

1. **Helper placement**:
   - Option A: in `profile.go` (recommended — operates on index keys).
   - Option B: in `decode.go` (alternative — co-located with `v1TrapOID`).
   - Risk/implications: A keeps decode.go focused on packet decoding; B groups all OID-form helpers together but couples decode.go to index concerns.

2. **Configurability of tolerance**:
   - Option A: tolerance is unconditional (recommended).
   - Option B: add a per-job or global flag to disable.
   - Risk/implications: A is simpler and absorbs a real-world MIB-tool defect class transparently. B preserves the option to detect mismatches in future audit work but adds config surface for a behavior most operators never need to toggle.

3. **Scope of the alternate-form lookup**:
   - Option A: trap-OID lookups only (recommended).
   - Option B: also apply to varbind OID lookups.
   - Risk/implications: A is correct because varbind OIDs are unambiguous; B would add unnecessary complexity and create a new false-positive class for varbind matching.

4. **Audit/regeneration scope**:
   - Option A: defer pack-content audit to follow-up SOW (recommended for this SOW).
   - Option B: include a one-time audit script and pack regeneration in this SOW.
   - Risk/implications: A keeps this SOW focused on the durable receiver fix and ships fast. B couples the receiver fix to a much larger remediation effort that may delay shipping.

## Plan

Closed as consolidated. SOW-0035 owns the implementation plan:

1. Implement `alternateTrapOID` helper and modify `ProfileIndex.Lookup` per Implementation plan step 1-2. Single-file change in `profile.go`.
2. Add table-driven unit tests in `profile_test.go` per Implementation plan step 3-4.
3. Update `profile-format.md`, `netdata.md` spec, `project-snmp-trap-profiles-authoring` skill, and `tools/snmp-traps-profile-gen/README.md` per Implementation plan step 5-7.
4. Validate per Validation plan.
5. Close SOW-0035 with this scope included.

## Execution Log

### 2026-05-25

- SOW drafted in `pending/`. Awaiting multi-assistant review per user request.
- No code changes yet. Receiver source confirmed intact at `decode.go:344-351`, `profile.go:161-167`, `collector.go:172`.

### 2026-05-26

- User directed that this SOW be incorporated into ingestion.
- This file is closed as consolidated and moved to `done/`; implementation and validation are tracked in SOW-0035.

## Validation

Acceptance criteria evidence: tracked in SOW-0035.
Tests or equivalent validation: tracked in SOW-0035.
Real-use evidence: tracked in SOW-0035.
Reviewer findings: tracked in SOW-0035.
Same-failure scan: tracked in SOW-0035.
Sensitive data gate: no sensitive data added to this consolidation record; implementation artifacts are governed by SOW-0035.
Artifact maintenance gate: SOW lifecycle action is consolidation into SOW-0035; all other artifact updates are tracked in SOW-0035.

Specs update: pending — `netdata.md` requires a paragraph on receiver tolerance citing RFC 3584 §3.1.

Project skills update: pending — `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` requires a note.

End-user/operator docs update: pending — `profile-format.md` requires the tolerance section.

End-user/operator skills update: pending — none expected unless review finds otherwise.

Lessons: pending.

Follow-up mapping: pending. Likely follow-up: a separate SOW for OOB pack content audit (which entries have the wrong form per source MIB declaration) and optional pack regeneration.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

- Pack content audit: enumerate which shipped entries have the wrong `.0.` form per their source MIB declaration (TRAP-TYPE vs NOTIFICATION-TYPE). Optional pack regeneration to converge on the canonical form. Tracked as a separate SOW once this one completes — the receiver tolerance fix makes the audit non-blocking but still desirable for clean pack hygiene.
- Receiver-side observability: emit a self-metric counter for "trap matched via alternate-form fallback" so we can quantify how often the tolerance is exercised in production. Useful both for confirming the fix is working and for prioritizing the pack audit.

## Regression Log

None yet.
