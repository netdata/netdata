// SPDX-License-Identifier: GPL-3.0-or-later

use std::sync::Arc;

use crate::storage::{Backend, FfiBackend};

/// Evaluation context: query-time parameters threaded through the evaluator.
#[derive(Clone)]
pub struct EvalContext {
    /// Query evaluation time, in Unix milliseconds.
    pub at_ms: i64,
    /// Lookback delta in milliseconds. For an instant vector selector
    /// without samples in `[at_ms - lookback_ms, at_ms]`, the series is
    /// dropped (Prometheus staleness rule). Default: 5 minutes.
    pub lookback_ms: i64,
    /// Host scope. `None` = localhost; `Some("*")` = all hosts; otherwise
    /// the specific host's machine_guid or hostname.
    pub host_machine_guid: Option<String>,
    /// Cardinality backstop passed to the shim on resolve.
    pub max_series: usize,
    /// Outer query range start (Unix ms). For an instant query, equals
    /// `at_ms`. Used by the `@` modifier's `start()` form (SOW-0025).
    pub outer_start_ms: i64,
    /// Outer query range end (Unix ms). For an instant query, equals
    /// `at_ms`. Used by the `@` modifier's `end()` form (SOW-0025).
    pub outer_end_ms: i64,
    /// Storage backend. Production uses `FfiBackend`; compliance tests
    /// and future unit tests inject `MemBackend` or another stand-in.
    /// `Arc<dyn Backend>` so it's cheap to clone across sub-eval
    /// contexts (e.g. subqueries) and across HTTP request handlers.
    pub backend: Arc<dyn Backend>,
}

impl Default for EvalContext {
    fn default() -> Self {
        Self {
            at_ms: 0,
            lookback_ms: 5 * 60 * 1000,
            host_machine_guid: None,
            max_series: 10_000,
            outer_start_ms: 0,
            outer_end_ms: 0,
            backend: Arc::new(FfiBackend),
        }
    }
}

impl EvalContext {
    pub fn at(mut self, at_ms: i64) -> Self {
        self.at_ms = at_ms;
        self
    }

    pub fn host(mut self, host: Option<String>) -> Self {
        self.host_machine_guid = host;
        self
    }
}
