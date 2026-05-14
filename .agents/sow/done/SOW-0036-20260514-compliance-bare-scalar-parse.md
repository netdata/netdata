# SOW-0036 - Compliance - Parse bare scalar expected values

## Status

Status: completed

Sub-state: bare-scalar expected lines in the compliance harness now
parse correctly. Compliance corpus 511 -> 541 (+30 cases; better
than the +20 estimate because `operators.test` and `functions.test`
also use the bare-scalar expected shape). 191 unit tests, 117
smoke, no production-code change.

## Requirements

### Purpose

The `literals.test` compliance file reports 0/25 passes. After
investigation, 20 of the 25 failures are scalar literal tests
(`12.34e6`, `1+1`, `+Inf`, `NaN`, `1/0`, etc.) whose expected
output is a single bare numeric value with no labelset. The
compliance harness's `parse_expected_line` returns `None` for
inputs lacking `{`, so the expected list ends up empty and the
comparator reports "got scalar X; expected 0 labelled series".

The remaining 5 cases are top-level string literals (`"Foo"`,
`""`) which our lowering layer rejects ("top-level string
literals are not supported"). They are a separate concern and
deferred to a follow-up.

### User Request

User chose `literals.test` as the next compliance target. The
investigation hypothesis was confirmed: 20 cases unlock with a
parser fix.

### Acceptance Criteria

1. `parse_expected_line` recognises a bare numeric token (no
   `{` prefix) as a scalar-expected entry with empty labels.
   Handles `NaN`, `+Inf`, `-Inf`, and standard float forms.
2. Compliance corpus: `literals.test` improves from 0/25 to
   ≥ 20/5. Total ≥ 511 (currently) + 20 = ≥ 531.
3. The 5 string-literal cases continue to fail but are tracked
   for a follow-up SOW (top-level string literal support in
   the lowering layer).
4. 191 unit tests pass; 117 smoke pass.

Out of scope: top-level string literals.

## Pre-Implementation Gate

Status: ready. Single-function change in
`tests/compliance.rs::parse_expected_line`.

Sensitive data handling plan: no sensitive data.

## Execution Log

### 2026-05-14

- Reproduced the bug: 20 of 25 `literals.test` failures share
  the pattern "got scalar X; expected 0 labelled series".
  Traced to `parse_expected_line` returning `None` for inputs
  with no `{` and no whitespace before the value (a bare
  `12340000` token).
- Fix: in the else-else branch of `parse_expected_line`, treat
  a bare numeric or `NaN`/`+Inf`/`-Inf` token as
  `ExpectedSeries { metric: None, labels: vec![], values:
  vec![<parsed>] }`.

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status

- (1) `parse_expected_line` recognises bare numeric tokens: **MET**.
- (2) literals.test ≥ 20/5, total ≥ 531: **EXCEEDED**.
  literals.test 20/5/0; total 511 -> 541 (+30 cases).
- (3) 5 string-literal cases still failing, tracked as a
  follow-up: **MET** (string-literal support deferred).
- (4) 191 unit tests + 117 smoke: **MET**.

### Lessons

- Compliance-corpus parser bugs compound: one parser fix
  unlocked more cases than the literals file alone, because
  the same expected-shape (bare scalar) appears in
  `operators.test` and `functions.test` too. Worth grepping
  for "expected 0 labelled series" patterns whenever a new
  category of failures emerges -- the diagnostic message is
  a good fingerprint for parser deficits.
- The fix used an early-return-from-the-else branch rather
  than restructuring the head/tail split. Kept the existing
  parsing flow intact and made the new behaviour easy to read
  and revert if needed.

### Followup

- **Top-level string literals**: 5 of the original 25
  `literals.test` cases still fail at lowering time because
  `Plan::Number/...` etc. don't carry a string variant. The
  lowering layer rejects with "top-level string literals are
  not supported". Small enough to bundle into a future
  literals-completeness SOW.
- The original EXPECTED_FAILS.md note said "almost certainly
  a comparator bug, not 25 real semantic deviations" -- the
  bug turned out to live in the **parser**, not the
  comparator. Update EXPECTED_FAILS.md to record the right
  diagnosis.

## Regression Log

None yet.
