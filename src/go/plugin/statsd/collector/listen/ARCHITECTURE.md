# StatsD collector internals

This V2 collector receives StatsD over UDP and TCP, applies explicitly selected native profiles, aggregates
measurements and publishes generic native charts plus receiver diagnostics. It runs as the `listen` module of the
standalone Go `statsd.plugin` (`src/go/cmd/statsdplugin`), which source builds include only with
`ENABLE_PLUGIN_STATSD`. Integration documentation and packaging are not delivered, there is no default listener,
transport or enablement, and it does not replace the existing C plugin.

Jobs are read from `statsd/listen.conf` or configured through DynCfg (`config_schema.json` is the form), and profiles
from `statsd/statsd.profiles/` under the user config directory.

## Source layout

| File | Responsibility |
|---|---|
| `collector.go` | `New`, the `Collector` type and the collector interface methods |
| `config.go`, `init.go`, `config_schema.json` | Configuration, defaults and validation; Init helpers; the DynCfg form |
| `run.go` | `Run`: expands listeners into sockets and orders admission around the server's lifetime |
| `internal/server` | Socket acquisition, UDP and TCP framing, the TCP connection cap and listener failure handling |
| `parser.go`, `prepare.go`, `rejections.go` | Record parsing, final label/metadata preparation, the rejection vocabulary |
| `profiles.go` | Profile loading, validation, lifetimes and the replace adapter |
| `receiver.go`, `handoff.go` | Ingest and admission under the receiver lock; the Collect handoff (`cut`/`release`) |
| `aggregate.go`, `internal/percentile` | Numeric updates and interval windows; certified weighted percentiles |
| `collect.go`, `write_metrics.go` | Collect orchestration; application metric writes |
| `chart_templates.go`, `charts.yaml`, `diagnostics.go` | Native template set composition; receiver diagnostics template and metrics |

## Runtime and framing

`listeners` lists required endpoints as `{protocol: udp|tcp|both, address: host:port}` with a numeric port; `both`, also
meant by an empty or omitted protocol, binds a UDP and a TCP socket on that address, so duplicates are checked per
socket and `both` conflicts with `udp` or `tcp` on the same address. `Init` and `Check` validate configuration and load
profiles without binding sockets, so a configuration test can run while an incumbent job owns the endpoints. `Run` binds
every socket in listener order; any failure closes the sockets that attempt already bound and returns, leaving retries
to the framework's configured startup retry. Readiness follows successful acquisition, not traffic. Cancellation stops
admission, closes listeners and clients and joins every reader before `Run` returns. The server hands each framed record
to the receiver and counts bytes, connections and framing rejections in collector-owned counters, so totals survive a
retried `Run`.

Socket errors reporting `Temporary()` (descriptor exhaustion, timeouts) back off 100 ms on the same bound socket; the
Go runtime already retries interrupted calls and aborted connections. Any other listener error while the job is not
stopping, including an unexpectedly closed socket, is permanent listener loss: admission stops, peers close and `Run`
returns the error, so the framework revokes output. A reader panic takes the same path instead of crashing the plugin.
There is no rebind loop or fallback address. A client disconnect or read error ends only that connection.

One framer-owned bound, 64 KiB, limits a record payload excluding its terminator; there is no receive queue.

- UDP: LF or CRLF ends a record and the datagram boundary ends its final record; a bare CR is content. Datagrams are
  never joined. A datagram longer than the bound can only be a truncated read and is rejected whole before any record
  is admitted.
- TCP: every record needs LF or CRLF, including the last; a fragment at EOF is rejected. A record longer than the
  bound is rejected once and discarded through its newline, then reading resumes. Each connection buffers at most one
  bound plus its terminator.
- `max_tcp_connections` caps simultaneous clients across the job's TCP listeners. A new client at capacity is closed
  and counted as refused; established clients keep their slots however quiet they are. There is no idle-client timer.

Empty lines are not records. Every other record is parsed independently, so malformed neighbors do not affect it.

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
UDP copies each datagram, and TCP each run of complete buffered records, once and cuts records as substrings. Parsing
and final preparation reuse fixed receiver-owned label and identity storage that is cleared after every record, so an
admitted update of an existing identity allocates nothing.

## Profiles

`profiles` lists profile names in precedence order. Files `<name>.yaml` or `<name>.yml` are found through
`profilecatalog` in the user configuration directories' `statsd.profiles/`; there is no stock catalog and no
automatic eligibility. `Init` strictly decodes only the selected files, each a single YAML document, so an unrelated
invalid file does not affect the job. Files are not watched: a changed profile applies when the job restarts.

```yaml
match: 'myapp.*'          # required; simple patterns over original StatsD names
relabeling:               # optional; ordered blocks of replace-only rules
  - match: 'myapp.pool.*.size'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'myapp\.pool\.([^.]+)\.size'
        target_label: pool
        replacement: '$1'
      - target_label: __name__
        replacement: myapp.pool.size
template:                 # optional; one native chart template group
  family: pools
  metrics: [g.value.myapp.pool.size]
  charts: [...]
```

A profile needs `relabeling`, `template` or both. Rules use the shared `pkg/relabel` replace semantics; other actions
are rejected. `template` is a `charttpl` group whose selectors name final output metrics
(`<type>.<role>.<encoded-final-name>`); its contexts use the `statsd` namespace and its entry ID is the profile name.

- Preprocessing: the first profile in configured order whose root `match` and some block `match` accept the original
  name owns the input. Only that pipeline runs; pipelines never chain. It changes the name and labels, never the wire
  type, value, rate, gauge operation or member. Metadata tags are visible to it, so a rule may set, rename or remove
  `nd_unit`, `nd_title`, `nd_family` or `measure_field` before final preparation. A rule producing an invalid name
  rejects the record.
- `__name__` addresses the metric name in rules. A sender tag with key `__name__` is an ordinary application label:
  it stays outside the relabel record, invisible to rules, and is restored unchanged.
- Activation: every profile with a template whose root `match` accepts the original name of a fully admitted record
  becomes active. Rejected input activates nothing. Membership only grows until restart; silence does not deactivate.
- Lifetimes: omitted chart and dimension expiry resolve to five successful cycles, authored positive values are kept,
  and a `lifecycle.dimensions` block without a positive `expire_after_cycles` is rejected.

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

`internal/percentile` uses `sketches-go` v1.4.8's logarithmic mapping and two bounded sign stores. A separate
exact-zero bin prevents tiny nonzero observations from silently becoming zero. Both percentiles must satisfy a 1%
relative error bound on the reference value, with exact zero or a gap when the reference is zero.

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
refresh activity. `max_series` defaults to 1,000 identities (provisional); `metric_idle_timeout` is a duration
defaulting to 30 minutes.

`Collect` reconciles the prior metric cycle, retires eligible entries, and detaches one coherent batch. At most one
receiving and one detached/reusable interval exist per identity. Pending observations survive idle expiry until that
handoff; an expired gauge baseline cannot accept deltas. Retained counters/gauges repeat; retained interval types
start an empty window. An unstarted or stopped receiver cannot publish held values or claim an empty healthy interval.
There is no extra expiry timer or sender-cadence inference.

Only Collect constructs direct metrix snapshot handles for application metrics, inside the framework-opened cycle of
a series' first write. The series keeps them: every successful cycle writes every retained series, so their
descriptors never go idle while the handles exist, and the handles end with the series. No Vec or handle map
survives retirement. Generic autogen uses finite expiry of five successful cycles.

The native TemplateSet holds the fixed diagnostics entry plus active profile entries in configured order. The cut
captures membership atomically with its batch, so input activating a profile after a cut belongs to the next cycle
together with its measurements. Collect builds a new set only when membership changed and otherwise keeps the same
pointer; the framework captures it after Collect and before metric commit and owns output reconciliation. A failed
cycle loses its interval but keeps activation, so the next successful publication creates the profile charts.

Declarations retain receiver/batch references and one staged-cycle marker. A later Collect compares metrix's successful
commit clock to resolve whether the previous write committed. Receive cannot infer failure from an unchanged clock
while that cycle is still open. Unreferenced committed metadata retires after
`H = max(DescriptorRetentionWindow, longest chart/dimension expiry + 1)` successful commits since its last write.
The longest expiry covers generic autogen and every prepared profile chart and dimension. Existing store
expiry/grace remain 10/10, so the generic path has `H=20`; a longer authored profile lifetime extends it.
Uncommitted unreferenced declarations retire on reconciliation. This bounds retained cohorts by the accepted series
cap and downstream lifetime, without a new metadata quota. Detached cleanup cannot remove a newly admitted
replacement entry.

Input work is proportional to record size, canonical label sorting and bounded estimator insertion, plus matching the
original name against configured profiles that have rules or are not yet active, and running at most one pipeline.
Profile work is fixed by trusted configuration and the record bound; there is no separate expansion guard. Expiry scans
occur at collection and at admission when an expired entry, type or capacity needs resolution. Admission scans only
once the earliest entry without pending input can have expired, a bound every cut recomputes and each scan refreshes,
so a new identity rejected at capacity costs O(1) otherwise. Per-name metadata aging
is linear in retained declarations. Percentile query sorts at most 2,049 representatives and reuses one Collect-owned
scratch. Native output/retention adds its existing cost. Benchmarks measure the complete path and simultaneous window
ownership; these are not whole-process RSS or end-to-end storage guarantees.

## Diagnostics

The fixed `_receiver` template entry publishes `statsd.receiver.*` charts from collector-owned metrics. They carry
no input text, application names, tags or sender addresses; framework job status and duration remain the only
collection-health charts.

| Metric | Meaning |
|---|---|
| `receiver.bytes{transport}` | Cumulative bytes read per configured transport |
| `receiver.updates` | Cumulative fully admitted records |
| `receiver.rejections{reason}` | Cumulative rejections: `syntax`, `value`, `rate`, `labels`, `metadata`, `type_conflict`, `gauge_baseline`, `capacity`, `overflow`, `oversize`, `unterminated` |
| `receiver.series`, `receiver.series_limit` | Retained identities after the cut and `max_series` |
| `receiver.tcp_connections`, `receiver.tcp_connections_limit` | Open TCP clients and `max_tcp_connections`, TCP jobs only |
| `receiver.tcp_connections_refused` | Cumulative clients closed at `max_tcp_connections`, TCP jobs only |
| `receiver.percentiles_withheld{reason}` | Cumulative ms/h windows with observations whose percentiles were gapped: `numeric_domain`, `observation_bound`, `mapping_domain`, `mapping_error`, `span`, `ambiguous_rank` |

Counters are cumulative, so the Agent computes rates. A quiet interval publishes zero rates, not a failure. Rank
ambiguity is only known at query time and is counted before the window resets. Input arriving while the receiver
is stopped is not published: a stopped receiver has no output.

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

Socket, profile and framework-job tests cover reception through native protocol output. Running-Agent storage is not
validated by this package's tests.
