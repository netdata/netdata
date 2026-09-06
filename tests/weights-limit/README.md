# Weights result selection tests

The OpenAPI alias regression requires Python 3 and PyYAML. Run from the
repository root:

```sh
python3 tests/weights-limit/weights-openapi-test.py
```

It checks both schema representations and all four weights endpoint references.
Optional aliases must not supply defaults for unchosen fields: a generated
`limit=1000&cardinality_limit=0` request is unlimited because the explicit
cardinality alias takes precedence. The checks preserve caller-supplied zero
and empty values, optionality, and the nonnegative integer constraint. This is
a schema regression; it does not emulate Swagger UI rendering or the HTTP
parser. Changes to request generation should also be checked against the
interactive explorer's Swagger UI bundle in `netdata/learn`.

The standalone C test checks the bounded candidate heap against an independent
full-sort oracle in both score directions, including ties and every cap around
each tested population size. It also selects 1,000 entries from one million
candidates and independently counts the entries stronger than the retained
boundary. Run from the repository root:

```sh
cc -std=c11 -O2 -Wall -Wextra -Werror -fsanitize=address,undefined \
  tests/weights-limit/weights-ranking-test.c -o /tmp/weights-ranking-test
/tmp/weights-ranking-test
```

The benchmark reports candidate-heap storage and comparison cost only. It does
not measure metric evaluation, normalization, dictionary construction, ancestor
marking, or serialization. Full-query scoring retains its existing result
population; selection adds per-result flags and at most N candidate pointers.

The black-box API cases live in `../query-corpus/weights_limit_test.go`. They
stream synthetic fixtures into an isolated Agent and exercise HTTP v1/v2/v3,
legacy chart/context output, grouped aggregation, and MCP caller policies.
Follow `.agents/skills/tests-query-corpus/SKILL.md` when changing or running the
corpus. From `tests/query-corpus`:

```sh
go test -count=1 -run TestWeightsLimit .
go test ./... -count=1
```

First-principles fixtures establish physical membership, complete parent means,
group winners, exact ties, and anomaly percentages. Paired unlimited/limited
responses additionally check unchanged serialized statistics and summaries
(Class C parity). Internal scores are selected before JSON formatting:
`src/libnetdata/buffer/buffer.h:print_netdata_double` rounds ordinary values to
seven fractional digits. Distinct internal scores may therefore look equal on
the wire; serialized-score parity does not establish internal ties or identical
membership in another service that can rank only serialized values. Separate
equal-valued fixtures prove stable semantic tie selection.

Legacy v1 output retains display-name dimension keys. Duplicate display names
can collapse when a client decodes those objects into maps; result-limit counts
describe emitted physical entries, not unique display names. The v2/v3 format
uses dimension IDs and physical rows.

Set `QUERY_CORPUS_WEIGHTS_EXPORT` to a scratch directory to export controlled
response pairs. Each JSON envelope contains the endpoint, encoded query, and
response; node names and identifiers belong only to synthetic corpus fixtures.
The build/source pairing is declared by the corpus harness, not inferred from
the response. These artifacts support downstream integration replay and are
not independent correctness oracles.
