# AI Agent Internal Specifications

Machine-readable behavior specifications for implementation verification.

## Document Index
See [index.md](index.md) for complete list.

## Purpose
- Verify implementation consistency
- Identify test coverage gaps
- Document all behavioral contracts
- Track configuration effects

## Format
Compact, no prose. All behaviors enumerated with conditions.

## Maintenance (Verified 2025-11-16)

- Every behavior-changing PR must update the relevant spec(s) **and** `docs/specs/index.md`.
- Specs referencing runtime behavior must also update `docs/AI-AGENT-GUIDE.md` to keep AI-facing docs aligned.
- Run Codex + Gemini second-opinion reviews on spec changes and incorporate their findings before merging (per TODO-INTERNAL-DOCUMENTATION.md).
