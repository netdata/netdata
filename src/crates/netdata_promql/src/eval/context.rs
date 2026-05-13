// SPDX-License-Identifier: GPL-3.0-or-later

/// Evaluation context: query-time parameters threaded through the evaluator.
#[derive(Debug, Clone)]
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
}

impl Default for EvalContext {
    fn default() -> Self {
        Self {
            at_ms: 0,
            lookback_ms: 5 * 60 * 1000,
            host_machine_guid: None,
            max_series: 10_000,
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
