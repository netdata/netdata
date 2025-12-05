TL;DR
- Goal: single path only (native tools + XML final-report), no transport options; clean dead paths and error handling for final reports.
- Status: code paths consolidated to xml-final only; pending/fallback paths removed; doc updates in progress; lint/build still to run.

Analysis
- Previous state: native, xml, and xml-final transports coexisted; pending final-report handling only worked for native, leaving xml-final paths dead and triggering synthetic failures.
- finalAnswer was set in llm-providers/base.ts from native tool calls; xml-final relied on post-sanitization parsing but still used that flag, causing mismatched logic and synthetic failures when no tool call was found.
- Pending path (text-fallback, tool-message fallback) was effectively dead in xml-final; keeping it added complexity without benefit once native/xml were removed.
- Fix required: simplify transport handling, unify final-report detection on xml-final path, adjust prompts/validation, and remove fallback code guarded by native-only checks.

Decisions (resolved)
1. Remove pending final-report path entirely? **Decision: remove.**
2. Where to compute final-report presence: move from provider to orchestrator post-sanitization? **Decision: yes.**
3. How to handle plain-text endings: allow text-fallback or treat as hard error? **Decision: hard error (no fallback).**

Plan
- Enforce single xml-final path everywhere (code + docs) — done in code, docs updated today.
- Simplify runner: no pending paths; final report via XML only; hard error on missing XML tag — done.
- Clean up provider finalAnswer flag usage; orchestrator decides — done.
- Remove unused code/tests/config tied to transport options; update documentation and samples — docs updated (README, SPECS, GUIDE, DESIGN, IMPLEMENTATION, CONTRACT, tools specs).
- Run lint/build to ensure zero warnings/errors — pending after doc edits.

Implied decisions
- Tests/docs for native/xml will be removed or marked obsolete.
- Prompts instructing models about native tool calls will be updated to xml-final-only guidance.

Testing requirements
- npm run lint
- npm run build

Documentation updates required
- docs/SPECS.md, docs/IMPLEMENTATION.md, docs/DESIGN.md, docs/AI-AGENT-GUIDE.md, README.md (transport defaults/usage, final-report contract)

Progress (Dec 05, 2025)
- Code paths for native/xml removed; xml-final is now the only runtime path.
- Pending/fallback final-report handling removed; hard error on missing XML final_report.
- Success/failure now tied to final report source (model vs synthetic); AJV validation no longer overwrites reports.
- Tool-message fallback wired; text fallback removed; tests passing locally (phase1).
- Documentation updated to reflect single transport path.

Next actions
1) Run `npm run lint` and `npm run build` after doc changes.
2) Keep TODO open until user confirms and lint/build are clean, then delete.
