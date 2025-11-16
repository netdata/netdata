# AI Agent Specifications

## Purpose
Machine-readable behavior specifications for ai-agent. Used to verify implementation consistency, identify test gaps, and validate behavioral contracts.

## Document Structure
Each spec follows this template:
- **TL;DR**: One-line description
- **Source Files**: Code locations
- **Behaviors**: Exhaustive behavior list with conditions
- **Configuration**: Settings that affect behavior
- **Telemetry**: Metrics/traces emitted
- **Logging**: Log entries produced
- **Events**: Events emitted/consumed
- **Test Coverage**: Existing tests and gaps
- **Invariants**: Conditions that must always hold

## Usage
When verifying implementation:
```
Question: "Is implementation aligned with spec X?"
Expected: Systematic review listing:
- Behaviors matching spec
- Behaviors deviating from spec
- Missing behaviors
- Undocumented behaviors
```

## Maintenance Rules
1. Every code change affecting behavior MUST update corresponding spec
2. New features require new spec or spec updates BEFORE implementation
3. Test additions should reference spec behaviors they validate
4. Gaps identified in specs MUST be tracked

## Code References
Format: `src/file.ts:line` or `src/file.ts:start-end`

## Invariants
- All conditions/branches documented
- All error paths documented
- All configuration effects documented
- No prose - facts only

## Business Logic Coverage (Verified 2025-11-16)

- **Multi-agent review loop**: Every spec update must run through Codex + Gemini second-opinion reviews (see TODO-INTERNAL-DOCUMENTATION.md) with discrepancies reconciled before merge; this file now codifies that requirement.
- **Cross-doc syncing**: Specs referencing runtime behavior must also update `docs/AI-AGENT-GUIDE.md` and `docs/specs/index.md` so assistants consuming either source never see divergent facts.
- **Traceable evidence**: Each behavior statement must cite a specific source file/line or deterministic test ID, keeping the specs machine-verifiable.
