# StatsD measurement core

This package is an unregistered V2 collector under development. It currently provides complete-record ingestion,
aggregation and generic native publication. Network framing, `Run` readiness, profiles and diagnostics are the next
stage; registration, configuration forms, integration documentation and live Agent validation follow that stage.
It does not select listener endpoints, default transports or enablement, and does not replace the existing C plugin yet.

## Input and identity

The framer supplies one complete record without its LF/CRLF terminator. It owns the single record-size limit; the
parser does not impose additional field, member or metadata byte quotas. Each nonempty malformed record rejects as
a whole. Records have the form `name:value|type`, optionally followed by one `|@rate` and one `|#key:value,...` in
either order. Names are nonempty UTF-8 without whitespace, controls, `:` or `|`. Numeric values use decimal/exponent
notation and must parse completely to a finite binary64 value. Hexadecimal, underscore separators and value packing
are unsupported. Set members are exact nonempty strings up to `|`, including spaces, colons and literal `zinit`.

Identical repeated tags collapse; conflicting repeats reject before replacement. A selected profile may replace the
name and labels before `prepareRecord`. It does not change the wire type, numeric value, sampling rate, gauge operation
or member. Final name plus canonical application labels identify a series; aliases therefore share state. One wire
type binds each final logical name across all label instances. Only successful admission establishes that binding.

Final preparation extracts `nd_unit`, `nd_title` and `nd_family`, then validates application labels once. Unknown
case-sensitive `nd_` keys reject. Metadata is name-wide for a wire type and excluded from identity; its first admitted
effective values include defaults. Later omissions inherit them, identical explicit values succeed and conflicts
reject without refreshing activity. Metadata never converts values.

The final label grammar preserves native Agent spelling:

- Keys: 1–199 bytes, ASCII letters/digits and `_ - . / [ ]`, not only underscores.
- Values: 1–799 bytes, valid UTF-8 without controls. ASCII uses the key alphabet plus `: + @ ( )` and single interior
  spaces; non-ASCII text is preserved. Leading/trailing/repeated ASCII spaces and all-underscore values reject.
- `_collect_job` is reserved both on the wire and after replacement. Final `measure_field` is reserved for all types.
  There is no blanket reservation of `le` or `quantile`.
- Metadata uses valid UTF-8 without controls, quotes or backslashes, leading/trailing Unicode whitespace, adjacent
  ASCII spaces or all-underscore text. Other printable punctuation is allowed; there is no separate byte quota.

Accepted retained strings are cloned at admission, so substrings cannot retain complete receive or replacement buffers.

## Measurements

| Type | Accepted updates | Publication | Empty retained interval |
|---|---|---|---|
| `c` | Finite nonnegative values, rate in `(0,1]`; add `value/rate` | Cumulative floating counter; Agent calculates rates | Repeat total |
| `g` | Unsigned absolute assignment or leading-sign delta; rate omitted or 1 | Current floating gauge | Repeat initialized value |
| `ms` | Finite nonnegative observations; weight `1/rate` | Interval min/max/mean/p50/p95, count and sum | Count/sum 0; all five value fields unavailable |
| `h` | Finite signed absolute observations; weight `1/rate` | Same statistics as `ms` | Same as `ms` |
| `s` | Nonempty exact member strings; rate omitted or 1 | Interval HLL cardinality, fixed precision 12 | Exactly 0 |

Gauge deltas need an unexpired absolute baseline. Rejected deltas are never buffered or replayed. To establish a
negative gauge, send an absolute zero followed by a negative delta. Basic numeric updates check all prospective
results for overflow before changing aggregates, metadata, bindings or activity. Histogram/timer count is the sum of
weights, sum is the sum of weighted values, mean is sum/count, and extrema describe received values. Percentile
reliability failure affects only the two percentile fields; the other valid statistics continue.

HLL has statistical error, not a universal per-window guarantee or an exact-small-set contract. No member strings are
retained. Its sparse/dense implementation comes from the existing `axiomhq/hyperloglog` dependency.

## Weighted percentile certification

The reference is the smallest observed value with cumulative weight at least `q * totalWeight`, for `q=1/2` and
`19/20`. Weights are mathematical reciprocals of parsed binary64 rates. For equal weights on 10 and 100, p50 is 10.
The library's built-in quantile does not implement this contract and is not used.

`percentile.go` uses `sketches-go` v1.4.8's logarithmic mapping and two bounded sign stores. A separate exact-zero bin
prevents tiny nonzero observations from silently becoming zero. Both percentiles must satisfy a 1% relative error
bound on the reference value, with exact zero or a gap when the reference is zero.

Weights are normalized by the first rate: `z = firstRate/rate`. Equal sampling rates therefore produce exact unit
weights, including rates such as `.3`. The certification domain is rates at least `2^-128` and at most `2^26`
observations. It bounds normalized weights to `[2^-128,2^128]` and total mass to `2^154`, avoiding underflow/overflow
in mass bounds. Inputs outside this domain can still contribute valid basic statistics.

- Exact lane: `FMA(z,rate,-firstRate)==0` proves exact division within this domain because a nonzero residual cannot
  underflow. The finest dyadic unit and at most `2^53` total integer units make every bin sum exact. Integer comparisons
  `2*C >= W` and `20*C >= 19*W` then implement the reference; their products fit in uint64.
- General lane: with `u=2^-53`, `n<=2^26`, use `delta=4*n*u`. It exceeds the normal-range division and positive
  summation relative-error bound. A stored bin mass `S` encloses its mathematical mass in
  `[S/(1+delta), S/(1-delta)]`, with every operation rounded outward using `Nextafter`. A rank is certified only when
  the preceding cumulative upper bound is below the threshold lower bound and the cumulative lower bound reaches
  the threshold upper bound. Ambiguity gaps both percentiles, rather than guessing a tie.
- Value mapping: mapping accuracy `.009` leaves room below 1%. Each actual representative must pass an outward-rounded
  pointwise 1% check. Unsupported magnitudes or prospective sign spans exceeding 1,024 bins gap both percentiles before
  a store can collapse. Sorting signed representatives is sufficient: the two bounding functions
  `v ± .01*abs(v)` are increasing, so their weighted quantiles bound the representative quantile, including zero.

Failure is sticky for the window and resets at handoff. Availability may depend on arrival order in difficult mixed
rate cases, but published values must always meet the same bound. There is no raw-sample fallback, tail salvage,
library fork, estimator mode or accuracy option. Tests use independent exact rational reciprocals and ranks, mapping
boundaries, ambiguous examples and useful-coverage floors; agreement with library quantiles is not the oracle.

## Ownership and publication

One receiver mutex owns admission, type bindings, activity and receiving aggregates. There is no receive queue or
per-sender state. The positive `max_series` cap covers all job identities; existing admitted identities continue at
capacity. `metric_idle_timeout` applies to accepted input only; zero disables idle retirement. Publication does not
refresh activity. These two defaults are provisional: 1,000 identities and five minutes.

`Collect` reconciles the prior metric cycle, retires eligible entries, and detaches one coherent batch. At most one
receiving and one detached/reusable interval exist per identity. Pending observations survive idle expiry until that
handoff; an expired gauge baseline cannot accept deltas. Retained counters/gauges repeat; retained interval types
start an empty window. An unstarted or stopped receiver cannot publish held values or claim an empty healthy interval.
There is no extra expiry timer or sender-cadence inference.

Only Collect constructs direct metrix snapshot handles, inside the framework-opened cycle. No permanent Vec or
handle map survives retirement. The native TemplateSet pointer is stable and generic autogen uses finite expiry of
five successful cycles. The framework captures that pointer before metric commit and owns output reconciliation.

Declarations retain receiver/batch references and one staged-cycle marker. A later Collect compares metrix's successful
commit clock to resolve whether the previous write committed. Receive cannot infer failure from an unchanged clock
while that cycle is still open. Unreferenced committed metadata retires after
`H = max(DescriptorRetentionWindow, longest chart/dimension expiry + 1)` successful commits since its last write.
Existing store expiry/grace remain 10/10, so the generic path has `H=20`. Uncommitted unreferenced declarations retire
on reconciliation. This bounds retained cohorts by the accepted series cap and downstream lifetime, without a new
metadata quota. Detached cleanup cannot remove a newly admitted replacement entry.

Input work is proportional to record size, canonical label sorting and bounded estimator insertion. Expiry scans
occur at collection and admission when an expired entry, type or capacity needs resolution. Per-name metadata aging
is linear in retained declarations. Percentile query sorts at most 2,049 representatives and reuses one Collect-owned
scratch. Native output/retention adds its existing cost. Benchmarks measure the complete path and simultaneous window
ownership; these are not whole-process RSS or end-to-end storage guarantees.

## Native output and accepted limitations

Names are `<type>.<role>.<encoded-final-name>`: `c.total`, `g.value`, `s.cardinality`, and `ms/h.values`, `count`, `sum`.
The codec preserves ASCII letters/digits/dot/hyphen and encodes every other UTF-8 byte, including underscore, as `_hh`.
MeasureSet fields append `_min`, `_max`, `_mean`, `_p50`, `_p95` without aliases to encoded source names. There is no
synthetic metric-name label. Generic contexts use namespace `statsd`.

Defaults use units `events`, `value`, `members`, `milliseconds`, `value` for c/g/s/ms/h, family `statsd`, and the encoded
final name as title (avoiding unsafe quotes from source names). Native counter autogen adds rate units. For ms/h,
count uses `observations`, values/sum use the declared unit, and count/sum titles identify their role. Unavailable
statistics are full-shaped NaN MeasureSet fields, including completely empty windows.

- Distinct application-label identities can still collide in native chart-ID rendering and merge displayed values
  or labels. The collector adds no collision rejection or synthetic identity label.
- After restart or expiry, a surviving Agent counter baseline can undercount the first new value. There is no
  persistence, priming, generation label or reset-preservation mechanism.
- Interval handoff is best effort. A failure after the cut can lose that interval; there is no replay or output
  acknowledgment. Cancellation before the cut leaves pending input untouched.
- Existing chartengine limits may omit an overlong autogenerated chart for otherwise valid admitted input. There is
  no collector-side full-ID preflight; accepted updates do not imply chart creation or Agent storage.

Running-Agent storage and network behavior are not validated by this package's public framework tests.
