TL;DR
- Goal: single path only (native tools + XML final-report), no transport options; clean dead paths (pending/report fallbacks) and error handling for final reports.
- Current bug: final-report pending path dead; synthetic errors when model ends without xml final_report.

Analysis
- Transports today: native, xml, xml-final. Session-turn-runner only sets pending reports when toolTransport === 'native'; xml-final never creates pending/accepts it.
- finalAnswer flag is set in llm-providers/base.ts from native tool calls; xml-final relies on post-sanitization parsing but still uses that flag, causing mismatched logic and synthetic failure when no tool call is found.
- Pending path (text-fallback, tool-message fallback) is effectively dead in xml-final; keeping it adds complexity without benefit if native/xml are removed.
- Removal of native/xml means: simplify transport handling, unify final-report detection on xml-final path, adjust prompts/validation, and remove fallback code guarded by native-only checks.

Decisions (pending)
1. Remove pending final-report path entirely? **Decision: remove.**
2. Where to compute final-report presence: move from provider to orchestrator post-sanitization? **Decision: yes.**
3. How to handle plain-text endings: allow text-fallback or treat as hard error? **Decision: hard error (no fallback).**

Plan
- Remove transport options; enforce single path (native tools + XML final-report).
- Simplify runner: no pending paths, final-report detection only via XML parsing, hard error on missing XML tag.
- Clean up provider finalAnswer flag usage; orchestrator decides.
- Remove unused code/tests/config tied to transport options; update documentation and samples.
- Run lint/build to ensure zero warnings/errors.

Implied decisions
- Tests/docs for native/xml will be removed or marked obsolete.
- Prompts instructing models about native tool calls will be updated to xml-final-only guidance.

Testing requirements
- npm run lint
- npm run build

Documentation updates required
- docs/SPECS.md, docs/IMPLEMENTATION.md, docs/DESIGN.md, docs/AI-AGENT-GUIDE.md, README.md (transport defaults/usage, final-report contract)
