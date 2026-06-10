# Vendored from Prometheus

These `.test` files are copied verbatim from the Prometheus repository
under the Apache 2.0 license (the relevant LICENSE notice is in the
parent repository).

Source: https://github.com/prometheus/prometheus
Commit: e793b26713cc7052c7558ae6ceffaa66c2a5b39f
Path:   promql/promqltest/testdata/

Vendored on: 2026-05-14

In-scope file list (features we implement):

* aggregators.test    -- supported
* at_modifier.test    -- supported
* functions.test      -- partial (skip rules in tests/compliance.rs cover
                         histogram_*, info, calendar, trig, mad_over_time,
                         double_exponential_smoothing)
* literals.test       -- supported
* name_label_dropping.test -- partial (no keep_metric_names)
* operators.test      -- supported
* range_queries.test  -- range eval not yet implemented in SOW-0030
* selectors.test      -- supported
* staleness.test      -- partial (we use a fixed 5min lookback)
* subquery.test       -- supported

Out of scope (not vendored): histograms.test, native_histograms.test,
info.test, type_and_unit.test, fill-modifier.test,
duration_expression.test, extended_vectors.test, trig_functions.test,
limit.test, collision.test, start_timestamps.test.
