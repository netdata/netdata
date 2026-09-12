# Offline Skill Invocation Checks

Use `cases.json` when changing runtime descriptions, task routers or shared selection policy. These are synthetic
selection questions, not instructions to carry out the embedded operations. They cover all runtime skills and include
positive, near-miss, review-lens, uncertain-dependency and authorization cases.

## Run A Walkthrough

1. Give a fresh-context reviewer the case IDs and prompts, root `AGENTS.md#skill-selection`, the actual checkout's
   runtime names/descriptions, and access to relevant entries/owner references. Do not give expected fields before
   the reviewer records its selections. A host's cached skill catalog may differ from the branch being tested.
2. Ask for each case's selected entries, reference depth, needed evidence and action boundary. The reviewer MUST NOT
   execute embedded commands, query services, load credentials, edit files, or launch other agents. A live-operation
   prompt is hypothetical input to this walkthrough, not user authorization to execute it.
3. Compare the recorded answers with `required`, `optional`, `not_selected_by_prompt` and `expect`. Verify apparent
   misses against the current source; the rubric can be wrong or stale. Record commands, revision, model/effort,
   case results, disagreements and limitations in a fresh local audit directory.

## Grade The Contract

- `required` identifies minimum runtime entries for the stated task. Owners/subtree instructions and references may
  also apply; these arrays are not exhaustive reading lists.
- `optional` permits relevant follow-up or uncertain-entry inspection. It does not require every reference in that
  skill. A justified candidate-entry read is acceptable even when it is not listed here.
- `not_selected_by_prompt` means the prompt alone does not select that workflow. It is not a blanket ban if inspection
  finds a real dependency. Penalize forcing unrelated procedures, not a brief entry check that resolves uncertainty.
- `expect` records the important semantic and action boundaries. Missing a required correctness contract, treating a
  reviewer lens as an exemption, broadening a selected write set, or executing hypothetical operations is a material
  miss. A different justified reading order or wording is not a failure.
- Existing user authorization persists. An implementation or live-query scenario may authorize its ordinary steps in
  a real task; merely reading that same scenario during this walkthrough authorizes none of them.

Record each case as supported, material miss, or unresolved, with evidence. Apply the project's bounded review rule:
verify and fix material defects, apply useful suggestions once, and do not add rounds for nits or exact verdict words.
One reviewer still covers the complete assigned unit and its interactions; additional lenses change emphasis.

## Limits

Structural checks can establish valid names, paths and coverage. A fresh-context walkthrough can expose ambiguous
routing and unsafe instructions. Neither establishes production model selection accuracy, token savings, latency,
service compatibility or a statistical improvement over a baseline. A measured evaluation needs repeated held-out
prompts and a separately specified model/configuration comparison; do not report this corpus as such a benchmark.

The query-specific question sets in neighboring directories are operational seeds with separate execution scope.
This offline corpus neither runs them nor changes the runtime skill names or owner contracts.
